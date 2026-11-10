package script

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/dop251/goja"
	"postOffice/internal/postman"
)

var (
	ErrSendRequestUnavailable = errors.New("pm.sendRequest is not available in this context")
	ErrJSONParseUnavailable   = errors.New("JSON.parse is not available in the script VM")
)

// newResponseObject builds the response shape shared by pm.response and the
// response pm.sendRequest hands to its callback.
func newResponseObject(vm *goja.Runtime, resp *ResponseData) (*goja.Object, error) {
	obj := vm.NewObject()

	for name, value := range map[string]any{
		"code":         resp.StatusCode,
		"status":       resp.Status,
		"responseTime": resp.ResponseTime,
		"responseSize": len(resp.Body),
	} {
		if err := obj.Set(name, value); err != nil {
			return nil, fmt.Errorf("failed to set response.%s: %w", name, err)
		}
	}

	headers := headersFromMap(resp.Headers)
	headerList, err := newHeaderList(vm, &headers, false)
	if err != nil {
		return nil, err
	}
	if err := obj.Set("headers", headerList); err != nil {
		return nil, fmt.Errorf("failed to set response.headers: %w", err)
	}

	// The VM's own JSON.parse yields real JS arrays and objects, which Go-map
	// wrappers from json.Unmarshal are not (Array.prototype methods, deep equality).
	jsonParse, ok := goja.AssertFunction(vm.Get("JSON").ToObject(vm).Get("parse"))
	if !ok {
		return nil, ErrJSONParseUnavailable
	}
	if err := setMethods(obj, "response", map[string]func(goja.FunctionCall) goja.Value{
		"text": func(goja.FunctionCall) goja.Value { return vm.ToValue(resp.Body) },
		"json": func(goja.FunctionCall) goja.Value {
			data, err := jsonParse(goja.Undefined(), vm.ToValue(resp.Body))
			if err != nil {
				panic(vm.NewGoError(fmt.Errorf("failed to parse JSON: %w", err)))
			}
			return data
		},
	}); err != nil {
		return nil, err
	}

	return obj, nil
}

// sendRequestSpecFromJS normalises Postman's pm.sendRequest argument: a URL
// string, or {url, method, header, body: {mode: "raw", raw}} where header is
// either a {name: value} object or a [{key, value}] list.
func sendRequestSpecFromJS(vm *goja.Runtime, arg goja.Value) SendRequestSpec {
	if url, ok := arg.Export().(string); ok {
		return SendRequestSpec{Method: http.MethodGet, URL: url}
	}

	options := arg.ToObject(vm)
	spec := SendRequestSpec{Method: http.MethodGet, URL: options.Get("url").String()}
	if method := options.Get("method"); method != nil && !goja.IsUndefined(method) {
		spec.Method = method.String()
	}

	if header := options.Get("header"); header != nil && !goja.IsUndefined(header) && !goja.IsNull(header) {
		headerObj := header.ToObject(vm)
		if headerObj.ClassName() == "Array" {
			for _, key := range headerObj.Keys() {
				spec.Headers = append(spec.Headers, headerFromJS(vm, headerObj.Get(key)))
			}
		} else {
			for _, key := range headerObj.Keys() {
				spec.Headers = append(spec.Headers, postman.Header{Key: key, Value: headerObj.Get(key).String()})
			}
		}
	}

	if body := options.Get("body"); body != nil && !goja.IsUndefined(body) && !goja.IsNull(body) {
		spec.Body = bodyUpdateValue(vm, body)
	}

	return spec
}

// installSendRequest registers pm.sendRequest. The HTTP call happens
// immediately, but the callback is queued on the event loop so it runs after
// the calling script, as it does in Postman. Without a callback it returns a
// Promise, so `await pm.sendRequest(...)` works too.
func installSendRequest(vm *goja.Runtime, pmObj *goja.Object, ctx *ExecutionContext, loop *EventLoop) error {
	return pmObj.Set("sendRequest", func(call goja.FunctionCall) goja.Value {
		spec := sendRequestSpecFromJS(vm, call.Argument(0))

		var resp *ResponseData
		err := ErrSendRequestUnavailable
		if ctx.SendRequest != nil {
			resp, err = ctx.SendRequest(spec)
		}

		var errValue, respValue goja.Value = goja.Null(), goja.Null()
		if err != nil {
			errValue = vm.NewGoError(err)
		} else {
			respObj, buildErr := newResponseObject(vm, resp)
			if buildErr != nil {
				panic(vm.NewGoError(buildErr))
			}
			respValue = respObj
		}

		if callback, ok := goja.AssertFunction(call.Argument(1)); ok {
			loop.schedule(0, callback, errValue, respValue)
			return goja.Undefined()
		}

		promise, resolve, reject := vm.NewPromise()
		if err != nil {
			_ = reject(errValue)
		} else {
			_ = resolve(respValue)
		}
		return vm.ToValue(promise)
	})
}
