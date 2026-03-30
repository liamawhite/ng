package server

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/liamawhite/ng/api/golang"
	"github.com/liamawhite/ng/backend/pkg/graph"
	"github.com/liamawhite/ng/backend/pkg/store"
)

// TaskServer implements api.TaskServiceServer.
type TaskServer struct {
	api.UnimplementedTaskServiceServer
	store *store.FileStore
}

func NewTaskServer(s *store.FileStore) *TaskServer {
	return &TaskServer{store: s}
}

func (s *TaskServer) Create(ctx context.Context, req *api.CreateTaskRequest) (*api.Task, error) {
	statusStr := taskStatusToString(req.Status)
	node := &graph.Node{
		Type:        graph.EntityTypeTask,
		Title:       req.Title,
		Content:     req.Content,
		Status:      statusStr,
		CompletedAt: completedAtForStatus(statusStr, nil),
		Pinned:      req.Pinned,
		Priority:    priorityToInt(req.Priority),
	}
	// Tasks have no outgoing relationship edges of their own; parents store the edges.
	if err := s.store.Create(node, nil); err != nil {
		return nil, status.Errorf(codes.Internal, "create task: %v", err)
	}
	if req.ProjectId != "" {
		if err := addChildEdge(s.store, req.ProjectId, node.ID, "task"); err != nil {
			return nil, status.Errorf(codes.Internal, "add task to project: %v", err)
		}
	}
	if req.ParentTaskId != "" {
		if err := addChildEdge(s.store, req.ParentTaskId, node.ID, "subtask"); err != nil {
			return nil, status.Errorf(codes.Internal, "add subtask to parent task: %v", err)
		}
	}
	return s.nodeToTask(node), nil
}

func (s *TaskServer) Get(ctx context.Context, req *api.GetTaskRequest) (*api.Task, error) {
	node, ok := s.store.Graph().GetNode(req.Id)
	if !ok || node.Type != graph.EntityTypeTask {
		return nil, status.Errorf(codes.NotFound, "task %q not found", req.Id)
	}
	return s.nodeToTask(node), nil
}

func (s *TaskServer) List(ctx context.Context, req *api.ListTasksRequest) (*api.ListTasksResponse, error) {
	// When filtering by a parent, read its outgoing edges to preserve insertion order.
	if req.ProjectId != "" || req.ParentTaskId != "" {
		parentID := req.ProjectId
		predicate := "task"
		if req.ParentTaskId != "" {
			parentID = req.ParentTaskId
			predicate = "subtask"
		}
		edges := s.store.Graph().GetOutgoingEdges(parentID)
		var tasks []*api.Task
		for _, e := range edges {
			if e.Predicate != predicate {
				continue
			}
			n, ok := s.store.Graph().GetNode(e.TargetID)
			if !ok || n.Type != graph.EntityTypeTask {
				continue
			}
			t := s.nodeToTask(n)
			if req.Pinned != nil && t.Pinned != req.GetPinned() {
				continue
			}
			tasks = append(tasks, t)
		}
		return &api.ListTasksResponse{Tasks: tasks}, nil
	}
	// No parent filter: return all tasks sorted by ID.
	nodes := s.store.Graph().ListNodes(graph.EntityTypeTask)
	var tasks []*api.Task
	for _, n := range nodes {
		t := s.nodeToTask(n)
		if req.Pinned != nil && t.Pinned != req.GetPinned() {
			continue
		}
		tasks = append(tasks, t)
	}
	return &api.ListTasksResponse{Tasks: tasks}, nil
}

func (s *TaskServer) Update(ctx context.Context, req *api.UpdateTaskRequest) (*api.Task, error) {
	existing, ok := s.store.Graph().GetNode(req.Id)
	if !ok || existing.Type != graph.EntityTypeTask {
		return nil, status.Errorf(codes.NotFound, "task %q not found", req.Id)
	}
	node := &graph.Node{
		ID:          req.Id,
		Type:        graph.EntityTypeTask,
		Title:       existing.Title,
		Content:     existing.Content,
		Status:      existing.Status,
		CompletedAt: existing.CompletedAt,
		Pinned:      existing.Pinned,
		Priority:    existing.Priority,
	}
	// Seed existing parent IDs from incoming edges.
	projectID, parentTaskID := "", ""
	for _, e := range s.store.Graph().GetIncomingEdges(req.Id) {
		switch e.Predicate {
		case "task":
			projectID = e.SourceID
		case "subtask":
			parentTaskID = e.SourceID
		}
	}
	oldProjectID := projectID
	oldParentTaskID := parentTaskID

	for _, path := range req.UpdateMask.Paths {
		switch path {
		case "title":
			if req.Title == "" {
				return nil, status.Errorf(codes.InvalidArgument, "title cannot be empty")
			}
			node.Title = req.Title
		case "content":
			node.Content = req.Content
		case "status":
			node.Status = taskStatusToString(req.Status)
			node.CompletedAt = completedAtForStatus(node.Status, existing.CompletedAt)
		case "project_id":
			projectID = req.ProjectId
		case "parent_task_id":
			parentTaskID = req.ParentTaskId
		case "pinned":
			node.Pinned = req.Pinned
		case "priority":
			node.Priority = priorityToInt(req.Priority)
		}
	}

	// Handle project change.
	if projectID != oldProjectID {
		if oldProjectID != "" {
			if err := removeChildEdge(s.store, oldProjectID, req.Id); err != nil {
				return nil, status.Errorf(codes.Internal, "remove from old project: %v", err)
			}
		}
		if projectID != "" {
			if err := addChildEdge(s.store, projectID, req.Id, "task"); err != nil {
				return nil, status.Errorf(codes.Internal, "add to new project: %v", err)
			}
		}
	}

	// Handle parent task change.
	if parentTaskID != oldParentTaskID {
		if oldParentTaskID != "" {
			if err := removeChildEdge(s.store, oldParentTaskID, req.Id); err != nil {
				return nil, status.Errorf(codes.Internal, "remove from old parent task: %v", err)
			}
		}
		if parentTaskID != "" {
			if err := addChildEdge(s.store, parentTaskID, req.Id, "subtask"); err != nil {
				return nil, status.Errorf(codes.Internal, "add to new parent task: %v", err)
			}
		}
	}

	// Update the task node with its own outgoing edges (subtask children, unchanged).
	ownEdges := s.store.Graph().GetOutgoingEdges(req.Id)
	if err := s.store.Update(node, ownEdges); err != nil {
		return nil, status.Errorf(codes.Internal, "update task: %v", err)
	}
	return s.nodeToTask(node), nil
}

func (s *TaskServer) Delete(ctx context.Context, req *api.DeleteTaskRequest) (*emptypb.Empty, error) {
	if _, ok := s.store.Graph().GetNode(req.Id); !ok {
		return nil, status.Errorf(codes.NotFound, "task %q not found", req.Id)
	}
	// Remove from parent edge lists before deleting.
	for _, e := range s.store.Graph().GetIncomingEdges(req.Id) {
		if e.Predicate == "task" || e.Predicate == "subtask" {
			if err := removeChildEdge(s.store, e.SourceID, req.Id); err != nil {
				return nil, status.Errorf(codes.Internal, "remove from parent: %v", err)
			}
		}
	}
	if err := s.store.Delete(req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "delete task: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func (s *TaskServer) nodeToTask(n *graph.Node) *api.Task {
	t := &api.Task{Id: n.ID, Title: n.Title, Content: n.Content, Status: stringToTaskStatus(n.Status), Completed: timeToTimestamp(n.CompletedAt), Pinned: n.Pinned, Priority: intToPriority(n.Priority)}
	// Parent IDs are derived from incoming edges — the parent owns the relationship.
	for _, e := range s.store.Graph().GetIncomingEdges(n.ID) {
		switch e.Predicate {
		case "task":
			t.ProjectId = e.SourceID
		case "subtask":
			t.ParentTaskId = e.SourceID
		}
	}
	return t
}
