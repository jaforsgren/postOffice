package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func expandedNestedTestModel() Model {
	m := nestedTestModel()
	return m.toggleExpandAllAtCurrentLevel()
}

func TestJumpToFolder_NextSkipsRequests(t *testing.T) {
	m := expandedNestedTestModel()
	m.cursor = 2 // "Leaf Request", items: Outer, Inner, Leaf Request, Outer Request, Top Request

	m = m.jumpToFolder(-1)

	if got := m.currentItems[m.cursor].Name; got != "Inner" {
		t.Errorf("expected cursor on Inner, got %q", got)
	}
}

func TestJumpToFolder_DownFromFolderGoesToNextFolder(t *testing.T) {
	m := expandedNestedTestModel()
	m.cursor = 0

	m = m.jumpToFolder(1)

	if got := m.currentItems[m.cursor].Name; got != "Inner" {
		t.Errorf("expected cursor on Inner, got %q", got)
	}
}

func TestJumpToFolder_StaysWhenNoFolderInDirection(t *testing.T) {
	m := expandedNestedTestModel()
	m.cursor = 1 // "Inner", no folders below it

	m = m.jumpToFolder(1)

	if m.cursor != 1 {
		t.Errorf("expected cursor to stay at 1, got %d", m.cursor)
	}
}

func TestJumpToFolder_ShiftKeysRouteThroughRegistry(t *testing.T) {
	m := expandedNestedTestModel()
	m.cursor = 4 // "Top Request"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftUp})
	m = updated.(Model)
	if got := m.currentItems[m.cursor].Name; got != "Inner" {
		t.Fatalf("shift+up: expected cursor on Inner, got %q", got)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftUp})
	m = updated.(Model)
	if got := m.currentItems[m.cursor].Name; got != "Outer" {
		t.Fatalf("shift+up: expected cursor on Outer, got %q", got)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftDown})
	m = updated.(Model)
	if got := m.currentItems[m.cursor].Name; got != "Inner" {
		t.Errorf("shift+down: expected cursor on Inner, got %q", got)
	}
}
