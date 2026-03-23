package integration_test

import (
	"path/filepath"
	"testing"

	api "github.com/liamawhite/ng/api/golang"
	"github.com/liamawhite/ng/backend/pkg/store"
)

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
		Id:      proj.Id,
		Title:   "Alpha Updated",
		Content: "updated body",
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

	list, _ := e.projects.List(bg, &api.ListProjectsRequest{ParentId: parent.Id})
	if len(list.Projects) != 1 || list.Projects[0].Id != child.Id {
		t.Fatal("ListProjects by parentId: child not found")
	}

	related, _ := e.graph.ListRelated(bg, &api.ListRelatedRequest{
		Id:        parent.Id,
		Predicate: api.Predicate_PREDICATE_PART_OF,
		Direction: api.Direction_DIRECTION_INCOMING,
	})
	if len(related.Entities) != 1 || related.Entities[0].Entity.GetProject().GetId() != child.Id {
		t.Fatal("ListRelated: child not found as incoming for parent")
	}

	// Remove the parent relationship.
	_, err = e.projects.Update(bg, &api.UpdateProjectRequest{
		Id:    child.Id,
		Title: "Child",
		// ParentId omitted → relationship cleared
	})
	if err != nil {
		t.Fatalf("Update to remove parent: %v", err)
	}
	got, _ = e.projects.Get(bg, &api.GetProjectRequest{Id: child.Id})
	if got.ParentId != "" {
		t.Fatalf("Get child after parent removal: ParentId=%q, want empty", got.ParentId)
	}
	related, _ = e.graph.ListRelated(bg, &api.ListRelatedRequest{
		Id:        parent.Id,
		Predicate: api.Predicate_PREDICATE_PART_OF,
		Direction: api.Direction_DIRECTION_INCOMING,
	})
	if len(related.Entities) != 0 {
		t.Fatal("ListRelated: child still in parent's edges after removal")
	}
}

// ---- File-based (watcher) invariant tests ----

// TestProjectVisible_AfterFileCreate verifies that a project written directly to
// disk becomes visible via the API once the watcher picks it up.
func TestProjectVisible_AfterFileCreate(t *testing.T) {
	e := newEnv(t)

	id := "00000000-0000-0000-0000-000000000001"
	writeFile(t, e.dir, id, projectFileContent(id, "From File", "file body", ""))

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

	writeFile(t, e.dir, proj.Id, projectFileContent(proj.Id, "Modified", "new body", ""))

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
	writeFile(t, e.dir, id, projectFileContent(id, "ToDelete", "", ""))

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
	writeFile(t, e.dir, id, projectFileContent(id, "FileProj", "", ""))

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
// via file edit is reflected in both Get and the graph's incoming edges.
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

	writeFile(t, e.dir, child.Id, projectFileContent(child.Id, "Child", "", parent.Id))

	waitFor(t, func() bool {
		got, err := e.projects.Get(bg, &api.GetProjectRequest{Id: child.Id})
		return err == nil && got.ParentId == parent.Id
	})

	got, _ = e.projects.Get(bg, &api.GetProjectRequest{Id: child.Id})
	if got.ParentId != parent.Id {
		t.Fatalf("child.ParentId=%q after file edit, want %q", got.ParentId, parent.Id)
	}

	related, _ := e.graph.ListRelated(bg, &api.ListRelatedRequest{
		Id:        parent.Id,
		Predicate: api.Predicate_PREDICATE_PART_OF,
		Direction: api.Direction_DIRECTION_INCOMING,
	})
	found := false
	for _, r := range related.Entities {
		if r.Entity.GetProject().GetId() == child.Id {
			found = true
		}
	}
	if !found {
		t.Fatal("child not in parent's ListRelated after file edit")
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
		Id:     proj.Id,
		Title:  "Status Project",
		Status: api.ProjectStatus_PROJECT_STATUS_COMPLETED,
		AreaId: personalArea.Id,
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
