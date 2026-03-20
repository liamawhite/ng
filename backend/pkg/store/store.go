package store

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/liamawhite/ng/backend/pkg/graph"
)

// FileStore manages the on-disk markdown files and keeps the in-memory graph in sync.
type FileStore struct {
	dir   string
	graph *graph.Graph
}

func New(dir string, g *graph.Graph) *FileStore {
	return &FileStore{dir: dir, graph: g}
}

// Load scans all *.md files in the notes directory and populates the graph.
func (s *FileStore) Load() error {
	matches, err := filepath.Glob(filepath.Join(s.dir, "*.md"))
	if err != nil {
		return fmt.Errorf("glob notes dir: %w", err)
	}
	for _, path := range matches {
		node, edges, err := ParseFile(path)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		s.graph.UpsertNode(node)
		s.graph.SetEdges(node.ID, edges)
	}
	return nil
}

// Create generates a UUID (if empty), writes the file, and upserts into the graph.
func (s *FileStore) Create(node *graph.Node, edges []graph.Edge) error {
	if node.ID == "" {
		node.ID = uuid.New().String()
	}
	// Ensure all edges carry the correct source ID.
	for i := range edges {
		edges[i].SourceID = node.ID
	}
	if err := WriteFile(s.dir, node, edges); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	s.graph.UpsertNode(node)
	s.graph.SetEdges(node.ID, edges)
	return nil
}

// Update writes the file and upserts into the graph.
func (s *FileStore) Update(node *graph.Node, edges []graph.Edge) error {
	if node.ID == "" {
		return fmt.Errorf("node ID is required for update")
	}
	if err := WriteFile(s.dir, node, edges); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	s.graph.UpsertNode(node)
	s.graph.SetEdges(node.ID, edges)
	return nil
}

// Delete removes the file and deletes the node from the graph.
func (s *FileStore) Delete(id string) error {
	path := filepath.Join(s.dir, id+".md")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove file: %w", err)
	}
	s.graph.DeleteNode(id)
	return nil
}

// Graph returns the underlying graph (for use by the server).
func (s *FileStore) Graph() *graph.Graph {
	return s.graph
}
