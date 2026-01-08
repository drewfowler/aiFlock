package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dfowler/flock/internal/task"
	"github.com/dfowler/flock/internal/zellij"
)

// View modes
type viewMode int

const (
	viewDashboard viewMode = iota
	viewNewTask
)

// Model is the main TUI model
type Model struct {
	tasks         *task.Manager
	zellij        *zellij.Controller
	selected      int
	mode          viewMode
	width         int
	height        int
	statusUpdates chan StatusUpdate
	err           error

	// New task form
	nameInput   textinput.Model
	promptInput textinput.Model
	cwdInput    textinput.Model
	focusIndex  int
}

// StatusUpdate represents a status change from the watcher
type StatusUpdate struct {
	TaskID string
	Status task.Status
}

// StatusMsg is sent when a status update is received
type StatusMsg StatusUpdate

// NewModel creates a new TUI model
func NewModel(tasks *task.Manager, zj *zellij.Controller, statusChan chan StatusUpdate) Model {
	// Name input
	nameInput := textinput.New()
	nameInput.Placeholder = "Task name"
	nameInput.CharLimit = 50
	nameInput.Width = 40

	// Prompt input
	promptInput := textinput.New()
	promptInput.Placeholder = "What should Claude do?"
	promptInput.CharLimit = 500
	promptInput.Width = 60

	// CWD input
	cwdInput := textinput.New()
	cwdInput.Placeholder = "Working directory (leave empty for current)"
	cwdInput.CharLimit = 200
	cwdInput.Width = 60

	return Model{
		tasks:         tasks,
		zellij:        zj,
		statusUpdates: statusChan,
		nameInput:     nameInput,
		promptInput:   promptInput,
		cwdInput:      cwdInput,
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		waitForStatus(m.statusUpdates),
	)
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

	case StatusMsg:
		// Update task status
		if err := m.tasks.UpdateStatus(msg.TaskID, msg.Status); err != nil {
			m.err = err
		}
		// Continue listening for updates
		return m, waitForStatus(m.statusUpdates)

	case tea.KeyMsg:
		switch m.mode {
		case viewDashboard:
			return m.updateDashboard(msg)
		case viewNewTask:
			return m.updateNewTask(msg)
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

	case "s":
		// Start selected task
		if len(tasks) > 0 && m.selected < len(tasks) {
			t := tasks[m.selected]
			if t.Status == task.StatusPending {
				cwd := t.Cwd
				if cwd == "" {
					cwd = "."
				}
				if err := m.zellij.NewTab(t.ID, t.TabName, t.Prompt, cwd); err != nil {
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
		// Delete selected task
		if len(tasks) > 0 && m.selected < len(tasks) {
			t := tasks[m.selected]
			if err := m.tasks.Delete(t.ID); err != nil {
				m.err = err
			}
			if m.selected >= len(m.tasks.List()) && m.selected > 0 {
				m.selected--
			}
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
		m.promptInput.Reset()
		m.cwdInput.Reset()
		return m, nil

	case "tab", "shift+tab", "down", "up":
		// Cycle focus
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
		m.promptInput.Blur()
		m.cwdInput.Blur()

		switch m.focusIndex {
		case 0:
			m.nameInput.Focus()
		case 1:
			m.promptInput.Focus()
		case 2:
			m.cwdInput.Focus()
		}

		return m, textinput.Blink

	case "enter":
		// Create task if name and prompt are filled
		name := strings.TrimSpace(m.nameInput.Value())
		prompt := strings.TrimSpace(m.promptInput.Value())
		cwd := strings.TrimSpace(m.cwdInput.Value())

		if name != "" && prompt != "" {
			if _, err := m.tasks.Create(name, prompt, cwd); err != nil {
				m.err = err
			}

			m.mode = viewDashboard
			m.nameInput.Reset()
			m.promptInput.Reset()
			m.cwdInput.Reset()
			m.selected = m.tasks.Count() - 1
		}
		return m, nil
	}

	// Update focused input
	var cmd tea.Cmd
	switch m.focusIndex {
	case 0:
		m.nameInput, cmd = m.nameInput.Update(msg)
	case 1:
		m.promptInput, cmd = m.promptInput.Update(msg)
	case 2:
		m.cwdInput, cmd = m.cwdInput.Update(msg)
	}

	return m, cmd
}

// View renders the UI
func (m Model) View() string {
	switch m.mode {
	case viewNewTask:
		return m.viewNewTask()
	default:
		return m.viewDashboard()
	}
}

// viewDashboard renders the main dashboard
func (m Model) viewDashboard() string {
	var b strings.Builder

	// Title
	title := titleStyle.Render("flock - AI Agent Controller")
	b.WriteString(title)
	b.WriteString("\n\n")

	// Task table
	tasks := m.tasks.List()
	if len(tasks) == 0 {
		b.WriteString("  No tasks yet. Press 'n' to create one.\n")
	} else {
		// Header
		header := fmt.Sprintf("  %-4s %-30s %-10s %-12s %-6s", "#", "Task", "Status", "Tab", "Age")
		b.WriteString(tableHeaderStyle.Render(header))
		b.WriteString("\n")

		// Rows
		for i, t := range tasks {
			status := StatusStyle(string(t.Status)).Render(string(t.Status))
			tab := t.TabName
			if t.Status == task.StatusPending {
				tab = "--"
			}

			row := fmt.Sprintf("  %-4s %-30s %-10s %-12s %-6s",
				t.ID,
				truncate(t.Name, 30),
				status,
				tab,
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
	stats := fmt.Sprintf("  Tasks: %d | Active: %d | Waiting: %d",
		m.tasks.Count(),
		m.tasks.ActiveCount(),
		m.tasks.WaitingCount(),
	)
	b.WriteString(lipgloss.NewStyle().Foreground(colorSecondary).Render(stats))
	b.WriteString("\n")

	// Error
	if m.err != nil {
		errMsg := lipgloss.NewStyle().Foreground(colorError).Render(fmt.Sprintf("Error: %v", m.err))
		b.WriteString("\n" + errMsg + "\n")
	}

	// Help
	help := helpStyle.Render("[n]ew  [s]tart  [j/k]navigate  [enter]jump  [d]elete  [q]uit")
	b.WriteString("\n" + help)

	return baseStyle.Render(b.String())
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

	b.WriteString(inputLabelStyle.Render("Prompt:"))
	b.WriteString("\n")
	b.WriteString(m.promptInput.View())
	b.WriteString("\n\n")

	b.WriteString(inputLabelStyle.Render("Working Directory:"))
	b.WriteString("\n")
	b.WriteString(m.cwdInput.View())
	b.WriteString("\n\n")

	help := helpStyle.Render("[tab]next field  [enter]create  [esc]cancel")
	b.WriteString(help)

	return modalStyle.Render(b.String())
}

// truncate truncates a string to the given length
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
