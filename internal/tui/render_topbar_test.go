package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestBuildLoadedNamesLine_None(t *testing.T) {
	m := NewModel(createMockParser())

	line := m.buildLoadedNamesLine("📁", "Collections", nil, func(string) bool { return false })

	if !strings.Contains(line, "none") {
		t.Errorf("expected 'none' for an empty list, got %q", line)
	}
}

func TestBuildLoadedNamesLine_ListsAllNames(t *testing.T) {
	m := NewModel(createMockParser())
	names := []string{"Alpha", "Beta"}

	line := m.buildLoadedNamesLine("📁", "Collections", names, func(string) bool { return false })

	if !strings.Contains(line, "Alpha") || !strings.Contains(line, "Beta") {
		t.Errorf("expected both loaded names in the line, got %q", line)
	}
}

func TestBuildLoadedNamesLine_TruncatesWithCount(t *testing.T) {
	m := NewModel(createMockParser())
	names := []string{
		"Collection One With A Long Name",
		"Collection Two With A Long Name",
		"Collection Three With A Long Name",
		"Collection Four With A Long Name",
	}

	line := m.buildLoadedNamesLine("📁", "Collections", names, func(string) bool { return false })

	if !strings.Contains(line, "+") {
		t.Errorf("expected a '+N' overflow indicator when names don't fit, got %q", line)
	}
}

func TestTopBar_ShowsAllLoadedCollectionsAndEnvironments(t *testing.T) {
	pathA := writeTempCollectionFile(t, "Collection Alpha")
	pathB := writeTempCollectionFile(t, "Collection Beta")
	envPath := writeTempEnvironmentFile(t, "Dev")

	parser := createMockParser()
	parser.LoadCollection(pathA)
	parser.LoadCollection(pathB)
	parser.LoadEnvironment(envPath)

	m := NewModel(parser)
	m.width = 160
	m.height = 40

	topBar := m.renderTopBar()

	if !strings.Contains(topBar, "Collection Alpha") || !strings.Contains(topBar, "Collection Beta") {
		t.Errorf("expected topbar to list all loaded collections, not just the active one:\n%s", topBar)
	}
	if !strings.Contains(topBar, "Dev") {
		t.Errorf("expected topbar to list loaded environments:\n%s", topBar)
	}
}

func TestTopBar_SeparatesContextEnvironmentsAndShortcuts(t *testing.T) {
	parser := createMockParser()
	parser.LoadEnvironment(writeTempEnvironmentFile(t, "Dev"))
	parser.LoadEnvironment(writeTempEnvironmentFile(t, "Staging"))
	parser.ToggleActiveEnvironment("Dev")

	m := NewModel(parser)
	m.width = 200
	m.height = 40
	m.mode = ModeRequests

	topBar := m.renderTopBar()

	for _, heading := range []string{"CONTEXT", "ENVIRONMENTS", "SHORTCUTS"} {
		if !strings.Contains(topBar, heading) {
			t.Errorf("expected a %s section:\n%s", heading, topBar)
		}
	}
	if !strings.Contains(topBar, strings.TrimSpace(topBarSeparator)) {
		t.Errorf("expected vertical rules between sections:\n%s", topBar)
	}
	if !strings.Contains(topBar, envActiveMarker+"Dev") || !strings.Contains(topBar, envInactiveMarker+"Staging") {
		t.Errorf("expected each loaded environment on its own row, active marked:\n%s", topBar)
	}
	t.Log("\n" + topBar)
}

func TestBuildEnvironmentsSection_OverflowShowsCount(t *testing.T) {
	parser := createMockParser()
	for _, name := range []string{"A", "B", "C", "D", "E", "F", "G"} {
		parser.LoadEnvironment(writeTempEnvironmentFile(t, name))
	}
	m := NewModel(parser)

	lines := m.buildEnvironmentsSection()

	if len(lines) != topBarSectionRows || !strings.Contains(lines[len(lines)-1], "+2 more") {
		t.Errorf("expected %d rows ending in '+2 more', got %q", topBarSectionRows, lines)
	}
}

func TestBuildShortcutsDisplay_DropsColumnsThatDoNotFit(t *testing.T) {
	m := NewModel(createMockParser())
	m.mode = ModeRequests

	narrow := m.buildShortcutsDisplay(1)
	wide := m.buildShortcutsDisplay(1000)

	if columnWidth(narrow) >= columnWidth(wide) {
		t.Errorf("expected fewer shortcut columns when narrow, got %d vs %d", columnWidth(narrow), columnWidth(wide))
	}
}

func TestView_FitsTheTerminalHeight(t *testing.T) {
	parser := createMockParser()
	for _, name := range []string{"Dev", "Staging", "Prod"} {
		parser.LoadEnvironment(writeTempEnvironmentFile(t, name))
	}
	for _, width := range []int{60, 100, 160, 240} {
		m := createTestModel()
		m.parser = parser
		m.width, m.height = width, 40
		m.statusMessage = "status"

		if got := lipgloss.Height(m.View()); got > m.height {
			t.Errorf("width %d: view is %d lines, taller than the %d-line terminal, so its top scrolls away", width, got, m.height)
		}
		if got := lipgloss.Height(m.renderTopBar()); got != topBarHeight {
			t.Errorf("width %d: top bar is %d lines, layout assumes %d", width, got, topBarHeight)
		}
	}
}
