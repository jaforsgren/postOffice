package tui

import (
	"fmt"
	"postOffice/internal/http"
	"postOffice/internal/workflow"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	stepPanelNotRunMessage     = "Not run yet — press enter to run the workflow"
	stepPanelNoLogsMessage     = "(no logs)"
	stepPanelNoResponseMessage = "No request was sent for this step in this run"
)

// renderWorkflowDetailWithStepPanel shows the workflow's steps on the left and
// the selected step's run output on the right.
func (m Model) renderWorkflowDetailWithStepPanel() string {
	layout := m.calculateRequestSplitLayout()

	stepList := renderSplitPane(
		strings.Join(m.buildWorkflowStepListLines(paneInnerHeight(layout.listHeight)), "\n"),
		layout.listWidth, layout.listHeight, mainWindowBorderColor,
	)

	panelLines := m.buildStepPanelLines(paneInnerWidth(layout.rightWidth))
	scroll := min(m.workflowStepPanelScroll, maxPaneScroll(len(panelLines), paneInnerHeight(layout.listHeight)))
	panel := renderSplitPane(
		strings.Join(panelLines[scroll:], "\n"),
		layout.rightWidth, layout.listHeight, m.stepPanelBorderColor(),
	)

	return lipgloss.JoinHorizontal(lipgloss.Top, stepList, panel)
}

// buildWorkflowStepListLines is a compact step list (status icon and ID) that
// fits the narrow left pane, scrolled to keep the cursor visible.
func (m Model) buildWorkflowStepListLines(height int) []string {
	wf := m.activeWorkflow
	if wf == nil {
		return []string{subtleStyle.Render("No workflow selected.")}
	}

	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render(wf.Name) + m.workflowRunIndicator(wf),
		"",
	}
	if len(wf.Steps) == 0 {
		return append(lines, subtleStyle.Render("No steps defined."))
	}

	rows := m.workflowStepRows()
	colors := m.workflowRowColors(rows)
	startIdx, endIdx := calculateVisibleWindow(m.workflowStepCursor, len(rows), height-len(lines))
	for i := startIdx; i < endIdx; i++ {
		row := rows[i]
		status := workflow.StatusPending
		if ss := m.runStepState(row.runID); ss != nil {
			status = ss.Status
		}

		icon := workflowStatusStyle(status).Render(workflow.StatusIcon(status))
		label := workflowRowBars(row, colors, styledRunStyler) + m.folderStepMarker(row) + icon + " " + row.step.ID
		if i == m.workflowStepCursor {
			lines = append(lines, selectedItemStyle.Render("> "+label))
		} else {
			lines = append(lines, normalItemStyle.Render("  "+label))
		}
	}
	return lines
}

// buildStepPanelLines lays out the selected step's run: header, its logs, then
// the request sent and response received. Lines are wrapped to width so long
// tokens and test failures stay readable instead of being cut off.
func (m Model) buildStepPanelLines(width int) []string {
	step, ok := m.selectedWorkflowStep()
	if !ok {
		return []string{noItemSelectedMessage}
	}

	ss := m.selectedRunStepState()
	if ss == nil {
		return wrapLines([]string{
			styledRunStyler.title(step.ID),
			subtleStyle.Render(stepTargetLabel(step)),
			"",
			subtleStyle.Render(stepPanelNotRunMessage),
		}, width)
	}

	lines := []string{
		styledRunStyler.title(ss.ID) + " " + styledRunStyler.status(ss.Status, "["+ss.Status+"]"),
		subtleStyle.Render(ss.Target()),
	}
	if ss.Total > 1 {
		lines = append(lines, subtleStyle.Render(fmt.Sprintf("attempt %d/%d", ss.Progress, ss.Total)))
	}
	// StepIndex holds the latest iteration's state, so the panel shows that iteration.
	if m.workflowState.IsMultiIteration() {
		lines = append(lines, subtleStyle.Render(fmt.Sprintf("iteration %d/%d", ss.Iteration, m.workflowState.Iterations)))
	}

	lines = append(lines, "")
	lines = append(lines, m.stepLogLines(ss)...)
	lines = append(lines, "")
	if ss.LastResponse == nil {
		lines = append(lines, subtleStyle.Render(stepPanelNoResponseMessage))
	} else {
		responseModel := m
		responseModel.lastResponse = ss.LastResponse
		lines = append(lines, responseModel.buildRequestSection()...)
		lines = append(lines, "")
		lines = append(lines, responseModel.buildResponseSection()...)
	}

	return wrapLines(lines, width)
}

func (m Model) stepPanelBorderColor() lipgloss.Color {
	if ss := m.selectedRunStepState(); ss != nil && ss.LastResponse != nil {
		return responseBorderColor(ss.LastResponse)
	}
	return popupBorderColor
}

// stepLogLines is the LOGS section of one step run: its log messages (script
// console output, test results, errors), or a placeholder when it has none.
func (m Model) stepLogLines(ss *workflow.StepState) []string {
	lines := []string{lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11")).Render("LOGS")}
	logs := m.stepRunLogs(ss)
	if len(logs) == 0 {
		return append(lines, subtleStyle.Render(stepPanelNoLogsMessage))
	}
	for _, msg := range logs {
		lines = append(lines, styleLogMessage(msg, styledRunStyler))
	}
	return lines
}

// stepRunLogs is the unstyled log messages of one step run, in order.
func (m Model) stepRunLogs(ss *workflow.StepState) []string {
	stepLogs, _ := groupLogsByStep(m.workflowState)
	return stepLogs[stepRunKeyOf(ss)]
}

func (m Model) selectedWorkflowStep() (workflow.Step, bool) {
	row, ok := m.selectedWorkflowRow()
	return row.step, ok
}

// selectedStepResponse is the selected step's latest response, preferring the
// current run's so a step that did not send this run shows nothing rather
// than an older response, and its execution when that response was recorded.
func (m Model) selectedStepResponse() (*http.Response, *RequestExecution) {
	step, ok := m.selectedWorkflowStep()
	if !ok || m.collection == nil {
		return nil, nil
	}
	execution, executed := m.requestExecutions[m.workflowStepItemID(step.Request)]

	if ss := m.selectedRunStepState(); ss != nil {
		if executed && execution.Response == ss.LastResponse {
			return ss.LastResponse, execution
		}
		return ss.LastResponse, nil
	}
	if executed && execution.Response != nil {
		return execution.Response, execution
	}
	return nil, nil
}

// runStepState is the state of stepID in the active workflow's latest run, or
// nil when that workflow has not run.
func (m Model) runStepState(stepID string) *workflow.StepState {
	if m.activeWorkflow == nil {
		return nil
	}
	return m.workflowRunStepState(m.activeWorkflow.ID, stepID)
}

// workflowRunStepState is the state of stepID in workflowID's latest run, or
// nil when that workflow has not run.
func (m Model) workflowRunStepState(workflowID, stepID string) *workflow.StepState {
	if m.workflowState.WorkflowID != workflowID || m.workflowState.StepIndex == nil {
		return nil
	}
	return m.workflowState.StepIndex[stepID]
}

func wrapLines(lines []string, width int) []string {
	wrap := lipgloss.NewStyle().Width(width)
	var wrapped []string
	for _, line := range lines {
		wrapped = append(wrapped, strings.Split(wrap.Render(line), "\n")...)
	}
	return wrapped
}

// maxSidePanelScroll is how far the right pane of a workflow split view can
// scroll when buildLines fills it.
func (m Model) maxSidePanelScroll(buildLines func(width int) []string) int {
	layout := m.calculateRequestSplitLayout()
	lineCount := len(buildLines(paneInnerWidth(layout.rightWidth)))
	return maxPaneScroll(lineCount, paneInnerHeight(layout.listHeight))
}
