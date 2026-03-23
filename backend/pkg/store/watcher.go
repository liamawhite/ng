package store

import (
	"log"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// Watcher watches the notes directory and updates the graph on file changes.
type Watcher struct {
	store   *FileStore
	watcher *fsnotify.Watcher
}

func NewWatcher(s *FileStore) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{store: s, watcher: fw}, nil
}

// Start adds the notes directory to the watcher and begins processing events.
// It returns immediately; events are processed in a background goroutine.
func (w *Watcher) Start() error {
	if err := w.watcher.Add(w.store.dir); err != nil {
		return err
	}
	go w.run()
	return nil
}

// Close stops the watcher.
func (w *Watcher) Close() error {
	return w.watcher.Close()
}

func (w *Watcher) run() {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if !strings.HasSuffix(event.Name, ".md") {
				continue
			}
			// Skip temp files written during atomic writes.
			if strings.HasSuffix(event.Name, ".md.tmp") {
				continue
			}

			switch {
			case event.Has(fsnotify.Create) || event.Has(fsnotify.Write):
				w.reload(event.Name)
			case event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename):
				// Try to reload first: an atomic rename (tmp → .md) fires a
				// Remove/Rename event on the destination even though the file is
				// immediately replaced. If the file still exists we reload it;
				// only delete the node when the file is truly gone.
				node, edges, err := ParseFile(event.Name)
				if err != nil {
					id := fileNameToID(event.Name)
					w.store.graph.DeleteNode(id)
					log.Printf("watcher: removed node %s", id)
				} else {
					w.store.graph.UpsertNode(node)
					w.store.graph.SetEdges(node.ID, edges)
					log.Printf("watcher: reloaded node %s from %s", node.ID, event.Name)
				}
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("watcher error: %v", err)
		}
	}
}

func (w *Watcher) reload(path string) {
	node, edges, err := ParseFile(path)
	if err != nil {
		log.Printf("watcher: failed to parse %s: %v", path, err)
		return
	}
	w.store.graph.UpsertNode(node)
	w.store.graph.SetEdges(node.ID, edges)
	log.Printf("watcher: reloaded node %s from %s", node.ID, path)
}

func fileNameToID(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, ".md")
}
