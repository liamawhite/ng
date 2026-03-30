// migrate-parent-child converts a ng data directory from the child-to-parent
// relationship model (part_of edges on children) to the parent-to-child model
// (task/subtask/subproject edges on parents).
//
// Usage (from repo root):
//
//	cd backend && go run ./cmd/migrate-parent-child --dir ~/.ng
//	cd backend && go run ./cmd/migrate-parent-child --dir ~/.ng --dry-run
//
// A timestamped backup of the data directory is created before any writes.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/liamawhite/ng/backend/pkg/graph"
	"github.com/liamawhite/ng/backend/pkg/store"
)

func main() {
	dir := flag.String("dir", "", "ng data directory (required)")
	dryRun := flag.Bool("dry-run", false, "print changes without writing any files")
	flag.Parse()

	if *dir == "" {
		fmt.Fprintln(os.Stderr, "error: --dir is required")
		flag.Usage()
		os.Exit(1)
	}

	absDir, err := filepath.Abs(*dir)
	if err != nil {
		log.Fatalf("resolve dir: %v", err)
	}

	if !*dryRun {
		backupDir := absDir + "-backup-" + time.Now().Format("20060102-150405")
		if err := copyDir(absDir, backupDir); err != nil {
			log.Fatalf("backup failed: %v", err)
		}
		fmt.Printf("backup: %s\n", backupDir)
	}

	// Parse all files.
	matches, err := filepath.Glob(filepath.Join(absDir, "*.md"))
	if err != nil {
		log.Fatalf("glob: %v", err)
	}

	type entry struct {
		node  *graph.Node
		edges []graph.Edge
	}
	entries := make(map[string]*entry)
	for _, path := range matches {
		node, edges, err := store.ParseFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %s: %v\n", filepath.Base(path), err)
			continue
		}
		entries[node.ID] = &entry{node: node, edges: edges}
	}

	// Build new edge sets: start with all non-part_of edges for each node.
	newEdges := make(map[string][]graph.Edge, len(entries))
	for id, e := range entries {
		var keep []graph.Edge
		for _, edge := range e.edges {
			if edge.Predicate != "part_of" {
				keep = append(keep, edge)
			}
		}
		newEdges[id] = keep
	}

	// Group children by parent, sorted alphabetically by title.
	type childEntry struct {
		title     string
		childID   string
		predicate string
	}
	parentChildren := make(map[string][]childEntry)

	for _, e := range entries {
		for _, edge := range e.edges {
			if edge.Predicate != "part_of" {
				continue
			}
			child := e.node
			parentEntry, ok := entries[edge.TargetID]
			if !ok {
				fmt.Fprintf(os.Stderr, "skip orphan: %s (%q) part_of %s (not found)\n",
					child.ID, child.Title, edge.TargetID)
				continue
			}
			parent := parentEntry.node

			var predicate string
			switch {
			case child.Type == graph.EntityTypeTask && parent.Type == graph.EntityTypeProject:
				predicate = "task"
			case child.Type == graph.EntityTypeTask && parent.Type == graph.EntityTypeTask:
				predicate = "subtask"
			case child.Type == graph.EntityTypeProject && parent.Type == graph.EntityTypeProject:
				predicate = "subproject"
			default:
				fmt.Fprintf(os.Stderr, "skip unknown edge: %s(%s) → %s(%s)\n",
					child.ID, child.Type, parent.ID, parent.Type)
				continue
			}

			parentChildren[edge.TargetID] = append(parentChildren[edge.TargetID], childEntry{
				title:     child.Title,
				childID:   child.ID,
				predicate: predicate,
			})
		}
	}

	// Sort children alphabetically by title within each parent, then append.
	for parentID, children := range parentChildren {
		sort.Slice(children, func(i, j int) bool {
			return children[i].title < children[j].title
		})
		for _, c := range children {
			newEdges[parentID] = append(newEdges[parentID], graph.Edge{
				SourceID:  parentID,
				Predicate: c.predicate,
				TargetID:  c.childID,
			})
		}
	}

	// Write changed files.
	changed, skipped := 0, 0
	for id, edges := range newEdges {
		e := entries[id]
		if edgesEqual(e.edges, edges) {
			skipped++
			continue
		}
		if *dryRun {
			fmt.Printf("would update: %s (%s)\n", id, e.node.Title)
			changed++
			continue
		}
		if err := store.WriteFile(absDir, e.node, edges); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", id, err)
			continue
		}
		fmt.Printf("updated: %s (%s)\n", id, e.node.Title)
		changed++
	}

	if *dryRun {
		fmt.Printf("\nDry run: %d files would be updated, %d unchanged\n", changed, skipped)
	} else {
		fmt.Printf("\nDone: %d files updated, %d unchanged\n", changed, skipped)
	}
}

// edgesEqual reports whether two edge slices have identical content.
func edgesEqual(a, b []graph.Edge) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// copyDir recursively copies src to dst.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
