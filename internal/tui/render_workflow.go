package tui

import (
	"fmt"
	"postOffice/internal/workflow"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderWorkflowsList() string {
	if m.workflowPreviewOpen {
		return m.renderWorkflowsListWithPreview()
	}
	h := m.calculateLayout().contentHeight
	return m.padToHeight(strings.Join(m.buildWorkflowListLines(h), "\n"), h)
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

	headerLines := append(m.workflowHeaderLines(wf), "")

	if len(wf.Steps) == 0 {
		content := strings.Join(headerLines, "\n") + subtleStyle.Render(noStepsMessage)
		return mainWindowStyle.
			Height(h).
			Width(m.width - 4).
			Render(content)
	}

	rows := m.workflowStepRows()
	colors := m.workflowRowColors(rows)
	visibleLines := h - len(headerLines) - 1
	startIdx, endIdx := calculateVisibleWindow(m.workflowStepCursor, len(rows), visibleLines)

	lines := append([]string{}, headerLines...)

	for i := startIdx; i < endIdx; i++ {
		row := rows[i]
		step := row.step
		selected := i == m.workflowStepCursor

		cursor := "  "
		if selected {
			cursor = "> "
		}

		// Sub-workflow steps are numbered by their folder step, so only the workflow's own steps get a number.
		num := workflowMetaStyle.Render(fmt.Sprintf("%-4s", ""))
		if row.isTopLevel() {
			num = workflowMetaStyle.Render(fmt.Sprintf("%-4s", fmt.Sprintf("[%d]", row.index+1)))
		}
		prefix := workflowRowBars(row, colors, styledRunStyler) + m.folderStepMarker(row)
		stepIDStr := fmt.Sprintf("%-20s", step.ID)
		requestStr := stepTargetLabel(step)
		if len(requestStr) > 40 {
			requestStr = requestStr[:37] + "..."
		}
		requestStr = fmt.Sprintf("%-40s", requestStr)

		annotationStr := ""
		if annotations := stepScriptAnnotations(step); annotations != "" {
			annotationStr = " " + workflowMetaStyle.Render(annotations)
		}

		// Step run state icon (from current/last workflow execution)
		runStateStr := ""
		if ss := m.runStepState(row.runID); ss != nil && ss.Status != workflow.StatusPending {
			icon := workflow.StatusIcon(ss.Status)
			runStateStr = "  " + workflowStatusStyle(ss.Status).Render(icon+" "+ss.Status)
			if ss.Total > 1 {
				runStateStr += workflowMetaStyle.Render(fmt.Sprintf(" (%d/%d)", ss.Progress, ss.Total))
			}
		}

		// HTTP execution info (from requestExecutions, shown when no active run state)
		executionInfo := ""
		if runStateStr == "" && m.collection != nil && !step.IsFolder() {
			if exec, exists := m.requestExecutions[m.workflowStepItemID(step.Request)]; exists {
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
			fullLine := cursor + num + " " + prefix +
				lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render(stepIDStr) +
				"  " + subtleStyle.Render(requestStr) +
				annotationStr + runStateStr + executionInfo
			lines = append(lines, selectedItemStyle.Render(fullLine))
		} else {
			fullLine := cursor + num + " " + prefix + normalItemStyle.Render(stepIDStr) +
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
	if m.workflowChan != nil && m.workflowState.WorkflowID == wf.ID {
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
	// groupBar marks one level of sub-workflow nesting; colorIndex tells the
	// folder steps apart.
	groupBar func(colorIndex int) string
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
		groupBar: func(colorIndex int) string {
			color := subWorkflowColors[colorIndex%len(subWorkflowColors)]
			return lipgloss.NewStyle().Foreground(color).Render(subWorkflowBar)
		},
	}
	// The editor export folds on indent, so it marks nesting with indent only.
	plainRunStyler = runStyler{
		title:    identity,
		heading:  identity,
		status:   func(_, text string) string { return text },
		subtle:   identity,
		failure:  identity,
		groupBar: func(int) string { return subWorkflowIndent },
	}
)

// subWorkflowColors color the bars marking each sub-workflow's steps in the
// run view, one colour per folder step in turn.
var subWorkflowColors = []lipgloss.Color{"39", "170", "214", "42", "75", "204"}

const (
	subWorkflowBar    = "▌ "
	subWorkflowIndent = "  "
	// folderStepLabel prefixes the folder a folder step runs.
	folderStepLabel           = "folder "
	folderStepCollapsedMarker = "▸ "
	folderStepExpandedMarker  = "▾ "
	folderStepMarkerBlank     = "  "
)

func (m Model) buildWorkflowRunLines() []string {
	return runReportLines(m.workflowState, styledRunStyler)
}

// Indents of the run report. The editor export folds on indent, so each
// step's log lines fold under the step that produced them.
const (
	runReportStepIndent = "  "
	runReportLogOffset  = "    "
	runReportLogIndent  = runReportStepIndent + runReportLogOffset
)

// runReportLines lays out a workflow run: header, one line per step (with the
// request and HTTP result once it has run) followed by that step's logs, then
// the workflow script's own logs and the run error. A folder step's line heads
// its sub-workflow's steps, which share its bar down the left edge.
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
		colors := subWorkflowColorIndexes(state)
		for _, ss := range state.Steps {
			bars := groupBars(ss, colors, style)
			lines = append(lines, runReportStepLine(ss, bars, state.IsMultiIteration(), style))
			logs := stepLogs[stepRunKeyOf(ss)]
			for _, msg := range logs {
				lines = append(lines, runReportStepIndent+bars+runReportLogOffset+styleLogMessage(msg, style))
			}
			if len(logs) > 0 {
				lines = append(lines, strings.TrimRight(runReportStepIndent+bars, " "))
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

// stepTargetLabel is what a step runs: its request, or its folder labelled as one.
func stepTargetLabel(step workflow.Step) string {
	if step.IsFolder() {
		return folderStepLabel + step.Folder
	}
	return step.Request
}

// subWorkflowColorIndexes numbers the run's folder steps in the order they
// are listed, so each sub-workflow's bar gets its own colour.
func subWorkflowColorIndexes(state workflow.ExecutionState) map[string]int {
	indexes := make(map[string]int)
	for _, ss := range state.Steps {
		if ss.IsFolder() {
			numberColor(indexes, ss.ID)
		}
	}
	return indexes
}

func numberColor(indexes map[string]int, folderStepID string) {
	if _, numbered := indexes[folderStepID]; !numbered {
		indexes[folderStepID] = len(indexes)
	}
}

// groupBars marks the sub-workflows a step is in with a bar each, outermost
// first; a folder step's own line also carries the bar of the sub-workflow it heads.
func groupBars(ss *workflow.StepState, colors map[string]int, style runStyler) string {
	return barsFor(ss.Groups, ss.ID, ss.IsFolder(), colors, style)
}

func barsFor(groups []string, stepID string, isFolder bool, colors map[string]int, style runStyler) string {
	if isFolder {
		groups = append(slices.Clone(groups), stepID)
	}
	var bars strings.Builder
	for _, group := range groups {
		bars.WriteString(style.groupBar(colors[group]))
	}
	return bars.String()
}

// workflowRowBars is groupBars for a row of the workflow detail step list.
func workflowRowBars(row workflowStepRow, colors map[string]int, style runStyler) string {
	return barsFor(row.groups, row.runID, row.step.IsFolder(), colors, style)
}

// workflowRowColors numbers folder steps like the run report does, so a
// sub-workflow has the same colour in both, then any not run yet in row order.
func (m Model) workflowRowColors(rows []workflowStepRow) map[string]int {
	colors := map[string]int{}
	if m.activeWorkflow != nil && m.workflowState.WorkflowID == m.activeWorkflow.ID {
		colors = subWorkflowColorIndexes(m.workflowState)
	}
	for _, row := range rows {
		if row.step.IsFolder() {
			numberColor(colors, row.runID)
		}
	}
	return colors
}

// folderStepMarker shows whether a folder step is expanded; other steps get
// the same width of space so their labels line up.
func (m Model) folderStepMarker(row workflowStepRow) string {
	switch {
	case !row.step.IsFolder():
		return folderStepMarkerBlank
	case m.isWorkflowStepExpanded(row.runID):
		return folderStepExpandedMarker
	default:
		return folderStepCollapsedMarker
	}
}

// runReportStepLine is one step's report line, after the bars of the
// sub-workflows it is in. In a multi-iteration run the step is prefixed with
// its iteration, since each iteration lists every step.
func runReportStepLine(ss *workflow.StepState, bars string, showIteration bool, style runStyler) string {
	stepID := workflow.LocalStepID(ss.ID)
	if showIteration {
		stepID = fmt.Sprintf(iterationStepPrefixFormat, ss.Iteration) + stepID
	}
	detail := ""
	if ss.Total > 1 {
		detail = fmt.Sprintf(" (%d/%d)", ss.Progress, ss.Total)
	}
	switch {
	case ss.IsFolder():
		detail += "  " + folderStepLabel + ss.Folder
	case ss.LastResponse != nil:
		detail += fmt.Sprintf("  %s  %s  %v", ss.Request, ss.LastResponse.Status, ss.LastResponse.Duration.Round(time.Millisecond))
	}
	return fmt.Sprintf("%s%s%s %-20s %s%s",
		runReportStepIndent,
		bars,
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
