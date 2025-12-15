package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderVariablesView() string {
	metrics := m.calculateLayout()

	if len(m.variables) == 0 {
		return m.renderEmptyVariables(metrics.contentHeight)
	}

	lines := m.buildVariablesLines()
	content := strings.Join(lines, "\n")

	return mainWindowStyle.
		Height(metrics.contentHeight).
		Width(m.width - 4).
		Render(content)
}

func (m Model) renderEmptyVariables(availableHeight int) string {
	emptyMsg := "No variables defined.\n\n"
	emptyMsg += "Variables can be defined in:\n"
	emptyMsg += "  - Environments (highest priority)\n"
	emptyMsg += "  - Collections\n"
	emptyMsg += "  - Folders (inherited in hierarchy)\n\n"
	emptyMsg += "Load a collection or environment with variables to see them here."

	return mainWindowStyle.
		Height(availableHeight).
		Width(m.width - 4).
		Render(emptyMsg)
}

func (m Model) buildVariablesLines() []string {
	var lines []string

	lines = append(lines, lipgloss.NewStyle().Bold(true).Render("Variables"))
	lines = append(lines, "")
	lines = append(lines, m.buildVariablesTable()...)

	return lines
}

func (m Model) buildVariablesTable() []string {
	var lines []string

	keyColWidth := 30
	valueColWidth := m.width - keyColWidth - 10

	headerKey := lipgloss.NewStyle().Bold(true).Width(keyColWidth).Render("Variable")
	headerValue := lipgloss.NewStyle().Bold(true).Render("Value")
	lines = append(lines, headerKey+"  "+headerValue)
	lines = append(lines, strings.Repeat("─", m.width-6))

	for i, variable := range m.variables {
		keyStyle := normalItemStyle
		valueStyle := normalItemStyle
		prefix := "  "

		if i == m.cursor {
			keyStyle = selectedItemStyle
			valueStyle = selectedItemStyle
			prefix = "> "
		}

		key := keyStyle.Width(keyColWidth - 2).Render(variable.Key)
		value := m.formatVariableValue(variable.Value, i == m.cursor, valueColWidth, keyColWidth)

		line := prefix + key + "  " + valueStyle.Render(value)
		lines = append(lines, line)

		if i == m.cursor {
			sourceStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true)
			sourceLine := strings.Repeat(" ", keyColWidth) + sourceStyle.Render("Source: "+variable.Source)
			lines = append(lines, sourceLine)
			lines = append(lines, "")
		}
	}

	return lines
}

func (m Model) formatVariableValue(value string, isSelected bool, valueColWidth int, keyColWidth int) string {
	if isSelected {
		if len(value) > valueColWidth {
			valueLines := strings.Split(value, "\n")
			if len(valueLines) > 1 {
				value = valueLines[0]
				for j := 1; j < len(valueLines) && j < 5; j++ {
					value += "\n" + strings.Repeat(" ", keyColWidth) + valueLines[j]
				}
				if len(valueLines) > 5 {
					value += "\n" + strings.Repeat(" ", keyColWidth) + "..."
				}
			}
		}
	} else {
		if len(value) > valueColWidth {
			value = value[:valueColWidth-3] + "..."
		}
		value = strings.ReplaceAll(value, "\n", " ")
	}

	return value
}
