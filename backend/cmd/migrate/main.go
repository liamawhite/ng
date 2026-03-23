// migrate reads project notes from ~/notes/projects and writes them to ~/.ng/
// in the ng store format (UUID-named YAML-frontmatter markdown files).
//
// Usage (from repo root):
//
//	cd backend && go run ./cmd/migrate
package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/liamawhite/ng/backend/pkg/graph"
	"github.com/liamawhite/ng/backend/pkg/store"
)

var (
	todoRe    = regexp.MustCompile(`^\s*- \[ \] (.+)$`)
	doneRe    = regexp.MustCompile(`^\s*- \[x\] (.+)$`)
	dateSufRe = regexp.MustCompile(`\s*✅\s*\d{4}-\d{2}-\d{2}\s*$`)
)

type srcTask struct {
	title string
	done  bool
}

type srcProject struct {
	title   string
	content string
	tasks   []srcTask
}

func parse(path string) (srcProject, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return srcProject{}, err
	}
	s := string(data)

	// Strip frontmatter
	if !strings.HasPrefix(s, "---\n") {
		return srcProject{}, fmt.Errorf("no frontmatter")
	}
	rest := s[4:]
	end := strings.Index(rest, "\n---\n")
	if end == -1 {
		return srcProject{}, fmt.Errorf("unclosed frontmatter")
	}
	body := rest[end+5:]

	// Extract H1 title
	var title string
	for _, line := range strings.SplitN(body, "\n", 30) {
		if strings.HasPrefix(line, "# ") {
			title = strings.TrimPrefix(line, "# ")
			break
		}
	}
	if title == "" {
		base := strings.TrimSuffix(filepath.Base(path), ".md")
		title = strings.ReplaceAll(base, "-", " ")
	}

	// Split body into sections keyed by ## heading (lowercased)
	sections := make(map[string]string)
	var curSec string
	var curBuf strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "## ") {
			if curSec != "" {
				sections[curSec] = curBuf.String()
			}
			curSec = strings.ToLower(strings.TrimPrefix(line, "## "))
			curBuf.Reset()
		} else {
			curBuf.WriteString(line)
			curBuf.WriteByte('\n')
		}
	}
	if curSec != "" {
		sections[curSec] = curBuf.String()
	}

	// Parse all checkbox lines in the tasks section (including indented subtasks)
	var tasks []srcTask
	for _, line := range strings.Split(sections["tasks"], "\n") {
		if m := todoRe.FindStringSubmatch(line); m != nil {
			tasks = append(tasks, srcTask{title: strings.TrimSpace(m[1])})
		} else if m := doneRe.FindStringSubmatch(line); m != nil {
			t := strings.TrimSpace(dateSufRe.ReplaceAllString(strings.TrimSpace(m[1]), ""))
			tasks = append(tasks, srcTask{title: t, done: true})
		}
	}

	return srcProject{
		title:   strings.TrimSpace(title),
		content: strings.TrimSpace(sections["notes"]),
		tasks:   tasks,
	}, nil
}

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("home dir: %v", err)
	}
	srcDir := filepath.Join(home, "notes", "projects")
	dstDir := filepath.Join(home, ".ng")

	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", dstDir, err)
	}

	var nProjects, nTasks int
	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}

		proj, parseErr := parse(path)
		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "  skip %s: %v\n", filepath.Base(path), parseErr)
			return nil
		}

		projID := uuid.New().String()
		projNode := &graph.Node{
			ID:      projID,
			Type:    graph.EntityTypeProject,
			Title:   proj.title,
			Content: proj.content,
		}
		if err := store.WriteFile(dstDir, projNode, nil); err != nil {
			return fmt.Errorf("write project %q: %w", proj.title, err)
		}
		nProjects++
		fmt.Printf("  [project] %s  (%d tasks)\n", proj.title, len(proj.tasks))

		for _, t := range proj.tasks {
			status := "todo"
			if t.done {
				status = "done"
			}
			taskNode := &graph.Node{
				ID:     uuid.New().String(),
				Type:   graph.EntityTypeTask,
				Title:  t.title,
				Status: status,
			}
			edge := graph.Edge{SourceID: taskNode.ID, Predicate: "part_of", TargetID: projID}
			if err := store.WriteFile(dstDir, taskNode, []graph.Edge{edge}); err != nil {
				return fmt.Errorf("write task %q: %w", t.title, err)
			}
			nTasks++
		}
		return nil
	})
	if err != nil {
		log.Fatalf("walk: %v", err)
	}

	fmt.Printf("\nDone: %d projects, %d tasks → %s\n", nProjects, nTasks, dstDir)
}
