package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/liamawhite/ng/api/golang"
	"github.com/liamawhite/ng/backend/pkg/graph"
	"github.com/liamawhite/ng/backend/pkg/server"
	"github.com/liamawhite/ng/backend/pkg/store"
)

func main() {
	dir := flag.String("dir", "./notes", "directory containing note files")
	port := flag.Int("port", 50051, "gRPC server port")
	httpPort := flag.Int("http-port", 8080, "HTTP gateway port")
	flag.Parse()

	if err := os.MkdirAll(*dir, 0o755); err != nil {
		log.Fatalf("create notes dir: %v", err)
	}

	g := graph.New()
	fs := store.New(*dir, g)

	if err := fs.Load(); err != nil {
		log.Fatalf("load notes: %v", err)
	}

	watcher, err := store.NewWatcher(fs)
	if err != nil {
		log.Fatalf("create watcher: %v", err)
	}
	if err := watcher.Start(); err != nil {
		log.Fatalf("start watcher: %v", err)
	}
	defer watcher.Close()

	projectServer := server.NewProjectServer(fs)
	taskServer := server.NewTaskServer(fs)
	graphServer := server.NewGraphServer(fs)

	// --- gRPC server ---
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	api.RegisterProjectServiceServer(grpcServer, projectServer)
	api.RegisterTaskServiceServer(grpcServer, taskServer)
	api.RegisterGraphServiceServer(grpcServer, graphServer)
	reflection.Register(grpcServer)

	go func() {
		log.Printf("gRPC server listening on :%d", *port)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("grpc serve: %v", err)
		}
	}()

	// --- HTTP gateway (in-process, no network hop) ---
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gwMux := runtime.NewServeMux()
	if err := api.RegisterProjectServiceHandlerServer(ctx, gwMux, projectServer); err != nil {
		log.Fatalf("register project gateway: %v", err)
	}
	if err := api.RegisterTaskServiceHandlerServer(ctx, gwMux, taskServer); err != nil {
		log.Fatalf("register task gateway: %v", err)
	}
	if err := api.RegisterGraphServiceHandlerServer(ctx, gwMux, graphServer); err != nil {
		log.Fatalf("register graph gateway: %v", err)
	}

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", *httpPort),
		Handler: corsMiddleware(gwMux),
	}

	go func() {
		log.Printf("HTTP gateway listening on :%d", *httpPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http serve: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down...")
	httpServer.Shutdown(context.Background())
	grpcServer.GracefulStop()
}

// corsMiddleware adds CORS headers so browser frontends can call the gateway.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
