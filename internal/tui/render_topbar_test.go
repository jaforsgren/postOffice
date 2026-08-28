package tui

import (
	"strings"
	"testing"
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
