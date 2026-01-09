package status

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dfowler/flock/internal/task"
	"github.com/dfowler/flock/internal/tui"
	"github.com/fsnotify/fsnotify"
)

// Watcher watches the status directory for changes
type Watcher struct {
	dir           string
	updates       chan tui.StatusUpdate
	done          chan struct{}
	lastStatus    map[string]string // tracks last known status per task
	initializing  bool              // true during initial file load (skip notifications)
}

// NewWatcher creates a new status watcher
func NewWatcher(dir string, updates chan tui.StatusUpdate) *Watcher {
	return &Watcher{
		dir:        dir,
		updates:    updates,
		done:       make(chan struct{}),
		lastStatus: make(map[string]string),
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

	// Process existing files (but don't send notifications for stale data)
	w.initializing = true
	files, err := os.ReadDir(w.dir)
	if err == nil {
		for _, f := range files {
			if strings.HasSuffix(f.Name(), ".status") {
				w.handleFile(filepath.Join(w.dir, f.Name()))
			}
		}
	}
	w.initializing = false

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
		// Silently skip invalid status files (e.g., from non-flock Claude Code sessions)
		// These are expected when Claude Code runs outside of flock context
		return
	}

	// Check if status changed and send notification (skip during initial load)
	lastStatus, exists := w.lastStatus[status.TaskID]
	if !exists || lastStatus != status.Status {
		w.lastStatus[status.TaskID] = status.Status
		// Only send notifications for real-time changes, not initial file load
		if !w.initializing {
			w.sendNotification(status.TaskID, status.TaskName, status.Status)
		}
	}

	w.updates <- tui.StatusUpdate{
		TaskID: status.TaskID,
		Status: task.Status(status.Status),
	}
}

// sendNotification sends a desktop notification for status changes
func (w *Watcher) sendNotification(taskID, taskName, status string) {
	var title, body, urgency string

	// Use task name if available, otherwise fall back to task ID
	displayName := taskName
	if displayName == "" {
		displayName = fmt.Sprintf("Task %s", taskID)
	}

	switch status {
	case "WAITING":
		title = "Flock: Agent Needs Attention"
		body = fmt.Sprintf("%s is waiting for input", displayName)
		urgency = "critical"
	case "WORKING":
		title = "Flock: Agent Working"
		body = fmt.Sprintf("%s is now working", displayName)
		urgency = "low"
	case "DONE":
		title = "Flock: Agent Complete"
		body = fmt.Sprintf("%s has finished", displayName)
		urgency = "normal"
	default:
		return
	}

	// Use notify-send for desktop notifications
	cmd := exec.Command("notify-send", "-u", urgency, title, body)
	if err := cmd.Run(); err != nil {
		log.Printf("failed to send notification: %v", err)
	}
}
