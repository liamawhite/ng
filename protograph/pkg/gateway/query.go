package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// FieldSelection is the recursive selection object from the query body.
// An empty map {} means "select this scalar field".
// A non-empty map means "select and recurse into these sub-fields".
type FieldSelection map[string]FieldSelection

// MethodQuery represents a single RPC invocation with its field selections.
type MethodQuery struct {
	// Params holds the "$" key — the request parameters encoded as a map.
	Params map[string]any
	// Fields is the set of response fields to include (and recurse into).
	Fields FieldSelection
}

// Query is the parsed top-level request body:
//
//	map[serviceName → map[methodName → MethodQuery]]
type Query map[string]map[string]MethodQuery

// ParseQuery reads and parses the POST body into a Query.
// The wire format is:
//
//	{
//	  "ng.v1.AreaService": {
//	    "listAreas": {
//	      "$": {},           // request params (empty = no params)
//	      "areas": {         // select the "areas" field
//	        "id": {},
//	        "title": {},
//	        "projects": { "id": {}, "title": {} }
//	      }
//	    }
//	  }
//	}
func ParseQuery(r *http.Request) (Query, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20)) // 4 MiB limit
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	// Unmarshal into a raw map first so we can handle the nested structure.
	var raw map[string]map[string]map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal query: %w", err)
	}

	q := make(Query, len(raw))
	for svcName, methods := range raw {
		q[svcName] = make(map[string]MethodQuery, len(methods))
		for methodName, selection := range methods {
			mq := MethodQuery{
				Params: map[string]any{},
				Fields: make(FieldSelection),
			}
			for k, v := range selection {
				if k == "$" {
					if params, ok := v.(map[string]any); ok {
						mq.Params = params
					}
					continue
				}
				mq.Fields[k] = parseFieldSelection(v)
			}
			q[svcName][methodName] = mq
		}
	}
	return q, nil
}

// parseFieldSelection recursively converts a raw any value into a FieldSelection.
func parseFieldSelection(v any) FieldSelection {
	m, ok := v.(map[string]any)
	if !ok {
		return FieldSelection{}
	}
	fs := make(FieldSelection, len(m))
	for k, child := range m {
		fs[k] = parseFieldSelection(child)
	}
	return fs
}
