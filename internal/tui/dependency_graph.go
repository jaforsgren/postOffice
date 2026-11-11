package tui

import (
	"fmt"
	"slices"

	tea "github.com/charmbracelet/bubbletea"
	"postOffice/internal/postman"
)

const dependencyGraphKey = "M"

// dependencyGraphView is the request whose dependency graph is shown and the
// graph view's cursor, an index into the rows naming a request.
type dependencyGraphView struct {
	target         postman.Item
	viewFolderPath []string
	cursor         int
	returnMode     ViewMode
}

func handleDependencyGraphKey(m Model) (Model, tea.Cmd) {
	item, ok := m.selectedItem()
	if !ok || !item.IsRequest() {
		m.statusMessage = "Select a request to show its dependency graph"
		return m, nil
	}
	return m.openDependencyGraph(item, m.folderPathForCursor())
}

func handleDependencyGraphCommand(m Model, _ []string) (Model, tea.Cmd) {
	return handleDependencyGraphKey(m)
}

func (m Model) openDependencyGraph(item postman.Item, viewFolderPath []string) (Model, tea.Cmd) {
	if _, err := m.dependencyScopeAt(viewFolderPath); err != nil {
		m.statusMessage = "Cannot show dependencies: " + err.Error()
		return m, nil
	}
	m.dependencyGraph = dependencyGraphView{
		target:         item,
		viewFolderPath: slices.Clone(viewFolderPath),
		returnMode:     m.mode,
	}
	m.mode = ModeDependencyGraph
	m.statusMessage = "Dependency graph of " + item.Name
	return m, nil
}

func (m Model) closeDependencyGraph() (Model, tea.Cmd) {
	m.mode = m.dependencyGraph.returnMode
	m.statusMessage = "Closed dependency graph"
	return m, nil
}

func (m Model) moveDependencyGraphCursor(delta int) Model {
	count := len(selectableRowIndexes(m.dependencyGraphRows()))
	m.dependencyGraph.cursor = max(0, min(m.dependencyGraph.cursor+delta, count-1))
	return m
}

func handleDependencyGraphRunKey(m Model) (Model, tea.Cmd) {
	return m.runWithPrerequisites(m.dependencyGraph.target, m.dependencyGraph.viewFolderPath)
}

// jumpToDependencyGraphRequest opens the requests view on the request under the graph cursor.
func (m Model) jumpToDependencyGraphRequest() (Model, tea.Cmd) {
	rows := m.dependencyGraphRows()
	selectable := selectableRowIndexes(rows)
	if m.dependencyGraph.cursor >= len(selectable) {
		m.statusMessage = "No request selected"
		return m, nil
	}
	requestPath := rows[selectable[m.dependencyGraph.cursor]].requestPath

	scope, err := m.dependencyScopeAt(m.dependencyGraph.viewFolderPath)
	if err != nil {
		m.statusMessage = "Cannot jump: " + err.Error()
		return m, nil
	}
	item, folderPath, err := resolveItemPath(scope.collection, requestPath)
	if err != nil {
		m.statusMessage = fmt.Sprintf("Cannot jump to %q: %v", requestPath, err)
		return m, nil
	}

	m = m.openRequestsFolder(scope.viewFolderPath(folderPath))
	if i := slices.IndexFunc(m.currentItems, func(candidate postman.Item) bool { return candidate.Name == item.Name }); i >= 0 {
		m.cursor = i
	}
	m.statusMessage = "Jumped to " + requestPath
	return m, nil
}

// selectableRowIndexes returns the indexes of the rows the cursor can stop on.
func selectableRowIndexes(rows []dependencyGraphRow) []int {
	var indexes []int
	for i, row := range rows {
		if row.requestPath != "" {
			indexes = append(indexes, i)
		}
	}
	return indexes
}
