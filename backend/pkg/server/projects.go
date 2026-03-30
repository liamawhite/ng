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
	statusStr := projectStatusToString(req.Status)
	node := &graph.Node{
		Type:        graph.EntityTypeProject,
		Title:       req.Title,
		Content:     req.Content,
		Status:      statusStr,
		CompletedAt: completedAtForStatus(statusStr, nil),
		Links:       protoToLinks(req.Links),
		Priority:    priorityToInt(req.Priority),
	}
	if req.EstimatedEffort != nil {
		node.EffortValue = req.EstimatedEffort.Value
		node.EffortUnit = effortUnitToString(req.EstimatedEffort.Unit)
	}
	// Only the in_area edge is stored on the project itself.
	edges := inAreaEdges("", req.AreaId)
	if err := s.store.Create(node, edges); err != nil {
		return nil, status.Errorf(codes.Internal, "create project: %v", err)
	}
	if req.ParentId != "" {
		if err := addChildEdge(s.store, req.ParentId, node.ID, "subproject"); err != nil {
			return nil, status.Errorf(codes.Internal, "add to parent project: %v", err)
		}
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
	// When filtering by parent, read its outgoing subproject edges to preserve insertion order.
	if req.ParentId != "" {
		edges := s.store.Graph().GetOutgoingEdges(req.ParentId)
		var projects []*api.Project
		for _, e := range edges {
			if e.Predicate != "subproject" {
				continue
			}
			n, ok := s.store.Graph().GetNode(e.TargetID)
			if !ok || n.Type != graph.EntityTypeProject {
				continue
			}
			p := s.nodeToProject(n)
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
	// No parent filter: return all projects sorted by ID.
	nodes := s.store.Graph().ListNodes(graph.EntityTypeProject)
	var projects []*api.Project
	for _, n := range nodes {
		p := s.nodeToProject(n)
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
		ID:          req.Id,
		Type:        graph.EntityTypeProject,
		Title:       existing.Title,
		Content:     existing.Content,
		Status:      existing.Status,
		CompletedAt: existing.CompletedAt,
		EffortValue: existing.EffortValue,
		EffortUnit:  existing.EffortUnit,
		Links:       existing.Links,
		Priority:    existing.Priority,
	}
	// Seed parentID from incoming subproject edges.
	parentID := ""
	for _, e := range s.store.Graph().GetIncomingEdges(req.Id) {
		if e.Predicate == "subproject" {
			parentID = e.SourceID
		}
	}
	oldParentID := parentID

	// Read all current outgoing edges (in_area + task children + subproject children).
	ownEdges := s.store.Graph().GetOutgoingEdges(req.Id)
	areaID := ""
	for _, e := range ownEdges {
		if e.Predicate == "in_area" {
			areaID = e.TargetID
		}
	}

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
			node.Status = projectStatusToString(req.Status)
			node.CompletedAt = completedAtForStatus(node.Status, existing.CompletedAt)
		case "area_id":
			areaID = req.AreaId
		case "parent_id":
			parentID = req.ParentId
		case "estimated_effort":
			if req.EstimatedEffort != nil {
				node.EffortValue = req.EstimatedEffort.Value
				node.EffortUnit = effortUnitToString(req.EstimatedEffort.Unit)
			} else {
				node.EffortValue = 0
				node.EffortUnit = ""
			}
		case "links":
			node.Links = protoToLinks(req.Links)
		case "priority":
			node.Priority = priorityToInt(req.Priority)
		}
	}

	// Handle parent change.
	if parentID != oldParentID {
		if oldParentID != "" {
			if err := removeChildEdge(s.store, oldParentID, req.Id); err != nil {
				return nil, status.Errorf(codes.Internal, "remove from old parent: %v", err)
			}
		}
		if parentID != "" {
			if err := addChildEdge(s.store, parentID, req.Id, "subproject"); err != nil {
				return nil, status.Errorf(codes.Internal, "add to new parent: %v", err)
			}
		}
	}

	// Build new own edges: replace in_area, preserve all child edges.
	var newOwnEdges []graph.Edge
	for _, e := range ownEdges {
		if e.Predicate != "in_area" {
			newOwnEdges = append(newOwnEdges, e)
		}
	}
	newOwnEdges = append(newOwnEdges, inAreaEdges(req.Id, areaID)...)

	if err := s.store.Update(node, newOwnEdges); err != nil {
		return nil, status.Errorf(codes.Internal, "update project: %v", err)
	}
	return s.nodeToProject(node), nil
}

func (s *ProjectServer) Delete(ctx context.Context, req *api.DeleteProjectRequest) (*emptypb.Empty, error) {
	if _, ok := s.store.Graph().GetNode(req.Id); !ok {
		return nil, status.Errorf(codes.NotFound, "project %q not found", req.Id)
	}
	// Remove from parent project's edge list before deleting.
	for _, e := range s.store.Graph().GetIncomingEdges(req.Id) {
		if e.Predicate == "subproject" {
			if err := removeChildEdge(s.store, e.SourceID, req.Id); err != nil {
				return nil, status.Errorf(codes.Internal, "remove from parent project: %v", err)
			}
		}
	}
	if err := s.store.Delete(req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "delete project: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func (s *ProjectServer) nodeToProject(n *graph.Node) *api.Project {
	p := &api.Project{
		Id:        n.ID,
		Title:     n.Title,
		Content:   n.Content,
		Status:    stringToProjectStatus(n.Status),
		Completed: timeToTimestamp(n.CompletedAt),
		Links:     linksToProto(n.Links),
		Priority:  intToPriority(n.Priority),
	}
	if n.EffortValue > 0 {
		p.EstimatedEffort = &api.Effort{
			Value: n.EffortValue,
			Unit:  stringToEffortUnit(n.EffortUnit),
		}
	}
	// in_area is still an outgoing edge on the project.
	for _, e := range s.store.Graph().GetOutgoingEdges(n.ID) {
		if e.Predicate == "in_area" {
			p.AreaId = e.TargetID
		}
	}
	// ParentId is derived from incoming subproject edges.
	for _, e := range s.store.Graph().GetIncomingEdges(n.ID) {
		if e.Predicate == "subproject" {
			p.ParentId = e.SourceID
		}
	}
	return p
}

func protoToLinks(proto []*api.Link) []graph.Link {
	links := make([]graph.Link, 0, len(proto))
	for _, l := range proto {
		links = append(links, graph.Link{URL: l.Url, Title: l.Title})
	}
	return links
}

func linksToProto(links []graph.Link) []*api.Link {
	result := make([]*api.Link, 0, len(links))
	for _, l := range links {
		result = append(result, &api.Link{Url: l.URL, Title: l.Title})
	}
	return result
}
