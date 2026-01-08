package zellij

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	defaultStatusDir = "/tmp/flock"
	layoutFileName   = "ai-session.kdl"
)

// Controller manages zellij tabs for AI agent sessions
type Controller struct {
	layoutPath    string
	statusDir     string
	controllerTab string
}

// NewController creates a new zellij controller
func NewController(configDir string) *Controller {
	layoutPath := filepath.Join(configDir, "configs", layoutFileName)
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
func (c *Controller) NewTab(taskID, tabName, prompt, cwd string) error {
	if err := c.EnsureStatusDir(); err != nil {
		return fmt.Errorf("failed to create status dir: %w", err)
	}

	// Set environment variables for the Claude Code hooks
	env := os.Environ()
	env = append(env,
		fmt.Sprintf("FLOCK_TASK_ID=%s", taskID),
		fmt.Sprintf("FLOCK_TAB_NAME=%s", tabName),
		fmt.Sprintf("FLOCK_STATUS_DIR=%s", c.statusDir),
	)

	// Create new tab with the AI session layout
	// Note: zellij doesn't support environment variables in layouts directly,
	// so we create a simple tab and run claude manually
	cmd := exec.Command("zellij", "action", "new-tab", "--name", tabName)
	cmd.Env = env
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create tab: %w", err)
	}

	// Now write the claude command to the new pane
	claudeCmd := fmt.Sprintf("cd %q && FLOCK_TASK_ID=%s FLOCK_TAB_NAME=%s claude %q", cwd, taskID, tabName, prompt)
	writeCmd := exec.Command("zellij", "action", "write-chars", claudeCmd)
	if err := writeCmd.Run(); err != nil {
		return fmt.Errorf("failed to write command: %w", err)
	}

	// Send enter to execute
	enterCmd := exec.Command("zellij", "action", "write", "10") // ASCII newline
	if err := enterCmd.Run(); err != nil {
		return fmt.Errorf("failed to send enter: %w", err)
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
	// First switch to the tab
	if err := c.GoToTab(tabName); err != nil {
		return err
	}

	// Then close it
	cmd := exec.Command("zellij", "action", "close-tab")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to close tab %s: %w", tabName, err)
	}

	return nil
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
