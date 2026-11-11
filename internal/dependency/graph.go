package dependency

import (
	"errors"
	"slices"
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
			if !requestExists(required.Request) {
				errs = append(errs, &MissingPrerequisiteError{Request: required.Request, RequiredBy: path})
			}
		}
	}

	w := newWalker(g, func(string, Requirement) bool { return true })
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
	w := newWalker(g, func(_ string, required Requirement) bool { return g.Staleness(required, state).IsStale() })
	if err := w.visit(target); err != nil {
		return nil, err
	}
	return w.order[:len(w.order)-1], nil
}

// RefreshPrerequisites returns what to run again before retrying requestPath after it
// answered with status: every direct requirement listing status in refresh_on, fresh or
// not, preceded by its own stale prerequisites, in execution order. It is empty when no
// requirement refreshes on status.
func (g *Graph) RefreshPrerequisites(requestPath string, status int, state SessionState) ([]string, error) {
	refreshed := make(map[string]bool)
	for _, required := range g.Node(requestPath).Requires {
		if slices.Contains(required.RefreshOn, status) {
			refreshed[required.Request] = true
		}
	}
	if len(refreshed) == 0 {
		return nil, nil
	}

	w := newWalker(g, func(parent string, required Requirement) bool {
		if parent == requestPath {
			return refreshed[required.Request]
		}
		return g.Staleness(required, state).IsStale()
	})
	if err := w.visit(requestPath); err != nil {
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
// following only the requirements of parent for which descend returns true.
type walker struct {
	graph    *Graph
	descend  func(parent string, required Requirement) bool
	visiting map[string]bool
	visited  map[string]bool
	stack    []string
	order    []string
}

func newWalker(g *Graph, descend func(parent string, required Requirement) bool) *walker {
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
		if !w.descend(path, required) {
			continue
		}
		if err := w.visit(required.Request); err != nil {
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
