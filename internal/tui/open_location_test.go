package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"postOffice/internal/postman"
)

func writeTempCollectionFile(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "collection.json")
	data := fmt.Sprintf(`{"info":{"name":%q},"item":[]}`, name)
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatalf("failed to write temp collection file: %v", err)
	}
	return path
}

func writeTempEnvironmentFile(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "environment.json")
	data := fmt.Sprintf(`{"id":"1","name":%q,"values":[]}`, name)
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatalf("failed to write temp environment file: %v", err)
	}
	return path
}

func TestItemLocation_Collections(t *testing.T) {
	path := writeTempCollectionFile(t, "My Collection")
	parser := postman.NewParser()
	if _, err := parser.LoadCollection(path); err != nil {
		t.Fatalf("failed to load collection: %v", err)
	}

	m := NewModel(parser)
	m.mode = ModeCollections
	m = m.loadCollectionsList()

	got, exists := m.itemLocation("My Collection")
	if !exists {
		t.Fatal("expected location to be found for loaded collection")
	}
	if got != path {
		t.Errorf("expected path %q, got %q", path, got)
	}
}

func TestItemLocation_Environments(t *testing.T) {
	path := writeTempEnvironmentFile(t, "My Env")
	parser := postman.NewParser()
	if _, err := parser.LoadEnvironment(path); err != nil {
		t.Fatalf("failed to load environment: %v", err)
	}

	m := NewModel(parser)
	m.mode = ModeEnvironments
	m = m.loadEnvironmentsList()

	got, exists := m.itemLocation("My Env")
	if !exists {
		t.Fatal("expected location to be found for loaded environment")
	}
	if got != path {
		t.Errorf("expected path %q, got %q", path, got)
	}
}

func TestItemLocation_OtherModeReturnsFalse(t *testing.T) {
	m := createTestModel()
	m.mode = ModeRequests

	if _, exists := m.itemLocation("anything"); exists {
		t.Error("expected no location outside Collections/Environments modes")
	}
}

func TestHandleOpenCollectionLocationKey_NoSelection(t *testing.T) {
	parser := postman.NewParser()
	m := NewModel(parser)
	m.mode = ModeCollections
	m.items = []string{}

	m, _ = handleOpenCollectionLocationKey(m)

	if m.statusMessage != "No collection selected" {
		t.Errorf("unexpected status message: %q", m.statusMessage)
	}
}

func TestHandleOpenCollectionLocationKey_OpensResolvedPath(t *testing.T) {
	path := writeTempCollectionFile(t, "My Collection")
	parser := postman.NewParser()
	if _, err := parser.LoadCollection(path); err != nil {
		t.Fatalf("failed to load collection: %v", err)
	}

	m := NewModel(parser)
	m.mode = ModeCollections
	m = m.loadCollectionsList()
	m.cursor = 0

	var openedPath string
	original := openFileLocation
	openFileLocation = func(p string) error {
		openedPath = p
		return nil
	}
	defer func() { openFileLocation = original }()

	m, _ = handleOpenCollectionLocationKey(m)

	if openedPath != path {
		t.Errorf("expected openFileLocation called with %q, got %q", path, openedPath)
	}
}

func TestHandleOpenEnvironmentLocationKey_NoSelection(t *testing.T) {
	parser := postman.NewParser()
	m := NewModel(parser)
	m.mode = ModeEnvironments
	m.items = []string{}

	m, _ = handleOpenEnvironmentLocationKey(m)

	if m.statusMessage != "No environment selected" {
		t.Errorf("unexpected status message: %q", m.statusMessage)
	}
}

func TestHandleOpenEnvironmentLocationKey_OpensResolvedPath(t *testing.T) {
	path := writeTempEnvironmentFile(t, "My Env")
	parser := postman.NewParser()
	if _, err := parser.LoadEnvironment(path); err != nil {
		t.Fatalf("failed to load environment: %v", err)
	}

	m := NewModel(parser)
	m.mode = ModeEnvironments
	m = m.loadEnvironmentsList()
	m.cursor = 0

	var openedPath string
	original := openFileLocation
	openFileLocation = func(p string) error {
		openedPath = p
		return nil
	}
	defer func() { openFileLocation = original }()

	m, _ = handleOpenEnvironmentLocationKey(m)

	if openedPath != path {
		t.Errorf("expected openFileLocation called with %q, got %q", path, openedPath)
	}
}

func TestFormatItemLine_ShowsLocationColumnForCollections(t *testing.T) {
	path := writeTempCollectionFile(t, "My Collection")
	parser := postman.NewParser()
	if _, err := parser.LoadCollection(path); err != nil {
		t.Fatalf("failed to load collection: %v", err)
	}

	m := NewModel(parser)
	m.mode = ModeCollections
	m = m.loadCollectionsList()
	m.width = 200

	line := m.formatItemLine(0, itemNameWidth)
	if !strings.Contains(line, path) {
		t.Errorf("expected rendered line to contain location %q, got %q", path, line)
	}
}

func TestFormatItemLine_NoLocationColumnForRequests(t *testing.T) {
	m := createTestModel()
	m.width = 200

	line := m.formatItemLine(0, itemNameWidth)
	if strings.Contains(line, ".json") {
		t.Errorf("expected no location column in requests mode, got %q", line)
	}
}

func TestHandleOpenLocationKey_ReportsError(t *testing.T) {
	path := writeTempCollectionFile(t, "My Collection")
	parser := postman.NewParser()
	if _, err := parser.LoadCollection(path); err != nil {
		t.Fatalf("failed to load collection: %v", err)
	}

	m := NewModel(parser)
	m.mode = ModeCollections
	m = m.loadCollectionsList()
	m.cursor = 0

	original := openFileLocation
	openFileLocation = func(p string) error {
		return fmt.Errorf("boom")
	}
	defer func() { openFileLocation = original }()

	m, _ = handleOpenCollectionLocationKey(m)

	if m.statusMessage == "" || m.statusMessage == fmt.Sprintf("Opened location: %s", path) {
		t.Errorf("expected error status message, got %q", m.statusMessage)
	}
}
