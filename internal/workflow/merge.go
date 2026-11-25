package workflow

import (
	"slices"
	"strings"
	"time"
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
// its results and logs, every other step keeps the previous run's, and the
// folder steps enclosing a step it ran take their sub-steps' status (see
// rollUpFolderSteps). Steps the previous run never listed are added. The run
// status, error and summary are the partial run's.
func MergePartialRun(previous, partial ExecutionState) ExecutionState {
	ran := make(map[stepRun]*StepState)
	for _, ss := range partial.Steps {
		if ranInPart(ss) {
			ran[stepRunOf(ss)] = ss
		}
	}
	listed := make(map[stepRun]bool, len(previous.Steps))

	merged := partial
	merged.Steps = make([]*StepState, 0, len(previous.Steps))
	for _, ss := range previous.Steps {
		listed[stepRunOf(ss)] = true
		if replacement, ok := ran[stepRunOf(ss)]; ok {
			ss = replacement
		}
		merged.Steps = append(merged.Steps, ss)
	}
	for _, ss := range partial.Steps {
		if !listed[stepRunOf(ss)] {
			merged.Steps = insertAfterGroup(merged.Steps, ss)
		}
	}
	rollUpFolderSteps(merged.Steps, ran)

	merged.IndexSteps()
	merged.Logs = mergeLogs(previous.Logs, partial.Logs, merged.Steps, partial.Steps)
	return merged
}

// rolledUpStatusPriority orders the statuses a folder step can take from its
// sub-steps: the first one any sub-step has wins.
var rolledUpStatusPriority = []string{StatusRunning, StatusFailed, StatusStopped, StatusSuccess}

// rollUpFolderSteps gives each folder step enclosing a step the partial run
// ran, but not run by it itself, the status of its sub-steps. A step run alone
// inside a sub-workflow leaves its folder steps pending, though it is running
// or done. Rolled-up states are copies: the previous run's are left untouched.
func rollUpFolderSteps(steps []*StepState, ran map[stepRun]*StepState) {
	enclosing := make(map[stepRun]bool)
	for _, ss := range ran {
		for _, group := range ss.Groups {
			enclosing[stepRun{stepID: group, iteration: ss.Iteration}] = true
		}
	}
	// Sub-steps are listed after their folder step, so walking backwards rolls
	// a nested folder step up before the folder steps enclosing it.
	for i := len(steps) - 1; i >= 0; i-- {
		key := stepRunOf(steps[i])
		if _, ranItself := ran[key]; ranItself || !enclosing[key] {
			continue
		}
		if status := subStepsStatus(steps, steps[i]); status != "" {
			rolledUp := *steps[i]
			rolledUp.Status = status
			// The folder step itself did not run, so the previous run's timing would mislead.
			rolledUp.StartedAt, rolledUp.FinishedAt = time.Time{}, time.Time{}
			steps[i] = &rolledUp
		}
	}
}

// subStepsStatus is the highest-priority status among folder's direct
// sub-steps in the same iteration, or empty when none has one.
func subStepsStatus(steps []*StepState, folder *StepState) string {
	statuses := make(map[string]bool)
	for _, ss := range steps {
		if ss.Iteration == folder.Iteration && len(ss.Groups) > 0 && ss.Groups[len(ss.Groups)-1] == folder.ID {
			statuses[ss.Status] = true
		}
	}
	for _, status := range rolledUpStatusPriority {
		if statuses[status] {
			return status
		}
	}
	return ""
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
