package integration_test

import (
	"testing"
	"time"

	api "github.com/liamawhite/ng/api/golang"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// ---- Task completed timestamp tests ----

func TestTaskCreate_CompletedTimestamp(t *testing.T) {
	tests := []struct {
		name      string
		status    api.TaskStatus
		wantSet   bool
	}{
		{"todo→nil", api.TaskStatus_TASK_STATUS_TODO, false},
		{"in_progress→nil", api.TaskStatus_TASK_STATUS_IN_PROGRESS, false},
		{"done→set", api.TaskStatus_TASK_STATUS_DONE, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			before := time.Now()
			task, err := e.tasks.Create(bg, &api.CreateTaskRequest{
				Title:  "T",
				Status: tc.status,
			})
			after := time.Now()
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			if tc.wantSet {
				if task.Completed == nil {
					t.Fatal("completed is nil, want set")
				}
				ts := task.Completed.AsTime()
				if ts.Before(before) || ts.After(after) {
					t.Fatalf("completed %v outside expected range [%v, %v]", ts, before, after)
				}
			} else {
				if task.Completed != nil {
					t.Fatalf("completed is %v, want nil", task.Completed.AsTime())
				}
			}
		})
	}
}

func TestTaskUpdate_CompletedTimestamp(t *testing.T) {
	tests := []struct {
		name          string
		initialStatus api.TaskStatus
		newStatus     api.TaskStatus
		wantSet       bool
		wantPreserved bool // true: completed must equal the value set on initial create
	}{
		{"todo→done sets", api.TaskStatus_TASK_STATUS_TODO, api.TaskStatus_TASK_STATUS_DONE, true, false},
		{"in_progress→done sets", api.TaskStatus_TASK_STATUS_IN_PROGRESS, api.TaskStatus_TASK_STATUS_DONE, true, false},
		{"done→todo clears", api.TaskStatus_TASK_STATUS_DONE, api.TaskStatus_TASK_STATUS_TODO, false, false},
		{"done→in_progress clears", api.TaskStatus_TASK_STATUS_DONE, api.TaskStatus_TASK_STATUS_IN_PROGRESS, false, false},
		{"done→done preserves", api.TaskStatus_TASK_STATUS_DONE, api.TaskStatus_TASK_STATUS_DONE, true, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			created, err := e.tasks.Create(bg, &api.CreateTaskRequest{
				Title:  "T",
				Status: tc.initialStatus,
			})
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			before := time.Now()
			updated, err := e.tasks.Update(bg, &api.UpdateTaskRequest{
				Id:         created.Id,
				Status:     tc.newStatus,
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"status"}},
			})
			after := time.Now()
			if err != nil {
				t.Fatalf("Update: %v", err)
			}

			switch {
			case !tc.wantSet:
				if updated.Completed != nil {
					t.Fatalf("completed is %v, want nil", updated.Completed.AsTime())
				}
			case tc.wantPreserved:
				if updated.Completed == nil {
					t.Fatal("completed is nil, want preserved")
				}
				if created.Completed == nil {
					t.Fatal("initial completed was nil; cannot verify preservation")
				}
				if !updated.Completed.AsTime().Equal(created.Completed.AsTime()) {
					t.Fatalf("completed changed: got %v, want %v", updated.Completed.AsTime(), created.Completed.AsTime())
				}
			default:
				if updated.Completed == nil {
					t.Fatal("completed is nil, want set")
				}
				ts := updated.Completed.AsTime()
				if ts.Before(before) || ts.After(after) {
					t.Fatalf("completed %v outside expected range [%v, %v]", ts, before, after)
				}
			}
		})
	}
}

func TestTaskUpdate_NonStatusMask_CompletedUnchanged(t *testing.T) {
	e := newEnv(t)

	// Create a done task so it has a completed timestamp.
	created, err := e.tasks.Create(bg, &api.CreateTaskRequest{
		Title:  "T",
		Status: api.TaskStatus_TASK_STATUS_DONE,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Completed == nil {
		t.Fatal("completed is nil after create with DONE status")
	}

	// Update only the title — completed must not change.
	updated, err := e.tasks.Update(bg, &api.UpdateTaskRequest{
		Id:         created.Id,
		Title:      "Updated",
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"title"}},
	})
	if err != nil {
		t.Fatalf("Update title: %v", err)
	}
	if updated.Completed == nil {
		t.Fatal("completed cleared by non-status update")
	}
	if !updated.Completed.AsTime().Equal(created.Completed.AsTime()) {
		t.Fatalf("completed changed: got %v, want %v", updated.Completed.AsTime(), created.Completed.AsTime())
	}
}

// ---- Project completed timestamp tests ----

func TestProjectCreate_CompletedTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		status  api.ProjectStatus
		wantSet bool
	}{
		{"unspecified→nil", api.ProjectStatus_PROJECT_STATUS_UNSPECIFIED, false},
		{"active→nil", api.ProjectStatus_PROJECT_STATUS_ACTIVE, false},
		{"backlog→nil", api.ProjectStatus_PROJECT_STATUS_BACKLOG, false},
		{"blocked→nil", api.ProjectStatus_PROJECT_STATUS_BLOCKED, false},
		{"abandoned→nil", api.ProjectStatus_PROJECT_STATUS_ABANDONED, false},
		{"completed→set", api.ProjectStatus_PROJECT_STATUS_COMPLETED, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			before := time.Now()
			proj, err := e.projects.Create(bg, &api.CreateProjectRequest{
				Title:  "P",
				Status: tc.status,
			})
			after := time.Now()
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			if tc.wantSet {
				if proj.Completed == nil {
					t.Fatal("completed is nil, want set")
				}
				ts := proj.Completed.AsTime()
				if ts.Before(before) || ts.After(after) {
					t.Fatalf("completed %v outside expected range [%v, %v]", ts, before, after)
				}
			} else {
				if proj.Completed != nil {
					t.Fatalf("completed is %v, want nil", proj.Completed.AsTime())
				}
			}
		})
	}
}

func TestProjectUpdate_CompletedTimestamp(t *testing.T) {
	tests := []struct {
		name          string
		initialStatus api.ProjectStatus
		newStatus     api.ProjectStatus
		wantSet       bool
		wantPreserved bool
	}{
		{"active→completed sets", api.ProjectStatus_PROJECT_STATUS_ACTIVE, api.ProjectStatus_PROJECT_STATUS_COMPLETED, true, false},
		{"backlog→completed sets", api.ProjectStatus_PROJECT_STATUS_BACKLOG, api.ProjectStatus_PROJECT_STATUS_COMPLETED, true, false},
		{"blocked→completed sets", api.ProjectStatus_PROJECT_STATUS_BLOCKED, api.ProjectStatus_PROJECT_STATUS_COMPLETED, true, false},
		{"abandoned→completed sets", api.ProjectStatus_PROJECT_STATUS_ABANDONED, api.ProjectStatus_PROJECT_STATUS_COMPLETED, true, false},
		{"completed→active clears", api.ProjectStatus_PROJECT_STATUS_COMPLETED, api.ProjectStatus_PROJECT_STATUS_ACTIVE, false, false},
		{"completed→backlog clears", api.ProjectStatus_PROJECT_STATUS_COMPLETED, api.ProjectStatus_PROJECT_STATUS_BACKLOG, false, false},
		{"completed→blocked clears", api.ProjectStatus_PROJECT_STATUS_COMPLETED, api.ProjectStatus_PROJECT_STATUS_BLOCKED, false, false},
		{"completed→abandoned clears", api.ProjectStatus_PROJECT_STATUS_COMPLETED, api.ProjectStatus_PROJECT_STATUS_ABANDONED, false, false},
		{"completed→completed preserves", api.ProjectStatus_PROJECT_STATUS_COMPLETED, api.ProjectStatus_PROJECT_STATUS_COMPLETED, true, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			created, err := e.projects.Create(bg, &api.CreateProjectRequest{
				Title:  "P",
				Status: tc.initialStatus,
			})
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			before := time.Now()
			updated, err := e.projects.Update(bg, &api.UpdateProjectRequest{
				Id:         created.Id,
				Status:     tc.newStatus,
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"status"}},
			})
			after := time.Now()
			if err != nil {
				t.Fatalf("Update: %v", err)
			}

			switch {
			case !tc.wantSet:
				if updated.Completed != nil {
					t.Fatalf("completed is %v, want nil", updated.Completed.AsTime())
				}
			case tc.wantPreserved:
				if updated.Completed == nil {
					t.Fatal("completed is nil, want preserved")
				}
				if created.Completed == nil {
					t.Fatal("initial completed was nil; cannot verify preservation")
				}
				if !updated.Completed.AsTime().Equal(created.Completed.AsTime()) {
					t.Fatalf("completed changed: got %v, want %v", updated.Completed.AsTime(), created.Completed.AsTime())
				}
			default:
				if updated.Completed == nil {
					t.Fatal("completed is nil, want set")
				}
				ts := updated.Completed.AsTime()
				if ts.Before(before) || ts.After(after) {
					t.Fatalf("completed %v outside expected range [%v, %v]", ts, before, after)
				}
			}
		})
	}
}

func TestProjectUpdate_NonStatusMask_CompletedUnchanged(t *testing.T) {
	e := newEnv(t)

	created, err := e.projects.Create(bg, &api.CreateProjectRequest{
		Title:  "P",
		Status: api.ProjectStatus_PROJECT_STATUS_COMPLETED,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Completed == nil {
		t.Fatal("completed is nil after create with COMPLETED status")
	}

	updated, err := e.projects.Update(bg, &api.UpdateProjectRequest{
		Id:         created.Id,
		Title:      "Updated",
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"title"}},
	})
	if err != nil {
		t.Fatalf("Update title: %v", err)
	}
	if updated.Completed == nil {
		t.Fatal("completed cleared by non-status update")
	}
	if !updated.Completed.AsTime().Equal(created.Completed.AsTime()) {
		t.Fatalf("completed changed: got %v, want %v", updated.Completed.AsTime(), created.Completed.AsTime())
	}
}
