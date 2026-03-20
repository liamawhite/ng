package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"

	api "github.com/liamawhite/ng/api/golang"
	"github.com/liamawhite/ng/backend/pkg/graph"
	"github.com/liamawhite/ng/backend/pkg/server"
	"github.com/liamawhite/ng/backend/pkg/store"
)

// App holds the application state and is bound to the Wails runtime.
type App struct {
	ctx        context.Context
	httpServer *http.Server
	grpcServer *grpc.Server
	watcher    *store.Watcher
	serverURL  string
}

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Notes dir: ~/.ng/notes
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("home dir: %v", err)
	}
	dir := filepath.Join(home, ".ng", "notes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatalf("create notes dir: %v", err)
	}

	g := graph.New()
	fs := store.New(dir, g)
	if err := fs.Load(); err != nil {
		log.Fatalf("load notes: %v", err)
	}

	a.watcher, err = store.NewWatcher(fs)
	if err != nil {
		log.Fatalf("create watcher: %v", err)
	}
	if err := a.watcher.Start(); err != nil {
		log.Fatalf("start watcher: %v", err)
	}

	projectServer := server.NewProjectServer(fs)
	taskServer := server.NewTaskServer(fs)
	graphServer := server.NewGraphServer(fs)

	// Pick a random free port for the HTTP gateway.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	port := lis.Addr().(*net.TCPAddr).Port
	a.serverURL = fmt.Sprintf("http://127.0.0.1:%d", port)

	// gRPC server (in-process only, no external port needed for the desktop app).
	a.grpcServer = grpc.NewServer()
	api.RegisterProjectServiceServer(a.grpcServer, projectServer)
	api.RegisterTaskServiceServer(a.grpcServer, taskServer)
	api.RegisterGraphServiceServer(a.grpcServer, graphServer)

	// HTTP gateway (in-process, calls service implementations directly).
	gwCtx, _ := context.WithCancel(ctx) //nolint:govet
	gwMux := runtime.NewServeMux()
	if err := api.RegisterProjectServiceHandlerServer(gwCtx, gwMux, projectServer); err != nil {
		log.Fatalf("register project gateway: %v", err)
	}
	if err := api.RegisterTaskServiceHandlerServer(gwCtx, gwMux, taskServer); err != nil {
		log.Fatalf("register task gateway: %v", err)
	}
	if err := api.RegisterGraphServiceHandlerServer(gwCtx, gwMux, graphServer); err != nil {
		log.Fatalf("register graph gateway: %v", err)
	}

	a.httpServer = &http.Server{Handler: corsMiddleware(gwMux)}
	go func() {
		log.Printf("desktop HTTP gateway on %s", a.serverURL)
		if err := a.httpServer.Serve(lis); err != nil && err != http.ErrServerClosed {
			log.Printf("http serve: %v", err)
		}
	}()
}

func (a *App) shutdown(ctx context.Context) {
	if a.watcher != nil {
		a.watcher.Close()
	}
	if a.httpServer != nil {
		a.httpServer.Shutdown(ctx)
	}
}

// GetServerURL is called by the frontend transport to locate the embedded gateway.
func (a *App) GetServerURL() string { return a.serverURL }

// corsMiddleware adds permissive CORS headers for the embedded WebView.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
