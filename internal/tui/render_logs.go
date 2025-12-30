package tui

import (
	"postOffice/internal/logger"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderLogsView() string {
	logs := logger.GetLogs()

	var content string
	if len(logs) == 0 {
		content = "No logs available for this session."
	} else {
		content = strings.Join(logs, "\n")
	}

	m.logsViewport.SetContent(content)

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	title := titleStyle.Render("Session Logs")
	help := helpStyle.Render("(j/k to scroll, esc to close)")

	header := title + " " + help

	viewportContent := m.logsViewport.View()

	fullContent := header + "\n\n" + viewportContent

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("33")).
		Height(m.height-10).
		Width(m.width-4).
		Padding(1, 2).
		Render(fullContent)
}
