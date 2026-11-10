package tui

import (
	"fmt"
	"slices"

	tea "github.com/charmbracelet/bubbletea"
)

const nextEnvironmentKey = "ctrl+n"

// handleNextEnvironmentKey switches to the next loaded environment, after
// the last one back to the first, without leaving the current view. From
// the combined environment it starts at the first one.
func handleNextEnvironmentKey(m Model) (Model, tea.Cmd) {
	names := m.parser.ListEnvironments()
	if len(names) == 0 {
		m.statusMessage = "No environments loaded"
		return m, nil
	}

	next := 0
	if m.environment != nil && !m.isViewingMergedEnvironment() {
		next = (slices.Index(names, m.environment.Name) + 1) % len(names)
	}
	environment, _ := m.parser.GetEnvironment(names[next])
	m.environment = environment
	m.mergedEnvironmentActive = false

	if m.mode == ModeVariables {
		m = m.loadVariablesList()
	}
	m.statusMessage = fmt.Sprintf("Environment: %s (%d/%d)", environment.Name, next+1, len(names))
	return m, nil
}
