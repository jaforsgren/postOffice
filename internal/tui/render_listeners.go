package tui

import (
	"encoding/json"
	"fmt"
	"maps"
	"postOffice/internal/postman"
	"postOffice/internal/servicebus"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	internalhttp "postOffice/internal/http"
)

const (
	listenedTimeLayout = "15:04:05"
	watchingIndicator  = "◉ watching"
	// newMessagesIndicator counts the messages that arrived while the
	// listeners view was closed.
	newMessagesIndicator = "◉ %d new message(s) (%s)"
	listenerOKIcon       = "●"
	listenerErrorIcon    = "✗"
	pausedIndicator      = "⏸ paused, %d held (%s resumes)"

	// relatedMessageWindow is how long after a request completes the
	// messages listeners receive count as published by it.
	relatedMessageWindow = 10 * time.Second
)

var (
	listenerTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	listenerOKStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	listenerErrorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	listenerSelectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
)

func (m Model) renderListenersView() string {
	title := listenerTitleStyle.Render(fmt.Sprintf("Listeners (%d watching, %d messages)", len(m.listeners), len(m.listenedMessages)))
	help := subtleStyle.Render(fmt.Sprintf("(enter: inspect | %s: resend | %s: save as request | /: search | %s: filter | %s: pause | %s: open | %s: clear | %s: stop all | esc: back)",
		runKey, saveListenedMessageKey, listenerFilterKey, pauseListenersKey, openReportKey, clearListenedMessagesKey, cancelRequestKey))
	lines := []string{title + " " + help}

	listeners := m.sortedListeners()
	if len(listeners) == 0 {
		lines = append(lines, subtleStyle.Render(fmt.Sprintf("No listener running. Press %s on a PEEK request, or a folder of them, to watch.", toggleListenerKey)))
	}
	for _, active := range listeners {
		lines = append(lines, listenerLine(active))
	}
	if filters := m.listenerFilterLine(); filters != "" {
		lines = append(lines, filters)
	}
	lines = append(lines, "")

	messages := m.visibleListenedMessages()
	if len(messages) == 0 {
		lines = append(lines, subtleStyle.Render("No messages yet."))
	} else {
		visibleLines := m.height - windowChromeHeight - len(lines)
		startIdx, endIdx := calculateVisibleWindow(m.listenerCursor, len(messages), visibleLines)
		for i := startIdx; i < endIdx; i++ {
			marker := "  "
			if i == m.listenerCursor {
				marker = listenerSelectedStyle.Render("▶ ")
			}
			lines = append(lines, marker+formatListenedMessage(messages[i]))
		}
	}

	// Each message takes one row, which calculateVisibleWindow counts on, so
	// rows are cut rather than wrapped.
	contentWidth := max(m.width-4-2*paneHorizontalPadding, 0)
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], contentWidth, "…")
	}
	return mainWindowStyle.
		Height(m.height - windowChromeHeight).
		Width(m.width - 4).
		Render(strings.Join(lines, "\n"))
}

// listenerLine is a running listener: its target and request, and its count
// of messages or its error and when it retries.
func listenerLine(active *listener) string {
	status := listenerOKStyle.Render(listenerOKIcon)
	detail := subtleStyle.Render(fmt.Sprintf("%d message(s)", active.received))
	if active.err != nil {
		status = listenerErrorStyle.Render(listenerErrorIcon)
		detail = listenerErrorStyle.Render(fmt.Sprintf("%v (retrying at %s)", active.err, active.retryAt.Format(listenedTimeLayout)))
	}
	return fmt.Sprintf(" %s %s  %s  %s", status, requestStyle.Render(active.target), subtleStyle.Render(active.name), detail)
}

// listenerFilterLine says what limits the listed messages: a pause, a
// listener filter and a search. Empty when nothing does.
func (m Model) listenerFilterLine() string {
	var parts []string
	if m.listenersPaused {
		parts = append(parts, watchingStyle.Render(fmt.Sprintf(pausedIndicator, len(m.heldListenedMessages), pauseListenersKey)))
	}
	if m.listenerFilter != "" {
		for _, source := range m.listenedSources() {
			if source.itemID == m.listenerFilter {
				parts = append(parts, fmt.Sprintf("only %s (%s)", source.target, listenerFilterKey))
			}
		}
	}
	if m.listenerQuery != "" {
		parts = append(parts, fmt.Sprintf("search %q (esc clears)", m.listenerQuery))
	}
	return strings.Join(parts, "  ")
}

func formatListenedMessage(listened listenedMessage) string {
	message := listened.message
	return fmt.Sprintf("%s  %s  %s  %s  %s",
		message.EnqueuedTime.Local().Format(listenedTimeLayout),
		requestStyle.Render(listened.source.target),
		subtleStyle.Render(fmt.Sprintf("#%d", message.SequenceNumber)),
		messageLabel(listened),
		subtleStyle.Render(message.BodyPreview()),
	)
}

// messageLabel names a message in lists by its listener's Display-Property.
func messageLabel(listened listenedMessage) string {
	return listened.message.Label(listened.source.displayProperty)
}

func sortedPropertyKeys(properties map[string]any) []string {
	return slices.Sorted(maps.Keys(properties))
}

// listenedMessageLines renders a message's system properties, application
// properties (sorted) and pretty-printed body. Delivery and dead-letter
// details show only when the broker set them.
func listenedMessageLines(listened listenedMessage) []string {
	message := listened.message
	lines := []string{
		fmt.Sprintf("%s  #%d  (%s)", listened.source.target, message.SequenceNumber, listened.source.name),
		subtleStyle.Render(fmt.Sprintf("%s/%s: next/previous | %s: resend | %s: save as request | %s: copy body | %s: open", nextListenedMessageKey, previousListenedMessageKey, runKey, saveListenedMessageKey, copyListenedMessageKey, openReportKey)),
		"",
		"Enqueued:       " + message.EnqueuedTime.Local().Format(timestampLayout),
	}
	if !message.ExpiresAt.IsZero() {
		lines = append(lines, "Expires:        "+message.ExpiresAt.Local().Format(timestampLayout))
	}
	lines = append(lines,
		fmt.Sprintf("Delivery count: %d", message.DeliveryCount),
		"Message ID:     "+message.MessageID,
		"Subject:        "+message.Subject,
		"Content type:   "+message.ContentType,
		"Correlation ID: "+message.CorrelationID,
	)
	if message.DeadLetterReason != "" || message.DeadLetterErrorDescription != "" || message.DeadLetterSource != "" {
		lines = append(lines,
			"",
			listenerErrorStyle.Render("Dead-lettered:"),
			"  Reason:       "+message.DeadLetterReason,
			"  Description:  "+message.DeadLetterErrorDescription,
			"  Source:       "+message.DeadLetterSource,
		)
	}
	lines = append(lines, "", "Application properties:")
	for _, key := range sortedPropertyKeys(message.ApplicationProperties) {
		lines = append(lines, fmt.Sprintf("  %s: %v", key, message.ApplicationProperties[key]))
	}
	lines = append(lines, "", "Body:")
	return append(lines, prettyBodyLines(message.BodyText())...)
}

func (m Model) renderListenerMessageView() string {
	return mainWindowStyle.
		Height(m.height - windowChromeHeight).
		Width(m.width - 4).
		Render(m.listenerMessageViewport.View())
}

// listenedMessagesJSON is messages as an indented JSON array.
func listenedMessagesJSON(messages []listenedMessage) (string, error) {
	exported := make([]servicebus.SourcedMessage, len(messages))
	for i, listened := range messages {
		exported[i] = listened.sourced()
	}
	data, err := json.MarshalIndent(exported, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode messages: %w", err)
	}
	return string(data) + "\n", nil
}

func (listened listenedMessage) sourced() servicebus.SourcedMessage {
	return servicebus.SourcedMessage{Request: listened.source.name, Target: listened.source.target, PeekedMessage: listened.message}
}

// relatedListenedMessages are the messages listeners received while response
// was being sent and for relatedMessageWindow after it completed: the events
// the request likely caused. None for a peek, which only reads messages.
func (m Model) relatedListenedMessages(response *internalhttp.Response) []listenedMessage {
	if response == nil || response.RequestMethod == postman.MethodServiceBusPeek {
		return nil
	}
	completedAt, ok := m.responseCompletedAt(response)
	if !ok {
		return nil
	}
	sentAt := completedAt.Add(-response.Duration)
	windowEnd := completedAt.Add(relatedMessageWindow)
	var related []listenedMessage
	for _, listened := range m.listenedMessages {
		if !listened.receivedAt.Before(sentAt) && !listened.receivedAt.After(windowEnd) {
			related = append(related, listened)
		}
	}
	return related
}

func (m Model) responseCompletedAt(response *internalhttp.Response) (time.Time, bool) {
	for _, execution := range m.requestExecutions {
		if execution.Response == response {
			return execution.Timestamp, true
		}
	}
	return time.Time{}, false
}

// relatedMessagesSection lists the messages a response's request likely
// published, while listeners are running or have seen some.
func (m Model) relatedMessagesSection() []string {
	if m.lastResponse == nil || m.lastResponse.RequestMethod == postman.MethodServiceBusPeek {
		return nil
	}
	related := m.relatedListenedMessages(m.lastResponse)
	if len(related) == 0 && len(m.listeners) == 0 {
		return nil
	}
	lines := []string{
		"",
		watchingStyle.Bold(true).Render("PUBLISHED"),
		subtleStyle.Render(fmt.Sprintf("Messages listeners received while sending and up to %v after (%s shows all)", relatedMessageWindow, listenersViewKey)),
	}
	if len(related) == 0 {
		return append(lines, subtleStyle.Render(fmt.Sprintf("None yet: watching %d queue(s) or subscription(s)", len(m.listeners))))
	}
	for _, listened := range related {
		lines = append(lines, "  "+formatListenedMessage(listened))
	}
	return lines
}
