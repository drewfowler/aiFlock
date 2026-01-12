package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	DefaultConfigDir  = ".flock"
	configFileName    = "config.json"
	promptsDir        = "prompts"
	templatesDir      = "templates"
	defaultTemplate   = "default.md"
)

// WorktreeMode defines how worktrees are used for tasks
type WorktreeMode string

const (
	WorktreeModeAuto   WorktreeMode = "auto"   // Use worktrees if in a git repo
	WorktreeModeAlways WorktreeMode = "always" // Always use worktrees
	WorktreeModeNever  WorktreeMode = "never"  // Never use worktrees
)

// Config holds flock configuration
type Config struct {
	PromptsDir           string       `json:"prompts_dir"`
	TemplatesDir         string       `json:"templates_dir"`
	NotificationsEnabled bool         `json:"notifications_enabled"`
	AutoStartTasks       bool         `json:"auto_start_tasks"`
	ConfirmBeforeDelete  bool         `json:"confirm_before_delete"`
	WorktreeMode         WorktreeMode `json:"worktree_mode"`

	// Internal paths (not saved to config file)
	configDir string
}

// Load loads configuration from ~/.flock/config.json
// If the file doesn't exist, returns default configuration
func Load() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	configDir := filepath.Join(home, DefaultConfigDir)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, err
	}

	cfg := &Config{
		PromptsDir:           filepath.Join(configDir, promptsDir),
		TemplatesDir:         filepath.Join(configDir, templatesDir),
		NotificationsEnabled: true,             // enabled by default
		AutoStartTasks:       false,            // disabled by default
		ConfirmBeforeDelete:  true,             // enabled by default
		WorktreeMode:         WorktreeModeAuto, // auto by default
		configDir:            configDir,
	}

	// Try to load existing config
	configPath := filepath.Join(configDir, configFileName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create directories with defaults
			if err := cfg.ensureDirectories(); err != nil {
				return nil, err
			}
			return cfg, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	cfg.configDir = configDir

	// Ensure directories exist
	if err := cfg.ensureDirectories(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Save saves the configuration to disk
func (c *Config) Save() error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	configPath := filepath.Join(c.configDir, configFileName)
	return os.WriteFile(configPath, data, 0644)
}

// ensureDirectories creates prompts and templates directories if they don't exist
func (c *Config) ensureDirectories() error {
	if err := os.MkdirAll(c.PromptsDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(c.TemplatesDir, 0755); err != nil {
		return err
	}
	return nil
}

// ConfigDir returns the base config directory (~/.flock)
func (c *Config) ConfigDir() string {
	return c.configDir
}

// DefaultTemplatePath returns the path to the default template
func (c *Config) DefaultTemplatePath() string {
	return filepath.Join(c.TemplatesDir, defaultTemplate)
}

// PromptFilePath returns the path for a task's prompt file
func (c *Config) PromptFilePath(taskID string) string {
	return filepath.Join(c.PromptsDir, taskID+".md")
}

// CycleWorktreeMode cycles through worktree modes: auto -> always -> never -> auto
func (c *Config) CycleWorktreeMode() {
	switch c.WorktreeMode {
	case WorktreeModeAuto:
		c.WorktreeMode = WorktreeModeAlways
	case WorktreeModeAlways:
		c.WorktreeMode = WorktreeModeNever
	case WorktreeModeNever:
		c.WorktreeMode = WorktreeModeAuto
	default:
		c.WorktreeMode = WorktreeModeAuto
	}
}

// WorktreeModeLabel returns a human-readable label for the current worktree mode
func (c *Config) WorktreeModeLabel() string {
	switch c.WorktreeMode {
	case WorktreeModeAuto:
		return "Auto"
	case WorktreeModeAlways:
		return "Always"
	case WorktreeModeNever:
		return "Never"
	default:
		return "Auto"
	}
}
