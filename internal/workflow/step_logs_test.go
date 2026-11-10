package workflow

import (
	"strings"
	"testing"
)

const workflowLogMessage = "between steps"

func TestRunStep_LogsAreTaggedWithTheirStep(t *testing.T) {
	server := statusServer(t)

	state, _ := runSteps(t, `
		await wf.run("first");
		console.log("`+workflowLogMessage+`");
		await wf.run("second");`,
		statusRequest("first", server.URL+"/"+okPath, testEvent(passingTest)),
		statusRequest("second", server.URL+"/"+okPath, testEvent(failingTest)),
	)

	want := []LogEntry{
		{StepID: "first", Message: LogPassMarker + " is 200"},
		{StepID: "", Message: workflowLogMessage},
		{StepID: "second", Message: LogFailMarker + " always fails:"},
	}
	if len(state.Logs) != len(want) {
		t.Fatalf("logs = %+v, want %+v", state.Logs, want)
	}
	for i, entry := range want {
		got := state.Logs[i]
		if got.StepID != entry.StepID || !strings.HasPrefix(got.Message, entry.Message) {
			t.Errorf("log %d = %+v, want step %q with message starting %q", i, got, entry.StepID, entry.Message)
		}
	}
}

func TestRunStep_ErrorForUnknownRequestIsTaggedWithTheStep(t *testing.T) {
	state, _ := runSteps(t, `await wf.run("ghost");`)

	if len(state.Logs) != 1 || state.Logs[0].StepID != "ghost" {
		t.Fatalf("logs = %+v, want one entry for step ghost", state.Logs)
	}
}
