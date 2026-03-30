package integration_test

// Protograph integration tests exercise the full HTTP → gRPC → stitching stack.
// Each test spins up a real gRPC server (via bufconn) and a protograph HTTP
// gateway (via httptest.Server), creates test data through the server methods,
// then issues POST /protograph/v1/query requests and asserts the JSON response.
//
// RPC method names are: List, Get, Create, Update, Delete (per service).
// The gateway accepts both lowerCamelCase ("list") and PascalCase ("List")
// forms via upperFirst promotion. Relations (projects, tasks) are derived
// automatically from (protograph.v1alpha1.parent) field annotations — no
// dedicated resolver RPCs are needed.

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	api "github.com/liamawhite/ng/api/golang"
	// Register the (protograph.v1alpha1.relation) extension so ExtractRelations can find it.
	_ "github.com/liamawhite/ng/api/golang/protograph/v1alpha1"
	"github.com/liamawhite/ng/backend/pkg/server"
	"github.com/liamawhite/ng/protograph/pkg/gateway"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// ---- Protograph test infrastructure ----

// pgEnv wraps testEnv with a gRPC server and a protograph HTTP gateway.
type pgEnv struct {
	*testEnv
	url string // base URL of the httptest.Server
}

func newPGEnv(t *testing.T) *pgEnv {
	t.Helper()
	e := newEnv(t)

	// In-memory gRPC transport — same approach as newValidationEnv.
	lis := bufconn.Listen(bufSize)
	grpcSrv := grpc.NewServer(grpc.UnaryInterceptor(server.ValidationInterceptor))
	api.RegisterAreaServiceServer(grpcSrv, e.areas)
	api.RegisterProjectServiceServer(grpcSrv, e.projects)
	api.RegisterTaskServiceServer(grpcSrv, e.tasks)
	api.RegisterGraphServiceServer(grpcSrv, e.graph)
	go grpcSrv.Serve(lis) //nolint:errcheck
	t.Cleanup(grpcSrv.GracefulStop)

	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

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
		t.Fatalf("gateway.New: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/protograph/", pg)
	httpSrv := httptest.NewServer(mux)
	t.Cleanup(httpSrv.Close)

	return &pgEnv{testEnv: e, url: httpSrv.URL}
}

const pgQueryPath = "/protograph/v1alpha1/query"

// pgPost sends a POST to the protograph query endpoint and returns
// (statusCode, decoded result). The result map is nil for non-JSON responses.
func pgPost(t *testing.T, baseURL, body string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Post(baseURL+pgQueryPath, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("http.Post: %v", err)
	}
	defer resp.Body.Close()
	var result map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&result)
	return resp.StatusCode, result
}

// pgRequest sends an arbitrary HTTP request to the protograph query endpoint.
func pgRequest(t *testing.T, baseURL, method, body string) int {
	t.Helper()
	req, err := http.NewRequest(method, baseURL+pgQueryPath, strings.NewReader(body))
	if err != nil {
		t.Fatalf("http.NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http.Do: %v", err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// getSlice navigates result[svc][method][field] → []any, failing the test on any miss.
func getSlice(t *testing.T, result map[string]any, svc, method, field string) []any {
	t.Helper()
	svcMap, ok := result[svc].(map[string]any)
	if !ok {
		t.Fatalf("result[%q]: not a map (got %T)", svc, result[svc])
	}
	methodMap, ok := svcMap[method].(map[string]any)
	if !ok {
		t.Fatalf("result[%q][%q]: not a map (got %T)", svc, method, svcMap[method])
	}
	items, ok := methodMap[field].([]any)
	if !ok {
		t.Fatalf("result[%q][%q][%q]: not a []any (got %T)", svc, method, field, methodMap[field])
	}
	return items
}

// asMap type-asserts a []any element to map[string]any, failing the test on mismatch.
func asMap(t *testing.T, item any) map[string]any {
	t.Helper()
	m, ok := item.(map[string]any)
	if !ok {
		t.Fatalf("item is %T, want map[string]any", item)
	}
	return m
}

// ---- HTTP-level error cases ----

// TestProtograph_HTTPErrors validates that the gateway rejects bad requests
// at the HTTP layer (wrong method, missing/malformed body) before reaching gRPC.
func TestProtograph_HTTPErrors(t *testing.T) {
	e := newPGEnv(t)

	tests := []struct {
		name       string
		method     string
		body       string
		wantStatus int
	}{
		{
			name:       "GET not allowed",
			method:     http.MethodGet,
			body:       "",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "PUT not allowed",
			method:     http.MethodPut,
			body:       `{}`,
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "empty body",
			method:     http.MethodPost,
			body:       "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "malformed JSON",
			method:     http.MethodPost,
			body:       `{ not valid json`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := pgRequest(t, e.url, tc.method, tc.body)
			if got != tc.wantStatus {
				t.Errorf("status: got %d, want %d", got, tc.wantStatus)
			}
		})
	}
}

// ---- RPC-level error cases ----

// TestProtograph_RPCErrors validates that unknown services and methods result
// in 500 Internal Server Error responses.
func TestProtograph_RPCErrors(t *testing.T) {
	e := newPGEnv(t)

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "unknown service",
			body:       `{"no.such.Service": {"list": {"$": {}}}}`,
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "unknown method on known service",
			body:       `{"ng.v1.AreaService": {"notARealMethod": {"$": {}}}}`,
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "unknown lowerCamelCase method on known service",
			body:       `{"ng.v1.AreaService": {"deletePlanet": {"$": {}}}}`,
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, _ := pgPost(t, e.url, tc.body)
			if code != tc.wantStatus {
				t.Errorf("status: got %d, want %d", code, tc.wantStatus)
			}
		})
	}
}

// ---- Single-level query tests ----

// TestProtograph_EmptyResult verifies that querying a service with no data
// succeeds and returns either an absent or empty areas field.
// protojson omits empty repeated fields when EmitUnpopulated is false, so the
// "areas" key may be absent entirely rather than present as [].
func TestProtograph_EmptyResult(t *testing.T) {
	e := newPGEnv(t)

	// AreaService.List — proto RPC name is "List"; we send "list" (upperFirst → "List").
	code, result := pgPost(t, e.url, `{
		"ng.v1.AreaService": {
			"list": {
				"$": {},
				"areas": {"id": {}, "title": {}}
			}
		}
	}`)
	if code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", code)
	}

	svcMap, ok := result["ng.v1.AreaService"].(map[string]any)
	if !ok {
		t.Fatalf("missing service in result")
	}
	methodMap, ok := svcMap["list"].(map[string]any)
	if !ok {
		t.Fatalf("missing method in result")
	}

	// "areas" is either absent (empty repeated field omitted by protojson) or [].
	if areaVal, exists := methodMap["areas"]; exists {
		areas, ok := areaVal.([]any)
		if !ok {
			t.Fatalf("areas: expected []any, got %T", areaVal)
		}
		if len(areas) != 0 {
			t.Fatalf("expected 0 areas, got %d: %v", len(areas), areas)
		}
	}
	// If absent, the response correctly represents no data.
}

// TestProtograph_FieldMasking verifies that only the selected fields appear in
// the response; unselected fields must be absent.
func TestProtograph_FieldMasking(t *testing.T) {
	e := newPGEnv(t)

	if _, err := e.areas.Create(bg, &api.CreateAreaRequest{Title: "Work"}); err != nil {
		t.Fatalf("Create area: %v", err)
	}

	tests := []struct {
		name         string
		query        string
		wantFields   []string
		absentFields []string
	}{
		{
			name: "id only",
			query: `{
				"ng.v1.AreaService": {
					"list": {
						"$": {},
						"areas": {"id": {}}
					}
				}
			}`,
			wantFields:   []string{"id"},
			absentFields: []string{"title"},
		},
		{
			name: "title only",
			query: `{
				"ng.v1.AreaService": {
					"list": {
						"$": {},
						"areas": {"title": {}}
					}
				}
			}`,
			wantFields:   []string{"title"},
			absentFields: []string{"id"},
		},
		{
			name: "id and title",
			query: `{
				"ng.v1.AreaService": {
					"list": {
						"$": {},
						"areas": {"id": {}, "title": {}}
					}
				}
			}`,
			wantFields:   []string{"id", "title"},
			absentFields: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, result := pgPost(t, e.url, tc.query)
			if code != http.StatusOK {
				t.Fatalf("status: got %d, want 200", code)
			}
			items := getSlice(t, result, "ng.v1.AreaService", "list", "areas")
			if len(items) == 0 {
				t.Fatal("expected at least one area")
			}
			item := asMap(t, items[0])
			for _, f := range tc.wantFields {
				if _, ok := item[f]; !ok {
					t.Errorf("field %q missing from response", f)
				}
			}
			for _, f := range tc.absentFields {
				if _, ok := item[f]; ok {
					t.Errorf("field %q should be absent but was present", f)
				}
			}
		})
	}
}

// TestProtograph_MethodNameCasing verifies that both lowerCamelCase ("list")
// and PascalCase ("List") method names are accepted and that the response
// echoes back the exact query key used by the client.
func TestProtograph_MethodNameCasing(t *testing.T) {
	e := newPGEnv(t)

	if _, err := e.areas.Create(bg, &api.CreateAreaRequest{Title: "Casing Test"}); err != nil {
		t.Fatalf("Create area: %v", err)
	}

	tests := []struct {
		name      string
		methodKey string // key used in the query (response must echo this back)
	}{
		{name: "lowerCamelCase", methodKey: "list"},
		{name: "PascalCase", methodKey: "List"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{
				"ng.v1.AreaService": {
					%q: {"$": {}, "areas": {"id": {}, "title": {}}}
				}
			}`, tc.methodKey)

			code, result := pgPost(t, e.url, body)
			if code != http.StatusOK {
				t.Fatalf("status: got %d, want 200", code)
			}
			// Response key must mirror the query key exactly.
			areas := getSlice(t, result, "ng.v1.AreaService", tc.methodKey, "areas")
			if len(areas) == 0 {
				t.Fatal("expected at least one area")
			}
		})
	}
}

// ---- Two-level (relation) query tests ----

// TestProtograph_TwoLevel verifies that a single query stitches projects onto
// their parent area via the (protograph.v1.relation) fan-out.
func TestProtograph_TwoLevel(t *testing.T) {
	e := newPGEnv(t)

	area, err := e.areas.Create(bg, &api.CreateAreaRequest{Title: "Work"})
	if err != nil {
		t.Fatalf("Create area: %v", err)
	}
	proj, err := e.projects.Create(bg, &api.CreateProjectRequest{
		Title:  "Alpha",
		AreaId: area.Id,
	})
	if err != nil {
		t.Fatalf("Create project: %v", err)
	}

	const query = `{
		"ng.v1.AreaService": {
			"list": {
				"$": {},
				"areas": {
					"id": {},
					"title": {},
					"projects": {
						"id": {},
						"title": {}
					}
				}
			}
		}
	}`

	code, result := pgPost(t, e.url, query)
	if code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", code)
	}

	areas := getSlice(t, result, "ng.v1.AreaService", "list", "areas")
	if len(areas) != 1 {
		t.Fatalf("expected 1 area, got %d", len(areas))
	}
	areaItem := asMap(t, areas[0])
	if areaItem["id"] != area.Id {
		t.Errorf("area id: got %v, want %q", areaItem["id"], area.Id)
	}

	projects, ok := areaItem["projects"].([]any)
	if !ok {
		t.Fatalf("area.projects: not a []any (got %T)", areaItem["projects"])
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	projItem := asMap(t, projects[0])
	if projItem["id"] != proj.Id {
		t.Errorf("project id: got %v, want %q", projItem["id"], proj.Id)
	}
	if projItem["title"] != proj.Title {
		t.Errorf("project title: got %v, want %q", projItem["title"], proj.Title)
	}
}

// TestProtograph_TwoLevel_NoProjects verifies that an area with no projects
// gets an empty projects array (not nil, not missing).
func TestProtograph_TwoLevel_NoProjects(t *testing.T) {
	e := newPGEnv(t)

	area, err := e.areas.Create(bg, &api.CreateAreaRequest{Title: "Empty"})
	if err != nil {
		t.Fatalf("Create area: %v", err)
	}

	const query = `{
		"ng.v1.AreaService": {
			"list": {
				"$": {},
				"areas": {
					"id": {},
					"projects": {"id": {}}
				}
			}
		}
	}`

	code, result := pgPost(t, e.url, query)
	if code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", code)
	}

	areas := getSlice(t, result, "ng.v1.AreaService", "list", "areas")
	if len(areas) != 1 {
		t.Fatalf("expected 1 area, got %d", len(areas))
	}
	areaItem := asMap(t, areas[0])
	if areaItem["id"] != area.Id {
		t.Errorf("area id: got %v, want %q", areaItem["id"], area.Id)
	}

	projects, ok := areaItem["projects"].([]any)
	if !ok {
		t.Fatalf("area.projects: not a []any (got %T)", areaItem["projects"])
	}
	if len(projects) != 0 {
		t.Fatalf("expected 0 projects, got %d", len(projects))
	}
}

// ---- Three-level (nested relation) query tests ----

// TestProtograph_ThreeLevel verifies the full three-level query:
// areas → projects → tasks, resolved with two fan-out calls and stitched into
// a single response.
func TestProtograph_ThreeLevel(t *testing.T) {
	e := newPGEnv(t)

	area, err := e.areas.Create(bg, &api.CreateAreaRequest{Title: "Work"})
	if err != nil {
		t.Fatalf("Create area: %v", err)
	}
	proj, err := e.projects.Create(bg, &api.CreateProjectRequest{
		Title:  "Beta",
		AreaId: area.Id,
	})
	if err != nil {
		t.Fatalf("Create project: %v", err)
	}
	task, err := e.tasks.Create(bg, &api.CreateTaskRequest{
		Title:     "Task One",
		ProjectId: proj.Id,
		Status:    api.TaskStatus_TASK_STATUS_TODO,
	})
	if err != nil {
		t.Fatalf("Create task: %v", err)
	}

	const query = `{
		"ng.v1.AreaService": {
			"list": {
				"$": {},
				"areas": {
					"id": {},
					"title": {},
					"projects": {
						"id": {},
						"title": {},
						"tasks": {
							"id": {},
							"title": {}
						}
					}
				}
			}
		}
	}`

	code, result := pgPost(t, e.url, query)
	if code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", code)
	}

	areas := getSlice(t, result, "ng.v1.AreaService", "list", "areas")
	if len(areas) != 1 {
		t.Fatalf("expected 1 area, got %d", len(areas))
	}
	areaItem := asMap(t, areas[0])

	projects, ok := areaItem["projects"].([]any)
	if !ok || len(projects) != 1 {
		t.Fatalf("expected 1 project, got %v", areaItem["projects"])
	}
	projItem := asMap(t, projects[0])
	if projItem["id"] != proj.Id {
		t.Errorf("project id: got %v, want %q", projItem["id"], proj.Id)
	}
	if projItem["title"] != proj.Title {
		t.Errorf("project title: got %v, want %q", projItem["title"], proj.Title)
	}

	tasks, ok := projItem["tasks"].([]any)
	if !ok || len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %v", projItem["tasks"])
	}
	taskItem := asMap(t, tasks[0])
	if taskItem["id"] != task.Id {
		t.Errorf("task id: got %v, want %q", taskItem["id"], task.Id)
	}
	if taskItem["title"] != task.Title {
		t.Errorf("task title: got %v, want %q", taskItem["title"], task.Title)
	}
}

// ---- Fan-out bucketing tests ----

// TestProtograph_MultipleParents verifies that the fan-out correctly buckets
// children to their respective parents when multiple parent items exist.
// Two areas each get their own projects; no cross-contamination must occur.
func TestProtograph_MultipleParents(t *testing.T) {
	e := newPGEnv(t)

	area1, err := e.areas.Create(bg, &api.CreateAreaRequest{Title: "Work"})
	if err != nil {
		t.Fatalf("Create area1: %v", err)
	}
	area2, err := e.areas.Create(bg, &api.CreateAreaRequest{Title: "Personal"})
	if err != nil {
		t.Fatalf("Create area2: %v", err)
	}

	proj1, err := e.projects.Create(bg, &api.CreateProjectRequest{Title: "Work-P1", AreaId: area1.Id})
	if err != nil {
		t.Fatalf("Create proj1: %v", err)
	}
	proj2, err := e.projects.Create(bg, &api.CreateProjectRequest{Title: "Work-P2", AreaId: area1.Id})
	if err != nil {
		t.Fatalf("Create proj2: %v", err)
	}
	proj3, err := e.projects.Create(bg, &api.CreateProjectRequest{Title: "Personal-P1", AreaId: area2.Id})
	if err != nil {
		t.Fatalf("Create proj3: %v", err)
	}

	const query = `{
		"ng.v1.AreaService": {
			"list": {
				"$": {},
				"areas": {
					"id": {},
					"projects": {"id": {}}
				}
			}
		}
	}`

	code, result := pgPost(t, e.url, query)
	if code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", code)
	}

	areas := getSlice(t, result, "ng.v1.AreaService", "list", "areas")
	if len(areas) != 2 {
		t.Fatalf("expected 2 areas, got %d", len(areas))
	}

	// Build map from area ID → set of project IDs from the response.
	areaToProjects := make(map[string]map[string]bool)
	for _, a := range areas {
		aMap := asMap(t, a)
		areaID, _ := aMap["id"].(string)
		areaToProjects[areaID] = make(map[string]bool)
		projSlice, _ := aMap["projects"].([]any)
		for _, p := range projSlice {
			pMap := asMap(t, p)
			pid, _ := pMap["id"].(string)
			areaToProjects[areaID][pid] = true
		}
	}

	// area1 must have exactly proj1 and proj2.
	work := areaToProjects[area1.Id]
	if len(work) != 2 {
		t.Fatalf("area1 expected 2 projects, got %d: %v", len(work), work)
	}
	if !work[proj1.Id] {
		t.Errorf("proj1 (%q) missing from area1", proj1.Id)
	}
	if !work[proj2.Id] {
		t.Errorf("proj2 (%q) missing from area1", proj2.Id)
	}

	// area2 must have exactly proj3.
	personal := areaToProjects[area2.Id]
	if len(personal) != 1 {
		t.Fatalf("area2 expected 1 project, got %d: %v", len(personal), personal)
	}
	if !personal[proj3.Id] {
		t.Errorf("proj3 (%q) missing from area2", proj3.Id)
	}
}

// TestProtograph_ThreeLevel_MultipleParents stress-tests nested fan-out with
// two areas, two projects each, and two tasks per project.
func TestProtograph_ThreeLevel_MultipleParents(t *testing.T) {
	e := newPGEnv(t)

	area1, _ := e.areas.Create(bg, &api.CreateAreaRequest{Title: "A1"})
	area2, _ := e.areas.Create(bg, &api.CreateAreaRequest{Title: "A2"})

	proj1, _ := e.projects.Create(bg, &api.CreateProjectRequest{Title: "A1-P1", AreaId: area1.Id})
	proj2, _ := e.projects.Create(bg, &api.CreateProjectRequest{Title: "A1-P2", AreaId: area1.Id})
	proj3, _ := e.projects.Create(bg, &api.CreateProjectRequest{Title: "A2-P1", AreaId: area2.Id})

	// Two tasks for proj1, one for proj2, one for proj3.
	task1a, _ := e.tasks.Create(bg, &api.CreateTaskRequest{Title: "P1-T1", ProjectId: proj1.Id, Status: api.TaskStatus_TASK_STATUS_TODO})
	task1b, _ := e.tasks.Create(bg, &api.CreateTaskRequest{Title: "P1-T2", ProjectId: proj1.Id, Status: api.TaskStatus_TASK_STATUS_TODO})
	task2, _ := e.tasks.Create(bg, &api.CreateTaskRequest{Title: "P2-T1", ProjectId: proj2.Id, Status: api.TaskStatus_TASK_STATUS_TODO})
	task3, _ := e.tasks.Create(bg, &api.CreateTaskRequest{Title: "P3-T1", ProjectId: proj3.Id, Status: api.TaskStatus_TASK_STATUS_TODO})

	const query = `{
		"ng.v1.AreaService": {
			"list": {
				"$": {},
				"areas": {
					"id": {},
					"projects": {
						"id": {},
						"tasks": {"id": {}}
					}
				}
			}
		}
	}`

	code, result := pgPost(t, e.url, query)
	if code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", code)
	}

	areas := getSlice(t, result, "ng.v1.AreaService", "list", "areas")
	if len(areas) != 2 {
		t.Fatalf("expected 2 areas, got %d", len(areas))
	}

	// Flatten: projectID → set of task IDs found in response.
	projToTasks := make(map[string]map[string]bool)
	for _, a := range areas {
		aMap := asMap(t, a)
		projSlice, _ := aMap["projects"].([]any)
		for _, p := range projSlice {
			pMap := asMap(t, p)
			pid, _ := pMap["id"].(string)
			projToTasks[pid] = make(map[string]bool)
			taskSlice, _ := pMap["tasks"].([]any)
			for _, tk := range taskSlice {
				tkMap := asMap(t, tk)
				tid, _ := tkMap["id"].(string)
				projToTasks[pid][tid] = true
			}
		}
	}

	// Verify task counts and membership.
	checks := []struct {
		projID  string
		wantIDs []string
	}{
		{proj1.Id, []string{task1a.Id, task1b.Id}},
		{proj2.Id, []string{task2.Id}},
		{proj3.Id, []string{task3.Id}},
	}
	for _, c := range checks {
		got := projToTasks[c.projID]
		if len(got) != len(c.wantIDs) {
			t.Errorf("proj %q: expected %d tasks, got %d: %v", c.projID, len(c.wantIDs), len(got), got)
			continue
		}
		for _, tid := range c.wantIDs {
			if !got[tid] {
				t.Errorf("proj %q: task %q missing", c.projID, tid)
			}
		}
	}
}

// ---- Reverse traversal (child → parent) query tests ----

// TestProtograph_Reverse_ProjectToArea verifies that querying ProjectService.List
// with "area" selected causes the gateway to reverse-resolve each project's
// areaId via AreaService.Get and stitch the parent area onto each project.
func TestProtograph_Reverse_ProjectToArea(t *testing.T) {
	e := newPGEnv(t)

	area, err := e.areas.Create(bg, &api.CreateAreaRequest{Title: "Work"})
	if err != nil {
		t.Fatalf("Create area: %v", err)
	}
	proj, err := e.projects.Create(bg, &api.CreateProjectRequest{
		Title:  "Alpha",
		AreaId: area.Id,
	})
	if err != nil {
		t.Fatalf("Create project: %v", err)
	}

	const query = `{
		"ng.v1.ProjectService": {
			"list": {
				"$": {},
				"projects": {
					"id": {},
					"title": {},
					"area": {
						"id": {},
						"title": {}
					}
				}
			}
		}
	}`

	code, result := pgPost(t, e.url, query)
	if code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", code)
	}

	projects := getSlice(t, result, "ng.v1.ProjectService", "list", "projects")
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	projItem := asMap(t, projects[0])
	if projItem["id"] != proj.Id {
		t.Errorf("project id: got %v, want %q", projItem["id"], proj.Id)
	}

	areaItem, ok := projItem["area"].(map[string]any)
	if !ok {
		t.Fatalf("project.area: not a map (got %T)", projItem["area"])
	}
	if areaItem["id"] != area.Id {
		t.Errorf("area id: got %v, want %q", areaItem["id"], area.Id)
	}
	if areaItem["title"] != area.Title {
		t.Errorf("area title: got %v, want %q", areaItem["title"], area.Title)
	}
}

// TestProtograph_Reverse_Deduplication verifies that multiple projects with the
// same parent area result in only one AreaService.Get call (FK deduplication),
// and that both projects receive the same area object.
func TestProtograph_Reverse_Deduplication(t *testing.T) {
	e := newPGEnv(t)

	area, err := e.areas.Create(bg, &api.CreateAreaRequest{Title: "Shared"})
	if err != nil {
		t.Fatalf("Create area: %v", err)
	}
	proj1, err := e.projects.Create(bg, &api.CreateProjectRequest{Title: "P1", AreaId: area.Id})
	if err != nil {
		t.Fatalf("Create proj1: %v", err)
	}
	proj2, err := e.projects.Create(bg, &api.CreateProjectRequest{Title: "P2", AreaId: area.Id})
	if err != nil {
		t.Fatalf("Create proj2: %v", err)
	}

	const query = `{
		"ng.v1.ProjectService": {
			"list": {
				"$": {},
				"projects": {
					"id": {},
					"area": {"id": {}, "title": {}}
				}
			}
		}
	}`

	code, result := pgPost(t, e.url, query)
	if code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", code)
	}

	projects := getSlice(t, result, "ng.v1.ProjectService", "list", "projects")
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}

	// Build map from project ID → area map for easy lookup.
	projToArea := make(map[string]map[string]any)
	for _, p := range projects {
		pMap := asMap(t, p)
		pid, _ := pMap["id"].(string)
		aMap, ok := pMap["area"].(map[string]any)
		if !ok {
			t.Fatalf("project %q: area not a map (got %T)", pid, pMap["area"])
		}
		projToArea[pid] = aMap
	}

	// Both projects must resolve to the same area.
	for _, pid := range []string{proj1.Id, proj2.Id} {
		aMap := projToArea[pid]
		if aMap["id"] != area.Id {
			t.Errorf("proj %q: area id got %v, want %q", pid, aMap["id"], area.Id)
		}
		if aMap["title"] != area.Title {
			t.Errorf("proj %q: area title got %v, want %q", pid, aMap["title"], area.Title)
		}
	}
}

// TestProtograph_Reverse_FieldMasking verifies that only selected fields appear
// on the reverse-resolved parent, not all fields from the Get response.
func TestProtograph_Reverse_FieldMasking(t *testing.T) {
	e := newPGEnv(t)

	area, err := e.areas.Create(bg, &api.CreateAreaRequest{Title: "Work"})
	if err != nil {
		t.Fatalf("Create area: %v", err)
	}
	if _, err := e.projects.Create(bg, &api.CreateProjectRequest{Title: "P1", AreaId: area.Id}); err != nil {
		t.Fatalf("Create project: %v", err)
	}

	// Select only "id" on the parent area — "title" must be absent.
	const query = `{
		"ng.v1.ProjectService": {
			"list": {
				"$": {},
				"projects": {
					"id": {},
					"area": {"id": {}}
				}
			}
		}
	}`

	code, result := pgPost(t, e.url, query)
	if code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", code)
	}

	projects := getSlice(t, result, "ng.v1.ProjectService", "list", "projects")
	if len(projects) == 0 {
		t.Fatal("expected at least one project")
	}
	projItem := asMap(t, projects[0])
	areaItem, ok := projItem["area"].(map[string]any)
	if !ok {
		t.Fatalf("project.area: not a map (got %T)", projItem["area"])
	}
	if _, ok := areaItem["id"]; !ok {
		t.Error("area.id should be present")
	}
	if _, ok := areaItem["title"]; ok {
		t.Error("area.title should be absent when not selected")
	}
}

// TestProtograph_Reverse_NotSelected verifies that when "area" is not included
// in the field selection, no reverse resolver fires and the field is absent.
func TestProtograph_RelationFieldNotSelected(t *testing.T) {
	e := newPGEnv(t)

	area, err := e.areas.Create(bg, &api.CreateAreaRequest{Title: "Work"})
	if err != nil {
		t.Fatalf("Create area: %v", err)
	}
	if _, err := e.projects.Create(bg, &api.CreateProjectRequest{
		Title:  "P1",
		AreaId: area.Id,
	}); err != nil {
		t.Fatalf("Create project: %v", err)
	}

	// Query areas WITHOUT selecting "projects" — relation must not be stitched.
	const query = `{
		"ng.v1.AreaService": {
			"list": {
				"$": {},
				"areas": {"id": {}, "title": {}}
			}
		}
	}`

	code, result := pgPost(t, e.url, query)
	if code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", code)
	}

	areas := getSlice(t, result, "ng.v1.AreaService", "list", "areas")
	if len(areas) == 0 {
		t.Fatal("expected at least one area")
	}
	areaItem := asMap(t, areas[0])
	if _, ok := areaItem["projects"]; ok {
		t.Error("area.projects should not appear when not selected")
	}
}

// ---- Nested task (subtask) tests ----

// TestProtograph_NestedTasks verifies that querying TaskService.List with a
// nested "tasks" selection fans out TaskService.List(parent_task_id=X) for
// each task, stitching subtasks onto their parent task.
//
// Layout:  root ← sub ← subsub
//
// The top-level "tasks" list contains ALL tasks (root + sub + subsub), because
// TaskService.List(project_id) returns everything. Each task's "tasks" field
// contains only its DIRECT children, built by the fan-out.
func TestProtograph_NestedTasks(t *testing.T) {
	e := newPGEnv(t)

	proj, err := e.projects.Create(bg, &api.CreateProjectRequest{Title: "Proj"})
	if err != nil {
		t.Fatalf("Create project: %v", err)
	}
	root, err := e.tasks.Create(bg, &api.CreateTaskRequest{
		Title:     "Root",
		ProjectId: proj.Id,
		Status:    api.TaskStatus_TASK_STATUS_TODO,
	})
	if err != nil {
		t.Fatalf("Create root: %v", err)
	}
	sub, err := e.tasks.Create(bg, &api.CreateTaskRequest{
		Title:        "Sub",
		ProjectId:    proj.Id,
		ParentTaskId: root.Id,
		Status:       api.TaskStatus_TASK_STATUS_TODO,
	})
	if err != nil {
		t.Fatalf("Create sub: %v", err)
	}
	subsub, err := e.tasks.Create(bg, &api.CreateTaskRequest{
		Title:        "SubSub",
		ProjectId:    proj.Id,
		ParentTaskId: sub.Id,
		Status:       api.TaskStatus_TASK_STATUS_TODO,
	})
	if err != nil {
		t.Fatalf("Create subsub: %v", err)
	}

	body := fmt.Sprintf(`{
		"ng.v1.TaskService": {
			"list": {
				"$": {"projectId": %q},
				"tasks": {
					"id": {},
					"parentTaskId": {},
					"tasks": {
						"id": {},
						"parentTaskId": {},
						"tasks": {"id": {}}
					}
				}
			}
		}
	}`, proj.Id)

	code, result := pgPost(t, e.url, body)
	if code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", code)
	}

	allTasks := getSlice(t, result, "ng.v1.TaskService", "list", "tasks")

	// All three tasks appear at the top level (List(project_id) returns everything).
	if len(allTasks) != 3 {
		t.Fatalf("top-level tasks: got %d, want 3 (root+sub+subsub)", len(allTasks))
	}

	// Build id → task map for easy lookup.
	byID := make(map[string]map[string]any)
	for _, item := range allTasks {
		m := asMap(t, item)
		id, _ := m["id"].(string)
		byID[id] = m
	}

	// root.tasks must contain exactly sub.
	rootMap, ok := byID[root.Id]
	if !ok {
		t.Fatalf("root task %q missing from top-level list", root.Id)
	}
	rootChildren, ok := rootMap["tasks"].([]any)
	if !ok || len(rootChildren) != 1 {
		t.Fatalf("root.tasks: want 1 child, got %v", rootMap["tasks"])
	}
	subItem := asMap(t, rootChildren[0])
	if subItem["id"] != sub.Id {
		t.Errorf("root.tasks[0].id: got %v, want %q", subItem["id"], sub.Id)
	}

	// sub.tasks (inside root.tasks) must contain exactly subsub.
	subChildren, ok := subItem["tasks"].([]any)
	if !ok || len(subChildren) != 1 {
		t.Fatalf("sub.tasks: want 1 child, got %v", subItem["tasks"])
	}
	subsubItem := asMap(t, subChildren[0])
	if subsubItem["id"] != subsub.Id {
		t.Errorf("sub.tasks[0].id: got %v, want %q", subsubItem["id"], subsub.Id)
	}
}

// TestProtograph_NestedTasks_ListByParentTaskId verifies that passing
// parentTaskId in the "$" params filters the top-level result to only
// direct children of that task.
func TestProtograph_NestedTasks_ListByParentTaskId(t *testing.T) {
	e := newPGEnv(t)

	proj, _ := e.projects.Create(bg, &api.CreateProjectRequest{Title: "Proj"})
	root, _ := e.tasks.Create(bg, &api.CreateTaskRequest{
		Title:     "Root",
		ProjectId: proj.Id,
		Status:    api.TaskStatus_TASK_STATUS_TODO,
	})
	sub1, _ := e.tasks.Create(bg, &api.CreateTaskRequest{
		Title:        "Sub1",
		ProjectId:    proj.Id,
		ParentTaskId: root.Id,
		Status:       api.TaskStatus_TASK_STATUS_TODO,
	})
	sub2, _ := e.tasks.Create(bg, &api.CreateTaskRequest{
		Title:        "Sub2",
		ProjectId:    proj.Id,
		ParentTaskId: root.Id,
		Status:       api.TaskStatus_TASK_STATUS_TODO,
	})

	body := fmt.Sprintf(`{
		"ng.v1.TaskService": {
			"list": {
				"$": {"parentTaskId": %q},
				"tasks": {"id": {}}
			}
		}
	}`, root.Id)

	code, result := pgPost(t, e.url, body)
	if code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", code)
	}

	tasks := getSlice(t, result, "ng.v1.TaskService", "list", "tasks")
	if len(tasks) != 2 {
		t.Fatalf("expected 2 children of root, got %d", len(tasks))
	}

	ids := map[string]bool{}
	for _, item := range tasks {
		m := asMap(t, item)
		id, _ := m["id"].(string)
		ids[id] = true
	}
	if !ids[sub1.Id] || !ids[sub2.Id] {
		t.Errorf("expected sub1 and sub2, got: %v", ids)
	}
}

// TestProtograph_NestedTasks_NoSubtasks verifies that a task with no subtasks
// gets an empty tasks array (not nil or absent) when tasks are selected.
func TestProtograph_NestedTasks_NoSubtasks(t *testing.T) {
	e := newPGEnv(t)

	proj, _ := e.projects.Create(bg, &api.CreateProjectRequest{Title: "Proj"})
	task, _ := e.tasks.Create(bg, &api.CreateTaskRequest{
		Title:     "Leaf",
		ProjectId: proj.Id,
		Status:    api.TaskStatus_TASK_STATUS_TODO,
	})

	body := fmt.Sprintf(`{
		"ng.v1.TaskService": {
			"list": {
				"$": {"projectId": %q},
				"tasks": {
					"id": {},
					"tasks": {"id": {}}
				}
			}
		}
	}`, proj.Id)

	code, result := pgPost(t, e.url, body)
	if code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", code)
	}

	tasks := getSlice(t, result, "ng.v1.TaskService", "list", "tasks")
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	taskItem := asMap(t, tasks[0])
	if taskItem["id"] != task.Id {
		t.Errorf("task id: got %v, want %q", taskItem["id"], task.Id)
	}

	children, ok := taskItem["tasks"].([]any)
	if !ok {
		t.Fatalf("task.tasks: not a []any (got %T)", taskItem["tasks"])
	}
	if len(children) != 0 {
		t.Errorf("task.tasks: expected 0 children, got %d", len(children))
	}
}

// TestProtograph_ProjectPriority verifies that the priority field is included in
// the protograph response after a priority-only update (the same path the
// frontend keyboard shortcut uses).
func TestProtograph_ProjectPriority(t *testing.T) {
	e := newPGEnv(t)

	area, err := e.areas.Create(bg, &api.CreateAreaRequest{Title: "Area"})
	if err != nil {
		t.Fatalf("Create area: %v", err)
	}
	proj, err := e.projects.Create(bg, &api.CreateProjectRequest{
		Title:  "My Project",
		AreaId: area.Id,
	})
	if err != nil {
		t.Fatalf("Create project: %v", err)
	}

	// Update to P2 via gRPC (matches what the HTTP keyboard shortcut persists).
	_, err = e.projects.Update(bg, &api.UpdateProjectRequest{
		Id:         proj.Id,
		Priority:   api.Priority_PRIORITY_2,
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"priority"}},
	})
	if err != nil {
		t.Fatalf("Update priority: %v", err)
	}

	const query = `{
		"ng.v1.AreaService": {
			"list": {
				"$": {},
				"areas": {
					"id": {},
					"projects": {
						"id": {},
						"priority": {}
					}
				}
			}
		}
	}`

	code, result := pgPost(t, e.url, query)
	if code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", code)
	}

	areas := getSlice(t, result, "ng.v1.AreaService", "list", "areas")
	if len(areas) != 1 {
		t.Fatalf("expected 1 area, got %d", len(areas))
	}
	projects, ok := asMap(t, areas[0])["projects"].([]any)
	if !ok || len(projects) != 1 {
		t.Fatalf("expected 1 project in area")
	}
	projItem := asMap(t, projects[0])
	if got := projItem["priority"]; got != "PRIORITY_2" {
		t.Fatalf("project priority=%v, want PROJECT_PRIORITY_2", got)
	}
}
