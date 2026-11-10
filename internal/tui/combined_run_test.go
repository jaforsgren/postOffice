package tui

import (
	nethttp "net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"postOffice/internal/history"
	"postOffice/internal/postman"
)

const (
	combinedAScriptLog = "collection A pre-request"
	combinedSetVarName = "set_by_script"
)

// combinedRunModel shows collections A and B in the combined view. A has a
// collection variable and a collection pre-request script; B's request sends
// {{a_var}} to server and sets a collection variable from its test script.
func combinedRunModel(t *testing.T, serverURL string) (Model, string, string) {
	t.Helper()
	pathA := writeJSONFile(t, "a.json", postman.Collection{
		Info:      postman.Info{Name: combinedA},
		Variables: []postman.Variable{{Key: combinedVarName, Value: "one"}},
		Events: []postman.Event{{Listen: "prerequest", Script: postman.Script{
			Type: "text/javascript",
			Exec: []string{"console.log('" + combinedAScriptLog + "')"},
		}}},
		Items: []postman.Item{{Name: usersRequest, Request: &postman.Request{Method: "GET", URL: postman.URL{Raw: serverURL + "/a"}}}},
	})
	pathB := writeJSONFile(t, "b.json", postman.Collection{
		Info: postman.Info{Name: combinedB},
		Items: []postman.Item{{Name: ordersFolder, Items: []postman.Item{{
			Name:    ordersRequest,
			Request: &postman.Request{Method: "GET", URL: postman.URL{Raw: serverURL + "/{{" + combinedVarName + "}}"}},
			Events: []postman.Event{{Listen: "test", Script: postman.Script{
				Type: "text/javascript",
				Exec: []string{"pm.collectionVariables.set('" + combinedSetVarName + "', 'saved')"},
			}}},
		}}}},
	})
	parser := postman.NewParser()
	for _, path := range []string{pathA, pathB} {
		if _, err := parser.LoadCollection(path); err != nil {
			t.Fatal(err)
		}
	}
	parser.ToggleActiveCollection(combinedA)
	parser.ToggleActiveCollection(combinedB)

	m := NewModel(parser, history.NewStore(""))
	m.mode = ModeCollections
	m = m.loadCollectionsList()
	m.cursor = 0
	m = m.handleSelection()
	if !m.isViewingMergedCollection() {
		t.Fatal("expected the combined view")
	}
	return m, pathA, pathB
}

func requestPathServer(t *testing.T) (*httptest.Server, *string) {
	t.Helper()
	var gotPath string
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(nethttp.StatusOK)
	}))
	t.Cleanup(server.Close)
	return server, &gotPath
}

// runCombinedOrders executes Collection B's request from the combined view.
func runCombinedOrders(t *testing.T, m Model) (Model, RequestCompleteMsg) {
	t.Helper()
	m = cursorOn(t, openFolder(t, openFolder(t, m, combinedB), ordersFolder), ordersRequest)
	m, cmd := m.executeRequest(m.currentItems[m.cursor], m.folderPathForCursor())
	if cmd == nil {
		t.Fatal("expected a request command")
	}
	msg, ok := cmd().(RequestCompleteMsg)
	if !ok {
		t.Fatal("expected a RequestCompleteMsg")
	}
	return m, msg
}

func TestCombinedView_RequestDoesNotResolveAnotherCollectionsVariables(t *testing.T) {
	server, gotPath := requestPathServer(t)
	m, _, _ := combinedRunModel(t, server.URL)

	_, _ = runCombinedOrders(t, m)

	if want := "/{{" + combinedVarName + "}}"; *gotPath != want {
		t.Errorf("expected B's request to leave A's variable unresolved (%s), got %s", want, *gotPath)
	}
}

func TestCombinedView_RequestDoesNotRunAnotherCollectionsScripts(t *testing.T) {
	server, _ := requestPathServer(t)
	m, _, _ := combinedRunModel(t, server.URL)

	_, msg := runCombinedOrders(t, m)

	if msg.TestResult != nil && slices.ContainsFunc(msg.TestResult.ConsoleLogs, func(line string) bool {
		return strings.Contains(line, combinedAScriptLog)
	}) {
		t.Errorf("expected A's collection script not to run for B's request, got logs %v", msg.TestResult.ConsoleLogs)
	}
}

func TestCombinedView_RequestKeepsItsOwnCollectionsVariables(t *testing.T) {
	m, _, _ := combinedRunModel(t, "http://unused")
	m = openFolder(t, m, combinedA)

	vars := m.variablesAt(m.breadcrumb)

	if !slices.ContainsFunc(vars, func(v postman.VariableSource) bool { return v.Key == combinedVarName }) {
		t.Errorf("expected A's own variable at %v, got %+v", m.breadcrumb, vars)
	}
}

func TestCombinedView_ScriptSetCollectionVariablePersistsToTheOwningCollection(t *testing.T) {
	server, _ := requestPathServer(t)
	m, pathA, pathB := combinedRunModel(t, server.URL)
	m, msg := runCombinedOrders(t, m)

	updated, _ := m.Update(msg)
	m = updated.(Model)

	if strings.Contains(m.statusMessage, "failed to save") {
		t.Fatalf("expected the variables to save, got %q", m.statusMessage)
	}
	if !fileContains(t, pathB, combinedSetVarName) {
		t.Error("expected Collection B's file to hold the script-set variable")
	}
	if fileContains(t, pathA, combinedSetVarName) {
		t.Error("expected Collection A's file untouched")
	}
	if !slices.ContainsFunc(m.collection.Variables, func(v postman.Variable) bool { return v.Key == combinedSetVarName }) {
		t.Error("expected the combined view to show the new variable")
	}
}
