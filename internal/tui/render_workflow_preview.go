package tui

import (
	"fmt"
	"postOffice/internal/workflow"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	workflowsListTitle        = "Workflows"
	noWorkflowsMessage        = "No workflows found."
	createWorkflowHintMessage = "Create one with: :wf new <id>"
	noStepsMessage            = "No steps defined."
	workflowPreviewStepsTitle = "STEPS"
)

var (
	workflowMetaStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	workflowHeadingStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
)

// renderWorkflowsListWithPreview shows the workflows on the left and the
// selected workflow's steps and last run on the right.
func (m Model) renderWorkflowsListWithPreview() string {
	layout := m.calculateRequestSplitLayout()
	paneHeight := paneInnerHeight(layout.listHeight)

	list := renderSplitPane(
		strings.Join(m.buildWorkflowListLines(paneHeight), "\n"),
		layout.listWidth, layout.listHeight, mainWindowBorderColor,
	)
	preview := renderSplitPane(
		scrolledContent(m.buildWorkflowPreviewLines(paneInnerWidth(layout.rightWidth)), m.workflowPreviewScroll, paneHeight),
		layout.rightWidth, layout.listHeight, popupBorderColor,
	)

	return lipgloss.JoinHorizontal(lipgloss.Top, list, preview)
}

// buildWorkflowListLines is one line per workflow, scrolled to keep the cursor
// visible, so workflows with many steps never push the others off screen.
func (m Model) buildWorkflowListLines(height int) []string {
	lines := []string{styledRunStyler.title(workflowsListTitle), ""}
	if len(m.workflows) == 0 {
		return append(lines, subtleStyle.Render(noWorkflowsMessage), subtleStyle.Render(createWorkflowHintMessage))
	}

	startIdx, endIdx := calculateVisibleWindow(m.workflowCursor, len(m.workflows), height-len(lines))
	for i := startIdx; i < endIdx; i++ {
		lines = append(lines, m.workflowListLine(m.workflows[i], i == m.workflowCursor))
	}
	return lines
}

func (m Model) workflowListLine(wf *workflow.Workflow, selected bool) string {
	status := m.lastRunStatus(wf)
	icon := workflowStatusStyle(status).Render(workflow.StatusIcon(status))

	prefix := "  "
	name := wf.Name
	if selected {
		prefix = "> "
		name = styledRunStyler.title(name)
	}

	line := prefix + icon + " " + name + "  " + workflowMetaStyle.Render(stepCountLabel(len(wf.Steps)))
	if wf.Description != "" {
		line += "  " + subtleStyle.Render(wf.Description)
	}
	return line
}

// lastRunStatus is the status of wf's latest run, or pending when it has not run.
func (m Model) lastRunStatus(wf *workflow.Workflow) string {
	if m.workflowState.WorkflowID == wf.ID && m.workflowState.Status != "" {
		return m.workflowState.Status
	}
	return workflow.StatusPending
}

func stepCountLabel(count int) string {
	if count == 1 {
		return "1 step"
	}
	return fmt.Sprintf("%d steps", count)
}

// buildWorkflowPreviewLines lays out the selected workflow: its header (name,
// description, step count, last run summary) and every step with its request,
// scripts and status in the last run, wrapped to width.
func (m Model) buildWorkflowPreviewLines(width int) []string {
	wf, ok := m.selectedListWorkflow()
	if !ok {
		return []string{noItemSelectedMessage}
	}

	lines := append(m.workflowHeaderLines(wf), "", workflowHeadingStyle.Render(workflowPreviewStepsTitle))
	if len(wf.Steps) == 0 {
		lines = append(lines, subtleStyle.Render(noStepsMessage))
	}
	for i, step := range wf.Steps {
		lines = append(lines, m.workflowPreviewStepLine(wf, i, step))
	}
	return wrapLines(lines, width)
}

func (m Model) workflowPreviewStepLine(wf *workflow.Workflow, index int, step workflow.Step) string {
	status := workflow.StatusPending
	if ss := m.workflowRunStepState(wf.ID, step.ID); ss != nil {
		status = ss.Status
	}

	line := workflowMetaStyle.Render(fmt.Sprintf("[%d]", index+1)) + " " +
		workflowStatusStyle(status).Render(workflow.StatusIcon(status)) + " " +
		step.ID + "  " + subtleStyle.Render(step.Request)
	if annotations := stepScriptAnnotations(step); annotations != "" {
		line += " " + workflowMetaStyle.Render(annotations)
	}
	return line
}

// workflowHeaderLines is the title (with run indicator), description, step
// count and version, and the summary of wf's finished run.
func (m Model) workflowHeaderLines(wf *workflow.Workflow) []string {
	lines := []string{styledRunStyler.title(wf.Name) + m.workflowRunIndicator(wf)}
	if wf.Description != "" {
		lines = append(lines, subtleStyle.Render(wf.Description))
	}
	meta := fmt.Sprintf("Steps: %d", len(wf.Steps))
	if wf.Version > 0 {
		meta += fmt.Sprintf("  v%d", wf.Version)
	}
	lines = append(lines, workflowMetaStyle.Render(meta))
	if m.workflowState.WorkflowID == wf.ID && isRunFinished(m.workflowState.Status) {
		lines = append(lines, workflowMetaStyle.Render(m.workflowState.Summary.String()))
	}
	return lines
}

// stepScriptAnnotations marks the scripts a step runs, e.g. "[pre] [post]".
func stepScriptAnnotations(step workflow.Step) string {
	var annotations []string
	if step.PreScript != "" {
		annotations = append(annotations, "[pre]")
	}
	if step.PostScript != "" {
		annotations = append(annotations, "[post]")
	}
	return strings.Join(annotations, " ")
}

func (m Model) selectedListWorkflow() (*workflow.Workflow, bool) {
	if m.workflowCursor < 0 || m.workflowCursor >= len(m.workflows) {
		return nil, false
	}
	return m.workflows[m.workflowCursor], true
}
