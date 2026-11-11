package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"postOffice/internal/dependency"
)

const (
	// dependencyGraphHeaderLines is the title line plus the blank line below it.
	dependencyGraphHeaderLines = 2

	dependencyTreeContinuation     = "│   "
	dependencyTreeLastContinuation = "    "
	dependencyCycleMarker          = "↻ cycle"
	dependencyShownAboveMarker     = "(prerequisites shown above)"
	skippedStepMarker              = " –"
	planTargetStatus               = "target"

	dependencyGraphProblemsTitle = "Problems (" + runWithPrerequisitesKey + " is blocked until they are fixed):"
	dependencyGraphOrderTitle    = "Execution order (" + runWithPrerequisitesKey + " sends the numbered requests, – are skipped):"
	dependencyGraphTreeTitle     = "Requires:"
	dependencyGraphProducesTitle = "Produces:"

	dependencyGraphHelp = "(enter/r/" + runWithPrerequisitesKey + ": run with prerequisites | l: jump to request | esc: back)"
)

// dependencyGraphRow is one line of the graph view; requestPath is set on lines
// naming a request of the collection, which the cursor can select to jump to.
type dependencyGraphRow struct {
	text        string
	requestPath string
}

func (m Model) renderDependencyGraphView() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)

	title := titleStyle.Render("Dependency Graph: " + m.dependencyGraph.target.Name)
	help := helpStyle.Render(dependencyGraphHelp)
	lines := []string{title + " " + help, ""}

	rows := m.dependencyGraphRows()
	selectable := selectableRowIndexes(rows)
	cursorRow := 0
	if m.dependencyGraph.cursor < len(selectable) {
		cursorRow = selectable[m.dependencyGraph.cursor]
	}

	visibleLines := m.height - windowChromeHeight - dependencyGraphHeaderLines
	startIdx, endIdx := calculateVisibleWindow(cursorRow, len(rows), visibleLines)
	for i := startIdx; i < endIdx; i++ {
		marker := "  "
		if i == cursorRow && len(selectable) > 0 {
			marker = selectedStyle.Render("▶ ")
		}
		lines = append(lines, marker+rows[i].text)
	}

	return mainWindowStyle.
		Height(m.height - windowChromeHeight).
		Width(m.width - 4).
		Render(strings.Join(lines, "\n"))
}

// dependencyGraphRows lays out the graph of the target request: validation
// problems, the execution order a run with prerequisites follows, the requires
// tree with each edge's rules and status, and the variables the target produces.
func (m Model) dependencyGraphRows() []dependencyGraphRow {
	view := m.dependencyGraph
	scope, err := m.dependencyScopeAt(view.viewFolderPath)
	if err != nil {
		return []dependencyGraphRow{{text: dependencyErrorStyle.Render(err.Error())}}
	}
	graph, err := scope.loadGraph()
	if err != nil {
		return []dependencyGraphRow{{text: dependencyErrorStyle.Render(err.Error())}}
	}

	targetPath := scope.requestPath(view.viewFolderPath, view.target.Name)
	state := m.dependencyState(scope)
	var rows []dependencyGraphRow

	if err := graph.Validate(scope.requestExists); err != nil {
		rows = append(rows, dependencyGraphRow{text: dependencyErrorStyle.Render(dependencyGraphProblemsTitle)})
		for _, line := range strings.Split(err.Error(), "\n") {
			rows = append(rows, dependencyGraphRow{text: "  " + dependencyErrorStyle.Render(line)})
		}
		rows = append(rows, dependencyGraphRow{})
	}

	if plan, err := graph.Plan(targetPath, state); err == nil {
		rows = append(rows, dependencyGraphRow{text: requestStyle.Render(dependencyGraphOrderTitle)})
		rows = append(rows, planRows(scope, plan, targetPath)...)
		rows = append(rows, dependencyGraphRow{})
	}

	rows = append(rows, dependencyGraphRow{text: requestStyle.Render(dependencyGraphTreeTitle)})
	rows = append(rows, dependencyGraphRow{text: "  " + targetPath, requestPath: targetPath})
	tree := dependencyTreeBuilder{scope: scope, graph: graph, state: state, expanded: map[string]bool{}}
	tree.addRequirements(targetPath, "  ", map[string]bool{targetPath: true})
	rows = append(rows, tree.rows...)

	if produces := graph.Node(targetPath).Produces; len(produces) > 0 {
		rows = append(rows, dependencyGraphRow{}, dependencyGraphRow{text: requestStyle.Render(dependencyGraphProducesTitle)})
		for _, name := range produces {
			rows = append(rows, dependencyGraphRow{text: "  " + name + " " + producedVariableStatus(name, state)})
		}
	}
	return rows
}

func planRows(scope dependencyScope, plan []dependency.PlanStep, targetPath string) []dependencyGraphRow {
	rows := make([]dependencyGraphRow, 0, len(plan))
	stepNumber := 0
	for _, step := range plan {
		number := skippedStepMarker
		if step.Runs {
			stepNumber++
			number = fmt.Sprintf("%2d", stepNumber)
		}
		row := dependencyGraphRow{text: "  " + number + ". " + step.Request + " " + planStepStatus(scope, step, targetPath)}
		if scope.requestExists(step.Request) {
			row.requestPath = step.Request
		}
		rows = append(rows, row)
	}
	return rows
}

func planStepStatus(scope dependencyScope, step dependency.PlanStep, targetPath string) string {
	switch {
	case step.Request == targetPath:
		return subtleStyle.Render(planTargetStatus)
	case !scope.requestExists(step.Request):
		return dependencyStatusTag(dependencyErrorStyle, prerequisiteMissing)
	}
	return stalenessTag(step.Staleness)
}

// dependencyTreeBuilder renders the requires edges below a request. A request
// already expanded elsewhere is not expanded again, and an edge back to a request
// on the current path is marked as a cycle instead of being followed.
type dependencyTreeBuilder struct {
	scope    dependencyScope
	graph    *dependency.Graph
	state    dependency.SessionState
	expanded map[string]bool
	rows     []dependencyGraphRow
}

func (b *dependencyTreeBuilder) addRequirements(requestPath, prefix string, onPath map[string]bool) {
	b.expanded[requestPath] = true
	requirements := b.graph.Node(requestPath).Requires
	for i, required := range requirements {
		branch, continuation := dependencyTreeBranch, dependencyTreeContinuation
		if i == len(requirements)-1 {
			branch, continuation = dependencyTreeLastBranch, dependencyTreeLastContinuation
		}

		text := prefix + branch + required.Request
		if rules := required.Rules(); rules != "" {
			text += " " + subtleStyle.Render("["+rules+"]")
		}
		text += " " + prerequisiteStatus(b.scope, b.graph, required, b.state)

		exists := b.scope.requestExists(required.Request)
		hasRequirements := len(b.graph.Node(required.Request).Requires) > 0
		descend := exists && hasRequirements
		switch {
		case onPath[required.Request]:
			text += " " + dependencyErrorStyle.Render(dependencyCycleMarker)
			descend = false
		case descend && b.expanded[required.Request]:
			text += " " + subtleStyle.Render(dependencyShownAboveMarker)
			descend = false
		}

		row := dependencyGraphRow{text: text}
		if exists {
			row.requestPath = required.Request
		}
		b.rows = append(b.rows, row)

		if descend {
			onPath[required.Request] = true
			b.addRequirements(required.Request, prefix+continuation, onPath)
			delete(onPath, required.Request)
		}
	}
}
