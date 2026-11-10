package script

import (
	"slices"
	"sort"
	"strings"

	"github.com/dop251/goja"
	"postOffice/internal/postman"
)

// headersFromMap flattens an http.Header-style map into a stable, key-sorted
// list (one entry per value), so pm.response.headers iterates deterministically.
func headersFromMap(headerMap map[string][]string) []postman.Header {
	keys := make([]string, 0, len(headerMap))
	for key := range headerMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var headers []postman.Header
	for _, key := range keys {
		for _, value := range headerMap[key] {
			headers = append(headers, postman.Header{Key: key, Value: value})
		}
	}
	return headers
}

func findHeader(headers []postman.Header, name string) int {
	return slices.IndexFunc(headers, func(h postman.Header) bool { return strings.EqualFold(h.Key, name) })
}

// headerFromJS accepts Postman's {key, value} object form.
func headerFromJS(vm *goja.Runtime, val goja.Value) postman.Header {
	obj := val.ToObject(vm)
	return postman.Header{Key: obj.Get("key").String(), Value: obj.Get("value").String()}
}

// newHeaderList builds a Postman HeaderList over *headers. Lookups are
// case-insensitive, like Postman's. Mutating methods (add/upsert/remove) are
// only installed when mutable is true, so pm.response.headers stays read-only.
func newHeaderList(vm *goja.Runtime, headers *[]postman.Header, mutable bool) (*goja.Object, error) {
	list := vm.NewObject()

	methods := map[string]func(goja.FunctionCall) goja.Value{
		"get": func(call goja.FunctionCall) goja.Value {
			if i := findHeader(*headers, call.Argument(0).String()); i >= 0 {
				return vm.ToValue((*headers)[i].Value)
			}
			return goja.Undefined()
		},
		"has": func(call goja.FunctionCall) goja.Value {
			i := findHeader(*headers, call.Argument(0).String())
			if i < 0 {
				return vm.ToValue(false)
			}
			if len(call.Arguments) > 1 {
				return vm.ToValue((*headers)[i].Value == call.Argument(1).String())
			}
			return vm.ToValue(true)
		},
		"toObject": func(call goja.FunctionCall) goja.Value {
			obj := vm.NewObject()
			for _, h := range *headers {
				_ = obj.Set(h.Key, h.Value)
			}
			return obj
		},
		"all": func(call goja.FunctionCall) goja.Value {
			all := make([]any, len(*headers))
			for i, h := range *headers {
				all[i] = map[string]any{"key": h.Key, "value": h.Value}
			}
			return vm.ToValue(all)
		},
		"each": func(call goja.FunctionCall) goja.Value {
			fn, ok := goja.AssertFunction(call.Argument(0))
			if !ok {
				panic(vm.NewTypeError("headers.each requires a function"))
			}
			for _, h := range slices.Clone(*headers) {
				if _, err := fn(goja.Undefined(), vm.ToValue(map[string]any{"key": h.Key, "value": h.Value})); err != nil {
					panic(err)
				}
			}
			return goja.Undefined()
		},
	}

	if mutable {
		methods["add"] = func(call goja.FunctionCall) goja.Value {
			*headers = append(*headers, headerFromJS(vm, call.Argument(0)))
			return goja.Undefined()
		}
		methods["upsert"] = func(call goja.FunctionCall) goja.Value {
			header := headerFromJS(vm, call.Argument(0))
			if i := findHeader(*headers, header.Key); i >= 0 {
				(*headers)[i].Value = header.Value
			} else {
				*headers = append(*headers, header)
			}
			return goja.Undefined()
		}
		methods["remove"] = func(call goja.FunctionCall) goja.Value {
			name := call.Argument(0).String()
			*headers = slices.DeleteFunc(*headers, func(h postman.Header) bool { return strings.EqualFold(h.Key, name) })
			return goja.Undefined()
		}
	}

	if err := setMethods(list, "headers", methods); err != nil {
		return nil, err
	}
	return list, nil
}
