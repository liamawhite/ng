package integration_test

import (
	"path/filepath"
	"testing"

	api "github.com/liamawhite/ng/api/golang"
	"github.com/liamawhite/ng/backend/pkg/store"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
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

	// Project's task list includes the task.
	listed, err := e.tasks.List(bg, &api.ListTasksRequest{ProjectId: proj.Id})
	if err != nil {
		t.Fatalf("ListTasks by projectId: %v", err)
	}
	if len(listed.Tasks) != 1 || listed.Tasks[0].Id != task.Id {
		t.Fatal("ListTasks: task not in project")
	}

	_, err = e.tasks.Update(bg, &api.UpdateTaskRequest{
		Id:         task.Id,
		Status:     api.TaskStatus_TASK_STATUS_DONE,
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"status"}},
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
	listed, _ = e.tasks.List(bg, &api.ListTasksRequest{ProjectId: proj.Id})
	if len(listed.Tasks) != 0 {
		t.Fatal("ListTasks: task still in project after delete")
	}
}

// ---- File-based (watcher) invariant tests ----

// TestTaskVisible_AfterFileCreate verifies that a task file with a project
// relationship is correctly indexed in the graph once the watcher reloads it.
func TestTaskVisible_AfterFileCreate(t *testing.T) {
	e := newEnv(t)

	projID := "00000000-0000-0000-0000-000000000002"
	taskID := "00000000-0000-0000-0000-000000000003"

	// In the new model, the project file stores the task edge.
	writeFile(t, e.dir, projID, projectFileContent(projID, "Proj", "", []string{taskID}, nil))
	writeFile(t, e.dir, taskID, taskFileContent(taskID, "Task", "todo", "", nil))

	waitFor(t, func() bool {
		tsk, err := e.tasks.Get(bg, &api.GetTaskRequest{Id: taskID})
		return err == nil && tsk.ProjectId == projID
	})

	task, _ := e.tasks.Get(bg, &api.GetTaskRequest{Id: taskID})
	if task.ProjectId != projID {
		t.Fatalf("task.ProjectId=%q, want %q", task.ProjectId, projID)
	}

	listed, _ := e.tasks.List(bg, &api.ListTasksRequest{ProjectId: projID})
	if len(listed.Tasks) != 1 || listed.Tasks[0].Id != taskID {
		t.Fatal("ListTasks: task not in project after file create")
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

	// In the new model, reassigning a task means updating the project files.
	writeFile(t, e.dir, proj1.Id, projectFileContent(proj1.Id, "P1", "", nil, nil))
	writeFile(t, e.dir, proj2.Id, projectFileContent(proj2.Id, "P2", "", []string{task.Id}, nil))

	waitFor(t, func() bool {
		got, err := e.tasks.Get(bg, &api.GetTaskRequest{Id: task.Id})
		return err == nil && got.ProjectId == proj2.Id
	})

	got, _ := e.tasks.Get(bg, &api.GetTaskRequest{Id: task.Id})
	if got.ProjectId != proj2.Id {
		t.Fatalf("task.ProjectId=%q after file modify, want %q", got.ProjectId, proj2.Id)
	}

	tasks1, _ := e.tasks.List(bg, &api.ListTasksRequest{ProjectId: proj1.Id})
	for _, tk := range tasks1.Tasks {
		if tk.Id == task.Id {
			t.Fatal("task still in proj1 after reassignment")
		}
	}

	tasks2, _ := e.tasks.List(bg, &api.ListTasksRequest{ProjectId: proj2.Id})
	found := false
	for _, tk := range tasks2.Tasks {
		if tk.Id == task.Id {
			found = true
		}
	}
	if !found {
		t.Fatal("task not in proj2 after reassignment")
	}
}

// TestTask_FileDelete_Watcher verifies that deleting a task file removes the
// task from the project's graph edges as well as the node index.
func TestTask_FileDelete_Watcher(t *testing.T) {
	e := newEnv(t)

	projID := "00000000-0000-0000-0000-000000000008"
	taskID := "00000000-0000-0000-0000-000000000009"

	writeFile(t, e.dir, projID, projectFileContent(projID, "Proj", "", []string{taskID}, nil))
	writeFile(t, e.dir, taskID, taskFileContent(taskID, "T", "todo", "", nil))

	waitFor(t, func() bool {
		tsk, err := e.tasks.Get(bg, &api.GetTaskRequest{Id: taskID})
		return err == nil && tsk.ProjectId == projID
	})

	deleteFile(t, e.dir, taskID)

	waitFor(t, func() bool {
		_, err := e.tasks.Get(bg, &api.GetTaskRequest{Id: taskID})
		return isNotFound(err)
	})

	listed, _ := e.tasks.List(bg, &api.ListTasksRequest{ProjectId: projID})
	if len(listed.Tasks) != 0 {
		t.Fatal("task still in project's task list after file delete")
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
	// Task file has no relationship data in the new model; title change is sufficient.
	writeFile(t, e.dir, task.Id, taskFileContent(task.Id, "T-watched", "done", "", nil))
	waitFor(t, func() bool {
		got, err := e.tasks.Get(bg, &api.GetTaskRequest{Id: task.Id})
		return err == nil && got.Title == "T-watched"
	})

	deleteFile(t, e.dir, task.Id)

	waitFor(t, func() bool {
		_, err := e.tasks.Get(bg, &api.GetTaskRequest{Id: task.Id})
		return isNotFound(err)
	})

	listed, _ := e.tasks.List(bg, &api.ListTasksRequest{ProjectId: proj.Id})
	if len(listed.Tasks) != 0 {
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

	listedAll, err := e.tasks.List(bg, &api.ListTasksRequest{ProjectId: proj.Id})
	if err != nil {
		t.Fatalf("ListTasks(projectId): %v", err)
	}
	if len(listedAll.Tasks) != 3 {
		t.Fatalf("ListTasks: got %d tasks, want 3", len(listedAll.Tasks))
	}
}

// ---- Subtask tests ----

// TestTaskSubtask_CreateAndGet verifies that a task created with a ParentTaskId
// has the correct ParentTaskId and ProjectId fields on Create and Get.
func TestTaskSubtask_CreateAndGet(t *testing.T) {
	e := newEnv(t)

	proj, _ := e.projects.Create(bg, &api.CreateProjectRequest{Title: "Proj"})
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
		t.Fatalf("Create subtask: %v", err)
	}
	if sub.ParentTaskId != root.Id {
		t.Fatalf("sub.ParentTaskId=%q, want %q", sub.ParentTaskId, root.Id)
	}
	if sub.ProjectId != proj.Id {
		t.Fatalf("sub.ProjectId=%q, want %q", sub.ProjectId, proj.Id)
	}

	got, err := e.tasks.Get(bg, &api.GetTaskRequest{Id: sub.Id})
	if err != nil {
		t.Fatalf("Get subtask: %v", err)
	}
	if got.ParentTaskId != root.Id {
		t.Fatalf("Get sub.ParentTaskId=%q, want %q", got.ParentTaskId, root.Id)
	}
	if got.ProjectId != proj.Id {
		t.Fatalf("Get sub.ProjectId=%q, want %q", got.ProjectId, proj.Id)
	}
}

// TestTaskSubtask_ListByProject verifies that ListTasks(project_id) returns
// both root tasks and subtasks (all tasks share the same project_id).
func TestTaskSubtask_ListByProject(t *testing.T) {
	e := newEnv(t)

	proj, _ := e.projects.Create(bg, &api.CreateProjectRequest{Title: "Proj"})
	root, _ := e.tasks.Create(bg, &api.CreateTaskRequest{
		Title:     "Root",
		ProjectId: proj.Id,
		Status:    api.TaskStatus_TASK_STATUS_TODO,
	})
	sub, _ := e.tasks.Create(bg, &api.CreateTaskRequest{
		Title:        "Sub",
		ProjectId:    proj.Id,
		ParentTaskId: root.Id,
		Status:       api.TaskStatus_TASK_STATUS_TODO,
	})

	list, err := e.tasks.List(bg, &api.ListTasksRequest{ProjectId: proj.Id})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(list.Tasks) != 2 {
		t.Fatalf("ListTasks(project_id): got %d, want 2", len(list.Tasks))
	}
	ids := map[string]bool{}
	for _, tk := range list.Tasks {
		ids[tk.Id] = true
	}
	if !ids[root.Id] || !ids[sub.Id] {
		t.Errorf("ListTasks(project_id): missing root or sub, got %v", ids)
	}
}

// TestTaskPinned_CreateAndUpdate verifies that the pinned field is persisted
// on create, survives a Get, and can be toggled via Update with update_mask.
func TestTaskPinned_CreateAndUpdate(t *testing.T) {
	e := newEnv(t)

	task, err := e.tasks.Create(bg, &api.CreateTaskRequest{
		Title:  "Pinned task",
		Status: api.TaskStatus_TASK_STATUS_TODO,
		Pinned: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !task.Pinned {
		t.Fatal("Create: pinned=false, want true")
	}

	got, err := e.tasks.Get(bg, &api.GetTaskRequest{Id: task.Id})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.Pinned {
		t.Fatal("Get: pinned=false, want true")
	}

	// Verify persisted in file.
	node, _, _ := store.ParseFile(filepath.Join(e.dir, task.Id+".md"))
	if !node.Pinned {
		t.Fatal("file after Create: pinned=false, want true")
	}

	updated, err := e.tasks.Update(bg, &api.UpdateTaskRequest{
		Id:         task.Id,
		Pinned:     false,
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"pinned"}},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Pinned {
		t.Fatal("Update: pinned=true, want false")
	}

	node, _, _ = store.ParseFile(filepath.Join(e.dir, task.Id+".md"))
	if node.Pinned {
		t.Fatal("file after Update: pinned=true, want false")
	}
}

// TestTaskSubtask_ListByParentTaskId verifies that ListTasks(parent_task_id)
// returns only the direct children of the specified parent task.
func TestTaskSubtask_ListByParentTaskId(t *testing.T) {
	e := newEnv(t)

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
	// Sub-subtask — should NOT appear when filtering by root.Id.
	subsub, _ := e.tasks.Create(bg, &api.CreateTaskRequest{
		Title:        "SubSub",
		ProjectId:    proj.Id,
		ParentTaskId: sub1.Id,
		Status:       api.TaskStatus_TASK_STATUS_TODO,
	})

	children, err := e.tasks.List(bg, &api.ListTasksRequest{ParentTaskId: root.Id})
	if err != nil {
		t.Fatalf("ListTasks(parent_task_id): %v", err)
	}
	if len(children.Tasks) != 2 {
		t.Fatalf("ListTasks(parent_task_id=root): got %d, want 2", len(children.Tasks))
	}
	childIDs := map[string]bool{}
	for _, tk := range children.Tasks {
		childIDs[tk.Id] = true
	}
	if !childIDs[sub1.Id] || !childIDs[sub2.Id] {
		t.Errorf("missing sub1 or sub2 in children: %v", childIDs)
	}
	if childIDs[subsub.Id] {
		t.Error("sub-subtask should not appear when filtering by root.Id")
	}

	// Filtering by sub1 returns only subsub.
	grandchildren, err := e.tasks.List(bg, &api.ListTasksRequest{ParentTaskId: sub1.Id})
	if err != nil {
		t.Fatalf("ListTasks(parent_task_id=sub1): %v", err)
	}
	if len(grandchildren.Tasks) != 1 || grandchildren.Tasks[0].Id != subsub.Id {
		t.Fatalf("ListTasks(parent_task_id=sub1): got %v, want [%s]", grandchildren.Tasks, subsub.Id)
	}
}

