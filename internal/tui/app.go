package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/dfowler/flock/internal/config"
	"github.com/dfowler/flock/internal/git"
	"github.com/dfowler/flock/internal/prompt"
	"github.com/dfowler/flock/internal/task"
	"github.com/dfowler/flock/internal/zellij"
	"golang.org/x/term"
)

// View modes
type viewMode int

const (
	viewDashboard viewMode = iota
	viewNewTask
	viewEditTask
	viewConfirmDelete
	viewConfirmWorktreeDelete
	viewConfirmMerge
	viewSettings
)

// Message represents a status message to display in the TUI
type Message struct {
	Text      string
	IsError   bool
	Timestamp time.Time
}

// Model is the main TUI model
type Model struct {
	tasks         *task.Manager
	zellij        *zellij.Controller
	config        *config.Config
	promptMgr     *prompt.Manager
	gitAssigner   *git.Assigner
	selected      int
	mode          viewMode
	width         int
	height        int
	statusUpdates chan StatusUpdate
	err           error

	// New task form (name, cwd, and optional goal - full prompt can be edited in external editor)
	nameInput  textinput.Model
	cwdInput   textinput.Model
	goalInput  textinput.Model
	focusIndex int

	// Edit task tracking
	editingTaskID string

	// Delete confirmation tracking
	deletingTaskID string

	// Merge confirmation tracking
	mergingTaskID string
	mergeDiffInfo string

	// Settings popup tracking
	settingsSelected int

	// Spinner for working status
	spinner spinner.Model

	// Status messages for the messages panel
	messages []Message

	// Glamour renderer for markdown (cached to avoid recreation on every render)
	glamourRenderer      *glamour.TermRenderer
	glamourRendererWidth int
}

// StatusUpdate represents a status change from the watcher
type StatusUpdate struct {
	TaskID string
	Status task.Status
}

// StatusMsg is sent when a status update is received
type StatusMsg StatusUpdate

// editorFinishedMsg is sent when the external editor closes for new task
type editorFinishedMsg struct {
	taskName   string
	promptFile string
	cwd        string
	err        error
}

// editFinishedMsg is sent when editing an existing task's prompt file completes
type editFinishedMsg struct {
	err error
}

// fzfFinishedMsg is sent when fzf directory selection completes
type fzfFinishedMsg struct {
	dir string
	err error
}

// NewModel creates a new TUI model
func NewModel(tasks *task.Manager, zj *zellij.Controller, cfg *config.Config, gitAssigner *git.Assigner, statusChan chan StatusUpdate) Model {
	// Name input
	nameInput := textinput.New()
	nameInput.Placeholder = "Task name"
	nameInput.CharLimit = 50
	nameInput.Width = 40

	// CWD input
	cwdInput := textinput.New()
	cwdInput.Placeholder = "Working directory (leave empty for current)"
	cwdInput.CharLimit = 200
	cwdInput.Width = 60

	// Prompt input (optional short prompt)
	goalInput := textinput.New()
	goalInput.Placeholder = "Prompt (optional - leave empty to open editor)"
	goalInput.CharLimit = 500
	goalInput.Width = 60

	// Spinner for working status
	s := spinner.New()
	s.Spinner = spinner.Spinner{
		Frames: []string{"⡇", "⠏", "⠛", "⠹", "⢸", "⣰", "⣤", "⣆"},
		FPS:    time.Millisecond * 100,
	}
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("39")) // blue

	// Get initial terminal size
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width, height = 80, 24 // fallback defaults
	}

	// Initialize glamour renderer
	// Right panel is 1/2 of width, content width subtracts borders (2) + padding (4)
	rightWidth := width - (width / 2)
	if rightWidth < 30 {
		rightWidth = 30
	}
	promptContentWidth := rightWidth - 6
	if promptContentWidth < 10 {
		promptContentWidth = 10
	}
	glamourRenderer, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(promptContentWidth),
	)

	return Model{
		tasks:                tasks,
		zellij:               zj,
		config:               cfg,
		promptMgr:            prompt.NewManager(cfg),
		gitAssigner:          gitAssigner,
		statusUpdates:        statusChan,
		nameInput:            nameInput,
		cwdInput:             cwdInput,
		goalInput:            goalInput,
		spinner:              s,
		width:                width,
		height:               height,
		glamourRenderer:      glamourRenderer,
		glamourRendererWidth: promptContentWidth,
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		waitForStatus(m.statusUpdates),
		m.spinner.Tick,
	)
}

// addMessage adds a message to the messages panel (keeps last 5 messages)
func (m *Model) addMessage(text string, isError bool) {
	msg := Message{
		Text:      text,
		IsError:   isError,
		Timestamp: time.Now(),
	}
	m.messages = append(m.messages, msg)
	// Keep only last 5 messages
	if len(m.messages) > 5 {
		m.messages = m.messages[len(m.messages)-5:]
	}
}

// waitForStatus waits for status updates from the watcher
func waitForStatus(ch chan StatusUpdate) tea.Cmd {
	return func() tea.Msg {
		return StatusMsg(<-ch)
	}
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Calculate prompt panel content width and update glamour renderer if needed
		// Right panel is 1/2 of width, content width subtracts borders (2) + padding (4)
		rightWidth := msg.Width - (msg.Width / 2)
		if rightWidth < 30 {
			rightWidth = 30
		}
		promptContentWidth := rightWidth - 6
		if promptContentWidth < 10 {
			promptContentWidth = 10
		}
		if m.glamourRendererWidth != promptContentWidth {
			if renderer, err := glamour.NewTermRenderer(
				glamour.WithAutoStyle(),
				glamour.WithWordWrap(promptContentWidth),
			); err == nil {
				m.glamourRenderer = renderer
				m.glamourRendererWidth = promptContentWidth
			}
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case StatusMsg:
		// Update task status (silently ignore if task doesn't exist)
		if t, exists := m.tasks.Get(msg.TaskID); exists {
			oldStatus := t.Status
			if err := m.tasks.UpdateStatus(msg.TaskID, msg.Status); err != nil {
				m.err = err
				m.addMessage(fmt.Sprintf("Error updating %s: %v", t.Name, err), true)
			} else if oldStatus != msg.Status && m.config.NotificationsEnabled {
				m.addMessage(fmt.Sprintf("%s → %s", t.Name, msg.Status), false)
			}
		}
		// Continue listening for updates
		return m, waitForStatus(m.statusUpdates)

	case editorFinishedMsg:
		// Editor closed - create the task
		if msg.err != nil {
			m.err = msg.err
			m.addMessage(fmt.Sprintf("Editor error: %v", msg.err), true)
		} else {
			// Try to assign a worktree if enabled
			var createOpts *task.CreateOptions
			if m.gitAssigner != nil {
				taskID := m.tasks.NextID()
				cwd := msg.cwd
				if cwd == "" {
					cwd = "."
				}
				// Convert to absolute path for worktree assignment
				if !filepath.IsAbs(cwd) {
					if absCwd, err := filepath.Abs(cwd); err == nil {
						cwd = absCwd
					}
				}
				// Get active tasks for worktree assignment
				activeTasks := m.getTaskWorktreeInfos()
				if assignment, err := m.gitAssigner.AssignWorktree(taskID, cwd, activeTasks); err != nil {
					m.addMessage(fmt.Sprintf("Worktree warning: %v", err), true)
				} else if assignment != nil {
					createOpts = &task.CreateOptions{
						WorktreePath: assignment.WorktreePath,
						GitBranch:    assignment.GitBranch,
						RepoRoot:     assignment.RepoRoot,
					}
				}
			}

			// Create the task with the prompt file and optional worktree
			t, err := m.tasks.CreateWithOptions(msg.taskName, msg.promptFile, msg.cwd, createOpts)
			if err != nil {
				m.err = err
				m.addMessage(fmt.Sprintf("Failed to create task: %v", err), true)
			} else {
				if t.GitBranch != "" {
					m.addMessage(fmt.Sprintf("Created task: %s (branch: %s)", msg.taskName, t.GitBranch), false)
				} else {
					m.addMessage(fmt.Sprintf("Created task: %s", msg.taskName), false)
				}
				m.selected = m.tasks.Count() - 1

				// Auto-start if enabled
				if m.config.AutoStartTasks {
					cwd := t.EffectiveCwd()
					if cwd == "" {
						cwd = "."
					}
					promptOrFile := t.GetPromptOrFile()
					isFile := t.PromptFile != ""
					if err := m.zellij.NewTab(t.ID, t.Name, t.TabName, promptOrFile, cwd, isFile); err != nil {
						m.err = err
						m.addMessage(fmt.Sprintf("Failed to auto-start: %v", err), true)
					} else {
						m.tasks.UpdateStatus(t.ID, task.StatusWorking)
					}
				}
			}
		}
		m.mode = viewDashboard
		return m, nil

	case editFinishedMsg:
		// Editor closed after editing existing task
		if msg.err != nil {
			m.err = msg.err
			m.addMessage(fmt.Sprintf("Editor error: %v", msg.err), true)
		} else {
			m.addMessage("Task updated", false)
		}
		m.mode = viewDashboard
		return m, nil

	case fzfFinishedMsg:
		// fzf directory selection completed
		if msg.err != nil {
			m.addMessage(fmt.Sprintf("fzf error: %v", msg.err), true)
		} else if msg.dir != "" {
			m.cwdInput.SetValue(msg.dir)
		}
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case viewDashboard:
			return m.updateDashboard(msg)
		case viewNewTask:
			return m.updateNewTask(msg)
		case viewEditTask:
			return m.updateEditTask(msg)
		case viewConfirmDelete:
			return m.updateConfirmDelete(msg)
		case viewConfirmWorktreeDelete:
			return m.updateConfirmWorktreeDelete(msg)
		case viewConfirmMerge:
			return m.updateConfirmMerge(msg)
		case viewSettings:
			return m.updateSettings(msg)
		}
	}

	return m, tea.Batch(cmds...)
}

// updateDashboard handles dashboard view input
func (m Model) updateDashboard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	tasks := m.tasks.List()

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "j", "down":
		if m.selected < len(tasks)-1 {
			m.selected++
		}

	case "k", "up":
		if m.selected > 0 {
			m.selected--
		}

	case "n":
		m.mode = viewNewTask
		m.nameInput.Focus()
		m.focusIndex = 0
		return m, textinput.Blink

	case "e":
		// Edit selected task (only if PENDING)
		if len(tasks) > 0 && m.selected < len(tasks) {
			t := tasks[m.selected]
			if t.Status == task.StatusPending {
				m.mode = viewEditTask
				m.editingTaskID = t.ID
				m.nameInput.SetValue(t.Name)
				m.cwdInput.SetValue(t.Cwd)
				m.nameInput.Focus()
				m.focusIndex = 0
				return m, textinput.Blink
			}
		}

	case "s":
		// Start selected task
		if len(tasks) > 0 && m.selected < len(tasks) {
			t := tasks[m.selected]
			if t.Status == task.StatusPending {
				cwd := t.EffectiveCwd()
				if cwd == "" {
					cwd = "."
				}
				// Use PromptFile if available, otherwise fall back to legacy Prompt
				promptOrFile := t.GetPromptOrFile()
				isFile := t.PromptFile != ""
				if err := m.zellij.NewTab(t.ID, t.Name, t.TabName, promptOrFile, cwd, isFile); err != nil {
					m.err = err
				} else {
					m.tasks.UpdateStatus(t.ID, task.StatusWorking)
				}
			}
		}

	case "enter":
		// Jump to task tab
		if len(tasks) > 0 && m.selected < len(tasks) {
			t := tasks[m.selected]
			if t.Status != task.StatusPending && t.TabName != "" {
				if err := m.zellij.GoToTab(t.TabName); err != nil {
					m.err = err
				}
			}
		}

	case "d":
		// Delete task (with or without confirmation based on settings)
		if len(tasks) > 0 && m.selected < len(tasks) {
			t := tasks[m.selected]
			if m.config.ConfirmBeforeDelete {
				m.deletingTaskID = t.ID
				m.mode = viewConfirmDelete
			} else {
				// Delete immediately without confirmation
				m.deleteTask(t.ID)
			}
		}

	case "m":
		// Merge task branch into main (only for tasks with worktrees)
		if len(tasks) > 0 && m.selected < len(tasks) {
			t := tasks[m.selected]
			if t.GitBranch != "" && t.RepoRoot != "" {
				m.mergingTaskID = t.ID
				// Get diff info for display
				if diffInfo, err := git.GetBranchDiff(t.RepoRoot, t.GitBranch); err == nil {
					m.mergeDiffInfo = diffInfo
				} else {
					m.mergeDiffInfo = "Unable to get diff info"
				}
				m.mode = viewConfirmMerge
			}
		}

	case "S":
		// Open settings popup
		m.mode = viewSettings
		m.settingsSelected = 0
	}

	return m, nil
}

// updateNewTask handles new task form input
func (m Model) updateNewTask(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "esc":
		m.mode = viewDashboard
		m.nameInput.Reset()
		m.cwdInput.Reset()
		m.goalInput.Reset()
		return m, nil

	case "tab", "shift+tab", "down", "up":
		// Cycle focus between name, cwd, and goal (3 fields)
		if msg.String() == "shift+tab" || msg.String() == "up" {
			m.focusIndex--
			if m.focusIndex < 0 {
				m.focusIndex = 2
			}
		} else {
			m.focusIndex++
			if m.focusIndex > 2 {
				m.focusIndex = 0
			}
		}

		m.nameInput.Blur()
		m.cwdInput.Blur()
		m.goalInput.Blur()

		switch m.focusIndex {
		case 0:
			m.nameInput.Focus()
		case 1:
			m.cwdInput.Focus()
		case 2:
			m.goalInput.Focus()
		}

		return m, textinput.Blink

	case "ctrl+f":
		// Open fzf to select a directory
		return m, m.openFzfDirSelector()

	case "ctrl+e":
		// Force open editor even if goal is filled
		name := strings.TrimSpace(m.nameInput.Value())
		cwd := strings.TrimSpace(m.cwdInput.Value())
		goal := strings.TrimSpace(m.goalInput.Value())

		if name != "" {
			// Reset inputs now
			m.nameInput.Reset()
			m.cwdInput.Reset()
			m.goalInput.Reset()

			// Get next task ID and create prompt file
			taskID := m.tasks.NextID()
			if cwd == "" {
				cwd = "."
			}

			// Create prompt file from template with goal
			promptFile, err := m.promptMgr.CreatePromptFileWithGoal(taskID, name, cwd, goal)
			if err != nil {
				m.err = err
				m.addMessage(fmt.Sprintf("Failed to create prompt file: %v", err), true)
				m.mode = viewDashboard
				return m, nil
			}

			// Open editor - this suspends the TUI
			return m, m.openEditor(name, promptFile, cwd)
		}
		return m, nil

	case "enter":
		// Create task - if goal is empty, open editor; otherwise create directly
		name := strings.TrimSpace(m.nameInput.Value())
		cwd := strings.TrimSpace(m.cwdInput.Value())
		goal := strings.TrimSpace(m.goalInput.Value())

		if name != "" {
			// Reset inputs now
			m.nameInput.Reset()
			m.cwdInput.Reset()
			m.goalInput.Reset()

			// Get next task ID and create prompt file
			taskID := m.tasks.NextID()
			if cwd == "" {
				cwd = "."
			}

			// Create prompt file from template with goal
			promptFile, err := m.promptMgr.CreatePromptFileWithGoal(taskID, name, cwd, goal)
			if err != nil {
				m.err = err
				m.addMessage(fmt.Sprintf("Failed to create prompt file: %v", err), true)
				m.mode = viewDashboard
				return m, nil
			}

			if goal == "" {
				// No goal provided - open editor
				return m, m.openEditor(name, promptFile, cwd)
			}

			// Goal provided - create task directly without opening editor
			return m, func() tea.Msg {
				return editorFinishedMsg{
					taskName:   name,
					promptFile: promptFile,
					cwd:        cwd,
					err:        nil,
				}
			}
		}
		return m, nil
	}

	// Update focused input
	var cmd tea.Cmd
	switch m.focusIndex {
	case 0:
		m.nameInput, cmd = m.nameInput.Update(msg)
	case 1:
		m.cwdInput, cmd = m.cwdInput.Update(msg)
	case 2:
		m.goalInput, cmd = m.goalInput.Update(msg)
	}

	return m, cmd
}

// openEditor returns a command that opens the editor and sends editorFinishedMsg when done
func (m Model) openEditor(taskName, promptFile, cwd string) tea.Cmd {
	editor := getEditor()

	// For GUI editors, start the process without blocking and return immediately
	if isGUIEditor(editor) {
		return func() tea.Msg {
			c := exec.Command(editor, promptFile)
			if err := c.Start(); err != nil {
				return editorFinishedMsg{
					taskName:   taskName,
					promptFile: promptFile,
					cwd:        cwd,
					err:        err,
				}
			}
			// Don't wait for GUI editor to close - return success immediately
			return editorFinishedMsg{
				taskName:   taskName,
				promptFile: promptFile,
				cwd:        cwd,
				err:        nil,
			}
		}
	}

	// For terminal editors, block until the editor closes
	c := exec.Command(editor, promptFile)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editorFinishedMsg{
			taskName:   taskName,
			promptFile: promptFile,
			cwd:        cwd,
			err:        err,
		}
	})
}

// getEditor returns the user's preferred editor
func getEditor() string {
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}
	if editor := os.Getenv("VISUAL"); editor != "" {
		return editor
	}
	return "vi"
}

// isGUIEditor returns true if the editor is a GUI application that detaches from the terminal
func isGUIEditor(editor string) bool {
	// Get just the binary name (handles paths like /usr/bin/code)
	base := filepath.Base(editor)
	// Handle cases like "code -w" by taking just the first part
	if idx := strings.Index(base, " "); idx != -1 {
		base = base[:idx]
	}

	guiEditors := []string{
		"code",          // VS Code
		"code-insiders", // VS Code Insiders
		"cursor",        // Cursor editor
		"subl",          // Sublime Text
		"sublime",       // Sublime Text
		"atom",          // Atom
		"gedit",         // GNOME editor
		"kate",          // KDE editor
		"gvim",          // GUI Vim
		"mvim",          // MacVim
		"idea",          // IntelliJ IDEA
		"goland",        // GoLand
		"pycharm",       // PyCharm
		"webstorm",      // WebStorm
		"zed",           // Zed editor
	}

	for _, gui := range guiEditors {
		if base == gui {
			return true
		}
	}
	return false
}

// updateEditTask handles edit task form input
func (m Model) updateEditTask(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "esc":
		m.mode = viewDashboard
		m.nameInput.Reset()
		m.cwdInput.Reset()
		m.editingTaskID = ""
		return m, nil

	case "tab", "shift+tab", "down", "up":
		// Cycle focus between name and cwd (2 fields)
		if msg.String() == "shift+tab" || msg.String() == "up" {
			m.focusIndex--
			if m.focusIndex < 0 {
				m.focusIndex = 1
			}
		} else {
			m.focusIndex++
			if m.focusIndex > 1 {
				m.focusIndex = 0
			}
		}

		m.nameInput.Blur()
		m.cwdInput.Blur()

		switch m.focusIndex {
		case 0:
			m.nameInput.Focus()
		case 1:
			m.cwdInput.Focus()
		}

		return m, textinput.Blink

	case "ctrl+f":
		// Open fzf to select a directory
		return m, m.openFzfDirSelector()

	case "enter":
		// Update task if name is filled
		name := strings.TrimSpace(m.nameInput.Value())
		cwd := strings.TrimSpace(m.cwdInput.Value())

		if name != "" {
			taskID := m.editingTaskID
			t, ok := m.tasks.Get(taskID)
			if !ok {
				m.mode = viewDashboard
				m.editingTaskID = ""
				return m, nil
			}

			// Update name and cwd
			if err := m.tasks.Update(taskID, func(t *task.Task) {
				t.Name = name
				t.Cwd = cwd
			}); err != nil {
				m.err = err
			}

			m.nameInput.Reset()
			m.cwdInput.Reset()
			m.editingTaskID = ""

			// Open editor for the prompt file
			return m, m.openEditorForEdit(t.PromptFile)
		}
		return m, nil
	}

	// Update focused input
	var cmd tea.Cmd
	switch m.focusIndex {
	case 0:
		m.nameInput, cmd = m.nameInput.Update(msg)
	case 1:
		m.cwdInput, cmd = m.cwdInput.Update(msg)
	}

	return m, cmd
}

// openEditorForEdit opens the editor for an existing prompt file
func (m Model) openEditorForEdit(promptFile string) tea.Cmd {
	editor := getEditor()

	// For GUI editors, start the process without blocking and return immediately
	if isGUIEditor(editor) {
		return func() tea.Msg {
			c := exec.Command(editor, promptFile)
			if err := c.Start(); err != nil {
				return editFinishedMsg{err: err}
			}
			// Don't wait for GUI editor to close
			return editFinishedMsg{err: nil}
		}
	}

	// For terminal editors, block until the editor closes
	c := exec.Command(editor, promptFile)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editFinishedMsg{err: err}
	})
}

// openFzfDirSelector opens fzf to select a directory
func (m Model) openFzfDirSelector() tea.Cmd {
	// Get home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return func() tea.Msg {
			return fzfFinishedMsg{dir: "", err: err}
		}
	}

	// Use fd if available, otherwise fall back to find
	// fd: fd --type d
	// find: find . -type d
	var listCmd string
	if _, err := exec.LookPath("fd"); err == nil {
		listCmd = "fd --type d --hidden --exclude .git . " + homeDir
	} else {
		listCmd = "find " + homeDir + " -type d -name '.git' -prune -o -type d -print"
	}

	// Create a temp file to capture output
	tmpFile, err := os.CreateTemp("", "flock-fzf-*.txt")
	if err != nil {
		return func() tea.Msg {
			return fzfFinishedMsg{dir: "", err: err}
		}
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	// Pipe to fzf and write output to temp file
	c := exec.Command("bash", "-c", listCmd+" | fzf --prompt='Select directory: ' > "+tmpPath)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		defer os.Remove(tmpPath)

		if err != nil {
			// fzf returns exit code 130 when cancelled (Ctrl+C or Esc)
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 130 {
				return fzfFinishedMsg{dir: "", err: nil}
			}
			return fzfFinishedMsg{dir: "", err: err}
		}

		// Read selected directory from temp file
		content, readErr := os.ReadFile(tmpPath)
		if readErr != nil {
			return fzfFinishedMsg{dir: "", err: readErr}
		}

		dir := strings.TrimSpace(string(content))
		return fzfFinishedMsg{dir: dir, err: nil}
	})
}

// updateConfirmDelete handles delete confirmation input
func (m Model) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		// Confirm deletion - check if we need to ask about worktree
		if t, ok := m.tasks.Get(m.deletingTaskID); ok && t.WorktreePath != "" {
			if m.config.Worktrees.Cleanup == config.WorktreeCleanupAsk {
				// Show worktree deletion confirmation
				m.mode = viewConfirmWorktreeDelete
				return m, nil
			}
			// Auto or keep - proceed with appropriate action
			m.deleteTaskWithWorktreeOption(m.deletingTaskID, m.config.Worktrees.Cleanup == config.WorktreeCleanupDelete)
		} else {
			m.deleteTaskWithWorktreeOption(m.deletingTaskID, false)
		}
		m.deletingTaskID = ""
		m.mode = viewDashboard

	case "n", "N", "esc":
		// Cancel deletion
		m.deletingTaskID = ""
		m.mode = viewDashboard

	case "ctrl+c":
		return m, tea.Quit
	}

	return m, nil
}

// updateConfirmWorktreeDelete handles worktree deletion confirmation input
func (m Model) updateConfirmWorktreeDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		// Delete task and worktree
		m.deleteTaskWithWorktreeOption(m.deletingTaskID, true)
		m.deletingTaskID = ""
		m.mode = viewDashboard

	case "n", "N", "enter":
		// Delete task but keep worktree
		m.deleteTaskWithWorktreeOption(m.deletingTaskID, false)
		m.deletingTaskID = ""
		m.mode = viewDashboard

	case "esc":
		// Cancel - go back to delete confirmation or dashboard
		m.deletingTaskID = ""
		m.mode = viewDashboard

	case "ctrl+c":
		return m, tea.Quit
	}

	return m, nil
}

// updateConfirmMerge handles merge confirmation input
func (m Model) updateConfirmMerge(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		// Perform the merge
		if t, ok := m.tasks.Get(m.mergingTaskID); ok && t.GitBranch != "" && t.RepoRoot != "" {
			result, err := git.MergeBranch(t.RepoRoot, t.GitBranch)
			if err != nil {
				m.addMessage(fmt.Sprintf("Merge error: %v", err), true)
			} else if result.Success {
				m.addMessage(result.Message, false)
			} else {
				m.addMessage(result.Message, true)
			}
		}
		m.mergingTaskID = ""
		m.mergeDiffInfo = ""
		m.mode = viewDashboard

	case "n", "N", "esc":
		// Cancel merge
		m.mergingTaskID = ""
		m.mergeDiffInfo = ""
		m.mode = viewDashboard

	case "ctrl+c":
		return m, tea.Quit
	}

	return m, nil
}

// updateSettings handles settings popup input
func (m Model) updateSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	settingsCount := 4

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "esc", "S":
		m.mode = viewDashboard
		return m, nil

	case "j", "down":
		if m.settingsSelected < settingsCount-1 {
			m.settingsSelected++
		}

	case "k", "up":
		if m.settingsSelected > 0 {
			m.settingsSelected--
		}

	case "enter", " ":
		// Toggle the selected setting
		switch m.settingsSelected {
		case 0:
			m.config.NotificationsEnabled = !m.config.NotificationsEnabled
		case 1:
			m.config.AutoStartTasks = !m.config.AutoStartTasks
		case 2:
			m.config.ConfirmBeforeDelete = !m.config.ConfirmBeforeDelete
		case 3:
			// Cycle through worktree cleanup options: ask -> delete -> keep -> ask
			switch m.config.Worktrees.Cleanup {
			case config.WorktreeCleanupAsk:
				m.config.Worktrees.Cleanup = config.WorktreeCleanupDelete
			case config.WorktreeCleanupDelete:
				m.config.Worktrees.Cleanup = config.WorktreeCleanupKeep
			case config.WorktreeCleanupKeep:
				m.config.Worktrees.Cleanup = config.WorktreeCleanupAsk
			default:
				m.config.Worktrees.Cleanup = config.WorktreeCleanupAsk
			}
		}
		if err := m.config.Save(); err != nil {
			m.addMessage(fmt.Sprintf("Failed to save settings: %v", err), true)
		}
	}

	return m, nil
}

// deleteTask handles the actual deletion of a task (legacy wrapper)
func (m *Model) deleteTask(taskID string) {
	// For non-confirmation deletes, check cleanup setting
	if t, ok := m.tasks.Get(taskID); ok && t.WorktreePath != "" {
		deleteWorktree := m.config.Worktrees.Cleanup == config.WorktreeCleanupDelete
		m.deleteTaskWithWorktreeOption(taskID, deleteWorktree)
	} else {
		m.deleteTaskWithWorktreeOption(taskID, false)
	}
}

// deleteTaskWithWorktreeOption handles deletion with explicit worktree cleanup option
func (m *Model) deleteTaskWithWorktreeOption(taskID string, deleteWorktree bool) {
	if t, ok := m.tasks.Get(taskID); ok {
		// Close the zellij tab if task was started
		if t.Status != task.StatusPending && t.TabName != "" {
			if err := m.zellij.CloseTab(t.TabName); err != nil {
				m.err = err
			}
			m.zellij.GoToController()
		}
		// Delete the status file to prevent stale updates
		m.zellij.DeleteStatusFile(taskID)
		// Delete the prompt file
		m.promptMgr.DeletePromptFile(taskID)
		// Release the worktree if assigned and deletion requested
		if deleteWorktree && m.gitAssigner != nil && t.WorktreePath != "" {
			if err := m.gitAssigner.ReleaseWorktree(t.WorktreePath, t.RepoRoot); err != nil {
				m.addMessage(fmt.Sprintf("Worktree cleanup warning: %v", err), true)
			} else {
				m.addMessage(fmt.Sprintf("Deleted worktree: %s", t.GitBranch), false)
			}
		} else if t.WorktreePath != "" && !deleteWorktree {
			m.addMessage(fmt.Sprintf("Kept worktree: %s", t.WorktreePath), false)
		}
		if err := m.tasks.Delete(taskID); err != nil {
			m.err = err
		}
		if m.selected >= len(m.tasks.List()) && m.selected > 0 {
			m.selected--
		}
	}
}

// View renders the UI
func (m Model) View() string {
	switch m.mode {
	case viewNewTask:
		return m.viewNewTask()
	case viewEditTask:
		return m.viewEditTask()
	case viewConfirmDelete:
		return m.viewConfirmDelete()
	case viewConfirmWorktreeDelete:
		return m.viewConfirmWorktreeDelete()
	case viewConfirmMerge:
		return m.viewConfirmMerge()
	case viewSettings:
		return m.viewSettings()
	default:
		return m.viewDashboard()
	}
}

// viewDashboard renders the main dashboard
func (m Model) viewDashboard() string {
	// Use actual terminal dimensions
	availableWidth := m.width
	availableHeight := m.height

	// Fallback for very small terminals or before first WindowSizeMsg
	if availableWidth < 60 || availableHeight < 15 {
		if availableWidth == 0 || availableHeight == 0 {
			return "Loading..."
		}
		return "Terminal too small. Please resize."
	}

	// Height allocation:
	// - Help bar: 1 line
	// - Status panel: fixed content height + borders
	// - Top row: remaining space
	helpBarHeight := 1
	statusContentHeight := 5                           // Content lines for status messages
	statusPanelHeight := statusContentHeight + 2       // +2 for borders
	topRowHeight := availableHeight - statusPanelHeight - helpBarHeight

	// Ensure minimum heights
	if topRowHeight < 10 {
		topRowHeight = 10
	}

	// Width allocation for columns - split equally
	leftWidth := availableWidth / 2
	rightWidth := availableWidth - leftWidth

	// Ensure minimum widths
	if leftWidth < 30 {
		leftWidth = 30
	}
	if rightWidth < 30 {
		rightWidth = 30
	}

	// Render panels
	// Width passed is total panel width (renderPanel handles borders internally)
	tasksPanel := m.renderTasksPanel(leftWidth, topRowHeight)
	promptPanel := m.renderPromptPanel(rightWidth, topRowHeight)
	statusPanel := m.renderStatusPanel(availableWidth, statusPanelHeight)

	// Help bar - truncate if needed
	helpText := "[n]ew  [e]dit  [s]tart  [m]erge  [S]ettings  [j/k]navigate  [enter]jump  [d]elete  [q]uit"
	if len(helpText) > availableWidth-2 {
		helpText = "[n]ew [e]dit [s]tart [m]erge [S]et [j/k]nav [enter]jump [d]el [q]uit"
	}
	helpBar := helpStyle.Render(helpText)

	// Compose layout: top row (tasks | prompt), then status, then help
	topRow := lipgloss.JoinHorizontal(lipgloss.Top, tasksPanel, promptPanel)
	content := lipgloss.JoinVertical(lipgloss.Left, topRow, statusPanel, helpBar)
	return content
}

// viewNewTask renders the new task form
func (m Model) viewNewTask() string {
	var b strings.Builder

	title := titleStyle.Render("New Task")
	b.WriteString(title)
	b.WriteString("\n\n")

	// Form fields
	b.WriteString(inputLabelStyle.Render("Name:"))
	b.WriteString("\n")
	b.WriteString(m.nameInput.View())
	b.WriteString("\n\n")

	b.WriteString(inputLabelStyle.Render("Working Directory:"))
	b.WriteString("\n")
	b.WriteString(m.cwdInput.View())
	b.WriteString("\n\n")

	b.WriteString(inputLabelStyle.Render("Prompt:"))
	b.WriteString("\n")
	b.WriteString(m.goalInput.View())
	b.WriteString("\n\n")

	b.WriteString(lipgloss.NewStyle().Foreground(colorSecondary).Render("Enter with prompt: create task | Enter without: open editor"))
	b.WriteString("\n\n")

	help := helpStyle.Render("[tab]next  [ctrl+f]fzf dir  [ctrl+e]open editor  [enter]create  [esc]cancel")
	b.WriteString(help)

	return m.centerContent(modalStyle.Render(b.String()))
}

// viewEditTask renders the edit task form
func (m Model) viewEditTask() string {
	var b strings.Builder

	title := titleStyle.Render("Edit Task")
	b.WriteString(title)
	b.WriteString("\n\n")

	// Form fields
	b.WriteString(inputLabelStyle.Render("Name:"))
	b.WriteString("\n")
	b.WriteString(m.nameInput.View())
	b.WriteString("\n\n")

	b.WriteString(inputLabelStyle.Render("Working Directory:"))
	b.WriteString("\n")
	b.WriteString(m.cwdInput.View())
	b.WriteString("\n\n")

	b.WriteString(lipgloss.NewStyle().Foreground(colorSecondary).Render("Press Enter to edit task prompt in editor..."))
	b.WriteString("\n\n")

	help := helpStyle.Render("[tab]next field  [ctrl+f]fzf dir  [enter]open editor  [esc]cancel")
	b.WriteString(help)

	return m.centerContent(modalStyle.Render(b.String()))
}

// viewConfirmDelete renders the delete confirmation dialog
func (m Model) viewConfirmDelete() string {
	var b strings.Builder

	t, ok := m.tasks.Get(m.deletingTaskID)
	if !ok {
		return m.viewDashboard()
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorError).
		Render("Delete Task?")
	b.WriteString(title)
	b.WriteString("\n\n")

	b.WriteString(fmt.Sprintf("Are you sure you want to delete task '%s'?\n", t.Name))

	if t.Status != task.StatusPending && t.Status != task.StatusDone {
		warning := lipgloss.NewStyle().
			Foreground(colorWarning).
			Render("Warning: This task is still running!")
		b.WriteString("\n" + warning + "\n")
	}

	b.WriteString("\n")
	help := helpStyle.Render("[y/enter]yes  [n]o  [esc]cancel")
	b.WriteString(help)

	return m.centerContent(modalStyle.Render(b.String()))
}

// viewConfirmWorktreeDelete renders the worktree deletion confirmation dialog
func (m Model) viewConfirmWorktreeDelete() string {
	var b strings.Builder

	t, ok := m.tasks.Get(m.deletingTaskID)
	if !ok {
		return m.viewDashboard()
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorWarning).
		Render("Delete Worktree?")
	b.WriteString(title)
	b.WriteString("\n\n")

	b.WriteString(fmt.Sprintf("Task '%s' has an associated worktree:\n", t.Name))
	b.WriteString(lipgloss.NewStyle().Foreground(colorSecondary).Render(fmt.Sprintf("  Branch: %s\n", t.GitBranch)))
	b.WriteString(lipgloss.NewStyle().Foreground(colorSecondary).Render(fmt.Sprintf("  Path: %s\n", t.WorktreePath)))
	b.WriteString("\n")
	b.WriteString("Do you want to delete the worktree and its branch?\n")

	b.WriteString("\n")
	help := helpStyle.Render("[y]es delete  [n/enter]keep worktree  [esc]cancel")
	b.WriteString(help)

	return m.centerContent(modalStyle.Render(b.String()))
}

// viewConfirmMerge renders the merge confirmation dialog
func (m Model) viewConfirmMerge() string {
	var b strings.Builder

	t, ok := m.tasks.Get(m.mergingTaskID)
	if !ok {
		return m.viewDashboard()
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")). // blue
		Render("Merge Branch?")
	b.WriteString(title)
	b.WriteString("\n\n")

	b.WriteString(fmt.Sprintf("Merge branch '%s' into main?\n\n", t.GitBranch))

	// Show diff info
	if m.mergeDiffInfo != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(colorSecondary).Render("Changes:\n"))
		// Limit diff info display
		lines := strings.Split(m.mergeDiffInfo, "\n")
		maxLines := 8
		if len(lines) > maxLines {
			for i := 0; i < maxLines-1; i++ {
				b.WriteString(lipgloss.NewStyle().Foreground(colorSecondary).Render("  " + lines[i] + "\n"))
			}
			b.WriteString(lipgloss.NewStyle().Foreground(colorSecondary).Render(fmt.Sprintf("  ... and %d more lines\n", len(lines)-maxLines+1)))
		} else {
			for _, line := range lines {
				b.WriteString(lipgloss.NewStyle().Foreground(colorSecondary).Render("  " + line + "\n"))
			}
		}
	}

	b.WriteString("\n")
	help := helpStyle.Render("[y/enter]merge  [n]o  [esc]cancel")
	b.WriteString(help)

	return m.centerContent(modalStyle.Render(b.String()))
}

// viewSettings renders the settings popup
func (m Model) viewSettings() string {
	var b strings.Builder

	title := titleStyle.Render("Settings")
	b.WriteString(title)
	b.WriteString("\n\n")

	// Checkbox style helpers
	checkbox := func(checked bool) string {
		if checked {
			return "[x]"
		}
		return "[ ]"
	}

	renderSetting := func(index int, checked bool, label, description string) {
		settingLabel := fmt.Sprintf("%s %s", checkbox(checked), label)
		if m.settingsSelected == index {
			settingLabel = selectedRowStyle.Render(settingLabel)
		}
		b.WriteString(settingLabel)
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorSecondary).Render("    " + description))
		b.WriteString("\n\n")
	}

	renderMultiOption := func(index int, label, description string, options []string, currentIdx int) {
		// Build option display: [Ask] Delete Keep
		var optDisplay strings.Builder
		for i, opt := range options {
			if i == currentIdx {
				optDisplay.WriteString(fmt.Sprintf("[%s] ", opt))
			} else {
				optDisplay.WriteString(fmt.Sprintf("%s ", opt))
			}
		}
		settingLabel := fmt.Sprintf("%s: %s", label, optDisplay.String())
		if m.settingsSelected == index {
			settingLabel = selectedRowStyle.Render(settingLabel)
		}
		b.WriteString(settingLabel)
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorSecondary).Render("    " + description))
		b.WriteString("\n\n")
	}

	// Setting 0: Notifications
	renderSetting(0, m.config.NotificationsEnabled, "Notifications", "Show status updates in the messages panel")

	// Setting 1: Auto-start tasks
	renderSetting(1, m.config.AutoStartTasks, "Auto-start tasks", "Automatically start tasks when created")

	// Setting 2: Confirm before delete
	renderSetting(2, m.config.ConfirmBeforeDelete, "Confirm before delete", "Show confirmation dialog when deleting tasks")

	// Setting 3: Worktree cleanup
	cleanupOptions := []string{"Ask", "Delete", "Keep"}
	cleanupIdx := 0
	switch m.config.Worktrees.Cleanup {
	case config.WorktreeCleanupAsk:
		cleanupIdx = 0
	case config.WorktreeCleanupDelete:
		cleanupIdx = 1
	case config.WorktreeCleanupKeep:
		cleanupIdx = 2
	}
	renderMultiOption(3, "Worktree cleanup", "How to handle worktrees when deleting tasks", cleanupOptions, cleanupIdx)

	help := helpStyle.Render("[j/k]navigate  [enter/space]toggle  [esc/S]close")
	b.WriteString(help)

	return m.centerContent(modalStyle.Render(b.String()))
}

// truncate truncates a string to the given length
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// wrapText wraps text to fit within the given width, returning wrapped lines
func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}

	var result []string
	lines := strings.Split(text, "\n")

	for _, line := range lines {
		if len(line) <= width {
			result = append(result, line)
			continue
		}

		// Wrap long lines
		for len(line) > width {
			// Try to find a space to break at
			breakPoint := width
			for i := width; i > 0; i-- {
				if line[i-1] == ' ' {
					breakPoint = i
					break
				}
			}
			// If no space found, break at width
			if breakPoint == width && line[width-1] != ' ' {
				// Check if there's a space before width
				spaceFound := false
				for i := width - 1; i > width/2; i-- {
					if line[i] == ' ' {
						breakPoint = i + 1
						spaceFound = true
						break
					}
				}
				if !spaceFound {
					breakPoint = width
				}
			}

			result = append(result, strings.TrimRight(line[:breakPoint], " "))
			line = strings.TrimLeft(line[breakPoint:], " ")
		}
		if len(line) > 0 {
			result = append(result, line)
		}
	}

	return result
}

// renderPanel renders a panel with title and border
func (m Model) renderPanel(title, content string, width, height int, active bool) string {
	// Choose style based on active state
	var boxStyle lipgloss.Style
	var titleStyleToUse lipgloss.Style

	if active {
		boxStyle = activeBoxStyle
		titleStyleToUse = activePanelTitleStyle
	} else {
		boxStyle = inactiveBoxStyle
		titleStyleToUse = panelTitleStyle
	}

	// Configure box dimensions
	// Width: total panel width minus borders (2)
	// Height: total panel height minus borders (2) minus padding (2 top + 2 bottom = 4)
	// Lipgloss Width/Height set the content area size
	contentWidth := width - 2   // Subtract border width
	contentHeight := height - 4 // Subtract border (2) + padding (2)
	if contentWidth < 10 {
		contentWidth = 10
	}
	if contentHeight < 1 {
		contentHeight = 1
	}

	boxStyle = boxStyle.
		Width(contentWidth).
		Height(contentHeight).
		Padding(1, 2) // Vertical and horizontal padding

	// Render title with styling
	styledTitle := titleStyleToUse.Render(title)

	// Create bordered box with content
	box := boxStyle.Render(content)

	// Overlay title on top border, offset from the corner
	lines := strings.Split(box, "\n")
	if len(lines) > 0 {
		firstLine := lines[0]

		// Find the corner character (╭) which marks the start of the visible border
		cornerIdx := strings.Index(firstLine, "╭")
		if cornerIdx >= 0 {
			// Get the border color code to restore after title
			var borderColorCode string
			if active {
				borderColorCode = "\x1b[38;5;39m" // Color 39 (blue) for active
			} else {
				borderColorCode = "\x1b[38;5;245m" // Color 245 (gray) for inactive
			}

			// Build the title section: "─ Title ─" with color restore after
			titleSection := "─ " + styledTitle + " " + borderColorCode + "─"

			// Calculate where to insert (in bytes, after the corner + 1 dash)
			// ╭ is 3 bytes, ─ is 3 bytes
			insertStart := cornerIdx + 3 + 3 // After "╭─"

			// Calculate how many dash bytes to replace
			// Each dash "─" is 3 bytes in UTF-8
			numDashesToRemove := len(title) + 4 // 1 + 1 + title + 1 + 1 visible chars
			insertEnd := insertStart + (numDashesToRemove * 3)

			if insertEnd < len(firstLine) {
				lines[0] = firstLine[:insertStart] + titleSection + firstLine[insertEnd:]
			}
		}
	}

	return strings.Join(lines, "\n")
}

// renderTasksPanel renders the tasks panel with task list
func (m Model) renderTasksPanel(width, height int) string {
	var b strings.Builder

	tasks := m.tasks.List()

	// Calculate content width (subtract borders 2 + horizontal padding 4 = 6)
	contentWidth := width - 6
	if contentWidth < 20 {
		contentWidth = 20
	}

	// Calculate dynamic column widths based on available content width
	// Fixed columns: ID (4), Status (12 with spinner), Branch (12), Age (6) = 34 fixed
	// Variable columns: Name, Directory share remaining space
	fixedWidth := 4 + 12 + 12 + 6 + 4 // +4 for spacing between columns
	variableWidth := contentWidth - fixedWidth
	if variableWidth < 20 {
		variableWidth = 20
	}
	nameWidth := variableWidth * 3 / 5
	dirWidth := variableWidth - nameWidth

	if len(tasks) == 0 {
		b.WriteString("No tasks yet. Press 'n' to create one.\n")
	} else {
		// Header with dynamic widths
		headerFmt := fmt.Sprintf("%%-%ds %%-%ds %%-%ds %%-%ds %%-%ds %%-%ds", 4, nameWidth, 12, 12, dirWidth, 6)
		header := fmt.Sprintf(headerFmt, "#", "Task", "Status", "Branch", "Directory", "Age")
		b.WriteString(tableHeaderStyle.Render(header))
		b.WriteString("\n")

		// Calculate available lines for task rows
		// height - 4 (borders + padding) - 1 (header) - 2 (stats + spacing)
		availableLines := height - 7
		if availableLines < 3 {
			availableLines = 3
		}

		// Determine visible range for scrolling
		startIdx := 0
		endIdx := len(tasks)
		if len(tasks) > availableLines {
			// Center the selected item in the visible range
			halfVisible := availableLines / 2
			startIdx = m.selected - halfVisible
			if startIdx < 0 {
				startIdx = 0
			}
			endIdx = startIdx + availableLines
			if endIdx > len(tasks) {
				endIdx = len(tasks)
				startIdx = endIdx - availableLines
				if startIdx < 0 {
					startIdx = 0
				}
			}
		}

		// Rows
		for i := startIdx; i < endIdx; i++ {
			t := tasks[i]
			// Show spinner next to WORKING status
			statusWidth := 12
			var statusDisplay string
			if t.Status == task.StatusWorking {
				statusDisplay = m.spinner.View() + " " + StatusStyle(string(t.Status)).Render(string(t.Status))
			} else {
				statusDisplay = "  " + StatusStyle(string(t.Status)).Render(string(t.Status))
			}
			// Pad status to fixed width based on visual width (ANSI codes don't count)
			statusVisualWidth := lipgloss.Width(statusDisplay)
			if statusVisualWidth < statusWidth {
				statusDisplay = statusDisplay + strings.Repeat(" ", statusWidth-statusVisualWidth)
			}

			// Show directory (use basename for brevity)
			dir := t.Cwd
			if dir == "" {
				dir = "."
			} else {
				dir = filepath.Base(dir)
			}

			// Show branch (empty if no worktree)
			branch := t.GitBranch

			// Build row with fixed-width columns using proper padding
			idCol := fmt.Sprintf("%-4s", t.ID)
			nameCol := fmt.Sprintf("%-*s", nameWidth, truncate(t.Name, nameWidth))
			branchCol := fmt.Sprintf("%-12s", truncate(branch, 12))
			dirCol := fmt.Sprintf("%-*s", dirWidth, truncate(dir, dirWidth))
			ageCol := fmt.Sprintf("%-6s", t.AgeString())

			row := idCol + " " + nameCol + " " + statusDisplay + " " + branchCol + " " + dirCol + " " + ageCol

			if i == m.selected {
				row = selectedRowStyle.Render(row)
			}
			b.WriteString(row)
			b.WriteString("\n")
		}

		// Show scroll indicator if needed
		if len(tasks) > availableLines {
			scrollInfo := fmt.Sprintf("(%d-%d of %d)", startIdx+1, endIdx, len(tasks))
			b.WriteString(lipgloss.NewStyle().Foreground(colorSecondary).Render(scrollInfo))
			b.WriteString("\n")
		}
	}

	// Stats
	stats := fmt.Sprintf("Tasks: %d | Active: %d | Waiting: %d",
		m.tasks.Count(),
		m.tasks.ActiveCount(),
		m.tasks.WaitingCount(),
	)
	b.WriteString(lipgloss.NewStyle().Foreground(colorSecondary).Render(stats))

	return m.renderPanel("Tasks", b.String(), width, height, true)
}

// renderStatusPanel renders the status panel
func (m Model) renderStatusPanel(width, height int) string {
	var b strings.Builder

	// Calculate content dimensions (borders 2 + horizontal padding 4 = 6)
	contentWidth := width - 6
	if contentWidth < 20 {
		contentWidth = 20
	}
	// Available lines = panel height - borders (2) - vertical padding (2)
	availableLines := height - 4
	if availableLines < 1 {
		availableLines = 1
	}

	if len(m.messages) == 0 && m.err == nil {
		b.WriteString(lipgloss.NewStyle().Foreground(colorSecondary).Render("No recent status updates"))
	} else {
		lineCount := 0
		// Show error if present
		if m.err != nil && lineCount < availableLines {
			errText := fmt.Sprintf("Error: %v", m.err)
			if len(errText) > contentWidth {
				errText = errText[:contentWidth-3] + "..."
			}
			errLine := lipgloss.NewStyle().Foreground(colorError).Render(errText)
			b.WriteString(errLine)
			b.WriteString("\n")
			lineCount++
		}
		// Show recent messages (limit to available lines)
		for _, msg := range m.messages {
			if lineCount >= availableLines {
				break
			}
			timestamp := msg.Timestamp.Format("15:04:05")
			msgText := fmt.Sprintf("[%s] %s", timestamp, msg.Text)
			if len(msgText) > contentWidth {
				msgText = msgText[:contentWidth-3] + "..."
			}
			var line string
			if msg.IsError {
				line = lipgloss.NewStyle().Foreground(colorError).Render(msgText)
			} else {
				line = lipgloss.NewStyle().Foreground(colorSecondary).Render(msgText)
			}
			b.WriteString(line)
			b.WriteString("\n")
			lineCount++
		}
	}

	return m.renderPanel("Status", b.String(), width, height, false)
}

// renderPromptPanel renders the prompt panel showing the selected task's .md file content
func (m Model) renderPromptPanel(width, height int) string {
	var b strings.Builder

	// Calculate content dimensions (borders 2 + horizontal padding 4 = 6)
	contentWidth := width - 6
	if contentWidth < 10 {
		contentWidth = 10
	}
	// Available lines = panel height - borders (2) - vertical padding (2)
	availableLines := height - 4
	if availableLines < 1 {
		availableLines = 1
	}

	tasks := m.tasks.List()
	if len(tasks) == 0 || m.selected >= len(tasks) {
		b.WriteString(lipgloss.NewStyle().Foreground(colorSecondary).Render("No task selected"))
		return m.renderPanel("Prompt", b.String(), width, height, false)
	}

	t := tasks[m.selected]
	promptFile := t.PromptFile

	if promptFile == "" {
		// Legacy task with inline prompt
		if t.Prompt != "" {
			// Wrap legacy prompt to fit content width
			lines := wrapText(t.Prompt, contentWidth)
			if len(lines) > availableLines {
				lines = lines[:availableLines-1]
				lines = append(lines, lipgloss.NewStyle().Foreground(colorSecondary).Render("... (truncated)"))
			}
			b.WriteString(strings.Join(lines, "\n"))
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(colorSecondary).Render("No prompt file"))
		}
		return m.renderPanel("Prompt", b.String(), width, height, false)
	}

	// Read the prompt file
	content, err := os.ReadFile(promptFile)
	if err != nil {
		b.WriteString(lipgloss.NewStyle().Foreground(colorError).Render(fmt.Sprintf("Error reading prompt: %v", err)))
		return m.renderPanel("Prompt", b.String(), width, height, false)
	}

	// Use cached glamour renderer
	if m.glamourRenderer == nil {
		// Fallback to plain text wrapping if glamour fails
		lines := wrapText(string(content), contentWidth)
		if len(lines) > availableLines {
			lines = lines[:availableLines-1]
			lines = append(lines, lipgloss.NewStyle().Foreground(colorSecondary).Render("... (truncated)"))
		}
		b.WriteString(strings.Join(lines, "\n"))
		return m.renderPanel("Prompt", b.String(), width, height, false)
	}

	rendered, err := m.glamourRenderer.Render(string(content))
	if err != nil {
		// Fallback to plain text wrapping if rendering fails
		lines := wrapText(string(content), contentWidth)
		if len(lines) > availableLines {
			lines = lines[:availableLines-1]
			lines = append(lines, lipgloss.NewStyle().Foreground(colorSecondary).Render("... (truncated)"))
		}
		b.WriteString(strings.Join(lines, "\n"))
		return m.renderPanel("Prompt", b.String(), width, height, false)
	}

	// Trim trailing whitespace/newlines from glamour output
	rendered = strings.TrimRight(rendered, "\n ")

	// Truncate to available lines if needed
	lines := strings.Split(rendered, "\n")
	if len(lines) > availableLines {
		lines = lines[:availableLines-1]
		lines = append(lines, lipgloss.NewStyle().Foreground(colorSecondary).Render("... (truncated)"))
	}

	b.WriteString(strings.Join(lines, "\n"))

	return m.renderPanel("Prompt", b.String(), width, height, false)
}

// centerContent centers the content both horizontally and vertically
func (m Model) centerContent(content string) string {
	// Get content dimensions
	contentWidth := lipgloss.Width(content)
	contentHeight := lipgloss.Height(content)

	// Calculate padding for centering
	horizontalPadding := 0
	verticalPadding := 0

	if m.width > contentWidth {
		horizontalPadding = (m.width - contentWidth) / 2
	}
	if m.height > contentHeight {
		verticalPadding = (m.height - contentHeight) / 2
	}

	// Apply centering
	return lipgloss.NewStyle().
		PaddingLeft(horizontalPadding).
		PaddingTop(verticalPadding).
		Render(content)
}

// getTaskWorktreeInfos converts task list to the interface needed by git.Assigner
func (m Model) getTaskWorktreeInfos() []git.TaskWorktreeInfo {
	tasks := m.tasks.List()
	infos := make([]git.TaskWorktreeInfo, len(tasks))
	for i, t := range tasks {
		infos[i] = t
	}
	return infos
}
