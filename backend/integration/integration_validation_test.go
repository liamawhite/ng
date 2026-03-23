package integration_test

// Validation tests exercise the ValidationInterceptor via a real in-memory
// gRPC server (using bufconn). The interceptor only fires on the gRPC
// transport, so tests that call server methods directly would bypass it.

import (
	"context"
	"net"
	"testing"

	api "github.com/liamawhite/ng/api/golang"
	"github.com/liamawhite/ng/backend/pkg/graph"
	"github.com/liamawhite/ng/backend/pkg/server"
	"github.com/liamawhite/ng/backend/pkg/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1 << 20 // 1 MiB

// validationEnv spins up a real gRPC server with the ValidationInterceptor
// over an in-memory bufconn listener, then returns gRPC clients for each
// service. All requests go through the interceptor exactly as they would in
// production.
type validationEnv struct {
	areas    api.AreaServiceClient
	projects api.ProjectServiceClient
	tasks    api.TaskServiceClient
	graph    api.GraphServiceClient
}

func newValidationEnv(t *testing.T) *validationEnv {
	t.Helper()

	dir := t.TempDir()
	g := graph.New()
	s := store.New(dir, g)

	areaServer := server.NewAreaServer(s)
	projectServer := server.NewProjectServer(s)
	taskServer := server.NewTaskServer(s)
	graphServer := server.NewGraphServer(s)

	lis := bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(server.ValidationInterceptor))
	api.RegisterAreaServiceServer(grpcServer, areaServer)
	api.RegisterProjectServiceServer(grpcServer, projectServer)
	api.RegisterTaskServiceServer(grpcServer, taskServer)
	api.RegisterGraphServiceServer(grpcServer, graphServer)

	go grpcServer.Serve(lis) //nolint:errcheck
	t.Cleanup(grpcServer.GracefulStop)

	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return &validationEnv{
		areas:    api.NewAreaServiceClient(conn),
		projects: api.NewProjectServiceClient(conn),
		tasks:    api.NewTaskServiceClient(conn),
		graph:    api.NewGraphServiceClient(conn),
	}
}

func isInvalidArgument(err error) bool {
	s, ok := status.FromError(err)
	return ok && s.Code() == codes.InvalidArgument
}

// ---- Area validation ----

func TestValidation_CreateArea_EmptyTitle(t *testing.T) {
	e := newValidationEnv(t)
	_, err := e.areas.Create(context.Background(), &api.CreateAreaRequest{Title: ""})
	if !isInvalidArgument(err) {
		t.Fatalf("CreateArea empty title: expected InvalidArgument, got %v", err)
	}
}

func TestValidation_GetArea_EmptyID(t *testing.T) {
	e := newValidationEnv(t)
	_, err := e.areas.Get(context.Background(), &api.GetAreaRequest{Id: ""})
	if !isInvalidArgument(err) {
		t.Fatalf("GetArea empty id: expected InvalidArgument, got %v", err)
	}
}

func TestValidation_UpdateArea_EmptyID(t *testing.T) {
	e := newValidationEnv(t)
	_, err := e.areas.Update(context.Background(), &api.UpdateAreaRequest{Id: "", Title: "x"})
	if !isInvalidArgument(err) {
		t.Fatalf("UpdateArea empty id: expected InvalidArgument, got %v", err)
	}
}

func TestValidation_DeleteArea_EmptyID(t *testing.T) {
	e := newValidationEnv(t)
	_, err := e.areas.Delete(context.Background(), &api.DeleteAreaRequest{Id: ""})
	if !isInvalidArgument(err) {
		t.Fatalf("DeleteArea empty id: expected InvalidArgument, got %v", err)
	}
}

// ---- Project validation ----

func TestValidation_CreateProject_EmptyTitle(t *testing.T) {
	e := newValidationEnv(t)
	_, err := e.projects.Create(context.Background(), &api.CreateProjectRequest{Title: ""})
	if !isInvalidArgument(err) {
		t.Fatalf("CreateProject empty title: expected InvalidArgument, got %v", err)
	}
}

func TestValidation_GetProject_EmptyID(t *testing.T) {
	e := newValidationEnv(t)
	_, err := e.projects.Get(context.Background(), &api.GetProjectRequest{Id: ""})
	if !isInvalidArgument(err) {
		t.Fatalf("GetProject empty id: expected InvalidArgument, got %v", err)
	}
}

func TestValidation_UpdateProject_EmptyID(t *testing.T) {
	e := newValidationEnv(t)
	_, err := e.projects.Update(context.Background(), &api.UpdateProjectRequest{Id: "", Title: "x"})
	if !isInvalidArgument(err) {
		t.Fatalf("UpdateProject empty id: expected InvalidArgument, got %v", err)
	}
}

func TestValidation_DeleteProject_EmptyID(t *testing.T) {
	e := newValidationEnv(t)
	_, err := e.projects.Delete(context.Background(), &api.DeleteProjectRequest{Id: ""})
	if !isInvalidArgument(err) {
		t.Fatalf("DeleteProject empty id: expected InvalidArgument, got %v", err)
	}
}

// ---- Task validation ----

func TestValidation_CreateTask_EmptyTitle(t *testing.T) {
	e := newValidationEnv(t)
	_, err := e.tasks.Create(context.Background(), &api.CreateTaskRequest{Title: ""})
	if !isInvalidArgument(err) {
		t.Fatalf("CreateTask empty title: expected InvalidArgument, got %v", err)
	}
}

func TestValidation_GetTask_EmptyID(t *testing.T) {
	e := newValidationEnv(t)
	_, err := e.tasks.Get(context.Background(), &api.GetTaskRequest{Id: ""})
	if !isInvalidArgument(err) {
		t.Fatalf("GetTask empty id: expected InvalidArgument, got %v", err)
	}
}

func TestValidation_UpdateTask_EmptyID(t *testing.T) {
	e := newValidationEnv(t)
	_, err := e.tasks.Update(context.Background(), &api.UpdateTaskRequest{Id: "", Title: "x"})
	if !isInvalidArgument(err) {
		t.Fatalf("UpdateTask empty id: expected InvalidArgument, got %v", err)
	}
}

func TestValidation_DeleteTask_EmptyID(t *testing.T) {
	e := newValidationEnv(t)
	_, err := e.tasks.Delete(context.Background(), &api.DeleteTaskRequest{Id: ""})
	if !isInvalidArgument(err) {
		t.Fatalf("DeleteTask empty id: expected InvalidArgument, got %v", err)
	}
}

// ---- Graph validation ----

func TestValidation_ListRelated_EmptyID(t *testing.T) {
	e := newValidationEnv(t)
	_, err := e.graph.ListRelated(context.Background(), &api.ListRelatedRequest{Id: ""})
	if !isInvalidArgument(err) {
		t.Fatalf("ListRelated empty id: expected InvalidArgument, got %v", err)
	}
}
