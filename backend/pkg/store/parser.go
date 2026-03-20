package store

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/liamawhite/ng/backend/pkg/graph"
	"gopkg.in/yaml.v3"
)

// frontmatter is the YAML structure stored at the top of each file.
type frontmatter struct {
	ID            string         `yaml:"id"`
	Type          string         `yaml:"type"`
	Title         string         `yaml:"title"`
	Status        string         `yaml:"status,omitempty"`
	Relationships []relationship `yaml:"relationships,omitempty"`
}

type relationship struct {
	Predicate string `yaml:"predicate"`
	Target    string `yaml:"target"`
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

	node := &graph.Node{
		ID:       fm.ID,
		Type:     graph.EntityType(fm.Type),
		Title:    fm.Title,
		Content:  content,
		Status:   fm.Status,
		FilePath: path,
	}

	edges := make([]graph.Edge, 0, len(fm.Relationships))
	for _, r := range fm.Relationships {
		edges = append(edges, graph.Edge{
			SourceID:  fm.ID,
			Predicate: r.Predicate,
			TargetID:  r.Target,
		})
	}

	return node, edges, nil
}

// WriteFile serializes a Node and its edges back to YAML frontmatter + content and writes atomically.
func WriteFile(dir string, node *graph.Node, edges []graph.Edge) error {
	rels := make([]relationship, 0, len(edges))
	for _, e := range edges {
		rels = append(rels, relationship{
			Predicate: e.Predicate,
			Target:    e.TargetID,
		})
	}

	fm := frontmatter{
		ID:            node.ID,
		Type:          string(node.Type),
		Title:         node.Title,
		Status:        node.Status,
		Relationships: rels,
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
