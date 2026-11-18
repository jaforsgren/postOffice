package tui

import (
	"encoding/json"
	"fmt"
	"maps"
	"postOffice/internal/http"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderResponsePopup(availableHeight int) string {
	if m.lastResponse == nil {
		return m.renderEmptyPopup("No response available", availableHeight)
	}

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(responseBorderColor(m.lastResponse)).
		Height(availableHeight).
		Width(m.width-4).
		Padding(1, 2).
		Render(m.responseViewport.View())
}

func responseBorderColor(response *http.Response) lipgloss.Color {
	if response != nil && (response.StatusCode < 200 || response.StatusCode >= 300) {
		return errorBorderColor
	}
	return popupBorderColor
}

func (m Model) renderResponseView() string {
	if m.lastResponse == nil && m.responsePanelItem == nil {
		metrics := m.calculateLayout()
		return mainWindowStyle.
			Height(metrics.contentHeight).
			Width(m.width - 4).
			Render("No response available. Execute a request with ctrl+e first.")
	}

	return mainWindowStyle.
		Height(m.height - 8).
		Width(m.width - 4).
		Render(m.responseViewport.View())
}

// buildResponseLines is the exchange with what came back first: the
// response, its test results, then the request as it was sent.
func (m Model) buildResponseLines() []string {
	lines := m.buildResponseSection()

	if m.lastTestResult != nil &&
		(len(m.lastTestResult.Tests) > 0 || len(m.lastTestResult.Errors) > 0 || len(m.lastTestResult.ConsoleLogs) > 0) {
		lines = append(lines, "")
		lines = append(lines, m.buildTestResultsSection()...)
	}

	lines = append(lines, "")
	lines = append(lines, m.buildRequestSection()...)
	return append(lines, m.relatedMessagesSection()...)
}

func (m Model) buildRequestSection() []string {
	var lines []string

	lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Render("REQUEST"))
	lines = append(lines, requestStyle.Render(fmt.Sprintf("%s %s", m.lastResponse.RequestMethod, m.lastResponse.RequestURL)))

	if len(m.lastResponse.RequestHeaders) > 0 {
		lines = append(lines, "")
		lines = append(lines, folderStyle.Render("Request Headers:"))
		for _, key := range slices.Sorted(maps.Keys(m.lastResponse.RequestHeaders)) {
			lines = append(lines, fmt.Sprintf("  %s: %s", key, m.lastResponse.RequestHeaders[key]))
		}
	}

	if m.lastResponse.RequestBody != "" {
		lines = append(lines, "")
		lines = append(lines, folderStyle.Render("Request Body:"))
		for _, line := range prettyBodyLines(m.lastResponse.RequestBody) {
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
			for _, key := range slices.Sorted(maps.Keys(m.lastResponse.Headers)) {
				for _, value := range m.lastResponse.Headers[key] {
					lines = append(lines, fmt.Sprintf("  %s: %s", key, value))
				}
			}
			lines = append(lines, "")
		}

		if m.lastResponse.Body != "" {
			lines = append(lines, requestStyle.Render("Response Body:"))
			lines = append(lines, prettyBodyLines(m.lastResponse.Body)...)
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

	if len(m.lastTestResult.ConsoleLogs) > 0 {
		lines = append(lines, "")
		logLabelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Bold(true)
		logStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
		lines = append(lines, logLabelStyle.Render("Console:"))
		for _, log := range m.lastTestResult.ConsoleLogs {
			lines = append(lines, logStyle.Render("  "+log))
		}
	}

	return lines
}

func prettyBodyLines(body string) []string {
	var js interface{}
	if err := json.Unmarshal([]byte(body), &js); err == nil {
		if pretty, err := json.MarshalIndent(js, "", "  "); err == nil {
			return strings.Split(string(pretty), "\n")
		}
	}
	return strings.Split(body, "\n")
}

func (m Model) renderEmptyPopup(message string, availableHeight int) string {
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(popupBorderColor).
		Height(availableHeight).
		Width(m.width-4).
		Padding(1, 2).
		Render(message)
}
