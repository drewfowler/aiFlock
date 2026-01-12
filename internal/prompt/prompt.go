package prompt

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dfowler/flock/internal/config"
)

const defaultTemplateContent = `# Task: {{name}}
# Working Directory: {{working_dir}}

## Goal


## Context


## Constraints

`

// Manager handles prompt file operations
type Manager struct {
	config *config.Config
}

// NewManager creates a new prompt manager
func NewManager(cfg *config.Config) *Manager {
	return &Manager{config: cfg}
}

// EnsureProjectTemplate creates the project-specific template in .claude/flock/templates/
// if it doesn't exist. Returns the path to the template.
func (m *Manager) EnsureProjectTemplate(projectDir string) (string, error) {
	// Convert to absolute path if needed
	if !filepath.IsAbs(projectDir) {
		absPath, err := filepath.Abs(projectDir)
		if err == nil {
			projectDir = absPath
		}
	}

	// Create .claude/flock/templates directory if needed
	templatesDir := filepath.Join(projectDir, ".claude", "flock", "templates")
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create templates directory: %w", err)
	}

	templatePath := filepath.Join(templatesDir, "default.md")

	// Check if template already exists
	if _, err := os.Stat(templatePath); err == nil {
		return templatePath, nil
	}

	// Create the default template
	if err := os.WriteFile(templatePath, []byte(defaultTemplateContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write template: %w", err)
	}

	return templatePath, nil
}

// CreatePromptFile creates a new prompt file from the template
func (m *Manager) CreatePromptFile(taskID, taskName, workingDir string) (string, error) {
	return m.CreatePromptFileWithGoal(taskID, taskName, workingDir, "")
}

// CreatePromptFileWithGoal creates a new prompt file from the template with an optional goal
func (m *Manager) CreatePromptFileWithGoal(taskID, taskName, workingDir, goal string) (string, error) {
	// Ensure project template exists and get its path
	templatePath, err := m.EnsureProjectTemplate(workingDir)
	if err != nil {
		return "", fmt.Errorf("failed to ensure template: %w", err)
	}

	// Read template
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("failed to read template: %w", err)
	}

	// Replace placeholders
	content := string(templateContent)
	content = strings.ReplaceAll(content, "{{name}}", taskName)
	content = strings.ReplaceAll(content, "{{working_dir}}", workingDir)

	// If goal is provided, insert it into the Goal section
	if goal != "" {
		// Find the Goal section and insert the goal text after it
		goalSection := "## Goal\n\n"
		goalInsert := "## Goal\n\n" + goal + "\n\n"
		content = strings.Replace(content, goalSection, goalInsert, 1)
	}

	// Write prompt file
	promptPath := m.config.PromptFilePath(taskID)
	if err := os.WriteFile(promptPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write prompt file: %w", err)
	}

	return promptPath, nil
}

// OpenInEditor opens the prompt file in the user's editor and blocks until closed
func (m *Manager) OpenInEditor(promptPath string) error {
	editor := getEditor()

	cmd := exec.Command(editor, promptPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// getEditor returns the user's preferred editor
func getEditor() string {
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}
	if editor := os.Getenv("VISUAL"); editor != "" {
		return editor
	}
	// Fallback to common editors
	for _, editor := range []string{"vim", "vi", "nano"} {
		if _, err := exec.LookPath(editor); err == nil {
			return editor
		}
	}
	return "vi"
}

// GetPromptFile returns the path to a task's prompt file
func (m *Manager) GetPromptFile(taskID string) string {
	return m.config.PromptFilePath(taskID)
}

// PromptFileExists checks if a prompt file exists for the task
func (m *Manager) PromptFileExists(taskID string) bool {
	promptPath := m.config.PromptFilePath(taskID)
	_, err := os.Stat(promptPath)
	return err == nil
}

// DeletePromptFile removes a task's prompt file
func (m *Manager) DeletePromptFile(taskID string) error {
	promptPath := m.config.PromptFilePath(taskID)
	if err := os.Remove(promptPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ListTemplates returns available template files for a given project directory
func (m *Manager) ListTemplates(projectDir string) ([]string, error) {
	templatesDir := filepath.Join(projectDir, ".claude", "flock", "templates")
	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No templates directory yet
		}
		return nil, err
	}

	var templates []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".md" {
			templates = append(templates, entry.Name())
		}
	}
	return templates, nil
}
