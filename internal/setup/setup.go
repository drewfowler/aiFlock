package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const hookScript = `#!/bin/bash
# Flock status update hook for Claude Code
# This script updates the status file for a task based on the hook event
# Installed by flock - safe to run globally (no-op if not in flock context)

# Read input from stdin (JSON from Claude Code)
INPUT=$(cat)

# Get task info from environment variables
TASK_ID="${FLOCK_TASK_ID:-}"
TASK_NAME="${FLOCK_TASK_NAME:-}"
TAB_NAME="${FLOCK_TAB_NAME:-}"
STATUS_DIR="${FLOCK_STATUS_DIR:-/tmp/flock}"

# Exit silently if no task ID is set (not running in flock context)
if [ -z "$TASK_ID" ]; then
    exit 0
fi

# Validate task ID is not empty or whitespace
TASK_ID=$(echo "$TASK_ID" | tr -d '[:space:]')
if [ -z "$TASK_ID" ]; then
    exit 0
fi

# Extract hook event name from input JSON
HOOK_EVENT=$(echo "$INPUT" | sed -n 's/.*"hook_event_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')

# Fallback to environment variable
if [ -z "$HOOK_EVENT" ]; then
    HOOK_EVENT="${CLAUDE_HOOK_EVENT_NAME:-}"
fi

# Map hook event to status
case "$HOOK_EVENT" in
    "UserPromptSubmit")
        STATUS="WORKING"
        ;;
    "PreToolUse")
        STATUS="WORKING"
        ;;
    "PostToolUse")
        STATUS="WORKING"
        ;;
    "Notification")
        STATUS="WAITING"
        ;;
    "Stop")
        STATUS="DONE"
        ;;
    "SubagentStop")
        exit 0
        ;;
    *)
        exit 0
        ;;
esac

# Ensure status directory exists
mkdir -p "$STATUS_DIR"

# Write status file
STATUS_FILE="$STATUS_DIR/$TASK_ID.status"
cat > "$STATUS_FILE" << EOF
status=$STATUS
task_id=$TASK_ID
task_name=$TASK_NAME
updated=$(date +%s)
tab_name=$TAB_NAME
EOF

exit 0
`

// Result represents the outcome of the setup check
type Result struct {
	HooksInstalled   bool
	SettingsUpdated  bool
	NeedsUserConsent bool
	Message          string
}

// Checker handles the setup verification and installation
type Checker struct {
	flockDir     string
	claudeDir    string
	hookPath     string
	settingsPath string
}

// NewChecker creates a new setup checker
func NewChecker() (*Checker, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	flockDir := filepath.Join(home, ".flock")
	claudeDir := filepath.Join(home, ".claude")

	return &Checker{
		flockDir:     flockDir,
		claudeDir:    claudeDir,
		hookPath:     filepath.Join(flockDir, "hooks", "update_status.sh"),
		settingsPath: filepath.Join(claudeDir, "settings.json"),
	}, nil
}

// Check verifies if flock hooks are properly configured
func (c *Checker) Check() (*Result, error) {
	result := &Result{}

	// Check if hook script exists
	hookExists := c.hookScriptExists()

	// Check if Claude settings has flock hooks
	hasFlockHooks, err := c.hasFlockHooks()
	if err != nil {
		return nil, fmt.Errorf("failed to check Claude settings: %w", err)
	}

	if hookExists && hasFlockHooks {
		result.HooksInstalled = true
		result.Message = "Flock hooks are properly configured"
		return result, nil
	}

	result.NeedsUserConsent = true
	if !hookExists && !hasFlockHooks {
		result.Message = "Flock hooks need to be installed"
	} else if !hookExists {
		result.Message = "Hook script needs to be installed"
	} else {
		result.Message = "Claude settings need to be updated with flock hooks"
	}

	return result, nil
}

// InstallHookScript installs the hook script to ~/.flock/hooks/
func (c *Checker) InstallHookScript() error {
	hookDir := filepath.Dir(c.hookPath)
	if err := os.MkdirAll(hookDir, 0755); err != nil {
		return fmt.Errorf("failed to create hooks directory: %w", err)
	}

	if err := os.WriteFile(c.hookPath, []byte(hookScript), 0755); err != nil {
		return fmt.Errorf("failed to write hook script: %w", err)
	}

	return nil
}

// UpdateClaudeSettings updates the global Claude settings with flock hooks
func (c *Checker) UpdateClaudeSettings() error {
	// Ensure claude directory exists
	if err := os.MkdirAll(c.claudeDir, 0755); err != nil {
		return fmt.Errorf("failed to create claude directory: %w", err)
	}

	// Read existing settings or create empty
	settings := make(map[string]interface{})
	data, err := os.ReadFile(c.settingsPath)
	if err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("failed to parse existing settings: %w", err)
		}
	}

	// Create the hook command pointing to our installed script
	hookCommand := fmt.Sprintf("%q 2>/dev/null || true", c.hookPath)

	// Define the hooks we need
	flockHooks := map[string]interface{}{
		"UserPromptSubmit": []interface{}{
			map[string]interface{}{
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": hookCommand,
					},
				},
			},
		},
		"PreToolUse": []interface{}{
			map[string]interface{}{
				"matcher": "*",
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": hookCommand,
					},
				},
			},
		},
		"Notification": []interface{}{
			map[string]interface{}{
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": hookCommand,
					},
				},
			},
		},
		"Stop": []interface{}{
			map[string]interface{}{
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": hookCommand,
					},
				},
			},
		},
	}

	// Merge with existing hooks or set new
	existingHooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		existingHooks = make(map[string]interface{})
	}

	// Add our hooks (this will override existing hooks for these events)
	for event, hook := range flockHooks {
		existingHooks[event] = hook
	}
	settings["hooks"] = existingHooks

	// Write back with nice formatting
	output, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	if err := os.WriteFile(c.settingsPath, output, 0644); err != nil {
		return fmt.Errorf("failed to write settings: %w", err)
	}

	return nil
}

// Install performs the full installation
func (c *Checker) Install() error {
	if err := c.InstallHookScript(); err != nil {
		return err
	}
	if err := c.UpdateClaudeSettings(); err != nil {
		return err
	}
	return nil
}

// hookScriptExists checks if our hook script is installed
func (c *Checker) hookScriptExists() bool {
	info, err := os.Stat(c.hookPath)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// hasFlockHooks checks if Claude settings has flock hooks configured
func (c *Checker) hasFlockHooks() (bool, error) {
	data, err := os.ReadFile(c.settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return false, err
	}

	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		return false, nil
	}

	// Check if any hook references our flock hook path
	hookJSON, _ := json.Marshal(hooks)
	hookStr := string(hookJSON)

	// Look for either the new path or old FLOCK_PROJECT_DIR reference
	return contains(hookStr, ".flock/hooks/update_status.sh") ||
		contains(hookStr, "FLOCK_PROJECT_DIR"), nil
}

// GetSettingsPath returns the path to Claude settings for display
func (c *Checker) GetSettingsPath() string {
	return c.settingsPath
}

// GetHookPath returns the path where hooks will be installed
func (c *Checker) GetHookPath() string {
	return c.hookPath
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
