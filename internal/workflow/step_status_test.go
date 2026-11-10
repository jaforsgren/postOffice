package workflow

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	internalhttp "postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/script"
)

const (
	okPath           = "ok"
	notFoundPath     = "missing"
	flakyPath        = "flaky"
	flakyFailures    = 2
	unreachableURL   = "http://127.0.0.1:1/down"
	statusVariable   = "step_status"
	passingTest      = "pm.test('is 200', function () { pm.response.to.have.status(200); });"
	failingTest      = "pm.test('always fails', function () { pm.expect(1).to.equal(2); });"
	throwingScript   = "throw new Error('boom');"
	skippingScript   = "pm.execution.skipRequest();"
	stepsFailedError = "one or more steps failed"
)

// statusServer answers okPath with 200, notFoundPath with 404, and flakyPath
// with 503 for its first flakyFailures calls and 200 after that.
func statusServer(t *testing.T) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	flakyCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimPrefix(r.URL.Path, "/") {
		case okPath:
			w.WriteHeader(http.StatusOK)
		case flakyPath:
			mu.Lock()
			flakyCalls++
			calls := flakyCalls
			mu.Unlock()
			if calls <= flakyFailures {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func statusRequest(name, url string, events ...postman.Event) postman.Item {
	return postman.Item{
		Name:    name,
		Request: &postman.Request{Method: "GET", URL: postman.URL{Raw: url}},
		Events:  events,
	}
}

func testEvent(lines ...string) postman.Event {
	return postman.Event{Listen: string(script.PhaseTest), Script: postman.Script{Exec: lines}}
}

func preRequestEvent(lines ...string) postman.Event {
	return postman.Event{Listen: string(script.PhasePreRequest), Script: postman.Script{Exec: lines}}
}

// runSteps runs one workflow step per item, in order, with workflowScript as
// the body of the workflow function (default: run every step once).
func runSteps(t *testing.T, workflowScript string, items ...postman.Item) (ExecutionState, *postman.Collection) {
	t.Helper()
	collection := &postman.Collection{Info: postman.Info{Name: "Status"}, Items: items}
	wf := &Workflow{ID: "status", Name: "Status"}
	var runAll []string
	for _, item := range items {
		wf.Steps = append(wf.Steps, Step{ID: item.Name, Request: item.Name})
		runAll = append(runAll, `await wf.run("`+item.Name+`");`)
	}
	if workflowScript == "" {
		workflowScript = strings.Join(runAll, "\n")
	}
	wf.Script = "export default async function (wf, pm) {\n" + workflowScript + "\n}"

	state := NewRunner(collection, nil, internalhttp.NewExecutor()).Run(wf, nil)
	return state, collection
}

func collectionVariable(collection *postman.Collection, key string) string {
	for _, v := range collection.Variables {
		if v.Key == key {
			return v.Value
		}
	}
	return ""
}

func TestRunStep_PassingTestsSucceed(t *testing.T) {
	server := statusServer(t)

	state, _ := runSteps(t, "", statusRequest("ok", server.URL+"/"+okPath, testEvent(passingTest)))

	if got := state.StepIndex["ok"].Status; got != StatusSuccess {
		t.Errorf("step status = %s, want %s", got, StatusSuccess)
	}
	if state.Status != StatusSuccess {
		t.Errorf("workflow status = %s (%s), want %s", state.Status, state.Error, StatusSuccess)
	}
}

func TestRunStep_FailingTestFailsStepAndWorkflowButScriptContinues(t *testing.T) {
	server := statusServer(t)

	state, collection := runSteps(t, `
		const result = await wf.run("failing");
		pm.collectionVariables.set("`+statusVariable+`", result.status);
		await wf.run("after");`,
		statusRequest("failing", server.URL+"/"+okPath, testEvent(failingTest)),
		statusRequest("after", server.URL+"/"+okPath),
	)

	if got := state.StepIndex["failing"].Status; got != StatusFailed {
		t.Errorf("failing step status = %s, want %s", got, StatusFailed)
	}
	if got := state.StepIndex["after"].Status; got != StatusSuccess {
		t.Errorf("a step after a failed one should still run, got %s", got)
	}
	if got := collectionVariable(collection, statusVariable); got != StatusFailed {
		t.Errorf("result.status seen by the workflow script = %q, want %q", got, StatusFailed)
	}
	if state.Status != StatusFailed || state.Error != stepsFailedError {
		t.Errorf("workflow = %s (%q), want %s (%q)", state.Status, state.Error, StatusFailed, stepsFailedError)
	}
}

func TestRunStep_FailureCases(t *testing.T) {
	server := statusServer(t)
	cases := map[string]postman.Item{
		"test script throws":        statusRequest("step", server.URL+"/"+okPath, testEvent(throwingScript)),
		"pre-request script throws": statusRequest("step", server.URL+"/"+okPath, preRequestEvent(throwingScript)),
		"request cannot be sent":    statusRequest("step", unreachableURL),
	}

	for name, item := range cases {
		t.Run(name, func(t *testing.T) {
			state, _ := runSteps(t, "", item)

			if got := state.StepIndex["step"].Status; got != StatusFailed {
				t.Errorf("step status = %s, want %s", got, StatusFailed)
			}
			if state.Status != StatusFailed {
				t.Errorf("workflow status = %s, want %s", state.Status, StatusFailed)
			}
		})
	}
}

func TestRunStep_HTTPErrorStatusWithoutTestsSucceeds(t *testing.T) {
	server := statusServer(t)

	state, _ := runSteps(t, "", statusRequest("missing", server.URL+"/"+notFoundPath))

	if got := state.StepIndex["missing"].Status; got != StatusSuccess {
		t.Errorf("a 404 with no tests should not fail the step, got %s", got)
	}
}

func TestRunStep_SkipRequestMarksStepSkipped(t *testing.T) {
	server := statusServer(t)

	state, _ := runSteps(t, "", statusRequest("skipped", server.URL+"/"+okPath, preRequestEvent(skippingScript)))

	if got := state.StepIndex["skipped"].Status; got != StatusSkipped {
		t.Errorf("step status = %s, want %s", got, StatusSkipped)
	}
	if state.Status != StatusSuccess {
		t.Errorf("a skipped step should not fail the workflow, got %s (%s)", state.Status, state.Error)
	}
}

func TestRunStep_LastAttemptOfPollDecidesStatus(t *testing.T) {
	server := statusServer(t)

	state, _ := runSteps(t, `await wf.run("flaky", { repeat: 5, until: (res) => res.code === 200 });`,
		statusRequest("flaky", server.URL+"/"+flakyPath, testEvent(passingTest)),
	)

	if got := state.StepIndex["flaky"].Status; got != StatusSuccess {
		t.Errorf("a poll that ends with a passing attempt should succeed, got %s", got)
	}
	if state.Status != StatusSuccess {
		t.Errorf("workflow status = %s (%s), want %s", state.Status, state.Error, StatusSuccess)
	}
}

const retrySelfScript = "if (pm.response.code !== 200) { pm.execution.setNextRequest(pm.info.requestId); }"

func TestRunStep_SetNextRequestOnItselfRetriesUntilReady(t *testing.T) {
	server := statusServer(t)

	state, _ := runSteps(t, "", statusRequest("flaky", server.URL+"/"+flakyPath, testEvent(retrySelfScript, passingTest)))

	if got := state.StepIndex["flaky"].Status; got != StatusSuccess {
		t.Errorf("a retried request whose last attempt passes should succeed, got %s", got)
	}
	if got := state.StepIndex["flaky"].LastResponse.StatusCode; got != http.StatusOK {
		t.Errorf("last response = %d, want %d", got, http.StatusOK)
	}
}

func TestRunStep_EndlessSetNextRequestOnItselfFails(t *testing.T) {
	server := statusServer(t)

	state, _ := runSteps(t, "", statusRequest("loop", server.URL+"/"+okPath, testEvent("pm.execution.setNextRequest(pm.info.requestName);")))

	if got := state.StepIndex["loop"].Status; got != StatusFailed {
		t.Errorf("a request that always retries itself should fail, got %s", got)
	}
}

func TestRunStep_SetNextRequestToAnotherRequestIsIgnored(t *testing.T) {
	server := statusServer(t)

	state, _ := runSteps(t, "",
		statusRequest("first", server.URL+"/"+okPath, testEvent("pm.execution.setNextRequest('third');")),
		statusRequest("second", server.URL+"/"+okPath),
	)

	if got := state.StepIndex["second"].Status; got != StatusSuccess {
		t.Errorf("the workflow script, not setNextRequest, should decide the order, got %s", got)
	}
}

func TestRun_TimeInsideStepsDoesNotCountAgainstTheScriptTimeout(t *testing.T) {
	const scriptTimeout = 100 * time.Millisecond
	const sleepMs = 150
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(scriptTimeout)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(slowServer.Close)
	collection := &postman.Collection{Items: []postman.Item{statusRequest("slow", slowServer.URL)}}
	wf := &Workflow{
		ID:     "slow",
		Steps:  []Step{{ID: "slow", Request: "slow"}},
		Script: fmt.Sprintf("export default async function (wf, pm) {\nawait wf.run(\"slow\");\nwf.sleep(%d);\nawait wf.run(\"slow\");\n}", sleepMs),
	}
	runner := NewRunner(collection, nil, internalhttp.NewExecutor())
	runner.scriptTimeout = scriptTimeout

	state := runner.Run(wf, nil)

	if state.Status != StatusSuccess {
		t.Errorf("workflow = %s (%s), want %s", state.Status, state.Error, StatusSuccess)
	}
}

func TestRun_SpinningWorkflowScriptTimesOut(t *testing.T) {
	runner := NewRunner(&postman.Collection{}, nil, internalhttp.NewExecutor())
	runner.scriptTimeout = 50 * time.Millisecond

	state := runner.Run(&Workflow{ID: "spin", Script: "export default async function (wf, pm) {\nwhile (true) {}\n}"}, nil)

	if state.Status != StatusFailed || !strings.Contains(state.Error, "timed out") {
		t.Errorf("workflow = %s (%q), want a timeout", state.Status, state.Error)
	}
}
