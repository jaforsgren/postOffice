package tui

import (
	"os"
	"strings"
	"testing"
)

func TestRepeat_DuplicatesAgain(t *testing.T) {
	m, pathA, _ := combinedModel(t)
	m = cursorOn(t, openFolder(t, m, combinedA), usersRequest)

	m = pressKeys(t, m, runes("D"), runes(repeatLastActionKey))

	collectionA, _ := m.parser.GetCollection(combinedA)
	if len(collectionA.Items) != 3 {
		t.Errorf("expected two copies, got %d items (%s)", len(collectionA.Items), m.statusMessage)
	}
	if strings.Count(readFile(t, pathA), "(copy)") < 2 {
		t.Error("expected both copies saved")
	}
}

func TestRepeat_TogglesFavoriteOnTheNewCursorItem(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m, _, _ := combinedModel(t)
	m = cursorOn(t, openFolder(t, m, combinedA), usersRequest)
	m = pressKeys(t, m, runes("D"), runes("f"))
	m = pressKeys(t, m, runes("k"), runes(repeatLastActionKey))

	if len(m.favorites) != 2 {
		t.Errorf("expected both requests favorited, got %v", m.favorites)
	}
}

func TestRepeat_NothingYet(t *testing.T) {
	m, _, _ := combinedModel(t)

	m = pressKeys(t, m, runes(repeatLastActionKey))

	if m.statusMessage != "Nothing to repeat" {
		t.Errorf("got %q", m.statusMessage)
	}
}

func TestExpandKeys_ZExpandsAndRepeats(t *testing.T) {
	m, _, _ := combinedModel(t)
	m = cursorOn(t, m, combinedB)

	m = pressKeys(t, m, runes(expandFolderKey))
	if !strings.Contains(strings.Join(m.items, "\n"), ordersFolder) {
		t.Fatalf("expected z to expand %s, got %v", combinedB, m.items)
	}

	m = pressKeys(t, cursorOn(t, m, combinedB), runes(repeatLastActionKey))
	if strings.Contains(strings.Join(m.items, "\n"), ordersFolder) {
		t.Errorf("expected . to collapse it again, got %v", m.items)
	}
}

func TestExpandKeys_ShiftZExpandsAll(t *testing.T) {
	m, _, _ := combinedModel(t)

	m = pressKeys(t, m, runes(expandAllFoldersKey))

	if !strings.Contains(strings.Join(m.items, "\n"), ordersRequest) {
		t.Errorf("expected Z to expand every folder, got %v", m.items)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
