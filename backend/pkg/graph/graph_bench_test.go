package graph

import (
	"fmt"
	"testing"
)

func BenchmarkGetNode(b *testing.B) {
	g := New()
	g.UpsertNode(node("target", EntityTypeProject))
	b.ResetTimer()
	for b.Loop() {
		g.GetNode("target")
	}
}

func BenchmarkListNodes(b *testing.B) {
	g := New()
	for i := range 1000 {
		g.UpsertNode(node(fmt.Sprintf("proj-%04d", i), EntityTypeProject))
	}
	b.ResetTimer()
	for b.Loop() {
		g.ListNodes(EntityTypeProject)
	}
}

func BenchmarkListRelated(b *testing.B) {
	g := New()
	g.UpsertNode(node("hub", EntityTypeProject))
	for i := range 100 {
		id := fmt.Sprintf("task-%03d", i)
		g.UpsertNode(node(id, EntityTypeTask))
		g.SetEdges(id, []Edge{edge(id, "part_of", "hub")})
	}
	b.ResetTimer()
	for b.Loop() {
		g.ListRelated("hub", "", "incoming")
	}
}

func BenchmarkSetEdges(b *testing.B) {
	g := New()
	g.UpsertNode(node("src", EntityTypeProject))
	edges := make([]Edge, 50)
	for i := range 50 {
		id := fmt.Sprintf("area-%02d", i)
		g.UpsertNode(node(id, EntityTypeArea))
		edges[i] = edge("src", "in_area", id)
	}
	b.ResetTimer()
	for b.Loop() {
		g.SetEdges("src", edges)
	}
}
