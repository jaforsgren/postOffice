package script

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/dop251/goja"
	"github.com/tidwall/gjson"
	"postOffice/internal/postman"
)

const (
	successStatusMin     = 200
	successStatusMax     = 300
	clientErrorStatusMin = 400
)

// SetupPmAPI registers the Postman-compatible pm object, console and timers
// on vm. Callers outside this package (e.g. the workflow runtime) must Drain
// the returned loop after running their script so queued pm.sendRequest and
// setTimeout callbacks execute.
func SetupPmAPI(vm *goja.Runtime, ctx *ExecutionContext, result *TestResult) (*EventLoop, error) {
	loop := newEventLoop(vm, DefaultScriptTimeout)
	if err := setupPmAPI(vm, ctx, result, PhaseTest, loop); err != nil {
		return nil, err
	}
	return loop, nil
}

func setupPmAPI(vm *goja.Runtime, ctx *ExecutionContext, result *TestResult, phase Phase, loop *EventLoop) error {
	if err := loop.install(); err != nil {
		return err
	}

	pmObj := vm.NewObject()

	if err := pmObj.Set("test", func(call goja.FunctionCall) goja.Value {
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

		if _, err := fn(goja.Undefined()); err != nil {
			result.AddTest(name, false, assertionMessage(err))
		} else {
			result.AddTest(name, true, "")
		}
		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("failed to set pm.test: %w", err)
	}

	expect, err := newExpect(vm)
	if err != nil {
		return err
	}
	if err := pmObj.Set("expect", expect); err != nil {
		return fmt.Errorf("failed to set pm.expect: %w", err)
	}

	if err := setupVariableAPIs(vm, pmObj, ctx, result); err != nil {
		return err
	}

	info, err := newInfoObject(vm, ctx.Info, phase)
	if err != nil {
		return err
	}
	if err := pmObj.Set("info", info); err != nil {
		return fmt.Errorf("failed to set pm.info: %w", err)
	}

	execution, err := newExecutionObject(vm, phase, result)
	if err != nil {
		return err
	}
	if err := pmObj.Set("execution", execution); err != nil {
		return fmt.Errorf("failed to set pm.execution: %w", err)
	}

	if ctx.Request != nil {
		request, err := newRequestObject(vm, ctx.Request)
		if err != nil {
			return err
		}
		if err := pmObj.Set("request", request); err != nil {
			return fmt.Errorf("failed to set pm.request: %w", err)
		}
	}

	if err := installSendRequest(vm, pmObj, ctx, loop); err != nil {
		return fmt.Errorf("failed to set pm.sendRequest: %w", err)
	}

	if ctx.Response != nil {
		response, err := newPmResponseObject(vm, ctx.Response)
		if err != nil {
			return err
		}
		if err := pmObj.Set("response", response); err != nil {
			return fmt.Errorf("failed to set pm.response: %w", err)
		}
		if err := vm.Set("responseBody", ctx.Response.Body); err != nil {
			return fmt.Errorf("failed to set responseBody global: %w", err)
		}
	}

	if err := vm.Set("pm", pmObj); err != nil {
		return fmt.Errorf("failed to set pm global: %w", err)
	}

	return setupConsole(vm, result)
}

// assertionMessage strips goja's " at <location>" suffix so a failed
// pm.expect reads like Postman's "expected 1 to equal 2".
func assertionMessage(err error) string {
	if exception, ok := err.(*goja.Exception); ok {
		if obj, ok := exception.Value().(*goja.Object); ok {
			if message := obj.Get("message"); message != nil && !goja.IsUndefined(message) {
				if name := obj.Get("name"); name != nil && name.String() == "AssertionError" {
					return name.String() + ": " + message.String()
				}
			}
		}
	}
	return err.Error()
}

func setupConsole(vm *goja.Runtime, result *TestResult) error {
	consoleObj := vm.NewObject()
	for _, method := range []string{"log", "warn", "error", "info", "debug"} {
		if err := consoleObj.Set(method, func(call goja.FunctionCall) goja.Value {
			args := make([]string, len(call.Arguments))
			for i, arg := range call.Arguments {
				args[i] = arg.String()
			}
			result.AddLog("[" + method + "] " + strings.Join(args, " "))
			return goja.Undefined()
		}); err != nil {
			return fmt.Errorf("failed to set console.%s: %w", method, err)
		}
	}
	if err := vm.Set("console", consoleObj); err != nil {
		return fmt.Errorf("failed to set console global: %w", err)
	}
	return nil
}

// variableScope is one get/set/unset store exposed to scripts
// (pm.collectionVariables, pm.environment, pm.variables).
type variableScope struct {
	get   func(string) (string, bool)
	set   func(string, string)
	unset func(string)
}

func newVariableObject(vm *goja.Runtime, owner string, scope variableScope, onSet func(key, value string)) (*goja.Object, error) {
	obj := vm.NewObject()
	err := setMethods(obj, owner, map[string]func(goja.FunctionCall) goja.Value{
		"get": func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return goja.Undefined()
			}
			if value, ok := scope.get(call.Arguments[0].String()); ok {
				return vm.ToValue(value)
			}
			return goja.Undefined()
		},
		"has": func(call goja.FunctionCall) goja.Value {
			_, ok := scope.get(call.Argument(0).String())
			return vm.ToValue(ok)
		},
		"set": func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				return goja.Undefined()
			}
			key := call.Arguments[0].String()
			value := call.Arguments[1].String()
			scope.set(key, value)
			if onSet != nil {
				onSet(key, value)
			}
			return goja.Undefined()
		},
		"unset": func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) >= 1 {
				scope.unset(call.Arguments[0].String())
			}
			return goja.Undefined()
		},
	})
	if err != nil {
		return nil, err
	}
	return obj, nil
}

func setupVariableAPIs(vm *goja.Runtime, pmObj *goja.Object, ctx *ExecutionContext, result *TestResult) error {
	collection := variableScope{
		get:   makeVariableGetter(&ctx.CollectionVars),
		set:   makeVariableSetter(&ctx.CollectionVars),
		unset: makeUnsetter(&ctx.CollectionVars, func(v postman.Variable) string { return v.Key }),
	}
	environment := variableScope{
		get:   makeEnvVariableGetter(&ctx.EnvironmentVars),
		set:   makeEnvVariableSetter(&ctx.EnvironmentVars),
		unset: makeUnsetter(&ctx.EnvironmentVars, func(v postman.EnvVariable) string { return v.Key }),
	}
	// pm.variables resolves environment before collection and writes to the
	// collection scope, the closest postOffice has to Postman's local scope.
	merged := variableScope{
		get: func(key string) (string, bool) {
			if value, ok := environment.get(key); ok {
				return value, true
			}
			return collection.get(key)
		},
		set:   collection.set,
		unset: collection.unset,
	}

	collectionObj, err := newVariableObject(vm, "pm.collectionVariables", collection, func(key, value string) {
		result.AddLog(fmt.Sprintf("[collection] %s = %s", key, value))
	})
	if err != nil {
		return err
	}
	environmentObj, err := newVariableObject(vm, "pm.environment", environment, nil)
	if err != nil {
		return err
	}
	if err := environmentObj.Set("name", ctx.EnvironmentName); err != nil {
		return fmt.Errorf("failed to set pm.environment.name: %w", err)
	}
	variablesObj, err := newVariableObject(vm, "pm.variables", merged, nil)
	if err != nil {
		return err
	}

	for name, obj := range map[string]*goja.Object{
		"collectionVariables": collectionObj,
		"environment":         environmentObj,
		// Legacy postOffice name for pm.environment.
		"environmentVariables": environmentObj,
		"variables":            variablesObj,
	} {
		if err := pmObj.Set(name, obj); err != nil {
			return fmt.Errorf("failed to set pm.%s: %w", name, err)
		}
	}
	return nil
}

// newPmResponseObject extends the shared response shape with Postman's
// pm.response.to.have/to.be assertion helpers.
func newPmResponseObject(vm *goja.Runtime, resp *ResponseData) (*goja.Object, error) {
	responseObj, err := newResponseObject(vm, resp)
	if err != nil {
		return nil, err
	}

	headers := headersFromMap(resp.Headers)
	toObj := vm.NewObject()
	haveObj := vm.NewObject()
	beObj := vm.NewObject()

	if err := setMethods(haveObj, "pm.response.to.have", map[string]func(goja.FunctionCall) goja.Value{
		"status": func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(vm.NewGoError(fmt.Errorf("status() requires expected status code")))
			}
			expected := call.Arguments[0].ToInteger()
			if actual := int64(resp.StatusCode); actual != expected {
				panic(vm.NewGoError(fmt.Errorf("expected status %d but got %d", expected, actual)))
			}
			return goja.Undefined()
		},
		"header": func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(vm.NewGoError(fmt.Errorf("header() requires header name")))
			}
			name := call.Arguments[0].String()
			if findHeader(headers, name) < 0 {
				panic(vm.NewGoError(fmt.Errorf("header '%s' not found in response", name)))
			}
			if len(call.Arguments) >= 2 {
				expected := call.Arguments[1].String()
				matches := slices.ContainsFunc(headers, func(h postman.Header) bool {
					return strings.EqualFold(h.Key, name) && h.Value == expected
				})
				if !matches {
					panic(vm.NewGoError(fmt.Errorf("header '%s' expected value '%s' but got %v", name, expected, resp.Headers[name])))
				}
			}
			return goja.Undefined()
		},
		"jsonBody": func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(vm.NewGoError(fmt.Errorf("jsonBody() requires JSONPath")))
			}
			path := call.Arguments[0].String()
			found := gjson.Get(resp.Body, path)
			if !found.Exists() {
				panic(vm.NewGoError(fmt.Errorf("JSONPath '%s' not found in response", path)))
			}
			if len(call.Arguments) >= 2 {
				var expected any
				if err := json.Unmarshal([]byte(call.Arguments[1].String()), &expected); err != nil {
					if expectedStr, actualStr := call.Arguments[1].String(), found.String(); actualStr != expectedStr {
						panic(vm.NewGoError(fmt.Errorf("JSONPath '%s' expected '%v' but got '%v'", path, expectedStr, actualStr)))
					}
				} else if found.Value() != expected {
					panic(vm.NewGoError(fmt.Errorf("JSONPath '%s' expected '%v' but got '%v'", path, expected, found.Value())))
				}
			}
			return goja.Undefined()
		},
	}); err != nil {
		return nil, err
	}

	statusGetters := map[string]func() error{
		"ok": func() error {
			if resp.StatusCode < successStatusMin || resp.StatusCode >= successStatusMax {
				return fmt.Errorf("expected 2xx status but got %d", resp.StatusCode)
			}
			return nil
		},
		"error": func() error {
			if resp.StatusCode < clientErrorStatusMin {
				return fmt.Errorf("expected 4xx/5xx status but got %d", resp.StatusCode)
			}
			return nil
		},
	}
	for name, check := range statusGetters {
		if err := beObj.DefineAccessorProperty(name, vm.ToValue(func(goja.FunctionCall) goja.Value {
			if err := check(); err != nil {
				panic(vm.NewGoError(err))
			}
			return goja.Undefined()
		}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
			return nil, fmt.Errorf("failed to define pm.response.to.be.%s: %w", name, err)
		}
	}

	if err := toObj.Set("have", haveObj); err != nil {
		return nil, fmt.Errorf("failed to set have: %w", err)
	}
	if err := toObj.Set("be", beObj); err != nil {
		return nil, fmt.Errorf("failed to set be: %w", err)
	}
	if err := responseObj.Set("to", toObj); err != nil {
		return nil, fmt.Errorf("failed to set to: %w", err)
	}
	return responseObj, nil
}

func makeUnsetter[T any](vars *[]T, keyOf func(T) string) func(string) {
	return func(key string) {
		*vars = slices.DeleteFunc(*vars, func(v T) bool { return keyOf(v) == key })
	}
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
