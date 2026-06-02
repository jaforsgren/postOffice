package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderSavedResponsesView() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))

	title := titleStyle.Render(fmt.Sprintf("Saved Responses: %s", m.savedResponseItemID))
	help := helpStyle.Render("(enter: view | d: delete | esc: back)")
	header := title + " " + help + "\n\n"

	var lines []string
	if len(m.savedResponses) == 0 {
		lines = append(lines, helpStyle.Render("No saved responses for this request."))
	} else {
		for i, sr := range m.savedResponses {
			statusStyle := okStyle
			if sr.StatusCode < 200 || sr.StatusCode >= 300 {
				statusStyle = errStyle
			}
			ts := sr.SavedAt.Format("2006-01-02 15:04:05")
			status := statusStyle.Render(sr.Status)
			duration := helpStyle.Render(fmt.Sprintf("(%dms)", sr.DurationMS))
			line := fmt.Sprintf("  %s  %s  %s", ts, status, duration)
			if i == m.savedResponseCursor {
				line = selectedStyle.Render("▶ ") + line
			} else {
				line = "  " + line
			}
			lines = append(lines, line)
		}
	}

	content := header + strings.Join(lines, "\n")
	m.savedResponseViewport.SetContent(content)

	return mainWindowStyle.
		Height(m.height - 8).
		Width(m.width - 4).
		Render(m.savedResponseViewport.View())
}
