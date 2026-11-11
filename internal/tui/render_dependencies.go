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

	noDependenciesHint = "none (" + editDependenciesKey + " in the requests list to declare)"
)

var (
	dependencyOKStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	dependencyWarningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	dependencyErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

// buildDependenciesSection renders the info item's requires/produces as a small tree.
// The item sits at m.breadcrumb, as for the Info view's variable resolution.
// Collections without a known file render nothing, since there is nowhere to declare dependencies.
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
		lines = append(lines, "  "+subtleStyle.Render(noDependenciesHint))
	} else {
		lines = append(lines, "  "+m.currentInfoItem.Name)
		edges := m.dependencyEdges(scope, graph, node)
		for i, edge := range edges {
			branch := dependencyTreeBranch
			if i == len(edges)-1 {
				branch = dependencyTreeLastBranch
			}
			lines = append(lines, dependencyTreeIndent+branch+edge)
		}
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
		edge := dependencyRequiresLabel + required.Request
		if rules := required.Rules(); rules != "" {
			edge += " " + subtleStyle.Render("["+rules+"]")
		}
		edges = append(edges, edge+" "+prerequisiteStatus(scope, graph, required, state))
	}
	for _, name := range node.Produces {
		edges = append(edges, dependencyProducesLabel+name+" "+producedVariableStatus(name, state))
	}
	return edges
}

func producedVariableStatus(name string, state dependency.SessionState) string {
	if !state.VariableSet(name) {
		return dependencyStatusTag(dependencyWarningStyle, variableUnset)
	}
	return dependencyStatusTag(dependencyOKStyle, variableSet)
}

func prerequisiteStatus(scope dependencyScope, graph *dependency.Graph, required dependency.Requirement, state dependency.SessionState) string {
	if !scope.requestExists(required.Request) {
		return dependencyStatusTag(dependencyErrorStyle, prerequisiteMissing)
	}
	return stalenessTag(graph.Staleness(required, state))
}

// stalenessTag shows a failed last run as an error and any other stale verdict as a warning.
func stalenessTag(staleness dependency.Staleness) string {
	switch staleness.Reason {
	case dependency.Fresh:
		return dependencyStatusTag(dependencyOKStyle, prerequisiteFresh)
	case dependency.Failed:
		return dependencyStatusTag(dependencyErrorStyle, prerequisiteStale+": "+staleness.String())
	}
	return dependencyStatusTag(dependencyWarningStyle, prerequisiteStale+": "+staleness.String())
}

func dependencyStatusTag(style lipgloss.Style, status string) string {
	return style.Render("(" + status + ")")
}
