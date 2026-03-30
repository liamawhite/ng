package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	api "github.com/liamawhite/ng/api/golang"
	"github.com/liamawhite/ng/backend/pkg/server"
	"github.com/liamawhite/ng/backend/pkg/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// gwEnv wraps testEnv with a full grpc-gateway HTTP server, mirroring the real backend.
type gwEnv struct {
	*testEnv
	url string
}

func newGWEnv(t *testing.T) *gwEnv {
	t.Helper()
	e := newEnv(t)

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

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	gwMux := runtime.NewServeMux()
	if err := api.RegisterProjectServiceHandlerClient(ctx, gwMux, api.NewProjectServiceClient(conn)); err != nil {
		t.Fatalf("register project gateway: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", gwMux)
	httpSrv := httptest.NewServer(mux)
	t.Cleanup(httpSrv.Close)

	return &gwEnv{testEnv: e, url: httpSrv.URL}
}

// ---- API-based invariant tests ----

// TestProjectCRUD_API verifies the full create/get/update/delete lifecycle.
// Invariants: ID stability; field consistency between Create and Get; updates
// reflected on disk; deleted node returns NotFound and file is removed.
func TestProjectCRUD_API(t *testing.T) {
	e := newEnv(t)

	proj, err := e.projects.Create(bg, &api.CreateProjectRequest{
		Title:   "Alpha",
		Content: "alpha body",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if proj.Id == "" {
		t.Fatal("Create returned empty ID")
	}
	if proj.Title != "Alpha" || proj.Content != "alpha body" {
		t.Fatalf("Create: got title=%q content=%q", proj.Title, proj.Content)
	}
	if !fileExists(e.dir, proj.Id) {
		t.Fatal("Create: file not on disk")
	}

	got, err := e.projects.Get(bg, &api.GetProjectRequest{Id: proj.Id})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Id != proj.Id || got.Title != proj.Title || got.Content != proj.Content {
		t.Fatal("Get: fields inconsistent with Create response")
	}

	_, err = e.projects.Update(bg, &api.UpdateProjectRequest{
		Id:         proj.Id,
		Title:      "Alpha Updated",
		Content:    "updated body",
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"title", "content"}},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ = e.projects.Get(bg, &api.GetProjectRequest{Id: proj.Id})
	if got.Title != "Alpha Updated" || got.Content != "updated body" {
		t.Fatalf("Get after Update: title=%q content=%q", got.Title, got.Content)
	}
	// File on disk must reflect the update.
	node, _, err := store.ParseFile(filepath.Join(e.dir, proj.Id+".md"))
	if err != nil {
		t.Fatalf("ParseFile after Update: %v", err)
	}
	if node.Title != "Alpha Updated" || node.Content != "updated body" {
		t.Fatalf("file after Update: title=%q content=%q", node.Title, node.Content)
	}

	_, err = e.projects.Delete(bg, &api.DeleteProjectRequest{Id: proj.Id})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = e.projects.Get(bg, &api.GetProjectRequest{Id: proj.Id})
	if !isNotFound(err) {
		t.Fatalf("Get after Delete: expected NotFound, got %v", err)
	}
	if fileExists(e.dir, proj.Id) {
		t.Fatal("Delete: file still on disk")
	}
}

// TestProjectHierarchy_API verifies parent/child project relationships.
// Invariants: child.parentId == parent.id; filtered list returns only direct
// children; removing parent clears the relationship from both API and graph.
func TestProjectHierarchy_API(t *testing.T) {
	e := newEnv(t)

	parent, err := e.projects.Create(bg, &api.CreateProjectRequest{Title: "Parent"})
	if err != nil {
		t.Fatalf("Create parent: %v", err)
	}
	child, err := e.projects.Create(bg, &api.CreateProjectRequest{
		Title:    "Child",
		ParentId: parent.Id,
	})
	if err != nil {
		t.Fatalf("Create child: %v", err)
	}
	if child.ParentId != parent.Id {
		t.Fatalf("child.ParentId=%q, want %q", child.ParentId, parent.Id)
	}

	got, _ := e.projects.Get(bg, &api.GetProjectRequest{Id: child.Id})
	if got.ParentId != parent.Id {
		t.Fatalf("Get child: ParentId=%q after Create", got.ParentId)
	}

	childList, _ := e.projects.List(bg, &api.ListProjectsRequest{ParentId: parent.Id})
	if len(childList.Projects) != 1 || childList.Projects[0].Id != child.Id {
		t.Fatal("ListProjects by parentId: child not found")
	}

	// Remove the parent relationship.
	_, err = e.projects.Update(bg, &api.UpdateProjectRequest{
		Id:         child.Id,
		Title:      "Child",
		ParentId:   "", // explicit empty value clears the relationship
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"title", "parent_id"}},
	})
	if err != nil {
		t.Fatalf("Update to remove parent: %v", err)
	}
	got, _ = e.projects.Get(bg, &api.GetProjectRequest{Id: child.Id})
	if got.ParentId != "" {
		t.Fatalf("Get child after parent removal: ParentId=%q, want empty", got.ParentId)
	}
	childList, _ = e.projects.List(bg, &api.ListProjectsRequest{ParentId: parent.Id})
	if len(childList.Projects) != 0 {
		t.Fatal("ListProjects: child still in parent's subprojects after removal")
	}
}

// ---- File-based (watcher) invariant tests ----

// TestProjectVisible_AfterFileCreate verifies that a project written directly to
// disk becomes visible via the API once the watcher picks it up.
func TestProjectVisible_AfterFileCreate(t *testing.T) {
	e := newEnv(t)

	id := "00000000-0000-0000-0000-000000000001"
	writeFile(t, e.dir, id, projectFileContent(id, "From File", "file body", nil, nil))

	waitFor(t, func() bool {
		_, err := e.projects.Get(bg, &api.GetProjectRequest{Id: id})
		return err == nil
	})

	got, err := e.projects.Get(bg, &api.GetProjectRequest{Id: id})
	if err != nil {
		t.Fatalf("Get after file create: %v", err)
	}
	if got.Id != id || got.Title != "From File" || got.Content != "file body" {
		t.Fatalf("Get: id=%q title=%q content=%q", got.Id, got.Title, got.Content)
	}
}

// TestProject_FileModify_Watcher verifies that modifying a project file
// directly is reflected in the API after the watcher reloads it.
func TestProject_FileModify_Watcher(t *testing.T) {
	e := newEnv(t)

	proj, err := e.projects.Create(bg, &api.CreateProjectRequest{Title: "Original"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	writeFile(t, e.dir, proj.Id, projectFileContent(proj.Id, "Modified", "new body", nil, nil))

	waitFor(t, func() bool {
		got, err := e.projects.Get(bg, &api.GetProjectRequest{Id: proj.Id})
		return err == nil && got.Title == "Modified"
	})

	got, _ := e.projects.Get(bg, &api.GetProjectRequest{Id: proj.Id})
	if got.Title != "Modified" || got.Content != "new body" {
		t.Fatalf("Get after file modify: title=%q content=%q", got.Title, got.Content)
	}
}

// TestProject_FileDelete_Watcher verifies that deleting a project file directly
// makes the project inaccessible via the API.
func TestProject_FileDelete_Watcher(t *testing.T) {
	e := newEnv(t)

	id := "00000000-0000-0000-0000-000000000007"
	writeFile(t, e.dir, id, projectFileContent(id, "ToDelete", "", nil, nil))

	waitFor(t, func() bool {
		_, err := e.projects.Get(bg, &api.GetProjectRequest{Id: id})
		return err == nil
	})

	deleteFile(t, e.dir, id)

	waitFor(t, func() bool {
		_, err := e.projects.Get(bg, &api.GetProjectRequest{Id: id})
		return isNotFound(err)
	})

	_, err := e.projects.Get(bg, &api.GetProjectRequest{Id: id})
	if !isNotFound(err) {
		t.Fatalf("Get after file delete: expected NotFound, got %v", err)
	}
}

// TestProject_FileCreate_APIDelete verifies that a project created via file
// can be deleted through the API, removing both the graph node and the file.
func TestProject_FileCreate_APIDelete(t *testing.T) {
	e := newEnv(t)

	id := "00000000-0000-0000-0000-000000000004"
	writeFile(t, e.dir, id, projectFileContent(id, "FileProj", "", nil, nil))

	waitFor(t, func() bool {
		_, err := e.projects.Get(bg, &api.GetProjectRequest{Id: id})
		return err == nil
	})

	_, err := e.projects.Delete(bg, &api.DeleteProjectRequest{Id: id})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if fileExists(e.dir, id) {
		t.Fatal("file still exists after API delete")
	}
	_, err = e.projects.Get(bg, &api.GetProjectRequest{Id: id})
	if !isNotFound(err) {
		t.Fatalf("Get after API delete: expected NotFound, got %v", err)
	}
}

// TestProject_AddParent_FileModify verifies that adding a parent relationship
// via file edit is reflected in both Get and List.
// Under the parent-to-child model, the PARENT file stores the subproject edge.
func TestProject_AddParent_FileModify(t *testing.T) {
	e := newEnv(t)

	parent, _ := e.projects.Create(bg, &api.CreateProjectRequest{Title: "Parent"})
	child, err := e.projects.Create(bg, &api.CreateProjectRequest{Title: "Child"})
	if err != nil {
		t.Fatalf("Create child: %v", err)
	}

	got, _ := e.projects.Get(bg, &api.GetProjectRequest{Id: child.Id})
	if got.ParentId != "" {
		t.Fatalf("child.ParentId=%q before file edit, want empty", got.ParentId)
	}

	// In the new model, the parent file stores the subproject edge.
	writeFile(t, e.dir, parent.Id, projectFileContent(parent.Id, "Parent", "", nil, []string{child.Id}))

	waitFor(t, func() bool {
		got, err := e.projects.Get(bg, &api.GetProjectRequest{Id: child.Id})
		return err == nil && got.ParentId == parent.Id
	})

	got, _ = e.projects.Get(bg, &api.GetProjectRequest{Id: child.Id})
	if got.ParentId != parent.Id {
		t.Fatalf("child.ParentId=%q after file edit, want %q", got.ParentId, parent.Id)
	}

	list, _ := e.projects.List(bg, &api.ListProjectsRequest{ParentId: parent.Id})
	found := false
	for _, p := range list.Projects {
		if p.Id == child.Id {
			found = true
		}
	}
	if !found {
		t.Fatal("child not in parent's ListProjects after file edit")
	}
}

// ---- Status and area ----

// TestProjectStatusAndArea verifies that status and area_id are persisted through
// Create, Update, and Get.
func TestProjectStatusAndArea(t *testing.T) {
	e := newEnv(t)

	workArea, err := e.areas.Create(bg, &api.CreateAreaRequest{Title: "work"})
	if err != nil {
		t.Fatalf("Create area: %v", err)
	}
	personalArea, err := e.areas.Create(bg, &api.CreateAreaRequest{Title: "personal"})
	if err != nil {
		t.Fatalf("Create personal area: %v", err)
	}

	proj, err := e.projects.Create(bg, &api.CreateProjectRequest{
		Title:  "Status Project",
		Status: api.ProjectStatus_PROJECT_STATUS_ACTIVE,
		AreaId: workArea.Id,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if proj.Status != api.ProjectStatus_PROJECT_STATUS_ACTIVE {
		t.Fatalf("Create response: status=%v, want ACTIVE", proj.Status)
	}
	if proj.AreaId != workArea.Id {
		t.Fatalf("Create response: area_id=%q, want %q", proj.AreaId, workArea.Id)
	}

	// Must be on disk.
	node, edges, err := store.ParseFile(filepath.Join(e.dir, proj.Id+".md"))
	if err != nil {
		t.Fatalf("ParseFile after Create: %v", err)
	}
	if node.Status != "active" {
		t.Fatalf("file after Create: status=%q", node.Status)
	}
	var foundAreaEdge bool
	for _, e := range edges {
		if e.Predicate == "in_area" && e.TargetID == workArea.Id {
			foundAreaEdge = true
		}
	}
	if !foundAreaEdge {
		t.Fatalf("file after Create: missing in_area edge to %q", workArea.Id)
	}

	// Update to different values.
	_, err = e.projects.Update(bg, &api.UpdateProjectRequest{
		Id:         proj.Id,
		Title:      "Status Project",
		Status:     api.ProjectStatus_PROJECT_STATUS_COMPLETED,
		AreaId:     personalArea.Id,
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"status", "area_id"}},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := e.projects.Get(bg, &api.GetProjectRequest{Id: proj.Id})
	if got.Status != api.ProjectStatus_PROJECT_STATUS_COMPLETED {
		t.Fatalf("Get after Update: status=%v, want COMPLETED", got.Status)
	}
	if got.AreaId != personalArea.Id {
		t.Fatalf("Get after Update: area_id=%q, want %q", got.AreaId, personalArea.Id)
	}
}

// TestListProjectsFilters verifies that List filters by status and area_id correctly.
func TestListProjectsFilters(t *testing.T) {
	e := newEnv(t)

	workArea, err := e.areas.Create(bg, &api.CreateAreaRequest{Title: "work"})
	if err != nil {
		t.Fatalf("Create work area: %v", err)
	}
	personalArea, err := e.areas.Create(bg, &api.CreateAreaRequest{Title: "personal"})
	if err != nil {
		t.Fatalf("Create personal area: %v", err)
	}

	_, _ = e.projects.Create(bg, &api.CreateProjectRequest{Title: "A", Status: api.ProjectStatus_PROJECT_STATUS_ACTIVE, AreaId: workArea.Id})
	_, _ = e.projects.Create(bg, &api.CreateProjectRequest{Title: "B", Status: api.ProjectStatus_PROJECT_STATUS_BACKLOG, AreaId: workArea.Id})
	_, _ = e.projects.Create(bg, &api.CreateProjectRequest{Title: "C", Status: api.ProjectStatus_PROJECT_STATUS_ACTIVE, AreaId: personalArea.Id})

	active, _ := e.projects.List(bg, &api.ListProjectsRequest{Status: api.ProjectStatus_PROJECT_STATUS_ACTIVE})
	if len(active.Projects) != 2 {
		t.Fatalf("filter by ACTIVE: got %d, want 2", len(active.Projects))
	}

	work, _ := e.projects.List(bg, &api.ListProjectsRequest{AreaId: workArea.Id})
	if len(work.Projects) != 2 {
		t.Fatalf("filter by area_id=work: got %d, want 2", len(work.Projects))
	}

	both, _ := e.projects.List(bg, &api.ListProjectsRequest{Status: api.ProjectStatus_PROJECT_STATUS_ACTIVE, AreaId: workArea.Id})
	if len(both.Projects) != 1 || both.Projects[0].Title != "A" {
		t.Fatalf("filter by ACTIVE+work: got %d projects", len(both.Projects))
	}
}

// TestProjectEffort_API verifies that estimated_effort is persisted through
// Create, Update (set and clear), and reflected on disk.
func TestProjectEffort_API(t *testing.T) {
	e := newEnv(t)

	proj, err := e.projects.Create(bg, &api.CreateProjectRequest{
		Title: "Effort Project",
		EstimatedEffort: &api.Effort{
			Value: 2,
			Unit:  api.EffortUnit_EFFORT_UNIT_WEEKS,
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if proj.EstimatedEffort == nil {
		t.Fatal("Create response: EstimatedEffort is nil")
	}
	if proj.EstimatedEffort.Value != 2 || proj.EstimatedEffort.Unit != api.EffortUnit_EFFORT_UNIT_WEEKS {
		t.Fatalf("Create response: effort=%v/%v, want 2/WEEKS", proj.EstimatedEffort.Value, proj.EstimatedEffort.Unit)
	}

	// Verify on disk.
	node, _, err := store.ParseFile(filepath.Join(e.dir, proj.Id+".md"))
	if err != nil {
		t.Fatalf("ParseFile after Create: %v", err)
	}
	if node.EffortValue != 2 || node.EffortUnit != "weeks" {
		t.Fatalf("file after Create: effort_value=%d effort_unit=%q", node.EffortValue, node.EffortUnit)
	}

	// Update to 1 month.
	updated, err := e.projects.Update(bg, &api.UpdateProjectRequest{
		Id:    proj.Id,
		Title: proj.Title,
		EstimatedEffort: &api.Effort{
			Value: 1,
			Unit:  api.EffortUnit_EFFORT_UNIT_MONTHS,
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"estimated_effort"}},
	})
	if err != nil {
		t.Fatalf("Update to 1 month: %v", err)
	}
	if updated.EstimatedEffort == nil || updated.EstimatedEffort.Value != 1 || updated.EstimatedEffort.Unit != api.EffortUnit_EFFORT_UNIT_MONTHS {
		t.Fatalf("Update response: effort=%v", updated.EstimatedEffort)
	}
	node, _, _ = store.ParseFile(filepath.Join(e.dir, proj.Id+".md"))
	if node.EffortValue != 1 || node.EffortUnit != "months" {
		t.Fatalf("file after Update: effort_value=%d effort_unit=%q", node.EffortValue, node.EffortUnit)
	}

	// Clear effort by sending nil.
	cleared, err := e.projects.Update(bg, &api.UpdateProjectRequest{
		Id:              proj.Id,
		Title:           proj.Title,
		EstimatedEffort: nil,
		UpdateMask:      &fieldmaskpb.FieldMask{Paths: []string{"estimated_effort"}},
	})
	if err != nil {
		t.Fatalf("Update to clear: %v", err)
	}
	if cleared.EstimatedEffort != nil {
		t.Fatalf("Update clear response: EstimatedEffort=%v, want nil", cleared.EstimatedEffort)
	}
	node, _, _ = store.ParseFile(filepath.Join(e.dir, proj.Id+".md"))
	if node.EffortValue != 0 || node.EffortUnit != "" {
		t.Fatalf("file after clear: effort_value=%d effort_unit=%q", node.EffortValue, node.EffortUnit)
	}
}

// TestProjectPriority_API verifies that priority is persisted through Create,
// Update (via field_mask), and defaults to 4 when unspecified.
func TestProjectPriority_API(t *testing.T) {
	e := newEnv(t)

	// Create with explicit priority 2.
	proj, err := e.projects.Create(bg, &api.CreateProjectRequest{
		Title:    "Priority Project",
		Priority: api.Priority_PRIORITY_2,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if proj.Priority != api.Priority_PRIORITY_2 {
		t.Fatalf("Create response: priority=%v, want PROJECT_PRIORITY_2", proj.Priority)
	}

	// Verify on disk.
	node, _, err := store.ParseFile(filepath.Join(e.dir, proj.Id+".md"))
	if err != nil {
		t.Fatalf("ParseFile after Create: %v", err)
	}
	if node.Priority != 2 {
		t.Fatalf("file after Create: priority=%d, want 2", node.Priority)
	}

	// Update to priority 5.
	updated, err := e.projects.Update(bg, &api.UpdateProjectRequest{
		Id:         proj.Id,
		Title:      proj.Title,
		Priority:   api.Priority_PRIORITY_5,
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"priority"}},
	})
	if err != nil {
		t.Fatalf("Update priority: %v", err)
	}
	if updated.Priority != api.Priority_PRIORITY_5 {
		t.Fatalf("Update response: priority=%v, want PROJECT_PRIORITY_5", updated.Priority)
	}
	node, _, _ = store.ParseFile(filepath.Join(e.dir, proj.Id+".md"))
	if node.Priority != 5 {
		t.Fatalf("file after Update: priority=%d, want 5", node.Priority)
	}

	// Create with unspecified priority — should default to 4.
	defaultProj, err := e.projects.Create(bg, &api.CreateProjectRequest{
		Title: "Default Priority Project",
	})
	if err != nil {
		t.Fatalf("Create default: %v", err)
	}
	if defaultProj.Priority != api.Priority_PRIORITY_4 {
		t.Fatalf("Create default response: priority=%v, want PROJECT_PRIORITY_4", defaultProj.Priority)
	}
	node, _, _ = store.ParseFile(filepath.Join(e.dir, defaultProj.Id+".md"))
	if node.Priority != 4 {
		t.Fatalf("file after Create default: priority=%d, want 4", node.Priority)
	}
}

// TestProjectPriority_HTTP verifies that a priority-only update sent as JSON
// (mimicking the frontend keyboard shortcut) correctly persists through the
// grpc-gateway HTTP → gRPC path.
func TestProjectPriority_HTTP(t *testing.T) {
	e := newGWEnv(t)

	proj, err := e.projects.Create(bg, &api.CreateProjectRequest{Title: "HTTP Priority"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Mimic what the frontend keyboard shortcut sends.
	body := `{"priority":"PRIORITY_2","updateMask":"priority"}`
	req, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/api/v1/projects/%s", e.url, proj.Id), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status=%d, want 200", resp.StatusCode)
	}

	// Decode response and check priority.
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := result["priority"]; got != "PRIORITY_2" {
		t.Fatalf("response priority=%v, want PROJECT_PRIORITY_2", got)
	}

	// Verify persisted on disk.
	node, _, err := store.ParseFile(filepath.Join(e.dir, proj.Id+".md"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if node.Priority != 2 {
		t.Fatalf("file priority=%d, want 2", node.Priority)
	}

	// Verify via a fresh GET.
	fetched, err := e.projects.Get(bg, &api.GetProjectRequest{Id: proj.Id})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if fetched.Priority != api.Priority_PRIORITY_2 {
		t.Fatalf("Get priority=%v, want PROJECT_PRIORITY_2", fetched.Priority)
	}
}
