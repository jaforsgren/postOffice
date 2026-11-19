package tui

import tea "github.com/charmbracelet/bubbletea"

const toggleTopBarKey = "T"

func handleToggleTopBarKey(m Model) (Model, tea.Cmd) {
	m.topBarHidden = !m.topBarHidden
	if m.topBarHidden {
		m.statusMessage = "Top bar hidden (" + toggleTopBarKey + " to show)"
	} else {
		m.statusMessage = "Top bar shown"
	}
	return m.resizeViewports(), nil
}

// renderedTopBarHeight is the height the top bar takes, none while it is hidden.
func (m Model) renderedTopBarHeight() int {
	if m.topBarHidden {
		return 0
	}
	return topBarHeight
}
