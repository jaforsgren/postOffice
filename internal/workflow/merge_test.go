package workflow

import (
	"slices"
	"testing"

	"postOffice/internal/postman"
)

func logsOf(state ExecutionState, stepID string) []string {
	var messages []string
	for _, entry := range state.Logs {
		if entry.StepID == stepID {
			messages = append(messages, entry.Message)
		}
	}
	return messages
}

func mergeTestItems() []postman.Item {
	return []postman.Item{
		recordedRequest("a", testScript("console.log('a ran');")),
		folder("outer", recordedRequest("b", testScript("console.log('b ran');")), recordedRequest("c")),
	}
}

func TestMergePartialRun_keepsTheResultsOfStepsItDidNotRun(t *testing.T) {
	server := newRecordingServer(t)
	previous := runWholeCollection(t, server, mergeTestItems()...)
	partial := runOnly(t, server, "outer/c", mergeTestItems()...)

	merged := MergePartialRun(previous, partial)

	if got, want := stepIDs(merged), stepIDs(previous); !slices.Equal(got, want) {
		t.Fatalf("steps = %v, want the previous run's %v", got, want)
	}
	for _, id := range []string{"a", "outer/b"} {
		if merged.StepIndex[id] != previous.StepIndex[id] {
			t.Errorf("%s = %+v, want the previous run's result kept", id, merged.StepIndex[id])
		}
	}
	if got := merged.StepIndex["outer"].Status; got != StatusSuccess {
		t.Errorf("outer = %s, want rolled up from its sub-steps to success", got)
	}
	if merged.StepIndex["outer/c"] != partial.StepIndex["outer/c"] {
		t.Errorf("outer/c should take the partial run's result")
	}
	if merged.StepIndex["a"].LastResponse == nil || !slices.Equal(logsOf(merged, "a"), logsOf(previous, "a")) {
		t.Errorf("a should keep its response and logs %v, got %v", logsOf(previous, "a"), logsOf(merged, "a"))
	}
}

func TestMergePartialRun_replacesTheLogsOfStepsItRan(t *testing.T) {
	server := newRecordingServer(t)
	previous := runWholeCollection(t, server, mergeTestItems()...)
	partial := runOnly(t, server, "a", mergeTestItems()...)

	merged := MergePartialRun(previous, partial)

	if got := logsOf(merged, "a"); !slices.Equal(got, logsOf(partial, "a")) {
		t.Errorf("a logs = %v, want only the partial run's %v", got, logsOf(partial, "a"))
	}
	if got := logsOf(merged, "outer/b"); !slices.Equal(got, logsOf(previous, "outer/b")) {
		t.Errorf("outer/b logs = %v, want the previous run's", got)
	}
	if merged.Status != partial.Status || merged.Summary != partial.Summary {
		t.Errorf("status %s/%s, want the partial run's outcome", merged.Status, merged.Summary)
	}
}

func TestMergePartialRun_placesStepsThePreviousRunNeverListedInTheirFolder(t *testing.T) {
	previous := ExecutionState{Steps: []*StepState{
		{ID: "outer", Folder: "outer", Status: StatusSuccess, Iteration: 1},
		{ID: "outer/b", Groups: []string{"outer"}, Status: StatusSuccess, Iteration: 1},
		{ID: "after", Status: StatusSuccess, Iteration: 1},
	}}
	added := &StepState{ID: "outer/new", Groups: []string{"outer"}, Status: StatusSuccess, Iteration: 1}
	partial := ExecutionState{Steps: []*StepState{added}}

	merged := MergePartialRun(previous, partial)

	if got := stepIDs(merged); !slices.Equal(got, []string{"outer", "outer/b", "outer/new", "after"}) {
		t.Errorf("steps = %v, want outer/new listed inside outer", got)
	}
}

// nestedRunStates are a run of a folder step "outer" holding a folder step
// "outer/inner", which holds "outer/inner/x"; "outer/b" is outer's other sub-step.
func nestedRunStates(outer, b, inner, x string) []*StepState {
	return []*StepState{
		{ID: "outer", Folder: "outer", Status: outer, Iteration: 1},
		{ID: "outer/b", Groups: []string{"outer"}, Status: b, Iteration: 1},
		{ID: "outer/inner", Folder: "inner", Groups: []string{"outer"}, Status: inner, Iteration: 1},
		{ID: "outer/inner/x", Groups: []string{"outer", "outer/inner"}, Status: x, Iteration: 1},
	}
}

func TestMergePartialRun_marksFolderStepsRunningUpToTheTopWhileASubStepRuns(t *testing.T) {
	previous := ExecutionState{Steps: nestedRunStates(StatusSuccess, StatusSuccess, StatusSuccess, StatusSuccess)}
	partial := ExecutionState{Steps: nestedRunStates(StatusPending, StatusPending, StatusPending, StatusRunning)}

	merged := MergePartialRun(previous, partial)

	for _, id := range []string{"outer", "outer/inner", "outer/inner/x"} {
		if got := merged.StepIndex[id].Status; got != StatusRunning {
			t.Errorf("%s = %s, want running", id, got)
		}
	}
	if got := merged.StepIndex["outer/b"].Status; got != StatusSuccess {
		t.Errorf("outer/b = %s, want the previous run's success", got)
	}
	if previous.Steps[0].Status != StatusSuccess {
		t.Error("rolling up must not change the previous run's states")
	}
}

func TestMergePartialRun_rollsFolderStepsUpFromEverySubStep(t *testing.T) {
	previous := ExecutionState{Steps: nestedRunStates(StatusFailed, StatusFailed, StatusFailed, StatusFailed)}
	partial := ExecutionState{Steps: nestedRunStates(StatusSkipped, StatusSkipped, StatusSkipped, StatusSuccess)}

	merged := MergePartialRun(previous, partial)

	if got := merged.StepIndex["outer/inner"].Status; got != StatusSuccess {
		t.Errorf("outer/inner = %s, want success: its only sub-step passed", got)
	}
	if got := merged.StepIndex["outer"].Status; got != StatusFailed {
		t.Errorf("outer = %s, want failed: outer/b still failed in the previous run", got)
	}
}

func TestMergePartialRun_withoutAPreviousRunListsEveryStepOfThePartialRun(t *testing.T) {
	partial := ExecutionState{Steps: nestedRunStates(StatusPending, StatusPending, StatusPending, StatusRunning)}

	merged := MergePartialRun(ExecutionState{}, partial)

	if got, want := stepIDs(merged), stepIDs(partial); !slices.Equal(got, want) {
		t.Errorf("steps = %v, want %v", got, want)
	}
	if got := merged.StepIndex["outer"].Status; got != StatusRunning {
		t.Errorf("outer = %s, want running", got)
	}
}
