package tui

import (
	"strings"
	"testing"

	"postOffice/internal/workflow"

	tea "github.com/charmbracelet/bubbletea"
)

var abortStepKeyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(abortWorkflowStepKey)}

// runningFolderWorkflowModel is folderWorkflowDetailModel mid-run: the invite folder step and its
// redeem sub-step are running. aborted records what the abort key asked the runner to abort.
func runningFolderWorkflowModel(t *testing.T, aborted *[]string) Model {
	t.Helper()
	m := folderWorkflowDetailModel(t)
	invite := &workflow.StepState{ID: "invite", Folder: "policies/TEST/invite", Status: workflow.StatusRunning, Iteration: 1}
	redeem := &workflow.StepState{ID: "invite/redeem", Request: "policies/TEST/invite/redeem", Groups: []string{"invite"}, Status: workflow.StatusRunning, Iteration: 1}
	setup := &workflow.StepState{ID: "setup", Request: "policies/TEST/setup", Status: workflow.StatusSuccess, Iteration: 1}
	m.workflowState = workflow.ExecutionState{
		WorkflowID: m.activeWorkflow.ID,
		Status:     workflow.StatusRunning,
		Iteration:  1,
		Steps:      []*workflow.StepState{setup, invite, redeem},
		StepIndex:  map[string]*workflow.StepState{"setup": setup, "invite": invite, "invite/redeem": redeem},
	}
	m.abortWorkflowStep = func(runID string) bool {
		*aborted = append(*aborted, runID)
		return true
	}
	return m
}

func TestAbortWorkflowStep_abortsTheSelectedRunningStep(t *testing.T) {
	var aborted []string
	m := cursorOnStep(t, runningFolderWorkflowModel(t, &aborted), "invite")

	m, _ = pressKey(m, abortStepKeyMsg)

	if len(aborted) != 1 || aborted[0] != "invite" {
		t.Errorf("aborted %v, want the selected folder step", aborted)
	}
	if !strings.Contains(m.statusMessage, "the run carries on") {
		t.Errorf("status = %q", m.statusMessage)
	}
}

func TestAbortWorkflowStep_withoutARunningSelectionAbortsTheInnermostStep(t *testing.T) {
	var aborted []string
	m := cursorOnStep(t, runningFolderWorkflowModel(t, &aborted), "setup")

	pressKey(m, abortStepKeyMsg)

	if len(aborted) != 1 || aborted[0] != "invite/redeem" {
		t.Errorf("aborted %v, want the innermost running step", aborted)
	}
}

func TestAbortWorkflowStep_withoutARun(t *testing.T) {
	m, _ := pressKey(folderWorkflowDetailModel(t), abortStepKeyMsg)

	if m.statusMessage != "No workflow is running" {
		t.Errorf("status = %q", m.statusMessage)
	}
}
