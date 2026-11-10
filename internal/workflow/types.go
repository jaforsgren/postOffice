package workflow

import (
	"encoding/json"
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

// Step is a named reference to a request within the collection.
type Step struct {
	ID         string `yaml:"id"`
	Request    string `yaml:"request"`               // collection path, e.g. "Auth/Login"
	PreScript  string `yaml:"pre_script,omitempty"`  // JS executed before the request
	PostScript string `yaml:"post_script,omitempty"` // JS executed after the request, receives `step.response`
}

// RunOptions controls optional wf.run() behavior.
type RunOptions struct {
	Repeat int
	Until  func(*StepResponse) bool
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
	ID           string
	Request      string
	Status       string
	Progress     int                    // current repeat iteration (1-based)
	Total        int                    // planned repeat count
	LastResponse *internalhttp.Response `json:"-"`
	// LastResult holds the last attempt's tests, script errors and console
	// output; nil until the step's request has run.
	LastResult *script.TestResult `json:"-"`
}

// ExecutionState is the full runtime state of a workflow execution.
type ExecutionState struct {
	WorkflowID   string
	WorkflowName string
	Status       string
	Steps        []*StepState          // ordered for display
	StepIndex    map[string]*StepState // keyed by step ID
	Logs         []LogEntry
	Error        string
}

// Log message markers, shared with the run report so it can highlight them.
const (
	LogPassMarker        = "✓"
	LogFailMarker        = "✗"
	LogScriptErrorPrefix = "script error:"
)

// LogEntry is one line of run output. StepID names the step that produced it,
// or is empty for output of the workflow script itself.
type LogEntry struct {
	StepID  string
	Message string
}
