package tui

import (
	"fmt"
	"postOffice/internal/http"
	"postOffice/internal/timing"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	timingKey = "t"

	timingMaxBarWidth = 40
	timingMinBarWidth = 10
	timingNameWidth   = 24
	timingValueWidth  = 12
	// border (2) + horizontal padding (4) + column gaps (3)
	timingPopupChrome = 9

	timingBarFill  = "█"
	timingBarTrack = "·"

	timingDisplayPrecision = time.Microsecond

	timingTotalLabel         = "Total"
	timingReusedLabel        = "reused"
	timingNotApplicableLabel = "n/a"
)

func handleTimingKey(m Model) (Model, tea.Cmd) {
	if m.mode == ModeTiming {
		return handleBackKey(m)
	}
	return m.openTiming()
}

func handleTimingCommand(m Model, _ []string) (Model, tea.Cmd) {
	return m.openTiming()
}

func (m Model) openTiming() (Model, tea.Cmd) {
	if m.mode != ModeResponse {
		m.statusMessage = "Timing is available from the response view (ctrl+r)"
		return m, nil
	}
	if m.lastResponse == nil {
		m.statusMessage = "No response available. Execute a request with ctrl+e first."
		return m, nil
	}
	m.mode = ModeTiming
	m.statusMessage = fmt.Sprintf("Request timing  (%s/q/esc close)", timingKey)
	return m, nil
}

func (m Model) renderTimingPopup() string {
	metrics := m.calculateLayout()
	popup := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("5")).
		Padding(1, 2).
		Render(strings.Join(buildTimingLines(m.lastResponse, timingBarWidthFor(m.width)), "\n"))

	return lipgloss.Place(m.width, metrics.contentHeight, lipgloss.Center, lipgloss.Center, popup)
}

// timingBarWidthFor shrinks the waterfall to fit narrow terminals while keeping it readable.
func timingBarWidthFor(terminalWidth int) int {
	available := terminalWidth - timingPopupChrome - timingNameWidth - timingValueWidth
	return max(timingMinBarWidth, min(timingMaxBarWidth, available))
}

func buildTimingLines(resp *http.Response, barWidth int) []string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	phaseBarStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	totalBarStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))

	lines := []string{
		titleStyle.Render("Request Timing"),
		subtleStyle.Render(fmt.Sprintf("%s %s", resp.RequestMethod, resp.RequestURL)),
		"",
	}

	if len(resp.Timing) == 0 {
		lines = append(lines, subtleStyle.Render("No phase breakdown available for this request"))
	}
	for _, phase := range resp.Timing {
		lines = append(lines, formatTimingRow(phase.Name, phaseValue(phase), phaseBarStyle.Render(phaseBar(phase, resp.Duration, barWidth))))
	}

	lines = append(lines, "")
	totalBar := waterfallBar(0, resp.Duration, resp.Duration, barWidth)
	lines = append(lines, formatTimingRow(timingTotalLabel, formatTimingDuration(resp.Duration), totalBarStyle.Render(totalBar)))

	if resp.Error != nil {
		lines = append(lines, "", subtleStyle.Render("Request failed; phases after the failure were not reached"))
	}

	return lines
}

func formatTimingRow(name, value, bar string) string {
	return fmt.Sprintf("%-*s %*s  %s", timingNameWidth, name, timingValueWidth, value, bar)
}

func phaseValue(phase timing.Phase) string {
	switch phase.Status {
	case timing.StatusReused:
		return timingReusedLabel
	case timing.StatusNotApplicable:
		return timingNotApplicableLabel
	default:
		return formatTimingDuration(phase.Duration)
	}
}

func phaseBar(phase timing.Phase, total time.Duration, width int) string {
	if phase.Status != timing.StatusMeasured {
		return strings.Repeat(timingBarTrack, width)
	}
	return waterfallBar(phase.Offset, phase.Duration, total, width)
}

func formatTimingDuration(d time.Duration) string {
	return d.Round(timingDisplayPrecision).String()
}

func waterfallBar(offset, duration, total time.Duration, width int) string {
	start, length := waterfallSpan(offset, duration, total, width)
	return strings.Repeat(timingBarTrack, start) +
		strings.Repeat(timingBarFill, length) +
		strings.Repeat(timingBarTrack, width-start-length)
}

// waterfallSpan maps a phase onto bar cells proportionally to total.
// Any phase with a non-zero duration gets at least one cell so short phases stay visible.
func waterfallSpan(offset, duration, total time.Duration, width int) (start, length int) {
	if total <= 0 || width <= 0 {
		return 0, 0
	}

	start = min(max(int(int64(offset)*int64(width)/int64(total)), 0), width-1)
	length = int((int64(duration)*int64(width) + int64(total)/2) / int64(total))
	if duration > 0 {
		length = max(length, 1)
	}
	length = min(length, width-start)

	return start, length
}
