package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"postOffice/internal/http"
)

const yankResponseBody = `{"ok":true}`

func modelWithCursorOn(t *testing.T, name string) Model {
	t.Helper()
	m := createTestModel()
	for i, item := range m.currentItems {
		if item.Name == name {
			m.cursor = i
			return m
		}
	}
	t.Fatalf("no item %q in the requests view", name)
	return m
}

func withExecutedResponse(m Model, name string) Model {
	m.requestExecutions[m.getRequestIdentifierByPath(m.collection.Info.Name, nil, name)] = &RequestExecution{
		Response: &http.Response{
			Status:         "200 OK",
			Body:           yankResponseBody,
			RequestMethod:  "POST",
			RequestURL:     "https://sent.example.com/api/create",
			RequestHeaders: map[string]string{"X-B": "2", "X-A": "1"},
		},
	}
	return m
}

func TestYankText_RequestsViewUsesSelectedRequest(t *testing.T) {
	target, err := modelWithCursorOn(t, formRequestName).currentYankTarget()
	if err != nil {
		t.Fatal(err)
	}

	url, _, _ := target.yankText(yankURLKey)
	if url != "https://example.com/api/create" {
		t.Errorf("expected the request URL, got %q", url)
	}
	curl, label, _ := target.yankText(yankRequestKey)
	if label != "curl" || !strings.Contains(curl, "-X POST") {
		t.Errorf("expected a POST curl, got %q (%s)", curl, label)
	}
	if _, _, err := target.yankText(yankBodyKey); !errors.Is(err, errYankNoResponse) {
		t.Errorf("expected errYankNoResponse before sending, got %v", err)
	}
}

func TestYankText_RequestsViewUsesLatestResponse(t *testing.T) {
	m := withExecutedResponse(modelWithCursorOn(t, formRequestName), formRequestName)
	target, _ := m.currentYankTarget()

	body, _, err := target.yankText(yankBodyKey)
	if err != nil || body != yankResponseBody {
		t.Errorf("expected the response body, got %q (%v)", body, err)
	}
	all, _, _ := target.yankText(yankAllKey)
	if !strings.Contains(all, "REQUEST") || !strings.Contains(all, "RESPONSE") || strings.Contains(all, "\x1b[") {
		t.Errorf("expected a plain request and response report, got %q", all)
	}
}

func TestYankText_ResponseViewUsesSentRequest(t *testing.T) {
	m := withExecutedResponse(modelWithCursorOn(t, formRequestName), formRequestName)
	m.lastResponse = m.requestExecutions[m.getRequestIdentifierByPath(m.collection.Info.Name, nil, formRequestName)].Response
	m.previousMode, m.mode = ModeRequests, ModeResponse
	target, _ := m.currentYankTarget()

	url, _, _ := target.yankText(yankURLKey)
	if url != "https://sent.example.com/api/create" {
		t.Errorf("expected the sent URL, got %q", url)
	}
	curl, _, _ := target.yankText(yankRequestKey)
	if strings.Index(curl, "X-A") > strings.Index(curl, "X-B") {
		t.Errorf("expected headers in sorted order, got %q", curl)
	}
}

func TestYankText_UnknownKey(t *testing.T) {
	target, _ := modelWithCursorOn(t, formRequestName).currentYankTarget()
	if _, _, err := target.yankText("z"); err == nil {
		t.Error("expected an error for an unknown yank")
	}
}

func TestCurrentYankTarget_FolderIsRejected(t *testing.T) {
	if _, err := modelWithCursorOn(t, "Test Folder").currentYankTarget(); !errors.Is(err, errYankNoRequest) {
		t.Errorf("expected errYankNoRequest, got %v", err)
	}
}

func TestCurrentYankTarget_InfoViewOverEnvironmentsIsRejected(t *testing.T) {
	m := modelWithCursorOn(t, formRequestName)
	item := m.currentItems[m.cursor]
	m.currentInfoItem = &item
	m.previousMode, m.mode = ModeEnvironments, ModeInfo

	if _, err := m.currentYankTarget(); !errors.Is(err, errYankNoRequest) {
		t.Errorf("expected errYankNoRequest, got %v", err)
	}
}

func TestFullReport_HasRequestInfoAndResponse(t *testing.T) {
	m := withExecutedResponse(modelWithCursorOn(t, formRequestName), formRequestName)
	target, _ := m.currentYankTarget()

	report, err := target.fullReport()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"REQUEST INFO", formRequestName, "RESPONSE", "true"} {
		if !strings.Contains(report, want) {
			t.Errorf("expected the report to contain %q, got %q", want, report)
		}
	}
}

func TestYankPrefix_WaitsForSecondKey(t *testing.T) {
	m := modelWithCursorOn(t, formRequestName)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(yankPrefixKey)})
	m = updated.(Model)
	if !m.pendingYank {
		t.Fatal("expected a pending yank")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.pendingYank {
		t.Error("expected esc to cancel the yank")
	}
}
