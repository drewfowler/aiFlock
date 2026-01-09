package zellij

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	defaultStatusDir = "/tmp/flock"
	layoutFileName   = "ai_with_editor.kdl"
)

// Controller manages zellij tabs for AI agent sessions
type Controller struct {
	layoutPath    string
	statusDir     string
	controllerTab string
}

// NewController creates a new zellij controller
func NewController(configDir string) *Controller {
	layoutPath := filepath.Join(configDir, "zellij", "layouts", layoutFileName)
	return &Controller{
		layoutPath:    layoutPath,
		statusDir:     defaultStatusDir,
		controllerTab: "flock",
	}
}

// EnsureStatusDir creates the status directory if it doesn't exist
func (c *Controller) EnsureStatusDir() error {
	return os.MkdirAll(c.statusDir, 0755)
}

// NewTab creates a new zellij tab for a task
func (c *Controller) NewTab(taskID, taskName, tabName, prompt, cwd string) error {
	if err := c.EnsureStatusDir(); err != nil {
		return fmt.Errorf("failed to create status dir: %w", err)
	}

	// Create new tab with the AI session layout
	cmd := exec.Command("zellij", "action", "new-tab", "--name", tabName, "--layout", c.layoutPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create tab: %w", err)
	}

	// Focus the claude pane (right pane in the vertical split)
	focusCmd := exec.Command("zellij", "action", "focus-next-pane")
	if err := focusCmd.Run(); err != nil {
		return fmt.Errorf("failed to focus claude pane: %w", err)
	}

	// Write the claude command with environment variables to the pane
	// Use export to ensure env vars are available to hook subprocesses
	// Global hooks at ~/.flock/hooks/ check for FLOCK_TASK_ID
	claudeCmd := fmt.Sprintf("cd %q && export FLOCK_TASK_ID=%s FLOCK_TASK_NAME=%q FLOCK_TAB_NAME=%s FLOCK_STATUS_DIR=%s && claude %q",
		cwd, taskID, taskName, tabName, c.statusDir, prompt)
	writeCmd := exec.Command("zellij", "action", "write-chars", claudeCmd)
	if err := writeCmd.Run(); err != nil {
		return fmt.Errorf("failed to write command: %w", err)
	}

	// Send enter to execute
	enterCmd := exec.Command("zellij", "action", "write", "10") // ASCII newline
	if err := enterCmd.Run(); err != nil {
		return fmt.Errorf("failed to send enter: %w", err)
	}

	// Return to the flock controller tab
	if err := c.GoToController(); err != nil {
		return fmt.Errorf("failed to return to controller: %w", err)
	}

	return nil
}

// GoToTab switches to the specified tab
func (c *Controller) GoToTab(tabName string) error {
	cmd := exec.Command("zellij", "action", "go-to-tab-name", tabName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to go to tab %s: %w", tabName, err)
	}
	return nil
}

// GoToController switches back to the controller tab
func (c *Controller) GoToController() error {
	return c.GoToTab(c.controllerTab)
}

// CloseTab closes the specified tab
func (c *Controller) CloseTab(tabName string) error {
	// Check if the tab exists before trying to close it
	// zellij action go-to-tab-name doesn't error on missing tabs, so we must check first
	if !c.TabExists(tabName) {
		return nil
	}

	// Switch to the tab
	if err := c.GoToTab(tabName); err != nil {
		return nil
	}

	// Then close it
	cmd := exec.Command("zellij", "action", "close-tab")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to close tab %s: %w", tabName, err)
	}

	return nil
}

// TabExists checks if a tab with the given name exists
func (c *Controller) TabExists(tabName string) bool {
	cmd := exec.Command("zellij", "action", "query-tab-names")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == tabName {
			return true
		}
	}
	return false
}

// StatusDir returns the status directory path
func (c *Controller) StatusDir() string {
	return c.statusDir
}

// SetControllerTab sets the name of the controller tab
func (c *Controller) SetControllerTab(name string) {
	c.controllerTab = name
}

// RenameCurrentTab renames the current tab
func (c *Controller) RenameCurrentTab(name string) error {
	cmd := exec.Command("zellij", "action", "rename-tab", name)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to rename tab: %w", err)
	}
	return nil
}

// IsInZellij checks if we're running inside a zellij session
func IsInZellij() bool {
	return os.Getenv("ZELLIJ") != ""
}

// DeleteStatusFile removes the status file for a task
func (c *Controller) DeleteStatusFile(taskID string) error {
	statusFile := filepath.Join(c.statusDir, taskID+".status")
	if err := os.Remove(statusFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete status file: %w", err)
	}
	return nil
}
