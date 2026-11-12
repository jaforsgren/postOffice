package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const topBarTitle = "PostOffice"

func TestToggleTopBar_HidesAndShowsTheTopBar(t *testing.T) {
	m := createTestModel()
	m.width, m.height = 100, 40

	m, _ = pressKey(m, runes(toggleTopBarKey))
	if strings.Contains(ansi.Strip(m.View()), topBarTitle) {
		t.Error("expected the top bar hidden after one press")
	}

	m, _ = pressKey(m, runes(toggleTopBarKey))
	if !strings.Contains(ansi.Strip(m.View()), topBarTitle) {
		t.Error("expected the top bar shown after a second press")
	}
}

func TestToggleTopBar_GivesItsRowsToTheContent(t *testing.T) {
	m := createTestModel()
	m.width, m.height = 100, 40
	shown := m.calculateLayout().contentHeight

	m, _ = pressKey(m, runes(toggleTopBarKey))

	if got := m.calculateLayout().contentHeight; got != shown+topBarHeight {
		t.Errorf("content height = %d, want %d", got, shown+topBarHeight)
	}
	if got := lipgloss.Height(m.View()); got != m.height {
		t.Errorf("view is %d lines, want the %d-line terminal filled", got, m.height)
	}
}

func TestToggleTopBar_WorksInEveryRegistryView(t *testing.T) {
	for _, mode := range viewSwitchModes {
		m := createTestModel()
		m.mode = mode

		m, _ = pressKey(m, runes(toggleTopBarKey))

		if !m.topBarHidden {
			t.Errorf("mode %v: expected %s to hide the top bar", mode, toggleTopBarKey)
		}
	}
}
