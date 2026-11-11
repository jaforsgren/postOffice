package tui

import (
	"postOffice/internal/dependency"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	dependencyTreeIndent     = "     "
	dependencyTreeBranch     = "├── "
	dependencyTreeLastBranch = "└── "
	dependencyRequiresLabel  = "requires → "
	dependencyProducesLabel  = "produces → "

	prerequisiteFresh   = "fresh"
	prerequisiteStale   = "stale"
	prerequisiteMissing = "missing"
	variableSet         = "set"
	variableUnset       = "unset"
)

var (
	dependencyOKStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	dependencyWarningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	dependencyErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

// buildDependenciesSection renders the info item's requires/produces as a small tree.
// The item sits at m.breadcrumb, as for the Info view's variable resolution.
// Requests without declared dependencies, and collections without a known file, render nothing.
func (m Model) buildDependenciesSection() []string {
	scope, err := m.dependencyScopeAt(m.breadcrumb)
	if err != nil {
		return nil
	}

	lines := []string{requestStyle.Render("Dependencies:")}
	graph, err := scope.loadGraph()
	if err != nil {
		return append(lines, "  "+dependencyErrorStyle.Render(err.Error()), "")
	}

	node := graph.Node(scope.requestPath(m.breadcrumb, m.currentInfoItem.Name))
	if len(node.Requires) == 0 && len(node.Produces) == 0 {
		return nil
	}

	lines = append(lines, "  "+m.currentInfoItem.Name)
	edges := m.dependencyEdges(scope, graph, node)
	for i, edge := range edges {
		branch := dependencyTreeBranch
		if i == len(edges)-1 {
			branch = dependencyTreeLastBranch
		}
		lines = append(lines, dependencyTreeIndent+branch+edge)
	}

	if err := graph.Validate(scope.requestExists); err != nil {
		for _, line := range strings.Split(err.Error(), "\n") {
			lines = append(lines, "  "+dependencyErrorStyle.Render(line))
		}
	}
	return append(lines, "")
}

func (m Model) dependencyEdges(scope dependencyScope, graph *dependency.Graph, node dependency.Node) []string {
	state := m.dependencyState(scope)
	edges := make([]string, 0, len(node.Requires)+len(node.Produces))

	for _, required := range node.Requires {
		edges = append(edges, dependencyRequiresLabel+required+" "+prerequisiteStatus(scope, graph, required, state))
	}
	for _, name := range node.Produces {
		status := dependencyStatusTag(dependencyOKStyle, variableSet)
		if !state.VariableSet(name) {
			status = dependencyStatusTag(dependencyWarningStyle, variableUnset)
		}
		edges = append(edges, dependencyProducesLabel+name+" "+status)
	}
	return edges
}

func prerequisiteStatus(scope dependencyScope, graph *dependency.Graph, requestPath string, state dependency.SessionState) string {
	switch {
	case !scope.requestExists(requestPath):
		return dependencyStatusTag(dependencyErrorStyle, prerequisiteMissing)
	case graph.IsStale(requestPath, state):
		return dependencyStatusTag(dependencyWarningStyle, prerequisiteStale)
	default:
		return dependencyStatusTag(dependencyOKStyle, prerequisiteFresh)
	}
}

func dependencyStatusTag(style lipgloss.Style, status string) string {
	return style.Render("(" + status + ")")
}
