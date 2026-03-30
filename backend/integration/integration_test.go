package integration_test

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
	areas    *server.AreaServer
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
		areas:    server.NewAreaServer(s),
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
// taskIDs lists child tasks; subprojectIDs lists child subprojects.
func projectFileContent(id, title, content string, taskIDs, subprojectIDs []string) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "id: %s\n", id)
	sb.WriteString("type: project\n")
	fmt.Fprintf(&sb, "title: %s\n", title)
	if len(taskIDs) > 0 {
		sb.WriteString("tasks:\n")
		for _, tid := range taskIDs {
			fmt.Fprintf(&sb, "    - %s\n", tid)
		}
	}
	if len(subprojectIDs) > 0 {
		sb.WriteString("projects:\n")
		for _, sid := range subprojectIDs {
			fmt.Fprintf(&sb, "    - %s\n", sid)
		}
	}
	sb.WriteString("---\n")
	if content != "" {
		fmt.Fprintf(&sb, "\n%s", content)
	}
	return sb.String()
}

// taskFileContent builds raw .md content for a task as an external editor would.
// subtaskIDs lists child subtasks.
func taskFileContent(id, title, taskStatus, content string, subtaskIDs []string) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "id: %s\n", id)
	sb.WriteString("type: task\n")
	fmt.Fprintf(&sb, "title: %s\n", title)
	if taskStatus != "" {
		fmt.Fprintf(&sb, "status: %s\n", taskStatus)
	}
	if len(subtaskIDs) > 0 {
		sb.WriteString("subtasks:\n")
		for _, sid := range subtaskIDs {
			fmt.Fprintf(&sb, "    - %s\n", sid)
		}
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

// ---- Mixed / cross-cutting tests ----

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

// TestListRelated_MixedTypes verifies that a project with both a child project
// and a task returns both via the domain APIs, with correct entity types.
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

	subprojects, err := e.projects.List(bg, &api.ListProjectsRequest{ParentId: parent.Id})
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(subprojects.Projects) != 1 || subprojects.Projects[0].Id != child.Id {
		t.Fatalf("ListProjects(parent_id): got %d, want child %q", len(subprojects.Projects), child.Id)
	}

	tasks, err := e.tasks.List(bg, &api.ListTasksRequest{ProjectId: parent.Id})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks.Tasks) != 1 || tasks.Tasks[0].Id != task.Id {
		t.Fatalf("ListTasks(project_id): got %d, want task %q", len(tasks.Tasks), task.Id)
	}
}

// TestMalformedFile_Watcher verifies that a file with invalid frontmatter
// causes a watcher parse error but does not corrupt or remove other valid
// nodes. A sentinel file written after the bad one lets us confirm the watcher
// has processed past it (events in the same directory are sequential).
func TestMalformedFile_Watcher(t *testing.T) {
	e := newEnv(t)

	goodID := "00000000-0000-0000-0000-000000000010"
	writeFile(t, e.dir, goodID, projectFileContent(goodID, "Valid", "good content", nil, nil))
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
	writeFile(t, e.dir, sentinelID, projectFileContent(sentinelID, "Sentinel", "", nil, nil))
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

// TestLoad_FileConsistency verifies that a store loaded from pre-existing files
// provides fully consistent state: nodes are visible, relationships are indexed.
func TestLoad_FileConsistency(t *testing.T) {
	dir := t.TempDir()

	projID := "00000000-0000-0000-0000-000000000005"
	taskID := "00000000-0000-0000-0000-000000000006"

	// Project stores the task edge; task file has no outgoing relationships.
	writeFile(t, dir, projID, projectFileContent(projID, "LoadedProject", "proj content", []string{taskID}, nil))
	writeFile(t, dir, taskID, taskFileContent(taskID, "LoadedTask", "done", "task content", nil))

	g := graph.New()
	s := store.New(dir, g)
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	projects := server.NewProjectServer(s)
	tasks := server.NewTaskServer(s)

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

	// Verify consistency via ListTasks.
	list, err := tasks.List(bg, &api.ListTasksRequest{ProjectId: projID})
	if err != nil {
		t.Fatalf("ListTasks after Load: %v", err)
	}
	if len(list.Tasks) != 1 || list.Tasks[0].Id != taskID {
		t.Fatal("ListTasks after Load: task not in project")
	}
}
