package tui

import (
	"fmt"
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
	scroll := min(m.workflowStepPanelScroll, maxStepPanelScroll(len(panelLines), paneInnerHeight(layout.listHeight)))
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

	startIdx, endIdx := calculateVisibleWindow(m.workflowStepCursor, len(wf.Steps), height-len(lines))
	for i := startIdx; i < endIdx; i++ {
		step := wf.Steps[i]
		status := workflow.StatusPending
		if ss := m.runStepState(step.ID); ss != nil {
			status = ss.Status
		}

		icon := workflowStatusStyle(status).Render(workflow.StatusIcon(status))
		if i == m.workflowStepCursor {
			lines = append(lines, selectedItemStyle.Render("> "+icon+" "+step.ID))
		} else {
			lines = append(lines, normalItemStyle.Render("  "+icon+" "+step.ID))
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

	ss := m.runStepState(step.ID)
	if ss == nil {
		return wrapLines([]string{
			styledRunStyler.title(step.ID),
			subtleStyle.Render(step.Request),
			"",
			subtleStyle.Render(stepPanelNotRunMessage),
		}, width)
	}

	lines := []string{
		styledRunStyler.title(ss.ID) + " " + styledRunStyler.status(ss.Status, "["+ss.Status+"]"),
		subtleStyle.Render(ss.Request),
	}
	if ss.Total > 1 {
		lines = append(lines, subtleStyle.Render(fmt.Sprintf("attempt %d/%d", ss.Progress, ss.Total)))
	}

	lines = append(lines, "", lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11")).Render("LOGS"))
	stepLogs, _ := groupLogsByStep(m.workflowState)
	if logs := stepLogs[ss.ID]; len(logs) > 0 {
		for _, msg := range logs {
			lines = append(lines, styleLogMessage(msg, styledRunStyler))
		}
	} else {
		lines = append(lines, subtleStyle.Render(stepPanelNoLogsMessage))
	}

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
	step, ok := m.selectedWorkflowStep()
	if !ok {
		return popupBorderColor
	}
	if ss := m.runStepState(step.ID); ss != nil && ss.LastResponse != nil {
		return responseBorderColor(ss.LastResponse)
	}
	return popupBorderColor
}

func (m Model) selectedWorkflowStep() (workflow.Step, bool) {
	if m.activeWorkflow == nil || m.workflowStepCursor < 0 || m.workflowStepCursor >= len(m.activeWorkflow.Steps) {
		return workflow.Step{}, false
	}
	return m.activeWorkflow.Steps[m.workflowStepCursor], true
}

// runStepState is the state of stepID in the active workflow's latest run, or
// nil when that workflow has not run.
func (m Model) runStepState(stepID string) *workflow.StepState {
	if m.activeWorkflow == nil || m.workflowState.WorkflowID != m.activeWorkflow.ID || m.workflowState.StepIndex == nil {
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

func (m Model) maxStepPanelScroll() int {
	layout := m.calculateRequestSplitLayout()
	lineCount := len(m.buildStepPanelLines(paneInnerWidth(layout.rightWidth)))
	return maxStepPanelScroll(lineCount, paneInnerHeight(layout.listHeight))
}

// maxStepPanelScroll stops scrolling once the last line is on screen, so the
// panel never scrolls into blank space.
func maxStepPanelScroll(lineCount, height int) int {
	return max(lineCount-height, 0)
}
