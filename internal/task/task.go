package task

import (
	"fmt"
	"time"
)

// Status represents the current state of a task
type Status string

const (
	StatusPending Status = "PENDING" // Task created but not started
	StatusWorking Status = "WORKING" // Claude is actively working
	StatusWaiting Status = "WAITING" // Claude needs user input
	StatusDone    Status = "DONE"    // Task completed
)

// Task represents an AI agent task
type Task struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Prompt    string    `json:"prompt"`
	Cwd       string    `json:"cwd"`
	Status    Status    `json:"status"`
	TabName   string    `json:"tab_name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NewTask creates a new task with the given name and prompt
func NewTask(id, name, prompt, cwd string) *Task {
	now := time.Now()
	// Format: agent-XXX-taskName (e.g., agent-001-changingReadMe)
	sanitized := sanitizeTabName(name)
	// Truncate task name portion to keep total tab name reasonable
	if len(sanitized) > 15 {
		sanitized = sanitized[:15]
	}
	tabName := fmt.Sprintf("agent-%s-%s", id, sanitized)
	return &Task{
		ID:        id,
		Name:      name,
		Prompt:    prompt,
		Cwd:       cwd,
		Status:    StatusPending,
		TabName:   tabName,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// sanitizeTabName removes characters that might cause issues in zellij tab names
func sanitizeTabName(name string) string {
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		// Allow alphanumeric, spaces, dashes, underscores
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == ' ' || c == '-' || c == '_' {
			result = append(result, c)
		}
	}
	return string(result)
}

// Age returns the duration since the task was created
func (t *Task) Age() time.Duration {
	return time.Since(t.CreatedAt)
}

// AgeString returns a human-readable age string
func (t *Task) AgeString() string {
	age := t.Age()
	if age < time.Minute {
		return fmt.Sprintf("%ds", int(age.Seconds()))
	}
	if age < time.Hour {
		return fmt.Sprintf("%dm", int(age.Minutes()))
	}
	if age < 24*time.Hour {
		return fmt.Sprintf("%dh", int(age.Hours()))
	}
	return fmt.Sprintf("%dd", int(age.Hours()/24))
}

// IsActive returns true if the task has been started (has a running tab)
func (t *Task) IsActive() bool {
	return t.Status != StatusPending && t.Status != StatusDone
}

// NeedsAttention returns true if the task needs user input
func (t *Task) NeedsAttention() bool {
	return t.Status == StatusWaiting
}
