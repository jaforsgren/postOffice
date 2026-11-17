package tui

import (
	"context"
	"errors"
	"fmt"
	"postOffice/internal/postman"
	"postOffice/internal/servicebus"
	"slices"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	// toggleListenerKey starts or stops watching the selected PEEK request,
	// or every PEEK request in the selected folder.
	toggleListenerKey = "w"
	// listenersViewKey opens the messages every listener has seen.
	listenersViewKey = "W"
	// clearListenedMessagesKey forgets the messages seen so far.
	clearListenedMessagesKey = "c"
	// pauseListenersKey holds new messages back so the list stays still.
	pauseListenersKey = "p"
	// listenerFilterKey cycles through showing one listener's messages.
	listenerFilterKey = "f"
	// nextListenedMessageKey and previousListenedMessageKey step through
	// the messages from the message view.
	nextListenedMessageKey     = "n"
	previousListenedMessageKey = "N"
	// copyListenedMessageKey copies the selected message's body.
	copyListenedMessageKey = "y"
	// saveListenedMessageKey saves a message as a request that sends it.
	saveListenedMessageKey = "S"

	// maxListenedMessages caps the messages kept across all listeners, and
	// those held while paused; the oldest are dropped first.
	maxListenedMessages = 500

	listenedMessagesFilePattern = "postoffice-messages-*.json"
	listenedMessagesLabel       = "messages"
)

var errNoListenedMessage = errors.New("no message selected")

// listenedMessageModes act on the selected listened message.
var listenedMessageModes = []ViewMode{ModeListeners, ModeListenerMessage}

// listenerSource is the PEEK request a listener watches. Messages keep it so
// they can be filtered, resent or saved after the listener stops.
type listenerSource struct {
	itemID          string
	name            string
	target          string
	displayProperty string
	collectionName  string
	// folderPath is the request's folder within collectionName.
	folderPath []string
	// request is the PEEK request with its {{variables}}, as defined.
	request *postman.Request
}

// listener is a background watch of one PEEK request: a servicebus.Watch
// goroutine polls the queue or subscription and sends what arrives on updates.
type listener struct {
	*listenerSource
	startedAt time.Time
	cancel    context.CancelFunc
	updates   <-chan servicebus.WatchUpdate
	received  int
	// err is the last poll's error and retryAt when the watch polls again;
	// both are cleared once a poll succeeds.
	err     error
	retryAt time.Time
}

// listenedMessage is a message a listener saw.
type listenedMessage struct {
	source     *listenerSource
	receivedAt time.Time
	message    servicebus.PeekedMessage
}

// ListenerMsg carries a listener's poll result to the TUI. Done is set once
// the listener's goroutine has exited.
type ListenerMsg struct {
	ItemID string
	// updates identifies the listener, so a stopped listener's late messages
	// don't land in a new one watching the same request.
	updates <-chan servicebus.WatchUpdate
	Update  servicebus.WatchUpdate
	Done    bool
}

func waitForListener(itemID string, updates <-chan servicebus.WatchUpdate) tea.Cmd {
	return func() tea.Msg {
		update, ok := <-updates
		return ListenerMsg{ItemID: itemID, updates: updates, Update: update, Done: !ok}
	}
}

// waitForAllListeners waits for every running listener, e.g. the ones
// resumed from the previous session.
func (m Model) waitForAllListeners() tea.Cmd {
	var cmds []tea.Cmd
	for _, active := range m.sortedListeners() {
		cmds = append(cmds, waitForListener(active.itemID, active.updates))
	}
	return tea.Batch(cmds...)
}

// handleToggleListenerKey starts watching the selected PEEK request, or stops
// the listener already watching it. On a folder it does the same for every
// PEEK request inside.
func handleToggleListenerKey(m Model) (Model, tea.Cmd) {
	item, ok := m.selectedItem()
	folderPath := m.folderPathForCursor()
	switch {
	case ok && item.IsFolder():
		return m.toggleFolderListeners(item, folderPath)
	case !ok || !item.IsServiceBusPeek():
		m.statusMessage = fmt.Sprintf("Select a %s asb:// request, or a folder holding some, to watch", postman.MethodServiceBusPeek)
		return m, nil
	}
	itemID := m.requestItemID(folderPath, item.Name)
	if _, watching := m.listeners[itemID]; watching {
		return m.stopListener(itemID), nil
	}
	m, cmd, err := m.startListener(m.listenerSourceAt(item, folderPath, itemID), m.variablesAt(folderPath))
	if err != nil {
		m.statusMessage = err.Error()
		m.statusIsError = true
		return m, nil
	}
	m.statusMessage = fmt.Sprintf("Watching %s (%s): %s shows messages", m.listeners[itemID].target, item.Name, listenersViewKey)
	return m, cmd
}

// listenerSourceAt is the request to watch for item at folderPath in the
// requests view, with its unsaved edits.
func (m Model) listenerSourceAt(item postman.Item, folderPath []string, itemID string) *listenerSource {
	request, _ := m.requestToSend(item, itemID)
	collection, collectionPath := m.collectionScope(folderPath)
	return &listenerSource{itemID: itemID, name: item.Name, collectionName: collection.Info.Name, folderPath: collectionPath, request: request}
}

// peekRequestAt is a PEEK request and the folder it is in, as the requests
// view addresses it.
type peekRequestAt struct {
	item       postman.Item
	folderPath []string
}

// peekRequestsIn lists the PEEK requests in items, which are at folderPath,
// and in their subfolders.
func peekRequestsIn(items []postman.Item, folderPath []string) []peekRequestAt {
	var requests []peekRequestAt
	for _, item := range items {
		switch {
		case item.IsFolder():
			requests = append(requests, peekRequestsIn(item.Items, append(slices.Clone(folderPath), item.Name))...)
		case item.IsServiceBusPeek():
			requests = append(requests, peekRequestAt{item: item, folderPath: folderPath})
		}
	}
	return requests
}

// toggleFolderListeners watches every PEEK request in folder, or stops them
// all when every one is already watched.
func (m Model) toggleFolderListeners(folder postman.Item, parentPath []string) (Model, tea.Cmd) {
	requests := peekRequestsIn(folder.Items, append(slices.Clone(parentPath), folder.Name))
	if len(requests) == 0 {
		m.statusMessage = fmt.Sprintf("No %s requests in %s", postman.MethodServiceBusPeek, folder.Name)
		return m, nil
	}
	itemIDs := make([]string, len(requests))
	allWatched := true
	for i, request := range requests {
		itemIDs[i] = m.requestItemID(request.folderPath, request.item.Name)
		_, watching := m.listeners[itemIDs[i]]
		allWatched = allWatched && watching
	}
	if allWatched {
		for _, itemID := range itemIDs {
			m = m.stopListener(itemID)
		}
		m.statusMessage = fmt.Sprintf("Stopped watching %d request(s) in %s", len(itemIDs), folder.Name)
		return m, nil
	}

	var cmds []tea.Cmd
	var failures []string
	for i, request := range requests {
		if _, watching := m.listeners[itemIDs[i]]; watching {
			continue
		}
		var cmd tea.Cmd
		var err error
		if m, cmd, err = m.startListener(m.listenerSourceAt(request.item, request.folderPath, itemIDs[i]), m.variablesAt(request.folderPath)); err != nil {
			failures = append(failures, err.Error())
			continue
		}
		cmds = append(cmds, cmd)
	}
	m.statusMessage = fmt.Sprintf("Watching %d more request(s) in %s: %s shows messages", len(cmds), folder.Name, listenersViewKey)
	if len(failures) > 0 {
		m.statusMessage = fmt.Sprintf("Watching %d more request(s) in %s; %s", len(cmds), folder.Name, strings.Join(failures, "; "))
		m.statusIsError = true
	}
	return m, tea.Batch(cmds...)
}

// startListener watches source.request in the background. Its variables
// resolve now; unlike a one-shot peek no scripts run.
func (m Model) startListener(source *listenerSource, variables []postman.VariableSource) (Model, tea.Cmd, error) {
	if unresolved := postman.UnresolvedVariables(source.request, variables); len(unresolved) > 0 {
		return m, nil, fmt.Errorf("cannot watch %s: unresolved %s (load or select an environment that defines them)", source.name, strings.Join(unresolved, ", "))
	}
	watcher, err := servicebus.OpenWatcher(source.request, variables)
	if err != nil {
		return m, nil, fmt.Errorf("cannot watch %s: %w", source.name, err)
	}
	source.target = watcher.Target().String()
	source.displayProperty = watcher.DisplayProperty()
	ctx, cancel := context.WithCancel(context.Background())
	updates := make(chan servicebus.WatchUpdate)
	go servicebus.Watch(ctx, watcher, updates)

	m.listeners[source.itemID] = &listener{
		listenerSource: source,
		startedAt:      time.Now(),
		cancel:         cancel,
		updates:        updates,
	}
	return m, waitForListener(source.itemID, updates), nil
}

func (m Model) stopListener(itemID string) Model {
	if active, ok := m.listeners[itemID]; ok {
		active.cancel()
		delete(m.listeners, itemID)
		m.statusMessage = fmt.Sprintf("Stopped watching %s", active.target)
	}
	return m
}

// stopAllListeners stops every listener; their messages stay listed.
func (m Model) stopAllListeners() Model {
	if len(m.listeners) == 0 {
		m.statusMessage = "No listener is running"
		return m
	}
	count := len(m.listeners)
	for itemID := range m.listeners {
		m = m.stopListener(itemID)
	}
	m.statusMessage = fmt.Sprintf("Stopped %d listener(s)", count)
	return m
}

// watchedRequests names the watched requests for the session file.
func (m Model) watchedRequests() []postman.WatchedRequest {
	var watched []postman.WatchedRequest
	for _, active := range m.sortedListeners() {
		watched = append(watched, postman.WatchedRequest{Collection: active.collectionName, FolderPath: active.folderPath, Request: active.name})
	}
	return watched
}

// resumeListeners watches the requests the previous session watched and
// returns the command waiting for them. A request already watched is left
// alone; one that is gone or can't be watched any more is reported.
func (m Model) resumeListeners(watched []postman.WatchedRequest) (Model, tea.Cmd) {
	var cmds []tea.Cmd
	var failures []string
	for _, request := range watched {
		collection, _ := m.parser.GetCollection(request.Collection)
		item := findRequest(collection, request.FolderPath, request.Request)
		if item == nil || !item.IsServiceBusPeek() {
			failures = append(failures, fmt.Sprintf("%s is gone", postman.JoinItemPath(append(append([]string{request.Collection}, request.FolderPath...), request.Request)...)))
			continue
		}
		itemID := m.getRequestIdentifierByPath(collection.Info.Name, request.FolderPath, item.Name)
		if _, watching := m.listeners[itemID]; watching {
			continue
		}
		source := &listenerSource{itemID: itemID, name: item.Name, collectionName: collection.Info.Name, folderPath: request.FolderPath, request: item.Request}
		var cmd tea.Cmd
		var err error
		if m, cmd, err = m.startListener(source, postman.GetAllVariables(collection, request.FolderPath, m.environment)); err != nil {
			failures = append(failures, err.Error())
			continue
		}
		cmds = append(cmds, cmd)
	}
	if len(failures) > 0 {
		m.statusMessage = "Not resuming every watch: " + strings.Join(failures, "; ")
		m.statusIsError = true
	}
	return m, tea.Batch(cmds...)
}

// findRequest returns the request named name in folderPath of collection, nil
// when there is none.
func findRequest(collection *postman.Collection, folderPath []string, name string) *postman.Item {
	if collection == nil {
		return nil
	}
	items := traverseToDepth(collection.Items, folderPath)
	for i := range items {
		if items[i].Name == name && items[i].IsRequest() {
			return &items[i]
		}
	}
	return nil
}

// handleListenerMsg records a poll result and waits for the listener's next
// one. Results of stopped listeners are dropped.
func (m Model) handleListenerMsg(msg ListenerMsg) (Model, tea.Cmd) {
	active, ok := m.listeners[msg.ItemID]
	if !ok || active.updates != msg.updates {
		return m, nil
	}
	if msg.Done {
		delete(m.listeners, msg.ItemID)
		return m, nil
	}
	active.err = msg.Update.Err
	active.retryAt = msg.Update.RetryAt
	active.received += len(msg.Update.Messages)
	messages := make([]listenedMessage, len(msg.Update.Messages))
	receivedAt := time.Now()
	for i, message := range msg.Update.Messages {
		messages[i] = listenedMessage{source: active.listenerSource, receivedAt: receivedAt, message: message}
	}
	if m.listenersPaused {
		m.heldListenedMessages = keepNewest(append(m.heldListenedMessages, messages...))
	} else {
		m = m.addListenedMessages(messages)
	}
	return m, waitForListener(msg.ItemID, msg.updates)
}

// addListenedMessages adds messages in enqueued order, so listeners' backlogs
// interleave by time, and keeps the cursor on the message it pointed at. A
// cursor on the newest message stays at the top and follows new arrivals.
func (m Model) addListenedMessages(messages []listenedMessage) Model {
	if len(messages) == 0 {
		return m
	}
	selected, hasSelection := m.selectedListenedMessage()
	keepSelection := hasSelection && m.listenerCursor > 0
	m.listenedMessages = append(m.listenedMessages, messages...)
	slices.SortStableFunc(m.listenedMessages, func(a, b listenedMessage) int {
		return a.message.EnqueuedTime.Compare(b.message.EnqueuedTime)
	})
	m.listenedMessages = keepNewest(m.listenedMessages)
	visible := m.visibleListenedMessages()
	m.listenerCursor = 0
	if keepSelection {
		m.listenerCursor = max(0, slices.IndexFunc(visible, selected.sameMessage))
	}
	if m.mode != ModeListeners && m.mode != ModeListenerMessage {
		m.unseenListenedMessages += len(messages)
	}
	if m.mode == ModeResponse {
		m.responseViewport.SetContent(strings.Join(m.responseViewLines(), "\n"))
	}
	return m
}

// sameMessage reports whether other is this message, seen by the same listener.
func (listened listenedMessage) sameMessage(other listenedMessage) bool {
	return listened.source.itemID == other.source.itemID && listened.message.SequenceNumber == other.message.SequenceNumber
}

// keepNewest drops the oldest messages beyond maxListenedMessages.
func keepNewest(messages []listenedMessage) []listenedMessage {
	if overflow := len(messages) - maxListenedMessages; overflow > 0 {
		return slices.Clone(messages[overflow:])
	}
	return messages
}

// visibleListenedMessages are the messages the listeners view lists, newest
// first: those of the listener filtered to (if any) that match the search.
func (m Model) visibleListenedMessages() []listenedMessage {
	var visible []listenedMessage
	for i := len(m.listenedMessages) - 1; i >= 0; i-- {
		listened := m.listenedMessages[i]
		if m.listenerFilter != "" && listened.source.itemID != m.listenerFilter {
			continue
		}
		if m.listenerQuery != "" && !strings.Contains(listenedSearchText(listened), m.listenerQuery) {
			continue
		}
		visible = append(visible, listened)
	}
	return visible
}

// listenedSearchText is what a search of the listeners view matches, in lower case.
func listenedSearchText(listened listenedMessage) string {
	message := listened.message
	parts := []string{listened.source.target, listened.source.name, message.MessageID, message.Subject, message.CorrelationID, message.BodyText()}
	for _, key := range sortedPropertyKeys(message.ApplicationProperties) {
		parts = append(parts, fmt.Sprintf("%s=%v", key, message.ApplicationProperties[key]))
	}
	return strings.ToLower(strings.Join(parts, "\n"))
}

// sortedListeners returns the running listeners in the order they started.
func (m Model) sortedListeners() []*listener {
	listeners := make([]*listener, 0, len(m.listeners))
	for _, active := range m.listeners {
		listeners = append(listeners, active)
	}
	slices.SortFunc(listeners, func(a, b *listener) int { return a.startedAt.Compare(b.startedAt) })
	return listeners
}

func (m Model) openListeners() (Model, tea.Cmd) {
	if m.mode != ModeListeners && m.mode != ModeListenerMessage {
		m.listenersReturnMode = m.mode
	}
	m.mode = ModeListeners
	m.unseenListenedMessages = 0
	m.statusMessage = fmt.Sprintf("%d listener(s), %d message(s)", len(m.listeners), len(m.listenedMessages))
	return m, nil
}

// closeListeners clears a search first, then returns to where the listeners
// view was opened from.
func (m Model) closeListeners() (Model, tea.Cmd) {
	if m.listenerQuery != "" {
		m = m.clearSearch()
		m.statusMessage = "Search cleared"
		return m, nil
	}
	m.mode = m.listenersReturnMode
	if m.mode == modeUnset {
		m.mode = ModeRequests
	}
	m.statusMessage = "Closed listeners"
	return m, nil
}

func handleListenersViewKey(m Model) (Model, tea.Cmd) {
	return m.openListeners()
}

func handleListenersCommand(m Model, _ []string) (Model, tea.Cmd) {
	return m.openListeners()
}

func handleClearListenedMessagesKey(m Model) (Model, tea.Cmd) {
	m.listenedMessages = nil
	m.heldListenedMessages = nil
	m.listenerCursor = 0
	m.statusMessage = "Cleared listened messages"
	return m, nil
}

// handlePauseListenersKey holds new messages back, or adds the ones held.
// Listeners keep polling while paused.
func handlePauseListenersKey(m Model) (Model, tea.Cmd) {
	if !m.listenersPaused {
		m.listenersPaused = true
		m.statusMessage = fmt.Sprintf("Paused: new messages are held until %s", pauseListenersKey)
		return m, nil
	}
	held := len(m.heldListenedMessages)
	m.listenersPaused = false
	m = m.addListenedMessages(m.heldListenedMessages)
	m.heldListenedMessages = nil
	m.statusMessage = fmt.Sprintf("Resumed: %d held message(s) added", held)
	return m, nil
}

// handleListenerFilterKey cycles from all messages through each listener's own.
func handleListenerFilterKey(m Model) (Model, tea.Cmd) {
	sources := m.listenedSources()
	if len(sources) == 0 {
		m.statusMessage = "No messages to filter"
		return m, nil
	}
	next := 0
	if m.listenerFilter != "" {
		next = slices.IndexFunc(sources, func(source *listenerSource) bool { return source.itemID == m.listenerFilter }) + 1
	}
	m.listenerCursor = 0
	if next >= len(sources) {
		m.listenerFilter = ""
		m.statusMessage = "Showing every listener's messages"
		return m, nil
	}
	m.listenerFilter = sources[next].itemID
	m.statusMessage = fmt.Sprintf("Showing messages from %s (%s) only; %s for the next listener", sources[next].target, sources[next].name, listenerFilterKey)
	return m, nil
}

// listenedSources are the listeners the listed messages came from, by name.
func (m Model) listenedSources() []*listenerSource {
	seen := make(map[string]bool)
	var sources []*listenerSource
	for _, listened := range m.listenedMessages {
		if !seen[listened.source.itemID] {
			seen[listened.source.itemID] = true
			sources = append(sources, listened.source)
		}
	}
	slices.SortFunc(sources, func(a, b *listenerSource) int { return strings.Compare(a.name, b.name) })
	return sources
}

func (m Model) selectedListenedMessage() (listenedMessage, bool) {
	messages := m.visibleListenedMessages()
	if m.listenerCursor >= len(messages) {
		return listenedMessage{}, false
	}
	return messages[m.listenerCursor], true
}

// showListenedMessage opens the selected message's properties and body.
func (m Model) showListenedMessage() (Model, tea.Cmd) {
	selected, ok := m.selectedListenedMessage()
	if !ok {
		m.statusMessage = errNoListenedMessage.Error()
		return m, nil
	}
	m.mode = ModeListenerMessage
	m.configureViewport(&m.listenerMessageViewport, strings.Join(listenedMessageLines(selected), "\n"))
	m.statusMessage = fmt.Sprintf("Message #%d from %s", selected.message.SequenceNumber, selected.source.target)
	return m, nil
}

func handleNextListenedMessageKey(m Model) (Model, tea.Cmd) {
	return m.moveListenerCursor(1).showListenedMessage()
}

func handlePreviousListenedMessageKey(m Model) (Model, tea.Cmd) {
	return m.moveListenerCursor(-1).showListenedMessage()
}

func (m Model) moveListenerCursor(delta int) Model {
	m.listenerCursor = max(0, min(m.listenerCursor+delta, len(m.visibleListenedMessages())-1))
	return m
}

// handleCopyListenedMessageKey copies the selected message's body.
func handleCopyListenedMessageKey(m Model) (Model, tea.Cmd) {
	selected, ok := m.selectedListenedMessage()
	if !ok {
		return m.copyFailed(errNoListenedMessage), nil
	}
	if err := clipboard.WriteAll(selected.message.BodyText()); err != nil {
		return m.copyFailed(err), nil
	}
	m.statusMessage = fmt.Sprintf("Copied the body of #%d to clipboard", selected.message.SequenceNumber)
	return m, nil
}

// handleOpenListenedMessagesKey opens the shown message (message view) or
// every listed message (listeners view) as JSON in the editor, where it can
// be saved to a file.
func handleOpenListenedMessagesKey(m Model) (Model, tea.Cmd) {
	messages := m.visibleListenedMessages()
	if m.mode == ModeListenerMessage {
		messages = nil
		if selected, ok := m.selectedListenedMessage(); ok {
			messages = []listenedMessage{selected}
		}
	}
	if len(messages) == 0 {
		m.statusMessage = errNoListenedMessage.Error()
		return m, nil
	}
	content, err := listenedMessagesJSON(messages)
	if err != nil {
		m.statusMessage = err.Error()
		return m, nil
	}
	return m.openReadOnlyReport(listenedMessagesFilePattern, content, listenedMessagesLabel)
}
