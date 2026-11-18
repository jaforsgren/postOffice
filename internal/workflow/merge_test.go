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
	for _, id := range []string{"a", "outer", "outer/b"} {
		if merged.StepIndex[id] != previous.StepIndex[id] {
			t.Errorf("%s = %+v, want the previous run's result kept", id, merged.StepIndex[id])
		}
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
