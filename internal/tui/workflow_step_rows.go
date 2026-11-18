package tui

import (
	"errors"
	"fmt"
	"slices"

	"postOffice/internal/logger"
	"postOffice/internal/postman"
	"postOffice/internal/workflow"
)

var errNoWorkflowStepSelected = errors.New("no step selected")

var errNestedWorkflowStep = errors.New("a sub-workflow's step belongs to its folder's workflow: open that workflow to change or run it on its own")

// workflowStepRow is one line of the workflow detail step list: a step of the
// active workflow or, under an expanded folder step, a step of its sub-workflow.
type workflowStepRow struct {
	step workflow.Step
	// runID is the step's run state ID, qualified inside a sub-workflow (e.g. "invite/redeem").
	runID string
	// index is the step's position in the active workflow; -1 inside a sub-workflow.
	index int
	// groups are the run IDs of the folder steps the row is nested in, outermost first.
	groups []string
}

func (r workflowStepRow) isTopLevel() bool { return r.index >= 0 }

// workflowStepRows lists the active workflow's steps with each expanded folder
// step followed by its sub-workflow's steps.
func (m Model) workflowStepRows() []workflowStepRow {
	if m.activeWorkflow == nil {
		return nil
	}
	var rows []workflowStepRow
	for i, step := range m.activeWorkflow.Steps {
		rows = m.appendWorkflowStepRow(rows, workflowStepRow{step: step, runID: step.ID, index: i})
	}
	return rows
}

func (m Model) appendWorkflowStepRow(rows []workflowStepRow, row workflowStepRow) []workflowStepRow {
	rows = append(rows, row)
	if !row.step.IsFolder() || !m.isWorkflowStepExpanded(row.runID) {
		return rows
	}
	groups := append(slices.Clone(row.groups), row.runID)
	for _, sub := range m.subWorkflowSteps(row.runID) {
		rows = m.appendWorkflowStepRow(rows, workflowStepRow{
			step:   sub,
			runID:  workflow.QualifiedStepID(row.runID, sub.ID),
			index:  -1,
			groups: groups,
		})
	}
	return rows
}

// subWorkflowSteps are the steps of the folder step runID: as the active
// workflow's latest run listed them, else as its folder's workflow defines them.
func (m Model) subWorkflowSteps(runID string) []workflow.Step {
	if ran := m.ranSubWorkflowSteps(runID); len(ran) > 0 {
		return ran
	}
	return m.folderStepDefinitions[runID]
}

// ranSubWorkflowSteps are the steps the latest iteration of the active
// workflow's run listed directly inside the folder step runID.
func (m Model) ranSubWorkflowSteps(runID string) []workflow.Step {
	state := m.workflowState
	if m.activeWorkflow == nil || state.WorkflowID != m.activeWorkflow.ID {
		return nil
	}
	var steps []workflow.Step
	for _, ss := range state.Steps {
		if ss.Iteration != state.Iteration || len(ss.Groups) == 0 || ss.Groups[len(ss.Groups)-1] != runID {
			continue
		}
		steps = append(steps, workflow.Step{ID: workflow.LocalStepID(ss.ID), Request: ss.Request, Folder: ss.Folder})
	}
	return steps
}

func (m Model) isWorkflowStepExpanded(runID string) bool {
	return m.activeWorkflow != nil && m.expandedWorkflowID == m.activeWorkflow.ID && m.expandedWorkflowSteps[runID]
}

func (m Model) selectedWorkflowRow() (workflowStepRow, bool) {
	rows := m.workflowStepRows()
	if m.workflowStepCursor < 0 || m.workflowStepCursor >= len(rows) {
		return workflowStepRow{}, false
	}
	return rows[m.workflowStepCursor], true
}

// selectedTopLevelStepIndex is the active workflow's index of the selected
// step, for actions that change or partly run the workflow's own steps.
func (m Model) selectedTopLevelStepIndex() (int, error) {
	row, ok := m.selectedWorkflowRow()
	if !ok {
		return 0, errNoWorkflowStepSelected
	}
	if !row.isTopLevel() {
		return 0, errNestedWorkflowStep
	}
	return row.index, nil
}

// selectedRunStepState is the selected step's state in the active workflow's latest run.
func (m Model) selectedRunStepState() *workflow.StepState {
	row, ok := m.selectedWorkflowRow()
	if !ok {
		return nil
	}
	return m.runStepState(row.runID)
}

// toggleWorkflowStepExpand expands or collapses the selected folder step.
func (m Model) toggleWorkflowStepExpand() Model {
	row, ok := m.selectedWorkflowRow()
	if !ok || !row.step.IsFolder() {
		m.statusMessage = "Only folder steps can be expanded"
		return m
	}
	m = m.scopeExpansionToActiveWorkflow()
	if m.expandedWorkflowSteps[row.runID] {
		delete(m.expandedWorkflowSteps, row.runID)
		m.statusMessage = fmt.Sprintf("Collapsed: %s", row.step.Folder)
		return m
	}
	m = m.expandWorkflowStep(row)
	m.statusMessage = fmt.Sprintf("Expanded: %s", row.step.Folder)
	return m
}

// toggleAllWorkflowStepsExpand expands every folder step at every depth, or
// collapses them all when any is expanded.
func (m Model) toggleAllWorkflowStepsExpand() Model {
	m = m.scopeExpansionToActiveWorkflow()
	if len(m.expandedWorkflowSteps) > 0 {
		clear(m.expandedWorkflowSteps)
		m.workflowStepCursor = min(m.workflowStepCursor, max(len(m.workflowStepRows())-1, 0))
		m.statusMessage = "Collapsed all folder steps"
		return m
	}
	// Expanding a row adds its sub-steps after it, so the walk reaches nested folder steps too.
	for i := 0; i < len(m.workflowStepRows()); i++ {
		if row := m.workflowStepRows()[i]; row.step.IsFolder() {
			m = m.expandWorkflowStep(row)
		}
	}
	m.statusMessage = "Expanded all folder steps"
	return m
}

func (m Model) expandWorkflowStep(row workflowStepRow) Model {
	m.expandedWorkflowSteps[row.runID] = true
	if _, known := m.folderStepDefinitions[row.runID]; known || len(m.ranSubWorkflowSteps(row.runID)) > 0 {
		return m
	}
	sub, err := m.folderWorkflowSource()(postman.SplitItemPath(row.step.Folder))
	if err != nil {
		logger.LogError("expandWorkflowStep", row.step.Folder, err)
		m.statusMessage = fmt.Sprintf("Cannot show folder %q: %v", row.step.Folder, err)
		m.statusIsError = true
		return m
	}
	m.folderStepDefinitions[row.runID] = sub.Steps
	return m
}

// scopeExpansionToActiveWorkflow forgets expanded steps and cached
// definitions of another workflow, whose run IDs mean nothing here.
func (m Model) scopeExpansionToActiveWorkflow() Model {
	if m.activeWorkflow == nil || m.expandedWorkflowID == m.activeWorkflow.ID {
		return m
	}
	m.expandedWorkflowID = m.activeWorkflow.ID
	m.expandedWorkflowSteps = make(map[string]bool)
	m.folderStepDefinitions = make(map[string][]workflow.Step)
	return m
}
