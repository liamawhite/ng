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
