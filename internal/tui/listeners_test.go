package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"postOffice/internal/history"
	"postOffice/internal/postman"
	"postOffice/internal/servicebus"
)

const (
	peekRequestName = "watch payments"
	testConnection  = "Endpoint=sb://localhost;SharedAccessKeyName=key;SharedAccessKey=secret;UseDevelopmentEmulator=true;"
)

// peekRequestModel lists one PEEK request, with Connection-String set when connection is.
func peekRequestModel(t *testing.T, connection string) Model {
	t.Helper()
	request := &postman.Request{Method: postman.MethodServiceBusPeek, URL: postman.URL{Raw: "asb://payments/postoffice-tap"}}
	if connection != "" {
		request.Header = []postman.Header{{Key: "Connection-String", Value: connection}}
	}
	parser := postman.NewParser()
	collection, err := parser.LoadCollection(writeJSONFile(t, "peek.json", postman.Collection{
		Info:  postman.Info{Name: "Events"},
		Items: []postman.Item{{Name: peekRequestName, Request: request}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(parser, history.NewStore(""))
	m.width, m.height = 160, 40
	m.collection = collection
	m.mode = ModeRequests
	return m.loadRequestsList()
}

func peeked(sequence int64, subject, body string) servicebus.PeekedMessage {
	return servicebus.PeekedMessage{
		SequenceNumber: sequence,
		EnqueuedTime:   time.Date(2026, 11, 14, 10, 0, int(sequence), 0, time.UTC),
		Subject:        subject,
		Body:           json.RawMessage(body),
	}
}

// fakeListener registers a listener that runs no goroutine; cancelled
// reports whether it was stopped.
func fakeListener(m Model, itemID string) (Model, *bool, chan listenerUpdate) {
	cancelled := false
	updates := make(chan listenerUpdate)
	m.listeners[itemID] = &listener{
		itemID:    itemID,
		name:      peekRequestName,
		target:    "payments/postoffice-tap",
		startedAt: time.Now(),
		cancel:    func() { cancelled = true },
		updates:   updates,
	}
	return m, &cancelled, updates
}

func TestToggleListener_RequiresAPeekRequest(t *testing.T) {
	m := slowRequestModel(t)

	m = pressKeys(t, m, runes(toggleListenerKey))

	if len(m.listeners) != 0 || !strings.Contains(m.statusMessage, postman.MethodServiceBusPeek) {
		t.Errorf("listeners = %d, status = %q, want none and a hint", len(m.listeners), m.statusMessage)
	}
}

func TestToggleListener_InvalidRequestReportsError(t *testing.T) {
	m := peekRequestModel(t, "")

	m = pressKeys(t, m, runes(toggleListenerKey))

	if len(m.listeners) != 0 || !m.statusIsError || !strings.Contains(m.statusMessage, "Connection-String") {
		t.Errorf("listeners = %d, status = %q, want none and the missing header", len(m.listeners), m.statusMessage)
	}
}

func TestToggleListener_UnresolvedVariableReportsIt(t *testing.T) {
	m := peekRequestModel(t, "{{sb_connection_string}}")

	m = pressKeys(t, m, runes(toggleListenerKey))

	if len(m.listeners) != 0 || !m.statusIsError || !strings.Contains(m.statusMessage, "{{sb_connection_string}}") {
		t.Errorf("listeners = %d, status = %q, want none and the unresolved variable", len(m.listeners), m.statusMessage)
	}
}

func TestToggleListener_StartsThenStops(t *testing.T) {
	m := peekRequestModel(t, testConnection)
	itemID := m.requestItemID(nil, peekRequestName)

	m = pressKeys(t, m, runes(toggleListenerKey))
	active, watching := m.listeners[itemID]
	if !watching || active.target != "payments/postoffice-tap" {
		t.Fatalf("listeners = %+v, want one watching payments/postoffice-tap", m.listeners)
	}

	m = pressKeys(t, m, runes(toggleListenerKey))
	if len(m.listeners) != 0 {
		t.Errorf("listeners = %d, want the listener stopped", len(m.listeners))
	}
}

func TestListenerMsg_RecordsMessagesAndWaitsAgain(t *testing.T) {
	m, _, updates := fakeListener(peekRequestModel(t, testConnection), "events/watch")

	m, cmd := m.handleListenerMsg(ListenerMsg{ItemID: "events/watch", updates: updates, Update: listenerUpdate{messages: []servicebus.PeekedMessage{peeked(1, "LF-1", `{}`)}}})

	if len(m.listenedMessages) != 1 || m.listeners["events/watch"].received != 1 || cmd == nil {
		t.Errorf("messages = %d, received = %d, cmd = %v, want the message recorded and another wait", len(m.listenedMessages), m.listeners["events/watch"].received, cmd)
	}
}

func TestListenerMsg_IgnoresStoppedListener(t *testing.T) {
	m, _, _ := fakeListener(peekRequestModel(t, testConnection), "events/watch")
	staleUpdates := make(chan listenerUpdate)

	m, cmd := m.handleListenerMsg(ListenerMsg{ItemID: "events/watch", updates: staleUpdates, Update: listenerUpdate{messages: []servicebus.PeekedMessage{peeked(1, "LF-1", `{}`)}}})

	if len(m.listenedMessages) != 0 || cmd != nil {
		t.Errorf("messages = %d, cmd = %v, want a stopped listener's update dropped", len(m.listenedMessages), cmd)
	}
}

func TestListenerMsg_DoneForgetsListener(t *testing.T) {
	m, _, updates := fakeListener(peekRequestModel(t, testConnection), "events/watch")

	m, _ = m.handleListenerMsg(ListenerMsg{ItemID: "events/watch", updates: updates, Done: true})

	if len(m.listeners) != 0 {
		t.Errorf("listeners = %d, want the exited listener forgotten", len(m.listeners))
	}
}

func TestListenerMsg_KeepsPollError(t *testing.T) {
	m, _, updates := fakeListener(peekRequestModel(t, testConnection), "events/watch")
	pollErr := fmt.Errorf("broker unreachable")

	m, _ = m.handleListenerMsg(ListenerMsg{ItemID: "events/watch", updates: updates, Update: listenerUpdate{err: pollErr}})

	if m.listeners["events/watch"].err != pollErr {
		t.Errorf("err = %v, want the poll error kept for display", m.listeners["events/watch"].err)
	}
}

func TestAddListenedMessages_CursorFollowsOrStays(t *testing.T) {
	m, _, _ := fakeListener(peekRequestModel(t, testConnection), "events/watch")
	source := m.listeners["events/watch"]
	m = m.addListenedMessages(source, []servicebus.PeekedMessage{peeked(1, "a", `{}`), peeked(2, "b", `{}`)})

	if m.listenerCursor != 0 {
		t.Fatalf("cursor = %d, want the newest message selected", m.listenerCursor)
	}
	m.listenerCursor = 1
	m = m.addListenedMessages(source, []servicebus.PeekedMessage{peeked(3, "c", `{}`)})

	if got := m.newestListenedMessages()[m.listenerCursor].message.Subject; got != "a" {
		t.Errorf("selected %q, want the cursor kept on the message it pointed at", got)
	}
}

func TestAddListenedMessages_DropsOldestBeyondCap(t *testing.T) {
	m, _, _ := fakeListener(peekRequestModel(t, testConnection), "events/watch")
	messages := make([]servicebus.PeekedMessage, maxListenedMessages+5)
	for i := range messages {
		messages[i] = peeked(int64(i+1), "s", `{}`)
	}

	m = m.addListenedMessages(m.listeners["events/watch"], messages)

	if len(m.listenedMessages) != maxListenedMessages || m.listenedMessages[0].message.SequenceNumber != 6 {
		t.Errorf("kept %d messages starting at #%d, want the newest %d", len(m.listenedMessages), m.listenedMessages[0].message.SequenceNumber, maxListenedMessages)
	}
}

func TestListenersView_OpensListsAndInspects(t *testing.T) {
	m, _, _ := fakeListener(peekRequestModel(t, testConnection), "events/watch")
	m = m.addListenedMessages(m.listeners["events/watch"], []servicebus.PeekedMessage{peeked(7, "LF-26-NWGXJJ", `{"event-name":"claim.created"}`)})

	m = pressKeys(t, m, runes(listenersViewKey))
	if m.mode != ModeListeners {
		t.Fatalf("mode = %v, want the listeners view", m.mode)
	}
	if view := m.renderListenersView(); !strings.Contains(view, "LF-26-NWGXJJ") || !strings.Contains(view, "#7") {
		t.Errorf("listeners view doesn't list the message:\n%s", view)
	}

	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ModeListenerMessage {
		t.Fatalf("mode = %v, want the message view", m.mode)
	}
	m = pressKeys(t, m, runes("q"), runes("q"))
	if m.mode != ModeRequests {
		t.Errorf("mode = %v, want back in requests after closing message then listeners", m.mode)
	}
}

func TestCancelKey_StopsAllListeners(t *testing.T) {
	m, cancelled, _ := fakeListener(peekRequestModel(t, testConnection), "events/watch")
	m.mode = ModeListeners

	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyCtrlX})

	if !*cancelled || len(m.listeners) != 0 {
		t.Errorf("cancelled = %v, listeners = %d, want every listener stopped", *cancelled, len(m.listeners))
	}
}

func TestClearListenedMessages(t *testing.T) {
	m, _, _ := fakeListener(peekRequestModel(t, testConnection), "events/watch")
	m = m.addListenedMessages(m.listeners["events/watch"], []servicebus.PeekedMessage{peeked(1, "a", `{}`)})
	m.mode = ModeListeners

	m = pressKeys(t, m, runes(clearListenedMessagesKey))

	if len(m.listenedMessages) != 0 {
		t.Errorf("messages = %d, want none", len(m.listenedMessages))
	}
}

func TestListenedMessageLines(t *testing.T) {
	message := peeked(3, "LF-1", `{"id":"LF-1"}`)
	message.ApplicationProperties = map[string]any{"z-last": 1, "message-name": "payment.completed.v1"}

	lines := strings.Join(listenedMessageLines(listenedMessage{listenerName: peekRequestName, target: "payments/postoffice-tap", message: message}), "\n")

	if strings.Index(lines, "message-name: payment.completed.v1") > strings.Index(lines, "z-last: 1") {
		t.Error("expected application properties sorted by key")
	}
	if !strings.Contains(lines, "\"id\": \"LF-1\"") {
		t.Errorf("expected the pretty-printed body, got:\n%s", lines)
	}
}

func TestBodyPreview(t *testing.T) {
	if got := bodyPreview(json.RawMessage("{\n  \"a\": 1\n}")); got != `{"a":1}` {
		t.Errorf("bodyPreview(JSON) = %q, want it compacted", got)
	}
	long := json.RawMessage(`"` + strings.Repeat("x", bodyPreviewWidth*2) + `"`)
	if got := bodyPreview(long); len(got) > bodyPreviewWidth+len("…") {
		t.Errorf("bodyPreview(long) is %d bytes, want it truncated", len(got))
	}
}

func TestMessageBodyText_UnwrapsTextBodies(t *testing.T) {
	if got := messageBodyText(json.RawMessage(`"plain text"`)); got != "plain text" {
		t.Errorf("messageBodyText = %q", got)
	}
}

// emulatorConnectionVariable opts in to running against the Service Bus
// emulator, as in the servicebus package's tests.
const emulatorConnectionVariable = "POSTOFFICE_SERVICEBUS_EMULATOR"

func TestEmulator_ListenerReceivesSentMessage(t *testing.T) {
	connection := os.Getenv(emulatorConnectionVariable)
	if connection == "" {
		t.Skipf("set %s to run against the Service Bus emulator", emulatorConnectionVariable)
	}
	m := peekRequestModel(t, connection)
	m, cmd := handleToggleListenerKey(m)
	if cmd == nil {
		t.Fatalf("listener didn't start: %s", m.statusMessage)
	}
	defer m.stopAllListeners()

	messageID := fmt.Sprintf("postoffice-tui-test-%d", time.Now().UnixNano())
	send := &postman.Request{
		Method: "ASB",
		URL:    postman.URL{Raw: "asb://payments"},
		Header: []postman.Header{{Key: "Connection-String", Value: connection}, {Key: "Message-Id", Value: messageID}},
		Body:   &postman.Body{Mode: postman.BodyModeRaw, Raw: `{"from":"tui test"}`},
	}
	if resp := servicebus.SendContext(context.Background(), send, nil); resp.Error != nil {
		t.Fatalf("send: %v", resp.Error)
	}

	deadline := time.After(cancelTestLimit)
	for {
		msgs := make(chan tea.Msg, 1)
		go func(cmd tea.Cmd) { msgs <- cmd() }(cmd)
		select {
		case msg := <-msgs:
			m, cmd = m.handleListenerMsg(msg.(ListenerMsg))
		case <-deadline:
			t.Fatal("the sent message never reached the listener")
		}
		for _, listened := range m.listenedMessages {
			if listened.message.MessageID == messageID {
				return
			}
		}
	}
}
