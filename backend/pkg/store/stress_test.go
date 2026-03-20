package store_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/liamawhite/ng/backend/pkg/graph"
	"github.com/liamawhite/ng/backend/pkg/store"
)

func newTestStore(t *testing.T) *store.FileStore {
	t.Helper()
	return store.New(t.TempDir(), graph.New())
}

// TestStressConcurrentCreate spawns N goroutines each creating a distinct node and verifies
// all N nodes end up in the graph.
func TestStressConcurrentCreate(t *testing.T) {
	const workers = 50
	s := newTestStore(t)

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := range workers {
		go func(i int) {
			defer wg.Done()
			node := &graph.Node{Type: graph.EntityTypeProject, Title: fmt.Sprintf("node-%d", i)}
			if err := s.Create(node, nil); err != nil {
				t.Errorf("worker %d: Create: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	if got := len(s.Graph().ListNodes(graph.EntityTypeProject)); got != workers {
		t.Errorf("expected %d nodes, got %d", workers, got)
	}
}

// TestStressConcurrentUpdateSameNode hammers concurrent updates on the same node, expecting no
// panics and the node to remain present throughout.
func TestStressConcurrentUpdateSameNode(t *testing.T) {
	const workers = 50
	s := newTestStore(t)

	node := &graph.Node{Type: graph.EntityTypeProject, Title: "original"}
	if err := s.Create(node, nil); err != nil {
		t.Fatalf("Create: %v", err)
	}
	id := node.ID

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := range workers {
		go func(i int) {
			defer wg.Done()
			n := &graph.Node{ID: id, Type: graph.EntityTypeProject, Title: fmt.Sprintf("title-%d", i)}
			if err := s.Update(n, nil); err != nil {
				t.Errorf("worker %d: Update: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	if _, ok := s.Graph().GetNode(id); !ok {
		t.Error("node missing from graph after concurrent updates")
	}
}

// TestStressConcurrentDelete creates N nodes then deletes them all concurrently, expecting an
// empty graph at the end.
func TestStressConcurrentDelete(t *testing.T) {
	const workers = 50
	s := newTestStore(t)

	ids := make([]string, workers)
	for i := range workers {
		node := &graph.Node{Type: graph.EntityTypeProject, Title: fmt.Sprintf("node-%d", i)}
		if err := s.Create(node, nil); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
		ids[i] = node.ID
	}

	var wg sync.WaitGroup
	wg.Add(workers)
	for _, id := range ids {
		go func(id string) {
			defer wg.Done()
			if err := s.Delete(id); err != nil {
				t.Errorf("Delete %s: %v", id, err)
			}
		}(id)
	}
	wg.Wait()

	if got := len(s.Graph().ListNodes(graph.EntityTypeProject)); got != 0 {
		t.Errorf("expected 0 nodes after deletes, got %d", got)
	}
}

// TestStressMixedOps runs creates, updates, and deletes concurrently on distinct node sets.
// The new project nodes from Create goroutines must all be present at the end.
func TestStressMixedOps(t *testing.T) {
	const workers = 30
	s := newTestStore(t)

	// Pre-create task nodes to update/delete.
	preIDs := make([]string, workers)
	for i := range workers {
		node := &graph.Node{Type: graph.EntityTypeTask, Title: fmt.Sprintf("pre-%d", i)}
		if err := s.Create(node, nil); err != nil {
			t.Fatalf("pre-create %d: %v", i, err)
		}
		preIDs[i] = node.ID
	}

	var wg sync.WaitGroup
	wg.Add(3 * workers)

	// Create new project nodes.
	for i := range workers {
		go func(i int) {
			defer wg.Done()
			node := &graph.Node{Type: graph.EntityTypeProject, Title: fmt.Sprintf("new-%d", i)}
			if err := s.Create(node, nil); err != nil {
				t.Errorf("Create %d: %v", i, err)
			}
		}(i)
	}

	// Update pre-created task nodes.
	for i, id := range preIDs {
		go func(i int, id string) {
			defer wg.Done()
			n := &graph.Node{ID: id, Type: graph.EntityTypeTask, Title: fmt.Sprintf("updated-%d", i)}
			if err := s.Update(n, nil); err != nil {
				t.Errorf("Update %d: %v", i, err)
			}
		}(i, id)
	}

	// Delete pre-created task nodes.
	for _, id := range preIDs {
		go func(id string) {
			defer wg.Done()
			if err := s.Delete(id); err != nil {
				t.Errorf("Delete %s: %v", id, err)
			}
		}(id)
	}

	wg.Wait()

	// New project nodes must all be present regardless of task churn.
	if got := len(s.Graph().ListNodes(graph.EntityTypeProject)); got != workers {
		t.Errorf("expected %d project nodes, got %d", workers, got)
	}
}

// TestStressConcurrentCreateWithEdges creates many task nodes that all point to a single project
// concurrently and verifies the incoming edge count on the project.
func TestStressConcurrentCreateWithEdges(t *testing.T) {
	const workers = 30
	s := newTestStore(t)

	target := &graph.Node{Type: graph.EntityTypeProject, Title: "parent"}
	if err := s.Create(target, nil); err != nil {
		t.Fatalf("Create target: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := range workers {
		go func(i int) {
			defer wg.Done()
			node := &graph.Node{Type: graph.EntityTypeTask, Title: fmt.Sprintf("task-%d", i)}
			edges := []graph.Edge{{Predicate: "part_of", TargetID: target.ID}}
			if err := s.Create(node, edges); err != nil {
				t.Errorf("Create task %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	if got := len(s.Graph().ListNodes(graph.EntityTypeTask)); got != workers {
		t.Errorf("expected %d tasks, got %d", workers, got)
	}
	related := s.Graph().ListRelated(target.ID, "part_of", "incoming")
	if len(related) != workers {
		t.Errorf("expected %d incoming edges on target, got %d", workers, len(related))
	}
}

// TestStressLoad writes N files directly via WriteFile then verifies Load populates the graph.
func TestStressLoad(t *testing.T) {
	const count = 200
	dir := t.TempDir()

	for i := range count {
		node := &graph.Node{
			ID:    fmt.Sprintf("node-%05d", i),
			Type:  graph.EntityTypeProject,
			Title: fmt.Sprintf("Project %d", i),
		}
		if err := store.WriteFile(dir, node, nil); err != nil {
			t.Fatalf("WriteFile %d: %v", i, err)
		}
	}

	g := graph.New()
	s := store.New(dir, g)
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := len(g.ListNodes(graph.EntityTypeProject)); got != count {
		t.Errorf("expected %d nodes after load, got %d", count, got)
	}
}

// TestStressConcurrentReadsDuringWrites reads from the graph while writes are in flight,
// primarily exercising the absence of data races.
func TestStressConcurrentReadsDuringWrites(t *testing.T) {
	const writers = 20
	const readers = 20
	s := newTestStore(t)

	var wg sync.WaitGroup
	wg.Add(writers + readers)

	for i := range writers {
		go func(i int) {
			defer wg.Done()
			node := &graph.Node{Type: graph.EntityTypeProject, Title: fmt.Sprintf("node-%d", i)}
			if err := s.Create(node, nil); err != nil {
				t.Errorf("Create %d: %v", i, err)
			}
		}(i)
	}

	for range readers {
		go func() {
			defer wg.Done()
			_ = s.Graph().ListNodes(graph.EntityTypeProject)
		}()
	}

	wg.Wait()
}
