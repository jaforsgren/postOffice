package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// Workflow detail keys. The run variants avoid u/f/r, which mean undo,
// favorite and refresh in the lists.
const (
	runUpToStepKey   = "<"
	runFromStepKey   = ">"
	runStepOnlyKey   = "x"
	stepPanelKey     = toggleSplitViewKey
	stepPanelDownKey = "]"
	stepPanelUpKey   = "["
	// The workflows list preview mirrors the detail's step panel keys.
	workflowPreviewKey     = stepPanelKey
	workflowPreviewDownKey = stepPanelDownKey
	workflowPreviewUpKey   = stepPanelUpKey
	refreshWorkflowsKey    = "r"
)

// handleRefreshWorkflowsKey rereads the workflow files, picking up edits
// made outside the TUI, and keeps the open workflow open.
func handleRefreshWorkflowsKey(m Model) (Model, tea.Cmd) {
	m, err := m.listWorkflows()
	if err != nil {
		m.statusMessage = fmt.Sprintf("Failed to reload workflows: %v", err)
		m.statusIsError = true
		return m, nil
	}
	if m.activeWorkflow != nil {
		if reloaded, ok := m.findListedWorkflow(m.activeWorkflowCollection, m.activeWorkflow.ID); ok {
			m.activeWorkflow = reloaded
			m.workflowStepCursor = min(m.workflowStepCursor, max(len(m.workflowStepRows())-1, 0))
		}
	}
	m.statusMessage = fmt.Sprintf("Reloaded %d workflow(s)", len(m.workflows))
	return m, nil
}
