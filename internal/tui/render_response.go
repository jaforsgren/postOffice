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

	borderColor := lipgloss.Color("5")
	if m.lastResponse.StatusCode < 200 || m.lastResponse.StatusCode >= 300 {
		borderColor = lipgloss.Color("1")
	}

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Height(availableHeight).
		Width(m.width-4).
		Padding(1, 2).
		Render(m.responseViewport.View())
}

func (m Model) buildResponseLines() []string {
	var lines []string

	lines = append(lines, lipgloss.NewStyle().Bold(true).Render("Response (press Esc to close)"))
	lines = append(lines, "")

	lines = append(lines, m.buildRequestSection()...)
	lines = append(lines, "")
	lines = append(lines, m.buildResponseSection()...)

	if m.lastTestResult != nil && (len(m.lastTestResult.Tests) > 0 || len(m.lastTestResult.Errors) > 0) {
		lines = append(lines, "")
		lines = append(lines, m.buildTestResultsSection()...)
	}

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
		statusStyle := requestStyle
		if m.lastResponse.StatusCode < 200 || m.lastResponse.StatusCode >= 300 {
			statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
		}
		lines = append(lines, statusStyle.Render(fmt.Sprintf("Status: %s", m.lastResponse.Status)))
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
		statusStyle := requestStyle
		if m.lastResponse.StatusCode < 200 || m.lastResponse.StatusCode >= 300 {
			statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
		}
		lines = append(lines, statusStyle.Render(fmt.Sprintf("Status: %s", m.lastResponse.Status)))
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

func (m Model) buildTestResultsSection() []string {
	var lines []string

	lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14")).Render("TEST RESULTS"))
	lines = append(lines, "")

	passedCount := 0
	failedCount := 0
	for _, test := range m.lastTestResult.Tests {
		if test.Passed {
			passedCount++
		} else {
			failedCount++
		}
	}

	summaryStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	if failedCount > 0 {
		summaryStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	}
	lines = append(lines, summaryStyle.Render(fmt.Sprintf("Tests: %d passed, %d failed, %d total", passedCount, failedCount, len(m.lastTestResult.Tests))))
	lines = append(lines, "")

	for _, test := range m.lastTestResult.Tests {
		icon := "✓"
		testStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
		if !test.Passed {
			icon = "✗"
			testStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
		}

		lines = append(lines, testStyle.Render(fmt.Sprintf("  %s %s", icon, test.Name)))
		if test.Error != "" {
			errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
			errorLines := strings.Split(test.Error, "\n")
			for _, errLine := range errorLines {
				lines = append(lines, errorStyle.Render("      "+errLine))
			}
		}
	}

	if len(m.lastTestResult.Errors) > 0 {
		lines = append(lines, "")
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
		lines = append(lines, errorStyle.Render("Script Errors:"))
		for _, err := range m.lastTestResult.Errors {
			lines = append(lines, errorStyle.Render("  • "+err))
		}
	}

	return lines
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
