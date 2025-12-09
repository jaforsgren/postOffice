package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("63")).
			Foreground(lipgloss.Color("6")).
			Background(lipgloss.Color("0")).
			Padding(0, 1)

	commandBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7")).
			Background(lipgloss.Color("0")).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("63"))
		// PaddingTop(2)

	selectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color("6")).
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("63")).
				Bold(true)

	normalItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7"))

	folderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("4")).
			Bold(true)

	requestStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("2"))

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("6")).
		// BorderStyle(lipgloss.NormalBorder()).
		// BorderForeground(lipgloss.Color("#001100")).
		Padding(0, 1)

	mainWindowStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("81")).
			Padding(1, 2)
)
