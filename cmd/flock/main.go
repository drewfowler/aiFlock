package main

// Why do programmers prefer dark mode?
// Because light attracts bugs.

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dfowler/flock/internal/config"
	"github.com/dfowler/flock/internal/setup"
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

	// Check and setup global Claude hooks
	if err := checkAndSetupHooks(); err != nil {
		log.Fatalf("setup failed: %v", err)
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
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
	watcher := status.NewWatcher(statusDir, statusChan, cfg)
	if err := watcher.Start(); err != nil {
		log.Fatalf("failed to start status watcher: %v", err)
	}
	defer watcher.Stop()

	// Create and run TUI
	model := tui.NewModel(manager, zjController, cfg, statusChan)
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

// checkAndSetupHooks verifies and optionally installs global Claude hooks
func checkAndSetupHooks() error {
	checker, err := setup.NewChecker()
	if err != nil {
		return err
	}

	result, err := checker.Check()
	if err != nil {
		return err
	}

	// Already configured
	if result.HooksInstalled && !result.NeedsUserConsent {
		return nil
	}

	// Need user consent to install
	fmt.Println("Flock Setup")
	fmt.Println("===========")
	fmt.Println()
	fmt.Println(result.Message)
	fmt.Println()
	fmt.Println("Flock needs to install global Claude Code hooks to track agent status.")
	fmt.Println("This will:")
	fmt.Printf("  1. Install hook script to: %s\n", checker.GetHookPath())
	fmt.Printf("  2. Update Claude settings: %s\n", checker.GetSettingsPath())
	fmt.Println()
	fmt.Println("The hooks are safe - they only activate when FLOCK_TASK_ID is set,")
	fmt.Println("so they won't affect your normal Claude Code usage.")
	fmt.Println()
	fmt.Print("Do you want to proceed? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response != "y" && response != "yes" {
		fmt.Println()
		fmt.Println("Setup cancelled. Flock cannot function without the hooks.")
		fmt.Println("You can manually configure the hooks later or run flock again.")
		os.Exit(0)
	}

	fmt.Println()
	fmt.Print("Installing hooks... ")

	if err := checker.Install(); err != nil {
		return fmt.Errorf("installation failed: %w", err)
	}

	fmt.Println("done!")
	fmt.Println()

	return nil
}
