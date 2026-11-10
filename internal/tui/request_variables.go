package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"postOffice/internal/postman"
)

const (
	requestVariablesKey      = "V"
	undefinedVariableSource  = "undefined: saving adds it to the collection"
	requestVariablesShortcut = "<Enter/e> Edit value  <j/k> Navigate  <Esc/q> Back"
	requestVariableEditHint  = "<Enter> Save to file  <Esc> Cancel"
)

// requestVariableRow is one {{variable}} a request uses, with the value and
// source it currently resolves to.
type requestVariableRow struct {
	Key     string
	Value   string
	Source  string
	Defined bool
}

// handleRequestVariablesKey lists the variables the selected request uses, so
// just those can be edited in place in whichever environment, collection or
// folder defines them.
func handleRequestVariablesKey(m Model) (Model, tea.Cmd) {
	item, ok := m.selectedItem()
	if !ok || !item.IsRequest() {
		m.statusMessage = "Select a request to edit its variables"
		return m, nil
	}
	if len(postman.ReferencedVariables(item.Request)) == 0 {
		m.statusMessage = fmt.Sprintf("%s uses no {{variables}}", item.Name)
		return m, nil
	}

	m.requestVarsItem = item
	m.requestVarsFolderPath = m.folderPathForCursor()
	m.requestVarsCursor = 0
	m.requestVarEditing = false
	m.previousMode = m.mode
	m.mode = ModeRequestVariables
	m.statusMessage = fmt.Sprintf("Variables used by %s", item.Name)
	return m, nil
}

func (m Model) requestVariableRows() []requestVariableRow {
	collection, folderPath := m.collectionScope(m.requestVarsFolderPath)
	resolved := make(map[string]postman.VariableSource)
	for _, variable := range postman.GetAllVariables(collection, folderPath, m.environment) {
		resolved[variable.Key] = variable
	}

	var rows []requestVariableRow
	for _, key := range postman.ReferencedVariables(m.requestVarsItem.Request) {
		variable, defined := resolved[key]
		row := requestVariableRow{Key: key, Value: variable.Value, Source: variable.Source, Defined: defined}
		if !defined {
			row.Source = undefinedVariableSource
		}
		rows = append(rows, row)
	}
	return rows
}

func (m Model) handleRequestVariablesKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.requestVarEditing {
		return m.handleRequestVariableEditKeys(msg)
	}

	rows := m.requestVariableRows()
	switch msg.String() {
	case "esc", "q", "h", "backspace":
		m.mode = m.previousMode
		m.statusMessage = ""
	case "j", "down":
		if m.requestVarsCursor < len(rows)-1 {
			m.requestVarsCursor++
		}
	case "k", "up":
		if m.requestVarsCursor > 0 {
			m.requestVarsCursor--
		}
	case "enter", "e":
		if m.requestVarsCursor < len(rows) {
			m.editFieldInput.SetValue(rows[m.requestVarsCursor].Value)
			m.editFieldInput.CursorEnd()
			m.requestVarEditing = true
			m.statusMessage = fmt.Sprintf("Editing {{%s}}", rows[m.requestVarsCursor].Key)
			return m, m.editFieldInput.Focus()
		}
	}
	return m, nil
}

func (m Model) handleRequestVariableEditKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.requestVarEditing = false
		m.editFieldInput.Blur()
		m.statusMessage = "Edit cancelled"
		return m, nil
	case tea.KeyEnter:
		m.requestVarEditing = false
		m.editFieldInput.Blur()
		rows := m.requestVariableRows()
		if m.requestVarsCursor >= len(rows) {
			return m, nil
		}
		return m.saveRequestVariable(rows[m.requestVarsCursor].Key, m.editFieldInput.Value()), nil
	}

	var cmd tea.Cmd
	m.editFieldInput, cmd = m.editFieldInput.Update(msg)
	return m, cmd
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

func (m Model) saveRequestVariable(key, value string) Model {
	collection, folderPath := m.collectionScope(m.requestVarsFolderPath)
	owner := postman.SetVariable(collection, folderPath, m.variableEnvironments(), key, value)

	var err error
	if owner.Scope == postman.VariableScopeEnvironment {
		err = m.parser.SaveEnvironment(owner.Name)
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

// reloadActiveEnvironment points m.environment back at the loaded
// environment(s) after one of them changed.
func (m Model) reloadActiveEnvironment() Model {
	if m.mergedEnvironmentActive {
		m.environment = m.parser.MergedEnvironment()
		return m
	}
	if environment, exists := m.parser.GetEnvironment(m.environment.Name); exists {
		m.environment = environment
	}
	return m
}

func (m Model) renderRequestVariables(availableHeight int) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	undefinedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))

	lines := []string{titleStyle.Render("Variables used by " + m.requestVarsItem.Name), ""}

	const keyColWidth = 30
	for i, row := range m.requestVariableRows() {
		prefix, style := "  ", normalItemStyle
		if i == m.requestVarsCursor {
			prefix, style = "> ", selectedItemStyle
		}

		value := style.Render(strings.ReplaceAll(row.Value, "\n", " "))
		if i == m.requestVarsCursor && m.requestVarEditing {
			value = m.editFieldInput.View()
		}
		lines = append(lines, prefix+style.Width(keyColWidth).Render("{{"+row.Key+"}}")+"  "+value)

		sourceStyle := dimStyle
		if !row.Defined {
			sourceStyle = undefinedStyle
		}
		lines = append(lines, strings.Repeat(" ", keyColWidth+4)+sourceStyle.Italic(true).Render(row.Source))
	}

	hint := requestVariablesShortcut
	if m.requestVarEditing {
		hint = requestVariableEditHint
	}
	lines = append(lines, "", dimStyle.Render(hint))

	return mainWindowStyle.
		Height(availableHeight).
		Width(m.width - 4).
		Render(strings.Join(lines, "\n"))
}
