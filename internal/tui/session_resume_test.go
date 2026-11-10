package tui

import (
	"postOffice/internal/history"
	"testing"

	"postOffice/internal/postman"
)

// simulateRelaunch mimics what main.go does on startup: a fresh Parser loads
// the persisted collection/environment paths and active sets, then NewModel
// restores the last session (mode/breadcrumb/cursor/collection/environment)
// on top of that.
func simulateRelaunch(t *testing.T) Model {
	t.Helper()
	parser := postman.NewParser()
	if err := parser.LoadState(); err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	return NewModel(parser, history.NewStore(""))
}

// TestQuit_FlushesStateEvenIfInMemorySetIsBehind guards the exit-time
// SaveState flush added to handleQuitKey. It simulates a second, concurrent
// session (parser2) saving a collection the current model's parser doesn't
// know about, then quits the current model and checks the second session's
// collection wasn't lost — SaveState's merge-on-write must survive being
// called from the quit path too, not just from load.
func TestQuit_FlushesStateEvenIfInMemorySetIsBehind(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	pathOwn := writeTempCollectionFile(t, "Owned By This Session")
	parser := postman.NewParser()
	m := NewModel(parser, history.NewStore(""))
	m = m.loadCollection(pathOwn)

	pathOther := writeTempCollectionFile(t, "Owned By Other Session")
	otherParser := postman.NewParser()
	otherParser.LoadCollection(pathOther)
	if err := otherParser.SaveState(); err != nil {
		t.Fatalf("other session's SaveState failed: %v", err)
	}

	m, _ = handleQuitKey(m)

	reloaded := postman.NewParser()
	if err := reloaded.LoadState(); err != nil {
		t.Fatalf("LoadState after quit failed: %v", err)
	}

	if _, exists := reloaded.GetCollection("Owned By This Session"); !exists {
		t.Error("expected this session's collection to survive quit")
	}
	if _, exists := reloaded.GetCollection("Owned By Other Session"); !exists {
		t.Error("expected the other session's collection not to be erased by this session's quit-time save")
	}
}

func TestSessionResume_LoadedCollectionsAndEnvironmentsSurviveRelaunch(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	collPath := writeTempCollectionFile(t, "Resume Collection")
	envPath := writeTempEnvironmentFile(t, "Resume Env")

	parser := postman.NewParser()
	m := NewModel(parser, history.NewStore(""))
	m = m.loadCollection(collPath)
	m = m.loadEnvironment(envPath)

	m2 := simulateRelaunch(t)

	if _, exists := m2.parser.GetCollection("Resume Collection"); !exists {
		t.Error("expected Resume Collection to still be loaded after relaunch")
	}
	if _, exists := m2.parser.GetEnvironment("Resume Env"); !exists {
		t.Error("expected Resume Env to still be loaded after relaunch")
	}
}

func TestSessionResume_ActiveEnvironmentsMergeAfterRelaunch(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	pathDev := writeTempEnvironmentFile(t, "Dev")
	pathShared := writeTempEnvironmentFile(t, "Shared")

	parser := postman.NewParser()
	m := NewModel(parser, history.NewStore(""))
	m = m.loadEnvironment(pathDev)
	m = m.loadEnvironment(pathShared)

	newModel, _ := handleToggleActiveEnvironmentKey(m)
	m = newModel
	m.mode = ModeEnvironments
	m = m.loadEnvironmentsList()
	m.cursor = 1 // "Shared" (list is sorted alphabetically: Dev, Shared)
	newModel, _ = handleToggleActiveEnvironmentKey(m)
	m = newModel

	m.saveSession()

	m2 := simulateRelaunch(t)

	if !m2.isViewingMergedEnvironment() {
		t.Fatal("expected merged environment to be restored as active after relaunch")
	}
	if m2.environment == nil || m2.environment.Name != "Dev + Shared" {
		t.Errorf("expected merged environment 'Dev + Shared', got %+v", m2.environment)
	}
}

func TestSessionResume_RequestsViewBreadcrumbSurvivesRelaunch(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	collPath := writeTempCollectionFile(t, "Resume Collection")

	parser := postman.NewParser()
	m := NewModel(parser, history.NewStore(""))
	m = m.loadCollection(collPath)
	m.mode = ModeRequests
	m.breadcrumb = []string{}
	m.cursor = 0

	m.saveSession()

	m2 := simulateRelaunch(t)

	if m2.mode != ModeRequests {
		t.Errorf("expected ModeRequests to be restored, got %v", m2.mode)
	}
	if m2.collection == nil || m2.collection.Info.Name != "Resume Collection" {
		t.Errorf("expected Resume Collection to be restored as active collection, got %+v", m2.collection)
	}
}

func TestSessionResume_MergedCollectionViewDoesNotBreakRestore(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	pathA := writeTempCollectionFile(t, "Collection A")
	pathB := writeTempCollectionFile(t, "Collection B")

	parser := postman.NewParser()
	m := NewModel(parser, history.NewStore(""))
	m = m.loadCollection(pathA)
	m = m.loadCollection(pathB)

	m.mode = ModeCollections
	m = m.loadCollectionsList()
	m.cursor = 0
	newModel, _ := handleToggleActiveCollectionKey(m)
	m = newModel
	m.cursor = 1
	newModel, _ = handleToggleActiveCollectionKey(m)
	m = newModel
	m.cursor = 0
	m = m.handleSelection() // enters the synthetic combined view

	if !m.isViewingMergedCollection() {
		t.Fatal("expected to be viewing the merged collection before saving session")
	}

	m.saveSession()

	m2 := simulateRelaunch(t)

	// The synthetic merged collection isn't itself restorable (it has no
	// backing file), so relaunch should fall back to the collections list
	// rather than getting stuck — and the active set (which is what actually
	// makes the combined view reproducible) should still be persisted.
	if m2.mode != ModeCollections {
		t.Errorf("expected fallback to ModeCollections, got %v", m2.mode)
	}
	if m2.parser.ActiveCollectionCount() != 2 {
		t.Errorf("expected both collections to still be active after relaunch, got %d", m2.parser.ActiveCollectionCount())
	}
}
