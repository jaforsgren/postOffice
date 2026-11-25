package tui

import (
	"postOffice/internal/history"
	"slices"
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

func TestSessionResume_OverlayViewReopensTheRequestsList(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	collPath := writeTempCollectionFile(t, "Resume Collection")

	parser := postman.NewParser()
	m := NewModel(parser, history.NewStore(""))
	m = m.loadCollection(collPath)
	m.mode = ModeRequests
	m = m.loadRequestsList()
	m.mode = ModeInfo

	m.saveSession()

	m2 := simulateRelaunch(t)

	if m2.mode != ModeRequests {
		t.Errorf("expected an Info session to reopen ModeRequests, got %v", m2.mode)
	}
	if m2.collection == nil || m2.collection.Info.Name != "Resume Collection" {
		t.Errorf("expected Resume Collection to be restored as active collection, got %+v", m2.collection)
	}
}

func TestRestorableSessionMode(t *testing.T) {
	cases := []struct {
		saved         ViewMode
		hasCollection bool
		want          ViewMode
	}{
		{ModeCollections, true, ModeCollections},
		{ModeEnvironments, false, ModeEnvironments},
		{ModeVariables, false, ModeVariables},
		{ModeRequests, true, ModeRequests},
		{ModeRequests, false, ModeCollections},
		{ModeInfo, true, ModeRequests},
		{ModeResponse, true, ModeRequests},
		{ModeInfo, false, ModeCollections},
	}
	for _, tc := range cases {
		if got := restorableSessionMode(tc.saved, tc.hasCollection); got != tc.want {
			t.Errorf("restorableSessionMode(%v, %v) = %v, want %v", tc.saved, tc.hasCollection, got, tc.want)
		}
	}
}

func TestSessionResume_ReopensTheCombinedView(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m, _, _ := combinedModel(t)
	m = cursorOn(t, openFolder(t, openFolder(t, m, combinedB), ordersFolder), ordersRequest)
	if err := m.parser.SaveState(); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	m.saveSession()
	m2 := simulateRelaunch(t)

	if !m2.isViewingMergedCollection() || m2.mode != ModeRequests {
		t.Fatalf("expected the combined view restored, got mode %v (%s)", m2.mode, m2.statusMessage)
	}
	if want := []string{combinedB, ordersFolder}; !slices.Equal(m2.breadcrumb, want) {
		t.Errorf("expected breadcrumb %v, got %v", want, m2.breadcrumb)
	}
	if item, ok := m2.selectedItem(); !ok || item.Name != ordersRequest {
		t.Errorf("expected the cursor on %s, got %+v", ordersRequest, item)
	}
}

func TestSessionResume_CombinedViewWithoutActiveCollectionsFallsBack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m, _, _ := combinedModel(t)
	m.mode = ModeRequests
	m.saveSession()
	m.parser.ToggleActiveCollection(combinedA)
	m.parser.ToggleActiveCollection(combinedB)
	if err := m.parser.SaveState(); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	m2 := simulateRelaunch(t)

	if m2.mode != ModeCollections || m2.collection != nil {
		t.Errorf("expected fallback to the collections list, got mode %v with %+v", m2.mode, m2.collection)
	}
}
