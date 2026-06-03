package tui

import (
	"fmt"
	"postOffice/internal/workflow"
	"strings"
	"time"

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

func (m Model) renderWorkflowDetail() string {
	metrics := m.calculateLayout()
	h := metrics.contentHeight

	wf := m.activeWorkflow
	if wf == nil {
		return mainWindowStyle.
			Height(h).
			Width(m.width - 4).
			Render(subtleStyle.Render("No workflow selected."))
	}

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	var headerLines []string
	headerLines = append(headerLines, headerStyle.Render(wf.Name))
	if wf.Description != "" {
		headerLines = append(headerLines, subtleStyle.Render(wf.Description))
	}
	meta := fmt.Sprintf("Steps: %d", len(wf.Steps))
	if wf.Version > 0 {
		meta += fmt.Sprintf("  v%d", wf.Version)
	}
	headerLines = append(headerLines, dimStyle.Render(meta))
	headerLines = append(headerLines, "")

	if len(wf.Steps) == 0 {
		content := strings.Join(headerLines, "\n") + subtleStyle.Render("No steps defined.")
		return mainWindowStyle.
			Height(h).
			Width(m.width - 4).
			Render(content)
	}

	visibleLines := h - len(headerLines) - 1
	startIdx, endIdx := calculateVisibleWindow(m.workflowStepCursor, len(wf.Steps), visibleLines)

	runningIndicator := ""
	if m.workflowChan != nil {
		runningIndicator = " " + workflowStatusStyle(workflow.StatusRunning).Render("● running")
	} else if m.workflowState.WorkflowID == wf.ID && m.workflowState.Status != "" {
		runningIndicator = " " + workflowStatusStyle(m.workflowState.Status).Render("["+m.workflowState.Status+"]")
	}
	if runningIndicator != "" {
		headerLines[0] = headerLines[0] + runningIndicator
	}

	lines := append([]string{}, headerLines...)

	for i := startIdx; i < endIdx; i++ {
		step := wf.Steps[i]
		selected := i == m.workflowStepCursor

		cursor := "  "
		if selected {
			cursor = "> "
		}

		num := dimStyle.Render(fmt.Sprintf("[%d]", i+1))
		stepIDStr := fmt.Sprintf("%-20s", step.ID)
		requestStr := step.Request
		if len(requestStr) > 40 {
			requestStr = requestStr[:37] + "..."
		}
		requestStr = fmt.Sprintf("%-40s", requestStr)

		var annotations []string
		if step.PreScript != "" {
			annotations = append(annotations, "[pre]")
		}
		if step.PostScript != "" {
			annotations = append(annotations, "[post]")
		}
		annotationStr := ""
		if len(annotations) > 0 {
			annotationStr = " " + dimStyle.Render(strings.Join(annotations, " "))
		}

		// Step run state icon (from current/last workflow execution)
		runStateStr := ""
		if m.workflowState.StepIndex != nil {
			if ss, ok := m.workflowState.StepIndex[step.ID]; ok && ss.Status != workflow.StatusPending {
				icon := stepIcon(ss.Status)
				runStateStr = "  " + workflowStatusStyle(ss.Status).Render(icon+" "+ss.Status)
				if ss.Total > 1 {
					runStateStr += dimStyle.Render(fmt.Sprintf(" (%d/%d)", ss.Progress, ss.Total))
				}
			}
		}

		// HTTP execution info (from requestExecutions, shown when no active run state)
		executionInfo := ""
		if runStateStr == "" && m.collection != nil {
			itemID := m.collection.Info.Name + "/" + step.Request
			if exec, exists := m.requestExecutions[itemID]; exists {
				statusColor := "8"
				if strings.HasPrefix(exec.Status, "2") {
					statusColor = "10"
				} else if strings.HasPrefix(exec.Status, "3") {
					statusColor = "11"
				} else if strings.HasPrefix(exec.Status, "4") {
					statusColor = "9"
				} else if strings.HasPrefix(exec.Status, "5") {
					statusColor = "1"
				}
				statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor))
				timeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
				executionInfo = "  " + statusStyle.Render(exec.Status) + " " + timeStyle.Render(formatTimeAgo(time.Since(exec.Timestamp)))
			}
		}

		if selected {
			fullLine := cursor + num + " " +
				lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render(stepIDStr) +
				"  " + subtleStyle.Render(requestStr) +
				annotationStr + runStateStr + executionInfo
			lines = append(lines, selectedItemStyle.Render(fullLine))
		} else {
			fullLine := cursor + num + " " + normalItemStyle.Render(stepIDStr) +
				"  " + subtleStyle.Render(requestStr) +
				annotationStr + runStateStr + executionInfo
			lines = append(lines, normalItemStyle.Render(fullLine))
		}
	}

	content := strings.Join(lines, "\n")
	return mainWindowStyle.
		Height(h).
		Width(m.width - 4).
		Render(content)
}

func (m Model) renderWorkflowRunView() string {
	return strings.Join(m.buildWorkflowRunLines(), "\n")
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
