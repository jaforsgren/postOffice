package workflow

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"postOffice/internal/engine"
	internalhttp "postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/script"
)

// recordingServer answers 200 to everything and records the request paths, in order.
type recordingServer struct {
	*httptest.Server
	mu   sync.Mutex
	hits []string
}

func newRecordingServer(t *testing.T) *recordingServer {
	t.Helper()
	rs := &recordingServer{}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rs.mu.Lock()
		rs.hits = append(rs.hits, strings.TrimPrefix(r.URL.Path, "/"))
		rs.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(rs.Close)
	return rs
}

func (rs *recordingServer) paths() string {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return strings.Join(rs.hits, ",")
}

func recordedRequest(name string, events ...postman.Event) postman.Item {
	return postman.Item{
		Name:    name,
		Request: &postman.Request{Method: "GET", URL: postman.URL{Raw: "{{base}}/" + name}},
		Events:  events,
	}
}

func testScript(lines ...string) postman.Event {
	return postman.Event{Listen: string(script.PhaseTest), Script: postman.Script{Exec: lines}}
}

func preRequestScript(lines ...string) postman.Event {
	return postman.Event{Listen: string(script.PhasePreRequest), Script: postman.Script{Exec: lines}}
}

func runWorkflowOver(t *testing.T, server *recordingServer, build func(*postman.Collection) (*Workflow, error), items ...postman.Item) ExecutionState {
	t.Helper()
	collection := &postman.Collection{
		Info:      postman.Info{Name: "Runner"},
		Variables: []postman.Variable{{Key: "base", Value: server.URL}},
		Items:     items,
	}
	wf, err := build(collection)
	if err != nil {
		t.Fatalf("unexpected error building workflow: %v", err)
	}
	return NewRunner(collection, nil, engine.New(internalhttp.NewExecutor())).Run(context.Background(), wf, ExecutionOptions{}, nil)
}

func runWholeCollection(t *testing.T, server *recordingServer, items ...postman.Item) ExecutionState {
	t.Helper()
	return runWorkflowOver(t, server, NewCollectionWorkflow, items...)
}

func TestCollectionWorkflow_runsEveryRequestInOrder(t *testing.T) {
	server := newRecordingServer(t)

	state := runWholeCollection(t, server,
		recordedRequest("a"),
		postman.Item{Name: "folder", Items: []postman.Item{recordedRequest("b")}},
		recordedRequest("c"),
	)

	if state.Status != StatusSuccess {
		t.Fatalf("status = %s (%s), want success", state.Status, state.Error)
	}
	if got := server.paths(); got != "a,b,c" {
		t.Errorf("requests = %s, want a,b,c", got)
	}
}

func TestFolderRun_setNextRequestJumpsByNameIDAndPath(t *testing.T) {
	server := newRecordingServer(t)
	retryOnceByID := testScript(
		"if (!pm.collectionVariables.get('retried')) {",
		"  pm.collectionVariables.set('retried', 'yes');",
		"  pm.execution.setNextRequest(pm.info.requestId);",
		"}",
	)

	state := runWholeCollection(t, server,
		recordedRequest("a", testScript("pm.execution.setNextRequest('c');")),
		recordedRequest("b"),
		postman.Item{Name: "folder", Items: []postman.Item{recordedRequest("c", retryOnceByID)}},
	)

	if state.Status != StatusSuccess {
		t.Fatalf("status = %s (%s), want success", state.Status, state.Error)
	}
	if got := server.paths(); got != "a,c,c" {
		t.Errorf("requests = %s, want a → c by name, then c again by its path id", got)
	}
}

func TestFolderRun_setNextRequestNullEndsTheRun(t *testing.T) {
	server := newRecordingServer(t)

	state := runWholeCollection(t, server,
		recordedRequest("a", testScript("pm.execution.setNextRequest(null);")),
		recordedRequest("b"),
	)

	if state.Status != StatusSuccess || server.paths() != "a" {
		t.Errorf("got %s (%s), want the run to stop successfully after a", server.paths(), state.Status)
	}
}

func TestFolderRun_preRequestRoutingWithSkip(t *testing.T) {
	server := newRecordingServer(t)

	state := runWholeCollection(t, server,
		recordedRequest("not in flow", preRequestScript("pm.execution.setNextRequest('in flow'); pm.execution.skipRequest();")),
		recordedRequest("skipped too"),
		recordedRequest("in flow"),
	)

	if got := server.paths(); got != "in flow" {
		t.Errorf("requests = %s, want only the routed request", got)
	}
	if state.Status != StatusSuccess {
		t.Errorf("status = %s (%s), want a skip not to fail the run", state.Status, state.Error)
	}
}

func TestFolderRun_unknownSetNextRequestTargetFailsTheRun(t *testing.T) {
	state := runWholeCollection(t, newRecordingServer(t), recordedRequest("a", testScript("pm.execution.setNextRequest('nowhere');")))

	if state.Status != StatusFailed || !strings.Contains(state.Error, `"nowhere" is not in this run`) {
		t.Errorf("got %s: %q, want a failed run naming the missing target", state.Status, state.Error)
	}
}

func TestFolderRun_selfLoopIsCappedByRetryLimit(t *testing.T) {
	server := newRecordingServer(t)

	state := runWholeCollection(t, server, recordedRequest("a", testScript("pm.execution.setNextRequest('a');")))

	if state.Status != StatusFailed {
		t.Errorf("got %s, want the retry limit to fail the run", state.Status)
	}
	if hits := len(strings.Split(server.paths(), ",")); hits != maxSelfRetries+1 {
		t.Errorf("sent %d requests, want %d (first send plus retries)", hits, maxSelfRetries+1)
	}
}

func TestFolderRun_endlessCycleBetweenRequestsIsStopped(t *testing.T) {
	server := newRecordingServer(t)

	state := runWholeCollection(t, server,
		recordedRequest("a", testScript("pm.execution.setNextRequest('b');")),
		recordedRequest("b", testScript("pm.execution.setNextRequest('a');")),
	)

	if state.Status != StatusFailed || !strings.Contains(state.Error, "setNextRequest loop") {
		t.Errorf("got %s: %q, want the loop guard to fail the run", state.Status, state.Error)
	}
	if hits := len(strings.Split(server.paths(), ",")); hits != MaxFolderRunRequests {
		t.Errorf("sent %d requests, want %d", hits, MaxFolderRunRequests)
	}
}

func TestFolderRun_stepKeepsLastScriptResult(t *testing.T) {
	state := runWholeCollection(t, newRecordingServer(t),
		recordedRequest("a", testScript("pm.test('is 200', () => pm.response.to.have.status(200));")),
	)

	result := state.Steps[0].LastResult
	if result == nil || len(result.Tests) != 1 || !result.Tests[0].Passed {
		t.Errorf("LastResult = %+v, want the passing test", result)
	}
}

func TestNewFolderPathWorkflow(t *testing.T) {
	collection := &postman.Collection{Items: []postman.Item{
		{Name: "outer", Items: []postman.Item{{Name: "inner", Items: []postman.Item{recordedRequest("a")}}}},
		recordedRequest("top"),
	}}

	wf, err := NewFolderPathWorkflow(collection, []string{"outer", "inner"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wf.Steps) != 1 || wf.Steps[0].Request != "outer/inner/a" || wf.Name != "outer/inner" {
		t.Errorf("got %+v", wf)
	}

	for _, path := range [][]string{{"missing"}, {"top"}, {"outer", "missing"}} {
		if _, err := NewFolderPathWorkflow(collection, path); !errors.Is(err, ErrFolderNotFound) {
			t.Errorf("%v: err = %v, want ErrFolderNotFound", path, err)
		}
	}
}

func TestNewRequestWorkflow(t *testing.T) {
	collection := &postman.Collection{Items: []postman.Item{
		{Name: "Users", Items: []postman.Item{recordedRequest("Create User")}},
		recordedRequest("Health"),
	}}

	wf, err := NewRequestWorkflow(collection, "Users/Create User")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wf.Steps) != 1 || wf.Steps[0].Request != "Users/Create User" || wf.Steps[0].ID != "create-user" {
		t.Errorf("got steps %+v", wf.Steps)
	}

	for _, path := range []string{"Users", "Users/Missing", "Health/x", "Nope"} {
		if _, err := NewRequestWorkflow(collection, path); !errors.Is(err, ErrRequestNotFound) {
			t.Errorf("%q: err = %v, want ErrRequestNotFound", path, err)
		}
	}
}

func TestRequestRefs(t *testing.T) {
	items := []postman.Item{
		recordedRequest("top"),
		{Name: "a/b", Items: []postman.Item{recordedRequest("inside")}},
		{Name: "ok", Items: []postman.Item{recordedRequest("x/y"), recordedRequest("fine")}},
	}

	refs := RequestRefs(items, []string{"root"})

	want := []string{`root/top`, `root/a\/b/inside`, `root/ok/x\/y`, `root/ok/fine`}
	if len(refs) != len(want) {
		t.Fatalf("got %d refs, want %d", len(refs), len(want))
	}
	for i, path := range want {
		if refs[i].Path != path {
			t.Errorf("refs[%d].Path = %q, want %q", i, refs[i].Path, path)
		}
	}
}

func TestNamesWithSeparatorRunInFolderAndRequestWorkflows(t *testing.T) {
	items := []postman.Item{
		{Name: "NEGATIVE", Items: []postman.Item{
			{Name: "a/b", Items: []postman.Item{recordedRequest("unknown policy 403/404")}},
		}},
	}

	t.Run("folder run", func(t *testing.T) {
		server := newRecordingServer(t)
		state := runWorkflowOver(t, server, func(c *postman.Collection) (*Workflow, error) {
			return NewFolderPathWorkflow(c, []string{"NEGATIVE"})
		}, items...)
		if state.Status != StatusSuccess || server.paths() != "unknown policy 403/404" {
			t.Errorf("status %s, hits %q", state.Status, server.paths())
		}
	})

	t.Run("single request by escaped path", func(t *testing.T) {
		server := newRecordingServer(t)
		state := runWorkflowOver(t, server, func(c *postman.Collection) (*Workflow, error) {
			return NewRequestWorkflow(c, `NEGATIVE/a\/b/unknown policy 403\/404`)
		}, items...)
		if state.Status != StatusSuccess || server.paths() != "unknown policy 403/404" {
			t.Errorf("status %s, hits %q", state.Status, server.paths())
		}
	})
}

func TestRequestWorkflow_ignoresSetNextRequest(t *testing.T) {
	server := newRecordingServer(t)

	state := runWorkflowOver(t, server, func(c *postman.Collection) (*Workflow, error) {
		return NewRequestWorkflow(c, "a")
	}, recordedRequest("a", testScript("pm.execution.setNextRequest('b');")), recordedRequest("b"))

	if state.Status != StatusSuccess || server.paths() != "a" {
		t.Errorf("got %s (%s), want only the requested request", server.paths(), state.Status)
	}
}
