package integration_test

import (
	"path/filepath"
	"testing"

	api "github.com/liamawhite/ng/api/golang"
	"github.com/liamawhite/ng/backend/pkg/store"
)

// ---- API-based invariant tests ----

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

// ---- File-based (watcher) invariant tests ----

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

	writeFile(t, e.dir, task.Id, taskFileContent(task.Id, "T", "todo", "", proj2.Id))

	waitFor(t, func() bool {
		got, err := e.tasks.Get(bg, &api.GetTaskRequest{Id: task.Id})
		return err == nil && got.ProjectId == proj2.Id
	})

	got, _ := e.tasks.Get(bg, &api.GetTaskRequest{Id: task.Id})
	if got.ProjectId != proj2.Id {
		t.Fatalf("task.ProjectId=%q after file modify, want %q", got.ProjectId, proj2.Id)
	}

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

// TestTask_FileDelete_Watcher verifies that deleting a task file removes the
// task from the project's graph edges as well as the node index.
func TestTask_FileDelete_Watcher(t *testing.T) {
	e := newEnv(t)

	projID := "00000000-0000-0000-0000-000000000008"
	taskID := "00000000-0000-0000-0000-000000000009"

	writeFile(t, e.dir, projID, projectFileContent(projID, "Proj", "", ""))
	writeFile(t, e.dir, taskID, taskFileContent(taskID, "T", "todo", "", projID))

	waitFor(t, func() bool {
		tsk, err := e.tasks.Get(bg, &api.GetTaskRequest{Id: taskID})
		return err == nil && tsk.ProjectId == projID
	})

	deleteFile(t, e.dir, taskID)

	waitFor(t, func() bool {
		_, err := e.tasks.Get(bg, &api.GetTaskRequest{Id: taskID})
		return isNotFound(err)
	})

	related, _ := e.graph.ListRelated(bg, &api.ListRelatedRequest{
		Id:        projID,
		Predicate: api.Predicate_PREDICATE_PART_OF,
		Direction: api.Direction_DIRECTION_INCOMING,
	})
	if len(related.Entities) != 0 {
		t.Fatal("task still in project's ListRelated after file delete")
	}
}

// TestTask_APICreate_FileDelete verifies that an API-created task deleted via
// file removal disappears from both the node index and the project's graph edges.
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

// ---- Edge lifecycle ----

// TestOrphanedTask_AfterProjectDelete verifies that deleting a project via the
// API cleans up the graph's edge indices so nodeToTask reflects an empty
// projectId in-memory. The task node itself must remain accessible.
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

	got, err = e.tasks.Get(bg, &api.GetTaskRequest{Id: task.Id})
	if err != nil {
		t.Fatalf("Get orphaned task: %v", err)
	}
	if got.ProjectId != "" {
		t.Fatalf("orphaned task: ProjectId=%q, want empty after project delete", got.ProjectId)
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

