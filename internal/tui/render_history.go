package tui

import (
	"fmt"
	"postOffice/internal/history"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	bytesPerKiB = 1024
	bytesPerMiB = bytesPerKiB * 1024

	// historyHeaderLines is the title line plus the blank line below it.
	historyHeaderLines = 2
	// windowChromeHeight is the vertical space taken by everything around the main window.
	windowChromeHeight = 8
)

func (m Model) renderHistoryView() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)

	entries := m.requestHistory.Entries()
	title := titleStyle.Render(fmt.Sprintf("Request History (%d/%d)", len(entries), history.MaxEntries))
	help := helpStyle.Render("(enter: replay | " + historyDiffKey + ": diff vs current | esc: back)")
	lines := []string{title + " " + help, ""}

	if len(entries) == 0 {
		lines = append(lines, helpStyle.Render("No requests executed yet."))
	} else {
		visibleLines := m.height - windowChromeHeight - historyHeaderLines
		startIdx, endIdx := calculateVisibleWindow(m.requestHistoryCursor, len(entries), visibleLines)
		for i := startIdx; i < endIdx; i++ {
			marker := "  "
			if i == m.requestHistoryCursor {
				marker = selectedStyle.Render("▶ ")
			}
			lines = append(lines, marker+formatHistoryEntry(entries[i]))
		}
	}

	return mainWindowStyle.
		Height(m.height - windowChromeHeight).
		Width(m.width - 4).
		Render(strings.Join(lines, "\n"))
}

func formatHistoryEntry(entry history.Entry) string {
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))

	snapshot := entry.Snapshot
	status := okStyle.Render(snapshot.Status)
	switch {
	case entry.Error != "":
		status = errStyle.Render("Error")
	case snapshot.StatusCode < 200 || snapshot.StatusCode >= 300:
		status = errStyle.Render(snapshot.Status)
	}

	environment := "-"
	if entry.EnvironmentName != "" {
		environment = entry.EnvironmentName
	}

	return fmt.Sprintf("%s  %-4s  %s  %s  %s  %s %s",
		snapshot.SavedAt.Format(timestampLayout),
		entry.Protocol,
		status,
		dimStyle.Render(fmt.Sprintf("(%dms, %s)", snapshot.DurationMS, formatByteSize(entry.ResponseSize()))),
		dimStyle.Render("["+environment+"]"),
		requestStyle.Render(entry.ItemName),
		dimStyle.Render(snapshot.RequestMethod+" "+snapshot.RequestURL),
	)
}

func formatByteSize(size int) string {
	switch {
	case size >= bytesPerMiB:
		return fmt.Sprintf("%.1fMB", float64(size)/bytesPerMiB)
	case size >= bytesPerKiB:
		return fmt.Sprintf("%.1fKB", float64(size)/bytesPerKiB)
	default:
		return fmt.Sprintf("%dB", size)
	}
}

func (m Model) renderHistoryDiffView() string {
	return mainWindowStyle.
		Height(m.height - windowChromeHeight).
		Width(m.width - 4).
		Render(m.historyDiffViewport.View())
}
