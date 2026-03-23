package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/liamawhite/ng/backend/pkg/graph"
)

// ---- helpers ----

func writeRaw(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeRaw %s: %v", name, err)
	}
	return path
}

const basicFile = `---
id: abc-123
type: area
title: Test Area
---
`

const fileWithRelationships = `---
id: proj-1
type: project
title: My Project
relationships:
    - predicate: part_of
      target: parent-1
    - predicate: in_area
      target: area-1
---
`

const fileWithContent = `---
id: task-1
type: task
title: My Task
status: todo
---

Some body content here.
`

// ---- ParseFile tests ----

func TestParseFile_Basic(t *testing.T) {
	dir := t.TempDir()
	path := writeRaw(t, dir, "abc-123.md", basicFile)

	node, edges, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if node.ID != "abc-123" || node.Title != "Test Area" || node.Type != graph.EntityTypeArea {
		t.Fatalf("ParseFile node: %+v", node)
	}
	if node.FilePath != path {
		t.Fatalf("FilePath: got %q, want %q", node.FilePath, path)
	}
	if len(edges) != 0 {
		t.Fatalf("edges: got %d, want 0", len(edges))
	}
}

func TestParseFile_WithRelationships(t *testing.T) {
	dir := t.TempDir()
	path := writeRaw(t, dir, "proj-1.md", fileWithRelationships)

	node, edges, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if node.ID != "proj-1" {
		t.Fatalf("node ID: %q", node.ID)
	}
	if len(edges) != 2 {
		t.Fatalf("edges: got %d, want 2", len(edges))
	}
	if edges[0].Predicate != "part_of" || edges[0].TargetID != "parent-1" {
		t.Fatalf("edge[0]: %+v", edges[0])
	}
	if edges[1].Predicate != "in_area" || edges[1].TargetID != "area-1" {
		t.Fatalf("edge[1]: %+v", edges[1])
	}
	for _, e := range edges {
		if e.SourceID != "proj-1" {
			t.Fatalf("edge SourceID: got %q, want proj-1", e.SourceID)
		}
	}
}

func TestParseFile_WithContent(t *testing.T) {
	dir := t.TempDir()
	path := writeRaw(t, dir, "task-1.md", fileWithContent)

	node, _, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if node.Content != "Some body content here.\n" {
		t.Fatalf("Content: %q", node.Content)
	}
	if node.Status != "todo" {
		t.Fatalf("Status: %q", node.Status)
	}
}

func TestParseFile_FrontmatterOnly(t *testing.T) {
	dir := t.TempDir()
	path := writeRaw(t, dir, "area.md", basicFile) // no body after second ---

	node, _, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile frontmatter-only: %v", err)
	}
	if node.Content != "" {
		t.Fatalf("Content: got %q, want empty", node.Content)
	}
}

func TestParseFile_MissingOpeningDelimiter(t *testing.T) {
	dir := t.TempDir()
	path := writeRaw(t, dir, "bad.md", "no frontmatter here\n")

	_, _, err := ParseFile(path)
	if err == nil {
		t.Fatal("expected error for missing opening delimiter")
	}
}

func TestParseFile_MissingClosingDelimiter(t *testing.T) {
	dir := t.TempDir()
	path := writeRaw(t, dir, "bad.md", "---\nid: x\ntype: area\ntitle: X\n")

	_, _, err := ParseFile(path)
	if err == nil {
		t.Fatal("expected error for missing closing delimiter")
	}
}

func TestParseFile_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := writeRaw(t, dir, "bad.md", "---\n: : : invalid yaml\n---\n")

	_, _, err := ParseFile(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestParseFile_NotFound(t *testing.T) {
	_, _, err := ParseFile("/does/not/exist.md")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// ---- WriteFile tests ----

func TestWriteFile_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	original := &graph.Node{
		ID:      "round-trip-id",
		Type:    graph.EntityTypeProject,
		Title:   "Round Trip",
		Content: "some content\n",
		Status:  "active",
	}
	edges := []graph.Edge{
		{SourceID: original.ID, Predicate: "in_area", TargetID: "area-xyz"},
	}

	if err := WriteFile(dir, original, edges); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, gotEdges, err := ParseFile(original.FilePath)
	if err != nil {
		t.Fatalf("ParseFile after WriteFile: %v", err)
	}
	if got.ID != original.ID || got.Title != original.Title || got.Content != original.Content || got.Status != original.Status {
		t.Fatalf("round-trip node mismatch: %+v", got)
	}
	if len(gotEdges) != 1 || gotEdges[0].Predicate != "in_area" || gotEdges[0].TargetID != "area-xyz" {
		t.Fatalf("round-trip edges mismatch: %+v", gotEdges)
	}
}

func TestWriteFile_UpdatesFilePath(t *testing.T) {
	dir := t.TempDir()
	n := &graph.Node{ID: "fp-test", Type: graph.EntityTypeArea, Title: "FP"}

	if err := WriteFile(dir, n, nil); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	want := filepath.Join(dir, "fp-test.md")
	if n.FilePath != want {
		t.Fatalf("FilePath: got %q, want %q", n.FilePath, want)
	}
}

func TestWriteFile_NoContent(t *testing.T) {
	dir := t.TempDir()
	n := &graph.Node{ID: "nc-test", Type: graph.EntityTypeArea, Title: "NC"}

	if err := WriteFile(dir, n, nil); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	data, _ := os.ReadFile(n.FilePath)
	// File should end with the closing --- and no extra newline.
	if strings.HasSuffix(string(data), "---\n\n") {
		t.Fatal("WriteFile no-content: trailing extra newline found")
	}
}

func TestWriteFile_WithEdges(t *testing.T) {
	dir := t.TempDir()
	n := &graph.Node{ID: "edge-test", Type: graph.EntityTypeProject, Title: "EP"}
	edges := []graph.Edge{
		{SourceID: n.ID, Predicate: "part_of", TargetID: "parent-99"},
	}

	if err := WriteFile(dir, n, edges); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, gotEdges, err := ParseFile(n.FilePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(gotEdges) != 1 || gotEdges[0].TargetID != "parent-99" {
		t.Fatalf("edges not serialized correctly: %+v", gotEdges)
	}
}

func TestWriteFile_Atomic(t *testing.T) {
	dir := t.TempDir()
	n := &graph.Node{ID: "atomic-test", Type: graph.EntityTypeArea, Title: "AT"}

	if err := WriteFile(dir, n, nil); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// No .tmp files should remain.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("atomic write left temp file: %s", e.Name())
		}
	}
}

func TestWriteFile_ConcurrentSameNode(t *testing.T) {
	dir := t.TempDir()
	const id = "concurrent-test"

	errc := make(chan error, 10)
	for range 10 {
		go func() {
			// Each goroutine gets its own Node so the FilePath write in
			// WriteFile doesn't race with other goroutines.
			n := &graph.Node{ID: id, Type: graph.EntityTypeArea, Title: "CT"}
			errc <- WriteFile(dir, n, nil)
		}()
	}
	for range 10 {
		if err := <-errc; err != nil {
			t.Errorf("concurrent WriteFile: %v", err)
		}
	}

	// File must be parseable after concurrent writes.
	_, _, err := ParseFile(filepath.Join(dir, id+".md"))
	if err != nil {
		t.Fatalf("ParseFile after concurrent writes: %v", err)
	}
}
