package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	internalhttp "postOffice/internal/http"
	"postOffice/internal/servicebus"
)

const (
	layoutTestWidth  = 120
	layoutTestHeight = 36
	overflowingLines = 200
)

func overflowingContent() string {
	lines := make([]string, overflowingLines)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i)
	}
	return strings.Join(lines, "\n")
}

func overflowingListenedMessages() []listenedMessage {
	messages := make([]servicebus.PeekedMessage, overflowingLines)
	for i := range messages {
		messages[i] = peeked(int64(i+1), "claim.created", `{"id":1}`)
	}
	return listenedFrom(testSource("taps/claims", "claims"), messages...)
}

// layoutTestModels are views whose content is taller than the terminal.
func layoutTestModels() map[string]Model {
	base := createTestModel()
	base.width, base.height = layoutTestWidth, layoutTestHeight

	listeners := base
	listeners.mode = ModeListeners
	listeners.listenedMessages = overflowingListenedMessages()

	listenerMessage := base
	listenerMessage.mode = ModeListenerMessage
	listenerMessage.configureViewport(&listenerMessage.listenerMessageViewport, overflowingContent())

	response := base
	response.mode = ModeResponse
	response.lastResponse = &internalhttp.Response{Status: "200 OK"}
	response.configureViewport(&response.responseViewport, overflowingContent())

	json := base
	json.mode = ModeJSON
	json.jsonContent = overflowingContent()
	json.configureViewport(&json.jsonViewport, overflowingContent())

	return map[string]Model{
		"listeners":        listeners,
		"listener message": listenerMessage,
		"response":         response,
		"json":             json,
	}
}

func TestLayout_OverflowingViewsFillTheTerminalExactly(t *testing.T) {
	for name, m := range layoutTestModels() {
		if got := lipgloss.Height(m.View()); got != m.height {
			t.Errorf("%s with top bar: view is %d lines, want %d", name, got, m.height)
		}

		m, _ = pressKey(m, runes(toggleTopBarKey))
		if got := lipgloss.Height(m.View()); got != m.height {
			t.Errorf("%s without top bar: view is %d lines, want %d", name, got, m.height)
		}
	}
}

func TestLayout_WindowResizeFitsViewports(t *testing.T) {
	m := layoutTestModels()["response"]

	resized, _ := m.Update(tea.WindowSizeMsg{Width: layoutTestWidth, Height: layoutTestHeight + overflowingLines/10})
	m = resized.(Model)

	if got := lipgloss.Height(m.View()); got != m.height {
		t.Errorf("view is %d lines after resize, want %d", got, m.height)
	}
}
