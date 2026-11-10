package script

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/dop251/goja"
	"postOffice/internal/postman"
)

const (
	// singleIterationCount is what pm.info.iterationCount reports outside a
	// multi-iteration run.
	singleIterationCount = 1
	iterationDataSource  = "Iteration data"
)

// IterationData is one row of a data-driven run: a CSV row keyed by its
// header, or one object of a JSON array. JSON values keep their types.
type IterationData map[string]any

// Iteration identifies one pass of a run and the data row that drives it.
type Iteration struct {
	// Index is 0-based, as pm.info.iteration reports it.
	Index int
	// Count is the run's total number of iterations; 0 means a single pass.
	Count int
	// Data is this pass's data-file row; nil when the run has no data file.
	Data IterationData
}

// count normalises Count so a zero Iteration reports a single pass.
func (it Iteration) count() int {
	if it.Count < singleIterationCount {
		return singleIterationCount
	}
	return it.Count
}

// value renders a data value as {{variable}} text: strings as-is, anything
// else (numbers, booleans, nested JSON) in its JSON form.
func (d IterationData) value(key string) (string, bool) {
	raw, ok := d[key]
	if !ok {
		return "", false
	}
	if text, isString := raw.(string); isString {
		return text, true
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return "", false
	}
	return string(encoded), true
}

// newIterationDataObject builds pm.iterationData over the current data row.
// Values keep their JSON types; without a data file every lookup misses.
func newIterationDataObject(vm *goja.Runtime, data IterationData) (*goja.Object, error) {
	obj := vm.NewObject()
	err := setMethods(obj, "pm.iterationData", map[string]func(goja.FunctionCall) goja.Value{
		"get": func(call goja.FunctionCall) goja.Value {
			if value, ok := data[call.Argument(0).String()]; ok {
				return vm.ToValue(value)
			}
			return goja.Undefined()
		},
		"has": func(call goja.FunctionCall) goja.Value {
			_, ok := data[call.Argument(0).String()]
			return vm.ToValue(ok)
		},
		"toObject": func(goja.FunctionCall) goja.Value {
			row := vm.NewObject()
			for key, value := range data {
				if err := row.Set(key, value); err != nil {
					panic(vm.NewGoError(fmt.Errorf("pm.iterationData.toObject: %w", err)))
				}
			}
			return row
		},
	})
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// variables exposes the row as variable sources, key-sorted for stable output.
func (d IterationData) variables() []postman.VariableSource {
	keys := make([]string, 0, len(d))
	for key := range d {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	sources := make([]postman.VariableSource, 0, len(keys))
	for _, key := range keys {
		if value, ok := d.value(key); ok {
			sources = append(sources, postman.VariableSource{Key: key, Value: value, Source: iterationDataSource})
		}
	}
	return sources
}
