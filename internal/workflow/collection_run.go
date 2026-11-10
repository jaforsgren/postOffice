package workflow

import (
	"errors"
	"fmt"
	"strings"

	"postOffice/internal/postman"
	"postOffice/internal/script"
)

// MaxCollectionRunSteps stops a run whose pm.execution.setNextRequest calls
// loop forever (a retry that never becomes ready, a cycle between requests).
const MaxCollectionRunSteps = 1000

const collectionRunIDPrefix = "run:"

var (
	ErrFolderNotFound     = errors.New("folder not found")
	ErrNextRequestMissing = errors.New("setNextRequest target is not in this run")
	ErrRunStepLimit       = errors.New("collection run step limit reached")
)

// runEntry is one request in a collection run, in run order.
type runEntry struct {
	path string
	item *postman.Item
}

// RunName names the run of folderPath for display.
func RunName(collection *postman.Collection, folderPath []string) string {
	name := "Run: " + collection.Info.Name
	if len(folderPath) > 0 {
		name += requestPathSeparator + strings.Join(folderPath, requestPathSeparator)
	}
	return name
}

// RunCollection runs every request under folderPath (the whole collection
// when empty) in order, like Postman's Collection Runner: each request's
// collection → folder → request scripts run, and pm.execution.setNextRequest
// jumps to another request in the run by name or id, or ends the run with null.
func (r *Runner) RunCollection(folderPath []string, onChange func(ExecutionState)) ExecutionState {
	r.onChange = onChange
	r.status = StatusRunning
	r.state = ExecutionState{
		WorkflowID:   collectionRunIDPrefix + strings.Join(folderPath, requestPathSeparator),
		WorkflowName: RunName(r.collection, folderPath),
		Status:       StatusRunning,
		StepIndex:    make(map[string]*StepState),
	}
	r.ctx = &script.ExecutionContext{}
	r.syncFromCollection()
	r.notifyProgress()

	entries, err := r.runEntries(folderPath)
	if err != nil {
		return r.finalize(StatusFailed, err.Error())
	}

	failed := false
	for index, executed := 0, 0; index < len(entries); executed++ {
		if executed >= MaxCollectionRunSteps {
			return r.finalize(StatusFailed, fmt.Sprintf("%v: stopped after %d requests, likely a setNextRequest loop", ErrRunStepLimit, MaxCollectionRunSteps))
		}

		entry := entries[index]
		step := &StepState{ID: fmt.Sprintf("%d. %s", executed+1, entry.item.Name), Request: entry.path, Status: StatusRunning, Progress: 1, Total: 1}
		r.state.Steps = append(r.state.Steps, step)
		r.state.StepIndex[step.ID] = step
		r.notifyProgress()

		outcome, execErr := r.executeItem(entry.item, entry.path)
		if execErr != nil {
			step.Status = StatusFailed
			r.addLog(fmt.Sprintf("[%s] %v", entry.path, execErr))
			failed = true
			index++
			continue
		}
		step.LastResponse = outcome.raw
		step.Status = r.recordOutcome(entry.path, outcome)
		failed = failed || step.Status == StatusFailed
		r.notifyProgress()

		next, stop, nextErr := nextRunIndex(entries, index, outcome.scripts)
		if nextErr != nil {
			return r.finalize(StatusFailed, nextErr.Error())
		}
		if stop {
			break
		}
		index = next
	}

	if failed {
		return r.finalize(StatusFailed, "one or more requests failed")
	}
	return r.finalize(StatusSuccess, "")
}

func (r *Runner) runEntries(folderPath []string) ([]runEntry, error) {
	if r.collection == nil {
		return nil, fmt.Errorf("no collection loaded")
	}
	items := r.collection.Items
	for _, name := range folderPath {
		folder := findChildFolder(items, name)
		if folder == nil {
			return nil, fmt.Errorf("%w: %q", ErrFolderNotFound, strings.Join(folderPath, requestPathSeparator))
		}
		items = folder.Items
	}

	var entries []runEntry
	var walk func(items []postman.Item, parent []string)
	walk = func(items []postman.Item, parent []string) {
		for i := range items {
			item := &items[i]
			if item.IsRequest() {
				entries = append(entries, runEntry{path: script.RequestPath(parent, item.Name), item: item})
				continue
			}
			walk(item.Items, append(append([]string{}, parent...), item.Name))
		}
	}
	walk(items, folderPath)
	return entries, nil
}

func findChildFolder(items []postman.Item, name string) *postman.Item {
	for i := range items {
		if items[i].Name == name && !items[i].IsRequest() {
			return &items[i]
		}
	}
	return nil
}

// recordOutcome logs a request's test results and console output and returns
// its step status.
func (r *Runner) recordOutcome(path string, outcome itemOutcome) string {
	result := outcome.scripts
	if result != nil {
		for _, log := range result.ConsoleLogs {
			r.addLog(fmt.Sprintf("[%s] %s", path, log))
		}
		for _, test := range result.Tests {
			if test.Passed {
				r.addLog(fmt.Sprintf("[%s] ✓ %s", path, test.Name))
			} else {
				r.addLog(fmt.Sprintf("[%s] ✗ %s: %s", path, test.Name, test.Error))
			}
		}
		for _, scriptErr := range result.Errors {
			r.addLog(fmt.Sprintf("[%s] script error: %s", path, scriptErr))
		}
	}

	switch {
	case outcome.raw != nil && outcome.raw.Skipped:
		return StatusSkipped
	case outcome.raw != nil && outcome.raw.Error != nil, result != nil && result.HasFailures():
		return StatusFailed
	default:
		return StatusSuccess
	}
}

// nextRunIndex applies a setNextRequest decision: the matching entry by
// request name (first in run order), then by request id; null ends the run.
// Without a decision the run moves on to the next entry.
func nextRunIndex(entries []runEntry, current int, result *script.TestResult) (next int, stop bool, err error) {
	if result == nil || !result.NextRequest.Set {
		return current + 1, current+1 >= len(entries), nil
	}
	target := result.NextRequest.Name
	if target == "" {
		return 0, true, nil
	}
	for i, entry := range entries {
		if entry.item.Name == target {
			return i, false, nil
		}
	}
	for i, entry := range entries {
		if entry.item.ID == target || entry.path == target {
			return i, false, nil
		}
	}
	return 0, true, fmt.Errorf("%w: %q", ErrNextRequestMissing, target)
}
