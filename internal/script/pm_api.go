package script

import (
	"fmt"

	"github.com/dop251/goja"
	"postOffice/internal/postman"
)

func setupPmAPI(vm *goja.Runtime, ctx *ExecutionContext, result *TestResult) error {
	pmObj := vm.NewObject()

	testFunc := func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			result.AddError("pm.test requires 2 arguments: name and function")
			return goja.Undefined()
		}

		name := call.Arguments[0].String()
		fn, ok := goja.AssertFunction(call.Arguments[1])
		if !ok {
			result.AddError(fmt.Sprintf("pm.test second argument must be a function for test '%s'", name))
			return goja.Undefined()
		}

		_, err := fn(goja.Undefined())
		if err != nil {
			result.AddTest(name, false, err.Error())
		} else {
			result.AddTest(name, true, "")
		}

		return goja.Undefined()
	}

	if err := pmObj.Set("test", testFunc); err != nil {
		return fmt.Errorf("failed to set pm.test: %w", err)
	}

	collectionVarsObj := vm.NewObject()
	collectionVarSetter := makeVariableSetter(&ctx.CollectionVars)
	if err := collectionVarsObj.Set("set", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		key := call.Arguments[0].String()
		value := call.Arguments[1].String()
		collectionVarSetter(key, value)
		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("failed to set collectionVariables.set: %w", err)
	}

	envVarsObj := vm.NewObject()
	envVarSetter := makeEnvVariableSetter(&ctx.EnvironmentVars)
	if err := envVarsObj.Set("set", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		key := call.Arguments[0].String()
		value := call.Arguments[1].String()
		envVarSetter(key, value)
		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("failed to set environmentVariables.set: %w", err)
	}

	if err := pmObj.Set("collectionVariables", collectionVarsObj); err != nil {
		return fmt.Errorf("failed to set pm.collectionVariables: %w", err)
	}

	if err := pmObj.Set("environmentVariables", envVarsObj); err != nil {
		return fmt.Errorf("failed to set pm.environmentVariables: %w", err)
	}

	responseObj := vm.NewObject()
	if err := responseObj.Set("text", ctx.Response.Body); err != nil {
		return fmt.Errorf("failed to set pm.response.text: %w", err)
	}

	toObj := vm.NewObject()
	haveObj := vm.NewObject()

	statusFunc := func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(vm.NewGoError(fmt.Errorf("status() requires expected status code")))
		}

		expectedStatus := call.Arguments[0].ToInteger()
		actualStatus := int64(ctx.Response.StatusCode)

		if actualStatus != expectedStatus {
			panic(vm.NewGoError(fmt.Errorf("expected status %d but got %d", expectedStatus, actualStatus)))
		}

		return goja.Undefined()
	}

	if err := haveObj.Set("status", statusFunc); err != nil {
		return fmt.Errorf("failed to set status: %w", err)
	}

	if err := toObj.Set("have", haveObj); err != nil {
		return fmt.Errorf("failed to set have: %w", err)
	}

	if err := responseObj.Set("to", toObj); err != nil {
		return fmt.Errorf("failed to set to: %w", err)
	}

	if err := pmObj.Set("response", responseObj); err != nil {
		return fmt.Errorf("failed to set pm.response: %w", err)
	}

	if err := vm.Set("pm", pmObj); err != nil {
		return fmt.Errorf("failed to set pm global: %w", err)
	}

	if err := vm.Set("responseBody", ctx.Response.Body); err != nil {
		return fmt.Errorf("failed to set responseBody global: %w", err)
	}

	return nil
}

func makeVariableSetter(vars *[]postman.Variable) func(string, string) {
	return func(key, value string) {
		for i := range *vars {
			if (*vars)[i].Key == key {
				(*vars)[i].Value = value
				return
			}
		}
		*vars = append(*vars, postman.Variable{
			Key:   key,
			Value: value,
		})
	}
}

func makeVariableGetter(vars *[]postman.Variable) func(string) (string, bool) {
	return func(key string) (string, bool) {
		for _, v := range *vars {
			if v.Key == key {
				return v.Value, true
			}
		}
		return "", false
	}
}

func makeEnvVariableSetter(vars *[]postman.EnvVariable) func(string, string) {
	return func(key, value string) {
		for i := range *vars {
			if (*vars)[i].Key == key {
				(*vars)[i].Value = value
				return
			}
		}
		*vars = append(*vars, postman.EnvVariable{
			Key:     key,
			Value:   value,
			Enabled: true,
			Type:    "default",
		})
	}
}

func makeEnvVariableGetter(vars *[]postman.EnvVariable) func(string) (string, bool) {
	return func(key string) (string, bool) {
		for _, v := range *vars {
			if v.Key == key {
				return v.Value, true
			}
		}
		return "", false
	}
}
