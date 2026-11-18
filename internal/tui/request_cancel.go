package tui

import (
	"fmt"
	"slices"

	"postOffice/internal/workflow"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	cancelRequestKey     = "ctrl+x"
	abortWorkflowStepKey = "X"
	statusCancelled      = "Cancelled"
)

// workflowRunModes are the views ctrl+x cancels the workflow run from.
var workflowRunModes = []ViewMode{ModeWorkflows, ModeWorkflowRun, ModeWorkflowDetail}

// handleCancelKey stops every listener from the listeners view, the workflow
// run in progress from the workflow views, otherwise the request being sent.
func handleCancelKey(m Model) (Model, tea.Cmd) {
	if m.mode == ModeListeners {
		return m.stopAllListeners(), nil
	}
	if slices.Contains(workflowRunModes, m.mode) {
		return handleCancelWorkflowRunKey(m)
	}
	return handleCancelRequestKey(m)
}

func handleCancelWorkflowRunKey(m Model) (Model, tea.Cmd) {
	if m.cancelWorkflowRun == nil {
		m.statusMessage = "No workflow is running"
		return m, nil
	}
	m.cancelWorkflowRun()
	m.statusMessage = fmt.Sprintf("Cancelling workflow %q...", m.workflowState.WorkflowName)
	return m, nil
}

// forgetWorkflowRun releases a finished workflow run's cancel function.
func (m Model) forgetWorkflowRun() Model {
	if m.cancelWorkflowRun != nil {
		m.cancelWorkflowRun()
		m.cancelWorkflowRun = nil
	}
	m.abortWorkflowStep = nil
	return m
}

// handleAbortWorkflowStepKey aborts one running step, a request or a whole
// folder step, and lets the run carry on with the next step.
func handleAbortWorkflowStepKey(m Model) (Model, tea.Cmd) {
	if m.abortWorkflowStep == nil {
		m.statusMessage = "No workflow is running"
		return m, nil
	}
	runID, ok := m.abortableWorkflowStepID()
	if !ok {
		m.statusMessage = "No step is running"
		return m, nil
	}
	if !m.abortWorkflowStep(runID) {
		m.statusMessage = fmt.Sprintf("Step %q already finished", runID)
		return m, nil
	}
	m.statusMessage = fmt.Sprintf("Aborting step %q, the run carries on...", runID)
	return m, nil
}

// abortableWorkflowStepID is the selected step when it is running (workflow
// detail), else the innermost running step: sub-steps are listed after their
// folder step, so the last running one is the deepest.
func (m Model) abortableWorkflowStepID() (string, bool) {
	if m.mode == ModeWorkflowDetail {
		if ss := m.selectedRunStepState(); ss != nil && ss.Status == workflow.StatusRunning {
			return ss.ID, true
		}
	}
	steps := m.workflowState.Steps
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].Status == workflow.StatusRunning {
			return steps[i].ID, true
		}
	}
	return "", false
}

// handleCancelRequestKey cancels the selected request while it is being
// sent, or the only request being sent when the selection isn't one.
func handleCancelRequestKey(m Model) (Model, tea.Cmd) {
	itemID, ok := m.cancellableRequestID()
	if !ok {
		if len(m.inFlightRequests) == 0 {
			m.statusMessage = "No request is being sent"
		} else {
			m.statusMessage = "Select the request to cancel"
		}
		return m, nil
	}
	m.inFlightRequests[itemID]()
	m.statusMessage = fmt.Sprintf("Cancelling %s...", itemID)
	return m, nil
}

func (m Model) cancellableRequestID() (string, bool) {
	if item, ok := m.selectedItem(); ok && m.mode == ModeRequests && item.IsRequest() {
		if itemID := m.requestItemID(m.folderPathForCursor(), item.Name); m.inFlightRequests[itemID] != nil {
			return itemID, true
		}
	}
	if len(m.inFlightRequests) == 1 {
		for itemID := range m.inFlightRequests {
			return itemID, true
		}
	}
	return "", false
}

// forgetInFlightRequest releases a finished request's cancel function.
func (m Model) forgetInFlightRequest(itemID string) Model {
	if cancel, ok := m.inFlightRequests[itemID]; ok {
		cancel()
		delete(m.inFlightRequests, itemID)
	}
	return m
}

// completeCancelledRequest marks a cancelled request in the list. It isn't
// recorded in history and its script variable changes aren't saved, since
// the request never completed.
func (m Model) completeCancelledRequest(msg RequestCompleteMsg) Model {
	m = m.forgetInFlightRequest(msg.ItemID)
	m.requestExecutions[msg.ItemID] = &RequestExecution{Status: statusCancelled, Duration: msg.Response.Duration}
	m.statusMessage = fmt.Sprintf("Cancelled: %s", msg.ItemName)
	return m
}
