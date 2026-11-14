package workflow

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"postOffice/internal/engine"
	internalhttp "postOffice/internal/http"
	"postOffice/internal/postman"
)

// cancelTestTimeout fails a test whose cancellation never takes effect
// instead of letting it hang until the HTTP client times out.
const cancelTestTimeout = 5 * time.Second

func stepsAroundWorkflow(between string) *Workflow {
	return &Workflow{
		ID:     "cancel",
		Steps:  []Step{{ID: "first", Request: "first"}, {ID: "second", Request: "second"}},
		Script: fmt.Sprintf("export default async function (wf, pm) {\nawait wf.run(\"first\");\n%s\nawait wf.run(\"second\");\n}", between),
	}
}

func runCancellable(t *testing.T, ctx context.Context, collection *postman.Collection, wf *Workflow) ExecutionState {
	t.Helper()
	done := make(chan ExecutionState, 1)
	go func() {
		done <- NewRunner(collection, nil, engine.New(internalhttp.NewExecutor())).Run(ctx, wf, ExecutionOptions{}, nil)
	}()
	select {
	case state := <-done:
		return state
	case <-time.After(cancelTestTimeout):
		t.Fatal("run did not stop after cancellation")
		return ExecutionState{}
	}
}

func assertCancelledRun(t *testing.T, state ExecutionState, firstStatus string) {
	t.Helper()
	if state.Status != StatusStopped || state.Error != ErrRunCancelled.Error() {
		t.Errorf("run = %s (%q), want %s (%q)", state.Status, state.Error, StatusStopped, ErrRunCancelled)
	}
	if got := state.StepIndex["first"].Status; got != firstStatus {
		t.Errorf("first step = %s, want %s", got, firstStatus)
	}
	if got := state.StepIndex["second"].Status; got != StatusSkipped {
		t.Errorf("second step = %s, want %s", got, StatusSkipped)
	}
}

func TestRun_CancelAbandonsRequestInFlight(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var secondCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/second" {
			secondCalls.Add(1)
			return
		}
		cancel()
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	collection := &postman.Collection{Items: []postman.Item{
		statusRequest("first", server.URL+"/first"),
		statusRequest("second", server.URL+"/second"),
	}}

	state := runCancellable(t, ctx, collection, stepsAroundWorkflow(""))

	assertCancelledRun(t, state, StatusStopped)
	if secondCalls.Load() != 0 {
		t.Errorf("second request sent %d time(s), want none after cancelling", secondCalls.Load())
	}
}

func TestRun_CancelInterruptsSleep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/first" {
			time.AfterFunc(10*time.Millisecond, cancel)
		}
	}))
	t.Cleanup(server.Close)
	collection := &postman.Collection{Items: []postman.Item{
		statusRequest("first", server.URL+"/first"),
		statusRequest("second", server.URL+"/second"),
	}}
	sleepLongerThanTheTest := fmt.Sprintf("wf.sleep(%d);", 2*cancelTestTimeout.Milliseconds())

	state := runCancellable(t, ctx, collection, stepsAroundWorkflow(sleepLongerThanTheTest))

	assertCancelledRun(t, state, StatusSuccess)
}

func TestRun_CancelInterruptsSpinningScript(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(10*time.Millisecond, cancel)

	state := runCancellable(t, ctx, &postman.Collection{}, &Workflow{ID: "spin", Script: "export default async function (wf, pm) {\nwhile (true) {}\n}"})

	if state.Status != StatusStopped || state.Error != ErrRunCancelled.Error() {
		t.Errorf("run = %s (%q), want %s (%q)", state.Status, state.Error, StatusStopped, ErrRunCancelled)
	}
}

func TestRun_CancelledBeforeStartSendsNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	t.Cleanup(server.Close)
	collection := &postman.Collection{Items: []postman.Item{
		statusRequest("first", server.URL+"/first"),
		statusRequest("second", server.URL+"/second"),
	}}

	state := runCancellable(t, ctx, collection, stepsAroundWorkflow(""))

	if state.Status != StatusStopped || calls.Load() != 0 {
		t.Errorf("run = %s with %d request(s), want %s and none sent", state.Status, calls.Load(), StatusStopped)
	}
}
