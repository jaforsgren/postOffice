package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func ctrlC() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyCtrlC} }

func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestCtrlC_QuitsFromEveryView(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m, _, _ := combinedModel(t)
	m = cursorOn(t, openFolder(t, m, combinedA), usersRequest)
	editing := m.enterEditMode(m.currentItems[m.cursor], m.folderPathForCursor())
	commandMode := m
	commandMode.commandMode = true

	for name, model := range map[string]Model{"requests": m, "edit form": editing, "command bar": commandMode} {
		if _, cmd := model.Update(ctrlC()); !quits(cmd) {
			t.Errorf("%s: expected ctrl+c to quit", name)
		}
	}
}

func TestQ_BacksOutOfTheListsAndQuitsFromCollections(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m, _, _ := combinedModel(t)
	m = openFolder(t, m, combinedA)

	m = pressKeys(t, m, runes("q"))
	if m.mode != ModeRequests || len(m.breadcrumb) != 0 {
		t.Fatalf("expected q to go up a folder, got %v at %v", m.mode, m.breadcrumb)
	}
	m = pressKeys(t, m, runes("q"))
	if m.mode != ModeCollections {
		t.Fatalf("expected q at the top to open the collections list, got %v", m.mode)
	}
	if _, cmd := m.Update(runes("q")); !quits(cmd) {
		t.Error("expected q to quit from the collections list")
	}
}

func TestQ_LeavesEnvironmentsForCollections(t *testing.T) {
	m := environmentsModel(t, "dev")
	m.mode = ModeEnvironments

	m = pressKeys(t, m, runes("q"))

	if m.mode != ModeCollections {
		t.Errorf("expected the collections list, got %v", m.mode)
	}
}
