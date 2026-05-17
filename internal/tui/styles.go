package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205"))

	activeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("46")) // green

	inactiveStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")) // dim

	cursorStyle = lipgloss.NewStyle().
			Reverse(true)

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(0, 1)

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))
)
