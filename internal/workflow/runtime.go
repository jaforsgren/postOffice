package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
	"postOffice/internal/amqp"
	internalgrpc "postOffice/internal/grpc"
	"postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/script"
	"postOffice/internal/servicebus"
)

const (
	// defaultScriptTimeout bounds each stretch of workflow script between steps.
	// Time inside steps and wf.sleep is not counted: requests bound themselves.
	defaultScriptTimeout = 30 * time.Second
	requestPathSeparator = "/"
	// maxSelfRetries stops a request whose script always calls
	// pm.execution.setNextRequest on itself from looping forever.
	maxSelfRetries = 100
)

// Runner executes a workflow against a collection and environment.
type Runner struct {
	collection   *postman.Collection
	environment  *postman.Environment
	httpExecutor *http.Executor
	grpcExecutor *internalgrpc.Executor

	wfSteps []Step
	options ExecutionOptions
	// iteration is the pass in progress; every request in it sees its data row.
	iteration script.Iteration
	// anyStepFailed fails the whole run at the end, while still letting the
	// workflow script carry on after a failed step (unless StopOnFailure).
	anyStepFailed bool
	ctx           *script.ExecutionContext
	state         ExecutionState
	status        string
	vm            *goja.Runtime
	loop          *script.EventLoop
	onChange      func(ExecutionState)
	// currentStepID attributes logs to the step being run; empty between steps.
	currentStepID string
	scriptTimeout time.Duration
	scriptTimer   *time.Timer
	timedOut      atomic.Bool
}

// NewRunner creates a runner bound to the given collection and environment.
func NewRunner(
	collection *postman.Collection,
	environment *postman.Environment,
	httpExecutor *http.Executor,
) *Runner {
	return &Runner{
		collection:    collection,
		environment:   environment,
		httpExecutor:  httpExecutor,
		grpcExecutor:  internalgrpc.NewExecutor(),
		scriptTimeout: defaultScriptTimeout,
	}
}

var (
	// ErrStepsFailed is the run error when the workflow script finished but at
	// least one step failed.
	ErrStepsFailed = errors.New("one or more steps failed")
	// ErrWorkflowTimeout is the run error when one iteration's script runs too long.
	ErrWorkflowTimeout = errors.New("workflow timed out")
	// ErrScriptFailed is the run error when the workflow script itself throws.
	ErrScriptFailed = errors.New("script error")
)

// Run executes wf once per iteration of opts, calling onChange after each
// state change. Collection and environment variables carry over between
// iterations; each iteration gets a fresh script VM and its own step states.
func (r *Runner) Run(wf *Workflow, opts ExecutionOptions, onChange func(ExecutionState)) ExecutionState {
	r.onChange = onChange
	r.status = StatusRunning
	r.anyStepFailed = false
	r.options = opts
	r.wfSteps = wf.Steps
	r.state = ExecutionState{
		WorkflowID:   wf.ID,
		WorkflowName: wf.Name,
		Status:       StatusRunning,
		StepIndex:    make(map[string]*StepState),
		Iterations:   opts.IterationCount(),
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
// for each declared step, so the run view lists every iteration's steps.
func (r *Runner) beginIteration(index int) {
	r.iteration = r.options.iteration(index)
	r.ctx.Iteration = r.iteration
	r.state.Iteration = index + 1
	for _, s := range r.wfSteps {
		r.addStepState(&StepState{ID: s.ID, Request: s.Request, Status: StatusPending})
	}
	r.notifyProgress()
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
	r.vm = goja.New()

	if err := r.setupPmAPI(); err != nil {
		return fmt.Errorf("pm API setup failed: %w", err)
	}

	// After the pm API, which installs its own console: the workflow's must win
	// so console output lands in the run log.
	if err := r.setupConsole(); err != nil {
		return fmt.Errorf("console setup failed: %w", err)
	}

	if err := script.SetupCryptoJS(r.vm); err != nil {
		return fmt.Errorf("CryptoJS setup failed: %w", err)
	}

	if err := r.setupWFAPI(); err != nil {
		return fmt.Errorf("wf API setup failed: %w", err)
	}

	transformed := transformScript(wf.Script)

	r.timedOut.Store(false)
	r.scriptTimer = time.AfterFunc(r.scriptTimeout, func() {
		r.timedOut.Store(true)
		r.vm.Interrupt("workflow timeout")
	})

	_, err := r.vm.RunString(transformed)
	if err == nil {
		for _, callbackErr := range r.loop.Drain() {
			r.addLog(fmt.Sprintf("[SCRIPT ERROR] %v", callbackErr))
		}
	}
	r.scriptTimer.Stop()

	if r.timedOut.Load() {
		return fmt.Errorf("%w after %v", ErrWorkflowTimeout, r.scriptTimeout)
	}

	if err != nil && r.status != StatusFailed && r.status != StatusStopped {
		return fmt.Errorf("%w: %v", ErrScriptFailed, err)
	}
	return nil
}

func (r *Runner) finalize(status, errMsg string) ExecutionState {
	r.status = status
	r.state.Status = status
	r.state.Error = errMsg

	for _, ss := range r.state.Steps {
		if ss.Status == StatusPending || ss.Status == StatusRunning {
			ss.Status = StatusSkipped
		}
	}
	r.summarizeSteps()

	r.notifyProgress()
	return r.state
}

// summarizeSteps counts every iteration's final step statuses into the
// summary; test counts accumulate as steps finish (see countTests).
func (r *Runner) summarizeSteps() {
	summary := &r.state.Summary
	summary.StepsPassed, summary.StepsFailed, summary.StepsSkipped = 0, 0, 0
	for _, ss := range r.state.Steps {
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
	}
	for _, ss := range r.state.Steps {
		cp := *ss
		snap.Steps = append(snap.Steps, &cp)
		snap.StepIndex[cp.ID] = &cp
	}
	r.onChange(snap)
}

// addLog records msg against the step currently running, if any.
func (r *Runner) addLog(msg string) {
	r.addStepLog(r.currentStepID, msg)
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
	consoleObj := r.vm.NewObject()
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
	return r.vm.Set("console", consoleObj)
}

func (r *Runner) setupPmAPI() error {
	r.ctx.SendRequest = r.httpExecutor.SendRequest
	if r.environment != nil {
		r.ctx.EnvironmentName = r.environment.Name
	}
	loop, err := script.SetupPmAPI(r.vm, r.ctx, &script.TestResult{})
	if err != nil {
		return err
	}
	r.loop = loop
	return nil
}

func (r *Runner) setupWFAPI() error {
	wfObj := r.vm.NewObject()

	if err := wfObj.Set("run", func(call goja.FunctionCall) goja.Value {
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
		if err != nil {
			r.addStepLog(stepID, fmt.Sprintf("[ERROR] %v", err))
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
		r.status = StatusFailed
		r.state.Status = StatusFailed
		r.state.Error = reason
		r.addLog(fmt.Sprintf("[FAIL] %s", reason))
		r.notifyProgress()
		panic(r.vm.NewGoError(fmt.Errorf("workflow.fail: %s", reason)))
	}); err != nil {
		return err
	}

	if err := wfObj.Set("stop", func(call goja.FunctionCall) goja.Value {
		reason := ""
		if len(call.Arguments) > 0 {
			reason = call.Arguments[0].String()
		}
		r.status = StatusStopped
		r.state.Status = StatusStopped
		r.state.Error = reason
		r.addLog(fmt.Sprintf("[STOP] %s", reason))
		r.notifyProgress()
		panic(r.vm.NewGoError(fmt.Errorf("workflow.stop: %s", reason)))
	}); err != nil {
		return err
	}

	if err := wfObj.Set("skip", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			stepID := call.Arguments[0].String()
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
			r.outsideScriptTimeout(func() { time.Sleep(time.Duration(ms) * time.Millisecond) })
		}
		return goja.Undefined()
	}); err != nil {
		return err
	}

	if err := wfObj.Set("context", r.vm.NewObject()); err != nil {
		return err
	}

	// wf.status getter
	if err := r.vm.Set("__wf__", wfObj); err != nil {
		return err
	}
	if err := r.vm.Set("__wfStatusGetter__", func(call goja.FunctionCall) goja.Value {
		return r.vm.ToValue(r.status)
	}); err != nil {
		return err
	}
	if _, err := r.vm.RunString(`
Object.defineProperty(__wf__, 'status', {
    get: function() { return __wfStatusGetter__(); },
    enumerable: true
});
delete __wfStatusGetter__;
`); err != nil {
		return err
	}

	return r.vm.Set("__wf__", wfObj)
}

// --- Step execution ---

func (r *Runner) runStep(stepID string, opts RunOptions) (*StepRunResult, error) {
	r.currentStepID = stepID
	defer func() { r.currentStepID = "" }()

	ss, ok := r.state.StepIndex[stepID]
	if !ok {
		// Step not declared in YAML — create a transient entry for tracking.
		ss = &StepState{ID: stepID, Request: r.findRequestPath(stepID), Status: StatusPending}
		r.addStepState(ss)
	}

	requestPath := ss.Request
	if requestPath == "" {
		requestPath = r.findRequestPath(stepID)
		ss.Request = requestPath
	}

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
	ss.Status = StatusRunning
	r.notifyProgress()

	wfStep := r.findStep(stepID)

	if wfStep != nil && wfStep.PreScript != "" {
		if err := r.runStepScript(wfStep.PreScript, stepID, nil); err != nil {
			r.failStep(ss)
			return nil, fmt.Errorf("pre-script for step %q failed: %w", stepID, err)
		}
	}

	var responses []*StepResponse
	var last *StepResponse
	// The last attempt decides the status and the counted tests: with
	// repeat/until, earlier attempts failing while a poll waits is expected.
	status := StatusSuccess
	var lastScripts *script.TestResult

	for i := 0; i < repeat; i++ {
		ss.Progress = i + 1
		r.notifyProgress()

		var outcome itemOutcome
		var execErr error
		r.outsideScriptTimeout(func() { outcome, status, execErr = r.executeItemWithSelfRetries(item, requestPath) })
		if execErr != nil {
			r.failStep(ss)
			return nil, execErr
		}
		lastScripts = outcome.scripts
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
		Status:    status,
		Response:  last,
		Responses: responses,
		Last:      last,
	}, nil
}

// outsideScriptTimeout runs wait with the workflow script timer paused.
func (r *Runner) outsideScriptTimeout(wait func()) {
	if r.scriptTimer == nil || !r.scriptTimer.Stop() {
		wait()
		return
	}
	wait()
	r.scriptTimer.Reset(r.scriptTimeout)
}

// executeItemWithSelfRetries re-sends the request while its scripts call
// pm.execution.setNextRequest on itself, as a Postman Runner does for polling
// reads. Jumps to other requests are ignored: the workflow script owns the order.
func (r *Runner) executeItemWithSelfRetries(item *postman.Item, requestPath string) (itemOutcome, string, error) {
	for attempt := 0; ; attempt++ {
		outcome, err := r.executeItem(item, requestPath)
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
	if r.options.StopOnFailure && r.status == StatusRunning {
		r.status = StatusFailed
		r.state.Error = fmt.Sprintf("%v: step %q", ErrStoppedOnFailure, ss.ID)
		r.addLog("[STOP] " + r.state.Error)
	}
	r.notifyProgress()
}

// folderPathOf returns the folders of a "folder/sub/request" path.
func folderPathOf(requestPath string) []string {
	parts := strings.Split(requestPath, requestPathSeparator)
	return parts[:len(parts)-1]
}

func (r *Runner) findRequestPath(stepID string) string {
	for _, s := range r.wfSteps {
		if s.ID == stepID {
			return s.Request
		}
	}
	return ""
}

func (r *Runner) findStep(stepID string) *Step {
	for i := range r.wfSteps {
		if r.wfSteps[i].ID == stepID {
			return &r.wfSteps[i]
		}
	}
	return nil
}

// runStepScript executes a plain JS string in the runner VM.
// response is nil for pre-scripts; post-scripts receive a `step` object with the response.
func (r *Runner) runStepScript(script, stepID string, response *StepResponse) error {
	if response != nil {
		stepObj := r.vm.NewObject()
		stepObj.Set("id", stepID)
		stepObj.Set("response", r.responseToJS(response))
		if err := r.vm.Set("step", stepObj); err != nil {
			return err
		}
		defer r.vm.Set("step", goja.Undefined())
	}

	_, err := r.vm.RunString(script)
	return err
}

func (r *Runner) resolveRequest(path string) (*postman.Item, error) {
	if r.collection == nil {
		return nil, fmt.Errorf("no collection loaded")
	}

	parts := strings.Split(path, requestPathSeparator)
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

func (r *Runner) executeItem(item *postman.Item, requestPath string) (itemOutcome, error) {
	// Write workflow-script variable changes to the collection before running.
	r.syncToCollection()

	scope := script.RequestScope{
		Collection:  r.collection,
		Environment: r.environment,
		FolderPath:  folderPathOf(requestPath),
		Item:        item,
		Sender:      r.httpExecutor.SendRequest,
		Iteration:   r.iteration,
	}

	variables := scope.Variables()

	if item.IsGRPC() {
		return r.executeGRPC(item, scope, variables), nil
	}

	if item.IsServiceBus() {
		return r.executeMessageBus(item, scope, "ASB")
	}

	if item.IsAMQP() {
		return r.executeMessageBus(item, scope, "AMQP")
	}

	if !item.IsRequest() || item.Request == nil {
		return itemOutcome{}, fmt.Errorf("item %q is not a request", item.Name)
	}

	resp, scriptResult := r.httpExecutor.Execute(item.Request, &scope, variables)

	// Pick up variable changes from pre/post-request scripts.
	r.syncFromCollection()

	stepResp := &StepResponse{
		Code:    resp.StatusCode,
		Headers: resp.Headers,
		Body:    resp.Body,
		Time:    resp.Duration.Milliseconds(),
	}

	if resp.Error != nil {
		r.addLog(fmt.Sprintf("[HTTP ERROR] %v", resp.Error))
	}

	retrySelf := scriptResult != nil && scriptResult.NextRequest.Set && scope.IsAddressedBy(scriptResult.NextRequest.Name)
	return itemOutcome{step: stepResp, raw: resp, scripts: scriptResult, retrySelf: retrySelf}, nil
}

func (r *Runner) executeMessageBus(item *postman.Item, scope script.RequestScope, protocol string) (itemOutcome, error) {
	if item.Request == nil {
		return itemOutcome{}, fmt.Errorf("item %q has no request", item.Name)
	}

	// Run Postman pre-request scripts (e.g. to generate IDs before the send).
	request, preResult := script.NewRequestRun(scope).PreRequest(item.Request)
	for _, e := range preResult.Errors {
		r.addLog(fmt.Sprintf("[%s PRE-SCRIPT ERROR] %s", protocol, e))
	}
	r.syncFromCollection()
	if preResult.Skipped {
		r.addLog(fmt.Sprintf("[%s SKIPPED] %s", protocol, item.Name))
		return itemOutcome{step: &StepResponse{}, raw: &http.Response{Skipped: true, Status: http.SkippedStatus}, scripts: preResult}, nil
	}

	variables := scope.Variables()

	var resp *http.Response
	switch protocol {
	case "ASB":
		resp = servicebus.Send(request, variables)
	case "AMQP":
		resp = amqp.Publish(request, variables)
	default:
		return itemOutcome{}, fmt.Errorf("unknown message bus protocol: %s", protocol)
	}

	r.syncFromCollection()

	stepResp := &StepResponse{
		Code: resp.StatusCode,
		Body: resp.Body,
		Time: resp.Duration.Milliseconds(),
	}

	if resp.Error != nil {
		r.addLog(fmt.Sprintf("[%s ERROR] %v", protocol, resp.Error))
	}

	return itemOutcome{step: stepResp, raw: resp, scripts: preResult}, nil
}

func (r *Runner) executeGRPC(item *postman.Item, scope script.RequestScope, variables []postman.VariableSource) itemOutcome {
	resp, scriptResult := r.grpcExecutor.Execute(item.Request, &scope, variables)

	// Pick up variable changes from pre/post-request scripts.
	r.syncFromCollection()

	stepResp := &StepResponse{
		Code: resp.StatusCode,
		Body: resp.Body,
		Time: resp.Duration.Milliseconds(),
	}
	if resp.Error != nil {
		r.addLog(fmt.Sprintf("[gRPC ERROR] %v", resp.Error))
		stepResp.Body = fmt.Sprintf(`{"error":%q}`, resp.Error.Error())
	}

	return itemOutcome{step: stepResp, raw: resp, scripts: scriptResult}
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
	obj := r.vm.NewObject()
	obj.Set("code", resp.Code)
	obj.Set("body", resp.Body)
	obj.Set("headers", resp.Headers)
	obj.Set("time", resp.Time)
	obj.Set("text", func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(resp.Body)
	})
	obj.Set("json", func(call goja.FunctionCall) goja.Value {
		var data any
		if err := json.Unmarshal([]byte(resp.Body), &data); err != nil {
			panic(r.vm.NewGoError(fmt.Errorf("response.json() failed: %w", err)))
		}
		return r.vm.ToValue(data)
	})
	return obj
}

func (r *Runner) resultToJS(result *StepRunResult) goja.Value {
	if result == nil {
		return r.emptyResultJS(StatusFailed)
	}
	obj := r.vm.NewObject()
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
	obj.Set("responses", r.vm.ToValue(responsesJS))
	return obj
}

func (r *Runner) emptyResultJS(status string) goja.Value {
	obj := r.vm.NewObject()
	obj.Set("status", status)
	emptyResp := r.vm.NewObject()
	emptyResp.Set("code", 0)
	emptyResp.Set("body", "")
	emptyResp.Set("headers", r.vm.NewObject())
	emptyResp.Set("time", 0)
	emptyResp.Set("text", func(goja.FunctionCall) goja.Value { return r.vm.ToValue("") })
	emptyResp.Set("json", func(goja.FunctionCall) goja.Value { return r.vm.NewObject() })
	obj.Set("response", emptyResp)
	obj.Set("last", emptyResp)
	obj.Set("responses", r.vm.ToValue([]any{}))
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
