package tui

import tea "github.com/charmbracelet/bubbletea"

// viewSwitchKeys open the main views directly, without command mode.
const (
	collectionsViewKey  = "1"
	requestsViewKey     = "2"
	environmentsViewKey = "3"
	variablesViewKey    = "4"
)

// viewSwitchModes are the views the view keys work from: every view whose
// keys go through the registry.
var viewSwitchModes = []ViewMode{
	ModeCollections, ModeRequests, ModeEnvironments, ModeVariables, ModeChanges,
	ModeResponse, ModeInfo, ModeJSON, ModeLog, ModeSavedResponses, ModeHistory,
	ModeWorkflows, ModeWorkflowDetail, ModeWorkflowRun, ModeDependencyGraph,
}

// viewSwitchBindings bind each view key to the command opening that view.
func viewSwitchBindings() []*KeyBinding {
	views := []struct {
		key         string
		description string
		open        CommandHandler
	}{
		{collectionsViewKey, "Collections view", handleCollectionsCommand},
		{requestsViewKey, "Requests view", handleRequestsCommand},
		{environmentsViewKey, "Environments view", handleEnvironmentsCommand},
		{variablesViewKey, "Variables view", handleVariablesCommand},
	}
	bindings := make([]*KeyBinding, len(views))
	for i, view := range views {
		open := view.open
		bindings[i] = &KeyBinding{
			Keys:        []string{view.key},
			Description: view.description,
			ShortHelp:   view.key,
			Handler:     func(m Model) (Model, tea.Cmd) { return open(m, nil) },
			AvailableIn: viewSwitchModes,
		}
	}
	return bindings
}
