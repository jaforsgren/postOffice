package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"postOffice/internal/http"
)

func panelContent(m Model) string {
	return ansi.Strip(strings.Join(m.responseViewLines(), "\n"))
}

func TestInfoKey_OnRequestShowsDefinitionAndNotSentNote(t *testing.T) {
	m := modelWithCursorOn(t, formRequestName)

	m = pressKeys(t, m, runes("i"))

	if m.mode != ModeResponse || m.responsePanelItem == nil {
		t.Fatalf("expected the request panel, got mode %v", m.mode)
	}
	content := panelContent(m)
	if !strings.Contains(content, "https://example.com/api/create") || !strings.Contains(content, requestPanelNotSentMessage) {
		t.Errorf("expected the definition and the not-sent note, got:\n%s", content)
	}
}

func TestInfoKey_OnSentRequestShowsDefinitionAndExchange(t *testing.T) {
	m := withExecutedResponse(modelWithCursorOn(t, formRequestName), formRequestName)

	m = pressKeys(t, m, runes("i"))

	content := panelContent(m)
	for _, want := range []string{"Method:", "RESPONSE", "Status: 200 OK", `"ok": true`} {
		if !strings.Contains(content, want) {
			t.Errorf("expected %q in the panel, got:\n%s", want, content)
		}
	}
}

func TestInfoKey_OnFolderStillShowsFolderInfo(t *testing.T) {
	m := modelWithCursorOn(t, "Test Folder")

	m = pressKeys(t, m, runes("i"))

	if m.mode != ModeInfo {
		t.Errorf("expected the folder info view, got mode %v", m.mode)
	}
}

func TestRequestPanel_CtrlRSendsAndRefreshesInPlace(t *testing.T) {
	m := pressKeys(t, modelWithCursorOn(t, formRequestName), runes("i"))

	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyCtrlE})
	itemID := m.getRequestIdentifierByPath(m.collection.Info.Name, nil, formRequestName)
	if _, sending := m.requestExecutions[itemID]; !sending {
		t.Fatal("expected ctrl+e to send the panel's request")
	}

	updated, _ := m.Update(RequestCompleteMsg{ItemID: itemID, ItemName: formRequestName, Response: &http.Response{StatusCode: 201, Status: "201 Created"}})
	m = updated.(Model)

	content := panelContent(m)
	if m.mode != ModeResponse || !strings.Contains(content, "Method:") || !strings.Contains(content, "201 Created") {
		t.Errorf("expected the panel to keep the definition and show the new response, got:\n%s", content)
	}
}

func TestRequestsView_CtrlRNoLongerOpensTheResponse(t *testing.T) {
	m := withExecutedResponse(modelWithCursorOn(t, formRequestName), formRequestName)

	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})

	if m.mode != ModeRequests {
		t.Errorf("expected ctrl+r to do nothing in the requests view (i opens the panel), got mode %v", m.mode)
	}
}

func TestRequestPanel_YankWorksBeforeSending(t *testing.T) {
	m := pressKeys(t, modelWithCursorOn(t, formRequestName), runes("i"))

	target, err := m.currentYankTarget()
	if err != nil {
		t.Fatal(err)
	}
	if url, _, _ := target.yankText(yankURLKey); url != "https://example.com/api/create" {
		t.Errorf("expected the request URL, got %q", url)
	}
}
