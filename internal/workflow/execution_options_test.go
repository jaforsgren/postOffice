package workflow

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	internalhttp "postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/script"
)

func TestExecutionOptions_IterationCount(t *testing.T) {
	twoRows := []script.IterationData{{"a": "1"}, {"a": "2"}}
	cases := []struct {
		name string
		opts ExecutionOptions
		want int
	}{
		{"zero value runs once", ExecutionOptions{}, 1},
		{"explicit count", ExecutionOptions{Iterations: 3}, 3},
		{"one per data row", ExecutionOptions{Data: twoRows}, 2},
		{"explicit count beats rows", ExecutionOptions{Iterations: 5, Data: twoRows}, 5},
	}
	for _, tc := range cases {
		if got := tc.opts.IterationCount(); got != tc.want {
			t.Errorf("%s: IterationCount() = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestExecutionOptions_IterationReusesLastRow(t *testing.T) {
	opts := ExecutionOptions{Iterations: 3, Data: []script.IterationData{{"user": "a"}, {"user": "b"}}}

	for index, want := range []string{"a", "b", "b"} {
		it := opts.iteration(index)
		if it.Index != index || it.Count != 3 || it.Data["user"] != want {
			t.Errorf("iteration(%d) = %+v, want index %d of 3 with user %q", index, it, index, want)
		}
	}
	if (ExecutionOptions{}).iteration(0).Data != nil {
		t.Error("expected no data row without a data file")
	}
}

func TestExecutionOptions_Validate(t *testing.T) {
	if err := (ExecutionOptions{Iterations: -1}).Validate(); !errors.Is(err, ErrInvalidIterations) {
		t.Errorf("expected ErrInvalidIterations, got %v", err)
	}
	if err := (ExecutionOptions{Iterations: 2}).Validate(); err != nil {
		t.Errorf("expected valid options, got %v", err)
	}
}

func writeDataFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadIterationData(t *testing.T) {
	t.Run("csv rows keyed by header", func(t *testing.T) {
		rows, err := LoadIterationData(writeDataFile(t, "users.csv", "user,age\nalice,30\nbob,41\n"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rows) != 2 || rows[0]["user"] != "alice" || rows[1]["age"] != "41" {
			t.Errorf("unexpected rows %+v", rows)
		}
	})

	t.Run("json objects keep their types", func(t *testing.T) {
		rows, err := LoadIterationData(writeDataFile(t, "users.JSON", `[{"user":"alice","age":30,"admin":true}]`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rows) != 1 || rows[0]["age"] != float64(30) || rows[0]["admin"] != true {
			t.Errorf("unexpected rows %+v", rows)
		}
	})

	failures := []struct {
		name, file, content string
		want                error
	}{
		{"unsupported extension", "users.txt", "user\nalice", ErrUnsupportedDataFormat},
		{"csv header only", "empty.csv", "user,age\n", ErrNoDataRows},
		{"empty json array", "empty.json", "[]", ErrNoDataRows},
		{"json object instead of array", "object.json", `{"user":"alice"}`, ErrInvalidDataFile},
		{"ragged csv", "ragged.csv", "user,age\nalice\n", ErrInvalidDataFile},
	}
	for _, tc := range failures {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := LoadIterationData(writeDataFile(t, tc.file, tc.content)); !errors.Is(err, tc.want) {
				t.Errorf("expected %v, got %v", tc.want, err)
			}
		})
	}

	t.Run("missing file", func(t *testing.T) {
		if _, err := LoadIterationData(filepath.Join(t.TempDir(), "missing.csv")); err == nil {
			t.Error("expected an error for a missing file")
		}
	})
}

// pathRecorder answers 200 except for notFoundPath, recording request paths and queries.
type pathRecorder struct {
	*httptest.Server
	mu   sync.Mutex
	hits []string
}

func newPathRecorder(t *testing.T) *pathRecorder {
	t.Helper()
	recorder := &pathRecorder{}
	recorder.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.mu.Lock()
		recorder.hits = append(recorder.hits, strings.TrimPrefix(r.URL.RequestURI(), "/"))
		recorder.mu.Unlock()
		if strings.HasPrefix(r.URL.Path, "/"+notFoundPath) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(recorder.Close)
	return recorder
}

func (p *pathRecorder) paths() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return strings.Join(p.hits, ",")
}

// runWithOptions runs every item once per iteration with the given options.
func runWithOptions(t *testing.T, environment *postman.Environment, opts ExecutionOptions, items ...postman.Item) (ExecutionState, *postman.Collection) {
	t.Helper()
	collection := &postman.Collection{Info: postman.Info{Name: "Options"}, Items: items}
	wf := &Workflow{ID: "options", Name: "Options"}
	var runAll []string
	for _, item := range items {
		wf.Steps = append(wf.Steps, Step{ID: item.Name, Request: item.Name})
		runAll = append(runAll, `await wf.run("`+item.Name+`");`)
	}
	wf.Script = "export default async function (wf, pm) {\n" + strings.Join(runAll, "\n") + "\n}"
	return NewRunner(collection, environment, internalhttp.NewExecutor()).Run(wf, opts, nil), collection
}

func TestRun_IterationsRepeatEveryStep(t *testing.T) {
	server := newPathRecorder(t)

	state, _ := runWithOptions(t, nil, ExecutionOptions{Iterations: 2},
		statusRequest("a", server.URL+"/a"),
		statusRequest("b", server.URL+"/b"),
	)

	if state.Status != StatusSuccess {
		t.Fatalf("expected success, got %s: %s", state.Status, state.Error)
	}
	if got := server.paths(); got != "a,b,a,b" {
		t.Errorf("expected two passes, got %s", got)
	}
	if len(state.Steps) != 4 || state.Steps[0].Iteration != 1 || state.Steps[3].Iteration != 2 {
		t.Errorf("expected four steps tagged with their iteration, got %+v", state.Steps)
	}
	if state.Iteration != 2 || state.Iterations != 2 {
		t.Errorf("expected final iteration 2/2, got %d/%d", state.Iteration, state.Iterations)
	}
	if want := (RunSummary{StepsPassed: 4}); state.Summary != want {
		t.Errorf("Summary = %+v, want %+v", state.Summary, want)
	}
}

func TestRun_StopOnFailureEndsAtFirstFailedStep(t *testing.T) {
	server := newPathRecorder(t)
	items := []postman.Item{
		statusRequest("a", server.URL+"/a"),
		statusRequest("fails", server.URL+"/a", testEvent(failingTest)),
		statusRequest("c", server.URL+"/c"),
	}

	stopping, _ := runWithOptions(t, nil, ExecutionOptions{StopOnFailure: true, Iterations: 2}, items...)

	if got := server.paths(); got != "a,a" {
		t.Errorf("expected the run to stop after the failing step, got %s", got)
	}
	if stopping.Status != StatusFailed || !strings.Contains(stopping.Error, ErrStoppedOnFailure.Error()) || !strings.Contains(stopping.Error, `"fails"`) {
		t.Errorf("expected ErrStoppedOnFailure naming the step, got %s: %s", stopping.Status, stopping.Error)
	}
	if want := (RunSummary{StepsPassed: 1, StepsFailed: 1, StepsSkipped: 1, TestsFailed: 1}); stopping.Summary != want {
		t.Errorf("Summary = %+v, want %+v", stopping.Summary, want)
	}

	continuing, _ := runWithOptions(t, nil, ExecutionOptions{}, items...)
	if continuing.Status != StatusFailed || continuing.Error != ErrStepsFailed.Error() {
		t.Errorf("expected the default run to continue and fail at the end, got %s: %s", continuing.Status, continuing.Error)
	}
	if want := (RunSummary{StepsPassed: 2, StepsFailed: 1, TestsFailed: 1}); continuing.Summary != want {
		t.Errorf("Summary = %+v, want %+v", continuing.Summary, want)
	}
}

func TestRun_StopOnFailureCoversRequestErrorsAndScriptErrors(t *testing.T) {
	cases := map[string]func(server *pathRecorder) postman.Item{
		"request error": func(*pathRecorder) postman.Item { return statusRequest("fails", unreachableURL) },
		"script error": func(s *pathRecorder) postman.Item {
			return statusRequest("fails", s.URL+"/a", testEvent(throwingScript))
		},
		"http 404 tests": func(s *pathRecorder) postman.Item {
			return statusRequest("fails", s.URL+"/"+notFoundPath, testEvent(passingTest))
		},
	}
	for name, failing := range cases {
		t.Run(name, func(t *testing.T) {
			server := newPathRecorder(t)
			state, _ := runWithOptions(t, nil, ExecutionOptions{StopOnFailure: true},
				failing(server),
				statusRequest("after", server.URL+"/after"),
			)
			if state.Status != StatusFailed || !strings.Contains(state.Error, ErrStoppedOnFailure.Error()) {
				t.Errorf("expected a stop on failure, got %s: %s", state.Status, state.Error)
			}
			if strings.Contains(server.paths(), "after") {
				t.Errorf("expected no request after the failure, got %s", server.paths())
			}
		})
	}
}

func TestRun_DataRowsDriveVariablesAndScripts(t *testing.T) {
	server := newPathRecorder(t)
	environment := &postman.Environment{Name: "dev", Values: []postman.EnvVariable{
		{Key: "user", Value: "from-env", Enabled: true},
		{Key: "base", Value: server.URL, Enabled: true},
	}}
	opts := ExecutionOptions{Data: []script.IterationData{{"user": "alice", "age": float64(30)}, {"user": "bob", "age": float64(41)}}}
	recordIteration := testEvent(
		"pm.collectionVariables.set('seen_' + pm.info.iteration, [pm.iterationData.get('user'), pm.variables.get('user'), pm.info.iterationCount, typeof pm.iterationData.get('age'), pm.iterationData.has('missing')].join('|'));",
	)

	state, collection := runWithOptions(t, environment, opts,
		statusRequest("lookup", "{{base}}/users/{{user}}?age={{age}}", recordIteration),
	)

	if state.Status != StatusSuccess {
		t.Fatalf("expected success, got %s: %s", state.Status, state.Error)
	}
	if got := server.paths(); got != "users/alice?age=30,users/bob?age=41" {
		t.Errorf("expected data values above environment values in URLs, got %s", got)
	}
	for key, want := range map[string]string{
		"seen_0": "alice|alice|2|number|false",
		"seen_1": "bob|bob|2|number|false",
	} {
		if got := collectionVariable(collection, key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if environment.Values[0].Value != "from-env" {
		t.Error("expected data values not to overwrite the environment")
	}
}

func TestRun_SelectedEnvironmentIsUsed(t *testing.T) {
	server := newPathRecorder(t)
	staging := &postman.Environment{Name: "staging", Values: []postman.EnvVariable{{Key: "base", Value: server.URL + "/staging", Enabled: true}}}

	runWithOptions(t, staging, ExecutionOptions{}, statusRequest("ping", "{{base}}/ping"))

	if got := server.paths(); got != "staging/ping" {
		t.Errorf("expected the runner's environment to resolve {{base}}, got %s", got)
	}
}

func TestRun_InvalidOptionsFailBeforeAnyRequest(t *testing.T) {
	server := newPathRecorder(t)

	state, _ := runWithOptions(t, nil, ExecutionOptions{Iterations: -2}, statusRequest("a", server.URL+"/a"))

	if state.Status != StatusFailed || !strings.Contains(state.Error, ErrInvalidIterations.Error()) {
		t.Errorf("expected ErrInvalidIterations, got %s: %s", state.Status, state.Error)
	}
	if server.paths() != "" {
		t.Errorf("expected no requests, got %s", server.paths())
	}
}
