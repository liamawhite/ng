// migrate-relationships converts a ng data directory from the generic
// relationships array format to the typed field format (tasks, projects,
// subtasks, area).
//
// Usage (from repo root):
//
//	cd backend && go run ./cmd/migrate-relationships --dir ~/.ng
//	cd backend && go run ./cmd/migrate-relationships --dir ~/.ng --dry-run
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
	"time"

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

	matches, err := filepath.Glob(filepath.Join(absDir, "*.md"))
	if err != nil {
		log.Fatalf("glob: %v", err)
	}

	changed := 0
	for _, path := range matches {
		node, edges, err := store.ParseFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %s: %v\n", filepath.Base(path), err)
			continue
		}
		if *dryRun {
			fmt.Printf("would rewrite: %s (%s)\n", node.ID, node.Title)
			changed++
			continue
		}
		if err := store.WriteFile(absDir, node, edges); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", node.ID, err)
			continue
		}
		fmt.Printf("rewritten: %s (%s)\n", node.ID, node.Title)
		changed++
	}

	if *dryRun {
		fmt.Printf("\nDry run: %d files would be rewritten\n", changed)
	} else {
		fmt.Printf("\nDone: %d files rewritten\n", changed)
	}
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
