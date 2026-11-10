package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestOpenHelp_ColonCommandShowsAllContextsGrouped(t *testing.T) {
	m := createTestModel()
	m.mode = ModeRequests

	m, _ = m.openHelp(true)

	if !m.helpShowAll {
		t.Fatal("expected :h to set helpShowAll true")
	}
	if m.mode != ModeHelp {
		t.Fatalf("expected mode ModeHelp, got %v", m.mode)
	}

	groups := m.buildHelpGroups()
	if len(groups) < 2 {
		t.Fatalf("expected shortcuts grouped across multiple contexts, got %d group(s)", len(groups))
	}

	titles := make(map[string]bool)
	for _, g := range groups {
		titles[g.title] = true
	}
	for _, want := range []string{"Collections", "Requests", "Environments"} {
		if !titles[want] {
			t.Errorf("expected a %q group in :h output, got groups: %v", want, titles)
		}
	}
}

func TestOpenHelp_QuestionMarkKeyShowsOnlyCurrentContext(t *testing.T) {
	m := createTestModel()
	m.mode = ModeRequests

	m, _ = m.openHelp(false)

	if m.helpShowAll {
		t.Fatal("expected ? key to set helpShowAll false")
	}

	groups := m.buildHelpGroups()
	if len(groups) != 1 {
		t.Fatalf("expected exactly 1 group for current context only, got %d", len(groups))
	}
	if groups[0].title != "Requests" {
		t.Errorf("expected group title Requests, got %q", groups[0].title)
	}
}

func TestBuildHelpGroups_EntriesDeduplicatedPerGroup(t *testing.T) {
	m := createTestModel()
	m.mode = ModeRequests
	m.helpShowAll = false

	groups := m.buildHelpGroups()
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}

	seen := make(map[string]bool)
	for _, entry := range groups[0].entries {
		if seen[entry.key] {
			t.Errorf("duplicate key entry %q within Requests group", entry.key)
		}
		seen[entry.key] = true
	}
}

func TestBuildHelpContent_AllContextsIncludesCommandsSection(t *testing.T) {
	m := createTestModel()
	m.helpShowAll = true

	content := m.buildHelpContent()

	for _, want := range []string{"Keyboard Shortcuts — All Contexts", "Commands", "Collections", "Requests"} {
		if !contains(content, want) {
			t.Errorf("expected all-contexts help content to contain %q, got:\n%s", want, content)
		}
	}
}

func TestBuildHelpContent_WideTerminalPutsCommandsBesideShortcuts(t *testing.T) {
	m := createTestModel()
	m.helpShowAll = true
	m.width = helpTwoColumnMinWidth

	lines := strings.Split(ansi.Strip(m.buildHelpContent()), "\n")

	if !slices.ContainsFunc(lines, func(line string) bool {
		return strings.Contains(line, "Collections") && strings.Contains(line, helpCommandsTitle)
	}) {
		t.Error("expected the Commands heading on the same line as the first shortcuts heading")
	}
}

func TestBuildHelpContent_NarrowTerminalStacksCommandsUnderShortcuts(t *testing.T) {
	m := createTestModel()
	m.helpShowAll = true
	m.width = helpTwoColumnMinWidth - 1

	content := ansi.Strip(m.buildHelpContent())

	if strings.Index(content, helpCommandsTitle) < strings.LastIndex(content, "Saved Responses") {
		t.Error("expected the Commands section below every shortcuts section")
	}
}

func TestBuildCommandHelpEntries_SortedWithAliases(t *testing.T) {
	m := createTestModel()

	entries := m.buildCommandHelpEntries()

	if !slices.IsSortedFunc(entries, func(a, b helpEntry) int { return strings.Compare(a.key, b.key) }) {
		t.Errorf("expected commands sorted by name, got %v", entries)
	}
	if !slices.ContainsFunc(entries, func(e helpEntry) bool { return e.key == ":run / :runner" }) {
		t.Errorf("expected :run with its :runner alias, got %v", entries)
	}
}

func TestBuildHelpGroups_IncludesShortcutsFromNewViews(t *testing.T) {
	m := createTestModel()
	m.helpShowAll = true

	keysByGroup := make(map[string][]string)
	for _, group := range m.buildHelpGroups() {
		for _, entry := range group.entries {
			keysByGroup[group.title] = append(keysByGroup[group.title], entry.key)
		}
	}

	cases := map[string]string{
		"Requests":     "<tab>",
		"Workflow Run": "<L>",
	}
	for group, key := range cases {
		if !slices.Contains(keysByGroup[group], key) {
			t.Errorf("expected %s in the %s group, got %v", key, group, keysByGroup[group])
		}
	}
	if !slices.ContainsFunc(keysByGroup["Requests"], func(key string) bool { return strings.Contains(key, "ctrl+e") }) {
		t.Errorf("expected ctrl+e in the Requests group, got %v", keysByGroup["Requests"])
	}
}

func TestHelpModeTitle_SharedSectionCoversEachMode(t *testing.T) {
	for _, mode := range []ViewMode{ModeInfo, ModeJSON, ModeLog} {
		if got := helpModeTitle(mode); got != "Info / JSON / Logs" {
			t.Errorf("helpModeTitle(%v) = %q, want the shared Info / JSON / Logs section", mode, got)
		}
	}
}
