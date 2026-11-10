package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// splitPane is a right-hand pane of the requests split view that can take
// focus to be scrolled.
type splitPane int

const (
	splitPaneNone splitPane = iota
	splitPaneInfo
	splitPaneResponse
	splitPaneCount
)

const (
	splitFocusInKey  = "right"
	splitFocusOutKey = "left"

	focusedPaneBorderColor = lipgloss.Color("11")

	splitFocusStatusMessage = "Scrolling %s pane: j/k scroll, g/G top/bottom, → response, ← back"
)

var splitPaneNames = map[splitPane]string{splitPaneInfo: "info", splitPaneResponse: "response"}

// handleSplitFocusInKey moves focus right: from the list into the info pane,
// then into the response pane.
func handleSplitFocusInKey(m Model) (Model, tea.Cmd) {
	if !m.splitViewActive {
		return m, nil
	}
	switch m.splitFocus {
	case splitPaneNone:
		return m.focusSplitPane(splitPaneInfo), nil
	case splitPaneInfo:
		return m.focusSplitPane(splitPaneResponse), nil
	}
	return m, nil
}

func (m Model) focusSplitPane(pane splitPane) Model {
	m.splitFocus = pane
	if pane == splitPaneNone {
		m.splitPaneScroll = [splitPaneCount]int{}
		m.statusMessage = ""
		return m
	}
	m.statusMessage = fmt.Sprintf(splitFocusStatusMessage, splitPaneNames[pane])
	return m
}

// handleSplitFocusKeys scrolls the focused pane. Any other key leaves focus
// and falls through to the requests view, so shortcuts keep working.
func (m Model) handleSplitFocusKeys(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	maxScroll := m.maxSplitPaneScroll(m.splitFocus)
	scroll := m.splitPaneScroll[m.splitFocus]
	pageSize := max(m.splitPaneHeight(m.splitFocus)-1, 1)

	switch msg.String() {
	case splitFocusOutKey, "esc":
		if m.splitFocus == splitPaneResponse {
			return m.focusSplitPane(splitPaneInfo), nil, true
		}
		return m.focusSplitPane(splitPaneNone), nil, true
	case splitFocusInKey:
		model, cmd := handleSplitFocusInKey(m)
		return model, cmd, true
	case "j", "down":
		scroll++
	case "k", "up":
		scroll--
	case "pgdown", "d":
		scroll += pageSize
	case "pgup", "u":
		scroll -= pageSize
	case "g", "home":
		scroll = 0
	case "G", "end":
		scroll = maxScroll
	default:
		return m.focusSplitPane(splitPaneNone), nil, false
	}
	m.splitPaneScroll[m.splitFocus] = min(max(scroll, 0), maxScroll)
	return m, nil, true
}

func (m Model) splitPaneLines(pane splitPane) []string {
	if pane == splitPaneInfo {
		return m.splitInfoLines()
	}
	content, _ := m.splitResponseContent()
	return strings.Split(content, "\n")
}

func (m Model) splitPaneHeight(pane splitPane) int {
	layout := m.calculateRequestSplitLayout()
	if pane == splitPaneInfo {
		return paneInnerHeight(layout.infoHeight)
	}
	return paneInnerHeight(layout.responseHeight)
}

func (m Model) maxSplitPaneScroll(pane splitPane) int {
	return maxPaneScroll(len(m.splitPaneLines(pane)), m.splitPaneHeight(pane))
}

func (m Model) splitPaneBorderColor(pane splitPane, unfocused lipgloss.Color) lipgloss.Color {
	if m.splitFocus == pane {
		return focusedPaneBorderColor
	}
	return unfocused
}
