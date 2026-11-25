package workflow

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	internalhttp "postOffice/internal/http"
	"postOffice/internal/script"
)

func finishedRunState() ExecutionState {
	started := time.Date(2026, 11, 25, 19, 0, 0, 0, time.UTC)
	login := &StepState{
		ID: "login", Request: "Auth/Login", Status: StatusSuccess, Iteration: 1,
		StartedAt: started, FinishedAt: started.Add(250 * time.Millisecond),
		LastResponse: &internalhttp.Response{StatusCode: 200, Status: "200 OK", Body: `{"token":"t"}`, Duration: 240 * time.Millisecond},
		LastResult:   &script.TestResult{Tests: []script.Test{{Name: "ok", Passed: true}}},
	}
	broken := &StepState{
		ID: "broken", Request: "Auth/Broken", Status: StatusFailed, Iteration: 1,
		StartedAt: started.Add(time.Second), FinishedAt: started.Add(2 * time.Second),
		LastResponse: &internalhttp.Response{Error: errors.New("connection refused")},
	}
	state := ExecutionState{
		WorkflowID: "auth", WorkflowName: "Auth", Status: StatusFailed, Iteration: 1, Iterations: 1,
		Steps:     []*StepState{login, broken},
		Logs:      []LogEntry{{StepID: "login", Iteration: 1, Message: "✓ ok"}},
		Summary:   RunSummary{StepsPassed: 1, StepsFailed: 1, TestsPassed: 1},
		StartedAt: started, FinishedAt: started.Add(2 * time.Second),
	}
	state.IndexSteps()
	return state
}

func TestRunStore_RecordedRunSurvivesReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), runStoreFileName)
	key := RunKey{Collection: "Collection", WorkflowID: "auth"}

	if err := NewRunStore(path).Record(key, finishedRunState()); err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	reloaded := NewRunStore(path)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	state, ok := reloaded.Latest(key)
	if !ok {
		t.Fatal("expected the recorded run after reload")
	}

	if state.Status != StatusFailed || state.Summary.StepsFailed != 1 || len(state.Logs) != 1 {
		t.Errorf("run outcome not restored: %+v", state)
	}
	if elapsed, _ := state.Elapsed(time.Now()); elapsed != 2*time.Second {
		t.Errorf("run elapsed = %v, want 2s", elapsed)
	}
	login := state.StepIndex["login"]
	if login == nil || login.LastResponse == nil || login.LastResponse.Body != `{"token":"t"}` {
		t.Fatalf("login response not restored: %+v", login)
	}
	if elapsed, _ := login.Elapsed(time.Now()); elapsed != 250*time.Millisecond {
		t.Errorf("login elapsed = %v, want 250ms", elapsed)
	}
	if login.LastResult == nil || len(login.LastResult.Tests) != 1 {
		t.Errorf("login test results not restored: %+v", login.LastResult)
	}
	broken := state.StepIndex["broken"]
	if broken == nil || broken.LastResponse == nil || broken.LastResponse.Error == nil || broken.LastResponse.Error.Error() != "connection refused" {
		t.Errorf("broken step's response error not restored: %+v", broken)
	}
}

func TestRunStore_KeepsWorkflowsOfOtherCollectionsApart(t *testing.T) {
	store := NewRunStore("")
	state := finishedRunState()
	_ = store.Record(RunKey{Collection: "A", WorkflowID: "auth"}, state)

	if _, ok := store.Latest(RunKey{Collection: "B", WorkflowID: "auth"}); ok {
		t.Error("expected no run for the same workflow ID in another collection")
	}
}

func TestRunStore_RecordKeepsRunsOfAnotherSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), runStoreFileName)
	first, second := NewRunStore(path), NewRunStore(path)

	_ = first.Record(RunKey{Collection: "C", WorkflowID: "one"}, finishedRunState())
	_ = second.Record(RunKey{Collection: "C", WorkflowID: "two"}, finishedRunState())

	reloaded := NewRunStore(path)
	_ = reloaded.Load()
	for _, id := range []string{"one", "two"} {
		if _, ok := reloaded.Latest(RunKey{Collection: "C", WorkflowID: id}); !ok {
			t.Errorf("expected run %q to be kept", id)
		}
	}
}

func TestRunStore_FileIsOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), runStoreFileName)
	_ = NewRunStore(path).Record(RunKey{Collection: "C", WorkflowID: "auth"}, finishedRunState())

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if info.Mode().Perm() != runStoreFileMode {
		t.Errorf("file mode = %v, want %v", info.Mode().Perm(), runStoreFileMode)
	}
}

func TestRunStore_LoadWithoutFileIsEmpty(t *testing.T) {
	store := NewRunStore(filepath.Join(t.TempDir(), runStoreFileName))
	if err := store.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if _, ok := store.Latest(RunKey{Collection: "C", WorkflowID: "auth"}); ok {
		t.Error("expected no runs")
	}
}

func TestElapsed(t *testing.T) {
	started := time.Date(2026, 11, 25, 19, 0, 0, 0, time.UTC)
	now := started.Add(3 * time.Second)

	if _, ok := (StepState{}).Elapsed(now); ok {
		t.Error("a step that never started has no elapsed time")
	}
	if elapsed, _ := (StepState{StartedAt: started}).Elapsed(now); elapsed != 3*time.Second {
		t.Errorf("running step elapsed = %v, want 3s", elapsed)
	}
	if elapsed, _ := (StepState{StartedAt: started, FinishedAt: started.Add(time.Second)}).Elapsed(now); elapsed != time.Second {
		t.Errorf("finished step elapsed = %v, want 1s", elapsed)
	}
}
