package tui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"postOffice/internal/workflow"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	streamedSetupStepID   = "setup"
	streamedFolderStepID  = "invite"
	streamedFolderSubStep = "invite/redeem"
)

func stepStates(statuses map[string]string) workflow.ExecutionState {
	state := workflow.ExecutionState{Iteration: 1, StepIndex: make(map[string]*workflow.StepState)}
	for _, id := range []string{streamedSetupStepID, streamedFolderStepID, streamedFolderSubStep} {
		status, ok := statuses[id]
		if !ok {
			continue
		}
		ss := &workflow.StepState{ID: id, Status: status, Iteration: 1}
		if id == streamedFolderSubStep {
			ss.Groups = []string{streamedFolderStepID}
		}
		state.Steps = append(state.Steps, ss)
		state.StepIndex[id] = ss
	}
	return state
}

// rerunFolderStep is a partial run of the folder step alone: snapshots show it
// running until its sub-step finishes, the final state has it succeeded.
func rerunFolderStep(_ context.Context, _ *workflow.Runner, onChange func(workflow.ExecutionState)) workflow.ExecutionState {
	for _, running := range []map[string]string{
		{streamedSetupStepID: workflow.StatusPending, streamedFolderStepID: workflow.StatusRunning, streamedFolderSubStep: workflow.StatusRunning},
		{streamedSetupStepID: workflow.StatusPending, streamedFolderStepID: workflow.StatusRunning, streamedFolderSubStep: workflow.StatusSuccess},
	} {
		snapshot := stepStates(running)
		snapshot.Status = workflow.StatusRunning
		onChange(snapshot)
	}
	final := stepStates(map[string]string{
		streamedSetupStepID:   workflow.StatusSkipped,
		streamedFolderStepID:  workflow.StatusSuccess,
		streamedFolderSubStep: workflow.StatusSuccess,
	})
	final.Status = workflow.StatusSuccess
	return final
}

func TestStreamWorkflowRun_reportsDoneAfterEverySnapshot(t *testing.T) {
	progress := make(chan WorkflowMsg, workflowProgressBuffer)

	streamWorkflowRun(context.Background(), nil, rerunFolderStep, progress)

	var msgs []WorkflowMsg
	for len(progress) > 0 {
		msgs = append(msgs, <-progress)
	}
	if len(msgs) != 3 {
		t.Fatalf("got %d messages, want 2 snapshots and the final state", len(msgs))
	}
	for _, msg := range msgs[:2] {
		if msg.Done {
			t.Error("a snapshot was reported as done")
		}
	}
	if last := msgs[2]; !last.Done || last.State.Status != workflow.StatusSuccess {
		t.Errorf("last message = done %v status %q, want the final state reported done", last.Done, last.State.Status)
	}
}

func TestWorkflowMsg_partialRunEndsWithFinalStatesOverThePreviousRun(t *testing.T) {
	m := folderWorkflowDetailModel(t)
	previous := stepStates(map[string]string{
		streamedSetupStepID:   workflow.StatusSuccess,
		streamedFolderStepID:  workflow.StatusFailed,
		streamedFolderSubStep: workflow.StatusFailed,
	})
	previous.WorkflowID = m.activeWorkflow.ID
	m.workflowState = previous
	m.partialRunBase = &previous
	progress := make(chan WorkflowMsg, workflowProgressBuffer)
	m.workflowChan = progress

	go streamWorkflowRun(context.Background(), nil, rerunFolderStep, progress)
	for m.workflowChan != nil {
		updated, _ := m.Update(waitForWorkflow(m.workflowChan)())
		m = updated.(Model)
	}

	if len(progress) > 0 {
		t.Errorf("%d message(s) left after the run was reported done", len(progress))
	}
	want := map[string]string{
		streamedSetupStepID:   workflow.StatusSuccess,
		streamedFolderStepID:  workflow.StatusSuccess,
		streamedFolderSubStep: workflow.StatusSuccess,
	}
	for id, status := range want {
		if got := m.workflowState.StepIndex[id].Status; got != status {
			t.Errorf("step %q = %q, want %q", id, got, status)
		}
	}
}

// runToCompletion runs the workflow run cmd started and feeds its messages to
// Update until the run is reported done.
func runToCompletion(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	cmds := cmd().(tea.BatchMsg)
	go cmds[0]()
	for m.workflowChan != nil {
		updated, _ := m.Update(waitForWorkflow(m.workflowChan)())
		m = updated.(Model)
	}
	return m
}

func TestRunStepKey_onASubStepRollsItsFolderStepUpWithoutAPreviousRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	m := folderWorkflowDetailModel(t)
	invite := &m.collection.Items[0].Items[0].Items[1]
	invite.Items[0].Request.URL.Raw = server.URL
	m = cursorOnStep(t, m, "invite")
	m, _ = pressKey(m, expandStepKeyMsg)
	m = cursorOnStep(t, m, "invite/redeem")

	m, cmd := handleWorkflowRunStepKey(m)
	if cmd == nil {
		t.Fatalf("no run started: %s", m.statusMessage)
	}
	m = runToCompletion(t, m, cmd)

	if got := m.runStepState("invite/redeem").Status; got != workflow.StatusSuccess {
		t.Fatalf("invite/redeem = %s, want success", got)
	}
	if got := m.runStepState("invite").Status; got != workflow.StatusSuccess {
		t.Errorf("invite = %s, want rolled up from invite/redeem to success", got)
	}
}
