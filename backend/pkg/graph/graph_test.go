package graph

import (
	"testing"
)

// ---- helpers ----

func node(id string, t EntityType) *Node {
	return &Node{ID: id, Type: t, Title: id}
}

func edge(src, pred, tgt string) Edge {
	return Edge{SourceID: src, Predicate: pred, TargetID: tgt}
}

// ---- tests ----

func TestNew(t *testing.T) {
	g := New()
	if len(g.nodes) != 0 || len(g.outgoing) != 0 || len(g.incoming) != 0 {
		t.Fatal("New: expected empty graph")
	}
}

func TestUpsertNode(t *testing.T) {
	g := New()
	g.UpsertNode(node("a", EntityTypeArea))

	n, ok := g.GetNode("a")
	if !ok || n.ID != "a" {
		t.Fatalf("GetNode: expected node a, got ok=%v", ok)
	}
}

func TestUpsertNode_Overwrite(t *testing.T) {
	g := New()
	g.UpsertNode(&Node{ID: "a", Type: EntityTypeArea, Title: "old"})
	g.UpsertNode(&Node{ID: "a", Type: EntityTypeArea, Title: "new"})

	n, _ := g.GetNode("a")
	if n.Title != "new" {
		t.Fatalf("UpsertNode overwrite: title=%q, want new", n.Title)
	}
}

func TestGetNode_NotFound(t *testing.T) {
	g := New()
	_, ok := g.GetNode("missing")
	if ok {
		t.Fatal("GetNode: expected not found for missing id")
	}
}

func TestDeleteNode_CleansOutgoing(t *testing.T) {
	g := New()
	g.UpsertNode(node("a", EntityTypeProject))
	g.UpsertNode(node("b", EntityTypeTask))
	g.SetEdges("a", []Edge{edge("a", "part_of", "b")})

	g.DeleteNode("a")

	if _, ok := g.outgoing["a"]; ok {
		t.Fatal("DeleteNode: outgoing entry for deleted node still exists")
	}
}

func TestDeleteNode_CleansIncoming(t *testing.T) {
	g := New()
	g.UpsertNode(node("a", EntityTypeProject))
	g.UpsertNode(node("b", EntityTypeTask))
	g.SetEdges("a", []Edge{edge("a", "part_of", "b")})

	g.DeleteNode("a")

	if edges, ok := g.incoming["b"]; ok && len(edges) > 0 {
		t.Fatalf("DeleteNode: incoming entry for target b still has %d edges", len(edges))
	}
}

func TestDeleteNode_RemovesTargetIncoming(t *testing.T) {
	// When the TARGET node is deleted, edges from other nodes pointing to it
	// should be removed from the outgoing index of those source nodes.
	g := New()
	g.UpsertNode(node("src", EntityTypeProject))
	g.UpsertNode(node("tgt", EntityTypeArea))
	g.SetEdges("src", []Edge{edge("src", "in_area", "tgt")})

	g.DeleteNode("tgt")

	if edges := g.outgoing["src"]; len(edges) > 0 {
		t.Fatalf("DeleteNode target: outgoing from src still has %d edges", len(edges))
	}
}

func TestSetEdges_ReplacesOld(t *testing.T) {
	g := New()
	g.UpsertNode(node("a", EntityTypeProject))
	g.UpsertNode(node("b", EntityTypeArea))
	g.UpsertNode(node("c", EntityTypeArea))

	g.SetEdges("a", []Edge{edge("a", "in_area", "b")})
	// Replace with edge to c; b's incoming entry should be removed.
	g.SetEdges("a", []Edge{edge("a", "in_area", "c")})

	if edges, ok := g.incoming["b"]; ok && len(edges) > 0 {
		t.Fatalf("SetEdges replace: old incoming for b still has %d edges", len(edges))
	}
	if len(g.incoming["c"]) != 1 {
		t.Fatalf("SetEdges replace: new incoming for c has %d edges, want 1", len(g.incoming["c"]))
	}
}

func TestSetEdges_NilEdges(t *testing.T) {
	g := New()
	g.UpsertNode(node("a", EntityTypeProject))
	g.UpsertNode(node("b", EntityTypeArea))
	g.SetEdges("a", []Edge{edge("a", "in_area", "b")})

	g.SetEdges("a", nil)

	if len(g.outgoing["a"]) != 0 {
		t.Fatal("SetEdges nil: outgoing not cleared")
	}
	if len(g.incoming["b"]) != 0 {
		t.Fatal("SetEdges nil: incoming not cleared")
	}
}

func TestListNodes_FiltersByType(t *testing.T) {
	g := New()
	g.UpsertNode(node("p1", EntityTypeProject))
	g.UpsertNode(node("p2", EntityTypeProject))
	g.UpsertNode(node("t1", EntityTypeTask))

	projects := g.ListNodes(EntityTypeProject)
	if len(projects) != 2 {
		t.Fatalf("ListNodes(project): got %d, want 2", len(projects))
	}
	tasks := g.ListNodes(EntityTypeTask)
	if len(tasks) != 1 {
		t.Fatalf("ListNodes(task): got %d, want 1", len(tasks))
	}
}

func TestListNodes_SortedByID(t *testing.T) {
	g := New()
	g.UpsertNode(node("z", EntityTypeProject))
	g.UpsertNode(node("a", EntityTypeProject))
	g.UpsertNode(node("m", EntityTypeProject))

	nodes := g.ListNodes(EntityTypeProject)
	ids := []string{nodes[0].ID, nodes[1].ID, nodes[2].ID}
	if ids[0] != "a" || ids[1] != "m" || ids[2] != "z" {
		t.Fatalf("ListNodes: not sorted, got %v", ids)
	}
}

func TestListRelated_OutgoingDir(t *testing.T) {
	g := New()
	g.UpsertNode(node("proj", EntityTypeProject))
	g.UpsertNode(node("area", EntityTypeArea))
	g.SetEdges("proj", []Edge{edge("proj", "in_area", "area")})

	related := g.ListRelated("proj", "", "outgoing")
	if len(related) != 1 || related[0].Direction != "outgoing" || related[0].Node.ID != "area" {
		t.Fatalf("ListRelated outgoing: got %v", related)
	}
}

func TestListRelated_IncomingDir(t *testing.T) {
	g := New()
	g.UpsertNode(node("proj", EntityTypeProject))
	g.UpsertNode(node("task", EntityTypeTask))
	g.SetEdges("task", []Edge{edge("task", "part_of", "proj")})

	related := g.ListRelated("proj", "", "incoming")
	if len(related) != 1 || related[0].Direction != "incoming" || related[0].Node.ID != "task" {
		t.Fatalf("ListRelated incoming: got %v", related)
	}
}

func TestListRelated_BothDirs(t *testing.T) {
	g := New()
	g.UpsertNode(node("proj", EntityTypeProject))
	g.UpsertNode(node("area", EntityTypeArea))
	g.UpsertNode(node("task", EntityTypeTask))
	g.SetEdges("proj", []Edge{edge("proj", "in_area", "area")})
	g.SetEdges("task", []Edge{edge("task", "part_of", "proj")})

	related := g.ListRelated("proj", "", "")
	if len(related) != 2 {
		t.Fatalf("ListRelated both: got %d entries, want 2", len(related))
	}
}

func TestListRelated_PredicateFilter(t *testing.T) {
	g := New()
	g.UpsertNode(node("proj", EntityTypeProject))
	g.UpsertNode(node("area", EntityTypeArea))
	g.UpsertNode(node("parent", EntityTypeProject))
	g.SetEdges("proj", []Edge{
		edge("proj", "in_area", "area"),
		edge("proj", "part_of", "parent"),
	})

	related := g.ListRelated("proj", "in_area", "")
	if len(related) != 1 || related[0].Node.ID != "area" {
		t.Fatalf("ListRelated predicate filter: got %v", related)
	}
}

func TestListRelated_SkipsOrphanedTargets(t *testing.T) {
	g := New()
	g.UpsertNode(node("proj", EntityTypeProject))
	g.UpsertNode(node("area", EntityTypeArea))
	g.SetEdges("proj", []Edge{edge("proj", "in_area", "area")})

	// Delete the target node — edge still exists in outgoing index but target is gone.
	g.DeleteNode("area")

	related := g.ListRelated("proj", "", "")
	if len(related) != 0 {
		t.Fatalf("ListRelated: expected 0 results for orphaned edge, got %d", len(related))
	}
}
