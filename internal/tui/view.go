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
	sections = append(sections, m.renderMainWindow())
	sections = append(sections, m.renderCommandBar())
	sections = append(sections, m.renderStatusBar())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) renderTopBar() string {
	title := " PostOffice "
	if m.collection != nil {
		title += fmt.Sprintf("- %s ", m.collection.Info.Name)
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

	title += fmt.Sprintf("[%s] ", modeStr)

	return titleStyle.Width(m.width).Render(title)
}

func (m Model) renderMainWindow() string {
	availableHeight := m.height - 4

	if m.mode == ModeResponse {
		return m.renderResponse(availableHeight)
	}

	if len(m.items) == 0 {
		emptyMsg := "No items to display\n\n"
		emptyMsg += "Commands:\n"
		emptyMsg += "  :load <path>    - Load a Postman collection\n"
		emptyMsg += "  :collections    - List loaded collections\n"
		emptyMsg += "  :request        - Browse requests in current collection\n"
		return lipgloss.NewStyle().
			Height(availableHeight).
			Width(m.width).
			Padding(1, 2).
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
	return lipgloss.NewStyle().
		Height(availableHeight).
		Width(m.width).
		Padding(1, 2).
		Render(content)
}

func (m Model) renderResponse(availableHeight int) string {
	if m.lastResponse == nil {
		return lipgloss.NewStyle().
			Height(availableHeight).
			Width(m.width).
			Padding(1, 2).
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
	return lipgloss.NewStyle().
		Height(availableHeight).
		Width(m.width).
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
