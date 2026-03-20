package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	"github.com/liamawhite/ng/api/golang"
	"github.com/liamawhite/ng/backend/pkg/graph"
	"github.com/liamawhite/ng/backend/pkg/server"
	"github.com/liamawhite/ng/backend/pkg/store"
)

func main() {
	dir := flag.String("dir", "./notes", "directory containing note files")
	port := flag.Int("port", 50051, "gRPC server port")
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

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	api.RegisterProjectServiceServer(grpcServer, server.NewProjectServer(fs))
	api.RegisterTaskServiceServer(grpcServer, server.NewTaskServer(fs))
	api.RegisterGraphServiceServer(grpcServer, server.NewGraphServer(fs))

	go func() {
		log.Printf("gRPC server listening on :%d", *port)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("serve: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down...")
	grpcServer.GracefulStop()
}
