package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	repeatLastActionKey = "."
	expandFolderKey     = "z"
	expandAllFoldersKey = "Z"
)

// handleRepeatLastActionKey repeats the last repeatable action (execute,
// delete, duplicate, favorite, expand) on the item now under the cursor.
func handleRepeatLastActionKey(m Model) (Model, tea.Cmd) {
	if m.lastRepeatableKey == "" {
		m.statusMessage = "Nothing to repeat"
		return m, nil
	}
	repeated, cmd, handled := m.commandRegistry.HandleKey(m, m.lastRepeatableKey)
	if !handled {
		m.statusMessage = fmt.Sprintf("Can't repeat %s here", m.lastRepeatableKey)
		return m, nil
	}
	return repeated, cmd
}
