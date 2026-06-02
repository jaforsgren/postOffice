package tui

import (
	"fmt"
	"postOffice/internal/workflow"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderWorkflowsList() string {
	metrics := m.calculateLayout()
	h := metrics.contentHeight

	var sb strings.Builder

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render("Workflows")
	sb.WriteString(title + "\n\n")

	if len(m.workflows) == 0 {
		sb.WriteString(subtleStyle.Render("No workflows found.\n"))
		sb.WriteString(subtleStyle.Render("Create one with: :wf new <id>"))
		return m.padToHeight(sb.String(), h)
	}

	for i, wf := range m.workflows {
		selected := i == m.workflowCursor
		prefix := "  "
		if selected {
			prefix = "> "
		}

		nameStyle := lipgloss.NewStyle()
		if selected {
			nameStyle = nameStyle.Bold(true).Foreground(lipgloss.Color("205"))
		}

		line := prefix + nameStyle.Render(wf.Name)
		if wf.Description != "" {
			line += "  " + subtleStyle.Render(wf.Description)
		}
		sb.WriteString(line + "\n")

		if selected && len(wf.Steps) > 0 {
			for _, step := range wf.Steps {
				sb.WriteString(subtleStyle.Render(fmt.Sprintf("    [%s] %s", step.ID, step.Request)) + "\n")
			}
		}
	}

	return m.padToHeight(sb.String(), h)
}

func (m Model) renderWorkflowRunView() string {
	return m.workflowViewport.View()
}

func (m Model) buildWorkflowRunLines() []string {
	state := m.workflowState
	var lines []string

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	statusStyle := workflowStatusStyle(state.Status)

	lines = append(lines, titleStyle.Render(state.WorkflowName)+" "+statusStyle.Render("["+state.Status+"]"))
	lines = append(lines, "")

	if len(state.Steps) == 0 {
		lines = append(lines, subtleStyle.Render("(no steps defined)"))
	} else {
		lines = append(lines, lipgloss.NewStyle().Bold(true).Render("Steps:"))
		for _, ss := range state.Steps {
			icon := stepIcon(ss.Status)
			repeatInfo := ""
			if ss.Total > 1 {
				repeatInfo = fmt.Sprintf(" (%d/%d)", ss.Progress, ss.Total)
			}
			line := fmt.Sprintf("  %s %-20s %s%s",
				icon,
				ss.ID,
				workflowStatusStyle(ss.Status).Render(ss.Status),
				subtleStyle.Render(repeatInfo),
			)
			lines = append(lines, line)
		}
	}

	if len(state.Logs) > 0 {
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Bold(true).Render("Logs:"))
		for _, log := range state.Logs {
			lines = append(lines, "  "+subtleStyle.Render(log))
		}
	}

	if state.Error != "" {
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("Error: "+state.Error))
	}

	return lines
}

func workflowStatusStyle(status string) lipgloss.Style {
	switch status {
	case workflow.StatusSuccess:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	case workflow.StatusFailed:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	case workflow.StatusRunning:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	case workflow.StatusSkipped:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	case workflow.StatusStopped:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	}
}

func stepIcon(status string) string {
	switch status {
	case workflow.StatusSuccess:
		return "✓"
	case workflow.StatusFailed:
		return "✗"
	case workflow.StatusRunning:
		return "▶"
	case workflow.StatusSkipped:
		return "−"
	case workflow.StatusStopped:
		return "■"
	default:
		return "○"
	}
}

func (m Model) padToHeight(content string, height int) string {
	lines := strings.Split(content, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
