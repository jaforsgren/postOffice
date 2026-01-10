package tui

import (
	"fmt"
	"postOffice/internal/postman"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderInfoPopup(availableHeight int) string {
	if m.currentInfoItem == nil && m.environment == nil {
		return m.renderEmptyPopup("No item selected", availableHeight)
	}

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("5")).
		Height(availableHeight).
		Width(m.width-4).
		Padding(1, 2).
		Render(m.infoViewport.View())
}

func (m Model) renderInfoView() string {
	if m.currentInfoItem == nil && m.environment == nil {
		metrics := m.calculateLayout()
		return mainWindowStyle.
			Height(metrics.contentHeight).
			Width(m.width - 4).
			Render("No item selected")
	}

	return mainWindowStyle.
		Height(m.height-8).
		Width(m.width-4).
		Render(m.infoViewport.View())
}

func (m Model) buildEnvironmentInfoLines() []string {
	var lines []string

	lines = append(lines, lipgloss.NewStyle().Bold(true).Render("Environment: "+m.environment.Name+" (q: close)"))
	lines = append(lines, "")
	lines = append(lines, requestStyle.Render("ID: ")+m.environment.ID)
	lines = append(lines, "")

	if len(m.environment.Values) == 0 {
		lines = append(lines, "No variables defined")
		return lines
	}

	lines = append(lines, m.buildEnvironmentVariablesTable()...)
	return lines
}

func (m Model) buildEnvironmentVariablesTable() []string {
	var lines []string

	keyColWidth := 30
	valueColWidth := m.width - keyColWidth - 10

	headerKey := lipgloss.NewStyle().Bold(true).Width(keyColWidth).Render("Variable")
	headerValue := lipgloss.NewStyle().Bold(true).Render("Value")
	lines = append(lines, headerKey+"  "+headerValue)
	lines = append(lines, strings.Repeat("─", m.width-10))

	for i, variable := range m.environment.Values {
		keyStyle := normalItemStyle
		valueStyle := normalItemStyle
		prefix := "  "

		if i == m.envVarCursor {
			keyStyle = selectedItemStyle
			valueStyle = selectedItemStyle
			prefix = "> "
		}

		key := keyStyle.Width(keyColWidth - 2).Render(variable.Key)
		value := m.formatEnvironmentVariableValue(variable, i == m.envVarCursor, valueColWidth, keyColWidth)

		line := prefix + key + "  " + valueStyle.Render(value)
		lines = append(lines, line)

		if i == m.envVarCursor && !variable.Enabled {
			disabledStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true)
			lines = append(lines, strings.Repeat(" ", keyColWidth)+disabledStyle.Render("(disabled)"))
		}

		if i == m.envVarCursor {
			lines = append(lines, "")
		}
	}

	return lines
}

func (m Model) formatEnvironmentVariableValue(variable postman.EnvVariable, isSelected bool, valueColWidth int, keyColWidth int) string {
	value := variable.Value

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

func (m Model) buildItemInfoLines() []string {
	var lines []string

	lines = append(lines, lipgloss.NewStyle().Bold(true).Render("Item Info (q: close)"))
	lines = append(lines, "")

	lines = append(lines, requestStyle.Render("Name:"))
	lines = append(lines, "  "+m.currentInfoItem.Name)
	lines = append(lines, "")

	if m.currentInfoItem.IsFolder() {
		lines = append(lines, m.buildFolderInfoLines()...)
	} else if m.currentInfoItem.IsRequest() {
		lines = append(lines, m.buildRequestInfoLines()...)
	}

	return lines
}

func (m Model) buildFolderInfoLines() []string {
	var lines []string

	lines = append(lines, requestStyle.Render("Type:"))
	lines = append(lines, "  Folder")
	lines = append(lines, "")

	lines = append(lines, requestStyle.Render("Contents:"))
	folderCount := 0
	requestCount := 0
	for _, item := range m.currentInfoItem.Items {
		if item.IsFolder() {
			folderCount++
		} else if item.IsRequest() {
			requestCount++
		}
	}
	lines = append(lines, fmt.Sprintf("  Folders: %d", folderCount))
	lines = append(lines, fmt.Sprintf("  Requests: %d", requestCount))
	lines = append(lines, fmt.Sprintf("  Total: %d", len(m.currentInfoItem.Items)))
	lines = append(lines, "")

	if m.currentInfoItem.Description != "" {
		lines = append(lines, requestStyle.Render("Description:"))
		descLines := strings.Split(m.currentInfoItem.Description, "\n")
		for _, line := range descLines {
			lines = append(lines, "  "+line)
		}
	}

	return lines
}

func (m Model) buildRequestInfoLines() []string {
	var lines []string

	lines = append(lines, requestStyle.Render("Type:"))
	lines = append(lines, "  HTTP Request")
	lines = append(lines, "")

	req := m.currentInfoItem.Request
	variables := m.parser.GetAllVariables(m.collection, m.breadcrumb, m.environment)

	lines = append(lines, m.buildMethodSection(req)...)
	lines = append(lines, m.buildURLSection(req, variables)...)
	lines = append(lines, m.buildHeadersSection(req, variables)...)
	lines = append(lines, m.buildBodySection(req, variables)...)
	lines = append(lines, m.buildScriptsSection()...)
	lines = append(lines, m.buildDescriptionSection()...)

	return lines
}

func (m Model) buildMethodSection(req *postman.Request) []string {
	var lines []string
	lines = append(lines, requestStyle.Render("Method:"))
	lines = append(lines, "  "+req.Method)
	lines = append(lines, "")
	return lines
}

func (m Model) buildURLSection(req *postman.Request, variables []postman.VariableSource) []string {
	var lines []string

	lines = append(lines, requestStyle.Render("URL:"))
	url := req.URL.Raw
	if url == "" && len(req.URL.Host) > 0 {
		url = "https://" + strings.Join(req.URL.Host, ".")
		if len(req.URL.Path) > 0 {
			url += "/" + strings.Join(req.URL.Path, "/")
		}
	}
	lines = append(lines, "  "+url)

	resolvedURL := postman.ResolveVariables(url, variables)
	if url != resolvedURL {
		resolvedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
		lines = append(lines, "  → "+resolvedStyle.Render(resolvedURL))
	}
	lines = append(lines, "")

	return lines
}

func (m Model) buildHeadersSection(req *postman.Request, variables []postman.VariableSource) []string {
	var lines []string

	if len(req.Header) > 0 {
		lines = append(lines, requestStyle.Render("Headers:"))
		for _, header := range req.Header {
			originalValue := header.Value
			resolvedValue := postman.ResolveVariables(originalValue, variables)

			lines = append(lines, fmt.Sprintf("  %s: %s", header.Key, originalValue))
			if originalValue != resolvedValue {
				resolvedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
				lines = append(lines, "    → "+resolvedStyle.Render(resolvedValue))
			}
		}
		lines = append(lines, "")
	}

	return lines
}

func (m Model) buildBodySection(req *postman.Request, variables []postman.VariableSource) []string {
	var lines []string

	if req.Body != nil && req.Body.Raw != "" {
		lines = append(lines, requestStyle.Render("Body:"))
		lines = append(lines, fmt.Sprintf("  Mode: %s", req.Body.Mode))

		originalBody := req.Body.Raw
		resolvedBody := postman.ResolveVariables(originalBody, variables)

		if originalBody != resolvedBody {
			lines = append(lines, "")
			lines = append(lines, folderStyle.Render("  Template:"))
			lines = append(lines, m.formatBodyLines(originalBody, 5)...)

			lines = append(lines, "")
			resolvedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
			lines = append(lines, resolvedStyle.Render("  Resolved:"))
			lines = append(lines, m.formatBodyLines(resolvedBody, 5)...)
		} else {
			bodyLines := strings.Split(req.Body.Raw, "\n")
			for _, line := range bodyLines {
				lines = append(lines, "  "+line)
			}
		}
		lines = append(lines, "")
	}

	return lines
}

func (m Model) formatBodyLines(body string, maxLines int) []string {
	var lines []string
	bodyLines := strings.Split(body, "\n")
	for i, line := range bodyLines {
		if i >= maxLines {
			lines = append(lines, "    ...")
			break
		}
		lines = append(lines, "    "+line)
	}
	return lines
}

func (m Model) buildScriptsSection() []string {
	var lines []string

	if len(m.currentInfoItem.Events) == 0 {
		return lines
	}

	for _, event := range m.currentInfoItem.Events {
		if event.Listen == "prerequest" && len(event.Script.Exec) > 0 {
			lines = append(lines, requestStyle.Render("Pre-Request Scripts:"))
			scriptCode := strings.Join(event.Script.Exec, "\n")
			scriptLines := strings.Split(scriptCode, "\n")
			for i, line := range scriptLines {
				lineNum := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(fmt.Sprintf("%3d", i+1))
				lines = append(lines, fmt.Sprintf("  %s │ %s", lineNum, line))
			}
			lines = append(lines, "")
		}
	}

	for _, event := range m.currentInfoItem.Events {
		if event.Listen == "test" && len(event.Script.Exec) > 0 {
			lines = append(lines, requestStyle.Render("Test Scripts:"))
			scriptCode := strings.Join(event.Script.Exec, "\n")
			scriptLines := strings.Split(scriptCode, "\n")
			for i, line := range scriptLines {
				lineNum := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(fmt.Sprintf("%3d", i+1))
				lines = append(lines, fmt.Sprintf("  %s │ %s", lineNum, line))
			}
			lines = append(lines, "")
		}
	}

	return lines
}

func (m Model) buildDescriptionSection() []string {
	var lines []string

	if m.currentInfoItem.Description != "" {
		lines = append(lines, requestStyle.Render("Description:"))
		descLines := strings.Split(m.currentInfoItem.Description, "\n")
		for _, line := range descLines {
			lines = append(lines, "  "+line)
		}
	}

	return lines
}

func (m Model) renderJSONPopup(availableHeight int) string {
	if m.jsonContent == "" {
		return m.renderEmptyPopup("No JSON available", availableHeight)
	}

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("5")).
		Height(availableHeight).
		Width(m.width-4).
		Padding(1, 2).
		Render(m.jsonViewport.View())
}
