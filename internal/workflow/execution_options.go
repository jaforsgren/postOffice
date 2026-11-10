package workflow

import (
	"errors"
	"fmt"

	"postOffice/internal/script"
)

const defaultIterationCount = 1

var (
	// ErrInvalidIterations is returned for a negative iteration count.
	ErrInvalidIterations = errors.New("iterations must be zero (automatic) or positive")
	// ErrStoppedOnFailure ends a run with StopOnFailure set at its first failed step.
	ErrStoppedOnFailure = errors.New("stopped on first failure")
)

// ExecutionOptions configures a whole workflow run, as Postman's Collection
// Runner options do. (RunOptions, by contrast, configures one wf.run() call
// inside a workflow script.) The zero value runs once and keeps going after
// failures. The environment a run uses is the one passed to NewRunner.
type ExecutionOptions struct {
	// StopOnFailure ends the run at the first failed step: a request that
	// errors, a failing test or a script error. Remaining steps are skipped.
	StopOnFailure bool
	// Iterations is how many times the workflow runs. Zero means one
	// iteration per Data row, or a single iteration without Data.
	Iterations int
	// Data drives a data-driven run: iteration i uses row i, and once rows
	// run out the last row is reused. See LoadIterationData.
	Data []script.IterationData
}

// Validate reports options that cannot be run.
func (o ExecutionOptions) Validate() error {
	if o.Iterations < 0 {
		return fmt.Errorf("%w: %d", ErrInvalidIterations, o.Iterations)
	}
	return nil
}

// IterationCount is the number of iterations the run will perform.
func (o ExecutionOptions) IterationCount() int {
	switch {
	case o.Iterations > 0:
		return o.Iterations
	case len(o.Data) > 0:
		return len(o.Data)
	default:
		return defaultIterationCount
	}
}

// iteration describes pass index (0-based) for scripts and variable resolution.
func (o ExecutionOptions) iteration(index int) script.Iteration {
	it := script.Iteration{Index: index, Count: o.IterationCount()}
	if len(o.Data) > 0 {
		it.Data = o.Data[min(index, len(o.Data)-1)]
	}
	return it
}
