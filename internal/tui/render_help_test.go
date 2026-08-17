package tui

import "testing"

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
