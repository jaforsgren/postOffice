package tui

import (
	"errors"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"postOffice/internal/history"
	internalhttp "postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/textdiff"

	tea "github.com/charmbracelet/bubbletea"
)

func sizedTestModel() Model {
	m := createTestModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return updated.(Model)
}

func pressKey(t *testing.T, m Model, key tea.KeyMsg) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(key)
	return updated.(Model), cmd
}

func historyEntryFor(url string) history.Entry {
	return history.Entry{
		ItemID:          "Test Collection/POST Request",
		ItemName:        "POST Request",
		Protocol:        history.ProtocolHTTP,
		EnvironmentName: "Test Environment",
		Snapshot: postman.SavedResponse{
			SavedAt:        time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
			RequestMethod:  "POST",
			RequestURL:     url,
			RequestHeaders: map[string]string{"X-Token": "resolved-secret"},
			RequestBody:    `{"resolved":true}`,
			StatusCode:     200,
			Status:         "200 OK",
			Body:           `{"value":1}`,
		},
	}
}

func TestNewHistoryEntry(t *testing.T) {
	executedAt := time.Now()
	msg := RequestCompleteMsg{
		ItemID:          "C/req",
		ItemName:        "req",
		Protocol:        history.ProtocolAMQP,
		EnvironmentName: "dev",
		Response: &internalhttp.Response{
			RequestMethod: "AMQP",
			RequestURL:    "amqp://broker/queue",
			Status:        "Message Published",
			StatusCode:    200,
			Body:          "ok",
			Duration:      1500 * time.Millisecond,
		},
	}

	entry := newHistoryEntry(msg, executedAt)

	if entry.Protocol != history.ProtocolAMQP || entry.EnvironmentName != "dev" || entry.ItemID != "C/req" {
		t.Errorf("entry metadata = %+v", entry)
	}
	if entry.Snapshot.RequestURL != "amqp://broker/queue" || entry.Snapshot.DurationMS != 1500 {
		t.Errorf("snapshot = %+v", entry.Snapshot)
	}
	if !entry.Snapshot.SavedAt.Equal(executedAt) {
		t.Errorf("SavedAt = %v, want %v", entry.Snapshot.SavedAt, executedAt)
	}
	if entry.Error != "" {
		t.Errorf("Error = %q, want empty", entry.Error)
	}

	msg.Response.Error = errors.New("connection refused")
	if got := newHistoryEntry(msg, executedAt).Error; got != "connection refused" {
		t.Errorf("Error = %q, want connection refused", got)
	}
}

func TestRequestComplete_RecordsHistory(t *testing.T) {
	m := sizedTestModel()
	msg := RequestCompleteMsg{
		ItemID:          "Test Collection/POST Request",
		ItemName:        "POST Request",
		Protocol:        history.ProtocolHTTP,
		EnvironmentName: "Test Environment",
		Response:        &internalhttp.Response{Status: "201 Created", StatusCode: 201, RequestURL: "https://example.com"},
	}

	updated, _ := m.Update(msg)
	m = updated.(Model)

	entries := m.requestHistory.Entries()
	if len(entries) != 1 {
		t.Fatalf("history length = %d, want 1", len(entries))
	}
	if entries[0].Snapshot.Status != "201 Created" || entries[0].EnvironmentName != "Test Environment" {
		t.Errorf("entry = %+v", entries[0])
	}
}

func TestRecordHistory_KeepsCursorOnSelectedEntry(t *testing.T) {
	m := sizedTestModel()
	for _, name := range []string{"a", "b", "c"} {
		m = m.recordHistory(RequestCompleteMsg{ItemName: name, Response: &internalhttp.Response{}})
	}
	m.requestHistoryCursor = 1 // "b"

	m = m.recordHistory(RequestCompleteMsg{ItemName: "d", Response: &internalhttp.Response{}})

	entry, _ := m.selectedHistoryEntry()
	if entry.ItemName != "b" {
		t.Errorf("selected = %s, want b", entry.ItemName)
	}
}

func TestOpenAndCloseRequestHistory(t *testing.T) {
	m := sizedTestModel()

	m, _ = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlO})
	if m.mode != ModeHistory {
		t.Fatalf("mode = %v, want ModeHistory", m.mode)
	}

	m, _ = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ModeRequests {
		t.Errorf("mode after esc = %v, want ModeRequests", m.mode)
	}
}

func TestHistoryCommandAndAliases(t *testing.T) {
	for _, name := range []string{"history", "hist", "hi"} {
		t.Run(name, func(t *testing.T) {
			m := sizedTestModel()
			m, _ = m.commandRegistry.ExecuteCommand(m, name, nil)
			if m.mode != ModeHistory {
				t.Errorf("mode = %v, want ModeHistory", m.mode)
			}
			if m.historyReturnMode != ModeRequests {
				t.Errorf("historyReturnMode = %v, want ModeRequests", m.historyReturnMode)
			}
		})
	}
}

func TestHistoryNavigation(t *testing.T) {
	m := sizedTestModel()
	for i := 0; i < 3; i++ {
		_ = m.requestHistory.Add(historyEntryFor("https://example.com"))
	}
	m, _ = m.openRequestHistory()

	m, _ = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m, _ = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m, _ = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.requestHistoryCursor != 2 {
		t.Errorf("cursor after 3x j = %d, want 2 (clamped)", m.requestHistoryCursor)
	}

	m, _ = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if m.requestHistoryCursor != 1 {
		t.Errorf("cursor after k = %d, want 1", m.requestHistoryCursor)
	}
}

func TestReplay_ResendsResolvedRequestAndShowsResponse(t *testing.T) {
	var gotMethod, gotHeader, gotBody string
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		gotMethod = r.Method
		gotHeader = r.Header.Get("X-Token")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(nethttp.StatusAccepted)
		_, _ = w.Write([]byte(`{"value":2}`))
	}))
	defer server.Close()

	m := sizedTestModel()
	_ = m.requestHistory.Add(historyEntryFor(server.URL))
	m, _ = m.openRequestHistory()

	m, cmd := pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("replay returned no command")
	}
	msg, ok := cmd().(RequestCompleteMsg)
	if !ok || !msg.Replayed {
		t.Fatalf("replay message = %#v, want replayed RequestCompleteMsg", msg)
	}

	if gotMethod != "POST" || gotHeader != "resolved-secret" || gotBody != `{"resolved":true}` {
		t.Errorf("server received %s header=%q body=%q", gotMethod, gotHeader, gotBody)
	}

	updated, _ := m.Update(msg)
	m = updated.(Model)

	if m.mode != ModeResponse || m.previousMode != ModeHistory {
		t.Errorf("mode = %v (previous %v), want ModeResponse from ModeHistory", m.mode, m.previousMode)
	}
	if m.lastResponse == nil || m.lastResponse.StatusCode != nethttp.StatusAccepted {
		t.Errorf("lastResponse = %+v", m.lastResponse)
	}
	if m.requestHistory.Len() != 2 {
		t.Errorf("history length = %d, want 2", m.requestHistory.Len())
	}
	if replayed := m.requestHistory.Entries()[0]; replayed.EnvironmentName != "Test Environment" {
		t.Errorf("replayed environment = %q, want original", replayed.EnvironmentName)
	}

	m, _ = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ModeHistory {
		t.Errorf("mode after esc = %v, want ModeHistory", m.mode)
	}
	if m.requestHistoryCursor != 1 {
		t.Errorf("cursor = %d, want 1 (still on the original entry)", m.requestHistoryCursor)
	}

	m, _ = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ModeRequests {
		t.Errorf("mode after second esc = %v, want ModeRequests", m.mode)
	}
}

func TestReplay_UnknownProtocol(t *testing.T) {
	m := sizedTestModel()
	entry := historyEntryFor("https://example.com")
	entry.Protocol = "SMTP"
	_ = m.requestHistory.Add(entry)
	m, _ = m.openRequestHistory()

	m, cmd := m.replayHistoryEntry()

	if cmd != nil {
		t.Error("expected no command for unknown protocol")
	}
	if !strings.Contains(m.statusMessage, "Cannot replay") {
		t.Errorf("statusMessage = %q", m.statusMessage)
	}
}

func TestReplay_EmptyHistory(t *testing.T) {
	m := sizedTestModel()
	m, _ = m.openRequestHistory()

	_, cmd := m.replayHistoryEntry()
	if cmd != nil {
		t.Error("expected no command for empty history")
	}
}

func TestSenderFor_AllProtocols(t *testing.T) {
	m := sizedTestModel()
	for _, protocol := range []history.Protocol{history.ProtocolHTTP, history.ProtocolGRPC, history.ProtocolServiceBus, history.ProtocolAMQP} {
		if send, err := m.senderFor(protocol); err != nil || send == nil {
			t.Errorf("senderFor(%s) = %v, %v", protocol, send, err)
		}
	}
	if _, err := m.senderFor("SMTP"); !errors.Is(err, ErrUnknownProtocol) {
		t.Errorf("senderFor(SMTP) error = %v, want ErrUnknownProtocol", err)
	}
}

func TestExecuteRequest_GRPCIsRecordedAsGRPC(t *testing.T) {
	m := sizedTestModel()
	item := postman.Item{
		Name:    "Say Hello",
		Request: &postman.Request{Method: "GRPC", URL: postman.URL{Raw: "grpc://"}},
	}

	_, cmd := m.executeRequest(item)
	if cmd == nil {
		t.Fatal("gRPC execution returned no command")
	}
	msg := cmd().(RequestCompleteMsg)
	if msg.Protocol != history.ProtocolGRPC || msg.Response.Error == nil {
		t.Errorf("msg = %+v, want gRPC protocol with invalid URL error", msg)
	}
}

func TestHistoryDiff_RequiresCurrentResponse(t *testing.T) {
	m := sizedTestModel()
	_ = m.requestHistory.Add(historyEntryFor("https://example.com"))
	m, _ = m.openRequestHistory()

	m, _ = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})

	if m.mode != ModeHistory {
		t.Errorf("mode = %v, want ModeHistory", m.mode)
	}
	if !strings.Contains(m.statusMessage, "No current response") {
		t.Errorf("statusMessage = %q", m.statusMessage)
	}
}

func TestHistoryDiff_ShowsChangesAndCloses(t *testing.T) {
	m := sizedTestModel()
	_ = m.requestHistory.Add(historyEntryFor("https://example.com"))
	m.lastResponse = &internalhttp.Response{Status: "500 Internal Server Error", StatusCode: 500, Body: `{"value":1}`}
	m, _ = m.openRequestHistory()

	m, _ = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})

	if m.mode != ModeHistoryDiff {
		t.Fatalf("mode = %v, want ModeHistoryDiff", m.mode)
	}
	content := m.renderHistoryDiffView()
	if !strings.Contains(content, "- Status: 200 OK") || !strings.Contains(content, "+ Status: 500 Internal Server Error") {
		t.Errorf("diff content missing status changes:\n%s", content)
	}

	m, _ = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if m.mode != ModeHistory {
		t.Errorf("mode after q = %v, want ModeHistory", m.mode)
	}
}

func TestBuildHistoryDiffContent_Identical(t *testing.T) {
	entry := historyEntryFor("https://example.com")
	lines := responseDiffLines(historyEntryResponse(entry))

	content := buildHistoryDiffContent(entry, textdiff.Lines(lines, lines))

	if !strings.Contains(content, "Responses are identical") {
		t.Errorf("content = %q", content)
	}
}

func TestResponseDiffLines(t *testing.T) {
	resp := &internalhttp.Response{
		Status:  "200 OK",
		Headers: map[string][]string{"X-B": {"2"}, "A-Header": {"1"}},
		Body:    `{"a":1}`,
	}

	got := responseDiffLines(resp)
	want := []string{"Status: 200 OK", "A-Header: 1", "X-B: 2", "", "{", `  "a": 1`, "}"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("responseDiffLines() = %q, want %q", got, want)
	}

	errResp := &internalhttp.Response{Error: errors.New("timeout")}
	if got := responseDiffLines(errResp); len(got) != 1 || got[0] != "Error: timeout" {
		t.Errorf("error lines = %q", got)
	}
}

func TestHistoryEntryResponse_RestoresError(t *testing.T) {
	entry := historyEntryFor("https://example.com")
	entry.Error = "dial tcp: refused"

	if resp := historyEntryResponse(entry); resp.Error == nil || resp.Error.Error() != entry.Error {
		t.Errorf("Error = %v, want %q", resp.Error, entry.Error)
	}
}

func TestRenderHistoryView(t *testing.T) {
	m := sizedTestModel()
	m, _ = m.openRequestHistory()
	if view := m.renderHistoryView(); !strings.Contains(view, "No requests executed yet") {
		t.Errorf("empty view = %q", view)
	}

	_ = m.requestHistory.Add(historyEntryFor("https://example.com/orders"))
	view := m.renderHistoryView()
	for _, want := range []string{"POST Request", "https://example.com/orders", "Test Environment", "200 OK", "2026-01-02 03:04:05"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q", want)
		}
	}
}

func TestFormatByteSize(t *testing.T) {
	tests := map[int]string{
		0:                   "0B",
		bytesPerKiB - 1:     "1023B",
		bytesPerKiB:         "1.0KB",
		bytesPerMiB * 3 / 2: "1.5MB",
	}
	for size, want := range tests {
		if got := formatByteSize(size); got != want {
			t.Errorf("formatByteSize(%d) = %q, want %q", size, got, want)
		}
	}
}
