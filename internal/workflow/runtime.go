package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
	"postOffice/internal/engine"
	"postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/script"
)

const (
	// defaultScriptTimeout bounds each stretch of workflow script between steps.
	// Time inside steps and wf.sleep is not counted: requests bound themselves.
	defaultScriptTimeout = 30 * time.Second
	// maxSelfRetries stops a request whose script always calls
	// pm.execution.setNextRequest on itself from looping forever.
	maxSelfRetries = 100
)

// Runner executes a workflow against a collection and environment.
type Runner struct {
	collection  *postman.Collection
	environment *postman.Environment
	engine      *engine.Engine
	// runCtx cancels the run: the request being sent, wf.sleep and the script.
	runCtx context.Context

	// frame is the workflow script running: the run's workflow, or the
	// sub-workflow of the folder step it is inside.
	frame   *frame
	options ExecutionOptions
	// iteration is the pass in progress; every request in it sees its data row.
	iteration script.Iteration
	// anyStepFailed fails the whole run at the end, while still letting the
	// workflow script carry on after a failed step (unless StopOnFailure).
	anyStepFailed bool
	ctx           *script.ExecutionContext
	state         ExecutionState
	status        string
	onChange      func(ExecutionState)
	// currentStepID attributes logs to the step being run (its qualified ID);
	// empty between steps.
	currentStepID string
	scriptTimeout time.Duration

	folderSource FolderWorkflowSource
	// folderWorkflows caches folderSource per folder path for the run.
	folderWorkflows map[string]*Workflow

	// abortMu guards abortable, which Abort reads from another goroutine.
	abortMu sync.Mutex
	// abortable cancels a running request step or folder step, by run state ID.
	abortable map[string]context.CancelFunc
}

// frame is one workflow script being run: the run's workflow, or a folder
// step's sub-workflow inside it. Each runs in its own VM with its own timeout.
type frame struct {
	parent *frame
	// ctx ends when the run is cancelled or this sub-workflow (or one enclosing it) is aborted.
	ctx   context.Context
	steps []Step
	// group is the folder step this frame runs the sub-workflow of; nil at the top.
	group *StepState
	// folders are the folder paths of this and the enclosing frames, to refuse
	// a sub-workflow that runs itself.
	folders []string
	// entry is the request the sub-workflow starts at, empty to start at its first step.
	entry string
	// jump is the setNextRequest target the script handed to the enclosing workflow (wf.jump).
	jump script.NextRequest
	// failed is set when any step inside the frame failed.
	failed bool

	vm          *goja.Runtime
	loop        *script.EventLoop
	scriptTimer *time.Timer
	timedOut    atomic.Bool
}

// qualify is the run state ID of the frame's step stepID.
func (f *frame) qualify(stepID string) string {
	return QualifiedStepID(f.groupID(), stepID)
}

// groupID is the folder step's run state ID; empty at the top or before a run.
func (f *frame) groupID() string {
	if f == nil || f.group == nil {
		return ""
	}
	return f.group.ID
}

// groups are the Groups of the frame's step states.
func (f *frame) groups() []string {
	if f.group == nil {
		return nil
	}
	return append(slices.Clone(f.group.Groups), f.group.ID)
}

// NewRunner creates a runner bound to the given collection and environment.
// Folder steps run generated folder workflows unless WithFolderWorkflows says otherwise.
func NewRunner(
	collection *postman.Collection,
	environment *postman.Environment,
	requestEngine *engine.Engine,
) *Runner {
	return &Runner{
		collection:    collection,
		environment:   environment,
		engine:        requestEngine,
		runCtx:        context.Background(),
		scriptTimeout: defaultScriptTimeout,
		folderSource:  GeneratedFolderWorkflows(collection),
	}
}

// WithFolderWorkflows sets where folder steps get the workflow they run.
func (r *Runner) WithFolderWorkflows(source FolderWorkflowSource) *Runner {
	r.folderSource = source
	return r
}

var (
	// ErrStepsFailed is the run error when the workflow script finished but at
	// least one step failed.
	ErrStepsFailed = errors.New("one or more steps failed")
	// ErrWorkflowTimeout is the run error when one iteration's script runs too long.
	ErrWorkflowTimeout = errors.New("workflow timed out")
	// ErrScriptFailed is the run error when the workflow script itself throws.
	ErrScriptFailed = errors.New("script error")
	// ErrRunCancelled is the run error when the run's context is cancelled.
	ErrRunCancelled = errors.New("run cancelled")
	// ErrStepAborted is logged on a step stopped with Abort.
	ErrStepAborted = errors.New("aborted by the user, the run carries on")
	// ErrNestedStepNotFound is the step error when a qualified step ID names no step inside the folder steps.
	ErrNestedStepNotFound = errors.New("no such step inside the folder steps")
	// ErrFolderCycle is the step error when a folder step's sub-workflow would run itself.
	ErrFolderCycle = errors.New("folder workflow runs itself")
	// errStageAborted ends the steps and script of a sub-workflow whose folder step was aborted.
	errStageAborted = errors.New("aborted")
	// errJumped ends a workflow script that handed a setNextRequest target to
	// its enclosing workflow; the script completed, it did not fail.
	errJumped = errors.New("workflow.jump")
)

// Run executes wf once per iteration of opts, calling onChange after each
// state change. Collection and environment variables carry over between
// iterations; each iteration gets a fresh script VM and its own step states.
// Cancelling ctx stops the run: the request being sent is abandoned and the
// run ends as stopped with ErrRunCancelled.
func (r *Runner) Run(ctx context.Context, wf *Workflow, opts ExecutionOptions, onChange func(ExecutionState)) ExecutionState {
	r.runCtx = ctx
	r.onChange = onChange
	r.status = StatusRunning
	r.anyStepFailed = false
	r.options = opts
	r.frame = &frame{ctx: ctx, steps: wf.Steps}
	r.abortable = make(map[string]context.CancelFunc)
	r.folderWorkflows = make(map[string]*Workflow)
	r.state = ExecutionState{
		WorkflowID:   wf.ID,
		WorkflowName: wf.Name,
		Status:       StatusRunning,
		StepIndex:    make(map[string]*StepState),
		Iterations:   opts.IterationCount(),
		StartedAt:    time.Now(),
	}
	if err := opts.Validate(); err != nil {
		return r.finalize(StatusFailed, err.Error())
	}
	r.ctx = &script.ExecutionContext{}
	r.syncFromCollection()

	for index := range r.state.Iterations {
		r.beginIteration(index)
		if err := r.runIteration(wf); err != nil {
			return r.finalize(StatusFailed, err.Error())
		}
		if r.status == StatusFailed || r.status == StatusStopped {
			return r.finalize(r.status, r.state.Error)
		}
	}

	if r.anyStepFailed {
		return r.finalize(StatusFailed, ErrStepsFailed.Error())
	}
	return r.finalize(StatusSuccess, "")
}

// beginIteration selects the iteration's data row and adds a pending state
// for each declared step, so the run view lists every iteration's steps,
// those inside folder steps' sub-workflows included.
func (r *Runner) beginIteration(index int) {
	r.iteration = r.options.iteration(index)
	r.ctx.Iteration = r.iteration
	r.state.Iteration = index + 1
	r.addPendingSteps(r.frame.steps, nil, nil)
	r.notifyProgress()
}

// addPendingSteps adds a pending state for each of steps, enclosed by the
// folder steps groups names, then for the steps of each folder step's
// sub-workflow. folders are the enclosing folder paths, so a sub-workflow
// that runs itself is listed once; running it fails the step.
func (r *Runner) addPendingSteps(steps []Step, groups, folders []string) {
	for _, step := range steps {
		ss := &StepState{
			ID:      QualifiedStepID(lastOf(groups), step.ID),
			Request: step.Request,
			Folder:  step.Folder,
			Groups:  groups,
			Status:  StatusPending,
		}
		r.addStepState(ss)
		if !step.IsFolder() || slices.Contains(folders, step.Folder) {
			continue
		}
		if sub, err := r.folderWorkflow(step.Folder); err == nil {
			r.addPendingSteps(sub.Steps, append(slices.Clone(groups), ss.ID), append(slices.Clone(folders), step.Folder))
		}
	}
}

func lastOf(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

func (r *Runner) addStepState(ss *StepState) {
	ss.Iteration = r.state.Iteration
	r.state.Steps = append(r.state.Steps, ss)
	r.state.StepIndex[ss.ID] = ss
}

// runIteration runs the workflow script once in a fresh VM. Steps that fail
// (or wf.fail/wf.stop) are reported through r.status, not the returned error,
// which is reserved for the script itself not running to completion.
func (r *Runner) runIteration(wf *Workflow) error {
	r.frame = &frame{ctx: r.runCtx, steps: wf.Steps}
	return r.runScript(wf.Script)
}

// runScript runs src as r.frame's workflow script in a fresh VM, with its own
// timeout. It returns an error only when the script did not run to completion.
func (r *Runner) runScript(src string) error {
	f := r.frame
	f.vm = goja.New()

	if err := r.setupPmAPI(); err != nil {
		return fmt.Errorf("pm API setup failed: %w", err)
	}

	// After the pm API, which installs its own console: the workflow's must win
	// so console output lands in the run log.
	if err := r.setupConsole(); err != nil {
		return fmt.Errorf("console setup failed: %w", err)
	}

	if err := script.SetupCryptoJS(f.vm); err != nil {
		return fmt.Errorf("CryptoJS setup failed: %w", err)
	}

	if err := r.setupWFAPI(); err != nil {
		return fmt.Errorf("wf API setup failed: %w", err)
	}

	transformed := transformScript(src)

	f.timedOut.Store(false)
	f.scriptTimer = time.AfterFunc(r.scriptTimeout, func() {
		f.timedOut.Store(true)
		f.vm.Interrupt("workflow timeout")
	})
	stopInterruptOnCancel := context.AfterFunc(f.ctx, func() { f.vm.Interrupt(errStageAborted) })

	_, err := f.vm.RunString(transformed)
	if f.jump.Set {
		err = nil
	}
	if err == nil {
		for _, callbackErr := range f.loop.Drain() {
			r.addLog(fmt.Sprintf("[SCRIPT ERROR] %v", callbackErr))
		}
	}
	f.scriptTimer.Stop()
	stopInterruptOnCancel()

	if f.timedOut.Load() {
		return fmt.Errorf("%w after %v", ErrWorkflowTimeout, r.scriptTimeout)
	}
	if r.runCtx.Err() != nil {
		r.markCancelled()
		return nil
	}
	if f.ctx.Err() != nil {
		return errStageAborted
	}

	if err != nil && r.status != StatusFailed && r.status != StatusStopped {
		return fmt.Errorf("%w: %v", ErrScriptFailed, err)
	}
	return nil
}

// failRun fails the run with reason and returns the error to panic with,
// which ends the workflow script.
func (r *Runner) failRun(reason string) *goja.Object {
	r.markRunFailed(reason)
	return r.frame.vm.NewGoError(fmt.Errorf("workflow.fail: %s", reason))
}

// markRunFailed fails the run with reason; later wf.run() calls return skipped.
func (r *Runner) markRunFailed(reason string) {
	r.status = StatusFailed
	r.state.Status = StatusFailed
	r.state.Error = reason
	r.addLog(fmt.Sprintf("[FAIL] %s", reason))
	r.notifyProgress()
}

// stop ends the run as stopped; later wf.run() calls return skipped.
func (r *Runner) stop(reason string) {
	r.status = StatusStopped
	r.state.Status = StatusStopped
	r.state.Error = reason
	r.addLog(fmt.Sprintf("[STOP] %s", reason))
	r.notifyProgress()
}

func (r *Runner) finalize(status, errMsg string) ExecutionState {
	r.status = status
	r.state.Status = status
	r.state.Error = errMsg
	r.state.FinishedAt = time.Now()

	for _, ss := range r.state.Steps {
		if ss.Status == StatusPending || ss.Status == StatusRunning {
			ss.Status = StatusSkipped
		}
		if !ss.StartedAt.IsZero() && ss.FinishedAt.IsZero() {
			ss.FinishedAt = r.state.FinishedAt
		}
	}
	r.summarizeSteps()

	r.notifyProgress()
	return r.state
}

// summarizeSteps counts every iteration's final request step statuses into
// the summary; folder steps only group them. Test counts accumulate as steps
// finish (see countTests).
func (r *Runner) summarizeSteps() {
	summary := &r.state.Summary
	summary.StepsPassed, summary.StepsFailed, summary.StepsSkipped = 0, 0, 0
	for _, ss := range r.state.Steps {
		if ss.IsFolder() {
			continue
		}
		switch ss.Status {
		case StatusSuccess:
			summary.StepsPassed++
		case StatusFailed:
			summary.StepsFailed++
		case StatusSkipped:
			summary.StepsSkipped++
		}
	}
}

// countTests adds a finished step's test results to the summary.
func (r *Runner) countTests(result *script.TestResult) {
	if result == nil {
		return
	}
	for _, test := range result.Tests {
		if test.Passed {
			r.state.Summary.TestsPassed++
		} else {
			r.state.Summary.TestsFailed++
		}
	}
}

func (r *Runner) notifyProgress() {
	if r.onChange == nil {
		return
	}
	snap := ExecutionState{
		WorkflowID:   r.state.WorkflowID,
		WorkflowName: r.state.WorkflowName,
		Status:       r.state.Status,
		Error:        r.state.Error,
		Logs:         append([]LogEntry{}, r.state.Logs...),
		StepIndex:    make(map[string]*StepState),
		Iteration:    r.state.Iteration,
		Iterations:   r.state.Iterations,
		Summary:      r.state.Summary,
		StartedAt:    r.state.StartedAt,
		FinishedAt:   r.state.FinishedAt,
	}
	for _, ss := range r.state.Steps {
		cp := *ss
		snap.Steps = append(snap.Steps, &cp)
		snap.StepIndex[cp.ID] = &cp
	}
	r.onChange(snap)
}

// addLog records msg against the step currently running; between steps of a
// sub-workflow, against its folder step.
func (r *Runner) addLog(msg string) {
	stepID := r.currentStepID
	if stepID == "" {
		stepID = r.frame.groupID()
	}
	r.addStepLog(stepID, msg)
}

func (r *Runner) addStepLog(stepID, msg string) {
	r.state.Logs = append(r.state.Logs, LogEntry{StepID: stepID, Iteration: r.state.Iteration, Message: msg})
}

// syncToCollection writes workflow-script variable changes to the live collection/environment
// so pre-request scripts see them.
func (r *Runner) syncToCollection() {
	if r.collection != nil {
		r.collection.Variables = r.ctx.CollectionVars
	}
	if r.environment != nil {
		r.environment.Values = r.ctx.EnvironmentVars
	}
}

// syncFromCollection picks up variable changes made by pre/post-request scripts.
func (r *Runner) syncFromCollection() {
	if r.collection != nil {
		r.ctx.CollectionVars = r.collection.Variables
	}
	if r.environment != nil {
		r.ctx.EnvironmentVars = r.environment.Values
	}
}

// --- JavaScript API setup ---

func (r *Runner) setupConsole() error {
	consoleObj := r.frame.vm.NewObject()
	logFn := func(call goja.FunctionCall) goja.Value {
		parts := make([]string, len(call.Arguments))
		for i, a := range call.Arguments {
			parts[i] = a.String()
		}
		r.addLog(strings.Join(parts, " "))
		return goja.Undefined()
	}
	for _, name := range []string{"log", "error", "warn", "info"} {
		if err := consoleObj.Set(name, logFn); err != nil {
			return err
		}
	}
	return r.frame.vm.Set("console", consoleObj)
}

func (r *Runner) setupPmAPI() error {
	r.ctx.SendRequest = r.engine.ScriptSender()
	if r.environment != nil {
		r.ctx.EnvironmentName = r.environment.Name
	}
	loop, err := script.SetupPmAPI(r.frame.vm, r.ctx, &script.TestResult{})
	if err != nil {
		return err
	}
	r.frame.loop = loop
	return nil
}

func (r *Runner) setupWFAPI() error {
	wfObj := r.frame.vm.NewObject()

	if err := wfObj.Set("run", func(call goja.FunctionCall) goja.Value {
		r.abortIfCancelled()
		if r.status == StatusFailed || r.status == StatusStopped {
			return r.emptyResultJS(StatusSkipped)
		}
		if len(call.Arguments) == 0 {
			return r.emptyResultJS(StatusFailed)
		}
		stepID := call.Arguments[0].String()

		opts := RunOptions{Repeat: 1}
		if len(call.Arguments) > 1 {
			opts = r.parseRunOptions(call.Arguments[1])
		}

		result, err := r.runStep(stepID, opts)
		if errors.Is(err, ErrRunCancelled) || errors.Is(err, errStageAborted) {
			r.abortIfCancelled()
		}
		if err != nil {
			r.addStepLog(r.frame.qualify(stepID), fmt.Sprintf("[ERROR] %v", err))
			return r.emptyResultJS(StatusFailed)
		}
		return r.resultToJS(result)
	}); err != nil {
		return err
	}

	if err := wfObj.Set("fail", func(call goja.FunctionCall) goja.Value {
		reason := ""
		if len(call.Arguments) > 0 {
			reason = call.Arguments[0].String()
		}
		panic(r.failRun(reason))
	}); err != nil {
		return err
	}

	if err := wfObj.Set("jump", func(call goja.FunctionCall) goja.Value {
		target := script.NextRequest{Set: true}
		if len(call.Arguments) > 0 && !goja.IsNull(call.Arguments[0]) && !goja.IsUndefined(call.Arguments[0]) {
			target.Name = call.Arguments[0].String()
		}
		if r.frame.group == nil && target.Name != "" {
			panic(r.failRun(fmt.Sprintf("setNextRequest target %q is not in this run", target.Name)))
		}
		r.frame.jump = target
		panic(r.frame.vm.NewGoError(errJumped))
	}); err != nil {
		return err
	}

	if r.frame.entry != "" {
		if err := wfObj.Set("entry", r.frame.entry); err != nil {
			return err
		}
		if stepID, ok := r.nextStepFor(r.frame.entry); ok {
			if err := wfObj.Set("entryStep", stepID); err != nil {
				return err
			}
		}
	}

	if err := wfObj.Set("stop", func(call goja.FunctionCall) goja.Value {
		reason := ""
		if len(call.Arguments) > 0 {
			reason = call.Arguments[0].String()
		}
		r.stop(reason)
		panic(r.frame.vm.NewGoError(fmt.Errorf("workflow.stop: %s", reason)))
	}); err != nil {
		return err
	}

	if err := wfObj.Set("skip", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			stepID := r.frame.qualify(call.Arguments[0].String())
			if ss, ok := r.state.StepIndex[stepID]; ok {
				ss.Status = StatusSkipped
				r.addStepLog(stepID, "[SKIP] skipped by the workflow script")
				r.notifyProgress()
			}
		}
		return goja.Undefined()
	}); err != nil {
		return err
	}

	if err := wfObj.Set("sleep", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		ms := call.Arguments[0].ToInteger()
		if ms > 0 {
			r.outsideScriptTimeout(func() { r.sleep(time.Duration(ms) * time.Millisecond) })
			r.abortIfCancelled()
		}
		return goja.Undefined()
	}); err != nil {
		return err
	}

	if err := wfObj.Set("context", r.frame.vm.NewObject()); err != nil {
		return err
	}

	// wf.status getter
	if err := r.frame.vm.Set("__wf__", wfObj); err != nil {
		return err
	}
	if err := r.frame.vm.Set("__wfStatusGetter__", func(call goja.FunctionCall) goja.Value {
		return r.frame.vm.ToValue(r.status)
	}); err != nil {
		return err
	}
	if _, err := r.frame.vm.RunString(`
Object.defineProperty(__wf__, 'status', {
    get: function() { return __wfStatusGetter__(); },
    enumerable: true
});
delete __wfStatusGetter__;
`); err != nil {
		return err
	}

	return r.frame.vm.Set("__wf__", wfObj)
}

// --- Step execution ---

// stepState is the run state of the frame's step stepID, added for a step the
// workflow script runs without declaring it.
func (r *Runner) stepState(stepID string) *StepState {
	qualifiedID := r.frame.qualify(stepID)
	if ss, ok := r.state.StepIndex[qualifiedID]; ok {
		return ss
	}
	ss := &StepState{ID: qualifiedID, Groups: r.frame.groups(), Status: StatusPending}
	if step := r.findStep(stepID); step != nil {
		ss.Request, ss.Folder = step.Request, step.Folder
	}
	r.addStepState(ss)
	return ss
}

func (r *Runner) runStep(stepID string, opts RunOptions) (*StepRunResult, error) {
	wfStep := r.findStep(stepID)
	if wfStep == nil && strings.Contains(stepID, SubStepSeparator) {
		return r.runNestedStep(stepID, opts)
	}
	if wfStep != nil && wfStep.IsFolder() {
		return r.runFolderStep(*wfStep, opts.Entry)
	}

	ss := r.stepState(stepID)
	r.currentStepID = ss.ID
	defer func() { r.currentStepID = "" }()

	requestPath := ss.Request
	if requestPath == "" {
		r.failStep(ss)
		return nil, fmt.Errorf("step %q has no request path", stepID)
	}

	item, err := r.resolveRequest(requestPath)
	if err != nil {
		r.failStep(ss)
		return nil, fmt.Errorf("cannot resolve %q: %w", requestPath, err)
	}

	repeat := opts.Repeat
	if repeat < 1 {
		repeat = 1
	}
	ss.Total = repeat
	r.startStep(ss)
	defer r.finishStep(ss)

	if wfStep != nil && wfStep.PreScript != "" {
		if err := r.runStepScript(wfStep.PreScript, stepID, nil); err != nil {
			r.failStep(ss)
			return nil, fmt.Errorf("pre-script for step %q failed: %w", stepID, err)
		}
	}

	var responses []*StepResponse
	var last *StepResponse
	var nextRequest script.NextRequest
	// The last attempt decides the status and the counted tests: with
	// repeat/until, earlier attempts failing while a poll waits is expected.
	status := StatusSuccess
	var lastScripts *script.TestResult

	stepCtx, abort := r.abortableContext(ss.ID, r.frame.ctx)
	defer abort()

	for i := 0; i < repeat; i++ {
		ss.Progress = i + 1
		r.notifyProgress()

		var outcome itemOutcome
		var execErr error
		r.outsideScriptTimeout(func() { outcome, status, execErr = r.executeItemWithSelfRetries(stepCtx, item, requestPath) })
		switch {
		case r.runCtx.Err() != nil:
			ss.Status = StatusStopped
			return nil, ErrRunCancelled
		case r.frame.ctx.Err() != nil:
			ss.Status = StatusStopped
			return nil, errStageAborted
		case stepCtx.Err() != nil:
			r.markAborted(ss)
			return &StepRunResult{Status: StatusStopped}, nil
		}
		if execErr != nil {
			r.failStep(ss)
			return nil, execErr
		}
		lastScripts = outcome.scripts
		ss.LastResult = outcome.scripts
		// A jump to the request itself was already retried here; only jumps
		// elsewhere go to the workflow script (folder runs follow them).
		if outcome.scripts != nil && !outcome.retrySelf {
			nextRequest = outcome.scripts.NextRequest
		}
		stepResp := outcome.step
		responses = append(responses, stepResp)
		last = stepResp
		if outcome.raw != nil {
			ss.LastResponse = outcome.raw
		}

		if opts.Until != nil && opts.Until(stepResp) {
			break
		}
	}
	r.countTests(lastScripts)

	if wfStep != nil && wfStep.PostScript != "" && last != nil {
		if err := r.runStepScript(wfStep.PostScript, stepID, last); err != nil {
			r.failStep(ss)
			return nil, fmt.Errorf("post-script for step %q failed: %w", stepID, err)
		}
	}

	if status == StatusFailed {
		r.failStep(ss)
	} else {
		ss.Status = status
		r.notifyProgress()
	}

	return &StepRunResult{
		Status:      status,
		Response:    last,
		Responses:   responses,
		Last:        last,
		NextRequest: nextRequest,
	}, nil
}

// runFolderStep runs the sub-workflow of the folder step in its own frame,
// entering it at the step running entry when set. The step fails when a step
// inside it failed; a setNextRequest target its sub-workflow handed up
// (wf.jump) becomes the step's nextRequest.
func (r *Runner) runFolderStep(step Step, entry string) (*StepRunResult, error) {
	ss := r.stepState(step.ID)
	if slices.Contains(r.frame.folders, step.Folder) {
		r.failStep(ss)
		return nil, fmt.Errorf("%w: %q", ErrFolderCycle, step.Folder)
	}
	sub, err := r.folderWorkflow(step.Folder)
	if err != nil {
		r.failStep(ss)
		return nil, fmt.Errorf("cannot resolve folder %q: %w", step.Folder, err)
	}

	r.startStep(ss)
	defer r.finishStep(ss)

	if err := r.runFolderStepScript(step.PreScript, ss); err != nil {
		r.failStep(ss)
		return nil, fmt.Errorf("pre-script for step %q failed: %w", step.ID, err)
	}

	parent := r.frame
	childCtx, abort := r.abortableContext(ss.ID, parent.ctx)
	defer abort()
	child := &frame{
		parent:  parent,
		ctx:     childCtx,
		steps:   sub.Steps,
		group:   ss,
		folders: append(slices.Clone(parent.folders), step.Folder),
		entry:   entry,
	}
	var scriptErr error
	r.outsideScriptTimeout(func() {
		r.frame = child
		scriptErr = r.runScript(sub.Script)
		r.frame = parent
	})

	switch {
	case r.runCtx.Err() != nil:
		ss.Status = StatusStopped
		return nil, ErrRunCancelled
	case parent.ctx.Err() != nil:
		ss.Status = StatusStopped
		return nil, errStageAborted
	case childCtx.Err() != nil:
		r.markAborted(ss)
		return &StepRunResult{Status: StatusStopped}, nil
	case scriptErr != nil:
		r.markRunFailed(fmt.Sprintf("folder step %q: %v", ss.ID, scriptErr))
		r.failStep(ss)
		return nil, scriptErr
	case r.status == StatusStopped:
		ss.Status = StatusStopped
		r.notifyProgress()
		return &StepRunResult{Status: StatusStopped}, nil
	}

	if err := r.runFolderStepScript(step.PostScript, ss); err != nil {
		r.failStep(ss)
		return nil, fmt.Errorf("post-script for step %q failed: %w", step.ID, err)
	}

	if child.failed || r.status == StatusFailed {
		r.failStep(ss)
	} else {
		ss.Status = StatusSuccess
		r.notifyProgress()
	}
	return &StepRunResult{Status: ss.Status, NextRequest: child.jump}, nil
}

// runNestedStep runs one step inside folder steps' sub-workflows on its own, by
// its qualified ID (e.g. "invite/redeem"): the enclosing sub-workflows get
// frames so the step runs and reports exactly as it does inside them, but their
// scripts and other steps do not run. Its step scripts run in the current VM.
func (r *Runner) runNestedStep(qualifiedID string, opts RunOptions) (*StepRunResult, error) {
	outer := r.frame
	defer func() { r.frame = outer }()

	names := strings.Split(qualifiedID, SubStepSeparator)
	for _, name := range names[:len(names)-1] {
		step := r.findStep(name)
		if step == nil || !step.IsFolder() {
			return nil, r.failNestedStep(outer, qualifiedID, fmt.Errorf("%w: %q has no folder step %q", ErrNestedStepNotFound, qualifiedID, name))
		}
		sub, err := r.folderWorkflow(step.Folder)
		if err != nil {
			return nil, r.failNestedStep(outer, qualifiedID, fmt.Errorf("cannot resolve folder %q: %w", step.Folder, err))
		}
		parent := r.frame
		r.frame = &frame{
			parent:      parent,
			ctx:         parent.ctx,
			steps:       sub.Steps,
			group:       r.stepState(step.ID),
			folders:     append(slices.Clone(parent.folders), step.Folder),
			vm:          parent.vm,
			loop:        parent.loop,
			scriptTimer: parent.scriptTimer,
		}
	}
	last := names[len(names)-1]
	if r.findStep(last) == nil {
		return nil, r.failNestedStep(outer, qualifiedID, fmt.Errorf("%w: %q", ErrNestedStepNotFound, qualifiedID))
	}
	return r.runStep(last, opts)
}

// failNestedStep fails the nested step qualifiedID that could not be reached
// from outer, like an undeclared top-level step, and returns err.
func (r *Runner) failNestedStep(outer *frame, qualifiedID string, err error) error {
	r.frame = outer
	r.failStep(r.stepState(qualifiedID))
	return err
}

// runFolderStepScript runs a folder step's pre- or post-script, logging to the step.
func (r *Runner) runFolderStepScript(src string, ss *StepState) error {
	if src == "" {
		return nil
	}
	r.currentStepID = ss.ID
	defer func() { r.currentStepID = "" }()
	return r.runStepScript(src, LocalStepID(ss.ID), nil)
}

// folderWorkflow is the workflow a folder step for folder runs, resolved once per run.
func (r *Runner) folderWorkflow(folder string) (*Workflow, error) {
	if wf, ok := r.folderWorkflows[folder]; ok {
		return wf, nil
	}
	wf, err := r.folderSource(postman.SplitItemPath(folder))
	if err != nil {
		return nil, err
	}
	if r.folderWorkflows == nil {
		r.folderWorkflows = make(map[string]*Workflow)
	}
	r.folderWorkflows[folder] = wf
	return wf, nil
}

// abortIfCancelled ends the workflow script once the run is cancelled (and
// stops the run) or its sub-workflow is aborted. Both also interrupt the VM,
// but only at its next instruction, which may already start another step.
func (r *Runner) abortIfCancelled() {
	if r.runCtx.Err() != nil {
		r.markCancelled()
		panic(r.frame.vm.NewGoError(ErrRunCancelled))
	}
	if r.frame.ctx.Err() != nil {
		panic(r.frame.vm.NewGoError(errStageAborted))
	}
}

// Abort stops the running request step or folder step with run state ID
// runID; the workflow around it carries on with its next step. It reports
// false when nothing with that ID is running. Safe to call while Run runs.
func (r *Runner) Abort(runID string) bool {
	r.abortMu.Lock()
	defer r.abortMu.Unlock()
	cancel, running := r.abortable[runID]
	if running {
		cancel()
	}
	return running
}

// abortableContext derives the context of the step runID from parent and lets
// Abort cancel it until the returned release is called.
func (r *Runner) abortableContext(runID string, parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	r.abortMu.Lock()
	if r.abortable == nil {
		r.abortable = make(map[string]context.CancelFunc)
	}
	r.abortable[runID] = cancel
	r.abortMu.Unlock()
	return ctx, func() {
		r.abortMu.Lock()
		delete(r.abortable, runID)
		r.abortMu.Unlock()
		cancel()
	}
}

// startStep marks ss running from now; a step the script runs again restarts its clock.
func (r *Runner) startStep(ss *StepState) {
	ss.Status = StatusRunning
	ss.StartedAt = time.Now()
	ss.FinishedAt = time.Time{}
	r.notifyProgress()
}

// finishStep stops ss's clock, whichever way the step ended.
func (r *Runner) finishStep(ss *StepState) {
	ss.FinishedAt = time.Now()
	r.notifyProgress()
}

// markAborted stops an aborted step. The run carries on but fails at the end,
// so an abort never passes for a success.
func (r *Runner) markAborted(ss *StepState) {
	ss.Status = StatusStopped
	r.anyStepFailed = true
	for f := r.frame; f != nil; f = f.parent {
		f.failed = true
	}
	r.addStepLog(ss.ID, "[ABORT] "+ErrStepAborted.Error())
	r.notifyProgress()
}

// markCancelled stops the run as cancelled unless it already stopped.
func (r *Runner) markCancelled() {
	if r.status != StatusStopped {
		r.stop(ErrRunCancelled.Error())
	}
}

// sleep waits for duration or until the run is cancelled or its sub-workflow aborted.
func (r *Runner) sleep(duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-r.frame.ctx.Done():
	}
}

// outsideScriptTimeout runs wait with the workflow script timer paused.
func (r *Runner) outsideScriptTimeout(wait func()) {
	if r.frame.scriptTimer == nil || !r.frame.scriptTimer.Stop() {
		wait()
		return
	}
	wait()
	r.frame.scriptTimer.Reset(r.scriptTimeout)
}

// executeItemWithSelfRetries re-sends the request while its scripts call
// pm.execution.setNextRequest on itself, as a Postman Runner does for polling
// reads. Jumps to other requests are left to the workflow script, which gets
// them as the step result's nextRequest (generated folder runs follow them).
func (r *Runner) executeItemWithSelfRetries(ctx context.Context, item *postman.Item, requestPath string) (itemOutcome, string, error) {
	for attempt := 0; ; attempt++ {
		if ctx.Err() != nil {
			return itemOutcome{}, StatusStopped, nil
		}
		outcome, err := r.executeItem(ctx, item, requestPath)
		if err != nil {
			return outcome, StatusFailed, err
		}
		status := r.recordOutcome(outcome)
		if !outcome.retrySelf {
			return outcome, status, nil
		}
		if attempt == maxSelfRetries {
			r.addLog(fmt.Sprintf("[RETRY LIMIT] %q called setNextRequest on itself more than %d times", item.Name, maxSelfRetries))
			return outcome, StatusFailed, nil
		}
	}
}

// nextStepFor finds the step a pm.execution.setNextRequest target refers to:
// first by request name, then by Postman request id or request path, in step
// order. A folder step matches when a step of its sub-workflow, at any depth,
// does. It reports false when no step in this workflow matches.
func (r *Runner) nextStepFor(target string) (string, bool) {
	byName := func(requestPath string) bool { return requestNameOf(requestPath) == target }
	byPathOrID := func(requestPath string) bool {
		if requestPath == target {
			return true
		}
		item, err := r.resolveRequest(requestPath)
		return err == nil && item.ID != "" && item.ID == target
	}
	for _, matches := range []func(string) bool{byName, byPathOrID} {
		for _, step := range r.frame.steps {
			if r.stepRunsRequest(step, matches, r.frame.folders) {
				return step.ID, true
			}
		}
	}
	return "", false
}

// stepRunsRequest reports whether step runs a request matches accepts, itself
// or inside its sub-workflow. folders are the folder workflows already being
// searched, which a sub-workflow running itself would search again.
func (r *Runner) stepRunsRequest(step Step, matches func(requestPath string) bool, folders []string) bool {
	if !step.IsFolder() {
		return step.Request != "" && matches(step.Request)
	}
	if slices.Contains(folders, step.Folder) {
		return false
	}
	sub, err := r.folderWorkflow(step.Folder)
	if err != nil {
		return false
	}
	folders = append(slices.Clone(folders), step.Folder)
	for _, subStep := range sub.Steps {
		if r.stepRunsRequest(subStep, matches, folders) {
			return true
		}
	}
	return false
}

// recordOutcome logs a request's test results and console output and returns
// its step status.
func (r *Runner) recordOutcome(outcome itemOutcome) string {
	result := outcome.scripts
	if result != nil {
		for _, log := range result.ConsoleLogs {
			r.addLog(log)
		}
		for _, test := range result.Tests {
			if test.Passed {
				r.addLog(fmt.Sprintf("%s %s", LogPassMarker, test.Name))
			} else {
				r.addLog(fmt.Sprintf("%s %s: %s", LogFailMarker, test.Name, test.Error))
			}
		}
		for _, scriptErr := range result.Errors {
			r.addLog(fmt.Sprintf("%s %s", LogScriptErrorPrefix, scriptErr))
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

// failStep marks ss failed. With StopOnFailure it also fails the run, which
// makes every later wf.run() return skipped and ends the iteration loop.
func (r *Runner) failStep(ss *StepState) {
	ss.Status = StatusFailed
	r.anyStepFailed = true
	for f := r.frame; f != nil; f = f.parent {
		f.failed = true
	}
	if r.options.StopOnFailure && r.status == StatusRunning {
		r.status = StatusFailed
		r.state.Error = fmt.Sprintf("%v: step %q", ErrStoppedOnFailure, ss.ID)
		r.addLog("[STOP] " + r.state.Error)
	}
	r.notifyProgress()
}

// folderPathOf returns the folders of a "folder/sub/request" path.
func folderPathOf(requestPath string) []string {
	parts := postman.SplitItemPath(requestPath)
	return parts[:len(parts)-1]
}

// requestNameOf returns the request name of a "folder/sub/request" path.
func requestNameOf(requestPath string) string {
	parts := postman.SplitItemPath(requestPath)
	return parts[len(parts)-1]
}

func (r *Runner) findStep(stepID string) *Step {
	for i := range r.frame.steps {
		if r.frame.steps[i].ID == stepID {
			return &r.frame.steps[i]
		}
	}
	return nil
}

// runStepScript executes a plain JS string in the runner VM.
// response is nil for pre-scripts; post-scripts receive a `step` object with the response.
func (r *Runner) runStepScript(script, stepID string, response *StepResponse) error {
	if response != nil {
		stepObj := r.frame.vm.NewObject()
		stepObj.Set("id", stepID)
		stepObj.Set("response", r.responseToJS(response))
		if err := r.frame.vm.Set("step", stepObj); err != nil {
			return err
		}
		defer r.frame.vm.Set("step", goja.Undefined())
	}

	_, err := r.frame.vm.RunString(script)
	return err
}

func (r *Runner) resolveRequest(path string) (*postman.Item, error) {
	if r.collection == nil {
		return nil, ErrNoCollection
	}

	parts := postman.SplitItemPath(path)
	current := r.collection.Items

	for depth, part := range parts {
		isLast := depth == len(parts)-1
		found := false
		for i := range current {
			if current[i].Name == part {
				if isLast {
					item := current[i]
					return &item, nil
				}
				current = current[i].Items
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("path segment %q not found (path: %q)", part, path)
		}
	}

	return nil, fmt.Errorf("request %q not found", path)
}

// itemOutcome is what executing one collection item produced.
type itemOutcome struct {
	step    *StepResponse
	raw     *http.Response
	scripts *script.TestResult
	// retrySelf is set when the request's scripts asked to run it again.
	retrySelf bool
}

func (r *Runner) executeItem(ctx context.Context, item *postman.Item, requestPath string) (itemOutcome, error) {
	scope := script.RequestScope{
		Collection:  r.collection,
		Environment: r.environment,
		FolderPath:  folderPathOf(requestPath),
		Item:        item,
		Iteration:   r.iteration,
	}
	execution, err := r.engine.Prepare(engine.Call{Item: item, Scope: scope})
	if err != nil {
		return itemOutcome{}, fmt.Errorf("item %q is not a request: %w", item.Name, err)
	}

	// Write workflow-script variable changes to the collection before running.
	r.syncToCollection()

	result := execution.Run(ctx)
	resp, scriptResult := result.Response, result.TestResult

	// Pick up variable changes from pre/post-request scripts.
	r.syncFromCollection()

	stepResp := &StepResponse{
		Code:    resp.StatusCode,
		Headers: resp.Headers,
		Body:    resp.Body,
		Time:    resp.Duration.Milliseconds(),
	}

	if resp.Error != nil {
		r.addLog(fmt.Sprintf("[%s ERROR] %v", result.Protocol, resp.Error))
		// Step scripts read the body, so a failure without one reports its error there.
		if stepResp.Body == "" {
			stepResp.Body = fmt.Sprintf(`{"error":%q}`, resp.Error.Error())
		}
	}

	retrySelf := scriptResult != nil && scriptResult.NextRequest.Set && scope.IsAddressedBy(scriptResult.NextRequest.Name)
	return itemOutcome{step: stepResp, raw: resp, scripts: scriptResult, retrySelf: retrySelf}, nil
}

// --- JavaScript helpers ---

func (r *Runner) parseRunOptions(val goja.Value) RunOptions {
	opts := RunOptions{Repeat: 1}
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return opts
	}
	obj, ok := val.(*goja.Object)
	if !ok {
		return opts
	}

	if repeatVal := obj.Get("repeat"); repeatVal != nil && !goja.IsUndefined(repeatVal) {
		if n := repeatVal.ToInteger(); n > 0 {
			opts.Repeat = int(n)
		}
	}

	if entryVal := obj.Get("entry"); entryVal != nil && !goja.IsUndefined(entryVal) && !goja.IsNull(entryVal) {
		opts.Entry = entryVal.String()
	}

	if untilFnVal := obj.Get("until"); untilFnVal != nil && !goja.IsUndefined(untilFnVal) {
		if fn, ok := goja.AssertFunction(untilFnVal); ok {
			opts.Until = func(resp *StepResponse) bool {
				jsResp := r.responseToJS(resp)
				result, err := fn(goja.Undefined(), jsResp)
				if err != nil {
					return false
				}
				return result.ToBoolean()
			}
		}
	}

	return opts
}

func (r *Runner) responseToJS(resp *StepResponse) *goja.Object {
	obj := r.frame.vm.NewObject()
	obj.Set("code", resp.Code)
	obj.Set("body", resp.Body)
	obj.Set("headers", resp.Headers)
	obj.Set("time", resp.Time)
	obj.Set("text", func(goja.FunctionCall) goja.Value {
		return r.frame.vm.ToValue(resp.Body)
	})
	obj.Set("json", func(call goja.FunctionCall) goja.Value {
		var data any
		if err := json.Unmarshal([]byte(resp.Body), &data); err != nil {
			panic(r.frame.vm.NewGoError(fmt.Errorf("response.json() failed: %w", err)))
		}
		return r.frame.vm.ToValue(data)
	})
	return obj
}

func (r *Runner) resultToJS(result *StepRunResult) goja.Value {
	if result == nil {
		return r.emptyResultJS(StatusFailed)
	}
	obj := r.frame.vm.NewObject()
	obj.Set("status", result.Status)

	var lastJS goja.Value = goja.Null()
	if result.Last != nil {
		lastJS = r.responseToJS(result.Last)
	}

	var responsesJS []any
	for _, resp := range result.Responses {
		responsesJS = append(responsesJS, r.responseToJS(resp))
	}

	obj.Set("response", lastJS)
	obj.Set("last", lastJS)
	obj.Set("responses", r.frame.vm.ToValue(responsesJS))
	r.setNextRequestJS(obj, result.NextRequest)
	return obj
}

// setNextRequestJS exposes a step's setNextRequest decision to the workflow
// script: nextRequest is undefined when the step made none, null for
// setNextRequest(null), else the target as given; nextStep is the matching
// step's ID when the target is part of this workflow.
func (r *Runner) setNextRequestJS(obj *goja.Object, next script.NextRequest) {
	if !next.Set {
		return
	}
	if next.Name == "" {
		obj.Set("nextRequest", goja.Null())
		return
	}
	obj.Set("nextRequest", next.Name)
	if stepID, ok := r.nextStepFor(next.Name); ok {
		obj.Set("nextStep", stepID)
	}
}

func (r *Runner) emptyResultJS(status string) goja.Value {
	obj := r.frame.vm.NewObject()
	obj.Set("status", status)
	emptyResp := r.frame.vm.NewObject()
	emptyResp.Set("code", 0)
	emptyResp.Set("body", "")
	emptyResp.Set("headers", r.frame.vm.NewObject())
	emptyResp.Set("time", 0)
	emptyResp.Set("text", func(goja.FunctionCall) goja.Value { return r.frame.vm.ToValue("") })
	emptyResp.Set("json", func(goja.FunctionCall) goja.Value { return r.frame.vm.NewObject() })
	obj.Set("response", emptyResp)
	obj.Set("last", emptyResp)
	obj.Set("responses", r.frame.vm.ToValue([]any{}))
	return obj
}

// transformScript strips "export default " and wraps the async function
// as a self-invoked expression so goja can execute it.
func transformScript(src string) string {
	src = strings.TrimSpace(src)
	src = strings.TrimPrefix(src, "export default ")
	src = strings.TrimSpace(src)
	return "(" + src + ")(__wf__, pm);"
}
