package tui

import (
	"context"
	"fmt"
	"postOffice/internal/postman"
	"postOffice/internal/servicebus"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	// toggleListenerKey starts or stops watching the selected PEEK request.
	toggleListenerKey = "w"
	// listenersViewKey opens the messages every listener has seen.
	listenersViewKey = "W"
	// clearListenedMessagesKey forgets the messages seen so far.
	clearListenedMessagesKey = "c"

	// maxListenedMessages caps the messages kept across all listeners; the
	// oldest are dropped first.
	maxListenedMessages = 500
)

// listener is a background watch of one PEEK request: a goroutine polls the
// queue or subscription and sends what arrives on updates.
type listener struct {
	itemID    string
	name      string
	target    string
	startedAt time.Time
	cancel    context.CancelFunc
	updates   <-chan listenerUpdate
	received  int
	// err is the last poll's error; nil once a poll succeeds again.
	err error
}

// listenerUpdate is what one poll found: new messages, or the error it hit.
type listenerUpdate struct {
	messages []servicebus.PeekedMessage
	err      error
}

// listenedMessage is a message a listener saw.
type listenedMessage struct {
	listenerName string
	target       string
	message      servicebus.PeekedMessage
}

// ListenerMsg carries a listener's poll result to the TUI. Done is set once
// the listener's goroutine has exited.
type ListenerMsg struct {
	ItemID string
	// updates identifies the listener, so a stopped listener's late messages
	// don't land in a new one watching the same request.
	updates <-chan listenerUpdate
	Update  listenerUpdate
	Done    bool
}

// watch polls watcher until ctx is cancelled, sending only polls that found
// messages or changed the error, so idle listeners don't redraw the TUI.
func watch(ctx context.Context, watcher *servicebus.Watcher, updates chan<- listenerUpdate) {
	defer close(updates)
	defer watcher.Close()
	var lastErr error
	for {
		messages, err := watcher.Poll(ctx)
		if ctx.Err() != nil {
			return
		}
		if len(messages) > 0 || err != nil || lastErr != nil {
			select {
			case updates <- listenerUpdate{messages: messages, err: err}:
			case <-ctx.Done():
				return
			}
		}
		lastErr = err
		select {
		case <-time.After(watcher.PollInterval()):
		case <-ctx.Done():
			return
		}
	}
}

func waitForListener(itemID string, updates <-chan listenerUpdate) tea.Cmd {
	return func() tea.Msg {
		update, ok := <-updates
		return ListenerMsg{ItemID: itemID, updates: updates, Update: update, Done: !ok}
	}
}

// handleToggleListenerKey starts watching the selected PEEK request, or
// stops the listener already watching it.
func handleToggleListenerKey(m Model) (Model, tea.Cmd) {
	item, ok := m.selectedItem()
	if !ok || !item.IsServiceBusPeek() {
		m.statusMessage = fmt.Sprintf("Select a %s asb:// request to watch", postman.MethodServiceBusPeek)
		return m, nil
	}
	folderPath := m.folderPathForCursor()
	itemID := m.requestItemID(folderPath, item.Name)
	if _, watching := m.listeners[itemID]; watching {
		return m.stopListener(itemID), nil
	}
	request, _ := m.requestToSend(item, itemID)
	if unresolved := m.unresolvedVariables(request, folderPath); len(unresolved) > 0 {
		m.statusMessage = fmt.Sprintf("Cannot watch %s: unresolved %s (load or select an environment that defines them)", item.Name, strings.Join(unresolved, ", "))
		m.statusIsError = true
		return m, nil
	}
	return m.startListener(itemID, item.Name, request, m.variablesAt(folderPath))
}

// startListener watches request in the background. Its variables resolve now;
// unlike a one-shot peek no scripts run.
func (m Model) startListener(itemID, name string, request *postman.Request, variables []postman.VariableSource) (Model, tea.Cmd) {
	watcher, err := servicebus.OpenWatcher(request, variables)
	if err != nil {
		m.statusMessage = fmt.Sprintf("Cannot watch %s: %v", name, err)
		m.statusIsError = true
		return m, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	updates := make(chan listenerUpdate)
	go watch(ctx, watcher, updates)

	m.listeners[itemID] = &listener{
		itemID:    itemID,
		name:      name,
		target:    watcher.Target().String(),
		startedAt: time.Now(),
		cancel:    cancel,
		updates:   updates,
	}
	m.statusMessage = fmt.Sprintf("Watching %s (%s): %s shows messages", watcher.Target(), name, listenersViewKey)
	return m, waitForListener(itemID, updates)
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
	active.err = msg.Update.err
	active.received += len(msg.Update.messages)
	m = m.addListenedMessages(active, msg.Update.messages)
	return m, waitForListener(msg.ItemID, msg.updates)
}

// addListenedMessages appends messages and keeps the cursor on the message
// it pointed at, since the list shows the newest first. A cursor on the
// newest message stays at the top and follows new arrivals.
func (m Model) addListenedMessages(source *listener, messages []servicebus.PeekedMessage) Model {
	if len(messages) == 0 {
		return m
	}
	for _, message := range messages {
		m.listenedMessages = append(m.listenedMessages, listenedMessage{listenerName: source.name, target: source.target, message: message})
	}
	if m.listenerCursor > 0 {
		m.listenerCursor += len(messages)
	}
	if overflow := len(m.listenedMessages) - maxListenedMessages; overflow > 0 {
		m.listenedMessages = slices.Clone(m.listenedMessages[overflow:])
	}
	m.listenerCursor = min(m.listenerCursor, max(len(m.listenedMessages)-1, 0))
	return m
}

// newestListenedMessages returns the listened messages, newest first.
func (m Model) newestListenedMessages() []listenedMessage {
	newestFirst := slices.Clone(m.listenedMessages)
	slices.Reverse(newestFirst)
	return newestFirst
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
	m.statusMessage = fmt.Sprintf("%d listener(s), %d message(s)", len(m.listeners), len(m.listenedMessages))
	return m, nil
}

func (m Model) closeListeners() (Model, tea.Cmd) {
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
	m.listenerCursor = 0
	m.statusMessage = "Cleared listened messages"
	return m, nil
}

// showListenedMessage opens the selected message's properties and body.
func (m Model) showListenedMessage() (Model, tea.Cmd) {
	messages := m.newestListenedMessages()
	if m.listenerCursor >= len(messages) {
		m.statusMessage = "No message selected"
		return m, nil
	}
	selected := messages[m.listenerCursor]
	m.mode = ModeListenerMessage
	m.configureViewport(&m.listenerMessageViewport, strings.Join(listenedMessageLines(selected), "\n"))
	m.statusMessage = fmt.Sprintf("Message #%d from %s (q to close)", selected.message.SequenceNumber, selected.target)
	return m, nil
}

func (m Model) moveListenerCursor(delta int) Model {
	m.listenerCursor = max(0, min(m.listenerCursor+delta, len(m.listenedMessages)-1))
	return m
}
