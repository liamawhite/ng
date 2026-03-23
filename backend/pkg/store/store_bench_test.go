package store

import (
	"fmt"
	"testing"

	"github.com/liamawhite/ng/backend/pkg/graph"
)

func BenchmarkParseFile(b *testing.B) {
	dir := b.TempDir()
	n := &graph.Node{ID: "bench-parse", Type: graph.EntityTypeProject, Title: "Bench", Status: "active"}
	edges := []graph.Edge{
		{SourceID: n.ID, Predicate: "part_of", TargetID: "parent-1"},
		{SourceID: n.ID, Predicate: "in_area", TargetID: "area-1"},
		{SourceID: n.ID, Predicate: "part_of", TargetID: "parent-2"},
		{SourceID: n.ID, Predicate: "in_area", TargetID: "area-2"},
		{SourceID: n.ID, Predicate: "part_of", TargetID: "parent-3"},
	}
	if err := WriteFile(dir, n, edges); err != nil {
		b.Fatalf("setup WriteFile: %v", err)
	}
	path := n.FilePath
	b.ResetTimer()
	for b.Loop() {
		ParseFile(path)
	}
}

func BenchmarkWriteFile(b *testing.B) {
	dir := b.TempDir()
	n := &graph.Node{ID: "bench-write", Type: graph.EntityTypeProject, Title: "Bench", Status: "active"}
	edges := []graph.Edge{
		{SourceID: n.ID, Predicate: "in_area", TargetID: "area-1"},
	}
	b.ResetTimer()
	for b.Loop() {
		WriteFile(dir, n, edges)
	}
}

func BenchmarkLoad_100(b *testing.B) {
	dir := b.TempDir()
	for i := range 100 {
		n := &graph.Node{
			ID:    fmt.Sprintf("node-%03d", i),
			Type:  graph.EntityTypeProject,
			Title: fmt.Sprintf("Node %d", i),
		}
		if err := WriteFile(dir, n, nil); err != nil {
			b.Fatalf("setup: %v", err)
		}
	}
	b.ResetTimer()
	for b.Loop() {
		g := graph.New()
		s := New(dir, g)
		s.Load()
	}
}

func BenchmarkLoad_1000(b *testing.B) {
	dir := b.TempDir()
	for i := range 1000 {
		n := &graph.Node{
			ID:    fmt.Sprintf("node-%04d", i),
			Type:  graph.EntityTypeProject,
			Title: fmt.Sprintf("Node %d", i),
		}
		if err := WriteFile(dir, n, nil); err != nil {
			b.Fatalf("setup: %v", err)
		}
	}
	b.ResetTimer()
	for b.Loop() {
		g := graph.New()
		s := New(dir, g)
		s.Load()
	}
}
