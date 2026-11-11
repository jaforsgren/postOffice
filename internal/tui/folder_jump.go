package tui

import tea "github.com/charmbracelet/bubbletea"

const (
	nextFolderKey     = "shift+down"
	previousFolderKey = "shift+up"
)

func handleNextFolderKey(m Model) (Model, tea.Cmd) {
	return m.jumpToFolder(1), nil
}

func handlePreviousFolderKey(m Model) (Model, tea.Cmd) {
	return m.jumpToFolder(-1), nil
}

// jumpToFolder moves the cursor to the nearest folder row in direction
// (+1 down, -1 up), leaving it in place when no folder lies that way.
func (m Model) jumpToFolder(direction int) Model {
	for i := m.cursor + direction; i >= 0 && i < len(m.currentItems); i += direction {
		if m.currentItems[i].IsFolder() {
			m.cursor = i
			return m
		}
	}
	return m
}
