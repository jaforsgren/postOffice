package script

import (
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"
	"postOffice/internal/postman"
)

const DefaultScriptTimeout = 5 * time.Second

type Runtime struct {
	vm      *goja.Runtime
	timeout time.Duration
}

func NewRuntime() *Runtime {
	return &Runtime{
		vm:      goja.New(),
		timeout: DefaultScriptTimeout,
	}
}

func NewRuntimeWithTimeout(timeout time.Duration) *Runtime {
	return &Runtime{
		vm:      goja.New(),
		timeout: timeout,
	}
}

func (r *Runtime) ExecuteTestScript(script postman.Script, ctx *ExecutionContext) *TestResult {
	result := &TestResult{
		Tests:  []Test{},
		Errors: []string{},
	}

	if len(script.Exec) == 0 {
		return result
	}

	if err := setupPmAPI(r.vm, ctx, result); err != nil {
		result.AddError(fmt.Sprintf("failed to setup pm API: %v", err))
		return result
	}

	scriptCode := strings.Join(script.Exec, "\n")

	timeoutChan := make(chan struct{})
	done := make(chan struct{})
	go func() {
		select {
		case <-time.After(r.timeout):
			r.vm.Interrupt("timeout")
			close(timeoutChan)
		case <-done:
		}
	}()

	_, err := r.vm.RunString(scriptCode)
	close(done)

	select {
	case <-timeoutChan:
		result.AddError(fmt.Sprintf("script execution timeout after %v", r.timeout))
	default:
		if err != nil {
			result.AddError(fmt.Sprintf("script execution failed: %v", err))
		}
	}

	return result
}

func ExecuteTestScripts(events []postman.Event, ctx *ExecutionContext) *TestResult {
	combinedResult := &TestResult{
		Tests:  []Test{},
		Errors: []string{},
	}

	for _, event := range events {
		if event.Listen == "test" {
			runtime := NewRuntime()
			result := runtime.ExecuteTestScript(event.Script, ctx)

			combinedResult.Tests = append(combinedResult.Tests, result.Tests...)
			combinedResult.Errors = append(combinedResult.Errors, result.Errors...)
		}
	}

	return combinedResult
}

func ExecutePreRequestScripts(events []postman.Event, ctx *ExecutionContext) []string {
	var errors []string

	for _, event := range events {
		if event.Listen == "prerequest" {
			runtime := NewRuntime()
			result := runtime.ExecutePreRequestScript(event.Script, ctx)
			errors = append(errors, result.Errors...)
		}
	}

	return errors
}

func (r *Runtime) ExecutePreRequestScript(script postman.Script, ctx *ExecutionContext) *TestResult {
	result := &TestResult{
		Tests:  []Test{},
		Errors: []string{},
	}

	if len(script.Exec) == 0 {
		return result
	}

	if err := setupPmAPI(r.vm, ctx, result); err != nil {
		result.AddError(fmt.Sprintf("failed to setup pm API: %v", err))
		return result
	}

	scriptCode := strings.Join(script.Exec, "\n")

	timeoutChan := make(chan struct{})
	done := make(chan struct{})
	go func() {
		select {
		case <-time.After(r.timeout):
			r.vm.Interrupt("timeout")
			close(timeoutChan)
		case <-done:
		}
	}()

	_, err := r.vm.RunString(scriptCode)
	close(done)

	select {
	case <-timeoutChan:
		result.AddError(fmt.Sprintf("script execution timeout after %v", r.timeout))
	default:
		if err != nil {
			result.AddError(fmt.Sprintf("script execution failed: %v", err))
		}
	}

	return result
}
