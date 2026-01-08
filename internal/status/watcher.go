package status

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/dfowler/flock/internal/task"
	"github.com/dfowler/flock/internal/tui"
	"github.com/fsnotify/fsnotify"
)

// Watcher watches the status directory for changes
type Watcher struct {
	dir     string
	updates chan tui.StatusUpdate
	done    chan struct{}
}

// NewWatcher creates a new status watcher
func NewWatcher(dir string, updates chan tui.StatusUpdate) *Watcher {
	return &Watcher{
		dir:     dir,
		updates: updates,
		done:    make(chan struct{}),
	}
}

// Start starts watching the status directory
func (w *Watcher) Start() error {
	// Ensure directory exists
	if err := os.MkdirAll(w.dir, 0755); err != nil {
		return err
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	go func() {
		defer watcher.Close()
		for {
			select {
			case <-w.done:
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
					w.handleFile(event.Name)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("watcher error: %v", err)
			}
		}
	}()

	if err := watcher.Add(w.dir); err != nil {
		return err
	}

	// Process existing files
	files, err := os.ReadDir(w.dir)
	if err == nil {
		for _, f := range files {
			if strings.HasSuffix(f.Name(), ".status") {
				w.handleFile(filepath.Join(w.dir, f.Name()))
			}
		}
	}

	return nil
}

// Stop stops the watcher
func (w *Watcher) Stop() {
	close(w.done)
}

// handleFile processes a status file change
func (w *Watcher) handleFile(path string) {
	if !strings.HasSuffix(path, ".status") {
		return
	}

	status, err := ParseStatusFile(path)
	if err != nil {
		log.Printf("failed to parse status file %s: %v", path, err)
		return
	}

	w.updates <- tui.StatusUpdate{
		TaskID: status.TaskID,
		Status: task.Status(status.Status),
	}
}
