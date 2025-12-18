package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderResponsePopup(availableHeight int) string {
	if m.lastResponse == nil {
		return m.renderEmptyPopup("No response available", availableHeight)
	}

	lines := m.buildResponseLines()
	visibleContent := sliceContent(lines, m.scrollOffset, availableHeight-2)
	content := strings.Join(visibleContent, "\n")

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("5")).
		Height(availableHeight).
		Width(m.width-4).
		Padding(1, 2).
		Render(content)
}

func (m Model) buildResponseLines() []string {
	var lines []string

	lines = append(lines, lipgloss.NewStyle().Bold(true).Render("Response (press Esc to close)"))
	lines = append(lines, "")

	lines = append(lines, m.buildRequestSection()...)
	lines = append(lines, "")
	lines = append(lines, m.buildResponseSection()...)

	return lines
}

func (m Model) buildRequestSection() []string {
	var lines []string

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
		maxPreviewLines := 666
		for i, line := range bodyLines {
			if i >= maxPreviewLines {
				lines = append(lines, "  ...")
				break
			}
			lines = append(lines, "  "+line)
		}
	}

	return lines
}

func (m Model) buildResponseSection() []string {
	var lines []string

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

	return lines
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

func (m Model) renderEmptyPopup(message string, availableHeight int) string {
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("5")).
		Height(availableHeight).
		Width(m.width-4).
		Padding(1, 2).
		Render(message)
}
