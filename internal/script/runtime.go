package script

import (
	"fmt"
	"strings"

	"github.com/dop251/goja"
	"postOffice/internal/postman"
)

type Runtime struct {
	vm *goja.Runtime
}

func NewRuntime() *Runtime {
	return &Runtime{
		vm: goja.New(),
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

	_, err := r.vm.RunString(scriptCode)
	if err != nil {
		result.AddError(fmt.Sprintf("script execution failed: %v", err))
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
