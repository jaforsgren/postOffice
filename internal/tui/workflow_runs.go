package tui

import (
	"fmt"
	"postOffice/internal/workflow"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// workflowElapsedTickInterval is how often the run views redraw while a run is
// in progress, so running steps' elapsed times keep counting between updates.
const workflowElapsedTickInterval = time.Second

// workflowElapsedTickMsg redraws the run views of run, the workflowRunSeq of
// the run it was scheduled for, so a finished run's last tick cannot start a
// second ticker beside the next run's.
type workflowElapsedTickMsg struct{ run int }

// WithWorkflowRuns keeps each workflow's latest run in runs, so a run's
// results can be looked at again after a restart.
func (m Model) WithWorkflowRuns(runs *workflow.RunStore) Model {
	m.workflowRuns = runs
	return m
}

// latestWorkflowRun is wf's run in progress, or its latest finished run.
func (m Model) latestWorkflowRun(wf *workflow.Workflow) (workflow.ExecutionState, bool) {
	if m.workflowState.WorkflowID == wf.ID {
		return m.workflowState, true
	}
	return m.workflowRuns.Latest(m.workflowRunKeyOf(wf, m.workflowOwner(wf)))
}

// workflowOwner is the collection wf was listed from, else the one workflows resolve against.
func (m Model) workflowOwner(wf *workflow.Workflow) string {
	if owner, listed := m.workflowOwners[wf]; listed {
		return owner
	}
	return m.workflowCollectionName()
}

func (m Model) workflowRunKeyOf(wf *workflow.Workflow, collectionName string) workflow.RunKey {
	return workflow.RunKey{Collection: collectionName, WorkflowID: wf.ID}
}

// showLatestWorkflowRun shows the active workflow's latest recorded run,
// unless a run is in progress: its state must not be replaced.
func (m Model) showLatestWorkflowRun() Model {
	if m.workflowChan != nil || m.activeWorkflow == nil {
		return m
	}
	if run, ok := m.workflowRuns.Latest(m.workflowRunKeyOf(m.activeWorkflow, m.activeWorkflowCollection)); ok {
		m.workflowState = run
	}
	return m
}

// recordWorkflowRun keeps the finished run shown in the store. A failure to
// persist it is reported in the status bar, after the run's outcome.
func (m Model) recordWorkflowRun() Model {
	if err := m.workflowRuns.Record(m.workflowRunKey, m.workflowState); err != nil {
		m.statusMessage += fmt.Sprintf(" (run not saved: %v)", err)
	}
	return m
}

func tickWorkflowElapsed(run int) tea.Cmd {
	return tea.Tick(workflowElapsedTickInterval, func(time.Time) tea.Msg { return workflowElapsedTickMsg{run: run} })
}

// handleWorkflowElapsedTick redraws the run view while its run is in progress
// and stops ticking once it is done. Unlike progress updates it leaves the
// scroll position alone, so reading earlier output is not interrupted.
func (m Model) handleWorkflowElapsedTick(msg workflowElapsedTickMsg) (Model, tea.Cmd) {
	if m.workflowChan == nil || msg.run != m.workflowRunSeq {
		return m, nil
	}
	m.configureViewport(&m.workflowViewport, strings.Join(m.buildWorkflowRunLines(), "\n"))
	return m, tickWorkflowElapsed(msg.run)
}

// formatElapsed rounds an elapsed time for display: milliseconds below a
// second, tenths of a second above it.
func formatElapsed(elapsed time.Duration) string {
	if elapsed < time.Second {
		return elapsed.Round(time.Millisecond).String()
	}
	return elapsed.Round(elapsedDisplayPrecision).String()
}

const elapsedDisplayPrecision = 100 * time.Millisecond

// elapsedLabel is "  <elapsed>" for a started step or run, else empty.
func elapsedLabel(elapsed time.Duration, started bool) string {
	if !started {
		return ""
	}
	return "  " + formatElapsed(elapsed)
}

// stepElapsedLabel is how long ss ran (so far, while it runs), or empty when it never started.
func stepElapsedLabel(ss *workflow.StepState) string {
	return elapsedLabel(ss.Elapsed(time.Now()))
}

// runSummaryLine is a finished run's summary, followed by when it finished and how long it took.
func runSummaryLine(state workflow.ExecutionState) string {
	line := state.Summary.String()
	if elapsed, ok := state.Elapsed(time.Now()); ok {
		line += fmt.Sprintf(" · took %s · finished %s", formatElapsed(elapsed), state.FinishedAt.Local().Format(runFinishedTimeFormat))
	}
	return line
}

const runFinishedTimeFormat = "2006-01-02 15:04:05"
