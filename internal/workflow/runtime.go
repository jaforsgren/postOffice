package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"
	"postOffice/internal/amqp"
	internalgrpc "postOffice/internal/grpc"
	"postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/script"
	"postOffice/internal/servicebus"
)

const defaultScriptTimeout = 30 * time.Second

// Runner executes a workflow against a collection and environment.
type Runner struct {
	collection   *postman.Collection
	environment  *postman.Environment
	httpExecutor *http.Executor

	wfSteps  []Step
	ctx      *script.ExecutionContext
	state    ExecutionState
	status   string
	vm       *goja.Runtime
	onChange func(ExecutionState)
}

// NewRunner creates a runner bound to the given collection and environment.
func NewRunner(
	collection *postman.Collection,
	environment *postman.Environment,
	httpExecutor *http.Executor,
) *Runner {
	return &Runner{
		collection:   collection,
		environment:  environment,
		httpExecutor: httpExecutor,
	}
}

// Run executes wf, calling onChange after each state change.
func (r *Runner) Run(wf *Workflow, onChange func(ExecutionState)) ExecutionState {
	r.onChange = onChange
	r.status = StatusRunning
	r.wfSteps = wf.Steps
	r.state = r.buildInitialState(wf)
	r.ctx = &script.ExecutionContext{}
	r.syncFromCollection()
	r.notifyProgress()

	r.vm = goja.New()

	if err := r.setupConsole(); err != nil {
		return r.finalize(StatusFailed, fmt.Sprintf("console setup failed: %v", err))
	}

	if err := r.setupPmAPI(); err != nil {
		return r.finalize(StatusFailed, fmt.Sprintf("pm API setup failed: %v", err))
	}

	if err := script.SetupCryptoJS(r.vm); err != nil {
		return r.finalize(StatusFailed, fmt.Sprintf("CryptoJS setup failed: %v", err))
	}

	if err := r.setupWFAPI(); err != nil {
		return r.finalize(StatusFailed, fmt.Sprintf("wf API setup failed: %v", err))
	}

	transformed := transformScript(wf.Script)

	done := make(chan struct{})
	timeoutChan := make(chan struct{})
	go func() {
		select {
		case <-time.After(defaultScriptTimeout):
			r.vm.Interrupt("workflow timeout")
			close(timeoutChan)
		case <-done:
		}
	}()

	_, err := r.vm.RunString(transformed)
	close(done)

	select {
	case <-timeoutChan:
		return r.finalize(StatusFailed, fmt.Sprintf("workflow timed out after %v", defaultScriptTimeout))
	default:
	}

	if err != nil {
		if r.status == StatusFailed || r.status == StatusStopped {
			return r.finalize(r.status, r.state.Error)
		}
		return r.finalize(StatusFailed, fmt.Sprintf("script error: %v", err))
	}

	if r.status == StatusFailed || r.status == StatusStopped {
		return r.finalize(r.status, r.state.Error)
	}
	return r.finalize(StatusSuccess, "")
}

func (r *Runner) buildInitialState(wf *Workflow) ExecutionState {
	state := ExecutionState{
		WorkflowID:   wf.ID,
		WorkflowName: wf.Name,
		Status:       StatusRunning,
		StepIndex:    make(map[string]*StepState),
	}
	for _, s := range wf.Steps {
		ss := &StepState{
			ID:      s.ID,
			Request: s.Request,
			Status:  StatusPending,
		}
		state.Steps = append(state.Steps, ss)
		state.StepIndex[s.ID] = ss
	}
	return state
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

	r.notifyProgress()
	return r.state
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
		Logs:         append([]string{}, r.state.Logs...),
		StepIndex:    make(map[string]*StepState),
	}
	for _, ss := range r.state.Steps {
		cp := *ss
		snap.Steps = append(snap.Steps, &cp)
		snap.StepIndex[cp.ID] = &cp
	}
	r.onChange(snap)
}

func (r *Runner) addLog(msg string) {
	r.state.Logs = append(r.state.Logs, msg)
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
	result := &script.TestResult{}
	if err := script.SetupPmAPI(r.vm, r.ctx, result); err != nil {
		return err
	}

	// Add pm.environment as an alias for pm.environmentVariables (Postman-standard name).
	pmVal := r.vm.Get("pm")
	if pmVal == nil {
		return nil
	}
	pmObj, ok := pmVal.(*goja.Object)
	if !ok {
		return nil
	}
	envVarsVal := pmObj.Get("environmentVariables")
	if envVarsVal == nil {
		return nil
	}
	return pmObj.Set("environment", envVarsVal)
}

func (r *Runner) setupWFAPI() error {
	wfObj := r.vm.NewObject()

	if err := wfObj.Set("run", func(call goja.FunctionCall) goja.Value {
		if r.status == StatusFailed || r.status == StatusStopped {
			return r.emptyResultJS()
		}
		if len(call.Arguments) == 0 {
			return r.emptyResultJS()
		}
		stepID := call.Arguments[0].String()

		opts := RunOptions{Repeat: 1}
		if len(call.Arguments) > 1 {
			opts = r.parseRunOptions(call.Arguments[1])
		}

		result, err := r.runStep(stepID, opts)
		if err != nil {
			r.addLog(fmt.Sprintf("[ERROR] step %q: %v", stepID, err))
			return r.emptyResultJS()
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
				r.addLog(fmt.Sprintf("[SKIP] %s", stepID))
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
			time.Sleep(time.Duration(ms) * time.Millisecond)
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
	ss, ok := r.state.StepIndex[stepID]
	if !ok {
		// Step not declared in YAML — create a transient entry for tracking.
		requestPath := r.findRequestPath(stepID)
		ss = &StepState{ID: stepID, Request: requestPath, Status: StatusPending}
		r.state.Steps = append(r.state.Steps, ss)
		r.state.StepIndex[stepID] = ss
	}

	requestPath := ss.Request
	if requestPath == "" {
		requestPath = r.findRequestPath(stepID)
		ss.Request = requestPath
	}

	if requestPath == "" {
		ss.Status = StatusFailed
		r.notifyProgress()
		return nil, fmt.Errorf("step %q has no request path", stepID)
	}

	item, err := r.resolveRequest(requestPath)
	if err != nil {
		ss.Status = StatusFailed
		r.notifyProgress()
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
			ss.Status = StatusFailed
			r.notifyProgress()
			return nil, fmt.Errorf("pre-script for step %q failed: %w", stepID, err)
		}
	}

	var responses []*StepResponse
	var last *StepResponse

	for i := 0; i < repeat; i++ {
		ss.Progress = i + 1
		r.notifyProgress()

		stepResp, rawResp, execErr := r.executeItem(item)
		if execErr != nil {
			ss.Status = StatusFailed
			r.notifyProgress()
			return nil, execErr
		}
		responses = append(responses, stepResp)
		last = stepResp
		if rawResp != nil {
			ss.LastResponse = rawResp
		}

		if opts.Until != nil && opts.Until(stepResp) {
			break
		}
	}

	if wfStep != nil && wfStep.PostScript != "" && last != nil {
		if err := r.runStepScript(wfStep.PostScript, stepID, last); err != nil {
			ss.Status = StatusFailed
			r.notifyProgress()
			return nil, fmt.Errorf("post-script for step %q failed: %w", stepID, err)
		}
	}

	ss.Status = StatusSuccess
	r.notifyProgress()

	return &StepRunResult{
		Response:  last,
		Responses: responses,
		Last:      last,
	}, nil
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

	parts := strings.Split(path, "/")
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

func (r *Runner) executeItem(item *postman.Item) (*StepResponse, *http.Response, error) {
	// Write workflow-script variable changes to the collection before running.
	r.syncToCollection()

	variables := r.getVariables()

	if item.IsGRPC() {
		stepResp, err := r.executeGRPC(item, variables)
		return stepResp, nil, err
	}

	if item.IsServiceBus() {
		return r.executeMessageBus(item, "ASB")
	}

	if item.IsAMQP() {
		return r.executeMessageBus(item, "AMQP")
	}

	if !item.IsRequest() || item.Request == nil {
		return nil, nil, fmt.Errorf("item %q is not a request", item.Name)
	}

	resp, _ := r.httpExecutor.Execute(item.Request, item, r.collection, r.environment, variables)

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

	return stepResp, resp, nil
}

func (r *Runner) executeMessageBus(item *postman.Item, protocol string) (*StepResponse, *http.Response, error) {
	if item.Request == nil {
		return nil, nil, fmt.Errorf("item %q has no request", item.Name)
	}

	// Run Postman pre-request scripts (e.g. to generate IDs before the send).
	if len(item.Events) > 0 {
		errs := script.ExecutePreRequestScripts(item.Events, r.ctx)
		for _, e := range errs {
			r.addLog(fmt.Sprintf("[%s PRE-SCRIPT ERROR] %s", protocol, e))
		}
		r.syncFromCollection()
	}

	variables := r.getVariables()

	var resp *http.Response
	switch protocol {
	case "ASB":
		resp = servicebus.Send(item.Request, variables)
	case "AMQP":
		resp = amqp.Publish(item.Request, variables)
	default:
		return nil, nil, fmt.Errorf("unknown message bus protocol: %s", protocol)
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

	return stepResp, resp, nil
}

func (r *Runner) executeGRPC(item *postman.Item, variables []postman.VariableSource) (*StepResponse, error) {
	rawURL := postman.ResolveVariables(item.Request.URL.Raw, variables)

	resolvedReq := *item.Request
	resolvedReq.URL.Raw = rawURL
	if item.Request.Body != nil {
		resolvedReq.Body = &postman.Body{
			Mode: item.Request.Body.Mode,
			Raw:  postman.ResolveVariables(item.Request.Body.Raw, variables),
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()
	respJSON, err := internalgrpc.ExecuteRequest(ctx, &resolvedReq)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		r.addLog(fmt.Sprintf("[gRPC ERROR] %v", err))
		return &StepResponse{
			Code: 0,
			Body: fmt.Sprintf(`{"error":%q}`, err.Error()),
			Time: elapsed,
		}, nil
	}

	return &StepResponse{
		Code: 200,
		Body: respJSON,
		Time: elapsed,
	}, nil
}

func (r *Runner) getVariables() []postman.VariableSource {
	return postman.GetAllVariables(r.collection, nil, r.environment)
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
		return r.emptyResultJS()
	}
	obj := r.vm.NewObject()

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

func (r *Runner) emptyResultJS() goja.Value {
	obj := r.vm.NewObject()
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
