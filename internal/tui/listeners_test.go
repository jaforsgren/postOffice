package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"postOffice/internal/history"
	internalhttp "postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/servicebus"
)

const (
	peekRequestName  = "watch payments"
	peekFolderName   = "Topics"
	peekCollection   = "Events"
	testConnection   = "Endpoint=sb://localhost;SharedAccessKeyName=key;SharedAccessKey=secret;UseDevelopmentEmulator=true;"
	connectionHeader = "Connection-String"
)

func peekRequest(connection string) *postman.Request {
	request := &postman.Request{Method: postman.MethodServiceBusPeek, URL: postman.URL{Raw: "asb://payments/postoffice-tap"}}
	if connection != "" {
		request.Header = []postman.Header{{Key: connectionHeader, Value: connection}}
	}
	return request
}

// listenerModel lists items of the Events collection in the requests view.
func listenerModel(t *testing.T, items ...postman.Item) Model {
	t.Helper()
	parser := postman.NewParser()
	collection, err := parser.LoadCollection(writeJSONFile(t, "peek.json", postman.Collection{Info: postman.Info{Name: peekCollection}, Items: items}))
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(parser, history.NewStore(""))
	m.width, m.height = 160, 40
	m.collection = collection
	m.mode = ModeRequests
	return m.loadRequestsList()
}

// peekRequestModel lists one PEEK request, with Connection-String set when connection is.
func peekRequestModel(t *testing.T, connection string) Model {
	t.Helper()
	return listenerModel(t, postman.Item{Name: peekRequestName, Request: peekRequest(connection)})
}

func peeked(sequence int64, subject, body string) servicebus.PeekedMessage {
	return servicebus.PeekedMessage{
		SequenceNumber: sequence,
		EnqueuedTime:   time.Date(2026, 11, 14, 10, 0, int(sequence), 0, time.UTC),
		Subject:        subject,
		Body:           json.RawMessage(body),
	}
}

func testSource(itemID, name string) *listenerSource {
	return &listenerSource{itemID: itemID, name: name, target: name + "/postoffice-tap", collectionName: peekCollection, request: peekRequest("{{sb_connection_string}}")}
}

func listenedFrom(source *listenerSource, messages ...servicebus.PeekedMessage) []listenedMessage {
	listened := make([]listenedMessage, len(messages))
	for i, message := range messages {
		listened[i] = listenedMessage{source: source, receivedAt: time.Now(), message: message}
	}
	return listened
}

// fakeListener registers a listener that runs no goroutine; cancelled
// reports whether it was stopped.
func fakeListener(m Model, itemID string) (Model, *bool, chan servicebus.WatchUpdate) {
	cancelled := false
	updates := make(chan servicebus.WatchUpdate)
	m.listeners[itemID] = &listener{
		listenerSource: testSource(itemID, peekRequestName),
		startedAt:      time.Now(),
		cancel:         func() { cancelled = true },
		updates:        updates,
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

	if len(m.listeners) != 0 || !m.statusIsError || !strings.Contains(m.statusMessage, connectionHeader) {
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
	if !watching || active.target != "payments/postoffice-tap" || active.collectionName != peekCollection {
		t.Fatalf("listeners = %+v, want one watching payments/postoffice-tap", m.listeners)
	}

	m = pressKeys(t, m, runes(toggleListenerKey))
	if len(m.listeners) != 0 {
		t.Errorf("listeners = %d, want the listener stopped", len(m.listeners))
	}
}

func TestToggleListener_FolderWatchesEveryPeekRequestThenStopsThem(t *testing.T) {
	m := listenerModel(t, postman.Item{Name: peekFolderName, Items: []postman.Item{
		{Name: "payments", Request: peekRequest(testConnection)},
		{Name: "send", Request: &postman.Request{Method: "ASB", URL: postman.URL{Raw: "asb://payments"}}},
		{Name: "Nested", Items: []postman.Item{{Name: "claims", Request: peekRequest(testConnection)}}},
	}})

	m = pressKeys(t, m, runes(toggleListenerKey))
	if len(m.listeners) != 2 {
		t.Fatalf("listeners = %d, want both PEEK requests in the folder watched: %s", len(m.listeners), m.statusMessage)
	}
	if _, nested := m.listeners[m.requestItemID([]string{peekFolderName, "Nested"}, "claims")]; !nested {
		t.Errorf("listeners = %v, want the nested request watched under its own folder", m.listeners)
	}

	m = pressKeys(t, m, runes(toggleListenerKey))
	if len(m.listeners) != 0 {
		t.Errorf("listeners = %d, want every one stopped", len(m.listeners))
	}
}

func TestListenerMsg_RecordsMessagesAndWaitsAgain(t *testing.T) {
	m, _, updates := fakeListener(peekRequestModel(t, testConnection), "events/watch")

	m, cmd := m.handleListenerMsg(ListenerMsg{ItemID: "events/watch", updates: updates, Update: servicebus.WatchUpdate{Messages: []servicebus.PeekedMessage{peeked(1, "LF-1", `{}`)}}})

	if len(m.listenedMessages) != 1 || m.listeners["events/watch"].received != 1 || cmd == nil {
		t.Errorf("messages = %d, received = %d, cmd = %v, want the message recorded and another wait", len(m.listenedMessages), m.listeners["events/watch"].received, cmd)
	}
}

func TestListenerMsg_IgnoresStoppedListener(t *testing.T) {
	m, _, _ := fakeListener(peekRequestModel(t, testConnection), "events/watch")
	staleUpdates := make(chan servicebus.WatchUpdate)

	m, cmd := m.handleListenerMsg(ListenerMsg{ItemID: "events/watch", updates: staleUpdates, Update: servicebus.WatchUpdate{Messages: []servicebus.PeekedMessage{peeked(1, "LF-1", `{}`)}}})

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

func TestListenerMsg_ShowsPollErrorAndRetry(t *testing.T) {
	m, _, updates := fakeListener(peekRequestModel(t, testConnection), "events/watch")
	pollErr := errors.New("broker unreachable")
	retryAt := time.Date(2026, 11, 16, 10, 4, 31, 0, time.Local)

	m, _ = m.handleListenerMsg(ListenerMsg{ItemID: "events/watch", updates: updates, Update: servicebus.WatchUpdate{Err: pollErr, RetryAt: retryAt}})

	if line := listenerLine(m.listeners["events/watch"]); !strings.Contains(line, "broker unreachable") || !strings.Contains(line, "10:04:31") {
		t.Errorf("listener line = %q, want the error and when it retries", line)
	}
}

func TestAddListenedMessages_CursorFollowsOrStays(t *testing.T) {
	source := testSource("events/watch", peekRequestName)
	m := peekRequestModel(t, testConnection)
	m = m.addListenedMessages(listenedFrom(source, peeked(1, "a", `{}`), peeked(2, "b", `{}`)))

	if m.listenerCursor != 0 {
		t.Fatalf("cursor = %d, want the newest message selected", m.listenerCursor)
	}
	m.listenerCursor = 1
	m = m.addListenedMessages(listenedFrom(source, peeked(3, "c", `{}`)))

	if selected, _ := m.selectedListenedMessage(); selected.message.Subject != "a" {
		t.Errorf("selected %q, want the cursor kept on the message it pointed at", selected.message.Subject)
	}
}

func TestAddListenedMessages_InterleavesByEnqueuedTime(t *testing.T) {
	m := peekRequestModel(t, testConnection)
	m = m.addListenedMessages(listenedFrom(testSource("events/payments", "payments"), peeked(1, "p1", `{}`), peeked(5, "p5", `{}`)))
	m = m.addListenedMessages(listenedFrom(testSource("events/claims", "claims"), peeked(3, "c3", `{}`)))

	var subjects []string
	for _, listened := range m.visibleListenedMessages() {
		subjects = append(subjects, listened.message.Subject)
	}
	if got := strings.Join(subjects, ","); got != "p5,c3,p1" {
		t.Errorf("listed %s, want newest enqueued first across listeners", got)
	}
}

func TestAddListenedMessages_DropsOldestBeyondCap(t *testing.T) {
	messages := make([]servicebus.PeekedMessage, maxListenedMessages+5)
	for i := range messages {
		messages[i] = peeked(int64(i+1), "s", `{}`)
	}

	m := peekRequestModel(t, testConnection).addListenedMessages(listenedFrom(testSource("events/watch", peekRequestName), messages...))

	if len(m.listenedMessages) != maxListenedMessages || m.listenedMessages[0].message.SequenceNumber != 6 {
		t.Errorf("kept %d messages starting at #%d, want the newest %d", len(m.listenedMessages), m.listenedMessages[0].message.SequenceNumber, maxListenedMessages)
	}
}

func TestAddListenedMessages_CountsUnseenUntilTheViewOpens(t *testing.T) {
	m := peekRequestModel(t, testConnection)

	m = m.addListenedMessages(listenedFrom(testSource("events/watch", peekRequestName), peeked(1, "a", `{}`), peeked(2, "b", `{}`)))
	if m.unseenListenedMessages != 2 || !strings.Contains(m.renderStatusBar(), fmt.Sprintf(newMessagesIndicator, 2, listenersViewKey)) {
		t.Fatalf("unseen = %d, status bar = %q, want 2 new messages announced", m.unseenListenedMessages, m.renderStatusBar())
	}

	m = pressKeys(t, m, runes(listenersViewKey))
	m = m.addListenedMessages(listenedFrom(testSource("events/watch", peekRequestName), peeked(3, "c", `{}`)))
	if m.unseenListenedMessages != 0 {
		t.Errorf("unseen = %d, want none once the listeners view shows them", m.unseenListenedMessages)
	}
}

func TestPauseListeners_HoldsMessagesUntilResumed(t *testing.T) {
	m, _, updates := fakeListener(peekRequestModel(t, testConnection), "events/watch")
	m.mode = ModeListeners

	m = pressKeys(t, m, runes(pauseListenersKey))
	m, _ = m.handleListenerMsg(ListenerMsg{ItemID: "events/watch", updates: updates, Update: servicebus.WatchUpdate{Messages: []servicebus.PeekedMessage{peeked(1, "a", `{}`)}}})
	if len(m.listenedMessages) != 0 || len(m.heldListenedMessages) != 1 {
		t.Fatalf("listed %d, held %d, want the message held while paused", len(m.listenedMessages), len(m.heldListenedMessages))
	}
	if view := m.renderListenersView(); !strings.Contains(view, fmt.Sprintf(pausedIndicator, 1, pauseListenersKey)) {
		t.Errorf("listeners view doesn't say it is paused:\n%s", view)
	}

	m = pressKeys(t, m, runes(pauseListenersKey))
	if len(m.listenedMessages) != 1 || len(m.heldListenedMessages) != 0 || m.listenersPaused {
		t.Errorf("listed %d, held %d, paused %v, want the held message listed on resume", len(m.listenedMessages), len(m.heldListenedMessages), m.listenersPaused)
	}
}

func TestListenerFilter_CyclesThroughListeners(t *testing.T) {
	payments, claims := testSource("events/payments", "payments"), testSource("events/claims", "claims")
	m := peekRequestModel(t, testConnection)
	m = m.addListenedMessages(append(listenedFrom(payments, peeked(1, "p", `{}`)), listenedFrom(claims, peeked(2, "c", `{}`))...))
	m.mode = ModeListeners

	var shown []string
	for range 3 {
		m = pressKeys(t, m, runes(listenerFilterKey))
		shown = append(shown, fmt.Sprint(len(m.visibleListenedMessages())))
	}

	if got := strings.Join(shown, ","); got != "1,1,2" || m.listenerFilter != "" {
		t.Errorf("visible counts = %s, want claims only, payments only, then all", got)
	}
}

func TestListenerSearch_FiltersAndEscClearsBeforeClosing(t *testing.T) {
	source := testSource("events/watch", peekRequestName)
	created := peeked(1, "", `{"id":"LF-1"}`)
	created.ApplicationProperties = map[string]any{"message-name": "claim.created.v1"}
	m := peekRequestModel(t, testConnection)
	m = m.addListenedMessages(listenedFrom(source, created, peeked(2, "", `{"id":"LF-2"}`)))
	m = pressKeys(t, m, runes(listenersViewKey), runes("/"), runes("CLAIM.created"), tea.KeyMsg{Type: tea.KeyEnter})

	if visible := m.visibleListenedMessages(); len(visible) != 1 || visible[0].message.SequenceNumber != 1 {
		t.Fatalf("visible = %d, want only the message whose property matches", len(visible))
	}

	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ModeListeners || len(m.visibleListenedMessages()) != 2 {
		t.Fatalf("mode = %v, visible = %d, want the search cleared and the view still open", m.mode, len(m.visibleListenedMessages()))
	}
	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ModeRequests {
		t.Errorf("mode = %v, want the view closed", m.mode)
	}
}

func TestListenersView_OpensListsAndInspects(t *testing.T) {
	m := peekRequestModel(t, testConnection)
	m = m.addListenedMessages(listenedFrom(testSource("events/watch", peekRequestName), peeked(7, "LF-26-NWGXJJ", `{"event-name":"claim.created"}`)))

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

func TestListenerMessageView_StepsThroughMessages(t *testing.T) {
	m := peekRequestModel(t, testConnection)
	m = m.addListenedMessages(listenedFrom(testSource("events/watch", peekRequestName), peeked(1, "first", `{}`), peeked(2, "second", `{}`)))
	m = pressKeys(t, m, runes(listenersViewKey), tea.KeyMsg{Type: tea.KeyEnter})

	m = pressKeys(t, m, runes(nextListenedMessageKey))
	if !strings.Contains(m.listenerMessageViewport.View(), "first") {
		t.Errorf("after %s the view shows:\n%s\nwant the older message", nextListenedMessageKey, m.listenerMessageViewport.View())
	}
	m = pressKeys(t, m, runes(previousListenedMessageKey))
	if !strings.Contains(m.listenerMessageViewport.View(), "second") || m.mode != ModeListenerMessage {
		t.Errorf("after %s the view shows:\n%s\nwant the newest message", previousListenedMessageKey, m.listenerMessageViewport.View())
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
	m := peekRequestModel(t, testConnection)
	m = m.addListenedMessages(listenedFrom(testSource("events/watch", peekRequestName), peeked(1, "a", `{}`)))
	m.mode = ModeListeners

	m = pressKeys(t, m, runes(clearListenedMessagesKey))

	if len(m.listenedMessages) != 0 {
		t.Errorf("messages = %d, want none", len(m.listenedMessages))
	}
}

func TestMessageLabel(t *testing.T) {
	message := peeked(1, "", `{}`)
	message.ApplicationProperties = map[string]any{"z-last": 1, "message-name": "claim.created.v1"}
	source := testSource("events/watch", peekRequestName)
	withDisplay := *source
	withDisplay.displayProperty = "z-last"
	withSubject := message
	withSubject.Subject = "LF-1"

	tests := []struct {
		name     string
		listened listenedMessage
		want     string
	}{
		{"display property", listenedMessage{source: &withDisplay, message: withSubject}, "1"},
		{"subject", listenedMessage{source: source, message: withSubject}, "LF-1"},
		{"first property", listenedMessage{source: source, message: message}, "message-name=claim.created.v1"},
		{"nothing", listenedMessage{source: source, message: peeked(1, "", `{}`)}, ""},
	}
	for _, tt := range tests {
		if got := messageLabel(tt.listened); got != tt.want {
			t.Errorf("%s: messageLabel = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestListenedMessageLines(t *testing.T) {
	message := peeked(3, "LF-1", `{"id":"LF-1"}`)
	message.ApplicationProperties = map[string]any{"z-last": 1, "message-name": "payment.completed.v1"}

	lines := strings.Join(listenedMessageLines(listenedFrom(testSource("events/watch", peekRequestName), message)[0]), "\n")

	if strings.Index(lines, "message-name: payment.completed.v1") > strings.Index(lines, "z-last: 1") {
		t.Error("expected application properties sorted by key")
	}
	if !strings.Contains(lines, "\"id\": \"LF-1\"") {
		t.Errorf("expected the pretty-printed body, got:\n%s", lines)
	}
	if strings.Contains(lines, "Dead-lettered") || strings.Contains(lines, "Expires") {
		t.Errorf("expected no dead-letter or expiry lines for a plain message, got:\n%s", lines)
	}
}

func TestListenedMessageLines_ShowsDeadLetterDetails(t *testing.T) {
	message := peeked(3, "LF-1", `{}`)
	message.DeadLetterReason = "MaxDeliveryCountExceeded"
	message.DeliveryCount = 10
	message.ExpiresAt = message.EnqueuedTime.Add(time.Hour)

	lines := strings.Join(listenedMessageLines(listenedFrom(testSource("events/watch", peekRequestName), message)[0]), "\n")

	for _, want := range []string{"Dead-lettered", "MaxDeliveryCountExceeded", "Delivery count: 10", "Expires"} {
		if !strings.Contains(lines, want) {
			t.Errorf("expected %q in:\n%s", want, lines)
		}
	}
}

func TestListenedMessagesJSON(t *testing.T) {
	content, err := listenedMessagesJSON(listenedFrom(testSource("events/watch", peekRequestName), peeked(4, "LF-1", `{"id":"LF-1"}`)))
	if err != nil {
		t.Fatal(err)
	}
	var exported []map[string]any
	if err := json.Unmarshal([]byte(content), &exported); err != nil {
		t.Fatalf("content isn't a JSON array: %v\n%s", err, content)
	}
	if len(exported) != 1 || exported[0]["request"] != peekRequestName || exported[0]["sequenceNumber"] != float64(4) || exported[0]["subject"] != "LF-1" {
		t.Errorf("exported = %v, want the message with its listener", exported)
	}
}

func TestRelatedMessages_ShowsWhatArrivedDuringAndAfterTheRequest(t *testing.T) {
	m, _, _ := fakeListener(peekRequestModel(t, testConnection), "events/watch")
	completedAt := time.Now()
	response := &internalhttp.Response{RequestMethod: "POST", RequestURL: "http://localhost/claims", StatusCode: 201, Duration: time.Second}
	m.requestExecutions["events/create"] = &RequestExecution{Timestamp: completedAt, Response: response}
	m.lastResponse = response
	source := testSource("events/watch", peekRequestName)
	before := listenedMessage{source: source, receivedAt: completedAt.Add(-time.Minute), message: peeked(1, "before", `{}`)}
	during := listenedMessage{source: source, receivedAt: completedAt.Add(-time.Second / 2), message: peeked(2, "during", `{}`)}
	after := listenedMessage{source: source, receivedAt: completedAt.Add(relatedMessageWindow / 2), message: peeked(3, "after", `{}`)}
	late := listenedMessage{source: source, receivedAt: completedAt.Add(2 * relatedMessageWindow), message: peeked(4, "late", `{}`)}
	m.listenedMessages = []listenedMessage{before, during, after, late}

	section := strings.Join(m.relatedMessagesSection(), "\n")

	if !strings.Contains(section, "during") || !strings.Contains(section, "after") || strings.Contains(section, "before") || strings.Contains(section, "late") {
		t.Errorf("section = %s, want only the messages received while sending and shortly after", section)
	}
}

func TestRelatedMessages_NoneForAPeek(t *testing.T) {
	m, _, _ := fakeListener(peekRequestModel(t, testConnection), "events/watch")
	m.lastResponse = &internalhttp.Response{RequestMethod: postman.MethodServiceBusPeek}

	if section := m.relatedMessagesSection(); section != nil {
		t.Errorf("section = %v, want none for a peek", section)
	}
}

func TestListenedSendItem_SendsTheMessageToItsTopic(t *testing.T) {
	message := peeked(9, "", `{"id":"LF-1"}`)
	message.ContentType = "application/json"
	message.ApplicationProperties = map[string]any{"message-name": "claim.created.v1"}

	item, err := listenedSendItem(listenedFrom(testSource("events/watch", peekRequestName), message)[0])
	if err != nil {
		t.Fatal(err)
	}

	request := item.Request
	if item.Name != "message-name=claim.created.v1 #9" || request.Method != "ASB" || request.URL.Raw != "asb://payments" || request.Body.Raw != `{"id":"LF-1"}` {
		t.Errorf("item = %s %+v, want an ASB send of the body to the topic", item.Name, request)
	}
	if request.Header[0] != (postman.Header{Key: connectionHeader, Value: "{{sb_connection_string}}"}) {
		t.Errorf("headers = %+v, want the PEEK's connection string kept as a variable first", request.Header)
	}
}

func TestSaveListenedMessage_AddsRequestNextToThePeek(t *testing.T) {
	m := peekRequestModel(t, testConnection)
	m = m.addListenedMessages(listenedFrom(testSource("events/watch", peekRequestName), peeked(9, "LF-1", `{"id":"LF-1"}`)))
	m = pressKeys(t, m, runes(listenersViewKey), runes(saveListenedMessageKey))

	collection, _ := m.parser.GetCollection(peekCollection)
	saved := findRequest(collection, nil, "LF-1 #9")
	if saved == nil || saved.Request.Method != "ASB" {
		t.Fatalf("collection items = %+v, status = %q, want the message saved as an ASB request", collection.Items, m.statusMessage)
	}
	path, _ := m.parser.GetCollectionPath(peekCollection)
	if data, err := os.ReadFile(path); err != nil || !strings.Contains(string(data), "LF-1 #9") {
		t.Errorf("collection file doesn't hold the saved request: %v", err)
	}
}

func TestResendListenedMessage_SendsThroughTheEngine(t *testing.T) {
	m := peekRequestModel(t, testConnection)
	m = m.addListenedMessages(listenedFrom(testSource("events/watch", peekRequestName), peeked(9, "LF-1", `{}`)))
	m.mode = ModeListeners

	m, cmd := handleResendListenedMessageKey(m)

	if cmd == nil || m.requestExecutions["events/watch"+resentItemSuffix] == nil {
		t.Errorf("cmd = %v, executions = %v, status = %q, want the send started", cmd, m.requestExecutions, m.statusMessage)
	}
}

func TestWatchedRequests_ResumeInTheNextSession(t *testing.T) {
	m := peekRequestModel(t, testConnection)
	m = pressKeys(t, m, runes(toggleListenerKey))
	watched := m.watchedRequests()
	m = m.stopAllListeners()
	if len(watched) != 1 || watched[0].Collection != peekCollection || len(watched[0].FolderPath) != 0 || watched[0].Request != peekRequestName {
		t.Fatalf("watched = %+v, want the PEEK request", watched)
	}

	m, cmd := m.resumeListeners(append(watched, postman.WatchedRequest{Collection: peekCollection, Request: "deleted"}))
	defer m.stopAllListeners()

	if len(m.listeners) != 1 || cmd == nil {
		t.Errorf("listeners = %d, want the watch resumed", len(m.listeners))
	}
	if !m.statusIsError || !strings.Contains(m.statusMessage, "deleted is gone") {
		t.Errorf("status = %q, want the missing request reported", m.statusMessage)
	}
	if _, again := m.resumeListeners(watched); len(m.listeners) != 1 || again != nil {
		t.Errorf("listeners = %d, want an already watched request left alone", len(m.listeners))
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
		Header: []postman.Header{{Key: connectionHeader, Value: connection}, {Key: "Message-Id", Value: messageID}},
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
