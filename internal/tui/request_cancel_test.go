package tui

import (
	nethttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"postOffice/internal/history"
	internalhttp "postOffice/internal/http"
	"postOffice/internal/postman"
)

const (
	slowRequest     = "Slow"
	cancelTestLimit = 5 * time.Second
)

// slowRequestModel lists one request to a server that answers only once the
// request is cancelled.
func slowRequestModel(t *testing.T) Model {
	t.Helper()
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)

	parser := postman.NewParser()
	collection, err := parser.LoadCollection(writeJSONFile(t, "slow.json", postman.Collection{
		Info:  postman.Info{Name: "Slow API"},
		Items: []postman.Item{{Name: slowRequest, Request: &postman.Request{Method: "GET", URL: postman.URL{Raw: server.URL}}}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(parser, history.NewStore(""))
	m.collection = collection
	m.mode = ModeRequests
	return m.loadRequestsList()
}

// sendInBackground starts the selected request and returns its result.
func sendInBackground(t *testing.T, m Model) (Model, <-chan tea.Msg) {
	t.Helper()
	m, cmd := m.executeRequest(m.currentItems[m.cursor], m.folderPathForCursor())
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	return m, done
}

func waitForMsg(t *testing.T, done <-chan tea.Msg) RequestCompleteMsg {
	t.Helper()
	select {
	case msg := <-done:
		return msg.(RequestCompleteMsg)
	case <-time.After(cancelTestLimit):
		t.Fatal("the request was not cancelled")
		return RequestCompleteMsg{}
	}
}

func TestCancelRequest_StopsTheSelectedRequest(t *testing.T) {
	m, done := sendInBackground(t, slowRequestModel(t))

	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyCtrlX})
	msg := waitForMsg(t, done)
	updated, _ := m.Update(msg)
	m = updated.(Model)

	if !msg.Cancelled {
		t.Fatal("expected the message marked cancelled")
	}
	itemID := m.requestItemID(nil, slowRequest)
	if execution := m.requestExecutions[itemID]; execution == nil || execution.Status != statusCancelled {
		t.Errorf("expected the request marked cancelled, got %+v", execution)
	}
	if len(m.inFlightRequests) != 0 || m.requestHistory.Len() != 0 {
		t.Errorf("expected nothing in flight and no history, got %d in flight, %d in history", len(m.inFlightRequests), m.requestHistory.Len())
	}
}

func TestCancelRequest_NothingBeingSent(t *testing.T) {
	m := slowRequestModel(t)

	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyCtrlX})

	if m.statusMessage != "No request is being sent" {
		t.Errorf("got %q", m.statusMessage)
	}
}

func TestCancelRequest_CompletedRequestIsForgotten(t *testing.T) {
	m := slowRequestModel(t)
	itemID := m.requestItemID(nil, slowRequest)
	m, _ = m.executeRequest(m.currentItems[m.cursor], nil)

	updated, _ := m.Update(RequestCompleteMsg{ItemID: itemID, ItemName: slowRequest, Response: &internalhttp.Response{Status: "200 OK", StatusCode: 200}})
	m = updated.(Model)

	if _, inFlight := m.inFlightRequests[itemID]; inFlight {
		t.Error("expected the completed request forgotten")
	}
}
