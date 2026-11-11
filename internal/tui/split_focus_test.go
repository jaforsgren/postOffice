package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"postOffice/internal/http"
)

var (
	keyRight = tea.KeyMsg{Type: tea.KeyRight}
	keyLeft  = tea.KeyMsg{Type: tea.KeyLeft}
)

// splitFocusModel has the split view open on a request whose response body
// is far taller than the response pane.
func splitFocusModel(t *testing.T) Model {
	t.Helper()
	m := withExecutedResponse(modelWithCursorOn(t, formRequestName), formRequestName)
	m.width, m.height = splitTestWidth, splitTestHeight
	m.splitViewActive = true
	itemID := m.getRequestIdentifierByPath(m.collection.Info.Name, nil, formRequestName)
	m.requestExecutions[itemID].Response = &http.Response{Status: "200 OK", StatusCode: 200, Body: tallBody()}
	return m
}

const tallBodyLines = 200

func tallBody() string {
	var lines []string
	for i := range tallBodyLines {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	return strings.Join(lines, "\n")
}

func TestSplitFocus_RightFocusesInfoThenResponse(t *testing.T) {
	m := splitFocusModel(t)

	m = pressKeys(t, m, keyRight)
	if m.splitFocus != splitPaneInfo {
		t.Fatalf("expected the info pane focused, got %v", m.splitFocus)
	}
	m = pressKeys(t, m, keyRight)
	if m.splitFocus != splitPaneResponse {
		t.Fatalf("expected the response pane focused, got %v", m.splitFocus)
	}
}

func TestSplitFocus_LeftStepsBackThenExits(t *testing.T) {
	m := pressKeys(t, splitFocusModel(t), keyRight, keyRight)

	m = pressKeys(t, m, keyLeft)
	if m.splitFocus != splitPaneInfo {
		t.Fatalf("expected left to go back to the info pane, got %v", m.splitFocus)
	}
	m = pressKeys(t, m, keyLeft)
	if m.splitFocus != splitPaneNone || m.mode != ModeRequests {
		t.Fatalf("expected left to leave focus and stay in the requests view, got focus %v mode %v", m.splitFocus, m.mode)
	}
}

func TestSplitFocus_JScrollsThePaneNotTheCursor(t *testing.T) {
	m := pressKeys(t, splitFocusModel(t), keyRight, keyRight)
	cursor := m.cursor
	before := ansi.Strip(m.renderRequestSplitView())

	m = pressKeys(t, m, runes("j"), runes("j"), runes("j"))

	if m.cursor != cursor {
		t.Errorf("expected the cursor to stay put, moved from %d to %d", cursor, m.cursor)
	}
	if m.splitPaneScroll[splitPaneResponse] != 3 {
		t.Errorf("expected the response pane scrolled by 3, got %d", m.splitPaneScroll[splitPaneResponse])
	}
	if ansi.Strip(m.renderRequestSplitView()) == before {
		t.Error("expected the rendered response pane to change when scrolled")
	}
}

func TestSplitFocus_ScrollStopsAtTheEnds(t *testing.T) {
	m := pressKeys(t, splitFocusModel(t), keyRight, keyRight)

	m = pressKeys(t, m, runes("k"))
	if m.splitPaneScroll[splitPaneResponse] != 0 {
		t.Errorf("expected scrolling up at the top to stay at 0, got %d", m.splitPaneScroll[splitPaneResponse])
	}
	m = pressKeys(t, m, runes("G"), runes("j"))
	if got, want := m.splitPaneScroll[splitPaneResponse], m.maxSplitPaneScroll(splitPaneResponse); got != want || want == 0 {
		t.Errorf("expected G to stop at the last page (%d), got %d", want, got)
	}
}

func TestSplitFocus_OtherKeysLeaveFocus(t *testing.T) {
	m := pressKeys(t, splitFocusModel(t), keyRight)

	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyTab})

	if m.splitFocus != splitPaneNone || m.splitViewActive {
		t.Errorf("expected tab to leave focus and close the split view, got focus %v split %v", m.splitFocus, m.splitViewActive)
	}
}

func TestSplitFocus_RightWithoutSplitViewDoesNothing(t *testing.T) {
	m := modelWithCursorOn(t, formRequestName)

	m = pressKeys(t, m, keyRight)

	if m.splitFocus != splitPaneNone {
		t.Errorf("expected no focus outside the split view, got %v", m.splitFocus)
	}
}
