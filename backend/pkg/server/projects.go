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
	node := &graph.Node{
		Type:    graph.EntityTypeProject,
		Title:   req.Title,
		Content: req.Content,
		Status:  projectStatusToString(req.Status),
	}
	edges := append(partOfEdges("", req.ParentId), inAreaEdges("", req.AreaId)...)
	if err := s.store.Create(node, edges); err != nil {
		return nil, status.Errorf(codes.Internal, "create project: %v", err)
	}
	return s.nodeToProject(node), nil
}

func (s *ProjectServer) Get(ctx context.Context, req *api.GetProjectRequest) (*api.Project, error) {
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
		if req.Status != api.ProjectStatus_PROJECT_STATUS_UNSPECIFIED && p.Status != req.Status {
			continue
		}
		if req.AreaId != "" && p.AreaId != req.AreaId {
			continue
		}
		projects = append(projects, p)
	}
	return &api.ListProjectsResponse{Projects: projects}, nil
}

func (s *ProjectServer) Update(ctx context.Context, req *api.UpdateProjectRequest) (*api.Project, error) {
	existing, ok := s.store.Graph().GetNode(req.Id)
	if !ok || existing.Type != graph.EntityTypeProject {
		return nil, status.Errorf(codes.NotFound, "project %q not found", req.Id)
	}
	node := &graph.Node{
		ID:      req.Id,
		Type:    graph.EntityTypeProject,
		Title:   req.Title,
		Content: req.Content,
		Status:  projectStatusToString(req.Status),
	}
	edges := append(partOfEdges(req.Id, req.ParentId), inAreaEdges(req.Id, req.AreaId)...)
	if err := s.store.Update(node, edges); err != nil {
		return nil, status.Errorf(codes.Internal, "update project: %v", err)
	}
	return s.nodeToProject(node), nil
}

func (s *ProjectServer) Delete(ctx context.Context, req *api.DeleteProjectRequest) (*emptypb.Empty, error) {
	if _, ok := s.store.Graph().GetNode(req.Id); !ok {
		return nil, status.Errorf(codes.NotFound, "project %q not found", req.Id)
	}
	if err := s.store.Delete(req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "delete project: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func (s *ProjectServer) nodeToProject(n *graph.Node) *api.Project {
	p := &api.Project{
		Id:      n.ID,
		Title:   n.Title,
		Content: n.Content,
		Status:  stringToProjectStatus(n.Status),
	}
	for _, e := range s.store.Graph().GetOutgoingEdges(n.ID) {
		switch e.Predicate {
		case "part_of":
			p.ParentId = e.TargetID
		case "in_area":
			p.AreaId = e.TargetID
		}
	}
	return p
}
