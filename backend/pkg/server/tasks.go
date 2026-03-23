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
	node := &graph.Node{
		Type:    graph.EntityTypeTask,
		Title:   req.Title,
		Content: req.Content,
		Status:  taskStatusToString(req.Status),
	}
	if err := s.store.Create(node, partOfEdges("", req.ProjectId)); err != nil {
		return nil, status.Errorf(codes.Internal, "create task: %v", err)
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
	nodes := s.store.Graph().ListNodes(graph.EntityTypeTask)
	var tasks []*api.Task
	for _, n := range nodes {
		t := s.nodeToTask(n)
		if req.ProjectId != "" && t.ProjectId != req.ProjectId {
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
		ID:      req.Id,
		Type:    graph.EntityTypeTask,
		Title:   req.Title,
		Content: req.Content,
		Status:  taskStatusToString(req.Status),
	}
	if err := s.store.Update(node, partOfEdges(req.Id, req.ProjectId)); err != nil {
		return nil, status.Errorf(codes.Internal, "update task: %v", err)
	}
	return s.nodeToTask(node), nil
}

func (s *TaskServer) Delete(ctx context.Context, req *api.DeleteTaskRequest) (*emptypb.Empty, error) {
	if _, ok := s.store.Graph().GetNode(req.Id); !ok {
		return nil, status.Errorf(codes.NotFound, "task %q not found", req.Id)
	}
	if err := s.store.Delete(req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "delete task: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func (s *TaskServer) nodeToTask(n *graph.Node) *api.Task {
	t := &api.Task{Id: n.ID, Title: n.Title, Content: n.Content, Status: stringToTaskStatus(n.Status)}
	for _, e := range s.store.Graph().GetOutgoingEdges(n.ID) {
		if e.Predicate == "part_of" {
			t.ProjectId = e.TargetID
			break
		}
	}
	return t
}
