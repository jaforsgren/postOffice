package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"postOffice/internal/postman"
)

const (
	requestVariablesKey     = "V"
	undefinedVariableSource = "undefined: saving adds it to the collection"

	unresolvedVariablesFormat = "⚠ Unresolved: %s (press %s to set them; new ones are saved as collection variables)"
)

var unresolvedVariablesStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))

// requestVariableRow is one {{variable}} a request uses, with the value and
// source it currently resolves to.
type requestVariableRow struct {
	Key         string
	Value       string
	Choices     []string
	ChoiceNotes map[string]string
	Source      string
	Defined     bool
}

func (row requestVariableRow) variableValue() postman.VariableValue {
	return postman.VariableValue{Value: row.Value, Choices: row.Choices, ChoiceNotes: row.ChoiceNotes}
}

// handleRequestVariablesKey lists the variables a request uses, so just
// those can be edited in place in whichever environment, collection or
// folder defines them. It works on the selected request, the request info
// panel's request, or the request being edited.
func handleRequestVariablesKey(m Model) (Model, tea.Cmd) {
	item, folderPath, ok := m.requestVariablesTarget()
	if !ok || !item.IsRequest() {
		m.statusMessage = "Select a request to edit its variables"
		return m, nil
	}
	if len(postman.ReferencedItemVariables(item)) == 0 {
		m.statusMessage = fmt.Sprintf("%s uses no {{variables}}", item.Name)
		return m, nil
	}

	m.requestVarsItem = item
	m.requestVarsFolderPath = folderPath
	m.requestVarsCursor = firstUndefinedRow(m.variableRows(item, folderPath))
	m.requestVarEdit = requestVarEditNone
	m.requestVarsReturnMode = m.mode
	m.mode = ModeRequestVariables
	m.statusMessage = fmt.Sprintf("Variables used by %s", item.Name)
	return m, nil
}

func (m Model) requestVariablesTarget() (postman.Item, []string, bool) {
	switch {
	case m.mode == ModeEdit && m.editRequest != nil:
		item := postman.Item{Name: m.editItemName, Request: m.editRequest}
		if saved := m.findItemByPath(m.editItemPath, m.editOriginalName); saved != nil {
			item.Events = saved.Events
		}
		return item, m.editItemPath, true
	case m.mode == ModeResponse && m.responsePanelItem != nil:
		return *m.responsePanelItem, m.requestsViewFolderPath(), true
	case m.mode == ModeRequests:
		item, ok := m.selectedItem()
		return item, m.folderPathForCursor(), ok
	}
	return postman.Item{}, nil, false
}

// firstUndefinedRow puts the cursor where a value is missing, or on the first row.
func firstUndefinedRow(rows []requestVariableRow) int {
	for i, row := range rows {
		if !row.Defined {
			return i
		}
	}
	return 0
}

// closeRequestVariables returns to the view the variables were opened from,
// refreshing the request info panel so it shows the new values.
func (m Model) closeRequestVariables() Model {
	m.mode = m.requestVarsReturnMode
	m.statusMessage = ""
	if m.mode == ModeResponse && m.responsePanelItem != nil {
		m.configureViewport(&m.responseViewport, strings.Join(m.responseViewLines(), "\n"))
	}
	return m
}

func (m Model) requestVariableRows() []requestVariableRow {
	return m.variableRows(m.requestVarsItem, m.requestVarsFolderPath)
}

// unresolvedVariables lists the {{variables}} req uses that nothing defines.
func (m Model) unresolvedVariables(req *postman.Request, folderPath []string) []string {
	return postman.UnresolvedVariables(req, m.variablesAt(folderPath))
}

// unresolvedVariablesLines is a warning naming the request's unresolved
// variables and how to set them, or nothing when all of them resolve.
func (m Model) unresolvedVariablesLines(req *postman.Request, folderPath []string) []string {
	keys := m.unresolvedVariables(req, folderPath)
	if len(keys) == 0 {
		return nil
	}
	warning := fmt.Sprintf(unresolvedVariablesFormat, strings.Join(keys, ", "), requestVariablesKey)
	return []string{unresolvedVariablesStyle.Render(warning)}
}

func (m Model) variableRows(item postman.Item, requestFolderPath []string) []requestVariableRow {
	resolved := make(map[string]postman.VariableSource)
	for _, variable := range m.variablesAt(requestFolderPath) {
		resolved[variable.Key] = variable
	}

	var rows []requestVariableRow
	for _, key := range postman.ReferencedItemVariables(item) {
		variable, defined := resolved[key]
		row := requestVariableRow{Key: key, Value: variable.Value, Choices: variable.Choices, ChoiceNotes: variable.ChoiceNotes, Source: variable.Source, Defined: defined}
		if !defined {
			row.Source = undefinedVariableSource
		}
		rows = append(rows, row)
	}
	return rows
}

// variableEnvironments are the loaded environments behind m.environment, in
// the precedence order variables resolve from. Edits must go to these, not
// to a merged copy, to reach the files.
func (m Model) variableEnvironments() []*postman.Environment {
	if m.environment == nil {
		return nil
	}
	names := []string{m.environment.Name}
	if m.mergedEnvironmentActive {
		names = m.parser.ActiveEnvironments()
	}

	var environments []*postman.Environment
	for _, name := range names {
		if environment, exists := m.parser.GetEnvironment(name); exists {
			environments = append(environments, environment)
		}
	}
	return environments
}

func (m Model) saveRequestVariable(key string, value postman.VariableValue) Model {
	collection, folderPath := m.collectionScope(m.requestVarsFolderPath)
	owner := postman.SetVariable(collection, folderPath, m.variableEnvironments(), key, value)

	var err error
	if owner.Scope == postman.VariableScopeEnvironment {
		err = m.saveEnvironmentUndoable(owner.Name)
		m = m.reloadActiveEnvironment()
	} else {
		err = m.saveOwningCollection(m.requestVarsFolderPath)
		m = m.reloadMergedCollection()
	}
	if err != nil {
		m.statusMessage = fmt.Sprintf("Set {{%s}} but failed to save %s: %v", key, owner.Name, err)
		m.statusIsError = true
		return m
	}
	m.statusMessage = fmt.Sprintf("Saved {{%s}} to %s", key, owner.Name)
	return m
}

// reloadMergedCollection rebuilds the combined view after a collection's
// variables changed, since it holds copies of them.
func (m Model) reloadMergedCollection() Model {
	if !m.isViewingMergedCollection() {
		return m
	}
	m.collection = m.parser.MergedCollection()
	return m
}

// saveScriptEnvironment saves the variables a request's scripts set in
// environment. A merged environment is a copy, and reloading it from the
// loaded environments drops its changes, so they are written back to those
// first.
func (m Model) saveScriptEnvironment(environment *postman.Environment) error {
	if loaded, exists := m.parser.GetEnvironment(environment.Name); exists && loaded == environment {
		return m.parser.SaveEnvironment(environment.Name)
	}
	for _, name := range m.parser.SyncMergedEnvironment(environment) {
		if err := m.parser.SaveEnvironment(name); err != nil {
			return err
		}
	}
	return nil
}

// reloadActiveEnvironment points m.environment back at the loaded
// environment(s) after one of them changed.
func (m Model) reloadActiveEnvironment() Model {
	if m.environment == nil {
		return m
	}
	if m.mergedEnvironmentActive {
		m.environment = m.parser.MergedEnvironment()
		return m
	}
	if environment, exists := m.parser.GetEnvironment(m.environment.Name); exists {
		m.environment = environment
	}
	return m
}
