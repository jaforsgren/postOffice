package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle = lipgloss.NewStyle().
			Bold(false).
		// BorderStyle(lipgloss.NormalBorder()).
		// BorderForeground(lipgloss.Color("63")).
		Foreground(lipgloss.Color("6")).
		Background(lipgloss.Color("#000000")).
		Padding(1, 1)

	commandBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7")).
			Background(lipgloss.Color("#000000")).
			Padding(0, 2)

	selectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("1")).
				Background(lipgloss.Color("#000000")).
				Bold(true)

	normalItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7"))

	folderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("4")).
			Bold(false)

	requestStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("2"))

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("#000000")).
			Padding(0, 1)

	mainWindowStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("81")).
			Padding(1, 2)
)
