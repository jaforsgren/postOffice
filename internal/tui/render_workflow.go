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
	if m.workflowState.WorkflowID == wf.ID && isRunFinished(m.workflowState.Status) {
		headerLines = append(headerLines, dimStyle.Render(m.workflowState.Summary.String()))
	}
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

	headerLines[0] += m.workflowRunIndicator(wf)

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
				icon := workflow.StatusIcon(ss.Status)
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

// workflowRunIndicator is the run status suffix for wf's title, or empty when
// wf has not run.
func (m Model) workflowRunIndicator(wf *workflow.Workflow) string {
	if m.workflowChan != nil {
		return " " + workflowStatusStyle(workflow.StatusRunning).Render("● running"+iterationLabel(m.workflowState))
	}
	if m.workflowState.WorkflowID == wf.ID && m.workflowState.Status != "" {
		return " " + workflowStatusStyle(m.workflowState.Status).Render("["+m.workflowState.Status+"]")
	}
	return ""
}

func (m Model) renderWorkflowRunView() string {
	return m.workflowViewport.View()
}

// refreshWorkflowRunViewport re-renders the run view, following the newest
// output while the run is in progress so its progress stays visible.
func (m Model) refreshWorkflowRunViewport() Model {
	m.configureViewport(&m.workflowViewport, strings.Join(m.buildWorkflowRunLines(), "\n"))
	if m.workflowState.Status == workflow.StatusRunning {
		m.workflowViewport.GotoBottom()
	}
	return m
}

// runStyler decorates run report text. The TUI renders it with colours; the
// editor export uses plainRunStyler, since ANSI codes would show up as noise.
type runStyler struct {
	title   func(string) string
	heading func(string) string
	status  func(status, text string) string
	subtle  func(string) string
	failure func(string) string
}

func render(style lipgloss.Style) func(string) string {
	return func(text string) string { return style.Render(text) }
}

func identity(text string) string { return text }

var (
	styledRunStyler = runStyler{
		title:   render(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))),
		heading: render(lipgloss.NewStyle().Bold(true)),
		status:  func(status, text string) string { return workflowStatusStyle(status).Render(text) },
		subtle:  func(text string) string { return subtleStyle.Render(text) },
		failure: render(lipgloss.NewStyle().Foreground(lipgloss.Color("9"))),
	}
	plainRunStyler = runStyler{
		title:   identity,
		heading: identity,
		status:  func(_, text string) string { return text },
		subtle:  identity,
		failure: identity,
	}
)

func (m Model) buildWorkflowRunLines() []string {
	return runReportLines(m.workflowState, styledRunStyler)
}

// Indents of the run report. The editor export folds on indent, so each
// step's log lines fold under the step that produced them.
const (
	runReportStepIndent = "  "
	runReportLogIndent  = "      "
)

// runReportLines lays out a workflow run: header, one line per step (with the
// request and HTTP result once it has run) followed by that step's logs, then
// the workflow script's own logs and the run error.
func runReportLines(state workflow.ExecutionState, style runStyler) []string {
	var lines []string

	lines = append(lines, style.title(state.WorkflowName)+" "+style.status(state.Status, "["+state.Status+iterationLabel(state)+"]"))
	if isRunFinished(state.Status) {
		lines = append(lines, style.subtle(state.Summary.String()))
	}
	lines = append(lines, "")

	stepLogs, workflowLogs := groupLogsByStep(state)

	if len(state.Steps) == 0 {
		lines = append(lines, style.subtle("(no steps defined)"))
	} else {
		lines = append(lines, style.heading("Steps:"))
		for _, ss := range state.Steps {
			lines = append(lines, runReportStepLine(ss, state.IsMultiIteration(), style))
			logs := stepLogs[stepRunKeyOf(ss)]
			for _, msg := range logs {
				lines = append(lines, runReportLogIndent+styleLogMessage(msg, style))
			}
			if len(logs) > 0 {
				lines = append(lines, "")
			}
		}
	}

	if len(workflowLogs) > 0 {
		lines = append(lines, "")
		lines = append(lines, style.heading("Workflow logs:"))
		for _, msg := range workflowLogs {
			lines = append(lines, runReportStepIndent+styleLogMessage(msg, style))
		}
	}

	if state.Error != "" {
		lines = append(lines, "")
		lines = append(lines, style.failure("Error: "+state.Error))
	}

	return lines
}

// runReportStepLine is one step's report line. In a multi-iteration run the
// step is prefixed with its iteration, since each iteration lists every step.
func runReportStepLine(ss *workflow.StepState, showIteration bool, style runStyler) string {
	stepID := ss.ID
	if showIteration {
		stepID = fmt.Sprintf(iterationStepPrefixFormat, ss.Iteration) + stepID
	}
	detail := ""
	if ss.Total > 1 {
		detail = fmt.Sprintf(" (%d/%d)", ss.Progress, ss.Total)
	}
	if ss.LastResponse != nil {
		detail += fmt.Sprintf("  %s  %s  %v", ss.Request, ss.LastResponse.Status, ss.LastResponse.Duration.Round(time.Millisecond))
	}
	return fmt.Sprintf("%s%s %-20s %s%s",
		runReportStepIndent,
		workflow.StatusIcon(ss.Status),
		stepID,
		style.status(ss.Status, ss.Status),
		style.subtle(detail),
	)
}

// stepRunKey identifies one step's run in one iteration: logs group by it so
// each iteration's output stays under that iteration's step.
type stepRunKey struct {
	stepID    string
	iteration int
}

func stepRunKeyOf(ss *workflow.StepState) stepRunKey {
	return stepRunKey{stepID: ss.ID, iteration: ss.Iteration}
}

// groupLogsByStep splits the run's logs into each step run's own, in order,
// and those of the workflow script, which also takes logs of steps not listed.
func groupLogsByStep(state workflow.ExecutionState) (map[stepRunKey][]string, []string) {
	listed := make(map[stepRunKey]bool, len(state.Steps))
	for _, ss := range state.Steps {
		listed[stepRunKeyOf(ss)] = true
	}

	stepLogs := make(map[stepRunKey][]string)
	var workflowLogs []string
	for _, entry := range state.Logs {
		key := stepRunKey{stepID: entry.StepID, iteration: entry.Iteration}
		if listed[key] {
			stepLogs[key] = append(stepLogs[key], entry.Message)
		} else {
			workflowLogs = append(workflowLogs, entry.Message)
		}
	}
	return stepLogs, workflowLogs
}

func styleLogMessage(msg string, style runStyler) string {
	switch {
	case strings.HasPrefix(msg, workflow.LogPassMarker):
		return style.status(workflow.StatusSuccess, msg)
	case strings.HasPrefix(msg, workflow.LogFailMarker), strings.HasPrefix(msg, workflow.LogScriptErrorPrefix):
		return style.failure(msg)
	default:
		return style.subtle(msg)
	}
}

// iterationStepPrefixFormat tells apart one step's runs in different iterations.
const iterationStepPrefixFormat = "#%d "

// isRunFinished reports whether a run reached a final status and has a summary.
func isRunFinished(status string) bool {
	return status == workflow.StatusSuccess || status == workflow.StatusFailed || status == workflow.StatusStopped
}

// iterationLabel is " · iteration i/n" for multi-iteration runs, else "".
func iterationLabel(state workflow.ExecutionState) string {
	if !state.IsMultiIteration() {
		return ""
	}
	return fmt.Sprintf(" · iteration %d/%d", state.Iteration, state.Iterations)
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

func (m Model) padToHeight(content string, height int) string {
	lines := strings.Split(content, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
