package script

import (
	"encoding/json"
	"fmt"

	"github.com/dop251/goja"
	"github.com/tidwall/gjson"
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
	collectionVarGetter := makeVariableGetter(&ctx.CollectionVars)
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

	if err := collectionVarsObj.Set("get", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		key := call.Arguments[0].String()
		if value, ok := collectionVarGetter(key); ok {
			return vm.ToValue(value)
		}
		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("failed to set collectionVariables.get: %w", err)
	}

	envVarsObj := vm.NewObject()
	envVarSetter := makeEnvVariableSetter(&ctx.EnvironmentVars)
	envVarGetter := makeEnvVariableGetter(&ctx.EnvironmentVars)
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

	if err := envVarsObj.Set("get", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		key := call.Arguments[0].String()
		if value, ok := envVarGetter(key); ok {
			return vm.ToValue(value)
		}
		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("failed to set environmentVariables.get: %w", err)
	}

	if err := pmObj.Set("collectionVariables", collectionVarsObj); err != nil {
		return fmt.Errorf("failed to set pm.collectionVariables: %w", err)
	}

	if err := pmObj.Set("environmentVariables", envVarsObj); err != nil {
		return fmt.Errorf("failed to set pm.environmentVariables: %w", err)
	}

	variablesObj := vm.NewObject()
	if err := variablesObj.Set("get", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		key := call.Arguments[0].String()
		if value, ok := envVarGetter(key); ok {
			return vm.ToValue(value)
		}
		if value, ok := collectionVarGetter(key); ok {
			return vm.ToValue(value)
		}
		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("failed to set pm.variables.get: %w", err)
	}

	if err := variablesObj.Set("set", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		key := call.Arguments[0].String()
		value := call.Arguments[1].String()
		collectionVarSetter(key, value)
		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("failed to set pm.variables.set: %w", err)
	}

	if err := pmObj.Set("variables", variablesObj); err != nil {
		return fmt.Errorf("failed to set pm.variables: %w", err)
	}

	if ctx.Response != nil {
		responseObj := vm.NewObject()

		textFunc := func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(ctx.Response.Body)
		}
		if err := responseObj.Set("text", textFunc); err != nil {
			return fmt.Errorf("failed to set pm.response.text: %w", err)
		}

		jsonFunc := func(call goja.FunctionCall) goja.Value {
			var data interface{}
			if err := json.Unmarshal([]byte(ctx.Response.Body), &data); err != nil {
				panic(vm.NewGoError(fmt.Errorf("failed to parse JSON: %w", err)))
			}
			return vm.ToValue(data)
		}
		if err := responseObj.Set("json", jsonFunc); err != nil {
			return fmt.Errorf("failed to set pm.response.json: %w", err)
		}

		if err := responseObj.Set("code", ctx.Response.StatusCode); err != nil {
			return fmt.Errorf("failed to set pm.response.code: %w", err)
		}

		if err := responseObj.Set("status", ctx.Response.Status); err != nil {
			return fmt.Errorf("failed to set pm.response.status: %w", err)
		}

		if err := responseObj.Set("responseTime", ctx.Response.ResponseTime); err != nil {
			return fmt.Errorf("failed to set pm.response.responseTime: %w", err)
		}

		if err := responseObj.Set("headers", ctx.Response.Headers); err != nil {
			return fmt.Errorf("failed to set pm.response.headers: %w", err)
		}

		toObj := vm.NewObject()
		haveObj := vm.NewObject()
		beObj := vm.NewObject()

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

		headerFunc := func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(vm.NewGoError(fmt.Errorf("header() requires header name")))
			}

			headerName := call.Arguments[0].String()
			headerValues, exists := ctx.Response.Headers[headerName]

			if !exists {
				panic(vm.NewGoError(fmt.Errorf("header '%s' not found in response", headerName)))
			}

			if len(call.Arguments) >= 2 {
				expectedValue := call.Arguments[1].String()
				found := false
				for _, v := range headerValues {
					if v == expectedValue {
						found = true
						break
					}
				}
				if !found {
					panic(vm.NewGoError(fmt.Errorf("header '%s' expected value '%s' but got %v", headerName, expectedValue, headerValues)))
				}
			}

			return goja.Undefined()
		}
		if err := haveObj.Set("header", headerFunc); err != nil {
			return fmt.Errorf("failed to set header: %w", err)
		}

		jsonBodyFunc := func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(vm.NewGoError(fmt.Errorf("jsonBody() requires JSONPath")))
			}

			path := call.Arguments[0].String()
			result := gjson.Get(ctx.Response.Body, path)

			if !result.Exists() {
				panic(vm.NewGoError(fmt.Errorf("JSONPath '%s' not found in response", path)))
			}

			if len(call.Arguments) >= 2 {
				var expected interface{}
				if err := json.Unmarshal([]byte(call.Arguments[1].String()), &expected); err != nil {
					expectedStr := call.Arguments[1].String()
					actualStr := result.String()
					if actualStr != expectedStr {
						panic(vm.NewGoError(fmt.Errorf("JSONPath '%s' expected '%v' but got '%v'", path, expectedStr, actualStr)))
					}
				} else {
					if result.Value() != expected {
						panic(vm.NewGoError(fmt.Errorf("JSONPath '%s' expected '%v' but got '%v'", path, expected, result.Value())))
					}
				}
			}

			return goja.Undefined()
		}
		if err := haveObj.Set("jsonBody", jsonBodyFunc); err != nil {
			return fmt.Errorf("failed to set jsonBody: %w", err)
		}

		if err := toObj.Set("have", haveObj); err != nil {
			return fmt.Errorf("failed to set have: %w", err)
		}

		if err := toObj.Set("be", beObj); err != nil {
			return fmt.Errorf("failed to set be: %w", err)
		}

		if err := responseObj.Set("to", toObj); err != nil {
			return fmt.Errorf("failed to set to: %w", err)
		}

		if err := pmObj.Set("response", responseObj); err != nil {
			return fmt.Errorf("failed to set pm.response: %w", err)
		}

		if err := vm.Set("responseBody", ctx.Response.Body); err != nil {
			return fmt.Errorf("failed to set responseBody global: %w", err)
		}
	}

	if err := vm.Set("pm", pmObj); err != nil {
		return fmt.Errorf("failed to set pm global: %w", err)
	}

	// Define ok and error as getters if response context is present
	if ctx.Response != nil {
		_, err := vm.RunString(fmt.Sprintf(`
			(function() {
				Object.defineProperty(pm.response.to.be, 'ok', {
					get: function() {
						if (%d < 200 || %d >= 300) {
							throw new Error('expected 2xx status but got %d');
						}
					}
				});
				Object.defineProperty(pm.response.to.be, 'error', {
					get: function() {
						if (%d < 400) {
							throw new Error('expected 4xx/5xx status but got %d');
						}
					}
				});
			})();
		`, ctx.Response.StatusCode, ctx.Response.StatusCode, ctx.Response.StatusCode, ctx.Response.StatusCode, ctx.Response.StatusCode))
		if err != nil {
			return fmt.Errorf("failed to define be.ok and be.error getters: %w", err)
		}
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
