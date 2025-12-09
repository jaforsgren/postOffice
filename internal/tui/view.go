package tui

import (
	"fmt"
	"postOffice/internal/postman"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	var sections []string

	sections = append(sections, m.renderTopBar())

	topBarHeight := 7
	statusHeight := 2
	totalOverhead := topBarHeight + statusHeight

	if m.mode == ModeResponse && m.lastResponse != nil {
		availableForMain := m.height - totalOverhead
		mainHeight := availableForMain / 2
		responseHeight := availableForMain - mainHeight
		sections = append(sections, m.renderItemsList(mainHeight))
		sections = append(sections, m.renderResponsePopup(responseHeight))
	} else if m.mode == ModeInfo && m.currentInfoItem != nil {
		availableForMain := m.height - totalOverhead
		mainHeight := availableForMain / 2
		infoHeight := availableForMain - mainHeight
		sections = append(sections, m.renderItemsList(mainHeight))
		sections = append(sections, m.renderInfoPopup(infoHeight))
	} else if m.mode == ModeEdit {
		availableHeight := m.height - totalOverhead
		sections = append(sections, m.renderEditPopup(availableHeight))
	} else if m.mode == ModeVariables {
		sections = append(sections, m.renderVariablesView())
	} else {
		sections = append(sections, m.renderMainWindow())
	}

	sections = append(sections, m.renderCommandBar())
	sections = append(sections, m.renderStatusBar())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) renderTopBar() string {
	var lines []string

	collectionName := "none"
	if m.collection != nil {
		collectionName = m.collection.Info.Name
		if m.modifiedCollections[collectionName] {
			collectionName += " (*)"
		}
	}

	modeStr := ""
	switch m.mode {
	case ModeCollections:
		modeStr = "Collections"
	case ModeRequests:
		modeStr = "Requests"
	case ModeResponse:
		modeStr = "Response"
	case ModeInfo:
		modeStr = "Info"
	case ModeEnvironments:
		modeStr = "Environments"
	case ModeVariables:
		modeStr = "Variables"
	case ModeEdit:
		modeStr = "Edit (*)"
	}

	path := "/"
	if len(m.breadcrumb) > 0 {
		path = "/" + strings.Join(m.breadcrumb, "/")
	}
	if m.searchActive && m.searchQuery != "" {
		searchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
		path += searchStyle.Render(fmt.Sprintf(" [search: %s]", m.searchQuery))
	}

	leftCol := []string{
		fmt.Sprintf("Collection: %s", collectionName),
		fmt.Sprintf("Mode:       %s", modeStr),
		fmt.Sprintf("Path:       %s", path),
		fmt.Sprintf("Items:      %d", len(m.items)),
	}

	rightCol := []string{
		"PostOffice",
	}

	for i := 0; i < len(leftCol); i++ {
		line := leftCol[i]
		if i < len(rightCol) {
			padding := m.width - len(line) - len(rightCol[i]) - 2
			if padding < 0 {
				padding = 0
			}
			line += strings.Repeat(" ", padding) + rightCol[i]
		}
		lines = append(lines, line)
	}

	shortcuts := m.getContextualShortcuts()
	lines = append(lines, "")
	lines = append(lines, shortcuts)

	content := strings.Join(lines, "\n")
	return titleStyle.Width(m.width).Render(content)
}

func (m Model) getContextualShortcuts() string {
	var shortcuts []string

	switch m.mode {
	case ModeCollections:
		shortcuts = []string{
			"<enter> Select",
			"</> Search",
			"<:l> Load",
			"<:r> Requests",
			"<q> Quit",
		}
	case ModeRequests:
		shortcuts = []string{
			"<enter> Select/Execute",
			"<i> Info",
			"</> Search",
			"<esc/h> Back",
			"<j/k> Navigate",
			"<:c> Collections",
			"<:l> Load",
			"<q> Quit",
		}
	case ModeResponse:
		shortcuts = []string{
			"<esc> Close",
			"<j/k> Scroll",
			"<q> Quit",
		}
	case ModeInfo:
		shortcuts = []string{
			"<esc> Close",
			"<j/k> Scroll",
			"<q> Quit",
		}
	case ModeEnvironments:
		shortcuts = []string{
			"<enter> View Info",
			"<i> Info",
			"</> Search",
			"<:le> Load",
			"<:c> Collections",
			"<q> Quit",
		}
	case ModeVariables:
		shortcuts = []string{
			"<j/k> Navigate",
			"<:c> Collections",
			"<:e> Environments",
			"<:r> Requests",
			"<q> Quit",
		}
	}

	return strings.Join(shortcuts, "  ")
}

func (m Model) renderItemsList(availableHeight int) string {
	if len(m.items) == 0 {
		emptyMsg := "No items to display\n\n"
		emptyMsg += "Commands:\n"
		emptyMsg += "  :load <path> or :l <path>     - Load a Postman collection\n"
		emptyMsg += "  :loadenv <path> or :le <path> - Load a Postman environment\n"
		emptyMsg += "  :collections or :c            - List loaded collections\n"
		emptyMsg += "  :environments or :e           - List loaded environments\n"
		emptyMsg += "  :variables or :v              - List all variables with sources\n"
		emptyMsg += "  :requests or :r               - Browse requests in current collection\n"
		emptyMsg += "  / (in lists)                  - Search (recursive for requests)\n"
		emptyMsg += "  :help or :h or :?             - Show this help\n"
		emptyMsg += "  :quit or :q                   - Quit application\n"
		if m.collection != nil {
			emptyMsg += fmt.Sprintf("\n[Collection loaded: %s - %d items]\n", m.collection.Info.Name, len(m.collection.Items))
		}
		return mainWindowStyle.
			Height(availableHeight).
			Width(m.width - 4).
			Render(emptyMsg)
	}

	var lines []string

	if len(m.breadcrumb) > 0 {
		breadcrumbLine := "/ " + strings.Join(m.breadcrumb, " / ")
		lines = append(lines, folderStyle.Render(breadcrumbLine))
		lines = append(lines, "")
	}

	startIdx := 0
	endIdx := len(m.items)

	visibleLines := availableHeight - len(lines) - 1
	if len(m.items) > visibleLines {
		if m.cursor > visibleLines/2 {
			startIdx = m.cursor - visibleLines/2
			if startIdx+visibleLines > len(m.items) {
				startIdx = len(m.items) - visibleLines
			}
		}
		endIdx = startIdx + visibleLines
		if endIdx > len(m.items) {
			endIdx = len(m.items)
		}
	}

	for i := startIdx; i < endIdx; i++ {
		line := m.items[i]

		modifiedPrefix := ""
		if m.mode == ModeRequests && i < len(m.currentItems) {
			item := m.currentItems[i]
			if item.IsRequest() {
				itemID := m.getRequestIdentifier(item)
				if m.isItemModified(itemID) {
					modifiedPrefix = "* "
				}
			}
		}

		if i == m.cursor {
			line = "> " + modifiedPrefix + line
			lines = append(lines, selectedItemStyle.Render(line))
		} else {
			line = "  " + modifiedPrefix + line
			style := normalItemStyle
			if strings.HasPrefix(m.items[i], "[DIR]") {
				style = folderStyle
			} else if strings.HasPrefix(m.items[i], "[GET]") || strings.HasPrefix(m.items[i], "[POST]") ||
				strings.HasPrefix(m.items[i], "[PUT]") || strings.HasPrefix(m.items[i], "[DELETE]") {
				style = requestStyle
			}
			lines = append(lines, style.Render(line))
		}
	}

	content := strings.Join(lines, "\n")
	return mainWindowStyle.
		Height(availableHeight).
		Width(m.width - 4).
		Render(content)
}

func (m Model) renderMainWindow() string {
	topBarHeight := 7
	statusHeight := 2
	totalOverhead := topBarHeight + statusHeight
	availableHeight := m.height - totalOverhead
	return m.renderItemsList(availableHeight)
}

func (m Model) renderResponse(availableHeight int) string {
	if m.lastResponse == nil {
		return mainWindowStyle.
			Height(availableHeight).
			Width(m.width - 4).
			Render("No response available")
	}

	var lines []string

	if m.lastResponse.Error != nil {
		lines = append(lines, requestStyle.Render("Error:"))
		lines = append(lines, m.lastResponse.Error.Error())
		lines = append(lines, "")
	} else {
		lines = append(lines, requestStyle.Render(fmt.Sprintf("Status: %s", m.lastResponse.Status)))
		lines = append(lines, folderStyle.Render(fmt.Sprintf("Duration: %v", m.lastResponse.Duration)))
		lines = append(lines, "")

		if len(m.lastResponse.Headers) > 0 {
			lines = append(lines, requestStyle.Render("Headers:"))
			for key, values := range m.lastResponse.Headers {
				for _, value := range values {
					lines = append(lines, fmt.Sprintf("  %s: %s", key, value))
				}
			}
			lines = append(lines, "")
		}

		if m.lastResponse.Body != "" {
			lines = append(lines, requestStyle.Render("Body:"))
			bodyLines := strings.Split(m.lastResponse.Body, "\n")
			maxBodyLines := availableHeight - len(lines) - 3
			if len(bodyLines) > maxBodyLines {
				bodyLines = bodyLines[:maxBodyLines]
				bodyLines = append(bodyLines, "... (truncated)")
			}
			lines = append(lines, bodyLines...)
		}
	}

	content := strings.Join(lines, "\n")
	return mainWindowStyle.
		Height(availableHeight).
		Width(m.width - 4).
		Render(content)
}

func (m Model) renderResponsePopup(availableHeight int) string {
	if m.lastResponse == nil {
		return lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("5")).
			Height(availableHeight).
			Width(m.width-4).
			Padding(1, 2).
			Render("No response available")
	}

	var lines []string
	lines = append(lines, lipgloss.NewStyle().Bold(true).Render("Response (press Esc to close)"))
	lines = append(lines, "")

	lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Render("REQUEST"))
	lines = append(lines, requestStyle.Render(fmt.Sprintf("%s %s", m.lastResponse.RequestMethod, m.lastResponse.RequestURL)))

	if len(m.lastResponse.RequestHeaders) > 0 {
		lines = append(lines, "")
		lines = append(lines, folderStyle.Render("Request Headers:"))
		for key, value := range m.lastResponse.RequestHeaders {
			lines = append(lines, fmt.Sprintf("  %s: %s", key, value))
		}
	}

	if m.lastResponse.RequestBody != "" {
		lines = append(lines, "")
		lines = append(lines, folderStyle.Render("Request Body:"))
		bodyLines := strings.Split(m.lastResponse.RequestBody, "\n")
		for _, line := range bodyLines {
			if len(lines) < 10 {
				lines = append(lines, "  "+line)
			}
		}
		if len(bodyLines) > 8 {
			lines = append(lines, "  ...")
		}
	}

	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10")).Render("RESPONSE"))

	if m.lastResponse.Error != nil {
		lines = append(lines, requestStyle.Render("Error:"))
		lines = append(lines, m.lastResponse.Error.Error())
	} else {
		lines = append(lines, requestStyle.Render(fmt.Sprintf("Status: %s", m.lastResponse.Status)))
		lines = append(lines, folderStyle.Render(fmt.Sprintf("Duration: %v", m.lastResponse.Duration)))
		lines = append(lines, "")

		if len(m.lastResponse.Headers) > 0 {
			lines = append(lines, requestStyle.Render("Response Headers:"))
			for key, values := range m.lastResponse.Headers {
				for _, value := range values {
					lines = append(lines, fmt.Sprintf("  %s: %s", key, value))
				}
			}
			lines = append(lines, "")
		}

		if m.lastResponse.Body != "" {
			lines = append(lines, requestStyle.Render("Response Body:"))
			bodyLines := strings.Split(m.lastResponse.Body, "\n")
			lines = append(lines, bodyLines...)
		}
	}

	visibleLines := availableHeight - 2
	if visibleLines < 1 {
		visibleLines = 1
	}

	startIdx := m.scrollOffset
	if startIdx > len(lines) {
		startIdx = len(lines)
	}
	endIdx := startIdx + visibleLines
	if endIdx > len(lines) {
		endIdx = len(lines)
	}

	visibleContent := lines[startIdx:endIdx]
	content := strings.Join(visibleContent, "\n")

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("5")).
		Height(availableHeight).
		Width(m.width-4).
		Padding(1, 2).
		Render(content)
}

func (m Model) renderInfoPopup(availableHeight int) string {
	if m.currentInfoItem == nil && m.environment == nil {
		return lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("5")).
			Height(availableHeight).
			Width(m.width-4).
			Padding(1, 2).
			Render("No item selected")
	}

	var lines []string

	if m.environment != nil && m.previousMode == ModeEnvironments {
		lines = append(lines, lipgloss.NewStyle().Bold(true).Render("Environment: "+m.environment.Name+" (press Esc to close)"))
		lines = append(lines, "")

		lines = append(lines, requestStyle.Render("ID: ")+m.environment.ID)
		lines = append(lines, "")

		if len(m.environment.Values) == 0 {
			lines = append(lines, "No variables defined")
		} else {
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

				var value string
				if i == m.envVarCursor {
					value = variable.Value
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
					value = variable.Value
					if len(value) > valueColWidth {
						value = value[:valueColWidth-3] + "..."
					}
					value = strings.ReplaceAll(value, "\n", " ")
				}

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
		}
	} else if m.currentInfoItem != nil {
		lines = append(lines, lipgloss.NewStyle().Bold(true).Render("Item Info (press Esc to close)"))
		lines = append(lines, "")

		lines = append(lines, requestStyle.Render("Name:"))
		lines = append(lines, "  "+m.currentInfoItem.Name)
		lines = append(lines, "")

		if m.currentInfoItem.IsFolder() {
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
		} else if m.currentInfoItem.IsRequest() {
			lines = append(lines, requestStyle.Render("Type:"))
			lines = append(lines, "  HTTP Request")
			lines = append(lines, "")

			req := m.currentInfoItem.Request
			variables := m.parser.GetAllVariables(m.collection, m.breadcrumb, m.environment)

			lines = append(lines, requestStyle.Render("Method:"))
			lines = append(lines, "  "+req.Method)
			lines = append(lines, "")

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

			if req.Body != nil && req.Body.Raw != "" {
				lines = append(lines, requestStyle.Render("Body:"))
				lines = append(lines, fmt.Sprintf("  Mode: %s", req.Body.Mode))

				originalBody := req.Body.Raw
				resolvedBody := postman.ResolveVariables(originalBody, variables)

				if originalBody != resolvedBody {
					lines = append(lines, "")
					lines = append(lines, folderStyle.Render("  Template:"))
					bodyLines := strings.Split(originalBody, "\n")
					maxLines := 5
					for i, line := range bodyLines {
						if i >= maxLines {
							lines = append(lines, "    ...")
							break
						}
						lines = append(lines, "    "+line)
					}

					lines = append(lines, "")
					resolvedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
					lines = append(lines, resolvedStyle.Render("  Resolved:"))
					resolvedLines := strings.Split(resolvedBody, "\n")
					for i, line := range resolvedLines {
						if i >= maxLines {
							lines = append(lines, "    ...")
							break
						}
						lines = append(lines, "    "+line)
					}
				} else {
					bodyLines := strings.Split(req.Body.Raw, "\n")
					for _, line := range bodyLines {
						lines = append(lines, "  "+line)
					}
				}
				lines = append(lines, "")
			}

			if m.currentInfoItem.Description != "" {
				lines = append(lines, requestStyle.Render("Description:"))
				descLines := strings.Split(m.currentInfoItem.Description, "\n")
				for _, line := range descLines {
					lines = append(lines, "  "+line)
				}
			}
		}
	}

	var content string
	if m.environment != nil && m.previousMode == ModeEnvironments {
		content = strings.Join(lines, "\n")
	} else {
		visibleLines := availableHeight - 2
		if visibleLines < 1 {
			visibleLines = 1
		}

		startIdx := m.scrollOffset
		if startIdx > len(lines) {
			startIdx = len(lines)
		}
		endIdx := startIdx + visibleLines
		if endIdx > len(lines) {
			endIdx = len(lines)
		}

		visibleContent := lines[startIdx:endIdx]
		content = strings.Join(visibleContent, "\n")
	}

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("5")).
		Height(availableHeight).
		Width(m.width-4).
		Padding(1, 2).
		Render(content)
}

func (m Model) renderVariablesView() string {
	topBarHeight := 7
	statusHeight := 2
	totalOverhead := topBarHeight + statusHeight
	availableHeight := m.height - totalOverhead

	if len(m.variables) == 0 {
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

	var lines []string
	lines = append(lines, lipgloss.NewStyle().Bold(true).Render("Variables"))
	lines = append(lines, "")

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

		var value string
		if i == m.cursor {
			value = variable.Value
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
			value = variable.Value
			if len(value) > valueColWidth {
				value = value[:valueColWidth-3] + "..."
			}
			value = strings.ReplaceAll(value, "\n", " ")
		}

		line := prefix + key + "  " + valueStyle.Render(value)
		lines = append(lines, line)

		if i == m.cursor {
			sourceStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true)
			sourceLine := strings.Repeat(" ", keyColWidth) + sourceStyle.Render("Source: "+variable.Source)
			lines = append(lines, sourceLine)
			lines = append(lines, "")
		}
	}

	content := strings.Join(lines, "\n")
	return mainWindowStyle.
		Height(availableHeight).
		Width(m.width - 4).
		Render(content)
}

func (m Model) renderEditPopup(availableHeight int) string {
	var lines []string

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
	}

	lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11")).Render(title))
	lines = append(lines, "")

	if m.editType == EditTypeRequest && m.editRequest != nil {
		fields := []struct {
			label string
			value string
		}{
			{"Method", m.editRequest.Method},
			{"URL", m.editRequest.URL.Raw},
			{"Body", ""},
		}

		if m.editRequest.Body != nil {
			fields[2].value = m.editRequest.Body.Raw
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
				displayValue = m.editFieldInput + "|"
			}

			if displayValue == "" {
				displayValue = "(empty)"
			}

			valueLines := strings.Split(displayValue, "\n")
			maxLines := 3
			for j, vLine := range valueLines {
				if j >= maxLines {
					lines = append(lines, "    ...")
					break
				}
				lines = append(lines, "    "+valueStyle.Render(vLine))
			}
			lines = append(lines, "")
		}
	}

	lines = append(lines, "")
	shortcuts := "<Enter> Edit field  <j/k> Navigate  <Esc> Cancel  <:w> Save  <:wq> Save & Exit"
	lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(shortcuts))

	content := strings.Join(lines, "\n")

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("11")).
		Height(availableHeight).
		Width(m.width-4).
		Padding(1, 2).
		Render(content)
}

func (m Model) renderCommandBar() string {
	if m.commandMode {
		return commandBarStyle.Width(m.width).Render(":" + m.commandInput)
	}
	if m.searchMode {
		return commandBarStyle.Width(m.width).Render("/" + m.searchQuery)
	}
	return commandBarStyle.Width(m.width).Render("")
}

func (m Model) renderStatusBar() string {
	help := "q: quit | ↑↓/jk: navigate | enter: select | backspace/h: back | /: search | :: command"
	if m.statusMessage != "" {
		help = m.statusMessage + " | " + help
	}
	return statusBarStyle.Width(m.width).Render(help)
}
