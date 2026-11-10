package workflow

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	internalhttp "postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/script"
)

// recordingServer answers every request with 200 and records the paths hit.
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

func runRequest(name string, events ...postman.Event) postman.Item {
	return postman.Item{
		Name:    name,
		Request: &postman.Request{Method: "GET", URL: postman.URL{Raw: "{{base}}/" + name}},
		Events:  events,
	}
}

func onTest(lines ...string) postman.Event {
	return postman.Event{Listen: string(script.PhaseTest), Script: postman.Script{Exec: lines}}
}

func onPreRequest(lines ...string) postman.Event {
	return postman.Event{Listen: string(script.PhasePreRequest), Script: postman.Script{Exec: lines}}
}

func runCollection(t *testing.T, server *recordingServer, folderPath []string, items ...postman.Item) ExecutionState {
	t.Helper()
	collection := &postman.Collection{
		Info:      postman.Info{Name: "Runner"},
		Variables: []postman.Variable{{Key: "base", Value: server.URL}},
		Items:     items,
	}
	return NewRunner(collection, nil, internalhttp.NewExecutor()).RunCollection(folderPath, nil)
}

func TestRunCollection_RunsRequestsInOrderIncludingNestedFolders(t *testing.T) {
	server := newRecordingServer(t)

	state := runCollection(t, server, nil,
		runRequest("a"),
		postman.Item{Name: "folder", Items: []postman.Item{runRequest("b"), {Name: "deeper", Items: []postman.Item{runRequest("c")}}}},
		runRequest("d"),
	)

	if state.Status != StatusSuccess {
		t.Fatalf("expected success, got %s: %s", state.Status, state.Error)
	}
	if got := server.paths(); got != "a,b,c,d" {
		t.Errorf("expected a,b,c,d, got %s", got)
	}
	if state.Steps[2].Request != "folder/deeper/c" {
		t.Errorf("expected step request path folder/deeper/c, got %q", state.Steps[2].Request)
	}
}

func TestRunCollection_ScopedToFolder(t *testing.T) {
	server := newRecordingServer(t)

	runCollection(t, server, []string{"folder"},
		runRequest("a"),
		postman.Item{Name: "folder", Items: []postman.Item{runRequest("b")}},
	)

	if got := server.paths(); got != "b" {
		t.Errorf("expected only the folder's request, got %s", got)
	}
}

func TestRunCollection_SetNextRequestJumpsByNameAndId(t *testing.T) {
	server := newRecordingServer(t)
	retryOnce := onTest(
		"if (!pm.collectionVariables.get('retried')) {",
		"  pm.collectionVariables.set('retried', 'yes');",
		"  pm.execution.setNextRequest(pm.info.requestId);",
		"}",
	)

	state := runCollection(t, server, nil,
		runRequest("a", onTest("pm.execution.setNextRequest('c');")),
		runRequest("b"),
		postman.Item{Name: "folder", Items: []postman.Item{runRequest("c", retryOnce)}},
	)

	if state.Status != StatusSuccess {
		t.Fatalf("expected success, got %s: %s", state.Status, state.Error)
	}
	if got := server.paths(); got != "a,c,c" {
		t.Errorf("expected a → c (by name), c again (by path id), got %s", got)
	}
}

func TestRunCollection_SetNextRequestNullEndsTheRun(t *testing.T) {
	server := newRecordingServer(t)

	state := runCollection(t, server, nil,
		runRequest("a", onTest("pm.execution.setNextRequest(null);")),
		runRequest("b"),
	)

	if state.Status != StatusSuccess || server.paths() != "a" {
		t.Errorf("expected the run to stop after a, got %s (%s)", server.paths(), state.Status)
	}
}

func TestRunCollection_PreRequestRoutingAndSkip(t *testing.T) {
	server := newRecordingServer(t)

	state := runCollection(t, server, nil,
		runRequest("not in flow", onPreRequest("pm.execution.setNextRequest('in flow'); pm.execution.skipRequest();")),
		runRequest("skipped too"),
		runRequest("in flow"),
	)

	if got := server.paths(); got != "in flow" {
		t.Errorf("expected only the routed request to be sent, got %s", got)
	}
	if state.Steps[0].Status != StatusSkipped {
		t.Errorf("expected the routing request to be marked skipped, got %s", state.Steps[0].Status)
	}
	if state.Status != StatusSuccess {
		t.Errorf("expected a skip not to fail the run, got %s", state.Status)
	}
}

func TestRunCollection_FailedTestsFailTheRunButItContinues(t *testing.T) {
	server := newRecordingServer(t)

	state := runCollection(t, server, nil,
		runRequest("a", onTest("pm.test('status is 404', () => pm.expect(pm.response.code).to.eql(404));")),
		runRequest("b"),
	)

	if state.Status != StatusFailed {
		t.Errorf("expected failed run, got %s", state.Status)
	}
	if server.paths() != "a,b" {
		t.Errorf("expected the run to continue after a failure, got %s", server.paths())
	}
	if state.Steps[0].Status != StatusFailed || state.Steps[1].Status != StatusSuccess {
		t.Errorf("unexpected step statuses %s, %s", state.Steps[0].Status, state.Steps[1].Status)
	}
	if !strings.Contains(strings.Join(state.Logs, "\n"), "✗ status is 404") {
		t.Errorf("expected the failed test in the logs, got %v", state.Logs)
	}
}

func TestRunCollection_Errors(t *testing.T) {
	t.Run("unknown setNextRequest target", func(t *testing.T) {
		state := runCollection(t, newRecordingServer(t), nil, runRequest("a", onTest("pm.execution.setNextRequest('nowhere');")))
		if state.Status != StatusFailed || !strings.Contains(state.Error, ErrNextRequestMissing.Error()) {
			t.Errorf("expected ErrNextRequestMissing, got %s: %s", state.Status, state.Error)
		}
	})

	t.Run("unknown folder", func(t *testing.T) {
		state := runCollection(t, newRecordingServer(t), []string{"missing"}, runRequest("a"))
		if state.Status != StatusFailed || !strings.Contains(state.Error, ErrFolderNotFound.Error()) {
			t.Errorf("expected ErrFolderNotFound, got %s: %s", state.Status, state.Error)
		}
	})

	t.Run("endless setNextRequest loop", func(t *testing.T) {
		state := runCollection(t, newRecordingServer(t), nil, runRequest("a", onTest("pm.execution.setNextRequest('a');")))
		if state.Status != StatusFailed || !strings.Contains(state.Error, ErrRunStepLimit.Error()) {
			t.Errorf("expected ErrRunStepLimit, got %s: %s", state.Status, state.Error)
		}
		if len(state.Steps) != MaxCollectionRunSteps {
			t.Errorf("expected %d steps before stopping, got %d", MaxCollectionRunSteps, len(state.Steps))
		}
	})
}

func TestNextRunIndex(t *testing.T) {
	entries := []runEntry{
		{path: "a", item: &postman.Item{Name: "a"}},
		{path: "f/b", item: &postman.Item{Name: "b", ID: "id-b"}},
	}
	cases := []struct {
		name     string
		result   *script.TestResult
		wantNext int
		wantStop bool
		wantErr  error
	}{
		{"no scripts", nil, 1, false, nil},
		{"no decision at the end", &script.TestResult{}, 0, true, nil},
		{"by name", &script.TestResult{NextRequest: script.NextRequest{Set: true, Name: "b"}}, 1, false, nil},
		{"by id", &script.TestResult{NextRequest: script.NextRequest{Set: true, Name: "id-b"}}, 1, false, nil},
		{"by path", &script.TestResult{NextRequest: script.NextRequest{Set: true, Name: "f/b"}}, 1, false, nil},
		{"null", &script.TestResult{NextRequest: script.NextRequest{Set: true}}, 0, true, nil},
		{"missing", &script.TestResult{NextRequest: script.NextRequest{Set: true, Name: "x"}}, 0, true, ErrNextRequestMissing},
	}
	for _, tc := range cases {
		current := 0
		if tc.name == "no decision at the end" {
			current = 1
		}
		next, stop, err := nextRunIndex(entries, current, tc.result)
		if !errors.Is(err, tc.wantErr) || stop != tc.wantStop || (!stop && next != tc.wantNext) {
			t.Errorf("%s: got next=%d stop=%v err=%v", tc.name, next, stop, err)
		}
	}
}
