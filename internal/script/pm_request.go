package script

import (
	"fmt"
	"strings"

	"github.com/dop251/goja"
)

const (
	urlSchemeSeparator = "://"
	rootPath           = "/"
	rawBodyMode        = "raw"
)

// urlPath returns the path part of a raw, possibly unresolved URL such as
// "{{baseUrl}}/api/v1/users?x=1", mirroring Postman's Url.getPath().
func urlPath(raw string) string {
	path, _, _ := strings.Cut(raw, "?")
	path, _, _ = strings.Cut(path, "#")
	if _, afterScheme, found := strings.Cut(path, urlSchemeSeparator); found {
		path = afterScheme
	}
	if i := strings.Index(path, rootPath); i >= 0 {
		return path[i:]
	}
	return rootPath
}

func defineGetter(vm *goja.Runtime, obj *goja.Object, name string, get func() any) error {
	if err := obj.DefineAccessorProperty(name, vm.ToValue(func(goja.FunctionCall) goja.Value {
		return vm.ToValue(get())
	}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		return fmt.Errorf("failed to define %s: %w", name, err)
	}
	return nil
}

func setMethods(obj *goja.Object, owner string, methods map[string]func(goja.FunctionCall) goja.Value) error {
	for name, method := range methods {
		if err := obj.Set(name, method); err != nil {
			return fmt.Errorf("failed to set %s.%s: %w", owner, name, err)
		}
	}
	return nil
}

// newRequestObject builds pm.request over rd. Writes go straight to rd, which
// the caller applies to the outgoing HTTP request after the pre-request phase.
func newRequestObject(vm *goja.Runtime, rd *RequestData) (*goja.Object, error) {
	request := vm.NewObject()

	headers, err := newHeaderList(vm, &rd.Headers, true)
	if err != nil {
		return nil, err
	}
	if err := request.Set("headers", headers); err != nil {
		return nil, fmt.Errorf("failed to set pm.request.headers: %w", err)
	}

	if err := defineGetter(vm, request, "method", func() any { return rd.Method }); err != nil {
		return nil, err
	}

	url := vm.NewObject()
	if err := setMethods(url, "pm.request.url", map[string]func(goja.FunctionCall) goja.Value{
		"toString": func(goja.FunctionCall) goja.Value { return vm.ToValue(rd.URL) },
		"getPath":  func(goja.FunctionCall) goja.Value { return vm.ToValue(urlPath(rd.URL)) },
	}); err != nil {
		return nil, err
	}
	if err := request.Set("url", url); err != nil {
		return nil, fmt.Errorf("failed to set pm.request.url: %w", err)
	}

	body := vm.NewObject()
	if err := defineGetter(vm, body, "raw", func() any { return rd.Body }); err != nil {
		return nil, err
	}
	if err := defineGetter(vm, body, "mode", func() any { return rawBodyMode }); err != nil {
		return nil, err
	}
	if err := setMethods(body, "pm.request.body", map[string]func(goja.FunctionCall) goja.Value{
		"toString": func(goja.FunctionCall) goja.Value { return vm.ToValue(rd.Body) },
		"update": func(call goja.FunctionCall) goja.Value {
			rd.Body = bodyUpdateValue(vm, call.Argument(0))
			return goja.Undefined()
		},
	}); err != nil {
		return nil, err
	}
	if err := request.Set("body", body); err != nil {
		return nil, fmt.Errorf("failed to set pm.request.body: %w", err)
	}

	return request, nil
}

// bodyUpdateValue accepts Postman's body.update forms: a raw string, or a
// {mode: "raw", raw: "..."} options object.
func bodyUpdateValue(vm *goja.Runtime, arg goja.Value) string {
	if exported, ok := arg.Export().(string); ok {
		return exported
	}
	options := arg.ToObject(vm)
	if mode := options.Get("mode"); mode != nil && mode.String() != rawBodyMode {
		panic(vm.NewTypeError("pm.request.body.update only supports raw bodies, got mode %q", mode.String()))
	}
	raw := options.Get("raw")
	if raw == nil || goja.IsUndefined(raw) {
		panic(vm.NewTypeError("pm.request.body.update requires a string or {mode: 'raw', raw}"))
	}
	return raw.String()
}

func newInfoObject(vm *goja.Runtime, info RequestInfo, phase Phase) (*goja.Object, error) {
	obj := vm.NewObject()
	for name, value := range map[string]any{
		"requestName":    info.RequestName,
		"requestId":      info.RequestID,
		"eventName":      string(phase),
		"iteration":      0,
		"iterationCount": 1,
	} {
		if err := obj.Set(name, value); err != nil {
			return nil, fmt.Errorf("failed to set pm.info.%s: %w", name, err)
		}
	}
	return obj, nil
}

func newExecutionObject(vm *goja.Runtime, phase Phase, result *TestResult) (*goja.Object, error) {
	obj := vm.NewObject()
	err := setMethods(obj, "pm.execution", map[string]func(goja.FunctionCall) goja.Value{
		// Only meaningful before the request is sent; Postman ignores it in test scripts.
		"skipRequest": func(goja.FunctionCall) goja.Value {
			if phase == PhasePreRequest {
				result.Skipped = true
			}
			return goja.Undefined()
		},
		"setNextRequest": func(call goja.FunctionCall) goja.Value {
			next := call.Argument(0)
			if goja.IsNull(next) || goja.IsUndefined(next) {
				result.NextRequest = NextRequest{Set: true}
			} else {
				result.NextRequest = NextRequest{Set: true, Name: next.String()}
			}
			return goja.Undefined()
		},
	})
	if err != nil {
		return nil, err
	}
	return obj, nil
}
