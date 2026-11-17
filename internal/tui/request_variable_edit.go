package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"postOffice/internal/postman"
)

// requestVarEditState is what the request variables panel is editing.
type requestVarEditState int

const (
	requestVarEditNone requestVarEditState = iota
	// requestVarEditText edits a plain variable's value as text.
	requestVarEditText
	// requestVarEditChoices picks a choice variable's value from its choices.
	requestVarEditChoices
	// requestVarEditNewChoice types a choice to add and choose.
	requestVarEditNewChoice
	// requestVarEditChoiceNote types the optional note explaining a choice.
	requestVarEditChoiceNote
	// requestVarEditNewKey types the name of a variable to add to a scope.
	requestVarEditNewKey
)

const (
	addChoiceKey    = "a"
	deleteChoiceKey = "d"
	choiceNoteKey   = "c"
	notePrefix      = "# "
	choiceMarker    = "▾"
	chosenLabel     = "(current)"
	newChoicePrefix = "+ "
)

type shortcutHint struct {
	keys        string
	description string
}

var (
	requestVariablesShortcuts = []shortcutHint{
		{"enter/e", "Edit value"},
		{addChoiceKey, "Add choice"},
		{"j/k", "Navigate"},
		{"esc/q", "Back"},
	}
	requestVariableTextShortcuts = []shortcutHint{
		{"enter", "Save to file"},
		{"esc", "Cancel"},
	}
	requestVariableChoicesShortcuts = []shortcutHint{
		{"enter", "Choose"},
		{"j/k", "Navigate"},
		{addChoiceKey, "Add choice"},
		{deleteChoiceKey, "Delete choice"},
		{choiceNoteKey, "Note choice"},
		{"esc", "Cancel"},
	}
	requestVariableNewChoiceShortcuts = []shortcutHint{
		{"enter", "Add and choose"},
		{"esc", "Cancel"},
	}
	requestVariableChoiceNoteShortcuts = []shortcutHint{
		{"enter", "Save note (empty removes it)"},
		{"esc", "Cancel"},
	}
)

// formatShortcuts renders hints as "<keys> description", the format the top
// bar's shortcuts section reads.
func formatShortcuts(hints []shortcutHint) []string {
	formatted := make([]string, len(hints))
	for i, hint := range hints {
		formatted[i] = fmt.Sprintf("<%s> %s", hint.keys, hint.description)
	}
	return formatted
}

// contextualShortcuts are the current view's shortcuts for the top bar. The
// request variables panel handles its own keys, so it lists its own.
func (m Model) contextualShortcuts() []string {
	if m.mode == ModeRequestVariables {
		return formatShortcuts(m.requestVariableShortcuts())
	}
	return m.commandRegistry.GetContextualShortcuts(m.mode)
}

func (m Model) requestVariableShortcuts() []shortcutHint {
	switch m.requestVarEdit {
	case requestVarEditText:
		return requestVariableTextShortcuts
	case requestVarEditChoices:
		return requestVariableChoicesShortcuts
	case requestVarEditNewChoice:
		return requestVariableNewChoiceShortcuts
	case requestVarEditChoiceNote:
		return requestVariableChoiceNoteShortcuts
	case requestVarEditNewKey:
		return newVariableShortcuts
	default:
		if m.requestVarsScope {
			return scopeVariablesShortcuts
		}
		return requestVariablesShortcuts
	}
}

func (m Model) selectedRequestVariableRow() (requestVariableRow, bool) {
	rows := m.requestVariableRows()
	if m.requestVarsCursor >= len(rows) {
		return requestVariableRow{}, false
	}
	return rows[m.requestVarsCursor], true
}

func (m Model) handleRequestVariablesKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	row, hasRow := m.selectedRequestVariableRow()
	switch m.requestVarEdit {
	case requestVarEditText:
		return m.handleRequestVariableTextKeys(msg, row)
	case requestVarEditChoices:
		return m.handleRequestVariableChoiceKeys(msg, row)
	case requestVarEditNewChoice:
		return m.handleRequestVariableNewChoiceKeys(msg, row)
	case requestVarEditChoiceNote:
		return m.handleRequestVariableChoiceNoteKeys(msg, row)
	case requestVarEditNewKey:
		return m.handleNewVariableKeys(msg)
	}

	switch msg.String() {
	case "esc", "q", "h", "backspace":
		m = m.closeRequestVariables()
	case "j", "down":
		if m.requestVarsCursor < len(m.requestVariableRows())-1 {
			m.requestVarsCursor++
		}
	case "k", "up":
		if m.requestVarsCursor > 0 {
			m.requestVarsCursor--
		}
	case "enter", "e":
		if hasRow {
			return m.startRequestVariableEdit(row)
		}
	case addChoiceKey:
		if hasRow {
			return m.startNewChoice(row)
		}
	case newVariableKey:
		if m.requestVarsScope {
			return m.startNewVariable()
		}
	case deleteVariableKey:
		if m.requestVarsScope && hasRow {
			return m.deleteScopeVariable(row), nil
		}
	}
	return m, nil
}

// startRequestVariableEdit opens the choices of a choice variable, or a text
// input for a plain one.
func (m Model) startRequestVariableEdit(row requestVariableRow) (Model, tea.Cmd) {
	if row.variableValue().IsChoice() {
		m.requestVarEdit = requestVarEditChoices
		m.requestVarChoiceCursor = row.variableValue().ChoiceIndex()
		m.statusMessage = fmt.Sprintf("Choose {{%s}}", row.Key)
		return m, nil
	}
	m.editFieldInput.SetValue(row.Value)
	m.editFieldInput.CursorEnd()
	m.requestVarEdit = requestVarEditText
	m.statusMessage = fmt.Sprintf("Editing {{%s}}", row.Key)
	return m, m.editFieldInput.Focus()
}

func (m Model) startNewChoice(row requestVariableRow) (Model, tea.Cmd) {
	m.editFieldInput.SetValue("")
	m.requestVarEdit = requestVarEditNewChoice
	m.statusMessage = fmt.Sprintf("New choice for {{%s}}", row.Key)
	return m, m.editFieldInput.Focus()
}

func (m Model) stopRequestVariableEdit(status string) Model {
	m.requestVarEdit = requestVarEditNone
	m.editFieldInput.Blur()
	m.statusMessage = status
	return m
}

func (m Model) handleRequestVariableTextKeys(msg tea.KeyMsg, row requestVariableRow) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		return m.stopRequestVariableEdit("Edit cancelled"), nil
	case tea.KeyEnter:
		m = m.stopRequestVariableEdit("")
		return m.saveRequestVariable(row.Key, postman.VariableValue{Value: m.editFieldInput.Value(), Choices: row.Choices}), nil
	}

	var cmd tea.Cmd
	m.editFieldInput, cmd = m.editFieldInput.Update(msg)
	return m, cmd
}

func (m Model) handleRequestVariableChoiceKeys(msg tea.KeyMsg, row requestVariableRow) (tea.Model, tea.Cmd) {
	value := row.variableValue()
	switch msg.String() {
	case "esc", "q":
		return m.stopRequestVariableEdit("Edit cancelled"), nil
	case "j", "down":
		if m.requestVarChoiceCursor < len(value.Choices)-1 {
			m.requestVarChoiceCursor++
		}
	case "k", "up":
		if m.requestVarChoiceCursor > 0 {
			m.requestVarChoiceCursor--
		}
	case "enter":
		m = m.stopRequestVariableEdit("")
		return m.saveRequestVariable(row.Key, value.WithChosen(m.requestVarChoiceCursor)), nil
	case addChoiceKey:
		return m.startNewChoice(row)
	case choiceNoteKey:
		return m.startChoiceNote(row)
	case deleteChoiceKey:
		remaining := value.WithoutChoice(m.requestVarChoiceCursor)
		m = m.saveRequestVariable(row.Key, remaining)
		if !remaining.IsChoice() {
			m.requestVarEdit = requestVarEditNone
		}
		m.requestVarChoiceCursor = min(m.requestVarChoiceCursor, max(len(remaining.Choices)-1, 0))
	}
	return m, nil
}

func (m Model) startChoiceNote(row requestVariableRow) (Model, tea.Cmd) {
	m.editFieldInput.SetValue(row.variableValue().ChoiceNote(m.requestVarChoiceCursor))
	m.editFieldInput.CursorEnd()
	m.requestVarEdit = requestVarEditChoiceNote
	m.statusMessage = fmt.Sprintf("Note for {{%s}} = %s", row.Key, row.Choices[m.requestVarChoiceCursor])
	return m, m.editFieldInput.Focus()
}

// handleRequestVariableChoiceNoteKeys saves the typed note on the choice
// under the cursor and returns to the choices either way.
func (m Model) handleRequestVariableChoiceNoteKeys(msg tea.KeyMsg, row requestVariableRow) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m = m.stopRequestVariableEdit("Edit cancelled")
		m.requestVarEdit = requestVarEditChoices
		return m, nil
	case tea.KeyEnter:
		note := strings.TrimSpace(m.editFieldInput.Value())
		m = m.stopRequestVariableEdit("")
		m.requestVarEdit = requestVarEditChoices
		return m.saveRequestVariable(row.Key, row.variableValue().WithChoiceNote(m.requestVarChoiceCursor, note)), nil
	}

	var cmd tea.Cmd
	m.editFieldInput, cmd = m.editFieldInput.Update(msg)
	return m, cmd
}

func (m Model) handleRequestVariableNewChoiceKeys(msg tea.KeyMsg, row requestVariableRow) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m = m.stopRequestVariableEdit("Edit cancelled")
		if row.variableValue().IsChoice() {
			m.requestVarEdit = requestVarEditChoices
		}
		return m, nil
	case tea.KeyEnter:
		choice := m.editFieldInput.Value()
		if choice == "" {
			return m, nil
		}
		m = m.stopRequestVariableEdit("")
		return m.saveRequestVariable(row.Key, row.variableValue().WithChoice(choice)), nil
	}

	var cmd tea.Cmd
	m.editFieldInput, cmd = m.editFieldInput.Update(msg)
	return m, cmd
}

func (m Model) renderRequestVariables(availableHeight int) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	undefinedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))

	title := "Variables used by " + m.requestVarsItem.Name
	if m.requestVarsScope {
		title = "Variables of " + m.scopeLabel()
	}
	lines := []string{titleStyle.Render(title), ""}

	const keyColWidth = 30
	indent := strings.Repeat(" ", keyColWidth+4)
	for i, row := range m.requestVariableRows() {
		selected := i == m.requestVarsCursor
		prefix, style := "  ", normalItemStyle
		if selected {
			prefix, style = "> ", selectedItemStyle
		}

		value := style.Render(strings.ReplaceAll(row.Value, "\n", " "))
		if selected && m.requestVarEdit == requestVarEditText {
			value = m.editFieldInput.View()
		}
		if row.variableValue().IsChoice() {
			value += "  " + dimStyle.Render(choiceSummary(row.variableValue()))
			if note := row.ChoiceNotes[row.Value]; note != "" {
				value += " " + dimStyle.Render(notePrefix+note)
			}
		}
		lines = append(lines, prefix+style.Width(keyColWidth).Render("{{"+row.Key+"}}")+"  "+value)

		if selected && m.requestVarEdit.showsChoices() {
			lines = append(lines, m.choiceListLines(row, indent, dimStyle)...)
		}

		sourceStyle := dimStyle
		if !row.Defined {
			sourceStyle = undefinedStyle
		}
		if !m.requestVarsScope {
			lines = append(lines, indent+sourceStyle.Italic(true).Render(row.Source))
		}
	}
	if m.requestVarsScope && len(m.requestVariableRows()) == 0 && m.requestVarEdit != requestVarEditNewKey {
		lines = append(lines, dimStyle.Render(emptyScopeMessage))
	}
	if m.requestVarEdit == requestVarEditNewKey {
		lines = append(lines, "  "+newChoicePrefix+m.editFieldInput.View())
	}

	lines = append(lines, "", dimStyle.Render(strings.Join(formatShortcuts(m.requestVariableShortcuts()), "  ")))

	return mainWindowStyle.
		Height(availableHeight).
		Width(m.width - 4).
		Render(strings.Join(lines, "\n"))
}

func (state requestVarEditState) showsChoices() bool {
	return state == requestVarEditChoices || state == requestVarEditNewChoice || state == requestVarEditChoiceNote
}

// notedChoices lists the choices, each followed by its note when it has one.
func notedChoices(value postman.VariableValue) []string {
	labels := make([]string, len(value.Choices))
	for i, choice := range value.Choices {
		labels[i] = choice
		if note := value.ChoiceNote(i); note != "" {
			labels[i] += " (" + note + ")"
		}
	}
	return labels
}

// choiceSummary marks a choice variable with which of its choices is chosen.
func choiceSummary(value postman.VariableValue) string {
	return fmt.Sprintf("%s %d/%d", choiceMarker, value.ChoiceIndex()+1, len(value.Choices))
}

// choiceListLines is the dropdown of row's choices, with the new choice
// input below it while one is being typed.
func (m Model) choiceListLines(row requestVariableRow, indent string, dimStyle lipgloss.Style) []string {
	var lines []string
	for i, choice := range row.Choices {
		prefix, style := "  ", normalItemStyle
		underCursor := m.requestVarEdit != requestVarEditNewChoice && i == m.requestVarChoiceCursor
		if underCursor {
			prefix, style = "▸ ", selectedItemStyle
		}
		line := indent + prefix + style.Render(fmt.Sprintf("[%d] %s", i, choice))
		if choice == row.Value {
			line += " " + dimStyle.Render(chosenLabel)
		}
		switch {
		case underCursor && m.requestVarEdit == requestVarEditChoiceNote:
			line += " " + notePrefix + m.editFieldInput.View()
		case row.ChoiceNotes[choice] != "":
			line += " " + dimStyle.Render(notePrefix+row.ChoiceNotes[choice])
		}
		lines = append(lines, line)
	}
	if m.requestVarEdit == requestVarEditNewChoice {
		lines = append(lines, indent+newChoicePrefix+m.editFieldInput.View())
	}
	return lines
}
