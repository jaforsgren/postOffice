package tui

import (
	"fmt"
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
	}

	modeStr := ""
	switch m.mode {
	case ModeCollections:
		modeStr = "Collections"
	case ModeRequests:
		modeStr = "Requests"
	case ModeResponse:
		modeStr = "Response"
	}

	path := "/"
	if len(m.breadcrumb) > 0 {
		path = "/" + strings.Join(m.breadcrumb, "/")
	}

	leftCol := []string{
		fmt.Sprintf("Collection: %s", collectionName),
		fmt.Sprintf("Mode:       %s", modeStr),
		fmt.Sprintf("Path:       %s", path),
		fmt.Sprintf("Items:      %d", len(m.items)),
	}

	rightCol := []string{
		"  ____             __  ____   _____ _         ",
		" / __ \\____  _____/ /_/ __ \\ / __(_) _______ ",
		"/ /_/ / __ \\/ ___/ __/ / / // /_/ / / ___/ _ \\",
		"/ ____/ /_/ (__  ) /_/ /_/ / __/ / / /__/  __/",
		"/_/    \\____/____/\\__/\\____/_/ /_/_/\\___/\\___/ ",
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
			"<:l> Load",
			"<:r> Requests",
			"<q> Quit",
		}
	case ModeRequests:
		shortcuts = []string{
			"<enter> Select/Execute",
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
	}

	return strings.Join(shortcuts, "  ")
}

func (m Model) renderItemsList(availableHeight int) string {
	if len(m.items) == 0 {
		emptyMsg := "No items to display\n\n"
		emptyMsg += "Commands:\n"
		emptyMsg += "  :load <path> or :l <path>  - Load a Postman collection\n"
		emptyMsg += "  :collections or :c         - List loaded collections\n"
		emptyMsg += "  :requests or :r            - Browse requests in current collection\n"
		emptyMsg += "  :help or :h or :?          - Show this help\n"
		emptyMsg += "  :quit or :q                - Quit application\n"
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
		if i == m.cursor {
			line = "> " + line
			lines = append(lines, selectedItemStyle.Render(line))
		} else {
			line = "  " + line
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

	if m.lastResponse.Error != nil {
		lines = append(lines, requestStyle.Render("Error:"))
		lines = append(lines, m.lastResponse.Error.Error())
	} else {
		lines = append(lines, requestStyle.Render(fmt.Sprintf("Status: %s", m.lastResponse.Status)))
		lines = append(lines, folderStyle.Render(fmt.Sprintf("Duration: %v", m.lastResponse.Duration)))
		lines = append(lines, "")

		if len(m.lastResponse.Headers) > 0 && availableHeight > 15 {
			lines = append(lines, requestStyle.Render("Headers:"))
			headerCount := 0
			for key, values := range m.lastResponse.Headers {
				if headerCount >= 5 {
					lines = append(lines, "  ... (more headers)")
					break
				}
				for _, value := range values {
					lines = append(lines, fmt.Sprintf("  %s: %s", key, value))
					headerCount++
				}
			}
			lines = append(lines, "")
		}

		if m.lastResponse.Body != "" {
			lines = append(lines, requestStyle.Render("Body:"))
			bodyLines := strings.Split(m.lastResponse.Body, "\n")
			maxBodyLines := availableHeight - len(lines) - 5
			if maxBodyLines < 1 {
				maxBodyLines = 1
			}
			if len(bodyLines) > maxBodyLines {
				bodyLines = bodyLines[:maxBodyLines]
				bodyLines = append(bodyLines, "... (truncated)")
			}
			lines = append(lines, bodyLines...)
		}
	}

	content := strings.Join(lines, "\n")
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("5")).
		Height(availableHeight).
		Width(m.width-4).
		Padding(1, 2).
		Render(content)
}

func (m Model) renderCommandBar() string {
	if m.commandMode {
		return commandBarStyle.Width(m.width).Render(":" + m.commandInput)
	}
	return commandBarStyle.Width(m.width).Render("")
}

func (m Model) renderStatusBar() string {
	help := "q: quit | ↑↓/jk: navigate | enter: select | backspace/h: back | :: command"
	if m.statusMessage != "" {
		help = m.statusMessage + " | " + help
	}
	return statusBarStyle.Width(m.width).Render(help)
}
