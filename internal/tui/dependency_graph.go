package tui

import (
	"errors"
	"fmt"
	"slices"

	tea "github.com/charmbracelet/bubbletea"
	"postOffice/internal/postman"
)

const dependencyGraphKey = "M"

// dependencyGraphRunKeys run the selected request with its prerequisites;
// dependencyGraphJumpKeys open it in the requests view, opposite to h going back.
var (
	dependencyGraphRunKeys  = []string{"enter", "r", runWithPrerequisitesKey, runKey}
	dependencyGraphJumpKeys = []string{"l", "right"}
)

var errNoDependencyGraphRequest = errors.New("no request selected")

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
	m.dependencyGraph.cursor = m.dependencyGraphTargetCursor()
	m.statusMessage = "Dependency graph of " + item.Name
	return m, nil
}

// dependencyGraphTargetCursor is the cursor on the first row naming the target,
// so running straight away runs the whole chain.
func (m Model) dependencyGraphTargetCursor() int {
	scope, err := m.dependencyScopeAt(m.dependencyGraph.viewFolderPath)
	if err != nil {
		return 0
	}
	targetPath := scope.requestPath(m.dependencyGraph.viewFolderPath, m.dependencyGraph.target.Name)
	rows := m.dependencyGraphRows()
	for cursor, i := range selectableRowIndexes(rows) {
		if rows[i].requestPath == targetPath {
			return cursor
		}
	}
	return 0
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

// runDependencyGraphRequest runs the request under the graph cursor after its stale prerequisites.
func (m Model) runDependencyGraphRequest() (Model, tea.Cmd) {
	selected, err := m.selectedDependencyGraphRequest()
	if err != nil {
		m.statusMessage = "Cannot run: " + err.Error()
		return m, nil
	}
	return m.runWithPrerequisites(selected.item, selected.viewFolderPath)
}

// jumpToDependencyGraphRequest opens the requests view on the request under the graph cursor.
func (m Model) jumpToDependencyGraphRequest() (Model, tea.Cmd) {
	selected, err := m.selectedDependencyGraphRequest()
	if err != nil {
		m.statusMessage = "Cannot jump: " + err.Error()
		return m, nil
	}

	m = m.openRequestsFolder(selected.viewFolderPath)
	if i := slices.IndexFunc(m.currentItems, func(candidate postman.Item) bool { return candidate.Name == selected.item.Name }); i >= 0 {
		m.cursor = i
	}
	m.statusMessage = "Jumped to " + selected.path
	return m, nil
}

type dependencyGraphSelection struct {
	path           string
	item           postman.Item
	viewFolderPath []string
}

func (m Model) selectedDependencyGraphRequest() (dependencyGraphSelection, error) {
	rows := m.dependencyGraphRows()
	selectable := selectableRowIndexes(rows)
	if m.dependencyGraph.cursor >= len(selectable) {
		return dependencyGraphSelection{}, errNoDependencyGraphRequest
	}
	requestPath := rows[selectable[m.dependencyGraph.cursor]].requestPath

	scope, err := m.dependencyScopeAt(m.dependencyGraph.viewFolderPath)
	if err != nil {
		return dependencyGraphSelection{}, err
	}
	item, folderPath, err := resolveItemPath(scope.collection, requestPath)
	if err != nil {
		return dependencyGraphSelection{}, fmt.Errorf("%q: %w", requestPath, err)
	}
	return dependencyGraphSelection{path: requestPath, item: *item, viewFolderPath: scope.viewFolderPath(folderPath)}, nil
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
