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

// AreaServer implements api.AreaServiceServer.
type AreaServer struct {
	api.UnimplementedAreaServiceServer
	store *store.FileStore
}

func NewAreaServer(s *store.FileStore) *AreaServer {
	return &AreaServer{store: s}
}

func (s *AreaServer) Create(ctx context.Context, req *api.CreateAreaRequest) (*api.Area, error) {
	node := &graph.Node{
		Type:  graph.EntityTypeArea,
		Title: req.Title,
		Color: req.Color,
	}
	if err := s.store.Create(node, nil); err != nil {
		return nil, status.Errorf(codes.Internal, "create area: %v", err)
	}
	return nodeToArea(node), nil
}

func (s *AreaServer) Get(ctx context.Context, req *api.GetAreaRequest) (*api.Area, error) {
	node, ok := s.store.Graph().GetNode(req.Id)
	if !ok || node.Type != graph.EntityTypeArea {
		return nil, status.Errorf(codes.NotFound, "area %q not found", req.Id)
	}
	return nodeToArea(node), nil
}

func (s *AreaServer) List(ctx context.Context, req *api.ListAreasRequest) (*api.ListAreasResponse, error) {
	nodes := s.store.Graph().ListNodes(graph.EntityTypeArea)
	areas := make([]*api.Area, 0, len(nodes))
	for _, n := range nodes {
		areas = append(areas, nodeToArea(n))
	}
	return &api.ListAreasResponse{Areas: areas}, nil
}

func (s *AreaServer) Update(ctx context.Context, req *api.UpdateAreaRequest) (*api.Area, error) {
	existing, ok := s.store.Graph().GetNode(req.Id)
	if !ok || existing.Type != graph.EntityTypeArea {
		return nil, status.Errorf(codes.NotFound, "area %q not found", req.Id)
	}
	node := &graph.Node{
		ID:    req.Id,
		Type:  graph.EntityTypeArea,
		Title: existing.Title,
		Color: existing.Color,
	}
	for _, path := range req.UpdateMask.Paths {
		switch path {
		case "title":
			if req.Title == "" {
				return nil, status.Errorf(codes.InvalidArgument, "title cannot be empty")
			}
			node.Title = req.Title
		case "color":
			node.Color = req.Color
		}
	}
	if err := s.store.Update(node, nil); err != nil {
		return nil, status.Errorf(codes.Internal, "update area: %v", err)
	}
	return nodeToArea(node), nil
}

func (s *AreaServer) Delete(ctx context.Context, req *api.DeleteAreaRequest) (*emptypb.Empty, error) {
	if _, ok := s.store.Graph().GetNode(req.Id); !ok {
		return nil, status.Errorf(codes.NotFound, "area %q not found", req.Id)
	}
	if err := s.store.Delete(req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "delete area: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func nodeToArea(n *graph.Node) *api.Area {
	return &api.Area{Id: n.ID, Title: n.Title, Color: n.Color}
}
