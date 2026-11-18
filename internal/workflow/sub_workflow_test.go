package workflow

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"postOffice/internal/engine"
	internalhttp "postOffice/internal/http"
	"postOffice/internal/postman"
)

func folder(name string, items ...postman.Item) postman.Item {
	return postman.Item{Name: name, Items: items}
}

func runWithFolderWorkflows(t *testing.T, server *recordingServer, wf *Workflow, source func(*postman.Collection) FolderWorkflowSource, items ...postman.Item) ExecutionState {
	t.Helper()
	collection := &postman.Collection{
		Info:      postman.Info{Name: "Runner"},
		Variables: []postman.Variable{{Key: "base", Value: server.URL}},
		Items:     items,
	}
	runner := NewRunner(collection, nil, engine.New(internalhttp.NewExecutor())).WithFolderWorkflows(source(collection))
	return runner.Run(context.Background(), wf, ExecutionOptions{}, nil)
}

func stepIDs(state ExecutionState) []string {
	ids := make([]string, len(state.Steps))
	for i, ss := range state.Steps {
		ids[i] = ss.ID
	}
	return ids
}

func TestSubWorkflow_runListsFolderStepsWithTheirStepsGrouped(t *testing.T) {
	server := newRecordingServer(t)

	state := runWholeCollection(t, server,
		recordedRequest("a"),
		folder("outer", recordedRequest("b"), folder("inner", recordedRequest("c"))),
	)

	if state.Status != StatusSuccess || server.paths() != "a,b,c" {
		t.Fatalf("got %s (%s) sending %s, want success sending a,b,c", state.Status, state.Error, server.paths())
	}
	wantIDs := []string{"a", "outer", "outer/b", "outer/inner", "outer/inner/c"}
	if got := stepIDs(state); !slices.Equal(got, wantIDs) {
		t.Fatalf("step IDs = %v, want %v", got, wantIDs)
	}
	inner := state.StepIndex["outer/inner/c"]
	if !slices.Equal(inner.Groups, []string{"outer", "outer/inner"}) || inner.Request != "outer/inner/c" {
		t.Errorf("outer/inner/c = %+v, want it grouped under outer and outer/inner", inner)
	}
	if outer := state.StepIndex["outer"]; outer.Folder != "outer" || outer.Status != StatusSuccess {
		t.Errorf("outer = %+v, want a succeeded folder step", outer)
	}
	if state.Summary.StepsPassed != 3 {
		t.Errorf("summary = %s, want only the 3 requests counted", state.Summary)
	}
}

func TestSubWorkflow_failedRequestFailsItsFolderStepsAndTheRun(t *testing.T) {
	state := runWholeCollection(t, newRecordingServer(t),
		folder("outer", folder("inner", recordedRequest("c", testScript("pm.test('fails', () => { throw new Error('no'); });")))),
		recordedRequest("after"),
	)

	if state.Status != StatusFailed {
		t.Errorf("status = %s, want failed", state.Status)
	}
	for _, id := range []string{"outer", "outer/inner", "outer/inner/c"} {
		if got := state.StepIndex[id].Status; got != StatusFailed {
			t.Errorf("%s status = %s, want failed", id, got)
		}
	}
	if got := state.StepIndex["after"].Status; got != StatusSuccess {
		t.Errorf("after status = %s, want the run to carry on", got)
	}
}

func TestSubWorkflow_setNextRequestEntersSiblingFolderAtTheTarget(t *testing.T) {
	server := newRecordingServer(t)

	state := runWholeCollection(t, server,
		folder("first", recordedRequest("a", testScript("pm.execution.setNextRequest('c');")), recordedRequest("skipped")),
		folder("second", recordedRequest("b"), folder("deep", recordedRequest("c"), recordedRequest("d"))),
	)

	if state.Status != StatusSuccess {
		t.Fatalf("status = %s (%s), want success", state.Status, state.Error)
	}
	if got := server.paths(); got != "a,c,d" {
		t.Errorf("requests = %s, want a, then second/deep entered at c", got)
	}
}

func TestSubWorkflow_setNextRequestNullInsideEndsTheWholeRun(t *testing.T) {
	server := newRecordingServer(t)

	state := runWholeCollection(t, server,
		folder("outer", recordedRequest("a", testScript("pm.execution.setNextRequest(null);")), recordedRequest("b")),
		recordedRequest("c"),
	)

	if state.Status != StatusSuccess || server.paths() != "a" {
		t.Errorf("got %s (%s) sending %s, want the run to end successfully after a", state.Status, state.Error, server.paths())
	}
}

func TestSubWorkflow_unknownTargetInsideFailsTheRun(t *testing.T) {
	state := runWholeCollection(t, newRecordingServer(t),
		folder("outer", recordedRequest("a", testScript("pm.execution.setNextRequest('nowhere');"))),
	)

	if state.Status != StatusFailed || !strings.Contains(state.Error, `"nowhere" is not in this run`) {
		t.Errorf("got %s: %q, want a failed run naming the missing target", state.Status, state.Error)
	}
}

func TestSubWorkflow_runsTheSavedWorkflowOfTheFolder(t *testing.T) {
	server := newRecordingServer(t)
	items := []postman.Item{recordedRequest("top"), folder("users", recordedRequest("list"), recordedRequest("create"))}
	customized := &Workflow{
		ID:     FolderWorkflowID([]string{"users"}),
		Steps:  []Step{{ID: "list", Request: "users/list"}, {ID: "create", Request: "users/create"}},
		Script: `export default async function(wf) { await wf.run("create"); }`,
	}
	parent := &Workflow{
		ID:     "runner",
		Steps:  []Step{{ID: "top", Request: "top"}, {ID: "users", Folder: "users"}},
		Script: `export default async function(wf) { await wf.run("top"); await wf.run("users"); }`,
	}

	state := runWithFolderWorkflows(t, server, parent, func(c *postman.Collection) FolderWorkflowSource {
		return SavedFolderWorkflows(c, []*Workflow{customized})
	}, items...)

	if state.Status != StatusSuccess || server.paths() != "top,create" {
		t.Errorf("got %s (%s) sending %s, want the customized users workflow to send only create", state.Status, state.Error, server.paths())
	}
}

func TestSubWorkflow_folderStepRunningItsOwnWorkflowFails(t *testing.T) {
	selfRunning := &Workflow{
		ID:     FolderWorkflowID([]string{"loop"}),
		Steps:  []Step{{ID: "again", Folder: "loop"}},
		Script: `export default async function(wf) { await wf.run("again"); }`,
	}
	parent := &Workflow{
		ID:     "runner",
		Steps:  []Step{{ID: "loop", Folder: "loop"}},
		Script: `export default async function(wf) { await wf.run("loop"); }`,
	}

	state := runWithFolderWorkflows(t, newRecordingServer(t), parent, func(c *postman.Collection) FolderWorkflowSource {
		return func([]string) (*Workflow, error) { return selfRunning, nil }
	}, folder("loop", recordedRequest("a")))

	if state.Status != StatusFailed {
		t.Errorf("status = %s, want failed", state.Status)
	}
	var logged bool
	for _, entry := range state.Logs {
		logged = logged || strings.Contains(entry.Message, ErrFolderCycle.Error())
	}
	if !logged {
		t.Errorf("logs = %+v, want the cycle reported", state.Logs)
	}
}

func TestSubWorkflow_unresolvableFolderFailsTheStep(t *testing.T) {
	wantErr := errors.New("no such folder")
	parent := &Workflow{
		ID:     "runner",
		Steps:  []Step{{ID: "gone", Folder: "gone"}},
		Script: `export default async function(wf) { await wf.run("gone"); }`,
	}

	state := runWithFolderWorkflows(t, newRecordingServer(t), parent, func(c *postman.Collection) FolderWorkflowSource {
		return func([]string) (*Workflow, error) { return nil, wantErr }
	})

	if state.Status != StatusFailed || state.StepIndex["gone"].Status != StatusFailed {
		t.Errorf("got run %s, step %s, want both failed", state.Status, state.StepIndex["gone"].Status)
	}
}

func TestSubWorkflow_scriptLogsGoToTheFolderStep(t *testing.T) {
	sub := &Workflow{
		ID:     FolderWorkflowID([]string{"users"}),
		Steps:  []Step{{ID: "list", Request: "users/list"}},
		Script: `export default async function(wf) { console.log("inside users"); await wf.run("list"); }`,
	}
	parent := &Workflow{
		ID:     "runner",
		Steps:  []Step{{ID: "users", Folder: "users"}},
		Script: `export default async function(wf) { await wf.run("users"); }`,
	}

	state := runWithFolderWorkflows(t, newRecordingServer(t), parent, func(c *postman.Collection) FolderWorkflowSource {
		return func([]string) (*Workflow, error) { return sub, nil }
	}, folder("users", recordedRequest("list")))

	want := LogEntry{StepID: "users", Iteration: 1, Message: "inside users"}
	if !slices.Contains(state.Logs, want) {
		t.Errorf("logs = %+v, want %+v", state.Logs, want)
	}
}

func TestQualifiedAndLocalStepID(t *testing.T) {
	if got := QualifiedStepID("", "login"); got != "login" {
		t.Errorf("QualifiedStepID top = %q", got)
	}
	qualified := QualifiedStepID("outer/inner", "login")
	if qualified != "outer/inner/login" || LocalStepID(qualified) != "login" || LocalStepID("login") != "login" {
		t.Errorf("qualified %q, local %q", qualified, LocalStepID(qualified))
	}
}
