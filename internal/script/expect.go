package script

import (
	_ "embed"
	"fmt"

	"github.com/dop251/goja"
)

//go:embed expect.js
var expectSource string

// newExpect evaluates the embedded Chai-compatible library and returns its
// expect function, used as pm.expect.
func newExpect(vm *goja.Runtime) (goja.Value, error) {
	expect, err := vm.RunScript("expect.js", expectSource)
	if err != nil {
		return nil, fmt.Errorf("failed to load pm.expect: %w", err)
	}
	return expect, nil
}
