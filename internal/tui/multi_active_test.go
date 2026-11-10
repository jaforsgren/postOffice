package tui

import (
	"postOffice/internal/history"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestSpacebarKeypress_TogglesActiveCollection goes through the real
// tea.Model.Update dispatch (not the handler function directly) with the
// exact KeyMsg bubbletea produces for a physical spacebar press
// (Type: tea.KeySpace, whose String() is " ", not the literal word "space").
// A key binding registered with Keys: []string{"space"} would silently never
// match and this test would catch it.
func TestSpacebarKeypress_TogglesActiveCollection(t *testing.T) {
	pathA := writeTempCollectionFile(t, "Collection A")
	parser := createMockParser()
	parser.LoadCollection(pathA)

	m := NewModel(parser, history.NewStore(""))
	m.mode = ModeCollections
	m = m.loadCollectionsList()
	m.cursor = 0

	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = newModel.(Model)

	if !m.parser.IsCollectionActive("Collection A") {
		t.Fatal("expected a real spacebar keypress to toggle Collection A active")
	}
}

func TestSpacebarKeypress_TogglesActiveEnvironment(t *testing.T) {
	envPath := writeTempEnvironmentFile(t, "Dev")
	parser := createMockParser()
	parser.LoadEnvironment(envPath)

	m := NewModel(parser, history.NewStore(""))
	m.mode = ModeEnvironments
	m = m.loadEnvironmentsList()
	m.cursor = 0

	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = newModel.(Model)

	if !m.parser.IsEnvironmentActive("Dev") {
		t.Fatal("expected a real spacebar keypress to toggle Dev active")
	}
}

func TestHandleToggleActiveCollectionKey(t *testing.T) {
	pathA := writeTempCollectionFile(t, "Collection A")
	pathB := writeTempCollectionFile(t, "Collection B")
	parser := createMockParser()
	parser.LoadCollection(pathA)
	parser.LoadCollection(pathB)

	m := NewModel(parser, history.NewStore(""))
	m.mode = ModeCollections
	m = m.loadCollectionsList()
	m.cursor = 0 // "Collection A" (list is sorted)

	newModel, _ := handleToggleActiveCollectionKey(m)
	m = newModel

	if !m.parser.IsCollectionActive("Collection A") {
		t.Fatal("expected Collection A to become active after toggle")
	}

	newModel, _ = handleToggleActiveCollectionKey(m)
	m = newModel

	if m.parser.IsCollectionActive("Collection A") {
		t.Error("expected Collection A to become inactive after second toggle")
	}
}

func TestHandleSelection_ModeCollections_SingleActiveUsesCursor(t *testing.T) {
	pathA := writeTempCollectionFile(t, "Collection A")
	pathB := writeTempCollectionFile(t, "Collection B")
	parser := createMockParser()
	parser.LoadCollection(pathA)
	parser.LoadCollection(pathB)

	m := NewModel(parser, history.NewStore(""))
	m.mode = ModeCollections
	m = m.loadCollectionsList()
	m.cursor = 1 // "Collection B"

	m = m.handleSelection()

	if m.collection == nil || m.collection.Info.Name != "Collection B" {
		t.Fatalf("expected single-select to switch to Collection B, got %+v", m.collection)
	}
	if m.isViewingMergedCollection() {
		t.Error("expected single-select not to be a merged view")
	}
}

func TestHandleSelection_ModeCollections_MultiActiveShowsCombinedView(t *testing.T) {
	pathA := writeTempCollectionFile(t, "Collection A")
	pathB := writeTempCollectionFile(t, "Collection B")
	parser := createMockParser()
	parser.LoadCollection(pathA)
	parser.LoadCollection(pathB)
	parser.ToggleActiveCollection("Collection A")
	parser.ToggleActiveCollection("Collection B")

	m := NewModel(parser, history.NewStore(""))
	m.mode = ModeCollections
	m = m.loadCollectionsList()
	m.cursor = 0

	m = m.handleSelection()

	if m.mode != ModeRequests {
		t.Fatalf("expected combined view to switch to ModeRequests, got %v", m.mode)
	}
	if !m.isViewingMergedCollection() {
		t.Fatal("expected combined view to be flagged as a merged collection")
	}
	if len(m.collection.Items) != 2 {
		t.Fatalf("expected 2 top-level group folders, got %d", len(m.collection.Items))
	}
}

func TestHandleSelection_ModeEnvironments_MultiActiveMerges(t *testing.T) {
	pathDev := writeTempEnvironmentFile(t, "Dev")
	pathShared := writeTempEnvironmentFile(t, "Shared")
	parser := createMockParser()
	parser.LoadEnvironment(pathDev)
	parser.LoadEnvironment(pathShared)
	parser.ToggleActiveEnvironment("Dev")
	parser.ToggleActiveEnvironment("Shared")

	m := NewModel(parser, history.NewStore(""))
	m.mode = ModeEnvironments
	m = m.loadEnvironmentsList()
	m.cursor = 0

	m = m.handleSelection()

	if !m.isViewingMergedEnvironment() {
		t.Fatal("expected merged environment to be flagged")
	}
	if m.environment == nil || m.environment.Name != "Dev + Shared" {
		t.Fatalf("expected merged environment name 'Dev + Shared', got %+v", m.environment)
	}
}

func TestEnterEditMode_BlockedForMergedCollection(t *testing.T) {
	pathA := writeTempCollectionFile(t, "Collection A")
	pathB := writeTempCollectionFile(t, "Collection B")
	parser := createMockParser()
	parser.LoadCollection(pathA)
	parser.LoadCollection(pathB)
	parser.ToggleActiveCollection("Collection A")
	parser.ToggleActiveCollection("Collection B")

	m := NewModel(parser, history.NewStore(""))
	m.mode = ModeCollections
	m = m.loadCollectionsList()
	m.cursor = 0
	m = m.handleSelection()

	if len(m.currentItems) == 0 {
		t.Fatal("expected merged view to have at least one top-level group folder")
	}

	before := m.mode
	m = m.enterEditMode(m.currentItems[0], m.breadcrumb)

	if m.mode != before {
		t.Errorf("expected enterEditMode to be blocked (mode unchanged), got %v", m.mode)
	}
	if m.statusMessage == "" {
		t.Error("expected a status message explaining why editing is blocked")
	}
	if !m.statusIsError {
		t.Error("expected the blocked-edit message to be flagged as an error")
	}
}

func TestStatusIsError_ResetOnNextKeypress(t *testing.T) {
	pathA := writeTempCollectionFile(t, "Collection A")
	pathB := writeTempCollectionFile(t, "Collection B")
	parser := createMockParser()
	parser.LoadCollection(pathA)
	parser.LoadCollection(pathB)
	parser.ToggleActiveCollection("Collection A")
	parser.ToggleActiveCollection("Collection B")

	m := NewModel(parser, history.NewStore(""))
	m.mode = ModeCollections
	m = m.loadCollectionsList()
	m.cursor = 0
	m = m.handleSelection()
	m = m.enterEditMode(m.currentItems[0], m.breadcrumb)

	if !m.statusIsError {
		t.Fatal("expected blocked edit to set statusIsError")
	}

	newModel, _ := m.handleNormalMode(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = newModel.(Model)

	if m.statusIsError {
		t.Error("expected statusIsError to reset on the next keypress")
	}
}

func TestEnterEditMode_AllowedForRealSingleCollection(t *testing.T) {
	m := createTestModel()
	m = m.navigateInto(m.currentItems[0])
	request := m.currentItems[0]

	if m.isViewingMergedCollection() {
		t.Fatal("createTestModel's collection should not be treated as merged")
	}

	m = m.enterEditMode(request, m.breadcrumb)

	if m.mode != ModeEdit {
		t.Errorf("expected real single collection editing to proceed, got mode %v", m.mode)
	}
}

func TestActiveMarker_RenderedForActiveCollections(t *testing.T) {
	pathA := writeTempCollectionFile(t, "Collection A")
	parser := createMockParser()
	parser.LoadCollection(pathA)
	parser.ToggleActiveCollection("Collection A")

	m := NewModel(parser, history.NewStore(""))
	m.mode = ModeCollections
	m = m.loadCollectionsList()

	line := m.formatItemLine(0, itemNameWidth)
	if !strings.Contains(line, "[x]") {
		t.Errorf("expected active collection line to show [x] marker, got %q", line)
	}
}

func TestStatusBar_ShowsSpaceHintInCollectionsAndEnvironments(t *testing.T) {
	m := NewModel(createMockParser(), history.NewStore(""))
	m.width = 120

	m.mode = ModeCollections
	if got := m.renderStatusBar(); !strings.Contains(got, "space: toggle active") {
		t.Errorf("expected Collections status bar to hint at space toggle, got %q", got)
	}

	m.mode = ModeEnvironments
	if got := m.renderStatusBar(); !strings.Contains(got, "space: toggle active") {
		t.Errorf("expected Environments status bar to hint at space toggle, got %q", got)
	}

	m.mode = ModeRequests
	if got := m.renderStatusBar(); strings.Contains(got, "space: toggle active") {
		t.Errorf("did not expect Requests status bar to mention space toggle, got %q", got)
	}
}
