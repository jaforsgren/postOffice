package tui

import (
	"strings"

	"postOffice/internal/postman"
	"postOffice/internal/workflow"
)

// searchableModes are the lists that / searches.
var searchableModes = []ViewMode{ModeCollections, ModeRequests, ModeEnvironments, ModeVariables, ModeWorkflows, ModeListeners}

// clearSearch shows the unfiltered list again.
func (m Model) clearSearch() Model {
	m.searchActive = false
	m.searchInput.SetValue("")
	m.items = m.allItems
	m.currentItems = m.allCurrentItems
	switch m.mode {
	case ModeVariables:
		m.variables = m.allVariables
	case ModeWorkflows:
		m.workflows = m.allWorkflows
		m.workflowCursor = 0
	case ModeListeners:
		m.listenerQuery = ""
		m = m.followLatestListenedMessage()
	}
	m.cursor = 0
	return m
}

func (m Model) searchResultCount() int {
	switch m.mode {
	case ModeVariables:
		return len(m.variables)
	case ModeWorkflows:
		return len(m.workflows)
	case ModeListeners:
		return len(m.visibleListenedMessages())
	default:
		return len(m.items)
	}
}

// filterVariables keeps the variables whose key or value contains query
// (lower case).
func filterVariables(variables []postman.VariableSource, query string) []postman.VariableSource {
	var matches []postman.VariableSource
	for _, variable := range variables {
		if strings.Contains(strings.ToLower(variable.Key), query) || strings.Contains(strings.ToLower(variable.Value), query) {
			matches = append(matches, variable)
		}
	}
	return matches
}

func variableKeys(variables []postman.VariableSource) []string {
	keys := make([]string, len(variables))
	for i, variable := range variables {
		keys[i] = variable.Key
	}
	return keys
}

// filterWorkflows keeps the workflows whose ID or name contains query (lower case).
func filterWorkflows(workflows []*workflow.Workflow, query string) []*workflow.Workflow {
	var matches []*workflow.Workflow
	for _, wf := range workflows {
		if strings.Contains(strings.ToLower(wf.ID), query) || strings.Contains(strings.ToLower(wf.Name), query) {
			matches = append(matches, wf)
		}
	}
	return matches
}
