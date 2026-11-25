package workflow

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	internalhttp "postOffice/internal/http"
	"postOffice/internal/script"
)

// Workflow is a single YAML file describing execution steps and inline JavaScript.
type Workflow struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Version     int    `yaml:"version,omitempty"`
	Steps       []Step `yaml:"steps"`
	Script      string `yaml:"script"`
}

// Step is a named reference to a request, or to a folder whose workflow runs
// as a sub-workflow, within the collection. A step sets Request or Folder.
type Step struct {
	ID         string `yaml:"id"`
	Request    string `yaml:"request,omitempty"`     // collection path, e.g. "Auth/Login"
	Folder     string `yaml:"folder,omitempty"`      // collection path, e.g. "Auth/Admin"
	PreScript  string `yaml:"pre_script,omitempty"`  // JS executed before the request or sub-workflow
	PostScript string `yaml:"post_script,omitempty"` // JS executed after the request (receives `step.response`) or sub-workflow
}

// IsFolder reports whether the step runs a folder's workflow rather than a request.
func (s Step) IsFolder() bool { return s.Folder != "" }

// Target is the collection path the step runs: its folder or its request.
func (s Step) Target() string {
	if s.IsFolder() {
		return s.Folder
	}
	return s.Request
}

// SubStepSeparator joins a folder step's ID and the IDs of its sub-workflow's
// steps into the qualified IDs run states use, e.g. "admin/login". Step IDs
// are slugs, which never contain it.
const SubStepSeparator = "/"

// QualifiedStepID is the run state ID of stepID inside the sub-workflow of the
// folder step with run state ID groupID; groupID is empty at the top level.
func QualifiedStepID(groupID, stepID string) string {
	if groupID == "" {
		return stepID
	}
	return groupID + SubStepSeparator + stepID
}

// LocalStepID is the step's own ID within its workflow, without the IDs of
// the folder steps enclosing it.
func LocalStepID(qualifiedID string) string {
	return qualifiedID[strings.LastIndex(qualifiedID, SubStepSeparator)+1:]
}

// RunOptions controls optional wf.run() behavior. Repeat and Until apply to
// request steps; Entry to folder steps, whose sub-workflow then starts at the
// step running that request (the target of a setNextRequest jump).
type RunOptions struct {
	Repeat int
	Until  func(*StepResponse) bool
	Entry  string
}

// StepResponse is the response object exposed to workflow scripts.
type StepResponse struct {
	Code    int
	Headers map[string][]string
	Body    string
	Time    int64 // milliseconds
}

// JSON parses Body as JSON.
func (r *StepResponse) JSON() (any, error) {
	var out any
	if err := json.Unmarshal([]byte(r.Body), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Text returns the raw response body.
func (r *StepResponse) Text() string { return r.Body }

// StepRunResult is returned to the workflow script by wf.run().
type StepRunResult struct {
	Status    string          // StatusSuccess, StatusFailed or StatusSkipped
	Response  *StepResponse   // alias for Last
	Responses []*StepResponse // all repetition results
	Last      *StepResponse   // last repetition result
	// NextRequest is the last attempt's pm.execution.setNextRequest decision.
	NextRequest script.NextRequest
}

// Status constants for steps and the overall workflow.
const (
	StatusPending = "pending"
	StatusRunning = "running"
	StatusSuccess = "success"
	StatusFailed  = "failed"
	StatusSkipped = "skipped"
	StatusStopped = "stopped"
)

// StatusIcon returns the one-character icon shown for a step or run status.
func StatusIcon(status string) string {
	switch status {
	case StatusSuccess:
		return "✓"
	case StatusFailed:
		return "✗"
	case StatusRunning:
		return "▶"
	case StatusSkipped:
		return "−"
	case StatusStopped:
		return "■"
	default:
		return "○"
	}
}

// StepState tracks the runtime state of a single step.
type StepState struct {
	ID        string
	Request   string
	Status    string
	Progress  int // current repeat iteration (1-based)
	Total     int // planned repeat count
	Iteration int // run iteration this step belongs to (1-based)
	// Folder is set on a folder step: the folder whose workflow it runs.
	Folder string
	// Groups are the qualified IDs of the folder steps enclosing this step,
	// outermost first; empty for the run's top-level steps.
	Groups       []string
	LastResponse *internalhttp.Response `json:"-"`
	// LastResult holds the last attempt's tests, script errors and console
	// output; nil until the step's request has run.
	LastResult *script.TestResult `json:"-"`
	// StartedAt and FinishedAt bound the step's run, its scripts, repeats and
	// sub-workflow included; zero until it starts and while it runs.
	StartedAt  time.Time
	FinishedAt time.Time
}

// Elapsed is how long the step ran, up to now while it still runs. ok is
// false when it never started.
func (s StepState) Elapsed(now time.Time) (elapsed time.Duration, ok bool) {
	return elapsedBetween(s.StartedAt, s.FinishedAt, now)
}

func elapsedBetween(started, finished, now time.Time) (time.Duration, bool) {
	if started.IsZero() {
		return 0, false
	}
	if finished.IsZero() {
		return now.Sub(started), true
	}
	return finished.Sub(started), true
}

// IsFolder reports whether the step ran a folder's workflow rather than a request.
func (s StepState) IsFolder() bool { return s.Folder != "" }

// Target is the collection path the step runs: its folder or its request.
func (s StepState) Target() string {
	if s.IsFolder() {
		return s.Folder
	}
	return s.Request
}

// RunSummary counts a run's outcomes, like the totals at the end of a
// Postman Collection Runner run.
type RunSummary struct {
	StepsPassed  int
	StepsFailed  int
	StepsSkipped int
	TestsPassed  int
	TestsFailed  int
}

// String renders the summary as one line, e.g.
// "steps: 3 passed, 1 failed, 0 skipped · tests: 7 passed, 2 failed".
func (s RunSummary) String() string {
	return fmt.Sprintf("steps: %d passed, %d failed, %d skipped · tests: %d passed, %d failed",
		s.StepsPassed, s.StepsFailed, s.StepsSkipped, s.TestsPassed, s.TestsFailed)
}

// ExecutionState is the full runtime state of a workflow execution.
type ExecutionState struct {
	WorkflowID   string
	WorkflowName string
	Status       string
	Steps        []*StepState // ordered for display, every iteration's steps in turn
	// StepIndex is keyed by step ID and holds the latest iteration's state;
	// it is derived from Steps (see IndexSteps), so it is not persisted.
	StepIndex map[string]*StepState `json:"-"`
	Logs      []LogEntry
	Error     string
	// Iteration is the 1-based iteration in progress (or last run); Iterations the planned count.
	Iteration  int
	Iterations int
	// Summary is filled in when the run finishes.
	Summary RunSummary
	// StartedAt and FinishedAt bound the whole run; FinishedAt is zero while it runs.
	StartedAt  time.Time
	FinishedAt time.Time
}

// Elapsed is how long the run took, up to now while it still runs. ok is
// false when it never started.
func (s ExecutionState) Elapsed(now time.Time) (elapsed time.Duration, ok bool) {
	return elapsedBetween(s.StartedAt, s.FinishedAt, now)
}

// IndexSteps rebuilds StepIndex from Steps; later iterations replace earlier ones.
func (s *ExecutionState) IndexSteps() {
	s.StepIndex = make(map[string]*StepState, len(s.Steps))
	for _, ss := range s.Steps {
		s.StepIndex[ss.ID] = ss
	}
}

// IsMultiIteration reports whether steps need their iteration shown to be told apart.
func (s ExecutionState) IsMultiIteration() bool {
	return s.Iterations > defaultIterationCount
}

// Log message markers, shared with the run report so it can highlight them.
const (
	LogPassMarker        = "✓"
	LogFailMarker        = "✗"
	LogScriptErrorPrefix = "script error:"
)

// LogEntry is one line of run output. StepID names the step that produced it,
// or is empty for output of the workflow script itself. Iteration is the run
// iteration it came from (1-based), so a step's logs stay apart per iteration.
type LogEntry struct {
	StepID    string
	Iteration int
	Message   string
}
