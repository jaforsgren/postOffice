package tui

import (
	"context"
	"errors"
	"fmt"
	"postOffice/internal/history"
	internalhttp "postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/textdiff"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	statusSending   = "Sending..."
	timestampLayout = "2006-01-02 15:04:05"

	requestHistoryKey = "ctrl+y"
	historyDiffKey    = "="
)

func (m Model) environmentName() string {
	if m.environment == nil {
		return ""
	}
	return m.environment.Name
}

func newSavedResponse(resp *internalhttp.Response, savedAt time.Time) postman.SavedResponse {
	return postman.SavedResponse{
		SavedAt:         savedAt,
		RequestMethod:   resp.RequestMethod,
		RequestURL:      resp.RequestURL,
		RequestHeaders:  resp.RequestHeaders,
		RequestBody:     resp.RequestBody,
		StatusCode:      resp.StatusCode,
		Status:          resp.Status,
		ResponseHeaders: resp.Headers,
		Body:            resp.Body,
		DurationMS:      resp.Duration.Milliseconds(),
	}
}

func newHistoryEntry(msg RequestCompleteMsg, executedAt time.Time) history.Entry {
	entry := history.Entry{
		ItemID:          msg.ItemID,
		ItemName:        msg.ItemName,
		Protocol:        msg.Protocol,
		EnvironmentName: msg.EnvironmentName,
		Snapshot:        newSavedResponse(msg.Response, executedAt),
	}
	if msg.Response.Error != nil {
		entry.Error = msg.Response.Error.Error()
	}
	return entry
}

// historyEntryResponse restores the response of entry, including a recorded error message.
func historyEntryResponse(entry history.Entry) *internalhttp.Response {
	resp := savedResponseToHTTPResponse(entry.Snapshot)
	if entry.Error != "" {
		resp.Error = errors.New(entry.Error)
	}
	return resp
}

// recordHistory adds the completed execution and keeps the history cursor on the
// entry it pointed at, since new entries are inserted at the top.
func (m Model) recordHistory(msg RequestCompleteMsg) Model {
	if err := m.requestHistory.Add(newHistoryEntry(msg, time.Now())); err != nil {
		m.statusMessage = fmt.Sprintf("Warning: failed to save request history: %v", err)
	}
	if m.requestHistory.Len() > 1 {
		m.requestHistoryCursor = min(m.requestHistoryCursor+1, m.requestHistory.Len()-1)
	}
	return m
}

func (m Model) openRequestHistory() (Model, tea.Cmd) {
	if m.mode != ModeHistory && m.mode != ModeHistoryDiff {
		m.historyReturnMode = m.mode
	}
	m.mode = ModeHistory
	m.requestHistoryCursor = 0
	m.statusMessage = fmt.Sprintf("%d request(s) in history", m.requestHistory.Len())
	return m, nil
}

func (m Model) closeRequestHistory() (Model, tea.Cmd) {
	m.mode = m.historyReturnMode
	if m.mode == modeUnset {
		m.mode = ModeRequests
	}
	m.statusMessage = "Closed request history"
	return m, nil
}

func (m Model) selectedHistoryEntry() (history.Entry, bool) {
	entries := m.requestHistory.Entries()
	if m.requestHistoryCursor >= len(entries) {
		return history.Entry{}, false
	}
	return entries[m.requestHistoryCursor], true
}

// replayHistoryEntry re-sends the recorded resolved request as-is: no variable
// resolution against the current environment and no pre-request or test scripts.
func (m Model) replayHistoryEntry() (Model, tea.Cmd) {
	entry, ok := m.selectedHistoryEntry()
	if !ok {
		m.statusMessage = "No history entry selected"
		return m, nil
	}

	execution, err := m.engine.PrepareReplay(entry)
	if err != nil {
		m.statusMessage = fmt.Sprintf("Cannot replay %s: %v", entry.ItemName, err)
		return m, nil
	}

	m.requestExecutions[entry.ItemID] = &RequestExecution{
		Status:    statusSending,
		Timestamp: time.Now(),
	}
	m.statusMessage = fmt.Sprintf("Replaying: %s %s", entry.Snapshot.RequestMethod, entry.ItemName)

	return m, func() tea.Msg {
		return RequestCompleteMsg{
			ItemID:          entry.ItemID,
			Response:        execution.Run(context.Background()).Response,
			EnvironmentName: entry.EnvironmentName,
			Protocol:        execution.Protocol(),
			ItemName:        entry.ItemName,
			Replayed:        true,
		}
	}
}

func (m Model) showHistoryDiff() (Model, tea.Cmd) {
	entry, ok := m.selectedHistoryEntry()
	if !ok {
		m.statusMessage = "No history entry selected"
		return m, nil
	}
	if m.lastResponse == nil {
		m.statusMessage = "No current response to diff against. Execute or replay a request first"
		return m, nil
	}

	diff := textdiff.Lines(responseDiffLines(historyEntryResponse(entry)), responseDiffLines(m.lastResponse))

	m.mode = ModeHistoryDiff
	m.configureViewport(&m.historyDiffViewport, buildHistoryDiffContent(entry, diff))
	m.statusMessage = fmt.Sprintf("Diff: %s (history → current response)", entry.ItemName)
	return m, nil
}

// responseDiffLines renders a response deterministically (sorted headers,
// pretty-printed body) so diffs only show real changes.
func responseDiffLines(resp *internalhttp.Response) []string {
	if resp.Error != nil {
		return []string{"Error: " + resp.Error.Error()}
	}

	lines := []string{"Status: " + resp.Status}

	keys := make([]string, 0, len(resp.Headers))
	for key := range resp.Headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		for _, value := range resp.Headers[key] {
			lines = append(lines, fmt.Sprintf("%s: %s", key, value))
		}
	}

	if resp.Body != "" {
		lines = append(lines, "")
		lines = append(lines, prettyBodyLines(resp.Body)...)
	}
	return lines
}

func buildHistoryDiffContent(entry history.Entry, diff []textdiff.Line) string {
	titleStyle := lipgloss.NewStyle().Bold(true)
	deletedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	insertedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))

	lines := []string{
		titleStyle.Render(fmt.Sprintf("Diff: %s @ %s → current response (q: close)",
			entry.ItemName, entry.Snapshot.SavedAt.Format(timestampLayout))),
		deletedStyle.Render("- history entry") + "  " + insertedStyle.Render("+ current response"),
		"",
	}

	if !textdiff.HasChanges(diff) {
		return strings.Join(append(lines, subtleStyle.Render("Responses are identical")), "\n")
	}

	for _, line := range diff {
		switch line.Op {
		case textdiff.OpDelete:
			lines = append(lines, deletedStyle.Render("- "+line.Text))
		case textdiff.OpInsert:
			lines = append(lines, insertedStyle.Render("+ "+line.Text))
		default:
			lines = append(lines, "  "+line.Text)
		}
	}
	return strings.Join(lines, "\n")
}
