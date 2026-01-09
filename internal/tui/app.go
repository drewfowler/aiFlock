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
	"github.com/charmbracelet/lipgloss"
	"github.com/dfowler/flock/internal/config"
	"github.com/dfowler/flock/internal/prompt"
	"github.com/dfowler/flock/internal/task"
	"github.com/dfowler/flock/internal/zellij"
)

// View modes
type viewMode int

const (
	viewDashboard viewMode = iota
	viewNewTask
	viewEditTask
	viewConfirmDelete
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
	selected      int
	mode          viewMode
	width         int
	height        int
	statusUpdates chan StatusUpdate
	err           error

	// New task form (name and cwd only - prompt is edited in external editor)
	nameInput  textinput.Model
	cwdInput   textinput.Model
	focusIndex int

	// Edit task tracking
	editingTaskID string

	// Delete confirmation tracking
	deletingTaskID string

	// Spinner for working status
	spinner spinner.Model

	// Status messages for the messages panel
	messages []Message
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

// NewModel creates a new TUI model
func NewModel(tasks *task.Manager, zj *zellij.Controller, cfg *config.Config, statusChan chan StatusUpdate) Model {
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

	// Spinner for working status
	s := spinner.New()
	s.Spinner = spinner.Spinner{
		Frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		FPS:    time.Millisecond * 100,
	}
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("39")) // blue

	return Model{
		tasks:         tasks,
		zellij:        zj,
		config:        cfg,
		promptMgr:     prompt.NewManager(cfg),
		statusUpdates: statusChan,
		nameInput:     nameInput,
		cwdInput:      cwdInput,
		spinner:       s,
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
			} else if oldStatus != msg.Status {
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
			// Create the task with the prompt file
			if _, err := m.tasks.Create(msg.taskName, msg.promptFile, msg.cwd); err != nil {
				m.err = err
				m.addMessage(fmt.Sprintf("Failed to create task: %v", err), true)
			} else {
				m.addMessage(fmt.Sprintf("Created task: %s", msg.taskName), false)
				m.selected = m.tasks.Count() - 1
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
				cwd := t.Cwd
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
		// Show delete confirmation
		if len(tasks) > 0 && m.selected < len(tasks) {
			t := tasks[m.selected]
			m.deletingTaskID = t.ID
			m.mode = viewConfirmDelete
		}
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

	case "enter":
		// Open editor if name is filled
		name := strings.TrimSpace(m.nameInput.Value())
		cwd := strings.TrimSpace(m.cwdInput.Value())

		if name != "" {
			// Reset inputs now
			m.nameInput.Reset()
			m.cwdInput.Reset()

			// Get next task ID and create prompt file
			taskID := m.tasks.NextID()
			if cwd == "" {
				cwd = "."
			}

			// Create prompt file from template
			promptFile, err := m.promptMgr.CreatePromptFile(taskID, name, cwd)
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

// openEditor returns a command that opens the editor and sends editorFinishedMsg when done
func (m Model) openEditor(taskName, promptFile, cwd string) tea.Cmd {
	editor := getEditor()
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
	c := exec.Command(editor, promptFile)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editFinishedMsg{err: err}
	})
}

// updateConfirmDelete handles delete confirmation input
func (m Model) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		// Confirm deletion
		if t, ok := m.tasks.Get(m.deletingTaskID); ok {
			// Close the zellij tab if task was started
			if t.Status != task.StatusPending && t.TabName != "" {
				if err := m.zellij.CloseTab(t.TabName); err != nil {
					m.err = err
				}
				m.zellij.GoToController()
			}
			// Delete the status file to prevent stale updates
			m.zellij.DeleteStatusFile(m.deletingTaskID)
			// Delete the prompt file
			m.promptMgr.DeletePromptFile(m.deletingTaskID)
			if err := m.tasks.Delete(m.deletingTaskID); err != nil {
				m.err = err
			}
			if m.selected >= len(m.tasks.List()) && m.selected > 0 {
				m.selected--
			}
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

// View renders the UI
func (m Model) View() string {
	switch m.mode {
	case viewNewTask:
		return m.viewNewTask()
	case viewEditTask:
		return m.viewEditTask()
	case viewConfirmDelete:
		return m.viewConfirmDelete()
	default:
		return m.viewDashboard()
	}
}

// viewDashboard renders the main dashboard
func (m Model) viewDashboard() string {
	// Calculate responsive dimensions
	availableWidth := m.width - 4 // Account for centering padding
	availableHeight := m.height - 2

	// Minimum dimensions
	if availableWidth < 60 {
		availableWidth = 60
	}
	if availableHeight < 20 {
		availableHeight = 20
	}

	// Panel heights - tasks panel takes most space, messages panel is smaller
	tasksHeight := availableHeight - 12 // Leave room for messages panel
	if tasksHeight < 10 {
		tasksHeight = 10
	}
	messagesHeight := 8

	// Render panels
	tasksPanel := m.renderTasksPanel(availableWidth-4, tasksHeight)
	messagesPanel := m.renderMessagesPanel(availableWidth-4, messagesHeight)

	// Help bar
	helpBar := helpStyle.Render("[n]ew  [e]dit  [s]tart  [j/k]navigate  [enter]jump  [d]elete  [q]uit")

	// Compose layout
	content := lipgloss.JoinVertical(lipgloss.Left, tasksPanel, messagesPanel, helpBar)
	return m.centerContent(content)
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

	b.WriteString(lipgloss.NewStyle().Foreground(colorSecondary).Render("Press Enter to open editor for task prompt..."))
	b.WriteString("\n\n")

	help := helpStyle.Render("[tab]next field  [enter]open editor  [esc]cancel")
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

	help := helpStyle.Render("[tab]next field  [enter]open editor  [esc]cancel")
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
	help := helpStyle.Render("[y]es  [n]o  [esc]cancel")
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

	// Configure box dimensions and padding
	boxStyle = boxStyle.
		Width(width).
		Height(height).
		Padding(1, 2)

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
	if len(tasks) == 0 {
		b.WriteString("No tasks yet. Press 'n' to create one.\n")
	} else {
		// Header
		header := fmt.Sprintf("%-4s %-25s %-10s %-20s %-6s", "#", "Task", "Status", "Directory", "Age")
		b.WriteString(tableHeaderStyle.Render(header))
		b.WriteString("\n")

		// Rows
		for i, t := range tasks {
			// Show spinner next to WORKING status
			var statusDisplay string
			if t.Status == task.StatusWorking {
				statusDisplay = m.spinner.View() + " " + StatusStyle(string(t.Status)).Render(string(t.Status))
			} else {
				statusDisplay = "  " + StatusStyle(string(t.Status)).Render(string(t.Status))
			}

			// Show directory (use basename for brevity)
			dir := t.Cwd
			if dir == "" {
				dir = "."
			} else {
				dir = filepath.Base(dir)
			}

			row := fmt.Sprintf("%-4s %-25s %-10s %-20s %-6s",
				t.ID,
				truncate(t.Name, 25),
				statusDisplay,
				truncate(dir, 20),
				t.AgeString(),
			)

			if i == m.selected {
				row = selectedRowStyle.Render(row)
			}
			b.WriteString(row)
			b.WriteString("\n")
		}
	}

	// Stats
	b.WriteString("\n")
	stats := fmt.Sprintf("Tasks: %d | Active: %d | Waiting: %d",
		m.tasks.Count(),
		m.tasks.ActiveCount(),
		m.tasks.WaitingCount(),
	)
	b.WriteString(lipgloss.NewStyle().Foreground(colorSecondary).Render(stats))

	return m.renderPanel("Tasks", b.String(), width, height, true)
}

// renderMessagesPanel renders the messages panel
func (m Model) renderMessagesPanel(width, height int) string {
	var b strings.Builder

	if len(m.messages) == 0 && m.err == nil {
		b.WriteString(lipgloss.NewStyle().Foreground(colorSecondary).Render("No recent messages"))
	} else {
		// Show error if present
		if m.err != nil {
			errLine := lipgloss.NewStyle().Foreground(colorError).Render(fmt.Sprintf("Error: %v", m.err))
			b.WriteString(errLine)
			b.WriteString("\n")
		}
		// Show recent messages
		for _, msg := range m.messages {
			timestamp := msg.Timestamp.Format("15:04:05")
			var line string
			if msg.IsError {
				line = lipgloss.NewStyle().Foreground(colorError).Render(fmt.Sprintf("[%s] %s", timestamp, msg.Text))
			} else {
				line = lipgloss.NewStyle().Foreground(colorSecondary).Render(fmt.Sprintf("[%s] %s", timestamp, msg.Text))
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	return m.renderPanel("Messages", b.String(), width, height, false)
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
