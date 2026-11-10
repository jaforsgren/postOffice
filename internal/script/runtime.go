package script

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"
	"postOffice/internal/postman"
)

// DefaultScriptTimeout bounds each synchronous JS segment: a script body or
// one queued callback. Waiting on timers and HTTP is not counted.
const DefaultScriptTimeout = 5 * time.Second

type Runtime struct {
	vm      *goja.Runtime
	timeout time.Duration
}

func NewRuntime() *Runtime {
	return NewRuntimeWithTimeout(DefaultScriptTimeout)
}

func NewRuntimeWithTimeout(timeout time.Duration) *Runtime {
	return &Runtime{
		vm:      goja.New(),
		timeout: timeout,
	}
}

func (r *Runtime) ExecuteTestScript(script postman.Script, ctx *ExecutionContext) *TestResult {
	return r.execute(PhaseTest, []postman.Script{script}, ctx)
}

func (r *Runtime) ExecutePreRequestScript(script postman.Script, ctx *ExecutionContext) *TestResult {
	return r.execute(PhasePreRequest, []postman.Script{script}, ctx)
}

// ExecuteChain runs every phase script in eventScopes in order (outermost
// scope first: collection, folders, request) in one VM, so implicit globals a
// collection script defines are visible to the request's script, as in
// Postman. A pre-request skip stops the remaining scripts.
func (r *Runtime) ExecuteChain(phase Phase, eventScopes [][]postman.Event, ctx *ExecutionContext) *TestResult {
	var scripts []postman.Script
	for _, events := range eventScopes {
		for _, event := range events {
			if event.Listen == string(phase) {
				scripts = append(scripts, event.Script)
			}
		}
	}
	return r.execute(phase, scripts, ctx)
}

func (r *Runtime) execute(phase Phase, scripts []postman.Script, ctx *ExecutionContext) *TestResult {
	result := newTestResult()

	scripts = nonEmptyScripts(scripts)
	if len(scripts) == 0 {
		return result
	}

	loop := newEventLoop(r.vm, r.timeout)
	if err := setupPmAPI(r.vm, ctx, result, phase, loop); err != nil {
		result.AddError(fmt.Sprintf("failed to setup pm API: %v", err))
		return result
	}
	if err := setupCryptoJS(r.vm); err != nil {
		result.AddError(fmt.Sprintf("failed to setup CryptoJS: %v", err))
		return result
	}

	for _, script := range scripts {
		if result.Skipped {
			break
		}
		if err := loop.RunScript(isolateScope(script)); err != nil {
			result.AddError(scriptErrorMessage(err))
		}
		for _, err := range loop.Drain() {
			result.AddError(scriptErrorMessage(err))
		}
	}

	return result
}

func nonEmptyScripts(scripts []postman.Script) []postman.Script {
	var nonEmpty []postman.Script
	for _, script := range scripts {
		if len(script.Exec) > 0 {
			nonEmpty = append(nonEmpty, script)
		}
	}
	return nonEmpty
}

// isolateScope wraps a script in its own function scope. Postman does the
// same: SAPI's collection and request test scripts both declare
// `const CLIENT_ERROR`, which would be a redeclaration error in one shared
// scope, while implicit globals (`sapi = {...}`) still carry over.
func isolateScope(script postman.Script) string {
	return "(function () {\n" + strings.Join(script.Exec, "\n") + "\n})();"
}

func scriptErrorMessage(err error) string {
	if errors.Is(err, ErrScriptTimeout) || errors.Is(err, ErrEventLoopTimeout) {
		return err.Error()
	}
	return fmt.Sprintf("script execution failed: %v", err)
}
