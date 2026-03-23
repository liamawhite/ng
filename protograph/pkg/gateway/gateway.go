// Package gateway implements the protograph query gateway.
// It accepts a POST /protograph/v1/query request, fans out gRPC calls,
// stitches results using (protograph.v1.relation) annotations, and returns
// a single JSON response.
package gateway

import (
	"encoding/json"
	"log"
	"net/http"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// ServiceRegistration binds a gRPC client connection with the FileDescriptors
// that describe its services.
type ServiceRegistration struct {
	// Conn is a gRPC client connection to the server that handles these services.
	Conn *grpc.ClientConn
	// Files lists the FileDescriptors containing the service definitions.
	// Callers should pass the generated File_xxx_proto variables directly:
	//   Files: []protoreflect.FileDescriptor{ngapi.File_ng_proto}
	Files []protoreflect.FileDescriptor
}

// Gateway is an http.Handler that serves POST /protograph/v1/query.
type Gateway struct {
	exec           *executor
	filesByService map[string]serviceInfo
}

// New builds a Gateway from the provided service registrations.
// It extracts (protograph.v1.relation) annotations from all FileDescriptors
// and builds the resolver index.
func New(registrations []ServiceRegistration) (*Gateway, error) {
	// Collect all unique files.
	var allFiles []protoreflect.FileDescriptor
	for _, reg := range registrations {
		allFiles = append(allFiles, resolveFileDescriptors(reg.Files)...)
	}

	relations := ExtractRelations(allFiles)
	log.Printf("protograph: loaded %d relation resolvers", countRelations(relations))

	// Use the first connection for now. For multi-service deployments, extend
	// ServiceRegistration to carry per-service routing.
	var conn *grpc.ClientConn
	if len(registrations) > 0 {
		conn = registrations[0].Conn
	}

	exec := newExecutor(conn, relations, allFiles)
	filesByService := buildServiceIndex(allFiles)

	return &Gateway{exec: exec, filesByService: filesByService}, nil
}

// ServeHTTP handles POST /protograph/v1alpha1/query.
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "protograph: only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	q, err := ParseQuery(r)
	if err != nil {
		http.Error(w, "protograph: bad query: "+err.Error(), http.StatusBadRequest)
		return
	}

	result, err := g.exec.execute(r.Context(), q, g.filesByService)
	if err != nil {
		log.Printf("protograph: execute error: %v", err)
		http.Error(w, "protograph: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		log.Printf("protograph: encode response: %v", err)
	}
}

func countRelations(rels *Relations) int {
	n := 0
	for _, m := range rels.Forward {
		n += len(m)
	}
	for _, m := range rels.Reverse {
		n += len(m)
	}
	return n
}
