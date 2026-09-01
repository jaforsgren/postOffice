package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderHistoryView() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))

	title := titleStyle.Render(fmt.Sprintf("History: %s", m.historyItem.Name))
	help := helpStyle.Render("(enter: rerun | v: view | d: delete | esc: back)")
	header := title + " " + help + "\n\n"

	var lines []string
	if len(m.history) == 0 {
		lines = append(lines, helpStyle.Render("No execution history for this request."))
	} else {
		for i, entry := range m.history {
			statusStyle := okStyle
			if entry.StatusCode < 200 || entry.StatusCode >= 300 {
				statusStyle = errStyle
			}
			ts := entry.Timestamp.Format("2006-01-02 15:04:05")
			status := statusStyle.Render(entry.Status)
			duration := helpStyle.Render(fmt.Sprintf("(%dms)", entry.DurationMS))
			line := fmt.Sprintf("  %s  %s  %s  %s", ts, entry.RequestMethod, status, duration)
			if i == m.historyCursor {
				line = selectedStyle.Render("▶ ") + line
			} else {
				line = "  " + line
			}
			lines = append(lines, line)
		}
	}

	content := header + strings.Join(lines, "\n")
	m.historyViewport.SetContent(content)

	return mainWindowStyle.
		Height(m.height - 8).
		Width(m.width - 4).
		Render(m.historyViewport.View())
}
