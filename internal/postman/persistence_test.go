package postman

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveState_Success(t *testing.T) {
	parser := NewParser()

	collection1 := &Collection{Info: Info{Name: "Collection 1"}}
	collection2 := &Collection{Info: Info{Name: "Collection 2"}}

	path1 := createTempCollection(t, collection1)
	path2 := createTempCollection(t, collection2)

	parser.LoadCollection(path1)
	parser.LoadCollection(path2)

	env1 := &Environment{Name: "Development"}
	envPath1 := createTempEnvironment(t, env1)
	parser.LoadEnvironment(envPath1)

	tmpHome := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	err := parser.SaveState()

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	configPath := filepath.Join(tmpHome, configFileName)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Expected config file to be created")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	var config PersistenceConfig
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	if len(config.CollectionPaths) != 2 {
		t.Errorf("Expected 2 collection paths, got %d", len(config.CollectionPaths))
	}
	if len(config.EnvironmentPaths) != 1 {
		t.Errorf("Expected 1 environment path, got %d", len(config.EnvironmentPaths))
	}
}

func TestSaveState_EmptyParser(t *testing.T) {
	parser := NewParser()

	tmpHome := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	err := parser.SaveState()

	if err != nil {
		t.Fatalf("Expected no error for empty parser, got %v", err)
	}

	configPath := filepath.Join(tmpHome, configFileName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	var config PersistenceConfig
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	if len(config.CollectionPaths) != 0 {
		t.Errorf("Expected 0 collection paths, got %d", len(config.CollectionPaths))
	}
	if len(config.EnvironmentPaths) != 0 {
		t.Errorf("Expected 0 environment paths, got %d", len(config.EnvironmentPaths))
	}
}

func TestLoadState_FileNotExists(t *testing.T) {
	parser := NewParser()

	tmpHome := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	err := parser.LoadState()

	if err != nil {
		t.Errorf("Expected no error when file doesn't exist, got %v", err)
	}
}

func TestLoadState_Success(t *testing.T) {
	collection1 := &Collection{Info: Info{Name: "Restored Collection 1"}}
	collection2 := &Collection{Info: Info{Name: "Restored Collection 2"}}

	path1 := createTempCollection(t, collection1)
	path2 := createTempCollection(t, collection2)

	env1 := &Environment{Name: "Restored Environment"}
	envPath1 := createTempEnvironment(t, env1)

	tmpHome := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	config := PersistenceConfig{
		CollectionPaths:  []string{path1, path2},
		EnvironmentPaths: []string{envPath1},
	}

	configPath := filepath.Join(tmpHome, configFileName)
	data, _ := json.MarshalIndent(config, "", "  ")
	os.WriteFile(configPath, data, 0644)

	parser := NewParser()
	err := parser.LoadState()

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	collections := parser.ListCollections()
	if len(collections) != 2 {
		t.Errorf("Expected 2 collections loaded, got %d", len(collections))
	}

	environments := parser.ListEnvironments()
	if len(environments) != 1 {
		t.Errorf("Expected 1 environment loaded, got %d", len(environments))
	}

	_, exists := parser.GetCollection("Restored Collection 1")
	if !exists {
		t.Error("Expected Restored Collection 1 to be loaded")
	}

	_, exists = parser.GetCollection("Restored Collection 2")
	if !exists {
		t.Error("Expected Restored Collection 2 to be loaded")
	}

	_, exists = parser.GetEnvironment("Restored Environment")
	if !exists {
		t.Error("Expected Restored Environment to be loaded")
	}
}

func TestLoadState_InvalidJSON(t *testing.T) {
	tmpHome := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	configPath := filepath.Join(tmpHome, configFileName)
	invalidJSON := `{invalid json}`
	os.WriteFile(configPath, []byte(invalidJSON), 0644)

	parser := NewParser()
	err := parser.LoadState()

	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestLoadState_NonexistentPaths(t *testing.T) {
	tmpHome := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	config := PersistenceConfig{
		CollectionPaths: []string{
			"/nonexistent/collection1.json",
			"/nonexistent/collection2.json",
		},
		EnvironmentPaths: []string{
			"/nonexistent/env.json",
		},
	}

	configPath := filepath.Join(tmpHome, configFileName)
	data, _ := json.MarshalIndent(config, "", "  ")
	os.WriteFile(configPath, data, 0644)

	parser := NewParser()
	err := parser.LoadState()

	if err != nil {
		t.Fatalf("Expected no error (failures should be silent), got %v", err)
	}

	collections := parser.ListCollections()
	if len(collections) != 0 {
		t.Errorf("Expected 0 collections (all failed to load), got %d", len(collections))
	}
}

func TestSaveSession_Success(t *testing.T) {
	parser := NewParser()

	session := &Session{
		CollectionName:  "Active Collection",
		EnvironmentName: "Active Environment",
		Mode:            1,
		Breadcrumb:      []string{"Folder1", "Subfolder"},
		Cursor:          5,
	}

	tmpHome := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	err := parser.SaveSession(session)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	sessionPath := filepath.Join(tmpHome, sessionFileName)
	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		t.Error("Expected session file to be created")
	}

	data, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatalf("Failed to read session file: %v", err)
	}

	var savedSession Session
	if err := json.Unmarshal(data, &savedSession); err != nil {
		t.Fatalf("Failed to unmarshal session: %v", err)
	}

	if savedSession.CollectionName != "Active Collection" {
		t.Errorf("Expected CollectionName 'Active Collection', got %s", savedSession.CollectionName)
	}
	if savedSession.EnvironmentName != "Active Environment" {
		t.Errorf("Expected EnvironmentName 'Active Environment', got %s", savedSession.EnvironmentName)
	}
	if savedSession.Mode != 1 {
		t.Errorf("Expected Mode 1, got %d", savedSession.Mode)
	}
	if len(savedSession.Breadcrumb) != 2 {
		t.Errorf("Expected 2 breadcrumb items, got %d", len(savedSession.Breadcrumb))
	}
	if savedSession.Cursor != 5 {
		t.Errorf("Expected Cursor 5, got %d", savedSession.Cursor)
	}
}

func TestSaveSession_EmptySession(t *testing.T) {
	parser := NewParser()

	session := &Session{}

	tmpHome := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	err := parser.SaveSession(session)

	if err != nil {
		t.Fatalf("Expected no error for empty session, got %v", err)
	}

	sessionPath := filepath.Join(tmpHome, sessionFileName)
	data, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatalf("Failed to read session file: %v", err)
	}

	var savedSession Session
	if err := json.Unmarshal(data, &savedSession); err != nil {
		t.Fatalf("Failed to unmarshal session: %v", err)
	}

	if savedSession.CollectionName != "" {
		t.Error("Expected empty CollectionName")
	}
	if savedSession.EnvironmentName != "" {
		t.Error("Expected empty EnvironmentName")
	}
}

func TestLoadSession_FileNotExists(t *testing.T) {
	parser := NewParser()

	tmpHome := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	session, err := parser.LoadSession()

	if err != nil {
		t.Errorf("Expected no error when file doesn't exist, got %v", err)
	}
	if session != nil {
		t.Error("Expected nil session when file doesn't exist")
	}
}

func TestLoadSession_Success(t *testing.T) {
	tmpHome := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	originalSession := Session{
		CollectionName:  "Test Collection",
		EnvironmentName: "Test Environment",
		Mode:            2,
		Breadcrumb:      []string{"Folder", "Subfolder"},
		Cursor:          10,
	}

	sessionPath := filepath.Join(tmpHome, sessionFileName)
	data, _ := json.MarshalIndent(originalSession, "", "  ")
	os.WriteFile(sessionPath, data, 0644)

	parser := NewParser()
	session, err := parser.LoadSession()

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if session == nil {
		t.Fatal("Expected session to be loaded")
	}

	if session.CollectionName != "Test Collection" {
		t.Errorf("Expected CollectionName 'Test Collection', got %s", session.CollectionName)
	}
	if session.EnvironmentName != "Test Environment" {
		t.Errorf("Expected EnvironmentName 'Test Environment', got %s", session.EnvironmentName)
	}
	if session.Mode != 2 {
		t.Errorf("Expected Mode 2, got %d", session.Mode)
	}
	if len(session.Breadcrumb) != 2 {
		t.Errorf("Expected 2 breadcrumb items, got %d", len(session.Breadcrumb))
	}
	if session.Breadcrumb[0] != "Folder" || session.Breadcrumb[1] != "Subfolder" {
		t.Error("Expected correct breadcrumb values")
	}
	if session.Cursor != 10 {
		t.Errorf("Expected Cursor 10, got %d", session.Cursor)
	}
}

func TestLoadSession_InvalidJSON(t *testing.T) {
	tmpHome := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	sessionPath := filepath.Join(tmpHome, sessionFileName)
	invalidJSON := `{not valid json}`
	os.WriteFile(sessionPath, []byte(invalidJSON), 0644)

	parser := NewParser()
	_, err := parser.LoadSession()

	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestPersistenceConfig_Structure(t *testing.T) {
	config := PersistenceConfig{
		CollectionPaths: []string{
			"/path/to/collection1.json",
			"/path/to/collection2.json",
		},
		EnvironmentPaths: []string{
			"/path/to/env1.json",
		},
	}

	if len(config.CollectionPaths) != 2 {
		t.Errorf("Expected 2 collection paths, got %d", len(config.CollectionPaths))
	}
	if len(config.EnvironmentPaths) != 1 {
		t.Errorf("Expected 1 environment path, got %d", len(config.EnvironmentPaths))
	}
}

func TestSession_Structure(t *testing.T) {
	session := Session{
		CollectionName:  "My Collection",
		EnvironmentName: "My Environment",
		Mode:            3,
		Breadcrumb:      []string{"A", "B", "C"},
		Cursor:          7,
	}

	if session.CollectionName != "My Collection" {
		t.Error("Expected correct CollectionName")
	}
	if session.EnvironmentName != "My Environment" {
		t.Error("Expected correct EnvironmentName")
	}
	if session.Mode != 3 {
		t.Error("Expected correct Mode")
	}
	if len(session.Breadcrumb) != 3 {
		t.Error("Expected correct Breadcrumb length")
	}
	if session.Cursor != 7 {
		t.Error("Expected correct Cursor")
	}
}

func TestSaveAndLoadState_RoundTrip(t *testing.T) {
	parser1 := NewParser()

	collection1 := &Collection{Info: Info{Name: "RT Collection 1"}}
	collection2 := &Collection{Info: Info{Name: "RT Collection 2"}}
	env1 := &Environment{Name: "RT Environment"}

	path1 := createTempCollection(t, collection1)
	path2 := createTempCollection(t, collection2)
	envPath := createTempEnvironment(t, env1)

	parser1.LoadCollection(path1)
	parser1.LoadCollection(path2)
	parser1.LoadEnvironment(envPath)

	tmpHome := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	err := parser1.SaveState()
	if err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	parser2 := NewParser()
	err = parser2.LoadState()
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	collections := parser2.ListCollections()
	if len(collections) != 2 {
		t.Errorf("Expected 2 collections after round trip, got %d", len(collections))
	}

	environments := parser2.ListEnvironments()
	if len(environments) != 1 {
		t.Errorf("Expected 1 environment after round trip, got %d", len(environments))
	}

	_, exists := parser2.GetCollection("RT Collection 1")
	if !exists {
		t.Error("Expected RT Collection 1 after round trip")
	}

	_, exists = parser2.GetCollection("RT Collection 2")
	if !exists {
		t.Error("Expected RT Collection 2 after round trip")
	}

	_, exists = parser2.GetEnvironment("RT Environment")
	if !exists {
		t.Error("Expected RT Environment after round trip")
	}
}

func TestSaveAndLoadSession_RoundTrip(t *testing.T) {
	parser := NewParser()

	tmpHome := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	originalSession := &Session{
		CollectionName:  "RT Collection",
		EnvironmentName: "RT Environment",
		Mode:            4,
		Breadcrumb:      []string{"RT Folder"},
		Cursor:          15,
	}

	err := parser.SaveSession(originalSession)
	if err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	loadedSession, err := parser.LoadSession()
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}

	if loadedSession.CollectionName != originalSession.CollectionName {
		t.Error("CollectionName mismatch in round trip")
	}
	if loadedSession.EnvironmentName != originalSession.EnvironmentName {
		t.Error("EnvironmentName mismatch in round trip")
	}
	if loadedSession.Mode != originalSession.Mode {
		t.Error("Mode mismatch in round trip")
	}
	if len(loadedSession.Breadcrumb) != len(originalSession.Breadcrumb) {
		t.Error("Breadcrumb length mismatch in round trip")
	}
	if loadedSession.Cursor != originalSession.Cursor {
		t.Error("Cursor mismatch in round trip")
	}
}

func TestConfigFileName(t *testing.T) {
	if configFileName != ".postoffice_collections.json" {
		t.Errorf("Expected configFileName '.postoffice_collections.json', got %s", configFileName)
	}
}

func TestSessionFileName(t *testing.T) {
	if sessionFileName != ".postoffice_session.json" {
		t.Errorf("Expected sessionFileName '.postoffice_session.json', got %s", sessionFileName)
	}
}

func TestLoadState_PartialFailure(t *testing.T) {
	collection1 := &Collection{Info: Info{Name: "Valid Collection"}}
	path1 := createTempCollection(t, collection1)

	tmpHome := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	config := PersistenceConfig{
		CollectionPaths: []string{
			path1,
			"/nonexistent/path.json",
		},
		EnvironmentPaths: []string{},
	}

	configPath := filepath.Join(tmpHome, configFileName)
	data, _ := json.MarshalIndent(config, "", "  ")
	os.WriteFile(configPath, data, 0644)

	parser := NewParser()
	err := parser.LoadState()

	if err != nil {
		t.Fatalf("Expected no error with partial failure, got %v", err)
	}

	collections := parser.ListCollections()
	if len(collections) != 1 {
		t.Errorf("Expected 1 collection (valid one loaded), got %d", len(collections))
	}

	_, exists := parser.GetCollection("Valid Collection")
	if !exists {
		t.Error("Expected valid collection to be loaded despite other failures")
	}
}

func TestSaveSession_NilSession(t *testing.T) {
	parser := NewParser()

	tmpHome := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	err := parser.SaveSession(nil)

	if err != nil {
		t.Error("Expected to handle nil session gracefully")
	}
}

func TestMultipleSaveState_Overwrites(t *testing.T) {
	tmpHome := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	parser1 := NewParser()
	collection1 := &Collection{Info: Info{Name: "First Save"}}
	path1 := createTempCollection(t, collection1)
	parser1.LoadCollection(path1)
	parser1.SaveState()

	parser2 := NewParser()
	collection2 := &Collection{Info: Info{Name: "Second Save"}}
	path2 := createTempCollection(t, collection2)
	parser2.LoadCollection(path2)
	parser2.SaveState()

	parser3 := NewParser()
	parser3.LoadState()

	collections := parser3.ListCollections()
	if len(collections) != 1 {
		t.Errorf("Expected 1 collection (second save should overwrite), got %d", len(collections))
	}

	_, exists := parser3.GetCollection("Second Save")
	if !exists {
		t.Error("Expected 'Second Save' collection from latest state")
	}

	_, exists = parser3.GetCollection("First Save")
	if exists {
		t.Error("Did not expect 'First Save' collection (should be overwritten)")
	}
}
