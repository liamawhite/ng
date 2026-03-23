package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/liamawhite/ng/api/golang"
	"github.com/liamawhite/ng/backend/pkg/graph"
	"github.com/liamawhite/ng/backend/pkg/server"
	"github.com/liamawhite/ng/backend/pkg/store"
	"github.com/liamawhite/ng/protograph/pkg/gateway"

	// Register the (protograph.v1alpha1.relation) extension so ExtractRelations can find it.
	_ "github.com/liamawhite/ng/api/golang/protograph/v1alpha1"
)

func main() {
	dir := flag.String("dir", "./notes", "directory containing note files")
	port := flag.Int("port", 50051, "gRPC server port")
	httpPort := flag.Int("http-port", 8080, "HTTP gateway port")
	flag.Parse()

	if err := os.MkdirAll(*dir, 0o755); err != nil {
		log.Fatalf("create notes dir: %v", err)
	}

	g := graph.New()
	fs := store.New(*dir, g)

	if err := fs.Load(); err != nil {
		log.Fatalf("load notes: %v", err)
	}

	watcher, err := store.NewWatcher(fs)
	if err != nil {
		log.Fatalf("create watcher: %v", err)
	}
	if err := watcher.Start(); err != nil {
		log.Fatalf("start watcher: %v", err)
	}
	defer watcher.Close()

	areaServer := server.NewAreaServer(fs)
	projectServer := server.NewProjectServer(fs)
	taskServer := server.NewTaskServer(fs)
	graphServer := server.NewGraphServer(fs)

	// --- gRPC server ---
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	// ValidationInterceptor runs for every unary RPC. The HTTP gateway is wired
	// via RegisterXxxHandlerClient (see below) so HTTP requests also pass through
	// this interceptor — at the cost of one loopback TCP round-trip per request.
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(server.ValidationInterceptor),
	)
	api.RegisterAreaServiceServer(grpcServer, areaServer)
	api.RegisterProjectServiceServer(grpcServer, projectServer)
	api.RegisterTaskServiceServer(grpcServer, taskServer)
	api.RegisterGraphServiceServer(grpcServer, graphServer)
	reflection.Register(grpcServer)

	go func() {
		log.Printf("gRPC server listening on :%d", *port)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("grpc serve: %v", err)
		}
	}()

	// --- HTTP gateway (dials gRPC server so interceptors fire for HTTP too) ---
	// Trade-off: RegisterHandlerClient routes every HTTP request through the gRPC
	// server over loopback, adding one TCP round-trip. The alternative,
	// RegisterHandlerServer (direct call, no hop), bypasses interceptors entirely.
	// For a local tool the loopback cost is negligible; revisit for high-throughput
	// deployments.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	grpcAddr := fmt.Sprintf("localhost:%d", *port)
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("dial gRPC for gateway: %v", err)
	}
	defer conn.Close()

	gwMux := runtime.NewServeMux()
	if err := api.RegisterAreaServiceHandlerClient(ctx, gwMux, api.NewAreaServiceClient(conn)); err != nil {
		log.Fatalf("register area gateway: %v", err)
	}
	if err := api.RegisterProjectServiceHandlerClient(ctx, gwMux, api.NewProjectServiceClient(conn)); err != nil {
		log.Fatalf("register project gateway: %v", err)
	}
	if err := api.RegisterTaskServiceHandlerClient(ctx, gwMux, api.NewTaskServiceClient(conn)); err != nil {
		log.Fatalf("register task gateway: %v", err)
	}
	if err := api.RegisterGraphServiceHandlerClient(ctx, gwMux, api.NewGraphServiceClient(conn)); err != nil {
		log.Fatalf("register graph gateway: %v", err)
	}

	pg, err := gateway.New([]gateway.ServiceRegistration{{
		Conn: conn,
		Files: []protoreflect.FileDescriptor{
			api.File_areas_proto,
			api.File_projects_proto,
			api.File_tasks_proto,
			api.File_graph_proto,
		},
	}})
	if err != nil {
		log.Fatalf("create protograph gateway: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", gwMux)
	mux.Handle("/protograph/v1alpha1/", pg)

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", *httpPort),
		Handler: corsMiddleware(mux),
	}

	go func() {
		log.Printf("HTTP gateway listening on :%d", *httpPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http serve: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down...")
	httpServer.Shutdown(context.Background())
	grpcServer.GracefulStop()
}

// corsMiddleware adds CORS headers so browser frontends can call the gateway.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
