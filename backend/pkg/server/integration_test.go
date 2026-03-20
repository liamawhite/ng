package server_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	api "github.com/liamawhite/ng/api/golang"
	"github.com/liamawhite/ng/backend/pkg/graph"
	"github.com/liamawhite/ng/backend/pkg/server"
	"github.com/liamawhite/ng/backend/pkg/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ---- Test infrastructure ----

// testEnv wires together a full server stack backed by a temporary directory.
type testEnv struct {
	dir      string
	projects *server.ProjectServer
	tasks    *server.TaskServer
	graph    *server.GraphServer
}

func newEnv(t *testing.T) *testEnv {
	t.Helper()
	dir := t.TempDir()
	g := graph.New()
	s := store.New(dir, g)
	w, err := store.NewWatcher(s)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	if err := w.Start(); err != nil {
		t.Fatalf("watcher.Start: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return &testEnv{
		dir:      dir,
		projects: server.NewProjectServer(s),
		tasks:    server.NewTaskServer(s),
		graph:    server.NewGraphServer(s),
	}
}

var bg = context.Background()

// waitFor polls cond every 20ms, failing the test if it is not satisfied within
// 1 second. This enforces the invariant that the system reaches consistency at
// most 1 second after any change (API call or direct file edit).
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("system not consistent within 1s of change")
}

func isNotFound(err error) bool {
	s, ok := status.FromError(err)
	return ok && s.Code() == codes.NotFound
}

// projectFileContent builds raw .md content for a project as an external editor would.
func projectFileContent(id, title, content, parentID string) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "id: %s\n", id)
	sb.WriteString("type: project\n")
	fmt.Fprintf(&sb, "title: %s\n", title)
	if parentID != "" {
		sb.WriteString("relationships:\n")
		fmt.Fprintf(&sb, "    - predicate: part_of\n      target: %s\n", parentID)
	}
	sb.WriteString("---\n")
	if content != "" {
		fmt.Fprintf(&sb, "\n%s", content)
	}
	return sb.String()
}

// taskFileContent builds raw .md content for a task as an external editor would.
func taskFileContent(id, title, taskStatus, content, projectID string) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "id: %s\n", id)
	sb.WriteString("type: task\n")
	fmt.Fprintf(&sb, "title: %s\n", title)
	if taskStatus != "" {
		fmt.Fprintf(&sb, "status: %s\n", taskStatus)
	}
	if projectID != "" {
		sb.WriteString("relationships:\n")
		fmt.Fprintf(&sb, "    - predicate: part_of\n      target: %s\n", projectID)
	}
	sb.WriteString("---\n")
	if content != "" {
		fmt.Fprintf(&sb, "\n%s", content)
	}
	return sb.String()
}

// writeFile writes content directly to dir/<id>.md, simulating an external editor.
func writeFile(t *testing.T, dir, id, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, id+".md"), []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile %s: %v", id, err)
	}
}

// deleteFile removes dir/<id>.md directly, simulating external deletion.
func deleteFile(t *testing.T, dir, id string) {
	t.Helper()
	if err := os.Remove(filepath.Join(dir, id+".md")); err != nil {
		t.Fatalf("deleteFile %s: %v", id, err)
	}
}

func fileExists(dir, id string) bool {
	_, err := os.Stat(filepath.Join(dir, id+".md"))
	return err == nil
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

// TestTaskCRUD_API verifies the task lifecycle including project assignment and
// graph relationship consistency.
// Invariants: task.projectId matches on Create/Get/List; task appears in
// project's incoming edges; delete removes the task from the graph.
func TestTaskCRUD_API(t *testing.T) {
	e := newEnv(t)

	proj, err := e.projects.Create(bg, &api.CreateProjectRequest{Title: "Proj"})
	if err != nil {
		t.Fatalf("Create project: %v", err)
	}

	task, err := e.tasks.Create(bg, &api.CreateTaskRequest{
		Title:     "T1",
		Content:   "body",
		ProjectId: proj.Id,
		Status:    api.TaskStatus_TASK_STATUS_TODO,
	})
	if err != nil {
		t.Fatalf("Create task: %v", err)
	}
	if task.ProjectId != proj.Id {
		t.Fatalf("Create task: projectId=%q, want %q", task.ProjectId, proj.Id)
	}
	if task.Status != api.TaskStatus_TASK_STATUS_TODO {
		t.Fatalf("Create task: status=%v, want TODO", task.Status)
	}

	got, err := e.tasks.Get(bg, &api.GetTaskRequest{Id: task.Id})
	if err != nil {
		t.Fatalf("Get task: %v", err)
	}
	if got.ProjectId != proj.Id {
		t.Fatalf("Get task: projectId=%q", got.ProjectId)
	}

	list, err := e.tasks.List(bg, &api.ListTasksRequest{ProjectId: proj.Id})
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	if len(list.Tasks) != 1 || list.Tasks[0].Id != task.Id {
		t.Fatal("ListTasks by projectId: task not found")
	}

	// Project's incoming part_of edges include the task.
	related, err := e.graph.ListRelated(bg, &api.ListRelatedRequest{
		Id:        proj.Id,
		Predicate: api.Predicate_PREDICATE_PART_OF,
		Direction: api.Direction_DIRECTION_INCOMING,
	})
	if err != nil {
		t.Fatalf("ListRelated: %v", err)
	}
	if len(related.Entities) != 1 || related.Entities[0].Entity.GetTask().GetId() != task.Id {
		t.Fatal("ListRelated: task not in project's incoming edges")
	}

	_, err = e.tasks.Update(bg, &api.UpdateTaskRequest{
		Id:        task.Id,
		Title:     "T1",
		ProjectId: proj.Id,
		Status:    api.TaskStatus_TASK_STATUS_DONE,
	})
	if err != nil {
		t.Fatalf("Update task: %v", err)
	}
	got, _ = e.tasks.Get(bg, &api.GetTaskRequest{Id: task.Id})
	if got.Status != api.TaskStatus_TASK_STATUS_DONE {
		t.Fatalf("Get task after Update: status=%v, want DONE", got.Status)
	}
	node, _, _ := store.ParseFile(filepath.Join(e.dir, task.Id+".md"))
	if node.Status != "done" {
		t.Fatalf("file after Update: status=%q, want done", node.Status)
	}

	_, _ = e.tasks.Delete(bg, &api.DeleteTaskRequest{Id: task.Id})
	related, _ = e.graph.ListRelated(bg, &api.ListRelatedRequest{
		Id:        proj.Id,
		Predicate: api.Predicate_PREDICATE_PART_OF,
		Direction: api.Direction_DIRECTION_INCOMING,
	})
	if len(related.Entities) != 0 {
		t.Fatal("ListRelated: task still in project after delete")
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

// TestTaskVisible_AfterFileCreate verifies that a task file with a project
// relationship is correctly indexed in the graph once the watcher reloads it.
func TestTaskVisible_AfterFileCreate(t *testing.T) {
	e := newEnv(t)

	projID := "00000000-0000-0000-0000-000000000002"
	taskID := "00000000-0000-0000-0000-000000000003"

	writeFile(t, e.dir, projID, projectFileContent(projID, "Proj", "", ""))
	writeFile(t, e.dir, taskID, taskFileContent(taskID, "Task", "todo", "", projID))

	waitFor(t, func() bool {
		tsk, err := e.tasks.Get(bg, &api.GetTaskRequest{Id: taskID})
		return err == nil && tsk.ProjectId == projID
	})

	task, _ := e.tasks.Get(bg, &api.GetTaskRequest{Id: taskID})
	if task.ProjectId != projID {
		t.Fatalf("task.ProjectId=%q, want %q", task.ProjectId, projID)
	}

	related, _ := e.graph.ListRelated(bg, &api.ListRelatedRequest{
		Id:        projID,
		Predicate: api.Predicate_PREDICATE_PART_OF,
		Direction: api.Direction_DIRECTION_INCOMING,
	})
	if len(related.Entities) != 1 || related.Entities[0].Entity.GetTask().GetId() != taskID {
		t.Fatal("ListRelated: task not in project's incoming edges after file create")
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

// TestTask_FileModify_ChangeProject_Watcher verifies that rewriting a task file
// to point to a different project is reflected in both Get and the graph's edge
// indices: the task leaves the old project's incoming edges and joins the new one.
func TestTask_FileModify_ChangeProject_Watcher(t *testing.T) {
	e := newEnv(t)

	proj1, _ := e.projects.Create(bg, &api.CreateProjectRequest{Title: "P1"})
	proj2, _ := e.projects.Create(bg, &api.CreateProjectRequest{Title: "P2"})
	task, err := e.tasks.Create(bg, &api.CreateTaskRequest{
		Title:     "T",
		ProjectId: proj1.Id,
		Status:    api.TaskStatus_TASK_STATUS_TODO,
	})
	if err != nil {
		t.Fatalf("Create task: %v", err)
	}

	// Rewrite task file pointing to proj2.
	writeFile(t, e.dir, task.Id, taskFileContent(task.Id, "T", "todo", "", proj2.Id))

	waitFor(t, func() bool {
		got, err := e.tasks.Get(bg, &api.GetTaskRequest{Id: task.Id})
		return err == nil && got.ProjectId == proj2.Id
	})

	got, _ := e.tasks.Get(bg, &api.GetTaskRequest{Id: task.Id})
	if got.ProjectId != proj2.Id {
		t.Fatalf("task.ProjectId=%q after file modify, want %q", got.ProjectId, proj2.Id)
	}

	// proj1 must no longer list the task.
	rel1, _ := e.graph.ListRelated(bg, &api.ListRelatedRequest{
		Id:        proj1.Id,
		Predicate: api.Predicate_PREDICATE_PART_OF,
		Direction: api.Direction_DIRECTION_INCOMING,
	})
	for _, r := range rel1.Entities {
		if r.Entity.GetTask().GetId() == task.Id {
			t.Fatal("task still in proj1's ListRelated after reassignment")
		}
	}

	// proj2 must now list the task.
	rel2, _ := e.graph.ListRelated(bg, &api.ListRelatedRequest{
		Id:        proj2.Id,
		Predicate: api.Predicate_PREDICATE_PART_OF,
		Direction: api.Direction_DIRECTION_INCOMING,
	})
	found := false
	for _, r := range rel2.Entities {
		if r.Entity.GetTask().GetId() == task.Id {
			found = true
		}
	}
	if !found {
		t.Fatal("task not in proj2's ListRelated after reassignment")
	}
}

// TestProject_FileDelete_Watcher verifies that deleting a project file directly
// makes the project inaccessible via the API.
// The file is created via writeFile so the watcher processes the Create event
// (establishing a per-file kqueue watch) before we delete it.
func TestProject_FileDelete_Watcher(t *testing.T) {
	e := newEnv(t)

	id := "00000000-0000-0000-0000-000000000007"
	writeFile(t, e.dir, id, projectFileContent(id, "ToDelete", "", ""))

	// Wait for the watcher to pick up the Create — this confirms the per-file
	// kqueue watch is established, so the subsequent deletion will be detected.
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

// TestTask_FileDelete_Watcher verifies that deleting a task file removes the
// task from the project's graph edges as well as the node index.
// Both files are created via writeFile so the watcher processes each Create
// (establishing per-file kqueue watches) before we delete the task.
func TestTask_FileDelete_Watcher(t *testing.T) {
	e := newEnv(t)

	projID := "00000000-0000-0000-0000-000000000008"
	taskID := "00000000-0000-0000-0000-000000000009"

	writeFile(t, e.dir, projID, projectFileContent(projID, "Proj", "", ""))
	writeFile(t, e.dir, taskID, taskFileContent(taskID, "T", "todo", "", projID))

	// Wait for the watcher to index both files and confirm the relationship.
	waitFor(t, func() bool {
		tsk, err := e.tasks.Get(bg, &api.GetTaskRequest{Id: taskID})
		return err == nil && tsk.ProjectId == projID
	})

	deleteFile(t, e.dir, taskID)

	waitFor(t, func() bool {
		_, err := e.tasks.Get(bg, &api.GetTaskRequest{Id: taskID})
		return isNotFound(err)
	})

	// Graph edges are cleaned up too.
	related, _ := e.graph.ListRelated(bg, &api.ListRelatedRequest{
		Id:        projID,
		Predicate: api.Predicate_PREDICATE_PART_OF,
		Direction: api.Direction_DIRECTION_INCOMING,
	})
	if len(related.Entities) != 0 {
		t.Fatal("task still in project's ListRelated after file delete")
	}
}

// ---- Cross-mode invariant tests ----

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

// TestTask_APICreate_FileDelete verifies that an API-created task deleted via
// file removal disappears from both the node index and the project's graph edges.
// After the API create we re-write the file via writeFile and wait for the watcher
// to confirm the write — this ensures a per-file kqueue watch is in place before
// the deletion, which is required for the Remove event to fire on macOS.
func TestTask_APICreate_FileDelete(t *testing.T) {
	e := newEnv(t)

	proj, _ := e.projects.Create(bg, &api.CreateProjectRequest{Title: "P"})
	task, err := e.tasks.Create(bg, &api.CreateTaskRequest{
		Title:     "T",
		ProjectId: proj.Id,
		Status:    api.TaskStatus_TASK_STATUS_IN_PROGRESS,
	})
	if err != nil {
		t.Fatalf("Create task: %v", err)
	}

	// Re-write with a distinguishable field value so we can tell when the watcher
	// has processed this specific write (confirming the per-file kqueue watch).
	writeFile(t, e.dir, task.Id, taskFileContent(task.Id, "T-watched", "done", "", proj.Id))
	waitFor(t, func() bool {
		got, err := e.tasks.Get(bg, &api.GetTaskRequest{Id: task.Id})
		return err == nil && got.Title == "T-watched"
	})

	deleteFile(t, e.dir, task.Id)

	waitFor(t, func() bool {
		_, err := e.tasks.Get(bg, &api.GetTaskRequest{Id: task.Id})
		return isNotFound(err)
	})

	related, _ := e.graph.ListRelated(bg, &api.ListRelatedRequest{
		Id:        proj.Id,
		Predicate: api.Predicate_PREDICATE_PART_OF,
		Direction: api.Direction_DIRECTION_INCOMING,
	})
	if len(related.Entities) != 0 {
		t.Fatal("task still in project graph after external file delete")
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

	// Rewrite child file to add parent relationship.
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

// ---- Type safety ----

// TestTypeIsolation verifies that the entity type is enforced on all Get and
// List operations. A project ID used in GetTask must return NotFound and vice
// versa; ListProjects must never contain a task and ListTasks must never
// contain a project.
func TestTypeIsolation(t *testing.T) {
	e := newEnv(t)

	proj, err := e.projects.Create(bg, &api.CreateProjectRequest{Title: "P"})
	if err != nil {
		t.Fatalf("Create project: %v", err)
	}
	task, err := e.tasks.Create(bg, &api.CreateTaskRequest{
		Title:  "T",
		Status: api.TaskStatus_TASK_STATUS_TODO,
	})
	if err != nil {
		t.Fatalf("Create task: %v", err)
	}

	_, err = e.tasks.Get(bg, &api.GetTaskRequest{Id: proj.Id})
	if !isNotFound(err) {
		t.Fatalf("GetTask(project ID): expected NotFound, got %v", err)
	}
	_, err = e.projects.Get(bg, &api.GetProjectRequest{Id: task.Id})
	if !isNotFound(err) {
		t.Fatalf("GetProject(task ID): expected NotFound, got %v", err)
	}

	projs, _ := e.projects.List(bg, &api.ListProjectsRequest{})
	for _, p := range projs.Projects {
		if p.Id == task.Id {
			t.Fatal("ListProjects returned a task")
		}
	}
	tasks, _ := e.tasks.List(bg, &api.ListTasksRequest{})
	for _, tk := range tasks.Tasks {
		if tk.Id == proj.Id {
			t.Fatal("ListTasks returned a project")
		}
	}
}

// ---- Multiple entities ----

// TestTask_NoProject verifies that a task without a project relationship is
// valid. Invariants: projectId is empty on Create/Get; the task appears in an
// unfiltered ListTasks but not in any project-filtered list.
func TestTask_NoProject(t *testing.T) {
	e := newEnv(t)

	proj, _ := e.projects.Create(bg, &api.CreateProjectRequest{Title: "P"})
	standalone, err := e.tasks.Create(bg, &api.CreateTaskRequest{
		Title:  "Standalone",
		Status: api.TaskStatus_TASK_STATUS_TODO,
		// No ProjectId.
	})
	if err != nil {
		t.Fatalf("Create standalone task: %v", err)
	}
	if standalone.ProjectId != "" {
		t.Fatalf("Create: projectId=%q, want empty", standalone.ProjectId)
	}

	got, err := e.tasks.Get(bg, &api.GetTaskRequest{Id: standalone.Id})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ProjectId != "" {
		t.Fatalf("Get: projectId=%q, want empty", got.ProjectId)
	}

	all, _ := e.tasks.List(bg, &api.ListTasksRequest{})
	found := false
	for _, tk := range all.Tasks {
		if tk.Id == standalone.Id {
			found = true
		}
	}
	if !found {
		t.Fatal("standalone task not in unfiltered ListTasks")
	}

	filtered, _ := e.tasks.List(bg, &api.ListTasksRequest{ProjectId: proj.Id})
	for _, tk := range filtered.Tasks {
		if tk.Id == standalone.Id {
			t.Fatal("standalone task appeared in project-filtered ListTasks")
		}
	}
}

// TestMultipleTasks_ListConsistency verifies that all tasks in a project are
// returned by both ListTasks and ListRelated, not just the first one.
func TestMultipleTasks_ListConsistency(t *testing.T) {
	e := newEnv(t)

	proj, _ := e.projects.Create(bg, &api.CreateProjectRequest{Title: "P"})

	var wantIDs []string
	for _, title := range []string{"T1", "T2", "T3"} {
		tk, err := e.tasks.Create(bg, &api.CreateTaskRequest{
			Title:     title,
			ProjectId: proj.Id,
			Status:    api.TaskStatus_TASK_STATUS_TODO,
		})
		if err != nil {
			t.Fatalf("Create %s: %v", title, err)
		}
		wantIDs = append(wantIDs, tk.Id)
	}

	list, err := e.tasks.List(bg, &api.ListTasksRequest{ProjectId: proj.Id})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(list.Tasks) != 3 {
		t.Fatalf("ListTasks: got %d, want 3", len(list.Tasks))
	}
	for _, id := range wantIDs {
		found := false
		for _, tk := range list.Tasks {
			if tk.Id == id {
				found = true
			}
		}
		if !found {
			t.Fatalf("task %s missing from ListTasks", id)
		}
	}

	related, err := e.graph.ListRelated(bg, &api.ListRelatedRequest{
		Id:        proj.Id,
		Predicate: api.Predicate_PREDICATE_PART_OF,
		Direction: api.Direction_DIRECTION_INCOMING,
	})
	if err != nil {
		t.Fatalf("ListRelated: %v", err)
	}
	if len(related.Entities) != 3 {
		t.Fatalf("ListRelated: got %d entities, want 3", len(related.Entities))
	}
}

// TestListRelated_MixedTypes verifies that a project with both a child project
// and a task as part_of returns both in ListRelated (incoming edges), and that
// the entity types are correctly distinguished.
func TestListRelated_MixedTypes(t *testing.T) {
	e := newEnv(t)

	parent, _ := e.projects.Create(bg, &api.CreateProjectRequest{Title: "Parent"})
	child, err := e.projects.Create(bg, &api.CreateProjectRequest{
		Title:    "Child",
		ParentId: parent.Id,
	})
	if err != nil {
		t.Fatalf("Create child project: %v", err)
	}
	task, err := e.tasks.Create(bg, &api.CreateTaskRequest{
		Title:     "T",
		ProjectId: parent.Id,
		Status:    api.TaskStatus_TASK_STATUS_TODO,
	})
	if err != nil {
		t.Fatalf("Create task: %v", err)
	}

	related, err := e.graph.ListRelated(bg, &api.ListRelatedRequest{
		Id:        parent.Id,
		Predicate: api.Predicate_PREDICATE_PART_OF,
		Direction: api.Direction_DIRECTION_INCOMING,
	})
	if err != nil {
		t.Fatalf("ListRelated: %v", err)
	}
	if len(related.Entities) != 2 {
		t.Fatalf("ListRelated: got %d entities, want 2", len(related.Entities))
	}

	var childFound, taskFound bool
	for _, r := range related.Entities {
		switch {
		case r.Entity.GetProject().GetId() == child.Id:
			childFound = true
		case r.Entity.GetTask().GetId() == task.Id:
			taskFound = true
		}
	}
	if !childFound {
		t.Fatal("child project not in parent's ListRelated")
	}
	if !taskFound {
		t.Fatal("task not in parent's ListRelated")
	}
}

// ---- Edge lifecycle ----

// TestOrphanedTask_AfterProjectDelete verifies that deleting a project via the
// API cleans up the graph's edge indices: the deleted project's incoming edges
// are removed from connected tasks' outgoing sets, so nodeToTask reflects an
// empty projectId in-memory. The task node itself must remain accessible.
func TestOrphanedTask_AfterProjectDelete(t *testing.T) {
	e := newEnv(t)

	proj, _ := e.projects.Create(bg, &api.CreateProjectRequest{Title: "P"})
	task, err := e.tasks.Create(bg, &api.CreateTaskRequest{
		Title:     "T",
		ProjectId: proj.Id,
		Status:    api.TaskStatus_TASK_STATUS_TODO,
	})
	if err != nil {
		t.Fatalf("Create task: %v", err)
	}

	got, _ := e.tasks.Get(bg, &api.GetTaskRequest{Id: task.Id})
	if got.ProjectId != proj.Id {
		t.Fatalf("pre-delete: task.ProjectId=%q, want %q", got.ProjectId, proj.Id)
	}

	_, err = e.projects.Delete(bg, &api.DeleteProjectRequest{Id: proj.Id})
	if err != nil {
		t.Fatalf("Delete project: %v", err)
	}

	_, err = e.projects.Get(bg, &api.GetProjectRequest{Id: proj.Id})
	if !isNotFound(err) {
		t.Fatalf("Get deleted project: expected NotFound, got %v", err)
	}

	// Task node still exists.
	got, err = e.tasks.Get(bg, &api.GetTaskRequest{Id: task.Id})
	if err != nil {
		t.Fatalf("Get orphaned task: %v", err)
	}
	// graph.DeleteNode clears the task's outgoing "part_of" edge, so the
	// in-memory projectId must be empty.
	if got.ProjectId != "" {
		t.Fatalf("orphaned task: ProjectId=%q, want empty after project delete", got.ProjectId)
	}
}

// ---- Error resilience ----

// TestMalformedFile_Watcher verifies that a file with invalid frontmatter
// causes a watcher parse error but does not corrupt or remove other valid
// nodes. A sentinel file written after the bad one lets us confirm the watcher
// has processed past it (events in the same directory are sequential).
func TestMalformedFile_Watcher(t *testing.T) {
	e := newEnv(t)

	goodID := "00000000-0000-0000-0000-000000000010"
	writeFile(t, e.dir, goodID, projectFileContent(goodID, "Valid", "good content", ""))
	waitFor(t, func() bool {
		_, err := e.projects.Get(bg, &api.GetProjectRequest{Id: goodID})
		return err == nil
	})

	// Write a malformed file (missing frontmatter delimiters).
	badID := "00000000-0000-0000-0000-000000000011"
	writeFile(t, e.dir, badID, "no frontmatter here\n")

	// Write a sentinel after the bad file; processing the sentinel confirms
	// the watcher has already attempted (and failed) the bad file.
	sentinelID := "00000000-0000-0000-0000-000000000012"
	writeFile(t, e.dir, sentinelID, projectFileContent(sentinelID, "Sentinel", "", ""))
	waitFor(t, func() bool {
		_, err := e.projects.Get(bg, &api.GetProjectRequest{Id: sentinelID})
		return err == nil
	})

	// Valid node is untouched.
	got, err := e.projects.Get(bg, &api.GetProjectRequest{Id: goodID})
	if err != nil {
		t.Fatalf("Get valid project after bad file: %v", err)
	}
	if got.Title != "Valid" || got.Content != "good content" {
		t.Fatalf("valid project corrupted: title=%q content=%q", got.Title, got.Content)
	}

	// Malformed file must not have produced a node.
	list, _ := e.projects.List(bg, &api.ListProjectsRequest{})
	for _, p := range list.Projects {
		if p.Id == badID {
			t.Fatal("malformed file produced a project node")
		}
	}
}

// ---- Load-time consistency test ----

// TestLoad_FileConsistency verifies that a store loaded from pre-existing files
// provides fully consistent state: nodes are visible, relationships are indexed.
func TestLoad_FileConsistency(t *testing.T) {
	dir := t.TempDir()

	projID := "00000000-0000-0000-0000-000000000005"
	taskID := "00000000-0000-0000-0000-000000000006"

	writeFile(t, dir, projID, projectFileContent(projID, "LoadedProject", "proj content", ""))
	writeFile(t, dir, taskID, taskFileContent(taskID, "LoadedTask", "done", "task content", projID))

	g := graph.New()
	s := store.New(dir, g)
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	projects := server.NewProjectServer(s)
	tasks := server.NewTaskServer(s)
	graphSrv := server.NewGraphServer(s)

	proj, err := projects.Get(bg, &api.GetProjectRequest{Id: projID})
	if err != nil {
		t.Fatalf("Get project after Load: %v", err)
	}
	if proj.Title != "LoadedProject" || proj.Content != "proj content" {
		t.Fatalf("project: title=%q content=%q", proj.Title, proj.Content)
	}

	task, err := tasks.Get(bg, &api.GetTaskRequest{Id: taskID})
	if err != nil {
		t.Fatalf("Get task after Load: %v", err)
	}
	if task.ProjectId != projID {
		t.Fatalf("task.ProjectId=%q, want %q", task.ProjectId, projID)
	}
	if task.Status != api.TaskStatus_TASK_STATUS_DONE {
		t.Fatalf("task.Status=%v, want DONE", task.Status)
	}

	// Graph edges are consistent.
	related, err := graphSrv.ListRelated(bg, &api.ListRelatedRequest{
		Id:        projID,
		Predicate: api.Predicate_PREDICATE_PART_OF,
		Direction: api.Direction_DIRECTION_INCOMING,
	})
	if err != nil {
		t.Fatalf("ListRelated: %v", err)
	}
	if len(related.Entities) != 1 || related.Entities[0].Entity.GetTask().GetId() != taskID {
		t.Fatal("ListRelated after Load: task not in project's incoming edges")
	}
}
