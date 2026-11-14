package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	listenedTimeLayout = "15:04:05"
	// bodyPreviewWidth bounds a message's one-line body preview.
	bodyPreviewWidth  = 120
	watchingIndicator = "◉ watching"
	listenerOKIcon    = "●"
	listenerErrorIcon = "✗"
)

func (m Model) renderListenersView() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)

	title := titleStyle.Render(fmt.Sprintf("Listeners (%d watching, %d messages)", len(m.listeners), len(m.listenedMessages)))
	help := helpStyle.Render(fmt.Sprintf("(enter: inspect | %s: clear | %s: stop all | esc: back)", clearListenedMessagesKey, cancelRequestKey))
	lines := []string{title + " " + help}

	listeners := m.sortedListeners()
	if len(listeners) == 0 {
		lines = append(lines, helpStyle.Render(fmt.Sprintf("No listener running. Press %s on a PEEK request to watch it.", toggleListenerKey)))
	}
	for _, active := range listeners {
		status := okStyle.Render(listenerOKIcon)
		detail := helpStyle.Render(fmt.Sprintf("%d message(s)", active.received))
		if active.err != nil {
			status = errStyle.Render(listenerErrorIcon)
			detail = errStyle.Render(active.err.Error())
		}
		lines = append(lines, fmt.Sprintf(" %s %s  %s  %s", status, requestStyle.Render(active.target), helpStyle.Render(active.name), detail))
	}
	lines = append(lines, "")

	messages := m.newestListenedMessages()
	if len(messages) == 0 {
		lines = append(lines, helpStyle.Render("No messages yet."))
	} else {
		visibleLines := m.height - windowChromeHeight - len(lines)
		startIdx, endIdx := calculateVisibleWindow(m.listenerCursor, len(messages), visibleLines)
		for i := startIdx; i < endIdx; i++ {
			marker := "  "
			if i == m.listenerCursor {
				marker = selectedStyle.Render("▶ ")
			}
			lines = append(lines, marker+formatListenedMessage(messages[i]))
		}
	}

	return mainWindowStyle.
		Height(m.height - windowChromeHeight).
		Width(m.width - 4).
		Render(strings.Join(lines, "\n"))
}

func formatListenedMessage(listened listenedMessage) string {
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	message := listened.message
	return fmt.Sprintf("%s  %s  %s  %s  %s",
		message.EnqueuedTime.Local().Format(listenedTimeLayout),
		requestStyle.Render(listened.target),
		dimStyle.Render(fmt.Sprintf("#%d", message.SequenceNumber)),
		message.Subject,
		dimStyle.Render(bodyPreview(message.Body)),
	)
}

// bodyPreview is the body on one line, compacted when it is JSON.
func bodyPreview(body json.RawMessage) string {
	var compact bytes.Buffer
	preview := string(body)
	if json.Compact(&compact, body) == nil {
		preview = compact.String()
	}
	if len(preview) > bodyPreviewWidth {
		preview = preview[:bodyPreviewWidth] + "…"
	}
	return preview
}

// listenedMessageLines renders a message's system properties, application
// properties (sorted) and pretty-printed body.
func listenedMessageLines(listened listenedMessage) []string {
	message := listened.message
	lines := []string{
		fmt.Sprintf("%s  #%d  (%s)", listened.target, message.SequenceNumber, listened.listenerName),
		"",
		"Enqueued:       " + message.EnqueuedTime.Local().Format(timestampLayout),
		"Message ID:     " + message.MessageID,
		"Subject:        " + message.Subject,
		"Content type:   " + message.ContentType,
		"Correlation ID: " + message.CorrelationID,
		"",
		"Application properties:",
	}
	keys := make([]string, 0, len(message.ApplicationProperties))
	for key := range message.ApplicationProperties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("  %s: %v", key, message.ApplicationProperties[key]))
	}
	lines = append(lines, "", "Body:")
	return append(lines, prettyBodyLines(messageBodyText(message.Body))...)
}

// messageBodyText unwraps a body peeked as a JSON string, so text bodies
// show as written.
func messageBodyText(body json.RawMessage) string {
	var text string
	if json.Unmarshal(body, &text) == nil {
		return text
	}
	return string(body)
}

func (m Model) renderListenerMessageView() string {
	return mainWindowStyle.
		Height(m.height - windowChromeHeight).
		Width(m.width - 4).
		Render(m.listenerMessageViewport.View())
}
