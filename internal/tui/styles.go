package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	colorPrimary   = lipgloss.Color("39")  // blue
	colorSecondary = lipgloss.Color("245") // gray
	colorSuccess   = lipgloss.Color("42")  // green
	colorWarning   = lipgloss.Color("220") // yellow
	colorError     = lipgloss.Color("196") // red

	// Status colors
	statusColors = map[string]lipgloss.Color{
		"PENDING": lipgloss.Color("245"), // gray
		"WORKING": lipgloss.Color("39"),  // blue
		"WAITING": lipgloss.Color("220"), // yellow
		"DONE":    lipgloss.Color("42"),  // green
	}

	// Base styles
	baseStyle = lipgloss.NewStyle().Padding(0, 1)

	// Main container with border
	containerStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(1, 2)

	// Title style
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			MarginBottom(1)

	// Table styles
	tableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorSecondary).
				BorderBottom(true).
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(colorSecondary)

	selectedRowStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("236")).
				Foreground(lipgloss.Color("255"))

	normalRowStyle = lipgloss.NewStyle()

	// Status badge styles
	statusStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Bold(true)

	// Help text style
	helpStyle = lipgloss.NewStyle().
			Foreground(colorSecondary).
			MarginTop(1)

	// Input styles
	inputLabelStyle = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true)

	inputStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(0, 1)

	// Modal styles
	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(1, 2)

	// Messages panel style
	messagesPanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorSecondary).
				Padding(0, 2).
				MarginTop(1)
)

// StatusStyle returns the style for a given status
func StatusStyle(status string) lipgloss.Style {
	color, ok := statusColors[status]
	if !ok {
		color = colorSecondary
	}
	return statusStyle.Foreground(color)
}
