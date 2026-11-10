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
