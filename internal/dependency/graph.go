package dependency

import (
	"errors"
	"sort"
)

// NewGraph returns an empty graph.
func NewGraph() *Graph {
	return &Graph{Requests: make(map[string]Node)}
}

// Node returns the declared dependencies of a request (zero value when none are declared).
func (g *Graph) Node(requestPath string) Node {
	return g.Requests[requestPath]
}

// IsStale reports whether a request must run again before its dependents:
// it has not run successfully this session, or one of its produced variables is unset or empty.
func (g *Graph) IsStale(requestPath string, state SessionState) bool {
	if !state.RanSuccessfully(requestPath) {
		return true
	}
	return len(g.UnsetProducedVariables(requestPath, state)) > 0
}

// UnsetProducedVariables returns the variables a request produces that are unset or empty.
func (g *Graph) UnsetProducedVariables(requestPath string, state SessionState) []string {
	var unset []string
	for _, name := range g.Node(requestPath).Produces {
		if !state.VariableSet(name) {
			unset = append(unset, name)
		}
	}
	return unset
}

// Validate checks that every key and requires entry names an existing request and
// that the graph is acyclic. All missing references and the first cycle found are joined.
func (g *Graph) Validate(requestExists func(requestPath string) bool) error {
	var errs []error
	paths := g.sortedPaths()

	for _, path := range paths {
		if !requestExists(path) {
			errs = append(errs, &UnknownRequestError{Request: path})
		}
		for _, required := range g.Requests[path].Requires {
			if !requestExists(required) {
				errs = append(errs, &MissingPrerequisiteError{Request: required, RequiredBy: path})
			}
		}
	}

	w := newWalker(g, func(string) bool { return true })
	for _, path := range paths {
		if err := w.visit(path); err != nil {
			errs = append(errs, err)
			break
		}
	}

	return errors.Join(errs...)
}

// StalePrerequisites returns the stale transitive prerequisites of target in execution
// order (every request after the ones it requires), excluding target itself.
// A fresh prerequisite is not descended into: its outputs are already available.
func (g *Graph) StalePrerequisites(target string, state SessionState) ([]string, error) {
	w := newWalker(g, func(path string) bool { return g.IsStale(path, state) })
	if err := w.visit(target); err != nil {
		return nil, err
	}
	return w.order[:len(w.order)-1], nil
}

func (g *Graph) sortedPaths() []string {
	paths := make([]string, 0, len(g.Requests))
	for path := range g.Requests {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

// walker performs a depth-first post-order traversal over requires edges,
// following only the prerequisites for which descend returns true.
type walker struct {
	graph    *Graph
	descend  func(requestPath string) bool
	visiting map[string]bool
	visited  map[string]bool
	stack    []string
	order    []string
}

func newWalker(g *Graph, descend func(requestPath string) bool) *walker {
	return &walker{
		graph:    g,
		descend:  descend,
		visiting: make(map[string]bool),
		visited:  make(map[string]bool),
	}
}

func (w *walker) visit(path string) error {
	if w.visited[path] {
		return nil
	}
	if w.visiting[path] {
		return w.cycleEndingAt(path)
	}

	w.visiting[path] = true
	w.stack = append(w.stack, path)
	for _, required := range w.graph.Requests[path].Requires {
		if !w.descend(required) {
			continue
		}
		if err := w.visit(required); err != nil {
			return err
		}
	}
	w.stack = w.stack[:len(w.stack)-1]
	delete(w.visiting, path)

	w.visited[path] = true
	w.order = append(w.order, path)
	return nil
}

func (w *walker) cycleEndingAt(path string) error {
	start := 0
	for i, p := range w.stack {
		if p == path {
			start = i
			break
		}
	}
	cycle := append(append([]string{}, w.stack[start:]...), path)
	return &CycleError{Path: cycle}
}
