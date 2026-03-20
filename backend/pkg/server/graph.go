package server

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/liamawhite/ng/api/golang"
	"github.com/liamawhite/ng/backend/pkg/graph"
	"github.com/liamawhite/ng/backend/pkg/store"
)

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
