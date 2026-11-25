package tui

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"postOffice/internal/workflow"
)

const (
	redeemStepID            = "invite/redeem"
	elapsedTestWindowWidth  = 200
	elapsedTestWindowHeight = 40
)

// runRedeemStep runs the invite/redeem sub-step alone against a local server,
// recording the run in runs.
func runRedeemStep(t *testing.T, runs *workflow.RunStore) Model {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(server.Close)
	m := folderWorkflowDetailModel(t).WithWorkflowRuns(runs)
	m.collection.Items[0].Items[0].Items[1].Items[0].Request.URL.Raw = server.URL
	m = cursorOnStep(t, m, "invite")
	m, _ = pressKey(m, expandStepKeyMsg)
	m = cursorOnStep(t, m, redeemStepID)

	m, cmd := handleWorkflowRunStepKey(m)
	if cmd == nil {
		t.Fatalf("no run started: %s", m.statusMessage)
	}
	return runToCompletion(t, m, cmd)
}

func TestWorkflowRuns_finishedRunIsShownAgainAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.json")
	ran := runRedeemStep(t, workflow.NewRunStore(path))

	reloaded := workflow.NewRunStore(path)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	m := folderWorkflowDetailModel(t).WithWorkflowRuns(reloaded)
	m, _ = m.openWorkflowDetail(m.activeWorkflow)

	if m.workflowState.Status != ran.workflowState.Status {
		t.Errorf("restored run status = %q, want %q", m.workflowState.Status, ran.workflowState.Status)
	}
	redeem := m.runStepState(redeemStepID)
	if redeem == nil || redeem.Status != workflow.StatusSuccess || redeem.LastResponse == nil {
		t.Fatalf("restored %s = %+v, want its successful run with the response", redeemStepID, redeem)
	}
	if _, started := redeem.Elapsed(time.Now()); !started || redeem.FinishedAt.IsZero() {
		t.Errorf("restored %s timing = %v..%v, want it kept", redeemStepID, redeem.StartedAt, redeem.FinishedAt)
	}
}

func TestWorkflowRuns_openingAWorkflowKeepsTheRunInProgress(t *testing.T) {
	runs := workflow.NewRunStore("")
	m := folderWorkflowDetailModel(t).WithWorkflowRuns(runs)
	key := m.workflowRunKeyOf(m.activeWorkflow, m.activeWorkflowCollection)
	_ = runs.Record(key, workflow.ExecutionState{WorkflowID: m.activeWorkflow.ID, Status: workflow.StatusFailed})
	m.workflowState = workflow.ExecutionState{WorkflowID: m.activeWorkflow.ID, Status: workflow.StatusRunning}
	m.workflowChan = make(chan WorkflowMsg)

	m, _ = m.openWorkflowDetail(m.activeWorkflow)

	if m.workflowState.Status != workflow.StatusRunning {
		t.Errorf("status = %q, want the run in progress kept", m.workflowState.Status)
	}
}

func TestWorkflowRuns_stepRowsShowElapsedTime(t *testing.T) {
	m := runRedeemStep(t, workflow.NewRunStore(""))
	m.width, m.height = elapsedTestWindowWidth, elapsedTestWindowHeight
	redeem := m.runStepState(redeemStepID)
	redeem.FinishedAt = redeem.StartedAt.Add(1500 * time.Millisecond)
	const want = "1.5s"

	if detail := m.renderWorkflowDetail(); !strings.Contains(detail, want) {
		t.Errorf("workflow detail does not show %s's elapsed %s:\n%s", redeemStepID, want, detail)
	}
	if report := strings.Join(runReportLines(m.workflowState, plainRunStyler), "\n"); !strings.Contains(report, want) {
		t.Errorf("run report does not show %s's elapsed %s:\n%s", redeemStepID, want, report)
	}
}

func TestWorkflowRuns_staleElapsedTickStops(t *testing.T) {
	m := folderWorkflowDetailModel(t)
	m.workflowChan = make(chan WorkflowMsg)
	m.workflowRunSeq = 2

	if _, cmd := m.handleWorkflowElapsedTick(workflowElapsedTickMsg{run: 1}); cmd != nil {
		t.Error("a tick of an earlier run kept ticking")
	}
	if _, cmd := m.handleWorkflowElapsedTick(workflowElapsedTickMsg{run: 2}); cmd == nil {
		t.Error("the run in progress stopped ticking")
	}
	m.workflowChan = nil
	if _, cmd := m.handleWorkflowElapsedTick(workflowElapsedTickMsg{run: 2}); cmd != nil {
		t.Error("ticking went on after the run finished")
	}
}

func TestFormatElapsed(t *testing.T) {
	for elapsed, want := range map[time.Duration]string{
		1234567 * time.Nanosecond: "1ms",
		250 * time.Millisecond:    "250ms",
		1500 * time.Millisecond:   "1.5s",
		83*time.Second + 260*1e6:  "1m23.3s",
	} {
		if got := formatElapsed(elapsed); got != want {
			t.Errorf("formatElapsed(%v) = %q, want %q", elapsed, got, want)
		}
	}
}
