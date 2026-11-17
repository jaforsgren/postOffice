package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"postOffice/internal/postman"
)

const (
	newVariableKey    = "n"
	deleteVariableKey = "d"
	scopeSourceFormat = "%s: %s"
	collectionScope   = "Collection"
	folderScope       = "Folder"
	emptyScopeMessage = "No variables here yet: " + newVariableKey + " to add one"
)

var (
	scopeVariablesShortcuts = []shortcutHint{
		{"enter/e", "Edit value"},
		{addChoiceKey, "Add choice"},
		{newVariableKey, "New variable"},
		{deleteVariableKey, "Delete variable"},
		{"j/k", "Navigate"},
		{"esc/q", "Back"},
	}
	newVariableShortcuts = []shortcutHint{
		{"enter", "Add (then type its value)"},
		{"esc", "Cancel"},
	}
)

// handleCollectionVariablesCommand opens the variables of the collection the
// requests view is in: in the combined view, the one owning the current
// folder or, at its top level, the selected one.
func handleCollectionVariablesCommand(m Model, _ []string) (Model, tea.Cmd) {
	if m.collection == nil {
		m.statusMessage = "Open a collection first"
		return m, nil
	}
	var viewPath []string
	if m.isViewingMergedCollection() {
		path := m.folderPathForCursor()
		if item, ok := m.selectedItem(); len(path) == 0 && ok && item.IsFolder() {
			path = []string{item.Name}
		}
		if len(path) == 0 {
			m.statusMessage = "Select a collection's folder to edit its variables"
			return m, nil
		}
		viewPath = path[:1]
	}
	return m.openScopeVariables(viewPath), nil
}

// openScopeVariables lists the variables defined on the folder at viewPath
// (a requests view path) itself, or on its collection when viewPath names
// the collection, for editing in place.
func (m Model) openScopeVariables(viewPath []string) Model {
	m.requestVarsScope = true
	m.requestVarsItem = postman.Item{}
	m.requestVarsFolderPath = slices.Clone(viewPath)
	m.requestVarsCursor = 0
	m.requestVarEdit = requestVarEditNone
	m.requestVarsReturnMode = m.mode
	m.mode = ModeRequestVariables
	m.statusMessage = "Variables of " + m.scopeLabel()
	return m
}

// scopeTarget is the real collection and the folder path inside it that the
// panel edits.
func (m Model) scopeTarget() (*postman.Collection, []string) {
	return m.collectionScope(m.requestVarsFolderPath)
}

func (m Model) scopeLabel() string {
	collection, path := m.scopeTarget()
	if len(path) == 0 {
		return fmt.Sprintf(scopeSourceFormat, collectionScope, collection.Info.Name)
	}
	return fmt.Sprintf(scopeSourceFormat, folderScope, strings.Join(path, " / "))
}

func (m Model) scopeVariableRows() []requestVariableRow {
	collection, path := m.scopeTarget()
	variables, err := postman.ScopeVariables(collection, path)
	if err != nil {
		return nil
	}
	label := m.scopeLabel()
	rows := make([]requestVariableRow, len(variables))
	for i, variable := range variables {
		rows[i] = requestVariableRow{Key: variable.Key, Value: variable.Value, Choices: variable.Choices, ChoiceNotes: variable.ChoiceNotes, Source: label, Defined: true}
	}
	return rows
}

// changeScopeVariables applies change to the scope's variables in the loaded
// collection and saves it, undoably. Combined-view edits are written back
// first, and the combined view is rebuilt after, so neither overwrites the
// other.
func (m Model) changeScopeVariables(description string, change func(*postman.Collection, []string) error) Model {
	m.syncMergedCollection()
	collection, path := m.scopeTarget()
	if err := change(collection, path); err != nil {
		m.statusMessage = fmt.Sprintf("Could not %s in %s: %v", description, m.scopeLabel(), err)
		m.statusIsError = true
		return m
	}
	m = m.reloadMergedCollection()
	if err := m.saveOwningCollection(m.requestVarsFolderPath); err != nil {
		m.markCollectionModified(m.requestVarsFolderPath)
		m.statusMessage = fmt.Sprintf("Changed %s but failed to save: %v", m.scopeLabel(), err)
		m.statusIsError = true
		return m
	}
	m.statusMessage = fmt.Sprintf("Saved: %s in %s", description, m.scopeLabel())
	return m
}

func (m Model) saveScopeVariable(key string, value postman.VariableValue) Model {
	return m.changeScopeVariables("{{"+key+"}}", func(collection *postman.Collection, path []string) error {
		return postman.SetScopeVariable(collection, path, key, value)
	})
}

func (m Model) deleteScopeVariable(row requestVariableRow) Model {
	m = m.changeScopeVariables("deleted {{"+row.Key+"}}", func(collection *postman.Collection, path []string) error {
		return postman.DeleteScopeVariable(collection, path, row.Key)
	})
	m.requestVarsCursor = min(m.requestVarsCursor, max(len(m.requestVariableRows())-1, 0))
	return m
}

func (m Model) startNewVariable() (Model, tea.Cmd) {
	m.editFieldInput.SetValue("")
	m.requestVarEdit = requestVarEditNewKey
	m.statusMessage = "New variable in " + m.scopeLabel()
	return m, m.editFieldInput.Focus()
}

// handleNewVariableKeys adds the typed name as an empty variable and goes
// straight on to editing its value.
func (m Model) handleNewVariableKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		return m.stopRequestVariableEdit("Edit cancelled"), nil
	case tea.KeyEnter:
		key := strings.TrimSpace(m.editFieldInput.Value())
		if key == "" {
			return m, nil
		}
		rows := m.requestVariableRows()
		if slices.ContainsFunc(rows, func(row requestVariableRow) bool { return row.Key == key }) {
			m.statusMessage = fmt.Sprintf("{{%s}} already exists in %s", key, m.scopeLabel())
			m.statusIsError = true
			return m, nil
		}
		m = m.stopRequestVariableEdit("")
		m = m.saveScopeVariable(key, postman.VariableValue{})
		if m.statusIsError {
			return m, nil
		}
		m.requestVarsCursor = len(rows)
		row, _ := m.selectedRequestVariableRow()
		return m.startRequestVariableEdit(row)
	}

	var cmd tea.Cmd
	m.editFieldInput, cmd = m.editFieldInput.Update(msg)
	return m, cmd
}
