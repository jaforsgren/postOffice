package workflow

import (
	"slices"
	"strings"
)

// stepRun identifies one step's run: a step ID in one run iteration.
type stepRun struct {
	stepID    string
	iteration int
}

func stepRunOf(ss *StepState) stepRun { return stepRun{stepID: ss.ID, iteration: ss.Iteration} }

// ranInPart reports whether a partial run ran the step; a partial run leaves
// the steps it does not run pending, then marks them skipped.
func ranInPart(ss *StepState) bool {
	return ss.Status != StatusPending && ss.Status != StatusSkipped
}

// MergePartialRun lays a partial run (one step, up to or from a step) over
// the previous run of the same workflow: the steps the partial run ran take
// its results and logs, every other step keeps the previous run's. The run
// status, error and summary are the partial run's.
func MergePartialRun(previous, partial ExecutionState) ExecutionState {
	ran := make(map[stepRun]*StepState)
	for _, ss := range partial.Steps {
		if ranInPart(ss) {
			ran[stepRunOf(ss)] = ss
		}
	}

	merged := partial
	merged.Steps = make([]*StepState, 0, len(previous.Steps))
	for _, ss := range previous.Steps {
		if replacement, ok := ran[stepRunOf(ss)]; ok {
			ss = replacement
			delete(ran, stepRunOf(ss))
		}
		merged.Steps = append(merged.Steps, ss)
	}
	for _, ss := range partial.Steps {
		if _, unlisted := ran[stepRunOf(ss)]; unlisted {
			merged.Steps = insertAfterGroup(merged.Steps, ss)
		}
	}

	merged.StepIndex = make(map[string]*StepState, len(merged.Steps))
	for _, ss := range merged.Steps {
		merged.StepIndex[ss.ID] = ss
	}
	merged.Logs = mergeLogs(previous.Logs, partial.Logs, merged.Steps, partial.Steps)
	return merged
}

// insertAfterGroup places a step the previous run never listed after the last
// step of its enclosing folder step, or at the end when that is not listed either.
func insertAfterGroup(steps []*StepState, ss *StepState) []*StepState {
	if len(ss.Groups) == 0 {
		return append(steps, ss)
	}
	group := ss.Groups[len(ss.Groups)-1]
	last := -1
	for i, listed := range steps {
		if listed.ID == group || strings.HasPrefix(listed.ID, group+SubStepSeparator) {
			last = i
		}
	}
	if last < 0 {
		return append(steps, ss)
	}
	return slices.Insert(steps, last+1, ss)
}

// mergeLogs keeps the previous run's logs of the steps still showing its
// results and the workflow script's, then adds the partial run's logs of the
// steps it ran and of its script.
func mergeLogs(previous, partial []LogEntry, merged, partialSteps []*StepState) []LogEntry {
	fromPartial := make(map[stepRun]bool)
	for _, ss := range partialSteps {
		if slices.Contains(merged, ss) {
			fromPartial[stepRunOf(ss)] = true
		}
	}
	var logs []LogEntry
	for _, entry := range previous {
		if !fromPartial[stepRun{stepID: entry.StepID, iteration: entry.Iteration}] {
			logs = append(logs, entry)
		}
	}
	for _, entry := range partial {
		if entry.StepID == "" || fromPartial[stepRun{stepID: entry.StepID, iteration: entry.Iteration}] {
			logs = append(logs, entry)
		}
	}
	return logs
}
