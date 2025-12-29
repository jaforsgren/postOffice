package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderEditPopup(availableHeight int) string {
	lines := m.buildEditLines()
	content := strings.Join(lines, "\n")

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("11")).
		Height(availableHeight).
		Width(m.width-4).
		Padding(1, 2).
		Render(content)
}

func (m Model) buildEditLines() []string {
	var lines []string

	title := m.buildEditTitle()
	lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11")).Render(title))
	lines = append(lines, "")

	if m.scriptSelectionMode {
		lines = append(lines, m.buildScriptSelectionList()...)
		shortcuts := "<Enter> Select  <j/k> Navigate  <Esc> Cancel"
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(shortcuts))
		return lines
	}

	if m.editType == EditTypeScript {
		lines = append(lines, m.buildScriptEditor()...)
		shortcuts := "<:w> Save  <:wq> Save & Exit  <Esc> Cancel"
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(shortcuts))
		return lines
	}

	if m.editType == EditTypeRequest && m.editRequest != nil {
		lines = append(lines, m.buildEditFields()...)
	}

	lines = append(lines, "")
	shortcuts := "<Enter> Edit field  <j/k> Navigate  <Esc> Cancel  <:w> Save  <:wq> Save & Exit"
	lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(shortcuts))

	return lines
}

func (m Model) buildEditTitle() string {
	title := "Edit "
	switch m.editType {
	case EditTypeRequest:
		if m.editRequest != nil {
			title += "Request: " + m.editRequest.Method
		}
	case EditTypeEnvVariable:
		title += "Environment Variable"
	case EditTypeCollectionVariable:
		title += "Collection Variable"
	case EditTypeScript:
		if m.scriptSelectionMode {
			title = "Select Script Type - " + m.editScriptItemName
		} else {
			scriptTypeName := "Pre-request"
			if m.editScriptType == ScriptTypeTest {
				scriptTypeName = "Test"
			}
			title = scriptTypeName + " Script - " + m.editScriptItemName
		}
	}
	return title
}

func (m Model) buildEditFields() []string {
	var lines []string

	headersText := ""
	if len(m.editRequest.Header) > 0 {
		var headerLines []string
		for _, h := range m.editRequest.Header {
			headerLines = append(headerLines, h.Key+": "+h.Value)
		}
		headersText = strings.Join(headerLines, "\n")
	}

	fields := []struct {
		label string
		value string
	}{
		{"Name", m.editItemName},
		{"Method", m.editRequest.Method},
		{"URL", m.editRequest.URL.Raw},
		{"Headers", headersText},
		{"Body", ""},
	}

	if m.editRequest.Body != nil {
		fields[4].value = m.editRequest.Body.Raw
	}

	for i, field := range fields {
		prefix := "  "
		labelStyle := lipgloss.NewStyle()
		valueStyle := lipgloss.NewStyle()

		if i == m.editFieldCursor {
			prefix = "> "
			labelStyle = labelStyle.Bold(true).Foreground(lipgloss.Color("10"))
			valueStyle = valueStyle.Foreground(lipgloss.Color("12"))
		}

		lines = append(lines, prefix+labelStyle.Render(field.label+":"))

		displayValue := field.value
		if i == m.editFieldCursor && m.editFieldMode {
			if i >= 3 {
				displayValue = m.editFieldTextArea.View()
			} else {
				displayValue = m.editFieldInput.View()
			}
		}

		if displayValue == "" && !m.editFieldMode {
			displayValue = "(empty)"
		}

		valueLines := strings.Split(displayValue, "\n")
		for _, vLine := range valueLines {
			lines = append(lines, "    "+valueStyle.Render(vLine))
		}
		lines = append(lines, "")
	}

	return lines
}

func (m Model) buildScriptSelectionList() []string {
	var lines []string

	for i, option := range m.items {
		prefix := "  "
		style := lipgloss.NewStyle()

		if i == m.cursor {
			prefix = "> "
			style = style.Bold(true).Foreground(lipgloss.Color("10"))
		}

		lines = append(lines, prefix+style.Render(option))
	}

	return lines
}

func (m Model) buildScriptEditor() []string {
	var lines []string

	lineCount := strings.Count(m.editFieldTextArea.Value(), "\n") + 1
	lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(fmt.Sprintf("Lines: %d", lineCount)))
	lines = append(lines, "")
	lines = append(lines, m.editFieldTextArea.View())

	return lines
}
