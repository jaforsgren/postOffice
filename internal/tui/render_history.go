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

	historyHelp = "enter: replay | " + historyDiffKey + ": diff vs current | " + deleteHistoryEntryKey + ": delete"
)

func (m Model) renderHistoryView() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)

	entries := m.visibleHistoryEntries()
	scope := m.requestHistoryScope
	heading := fmt.Sprintf("Request History (%d/%d)", len(entries), history.MaxEntries)
	emptyMessage := "No requests executed yet."
	helpKeys := historyHelp
	if !scope.isAll() {
		heading = fmt.Sprintf("Request History: %s (%d)", scope.itemName, len(entries))
		emptyMessage = fmt.Sprintf("No history for %s.", scope.itemName)
		helpKeys += " | " + allHistoryCommand + ": all requests"
	}
	lines := []string{titleStyle.Render(heading) + " " + helpStyle.Render("("+helpKeys+" | esc: back)"), ""}

	if len(entries) == 0 {
		lines = append(lines, helpStyle.Render(emptyMessage))
	} else {
		visibleLines := m.mainWindowRows() - historyHeaderLines
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
		Height(m.calculateLayout().contentHeight).
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
		Height(m.calculateLayout().contentHeight).
		Width(m.width - 4).
		Render(m.historyDiffViewport.View())
}
