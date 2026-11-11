package tui

import (
	"testing"

	"postOffice/internal/http"
	"postOffice/internal/workflow"

	tea "github.com/charmbracelet/bubbletea"
)

const fullWorkflowScript = "export default async function (wf, pm) { await wf.run(\"login\"); await wf.run(\"list\"); }"

var (
	runKeyMsg = tea.KeyMsg{Type: tea.KeyCtrlE}
	enterKey = tea.KeyMsg{Type: tea.KeyEnter}
)

func workflowKeysTestModel() Model {
	m := panelTestModel()
	m.activeWorkflow.Script = fullWorkflowScript
	m.workflows = []*workflow.Workflow{m.activeWorkflow}
	return m
}

func TestStartWorkflowPartial_KeepsTheFullWorkflowActive(t *testing.T) {
	m := workflowKeysTestModel()
	wf := m.activeWorkflow

	m, _ = pressKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	if m.workflowChan == nil {
		t.Fatal("expected r to start a run")
	}
	if m.activeWorkflow != wf || m.activeWorkflow.Script != fullWorkflowScript {
		t.Error("a partial run must not replace the active workflow or its script")
	}
}

func TestCtrlR_InWorkflowsListRunsSelectedWorkflow(t *testing.T) {
	m := workflowKeysTestModel()
	m.mode = ModeWorkflows

	m, _ = pressKey(m, runKeyMsg)

	if m.workflowChan == nil {
		t.Fatal("expected ctrl+e to start the selected workflow")
	}
	if m.mode != ModeWorkflowDetail {
		t.Errorf("mode = %v, want the workflow detail showing the run", m.mode)
	}
}

func TestEnter_InWorkflowsListOpensDetailWithoutRunning(t *testing.T) {
	m := workflowKeysTestModel()
	m.mode = ModeWorkflows

	m, _ = pressKey(m, enterKey)

	if m.mode != ModeWorkflowDetail || m.workflowChan != nil {
		t.Errorf("mode = %v running = %v, want detail opened without a run", m.mode, m.workflowChan != nil)
	}
}

func TestCtrlR_InWorkflowDetailRunsFullWorkflow(t *testing.T) {
	m := workflowKeysTestModel()

	m, _ = pressKey(m, runKeyMsg)

	if m.workflowChan == nil {
		t.Fatal("expected ctrl+e to run the workflow")
	}
	if m.mode != ModeWorkflowDetail {
		t.Errorf("mode = %v, want to stay in workflow detail", m.mode)
	}
}

func TestEnter_InWorkflowDetailShowsSelectedStepResponse(t *testing.T) {
	m := workflowKeysTestModel()
	want := m.workflowState.StepIndex[loginStepID].LastResponse

	m, _ = pressKey(m, enterKey)

	if m.mode != ModeResponse || m.lastResponse != want {
		t.Errorf("mode = %v, want the login step's response shown", m.mode)
	}
}

func TestCtrlR_InStepResponseResendsTheStep(t *testing.T) {
	m := workflowKeysTestModel()
	m, _ = pressKey(m, enterKey)

	m, _ = pressKey(m, runKeyMsg)

	if m.mode != ModeWorkflowDetail || m.workflowChan == nil {
		t.Errorf("mode = %v running = %v, want back in detail rerunning the step", m.mode, m.workflowChan != nil)
	}
}

func TestTopBarResponse_InWorkflowDetailFollowsSelectedStep(t *testing.T) {
	m := workflowKeysTestModel()
	m.lastResponse = &http.Response{StatusCode: 500, Status: "500 Internal Server Error"}

	response, _ := m.topBarResponse()
	if response != m.workflowState.StepIndex[loginStepID].LastResponse {
		t.Errorf("top bar response = %+v, want the login step's", response)
	}

	m.workflowStepCursor = 1
	if response, _ := m.topBarResponse(); response != nil {
		t.Errorf("top bar response = %+v, want none for a step that sent nothing", response)
	}
}

func TestGetModeString_NamesWorkflowModes(t *testing.T) {
	for _, mode := range []ViewMode{ModeWorkflows, ModeWorkflowDetail, ModeWorkflowRun, ModeGRPCReflect} {
		m := createTestModel()
		m.mode = mode
		if m.getModeString() == "" {
			t.Errorf("mode %v has no name in the top bar", mode)
		}
	}
}
