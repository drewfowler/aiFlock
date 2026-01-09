package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dfowler/flock/internal/status"
	"github.com/dfowler/flock/internal/task"
	"github.com/dfowler/flock/internal/tui"
	"github.com/dfowler/flock/internal/zellij"
)

const statusDir = "/tmp/flock"

func main() {
	// Check if running in zellij
	if !zellij.IsInZellij() {
		fmt.Fprintln(os.Stderr, "flock must be run inside a zellij session")
		fmt.Fprintln(os.Stderr, "Start zellij first: zellij")
		os.Exit(1)
	}

	// Get project directory
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	// Initialize task store
	store, err := task.NewStore()
	if err != nil {
		log.Fatalf("failed to create store: %v", err)
	}

	// Initialize task manager
	manager := task.NewManager(store)
	if err := manager.Load(); err != nil {
		log.Printf("warning: failed to load tasks: %v", err)
	}

	// Clean up stale status files (for tasks that no longer exist)
	cleanupStaleStatusFiles(statusDir, manager)

	// Initialize zellij controller
	zjController := zellij.NewController(cwd)

	// Rename current tab to 'flock'
	if err := zjController.RenameCurrentTab("flock"); err != nil {
		log.Printf("warning: failed to rename tab: %v", err)
	}

	// Create status update channel
	statusChan := make(chan tui.StatusUpdate, 100)

	// Start status watcher
	watcher := status.NewWatcher(statusDir, statusChan)
	if err := watcher.Start(); err != nil {
		log.Fatalf("failed to start status watcher: %v", err)
	}
	defer watcher.Stop()

	// Create and run TUI
	model := tui.NewModel(manager, zjController, statusChan)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}

// cleanupStaleStatusFiles removes status files for tasks that no longer exist
func cleanupStaleStatusFiles(statusDir string, manager *task.Manager) {
	files, err := os.ReadDir(statusDir)
	if err != nil {
		return // Directory might not exist yet
	}

	for _, f := range files {
		if !f.IsDir() && filepath.Ext(f.Name()) == ".status" {
			// Extract task ID from filename (e.g., "001.status" -> "001")
			taskID := f.Name()[:len(f.Name())-7] // Remove ".status"
			if _, exists := manager.Get(taskID); !exists {
				// Task doesn't exist, remove stale status file
				statusFile := filepath.Join(statusDir, f.Name())
				os.Remove(statusFile)
			}
		}
	}
}

// getProjectDir returns the project directory (where configs are stored)
func getProjectDir() string {
	// Try to find the project root by looking for go.mod
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "."
}
