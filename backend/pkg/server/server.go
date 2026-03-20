package server

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/liamawhite/ng/api/golang"
	"github.com/liamawhite/ng/backend/pkg/graph"
	"github.com/liamawhite/ng/backend/pkg/store"
)

// ProjectServer implements api.ProjectServiceServer.
type ProjectServer struct {
	api.UnimplementedProjectServiceServer
	store *store.FileStore
}

func NewProjectServer(s *store.FileStore) *ProjectServer {
	return &ProjectServer{store: s}
}

func (s *ProjectServer) Create(ctx context.Context, req *api.CreateProjectRequest) (*api.Project, error) {
	if req.Title == "" {
		return nil, status.Error(codes.InvalidArgument, "title is required")
	}
	node := &graph.Node{
		Type:    graph.EntityTypeProject,
		Title:   req.Title,
		Content: req.Content,
	}
	if err := s.store.Create(node, partOfEdges("", req.ParentId)); err != nil {
		return nil, status.Errorf(codes.Internal, "create project: %v", err)
	}
	return s.nodeToProject(node), nil
}

func (s *ProjectServer) Get(ctx context.Context, req *api.GetProjectRequest) (*api.Project, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	node, ok := s.store.Graph().GetNode(req.Id)
	if !ok || node.Type != graph.EntityTypeProject {
		return nil, status.Errorf(codes.NotFound, "project %q not found", req.Id)
	}
	return s.nodeToProject(node), nil
}

func (s *ProjectServer) List(ctx context.Context, req *api.ListProjectsRequest) (*api.ListProjectsResponse, error) {
	nodes := s.store.Graph().ListNodes(graph.EntityTypeProject)
	var projects []*api.Project
	for _, n := range nodes {
		p := s.nodeToProject(n)
		if req.ParentId != "" && p.ParentId != req.ParentId {
			continue
		}
		projects = append(projects, p)
	}
	return &api.ListProjectsResponse{Projects: projects}, nil
}

func (s *ProjectServer) Update(ctx context.Context, req *api.UpdateProjectRequest) (*api.Project, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	existing, ok := s.store.Graph().GetNode(req.Id)
	if !ok || existing.Type != graph.EntityTypeProject {
		return nil, status.Errorf(codes.NotFound, "project %q not found", req.Id)
	}
	node := &graph.Node{
		ID:      req.Id,
		Type:    graph.EntityTypeProject,
		Title:   req.Title,
		Content: req.Content,
	}
	if err := s.store.Update(node, partOfEdges(req.Id, req.ParentId)); err != nil {
		return nil, status.Errorf(codes.Internal, "update project: %v", err)
	}
	return s.nodeToProject(node), nil
}

func (s *ProjectServer) Delete(ctx context.Context, req *api.DeleteProjectRequest) (*emptypb.Empty, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if _, ok := s.store.Graph().GetNode(req.Id); !ok {
		return nil, status.Errorf(codes.NotFound, "project %q not found", req.Id)
	}
	if err := s.store.Delete(req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "delete project: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func (s *ProjectServer) nodeToProject(n *graph.Node) *api.Project {
	p := &api.Project{Id: n.ID, Title: n.Title, Content: n.Content}
	for _, e := range s.store.Graph().GetOutgoingEdges(n.ID) {
		if e.Predicate == "part_of" {
			p.ParentId = e.TargetID
			break
		}
	}
	return p
}

// TaskServer implements api.TaskServiceServer.
type TaskServer struct {
	api.UnimplementedTaskServiceServer
	store *store.FileStore
}

func NewTaskServer(s *store.FileStore) *TaskServer {
	return &TaskServer{store: s}
}

func (s *TaskServer) Create(ctx context.Context, req *api.CreateTaskRequest) (*api.Task, error) {
	if req.Title == "" {
		return nil, status.Error(codes.InvalidArgument, "title is required")
	}
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
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
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
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
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
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
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

// GraphServer implements api.GraphServiceServer.
type GraphServer struct {
	api.UnimplementedGraphServiceServer
	store *store.FileStore
}

func NewGraphServer(s *store.FileStore) *GraphServer {
	return &GraphServer{store: s}
}

func (s *GraphServer) ListRelated(ctx context.Context, req *api.ListRelatedRequest) (*api.ListRelatedResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	related := s.store.Graph().ListRelated(
		req.Id,
		protoPredicateToString(req.Predicate),
		protoDirectionToString(req.Direction),
	)

	entities := make([]*api.RelatedEntity, 0, len(related))
	for _, r := range related {
		entity, err := s.nodeToEntity(r.Node)
		if err != nil {
			continue
		}
		entities = append(entities, &api.RelatedEntity{
			Predicate: stringToPredicate(r.Predicate),
			Direction: stringToDirection(r.Direction),
			Entity:    entity,
		})
	}
	return &api.ListRelatedResponse{Entities: entities}, nil
}

func (s *GraphServer) nodeToEntity(n *graph.Node) (*api.Entity, error) {
	switch n.Type {
	case graph.EntityTypeProject:
		ps := &ProjectServer{store: s.store}
		return &api.Entity{Entity: &api.Entity_Project{Project: ps.nodeToProject(n)}}, nil
	case graph.EntityTypeTask:
		ts := &TaskServer{store: s.store}
		return &api.Entity{Entity: &api.Entity_Task{Task: ts.nodeToTask(n)}}, nil
	default:
		return nil, fmt.Errorf("unknown entity type: %s", n.Type)
	}
}

// --- Shared helpers ---

func partOfEdges(sourceID, targetID string) []graph.Edge {
	if targetID == "" {
		return nil
	}
	return []graph.Edge{{SourceID: sourceID, Predicate: "part_of", TargetID: targetID}}
}

func taskStatusToString(s api.TaskStatus) string {
	switch s {
	case api.TaskStatus_TASK_STATUS_TODO:
		return "todo"
	case api.TaskStatus_TASK_STATUS_IN_PROGRESS:
		return "in_progress"
	case api.TaskStatus_TASK_STATUS_DONE:
		return "done"
	default:
		return "todo"
	}
}

func stringToTaskStatus(s string) api.TaskStatus {
	switch s {
	case "todo":
		return api.TaskStatus_TASK_STATUS_TODO
	case "in_progress":
		return api.TaskStatus_TASK_STATUS_IN_PROGRESS
	case "done":
		return api.TaskStatus_TASK_STATUS_DONE
	default:
		return api.TaskStatus_TASK_STATUS_UNSPECIFIED
	}
}

func protoPredicateToString(p api.Predicate) string {
	switch p {
	case api.Predicate_PREDICATE_PART_OF:
		return "part_of"
	default:
		return ""
	}
}

func protoDirectionToString(d api.Direction) string {
	switch d {
	case api.Direction_DIRECTION_OUTGOING:
		return "outgoing"
	case api.Direction_DIRECTION_INCOMING:
		return "incoming"
	default:
		return ""
	}
}

func stringToPredicate(s string) api.Predicate {
	switch s {
	case "part_of":
		return api.Predicate_PREDICATE_PART_OF
	default:
		return api.Predicate_PREDICATE_UNSPECIFIED
	}
}

func stringToDirection(s string) api.Direction {
	switch s {
	case "outgoing":
		return api.Direction_DIRECTION_OUTGOING
	case "incoming":
		return api.Direction_DIRECTION_INCOMING
	default:
		return api.Direction_DIRECTION_UNSPECIFIED
	}
}
