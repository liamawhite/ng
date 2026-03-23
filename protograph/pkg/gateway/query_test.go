package gateway

import (
	"net/http"
	"strings"
	"testing"
)

func TestParseQuery(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
		check   func(t *testing.T, q Query)
	}{
		{
			name: "single service single method no params",
			body: `{
				"svc.v1.FooService": {
					"listFoos": {
						"$": {},
						"foos": { "id": {}, "name": {} }
					}
				}
			}`,
			check: func(t *testing.T, q Query) {
				svc, ok := q["svc.v1.FooService"]
				if !ok {
					t.Fatal("missing service key")
				}
				mq, ok := svc["listFoos"]
				if !ok {
					t.Fatal("missing method key")
				}
				if len(mq.Params) != 0 {
					t.Fatalf("params: got %v, want empty", mq.Params)
				}
				if _, ok := mq.Fields["foos"]; !ok {
					t.Fatal("missing foos field")
				}
				if _, ok := mq.Fields["foos"]["id"]; !ok {
					t.Fatal("missing foos.id field")
				}
				if _, ok := mq.Fields["foos"]["name"]; !ok {
					t.Fatal("missing foos.name field")
				}
			},
		},
		{
			name: "params passed through",
			body: `{
				"svc.v1.FooService": {
					"getFoo": {
						"$": { "id": "abc-123" },
						"id": {},
						"name": {}
					}
				}
			}`,
			check: func(t *testing.T, q Query) {
				mq := q["svc.v1.FooService"]["getFoo"]
				if v, ok := mq.Params["id"]; !ok || v != "abc-123" {
					t.Fatalf("params.id: got %v, want abc-123", v)
				}
				if _, ok := mq.Fields["id"]; !ok {
					t.Fatal("missing id field in Fields")
				}
				if _, ok := mq.Fields["$"]; ok {
					t.Fatal("$ must not appear in Fields")
				}
			},
		},
		{
			name: "nested field selection",
			body: `{
				"svc.v1.FooService": {
					"listFoos": {
						"$": {},
						"foos": {
							"id": {},
							"bars": {
								"id": {},
								"value": {}
							}
						}
					}
				}
			}`,
			check: func(t *testing.T, q Query) {
				fields := q["svc.v1.FooService"]["listFoos"].Fields
				bars, ok := fields["foos"]["bars"]
				if !ok {
					t.Fatal("missing foos.bars")
				}
				if _, ok := bars["id"]; !ok {
					t.Fatal("missing foos.bars.id")
				}
				if _, ok := bars["value"]; !ok {
					t.Fatal("missing foos.bars.value")
				}
			},
		},
		{
			name: "multiple services",
			body: `{
				"svc.v1.FooService": { "listFoos": { "$": {}, "foos": { "id": {} } } },
				"svc.v1.BarService": { "listBars": { "$": {}, "bars": { "id": {} } } }
			}`,
			check: func(t *testing.T, q Query) {
				if _, ok := q["svc.v1.FooService"]; !ok {
					t.Fatal("missing FooService")
				}
				if _, ok := q["svc.v1.BarService"]; !ok {
					t.Fatal("missing BarService")
				}
			},
		},
		{
			name: "multiple methods in one service",
			body: `{
				"svc.v1.FooService": {
					"listFoos": { "$": {}, "foos": { "id": {} } },
					"getFoo":   { "$": { "id": "x" }, "id": {} }
				}
			}`,
			check: func(t *testing.T, q Query) {
				svc := q["svc.v1.FooService"]
				if len(svc) != 2 {
					t.Fatalf("got %d methods, want 2", len(svc))
				}
			},
		},
		{
			name:    "invalid json",
			body:    `{ not valid json`,
			wantErr: true,
		},
		{
			name:    "empty body",
			body:    ``,
			wantErr: true,
		},
		{
			name: "empty selection tree is valid",
			body: `{ "svc.v1.FooService": { "listFoos": { "$": {} } } }`,
			check: func(t *testing.T, q Query) {
				mq := q["svc.v1.FooService"]["listFoos"]
				if len(mq.Fields) != 0 {
					t.Fatalf("expected empty fields, got %v", mq.Fields)
				}
			},
		},
		{
			name: "missing $ key uses empty params",
			body: `{ "svc.v1.FooService": { "listFoos": { "id": {} } } }`,
			check: func(t *testing.T, q Query) {
				mq := q["svc.v1.FooService"]["listFoos"]
				if mq.Params == nil {
					t.Fatal("Params should default to empty map, not nil")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			q, err := ParseQuery(req)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.check != nil {
				tc.check(t, q)
			}
		})
	}
}
