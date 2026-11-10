package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// unsavedOrdersModel is the combined view with Collection B's Get Orders
// edited but not saved, back at the combined view's top level.
func unsavedOrdersModel(t *testing.T) Model {
	t.Helper()
	m, _, _ := combinedModel(t)
	m = cursorOn(t, openFolder(t, openFolder(t, m, combinedB), ordersFolder), ordersRequest)
	m = m.enterEditMode(m.currentItems[m.cursor], m.folderPathForCursor())
	m.editRequest.Method = "PATCH"
	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	return m.navigateUp().navigateUp()
}

func rowFor(t *testing.T, m Model, name string) string {
	t.Helper()
	for i, item := range m.items {
		if strings.Contains(item, name) {
			return m.formatItemLine(i, itemNameWidth)
		}
	}
	t.Fatalf("no row %q in %v", name, m.items)
	return ""
}

func TestUnsavedMarker_OnFoldersHoldingAnUnsavedRequest(t *testing.T) {
	m := unsavedOrdersModel(t)

	if !strings.Contains(rowFor(t, m, combinedB), unsavedMarker) {
		t.Error("expected Collection B's group folder marked")
	}
	if strings.Contains(rowFor(t, m, combinedA), unsavedMarker) {
		t.Error("expected Collection A's group folder unmarked")
	}

	m = openFolder(t, m, combinedB)
	if !strings.Contains(rowFor(t, m, ordersFolder), unsavedMarker) {
		t.Error("expected the Orders folder marked")
	}
}

func TestUnsavedMarker_OnCollections(t *testing.T) {
	m := unsavedOrdersModel(t)
	m.mode = ModeCollections
	m = m.loadCollectionsList()

	if !strings.Contains(rowFor(t, m, combinedB), unsavedMarker) || strings.Contains(rowFor(t, m, combinedA), unsavedMarker) {
		t.Errorf("expected only Collection B marked:\n%s\n%s", rowFor(t, m, combinedA), rowFor(t, m, combinedB))
	}
}

func TestUnsavedMarker_OnEnvironments(t *testing.T) {
	m := environmentsModel(t, "dev", "prod")
	m.mode = ModeEnvironments
	m = m.loadEnvironmentsList()
	m.modifiedEnvironments["prod"] = true

	if !strings.Contains(rowFor(t, m, "prod"), unsavedMarker) || strings.Contains(rowFor(t, m, "dev"), unsavedMarker) {
		t.Error("expected only prod marked")
	}
}
