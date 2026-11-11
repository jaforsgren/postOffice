package dependency

import "time"

// Graph declares request dependencies for one collection, keyed by request path
// in the same "Folder/Request" format workflow steps use.
type Graph struct {
	Requests map[string]Node `yaml:"requests"`
}

// Node lists the requests a request requires and the variables it produces.
type Node struct {
	Requires []Requirement `yaml:"requires"`
	Produces []string      `yaml:"produces"`
}

// Requirement is one requires entry: a prerequisite request and the rules that
// decide when it must run again. In YAML it is either the bare request path or
// a mapping with the request key and optional rules.
type Requirement struct {
	Request string
	// MaxAge marks the prerequisite stale once its last successful run is older; zero means no limit.
	MaxAge time.Duration
	// RefreshOn lists response statuses of the requiring request that re-run the
	// prerequisite and retry the requiring request once.
	RefreshOn []int
	// WhenMissing replaces the session freshness rules: the prerequisite runs only
	// while one of these variables is unset or empty, even if it never ran this session.
	WhenMissing []string
}

// SessionState answers the questions the staleness rules need about the running session.
type SessionState interface {
	// LastSuccess returns when the request last completed, if that run was successful this session.
	LastSuccess(requestPath string) (time.Time, bool)
	// LastFailure says why the request's last completed run this session was not
	// successful, or returns "" when it succeeded or has not completed.
	LastFailure(requestPath string) string
	// VariableSet reports whether the variable currently resolves to a non-empty value.
	VariableSet(name string) bool
	// ChangedInputs returns the variables the request used in its last successful
	// run whose values have changed since.
	ChangedInputs(requestPath string) []string
	Now() time.Time
}

// StaleReason says why a prerequisite has to run again.
type StaleReason int

const (
	Fresh StaleReason = iota
	NeverRan
	Failed
	Expired
	ProducedUnset
	InputsChanged
	VariablesMissing
)

// Staleness is the verdict on one requirement. Variables lists the unset,
// changed or missing variables; Age and MaxAge are set for Expired and Failure for Failed.
type Staleness struct {
	Reason    StaleReason
	Variables []string
	Age       time.Duration
	MaxAge    time.Duration
	Failure   string
}

// PlanStep is one request in a target's execution order.
type PlanStep struct {
	Request string
	// Staleness is the first stale verdict among the requirements naming this
	// request, or fresh when all of them are fresh. It is fresh for the target.
	Staleness Staleness
	// Runs reports whether running the target with its prerequisites sends this
	// request: the target always, a stale prerequisite unless only fresh requests need it.
	Runs bool
}
