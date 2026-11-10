package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func undoKeyMsg() tea.KeyMsg { return runes(undoKey) }
func redoKeyMsg() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyCtrlR} }

// deletedUsersModel is the combined view inside Collection A after deleting
// its only request.
func deletedUsersModel(t *testing.T) (Model, string) {
	t.Helper()
	m, pathA, _ := combinedModel(t)
	m = cursorOn(t, openFolder(t, m, combinedA), usersRequest)
	m = m.deleteRequest(m.currentItems[m.cursor], m.folderPathForCursor())
	if fileContains(t, pathA, usersRequest) {
		t.Fatal("expected the request deleted from the file")
	}
	return m, pathA
}

func TestUndo_RestoresADeletedRequest(t *testing.T) {
	m, pathA := deletedUsersModel(t)

	m = pressKeys(t, m, undoKeyMsg())

	if !fileContains(t, pathA, usersRequest) {
		t.Errorf("expected the request back in the file (%s)", m.statusMessage)
	}
	if len(m.currentItems) != 1 || m.currentItems[0].Name != usersRequest {
		t.Errorf("expected the request back in the view, got %v", m.items)
	}
	collectionA, _ := m.parser.GetCollection(combinedA)
	if len(collectionA.Items) != 1 {
		t.Errorf("expected the loaded collection restored, got %+v", collectionA.Items)
	}
}

func TestRedo_DeletesItAgain(t *testing.T) {
	m, pathA := deletedUsersModel(t)

	m = pressKeys(t, m, undoKeyMsg(), redoKeyMsg())

	collectionA, _ := m.parser.GetCollection(combinedA)
	if fileContains(t, pathA, usersRequest) || len(collectionA.Items) != 0 {
		t.Errorf("expected the request deleted again, got %+v (%s)", collectionA.Items, m.statusMessage)
	}
	m = pressKeys(t, m, redoKeyMsg())
	if m.statusMessage != "Nothing to redo" {
		t.Errorf("got %q", m.statusMessage)
	}
}

func TestUndo_ANewChangeClearsRedo(t *testing.T) {
	m, _ := deletedUsersModel(t)
	m = pressKeys(t, m, undoKeyMsg())

	m = m.duplicateRequest(m.currentItems[0], m.breadcrumb)
	m = pressKeys(t, m, redoKeyMsg())

	if m.statusMessage != "Nothing to redo" {
		t.Errorf("expected the redo dropped, got %q", m.statusMessage)
	}
}

func TestUndo_RefusesWhileTheCollectionHasUnsavedEdits(t *testing.T) {
	m, pathA := deletedUsersModel(t)
	m.modifiedCollections[combinedA] = true

	m = pressKeys(t, m, undoKeyMsg())

	if fileContains(t, pathA, usersRequest) || !m.statusIsError {
		t.Errorf("expected the undo refused, got %q", m.statusMessage)
	}
}

func TestUndo_RestoresAnEnvironmentValue(t *testing.T) {
	m, _, envPath := requestVariablesModel(t)
	m = pressKeys(t, m, runes(requestVariablesKey), runes("k"), runes("k"), runes("e"))
	m.editFieldInput.SetValue("p2")
	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyEsc})

	m = pressKeys(t, m, undoKeyMsg())

	if fileContains(t, envPath, "p2") || m.environment.Values[0].Value != "p1" {
		t.Errorf("expected p1 restored, got %+v (%s)", m.environment.Values[0], m.statusMessage)
	}
}

func TestUndo_NothingToUndo(t *testing.T) {
	m, _, _ := combinedModel(t)

	m = pressKeys(t, m, undoKeyMsg())

	if m.statusMessage != "Nothing to undo" {
		t.Errorf("got %q", m.statusMessage)
	}
}

func TestDelete_LeavesTheCollectionSaved(t *testing.T) {
	m, _ := deletedUsersModel(t)

	if m.isCollectionUnsaved(combinedA) {
		t.Errorf("expected nothing unsaved after the delete was saved, got %v", m.modifiedCollections)
	}
}
