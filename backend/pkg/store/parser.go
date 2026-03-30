package store

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/liamawhite/ng/backend/pkg/graph"
	"gopkg.in/yaml.v3"
)

// frontmatter is the YAML structure stored at the top of each file.
type frontmatter struct {
	ID          string      `yaml:"id"`
	Type        string      `yaml:"type"`
	Title       string      `yaml:"title"`
	Status      string      `yaml:"status,omitempty"`
	Color       string      `yaml:"color,omitempty"`
	CompletedAt string      `yaml:"completed_at,omitempty"` // RFC3339 timestamp
	EffortValue int32       `yaml:"effort_value,omitempty"`
	EffortUnit  string      `yaml:"effort_unit,omitempty"` // "days" | "weeks" | "months"
	Priority    int32       `yaml:"priority,omitempty"`    // 1-5; 0 = unset (default 4)
	Pinned      bool        `yaml:"pinned,omitempty"`
	Links       []linkEntry `yaml:"links,omitempty"`
	// Typed relationship fields. Area is a scalar FK; Tasks/Projects/Subtasks are
	// ordered lists whose array order defines display order.
	Area     string   `yaml:"area,omitempty"`     // project → area (in_area edge)
	Tasks    []string `yaml:"tasks,omitempty"`    // project → tasks (task edges)
	Projects []string `yaml:"projects,omitempty"` // project → subprojects (subproject edges)
	Subtasks []string `yaml:"subtasks,omitempty"` // task → subtasks (subtask edges)
}

type linkEntry struct {
	URL   string `yaml:"url"`
	Title string `yaml:"title,omitempty"`
}

const separator = "---\n"

// ParseFile reads a markdown file with YAML frontmatter and returns the Node and Edges.
func ParseFile(path string) (*graph.Node, []graph.Edge, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read file: %w", err)
	}

	fm, content, err := splitFrontmatter(data)
	if err != nil {
		return nil, nil, fmt.Errorf("parse frontmatter in %s: %w", path, err)
	}

	links := make([]graph.Link, 0, len(fm.Links))
	for _, l := range fm.Links {
		links = append(links, graph.Link{URL: l.URL, Title: l.Title})
	}
	node := &graph.Node{
		ID:          fm.ID,
		Type:        graph.EntityType(fm.Type),
		Title:       fm.Title,
		Content:     content,
		Status:      fm.Status,
		Color:       fm.Color,
		FilePath:    path,
		EffortValue: fm.EffortValue,
		EffortUnit:  fm.EffortUnit,
		Links:       links,
		Priority:    fm.Priority,
		Pinned:      fm.Pinned,
	}
	if fm.CompletedAt != "" {
		t, err := time.Parse(time.RFC3339, fm.CompletedAt)
		if err == nil {
			node.CompletedAt = &t
		}
	}

	var edges []graph.Edge
	if fm.Area != "" {
		edges = append(edges, graph.Edge{SourceID: fm.ID, Predicate: "in_area", TargetID: fm.Area})
	}
	for _, id := range fm.Tasks {
		edges = append(edges, graph.Edge{SourceID: fm.ID, Predicate: "task", TargetID: id})
	}
	for _, id := range fm.Projects {
		edges = append(edges, graph.Edge{SourceID: fm.ID, Predicate: "subproject", TargetID: id})
	}
	for _, id := range fm.Subtasks {
		edges = append(edges, graph.Edge{SourceID: fm.ID, Predicate: "subtask", TargetID: id})
	}

	return node, edges, nil
}

// WriteFile serializes a Node and its edges back to YAML frontmatter + content and writes atomically.
func WriteFile(dir string, node *graph.Node, edges []graph.Edge) error {
	var completedAt string
	if node.CompletedAt != nil {
		completedAt = node.CompletedAt.UTC().Format(time.RFC3339)
	}
	fmLinks := make([]linkEntry, 0, len(node.Links))
	for _, l := range node.Links {
		fmLinks = append(fmLinks, linkEntry{URL: l.URL, Title: l.Title})
	}
	fm := frontmatter{
		ID:          node.ID,
		Type:        string(node.Type),
		Title:       node.Title,
		Status:      node.Status,
		Color:       node.Color,
		CompletedAt: completedAt,
		EffortValue: node.EffortValue,
		EffortUnit:  node.EffortUnit,
		Links:       fmLinks,
		Priority:    node.Priority,
		Pinned:      node.Pinned,
	}
	for _, e := range edges {
		switch e.Predicate {
		case "in_area":
			fm.Area = e.TargetID
		case "task":
			fm.Tasks = append(fm.Tasks, e.TargetID)
		case "subproject":
			fm.Projects = append(fm.Projects, e.TargetID)
		case "subtask":
			fm.Subtasks = append(fm.Subtasks, e.TargetID)
		}
	}

	fmBytes, err := yaml.Marshal(fm)
	if err != nil {
		return fmt.Errorf("marshal frontmatter: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteString(separator)
	buf.Write(fmBytes)
	buf.WriteString(separator)
	if node.Content != "" {
		buf.WriteString("\n")
		buf.WriteString(node.Content)
	}

	path := filepath.Join(dir, node.ID+".md")

	// Write atomically via temp file + rename. Use a unique suffix so that concurrent
	// writes to the same node don't collide on the temp path.
	tmp := filepath.Join(dir, node.ID+"."+uuid.New().String()+".tmp")
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	node.FilePath = path
	return nil
}

// splitFrontmatter splits a markdown file into frontmatter and body content.
func splitFrontmatter(data []byte) (frontmatter, string, error) {
	s := string(data)
	if !strings.HasPrefix(s, separator) {
		return frontmatter{}, "", fmt.Errorf("missing opening ---")
	}
	rest := s[len(separator):]
	fmStr, after, ok := strings.Cut(rest, separator)
	if !ok {
		return frontmatter{}, "", fmt.Errorf("missing closing ---")
	}
	body := strings.TrimPrefix(after, "\n")

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(fmStr), &fm); err != nil {
		return frontmatter{}, "", err
	}
	return fm, body, nil
}
