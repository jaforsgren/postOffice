package workflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"postOffice/internal/engine"
	internalhttp "postOffice/internal/http"
	"postOffice/internal/postman"
)

const (
	hangingPath     = "hang"
	abortRetryDelay = 5 * time.Millisecond
	abortTimeout    = 5 * time.Second
)

// hangingServer answers every path at once except "hang", which blocks until the request is cancelled.
func hangingServer(t *testing.T) *recordingServer {
	t.Helper()
	rs := &recordingServer{}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		rs.mu.Lock()
		rs.hits = append(rs.hits, path)
		rs.mu.Unlock()
		if path == hangingPath {
			<-r.Context().Done()
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(rs.Close)
	return rs
}

// runAborting runs the whole collection and aborts runID once it shows as running.
func runAborting(t *testing.T, server *recordingServer, runID string, items ...postman.Item) ExecutionState {
	t.Helper()
	collection := &postman.Collection{
		Info:      postman.Info{Name: "Runner"},
		Variables: []postman.Variable{{Key: "base", Value: server.URL}},
		Items:     items,
	}
	wf, err := NewCollectionWorkflow(collection)
	if err != nil {
		t.Fatal(err)
	}
	runner := NewRunner(collection, nil, engine.New(internalhttp.NewExecutor()))
	var once sync.Once
	onChange := func(state ExecutionState) {
		if ss := state.StepIndex[runID]; ss != nil && ss.Status == StatusRunning {
			once.Do(func() { go abortWhenRegistered(t, runner, runID) })
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), abortTimeout)
	defer cancel()
	return runner.Run(ctx, wf, ExecutionOptions{}, onChange)
}

// abortWhenRegistered retries Abort: a step shows as running a moment before its request starts.
func abortWhenRegistered(t *testing.T, runner *Runner, runID string) {
	deadline := time.Now().Add(abortTimeout)
	for !runner.Abort(runID) {
		if time.Now().After(deadline) {
			t.Errorf("%s never became abortable", runID)
			return
		}
		time.Sleep(abortRetryDelay)
	}
}

func TestAbort_requestStepStopsAndTheRunCarriesOn(t *testing.T) {
	server := hangingServer(t)

	state := runAborting(t, server, hangingPath, recordedRequest(hangingPath), recordedRequest("after"))

	if got := state.StepIndex[hangingPath].Status; got != StatusStopped {
		t.Errorf("aborted step status = %s, want stopped", got)
	}
	if got := state.StepIndex["after"].Status; got != StatusSuccess {
		t.Errorf("next step status = %s, want the run to carry on", got)
	}
	if state.Status != StatusFailed {
		t.Errorf("run status = %s, want failed: an abort is not a success", state.Status)
	}
	aborted := LogEntry{StepID: hangingPath, Iteration: 1, Message: "[ABORT] " + ErrStepAborted.Error()}
	if !slices.Contains(state.Logs, aborted) {
		t.Errorf("logs = %+v, want %+v", state.Logs, aborted)
	}
}

func TestAbort_folderStepEndsItsSubWorkflowAndTheRunCarriesOn(t *testing.T) {
	server := hangingServer(t)

	state := runAborting(t, server, "stage",
		folder("stage", recordedRequest(hangingPath), recordedRequest("never")),
		recordedRequest("after"),
	)

	if got := state.StepIndex["stage"].Status; got != StatusStopped {
		t.Errorf("aborted folder step status = %s, want stopped", got)
	}
	if got := state.StepIndex["stage/never"].Status; got != StatusSkipped {
		t.Errorf("rest of the sub-workflow = %s, want skipped", got)
	}
	if got := state.StepIndex["after"].Status; got != StatusSuccess {
		t.Errorf("next step status = %s, want the run to carry on", got)
	}
	if strings.Contains(server.paths(), "never") {
		t.Errorf("requests = %s, want nothing after the abort inside the stage", server.paths())
	}
}

func TestAbort_requestInsideASubWorkflowLeavesTheRestOfIt(t *testing.T) {
	server := hangingServer(t)

	state := runAborting(t, server, "stage/"+hangingPath,
		folder("stage", recordedRequest(hangingPath), recordedRequest("next")),
	)

	if got := state.StepIndex["stage/next"].Status; got != StatusSuccess {
		t.Errorf("next sub-step status = %s, want the sub-workflow to carry on", got)
	}
	if got := state.StepIndex["stage"].Status; got != StatusFailed {
		t.Errorf("folder step status = %s, want failed: a step inside it was aborted", got)
	}
}

func TestAbort_nothingRunningReportsFalse(t *testing.T) {
	runner := NewRunner(nil, nil, engine.New(internalhttp.NewExecutor()))

	if runner.Abort("nope") {
		t.Error("Abort of a step that is not running should report false")
	}
}
