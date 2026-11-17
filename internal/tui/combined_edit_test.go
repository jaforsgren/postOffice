package tui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"postOffice/internal/history"
	"postOffice/internal/postman"
)

const (
	combinedA       = "Collection A"
	combinedB       = "Collection B"
	usersRequest    = "Get Users"
	ordersFolder    = "Orders"
	ordersRequest   = "Get Orders"
	combinedVarName = "a_var"
)

// combinedModel shows collections A and B in the combined view, both saved
// to files whose paths it returns.
func combinedModel(t *testing.T) (Model, string, string) {
	t.Helper()
	pathA := writeJSONFile(t, "a.json", postman.Collection{
		Info:      postman.Info{Name: combinedA},
		Variables: []postman.Variable{{Key: combinedVarName, Value: "one"}},
		Items:     []postman.Item{{Name: usersRequest, Request: &postman.Request{Method: "GET", URL: postman.URL{Raw: "https://a/{{" + combinedVarName + "}}"}}}},
	})
	pathB := writeJSONFile(t, "b.json", postman.Collection{
		Info: postman.Info{Name: combinedB},
		Items: []postman.Item{{Name: ordersFolder, Items: []postman.Item{
			{Name: ordersRequest, Request: &postman.Request{Method: "GET", URL: postman.URL{Raw: "https://b/orders"}}},
		}}},
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

func openFolder(t *testing.T, m Model, name string) Model {
	t.Helper()
	for _, item := range m.currentItems {
		if item.Name == name {
			return m.navigateInto(item)
		}
	}
	t.Fatalf("no folder %q at %v", name, m.breadcrumb)
	return m
}

func cursorOn(t *testing.T, m Model, name string) Model {
	t.Helper()
	for i, item := range m.currentItems {
		if item.Name == name {
			m.cursor = i
			return m
		}
	}
	t.Fatalf("no item %q at %v", name, m.breadcrumb)
	return m
}

func fileContains(t *testing.T, path, text string) bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Contains(string(data), text)
}

func TestCombinedView_EditSavesToTheOwningCollection(t *testing.T) {
	m, _, pathB := combinedModel(t)
	m = openFolder(t, openFolder(t, m, combinedB), ordersFolder)
	m = cursorOn(t, m, ordersRequest)

	m = m.enterEditMode(m.currentItems[m.cursor], m.folderPathForCursor())
	if m.mode != ModeEdit {
		t.Fatalf("expected edit mode in the combined view, got %v (%s)", m.mode, m.statusMessage)
	}
	m.editRequest.Method = "POST"
	m, err := m.saveEdit()

	if err != nil {
		t.Fatalf("expected the save to succeed, got %v", err)
	}
	if !fileContains(t, pathB, `"POST"`) {
		t.Error("expected Collection B's file to hold the edit")
	}
	if len(m.modifiedCollections) != 0 {
		t.Errorf("expected nothing left unsaved, got %v", m.modifiedCollections)
	}
}

func TestCombinedView_EscMarksTheOwningCollectionUnsaved(t *testing.T) {
	m, _, _ := combinedModel(t)
	m = cursorOn(t, openFolder(t, m, combinedA), usersRequest)
	m = m.enterEditMode(m.currentItems[m.cursor], m.folderPathForCursor())
	m.editRequest.Method = "PUT"

	updated, _ := m.handleEditModeKeys(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)

	if !m.modifiedCollections[combinedA] || len(m.modifiedCollections) != 1 {
		t.Errorf("expected only %s marked unsaved, got %v", combinedA, m.modifiedCollections)
	}

	m, err := m.saveAllModifiedRequests()
	if err != nil {
		t.Fatalf("expected :w to save the real collection, got %v", err)
	}
}

func TestCombinedView_DuplicateAtACollectionsTopLevelReachesItsFile(t *testing.T) {
	m, pathA, pathB := combinedModel(t)
	m = cursorOn(t, openFolder(t, m, combinedA), usersRequest)

	m = m.duplicateRequest(m.currentItems[m.cursor], m.folderPathForCursor())

	if !fileContains(t, pathA, usersRequest+" (copy)") {
		t.Error("expected the copy in Collection A's file")
	}
	if fileContains(t, pathB, usersRequest) {
		t.Error("expected Collection B's file untouched")
	}
	if a, _ := m.parser.GetCollection(combinedA); len(a.Items) != 2 {
		t.Errorf("expected the loaded Collection A to hold the copy too, got %d items", len(a.Items))
	}
}

func TestCombinedView_DeleteAtACollectionsTopLevelReachesItsFile(t *testing.T) {
	m, pathA, _ := combinedModel(t)
	m = cursorOn(t, openFolder(t, m, combinedA), usersRequest)

	m = m.deleteRequest(m.currentItems[m.cursor], m.folderPathForCursor())

	if fileContains(t, pathA, usersRequest) {
		t.Error("expected the request gone from Collection A's file")
	}
	if a, _ := m.parser.GetCollection(combinedA); len(a.Items) != 0 {
		t.Errorf("expected the loaded Collection A emptied, got %+v", a.Items)
	}
}

func TestCombinedView_TopLevelRefusesNewItems(t *testing.T) {
	m, _, _ := combinedModel(t)

	m, _ = handleAddNewRequestKey(m)

	if m.statusMessage != combinedRootEditMessage || m.requestTypeSelectionMode {
		t.Errorf("expected adding at the combined root to be refused, got %q", m.statusMessage)
	}
	if _, cmd := m.openVimForCurrentLevel(); cmd != nil {
		t.Error("expected bulk editing the combined root to be refused")
	}
}

func TestCombinedView_RequestVariablesUseTheOwningCollection(t *testing.T) {
	m, pathA, _ := combinedModel(t)
	m = cursorOn(t, openFolder(t, m, combinedA), usersRequest)
	m = pressKeys(t, m, runes(requestVariablesKey), runes("e"))
	m.editFieldInput.SetValue("two")

	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if !fileContains(t, pathA, `"two"`) {
		t.Errorf("expected Collection A's file to hold the new value (%s)", m.statusMessage)
	}
	if got := m.requestVariableRows()[0].Value; got != "two" {
		t.Errorf("expected the view to show the new value, got %q", got)
	}
}

func TestCombinedView_ScriptSavesToTheOwningCollection(t *testing.T) {
	m, pathA, _ := combinedModel(t)
	m = cursorOn(t, openFolder(t, m, combinedA), usersRequest)
	msg := requestScriptsMsg(t, m.currentItems[m.cursor], m.folderPathForCursor())
	writeScriptBuffer(t, msg, testScriptLabel, "pm.test('combined', () => {})")

	m = m.applyVimScriptsEdit(msg)

	if !fileContains(t, pathA, "combined") {
		t.Errorf("expected Collection A's file to hold the script (%s)", m.statusMessage)
	}
}

func TestCombinedView_EditorEditMarksTheOwningCollection(t *testing.T) {
	m, _, _ := combinedModel(t)
	m = openFolder(t, openFolder(t, m, combinedB), ordersFolder)
	folderPath := m.folderPathForCursor()
	msg := VimEditCompleteMsg{FolderPath: folderPath, OriginalName: ordersRequest, ItemID: ordersRequest}

	m = m.applyVimEditToCollection(msg, vimEditPayload{Name: ordersRequest, Method: "DELETE", URL: "https://b/orders"})

	if !m.modifiedCollections[combinedB] || len(m.modifiedCollections) != 1 {
		t.Errorf("expected only %s marked unsaved, got %v", combinedB, m.modifiedCollections)
	}
	b, _ := m.parser.GetCollection(combinedB)
	if b.Items[0].Items[0].Request.Method != "DELETE" {
		t.Errorf("expected the loaded Collection B to hold the edit, got %+v", b.Items[0].Items[0].Request)
	}
}

func TestCombinedView_FavoriteIsKeyedByItsOwnCollection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m, _, _ := combinedModel(t)
	m = cursorOn(t, openFolder(t, m, combinedA), usersRequest)

	m, _ = m.toggleFavoriteAtCursor()

	if want := combinedA + "/" + usersRequest; !m.favorites[want] || len(m.favorites) != 1 {
		t.Errorf("expected the favorite stored as %q, got %v", want, m.favorites)
	}
}

func TestCombinedView_FavoritesListsEachCollectionsFavorites(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m, _, _ := combinedModel(t)
	m.favorites[combinedB+"/"+ordersFolder+"/"+ordersRequest] = true

	m, _ = m.enterFavoritesView()

	if !m.viewingFavorites || len(m.currentItems) != 1 || m.currentItems[0].Name != ordersRequest {
		t.Fatalf("expected the favorites view with %s, got %v (%s)", ordersRequest, m.items, m.statusMessage)
	}
	if !strings.Contains(m.items[0], combinedB+" / "+ordersFolder+" / "+ordersRequest) {
		t.Errorf("expected the row to show its collection and folder, got %q", m.items[0])
	}
	if got := m.folderPathForCursor(); len(got) != 2 || got[0] != combinedB {
		t.Errorf("expected the row's real combined-view path, got %v", got)
	}
}
