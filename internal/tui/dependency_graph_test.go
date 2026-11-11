package tui

import (
	nethttp "net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"postOffice/internal/postman"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const (
	depProfilePath  = "Users/Profile"
	depProfileRoute = "/profile"
	depListPath     = "Users/List"
	depListRoute    = "/list"
	// depDiamondGraph: Get User needs Profile and Login, Profile needs Login too, and Login needs List.
	depDiamondGraph = `requests:
  Users/Get User:
    requires:
      - Users/Profile
      - request: Auth/Login
        max_age: 1h
  Users/Profile:
    requires: [Auth/Login]
  Auth/Login:
    requires: [Users/List]
    produces: [token]
`
)

// newDependencyGraphTestModel opens the graph view of Users/Get User in a
// collection that also has Users/Profile and Users/List.
func newDependencyGraphTestModel(t *testing.T, server *recordingServer, graphYAML string) Model {
	t.Helper()
	collection := dependencyTestCollection(server.URL, setTokenEvents())
	users := &collection.Items[1]
	for name, route := range map[string]string{"Profile": depProfileRoute, "List": depListRoute} {
		users.Items = append(users.Items, postman.Item{
			Name:    name,
			Request: &postman.Request{Method: "GET", URL: postman.URL{Raw: server.URL + route}},
		})
	}

	m, _ := newDependencyTestModel(t, collection, graphYAML)
	m, _ = pressKey(m, runes(dependencyGraphKey))
	if m.mode != ModeDependencyGraph {
		t.Fatalf("expected the dependency graph view, status: %s", m.statusMessage)
	}
	return m
}

func dependencyGraphText(m Model) string {
	rows := m.dependencyGraphRows()
	lines := make([]string, len(rows))
	for i, row := range rows {
		lines[i] = ansi.Strip(row.text)
	}
	return strings.Join(lines, "\n")
}

func assertGraphContains(t *testing.T, m Model, fragments ...string) {
	t.Helper()
	text := dependencyGraphText(m)
	for _, fragment := range fragments {
		if !strings.Contains(text, fragment) {
			t.Errorf("graph missing %q:\n%s", fragment, text)
		}
	}
}

func selectableGraphPaths(m Model) []string {
	rows := m.dependencyGraphRows()
	var paths []string
	for _, i := range selectableRowIndexes(rows) {
		paths = append(paths, rows[i].requestPath)
	}
	return paths
}

func TestDependencyGraph_OpensFromRequestsAndClosesBack(t *testing.T) {
	m := newDependencyGraphTestModel(t, newRecordingServer(t, nil), depDiamondGraph)

	m, _ = pressKey(m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.mode != ModeRequests {
		t.Errorf("expected esc to return to the requests view, got mode %v", m.mode)
	}
}

func TestDependencyGraph_CommandOpensGraph(t *testing.T) {
	server := newRecordingServer(t, nil)
	m, _ := newDependencyTestModel(t, dependencyTestCollection(server.URL, nil), depGraphWithLogin)

	m, _ = handleDependencyGraphCommand(m, nil)

	if m.mode != ModeDependencyGraph || m.dependencyGraph.target.Name != "Get User" {
		t.Errorf("expected the graph of Get User, got mode %v target %q", m.mode, m.dependencyGraph.target.Name)
	}
}

func TestDependencyGraph_ShowsExecutionOrderAndTree(t *testing.T) {
	m := newDependencyGraphTestModel(t, newRecordingServer(t, nil), depDiamondGraph)

	assertGraphContains(t, m,
		dependencyGraphOrderTitle,
		" 1. "+depListPath+" (stale: not run this session)",
		" 2. "+depLoginPath+" (stale: not run this session)",
		" 3. "+depProfilePath+" (stale: not run this session)",
		" 4. "+depGetUserPath+" "+planTargetStatus,
		dependencyGraphTreeTitle,
		dependencyTreeBranch+depProfilePath+" (stale: not run this session)",
		dependencyTreeContinuation+dependencyTreeLastBranch+depLoginPath,
		dependencyTreeContinuation+dependencyTreeLastContinuation+dependencyTreeLastBranch+depListPath,
		dependencyTreeLastBranch+depLoginPath+" [max_age 1h] (stale: not run this session) "+dependencyShownAboveMarker,
	)
}

func TestDependencyGraph_MarksFreshAndSkippedAfterRun(t *testing.T) {
	server := newRecordingServer(t, nil)
	m := newDependencyGraphTestModel(t, server, depGraphWithLogin)

	m, cmd := pressKey(m, runes(runWithPrerequisitesKey))
	m = drainCommands(t, m, cmd)

	if got, want := server.recordedHits(), []string{depLoginRoute, depUserRoute}; !reflect.DeepEqual(got, want) {
		t.Errorf("hits = %v, want %v", got, want)
	}
	if m.mode != ModeDependencyGraph {
		t.Errorf("expected to stay in the graph while it runs, got mode %v", m.mode)
	}
	assertGraphContains(t, m,
		skippedStepMarker+". "+depLoginPath+" ("+prerequisiteFresh+")",
		" 1. "+depGetUserPath,
		dependencyGraphProducesTitle,
	)
}

func TestDependencyGraph_MarksFailedPrerequisite(t *testing.T) {
	server := newRecordingServer(t, map[string]int{depLoginRoute: nethttp.StatusInternalServerError})
	m := newDependencyGraphTestModel(t, server, depGraphWithLogin)

	m, cmd := pressKey(m, runes(runWithPrerequisitesKey))
	m = drainCommands(t, m, cmd)

	assertGraphContains(t, m, depLoginPath+" (stale: last run failed: status 500)")
}

func TestDependencyGraph_ReportsCycleBeforeExecution(t *testing.T) {
	m := newDependencyGraphTestModel(t, newRecordingServer(t, nil), `requests:
  Users/Get User:
    requires: [Auth/Login]
  Auth/Login:
    requires: [Users/Get User]
`)

	assertGraphContains(t, m,
		dependencyGraphProblemsTitle,
		"dependency cycle: Auth/Login → Users/Get User → Auth/Login",
		dependencyTreeLastContinuation+dependencyTreeLastBranch+depGetUserPath,
		dependencyCycleMarker,
	)
	if strings.Contains(dependencyGraphText(m), dependencyGraphOrderTitle) {
		t.Error("expected no execution order for a cyclic graph")
	}
}

func TestDependencyGraph_MissingPrerequisiteIsNotSelectable(t *testing.T) {
	m := newDependencyGraphTestModel(t, newRecordingServer(t, nil), "requests:\n  Users/Get User:\n    requires: [Auth/Logout]\n")

	assertGraphContains(t, m, "Auth/Logout ("+prerequisiteMissing+")")
	if slices.Contains(selectableGraphPaths(m), "Auth/Logout") {
		t.Error("expected a missing request not to be selectable")
	}
}

func TestDependencyGraph_EnterJumpsToSelectedRequest(t *testing.T) {
	m := newDependencyGraphTestModel(t, newRecordingServer(t, nil), depDiamondGraph)
	m, _ = pressKey(m, runes("j"))
	if got := selectableGraphPaths(m)[m.dependencyGraph.cursor]; got != depLoginPath {
		t.Fatalf("expected the second selectable row to be %s, got %s", depLoginPath, got)
	}

	m, _ = pressKey(m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.mode != ModeRequests || !reflect.DeepEqual(m.breadcrumb, []string{"Auth"}) {
		t.Fatalf("expected the requests view in Auth, got mode %v breadcrumb %v", m.mode, m.breadcrumb)
	}
	if item, ok := m.selectedItem(); !ok || item.Name != "Login" {
		t.Errorf("expected the cursor on Login, got %+v", item)
	}
}

func TestDependencyGraph_CursorStaysWithinSelectableRows(t *testing.T) {
	m := newDependencyGraphTestModel(t, newRecordingServer(t, nil), depDiamondGraph)
	count := len(selectableGraphPaths(m))

	m, _ = pressKey(m, runes("k"))
	if m.dependencyGraph.cursor != 0 {
		t.Errorf("expected the cursor to stop at the first row, got %d", m.dependencyGraph.cursor)
	}
	for range count + 2 {
		m, _ = pressKey(m, runes("j"))
	}
	if m.dependencyGraph.cursor != count-1 {
		t.Errorf("expected the cursor to stop at the last row %d, got %d", count-1, m.dependencyGraph.cursor)
	}
}

func TestDependencyGraph_RendersSelectedRow(t *testing.T) {
	m := newDependencyGraphTestModel(t, newRecordingServer(t, nil), depDiamondGraph)
	m.width, m.height = splitTestWidth, splitTestHeight

	view := ansi.Strip(m.renderDependencyGraphView())

	if !strings.Contains(view, "Dependency Graph: Get User") || !strings.Contains(view, "▶    1. "+depListPath) {
		t.Errorf("expected the title and the cursor on the first step, got:\n%s", view)
	}
}
