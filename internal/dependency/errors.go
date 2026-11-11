package dependency

import (
	"fmt"
	"strings"
)

const cycleArrow = " → "

// CycleError reports a dependency cycle; Path starts and ends with the same request.
type CycleError struct {
	Path []string
}

func (e *CycleError) Error() string {
	return "dependency cycle: " + strings.Join(e.Path, cycleArrow)
}

// MissingPrerequisiteError reports a requires entry that names no request in the collection.
type MissingPrerequisiteError struct {
	Request    string
	RequiredBy string
}

func (e *MissingPrerequisiteError) Error() string {
	return fmt.Sprintf("request %q required by %q not found in collection", e.Request, e.RequiredBy)
}

// UnknownRequestError reports a dependency entry whose key names no request in the collection.
type UnknownRequestError struct {
	Request string
}

func (e *UnknownRequestError) Error() string {
	return fmt.Sprintf("dependency entry %q does not match a request in the collection", e.Request)
}
