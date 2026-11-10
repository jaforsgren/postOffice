package tui

import (
	"slices"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func jumpBack() tea.KeyMsg    { return tea.KeyMsg{Type: tea.KeyCtrlO} }
func jumpForward() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyTab} }

// openOrders goes from the combined view's top level into B/Orders by keys.
func openOrders(t *testing.T, m Model) Model {
	t.Helper()
	m = pressKeys(t, cursorOn(t, m, combinedB), enterKey)
	return pressKeys(t, cursorOn(t, m, ordersFolder), enterKey)
}

func TestJumplist_BackAndForwardThroughFolders(t *testing.T) {
	m, _, _ := combinedModel(t)
	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = openOrders(t, m)

	m = pressKeys(t, m, jumpBack())
	if want := []string{combinedB}; !slices.Equal(m.breadcrumb, want) {
		t.Fatalf("expected back in %v, got %v", want, m.breadcrumb)
	}
	if item, _ := m.selectedItem(); item.Name != ordersFolder {
		t.Errorf("expected the cursor back on %s, got %s", ordersFolder, item.Name)
	}

	m = pressKeys(t, m, jumpBack())
	if len(m.breadcrumb) != 0 {
		t.Fatalf("expected back at the top level, got %v", m.breadcrumb)
	}
	if item, _ := m.selectedItem(); item.Name != combinedB {
		t.Errorf("expected the cursor back on %s, got %s", combinedB, item.Name)
	}

	m = pressKeys(t, m, jumpForward(), jumpForward())
	if want := []string{combinedB, ordersFolder}; !slices.Equal(m.breadcrumb, want) {
		t.Errorf("expected forward in %v again, got %v", want, m.breadcrumb)
	}
	if item, _ := m.selectedItem(); item.Name != ordersRequest {
		t.Errorf("expected the cursor on %s, got %s", ordersRequest, item.Name)
	}
}

func TestJumplist_ReturnsToAnotherCollection(t *testing.T) {
	m, _, _ := combinedModel(t)
	m = openOrders(t, m)
	m = pressKeys(t, m, runes(":"))
	m.commandInput.SetValue("c")
	m = pressKeys(t, m, enterKey)
	m.parser.ToggleActiveCollection(combinedA)
	m.parser.ToggleActiveCollection(combinedB)
	m = pressKeys(t, cursorOnItem(t, m, combinedA), enterKey)
	if m.isViewingMergedCollection() || m.collection.Info.Name != combinedA {
		t.Fatalf("expected Collection A's own view, got %s", m.collection.Info.Name)
	}
	m.parser.ToggleActiveCollection(combinedA)
	m.parser.ToggleActiveCollection(combinedB)

	m = pressKeys(t, m, jumpBack())

	if !m.isViewingMergedCollection() || !slices.Equal(m.breadcrumb, []string{combinedB, ordersFolder}) {
		t.Errorf("expected back in the combined view's Orders, got %s at %v", m.collection.Info.Name, m.breadcrumb)
	}
}

func TestJumplist_NewJumpDropsTheForwardJumps(t *testing.T) {
	m, _, _ := combinedModel(t)
	m = openOrders(t, m)
	m = pressKeys(t, m, jumpBack(), jumpBack())

	m = pressKeys(t, cursorOn(t, m, combinedA), enterKey)

	if m.jumpIndex != len(m.jumps) || len(m.jumps) != 1 {
		t.Errorf("expected only the top level left in the jumplist, got %+v at %d", m.jumps, m.jumpIndex)
	}
	m = pressKeys(t, m, jumpForward())
	if m.statusMessage != "Already at the newest jump" {
		t.Errorf("expected nothing to jump forward to, got %q", m.statusMessage)
	}
}

func TestJumplist_EmptyJumpBack(t *testing.T) {
	m, _, _ := combinedModel(t)

	m = pressKeys(t, m, jumpBack())

	if m.statusMessage != "Already at the oldest jump" {
		t.Errorf("got %q", m.statusMessage)
	}
}

func cursorOnItem(t *testing.T, m Model, name string) Model {
	t.Helper()
	for i, item := range m.items {
		if item == name {
			m.cursor = i
			return m
		}
	}
	t.Fatalf("no item %q in %v", name, m.items)
	return m
}
