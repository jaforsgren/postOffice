package dependency

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Staleness decides whether requirement's prerequisite must run again.
// With WhenMissing it only checks those variables. Otherwise the prerequisite is
// stale when it has not run successfully this session, its last success is older
// than MaxAge, a variable it produces is unset, or a variable it used (other than
// the ones it produces) has changed since.
func (g *Graph) Staleness(requirement Requirement, state SessionState) Staleness {
	if len(requirement.WhenMissing) > 0 {
		if missing := unsetVariables(requirement.WhenMissing, state); len(missing) > 0 {
			return Staleness{Reason: VariablesMissing, Variables: missing}
		}
		return Staleness{}
	}

	lastSuccess, ran := state.LastSuccess(requirement.Request)
	if !ran {
		if failure := state.LastFailure(requirement.Request); failure != "" {
			return Staleness{Reason: Failed, Failure: failure}
		}
		return Staleness{Reason: NeverRan}
	}
	if age := state.Now().Sub(lastSuccess); requirement.MaxAge > 0 && age > requirement.MaxAge {
		return Staleness{Reason: Expired, Age: age, MaxAge: requirement.MaxAge}
	}
	if unset := g.UnsetProducedVariables(requirement.Request, state); len(unset) > 0 {
		return Staleness{Reason: ProducedUnset, Variables: unset}
	}
	if changed := g.changedInputs(requirement.Request, state); len(changed) > 0 {
		return Staleness{Reason: InputsChanged, Variables: changed}
	}
	return Staleness{}
}

// changedInputs leaves out the request's produced variables: a request that sends
// the token it refreshes would otherwise go stale whenever another request sets that token.
func (g *Graph) changedInputs(requestPath string, state SessionState) []string {
	produces := g.Node(requestPath).Produces
	var changed []string
	for _, name := range state.ChangedInputs(requestPath) {
		if !slices.Contains(produces, name) {
			changed = append(changed, name)
		}
	}
	return changed
}

// UnsetProducedVariables returns the variables a request produces that are unset or empty.
func (g *Graph) UnsetProducedVariables(requestPath string, state SessionState) []string {
	return unsetVariables(g.Node(requestPath).Produces, state)
}

func unsetVariables(names []string, state SessionState) []string {
	var unset []string
	for _, name := range names {
		if !state.VariableSet(name) {
			unset = append(unset, name)
		}
	}
	return unset
}

func (s Staleness) IsStale() bool {
	return s.Reason != Fresh
}

// String explains the verdict for the info view, e.g. "ran 1h2m ago, max_age 1h".
func (s Staleness) String() string {
	variables := strings.Join(s.Variables, ", ")
	switch s.Reason {
	case NeverRan:
		return "not run this session"
	case Failed:
		return "last run failed: " + s.Failure
	case Expired:
		return fmt.Sprintf("ran %s ago, %s %s", formatDuration(s.Age.Round(time.Second)), maxAgeKey, formatDuration(s.MaxAge))
	case ProducedUnset:
		return variables + " unset"
	case InputsChanged:
		return "inputs changed: " + variables
	case VariablesMissing:
		return variables + " missing"
	}
	return "fresh"
}
