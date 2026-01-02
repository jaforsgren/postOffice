package postman

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func createTempCollection(t *testing.T, collection *Collection) string {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "collection.json")

	data, err := json.MarshalIndent(collection, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal collection: %v", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		t.Fatalf("Failed to write collection file: %v", err)
	}

	return filePath
}

func createTempEnvironment(t *testing.T, env *Environment) string {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "environment.json")

	data, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal environment: %v", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		t.Fatalf("Failed to write environment file: %v", err)
	}

	return filePath
}

func TestNewParser(t *testing.T) {
	parser := NewParser()

	if parser == nil {
		t.Fatal("Expected parser to be created")
	}
	if parser.collections == nil {
		t.Error("Expected collections map to be initialized")
	}
	if parser.pathMap == nil {
		t.Error("Expected pathMap to be initialized")
	}
	if parser.environments == nil {
		t.Error("Expected environments map to be initialized")
	}
	if parser.envPathMap == nil {
		t.Error("Expected envPathMap to be initialized")
	}
}

func TestLoadCollection_Success(t *testing.T) {
	collection := &Collection{
		Info: Info{
			Name:        "Test Collection",
			Description: "A test collection",
			Schema:      "https://schema.getpostman.com/json/collection/v2.1.0/collection.json",
		},
		Items: []Item{
			{
				Name: "Test Request",
				Request: &Request{
					Method: "GET",
					URL: URL{
						Raw: "https://example.com/api/test",
					},
				},
			},
		},
	}

	filePath := createTempCollection(t, collection)
	parser := NewParser()

	loadedCollection, err := parser.LoadCollection(filePath)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if loadedCollection == nil {
		t.Fatal("Expected collection to be loaded")
	}
	if loadedCollection.Info.Name != "Test Collection" {
		t.Errorf("Expected name 'Test Collection', got %s", loadedCollection.Info.Name)
	}
	if len(loadedCollection.Items) != 1 {
		t.Errorf("Expected 1 item, got %d", len(loadedCollection.Items))
	}
}

func TestLoadCollection_WithTilde(t *testing.T) {
	parser := NewParser()

	_, err := parser.LoadCollection("~/nonexistent.json")

	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestLoadCollection_InvalidPath(t *testing.T) {
	parser := NewParser()

	_, err := parser.LoadCollection("/invalid/path/collection.json")

	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestLoadCollection_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "invalid.json")

	invalidJSON := `{"invalid": json content}`
	if err := os.WriteFile(filePath, []byte(invalidJSON), 0644); err != nil {
		t.Fatalf("Failed to write invalid JSON: %v", err)
	}

	parser := NewParser()
	_, err := parser.LoadCollection(filePath)

	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestLoadCollection_StoresInMaps(t *testing.T) {
	collection := &Collection{
		Info: Info{Name: "Stored Collection"},
		Items: []Item{
			{Name: "Item 1"},
		},
	}

	filePath := createTempCollection(t, collection)
	parser := NewParser()

	_, err := parser.LoadCollection(filePath)
	if err != nil {
		t.Fatalf("Failed to load collection: %v", err)
	}

	storedCollection, exists := parser.collections["Stored Collection"]
	if !exists {
		t.Error("Expected collection to be stored in collections map")
	}
	if storedCollection == nil {
		t.Fatal("Expected stored collection to not be nil")
	}
	if storedCollection.Info.Name != "Stored Collection" {
		t.Errorf("Expected name 'Stored Collection', got %s", storedCollection.Info.Name)
	}

	storedPath, exists := parser.pathMap["Stored Collection"]
	if !exists {
		t.Error("Expected path to be stored in pathMap")
	}
	if storedPath != filePath {
		t.Errorf("Expected path %s, got %s", filePath, storedPath)
	}
}

func TestGetCollection_Exists(t *testing.T) {
	collection := &Collection{
		Info: Info{Name: "Get Test Collection"},
	}

	filePath := createTempCollection(t, collection)
	parser := NewParser()
	parser.LoadCollection(filePath)

	retrieved, exists := parser.GetCollection("Get Test Collection")

	if !exists {
		t.Error("Expected collection to exist")
	}
	if retrieved == nil {
		t.Fatal("Expected retrieved collection to not be nil")
	}
	if retrieved.Info.Name != "Get Test Collection" {
		t.Errorf("Expected name 'Get Test Collection', got %s", retrieved.Info.Name)
	}
}

func TestGetCollection_NotExists(t *testing.T) {
	parser := NewParser()

	_, exists := parser.GetCollection("Nonexistent Collection")

	if exists {
		t.Error("Expected collection to not exist")
	}
}

func TestGetCollectionPath_Exists(t *testing.T) {
	collection := &Collection{
		Info: Info{Name: "Path Test Collection"},
	}

	filePath := createTempCollection(t, collection)
	parser := NewParser()
	parser.LoadCollection(filePath)

	path, exists := parser.GetCollectionPath("Path Test Collection")

	if !exists {
		t.Error("Expected path to exist")
	}
	if path != filePath {
		t.Errorf("Expected path %s, got %s", filePath, path)
	}
}

func TestGetCollectionPath_NotExists(t *testing.T) {
	parser := NewParser()

	_, exists := parser.GetCollectionPath("Nonexistent")

	if exists {
		t.Error("Expected path to not exist")
	}
}

func TestListCollections_Empty(t *testing.T) {
	parser := NewParser()

	collections := parser.ListCollections()

	if len(collections) != 0 {
		t.Errorf("Expected 0 collections, got %d", len(collections))
	}
}

func TestListCollections_Multiple(t *testing.T) {
	parser := NewParser()

	collection1 := &Collection{Info: Info{Name: "Collection 1"}}
	collection2 := &Collection{Info: Info{Name: "Collection 2"}}
	collection3 := &Collection{Info: Info{Name: "Collection 3"}}

	path1 := createTempCollection(t, collection1)
	path2 := createTempCollection(t, collection2)
	path3 := createTempCollection(t, collection3)

	parser.LoadCollection(path1)
	parser.LoadCollection(path2)
	parser.LoadCollection(path3)

	collections := parser.ListCollections()

	if len(collections) != 3 {
		t.Errorf("Expected 3 collections, got %d", len(collections))
	}

	collectionMap := make(map[string]bool)
	for _, name := range collections {
		collectionMap[name] = true
	}

	if !collectionMap["Collection 1"] {
		t.Error("Expected Collection 1 in list")
	}
	if !collectionMap["Collection 2"] {
		t.Error("Expected Collection 2 in list")
	}
	if !collectionMap["Collection 3"] {
		t.Error("Expected Collection 3 in list")
	}
}

func TestLoadEnvironment_Success(t *testing.T) {
	env := &Environment{
		ID:   "env-123",
		Name: "Test Environment",
		Values: []EnvVariable{
			{Key: "apiKey", Value: "secret", Enabled: true},
		},
	}

	filePath := createTempEnvironment(t, env)
	parser := NewParser()

	loadedEnv, err := parser.LoadEnvironment(filePath)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if loadedEnv == nil {
		t.Fatal("Expected environment to be loaded")
	}
	if loadedEnv.Name != "Test Environment" {
		t.Errorf("Expected name 'Test Environment', got %s", loadedEnv.Name)
	}
	if len(loadedEnv.Values) != 1 {
		t.Errorf("Expected 1 value, got %d", len(loadedEnv.Values))
	}
}

func TestLoadEnvironment_InvalidPath(t *testing.T) {
	parser := NewParser()

	_, err := parser.LoadEnvironment("/invalid/path/env.json")

	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestLoadEnvironment_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "invalid.json")

	invalidJSON := `not valid json`
	if err := os.WriteFile(filePath, []byte(invalidJSON), 0644); err != nil {
		t.Fatalf("Failed to write invalid JSON: %v", err)
	}

	parser := NewParser()
	_, err := parser.LoadEnvironment(filePath)

	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestLoadEnvironment_StoresInMaps(t *testing.T) {
	env := &Environment{
		Name: "Stored Environment",
		Values: []EnvVariable{
			{Key: "key1", Value: "value1", Enabled: true},
		},
	}

	filePath := createTempEnvironment(t, env)
	parser := NewParser()

	_, err := parser.LoadEnvironment(filePath)
	if err != nil {
		t.Fatalf("Failed to load environment: %v", err)
	}

	storedEnv, exists := parser.environments["Stored Environment"]
	if !exists {
		t.Error("Expected environment to be stored in environments map")
	}
	if storedEnv == nil {
		t.Fatal("Expected stored environment to not be nil")
	}

	storedPath, exists := parser.envPathMap["Stored Environment"]
	if !exists {
		t.Error("Expected path to be stored in envPathMap")
	}
	if storedPath != filePath {
		t.Errorf("Expected path %s, got %s", filePath, storedPath)
	}
}

func TestGetEnvironment_Exists(t *testing.T) {
	env := &Environment{
		Name: "Get Test Environment",
	}

	filePath := createTempEnvironment(t, env)
	parser := NewParser()
	parser.LoadEnvironment(filePath)

	retrieved, exists := parser.GetEnvironment("Get Test Environment")

	if !exists {
		t.Error("Expected environment to exist")
	}
	if retrieved == nil {
		t.Fatal("Expected retrieved environment to not be nil")
	}
	if retrieved.Name != "Get Test Environment" {
		t.Errorf("Expected name 'Get Test Environment', got %s", retrieved.Name)
	}
}

func TestGetEnvironment_NotExists(t *testing.T) {
	parser := NewParser()

	_, exists := parser.GetEnvironment("Nonexistent Environment")

	if exists {
		t.Error("Expected environment to not exist")
	}
}

func TestListEnvironments_Empty(t *testing.T) {
	parser := NewParser()

	environments := parser.ListEnvironments()

	if len(environments) != 0 {
		t.Errorf("Expected 0 environments, got %d", len(environments))
	}
}

func TestListEnvironments_Multiple(t *testing.T) {
	parser := NewParser()

	env1 := &Environment{Name: "Development"}
	env2 := &Environment{Name: "Staging"}
	env3 := &Environment{Name: "Production"}

	path1 := createTempEnvironment(t, env1)
	path2 := createTempEnvironment(t, env2)
	path3 := createTempEnvironment(t, env3)

	parser.LoadEnvironment(path1)
	parser.LoadEnvironment(path2)
	parser.LoadEnvironment(path3)

	environments := parser.ListEnvironments()

	if len(environments) != 3 {
		t.Errorf("Expected 3 environments, got %d", len(environments))
	}

	envMap := make(map[string]bool)
	for _, name := range environments {
		envMap[name] = true
	}

	if !envMap["Development"] {
		t.Error("Expected Development in list")
	}
	if !envMap["Staging"] {
		t.Error("Expected Staging in list")
	}
	if !envMap["Production"] {
		t.Error("Expected Production in list")
	}
}

func TestSaveCollection_Success(t *testing.T) {
	collection := &Collection{
		Info: Info{Name: "Save Test Collection"},
		Items: []Item{
			{Name: "Original Item"},
		},
	}

	filePath := createTempCollection(t, collection)
	parser := NewParser()
	parser.LoadCollection(filePath)

	retrieved, _ := parser.GetCollection("Save Test Collection")
	retrieved.Items = append(retrieved.Items, Item{Name: "New Item"})

	err := parser.SaveCollection("Save Test Collection")

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}

	var savedCollection Collection
	if err := json.Unmarshal(data, &savedCollection); err != nil {
		t.Fatalf("Failed to unmarshal saved collection: %v", err)
	}

	if len(savedCollection.Items) != 2 {
		t.Errorf("Expected 2 items in saved collection, got %d", len(savedCollection.Items))
	}
}

func TestSaveCollection_NotFound(t *testing.T) {
	parser := NewParser()

	err := parser.SaveCollection("Nonexistent Collection")

	if err == nil {
		t.Error("Expected error for nonexistent collection")
	}
}

func TestSaveCollection_NoPath(t *testing.T) {
	parser := NewParser()
	parser.collections["Test"] = &Collection{Info: Info{Name: "Test"}}

	err := parser.SaveCollection("Test")

	if err == nil {
		t.Error("Expected error when path not found")
	}
}

func TestSaveEnvironment_Success(t *testing.T) {
	env := &Environment{
		Name: "Save Test Environment",
		Values: []EnvVariable{
			{Key: "original", Value: "value", Enabled: true},
		},
	}

	filePath := createTempEnvironment(t, env)
	parser := NewParser()
	parser.LoadEnvironment(filePath)

	retrieved, _ := parser.GetEnvironment("Save Test Environment")
	retrieved.Values = append(retrieved.Values, EnvVariable{Key: "new", Value: "newValue", Enabled: true})

	err := parser.SaveEnvironment("Save Test Environment")

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}

	var savedEnv Environment
	if err := json.Unmarshal(data, &savedEnv); err != nil {
		t.Fatalf("Failed to unmarshal saved environment: %v", err)
	}

	if len(savedEnv.Values) != 2 {
		t.Errorf("Expected 2 values in saved environment, got %d", len(savedEnv.Values))
	}
}

func TestSaveEnvironment_NotFound(t *testing.T) {
	parser := NewParser()

	err := parser.SaveEnvironment("Nonexistent Environment")

	if err == nil {
		t.Error("Expected error for nonexistent environment")
	}
}

func TestSaveEnvironment_NoPath(t *testing.T) {
	parser := NewParser()
	parser.environments["Test"] = &Environment{Name: "Test"}

	err := parser.SaveEnvironment("Test")

	if err == nil {
		t.Error("Expected error when path not found")
	}
}

func TestExpandPath_WithTilde(t *testing.T) {
	result, err := expandPath("~/test/file.json")

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result == "~/test/file.json" {
		t.Error("Expected tilde to be expanded")
	}
	if result[0] == '~' {
		t.Error("Expected tilde to be replaced with home directory")
	}
}

func TestExpandPath_WithoutTilde(t *testing.T) {
	input := "/absolute/path/file.json"
	result, err := expandPath(input)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result != input {
		t.Errorf("Expected path to remain unchanged, got %s", result)
	}
}

func TestExpandPath_WithWhitespace(t *testing.T) {
	input := "  /path/with/spaces  "
	result, err := expandPath(input)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result != "/path/with/spaces" {
		t.Errorf("Expected whitespace to be trimmed, got '%s'", result)
	}
}

func TestMultipleCollections(t *testing.T) {
	parser := NewParser()

	collection1 := &Collection{Info: Info{Name: "Collection A"}}
	collection2 := &Collection{Info: Info{Name: "Collection B"}}

	path1 := createTempCollection(t, collection1)
	path2 := createTempCollection(t, collection2)

	parser.LoadCollection(path1)
	parser.LoadCollection(path2)

	col1, exists1 := parser.GetCollection("Collection A")
	col2, exists2 := parser.GetCollection("Collection B")

	if !exists1 || !exists2 {
		t.Error("Expected both collections to exist")
	}
	if col1.Info.Name != "Collection A" {
		t.Error("Expected Collection A")
	}
	if col2.Info.Name != "Collection B" {
		t.Error("Expected Collection B")
	}

	collections := parser.ListCollections()
	if len(collections) != 2 {
		t.Errorf("Expected 2 collections in list, got %d", len(collections))
	}
}

func TestMultipleEnvironments(t *testing.T) {
	parser := NewParser()

	env1 := &Environment{Name: "Dev"}
	env2 := &Environment{Name: "Prod"}

	path1 := createTempEnvironment(t, env1)
	path2 := createTempEnvironment(t, env2)

	parser.LoadEnvironment(path1)
	parser.LoadEnvironment(path2)

	e1, exists1 := parser.GetEnvironment("Dev")
	e2, exists2 := parser.GetEnvironment("Prod")

	if !exists1 || !exists2 {
		t.Error("Expected both environments to exist")
	}
	if e1.Name != "Dev" {
		t.Error("Expected Dev environment")
	}
	if e2.Name != "Prod" {
		t.Error("Expected Prod environment")
	}

	environments := parser.ListEnvironments()
	if len(environments) != 2 {
		t.Errorf("Expected 2 environments in list, got %d", len(environments))
	}
}

func TestSaveCollection_AtomicWrite(t *testing.T) {
	collection := &Collection{
		Info: Info{Name: "Atomic Test"},
		Items: []Item{
			{Name: "Item 1"},
		},
	}

	filePath := createTempCollection(t, collection)
	parser := NewParser()
	parser.LoadCollection(filePath)

	retrieved, _ := parser.GetCollection("Atomic Test")
	retrieved.Items[0].Name = "Modified Item"

	err := parser.SaveCollection("Atomic Test")
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	tmpPath := filePath + ".tmp"
	if _, err := os.Stat(tmpPath); err == nil {
		t.Error("Expected temp file to be removed after successful save")
	}
}

func TestLoadCollection_ComplexStructure(t *testing.T) {
	collection := &Collection{
		Info: Info{
			Name:        "Complex Collection",
			Description: "Multi-level structure",
		},
		Variables: []Variable{
			{Key: "baseUrl", Value: "https://api.example.com"},
		},
		Items: []Item{
			{
				Name: "Folder 1",
				Items: []Item{
					{
						Name: "Subfolder",
						Items: []Item{
							{
								Name: "Deep Request",
								Request: &Request{
									Method: "GET",
									URL: URL{
										Raw: "{{baseUrl}}/users",
									},
									Header: []Header{
										{Key: "Authorization", Value: "Bearer {{token}}"},
									},
								},
							},
						},
						Variables: []Variable{
							{Key: "token", Value: "abc123"},
						},
					},
				},
			},
		},
	}

	filePath := createTempCollection(t, collection)
	parser := NewParser()

	loaded, err := parser.LoadCollection(filePath)

	if err != nil {
		t.Fatalf("Failed to load complex collection: %v", err)
	}
	if len(loaded.Variables) != 1 {
		t.Error("Expected collection variables")
	}
	if len(loaded.Items) != 1 {
		t.Error("Expected top-level folder")
	}
	if len(loaded.Items[0].Items) != 1 {
		t.Error("Expected subfolder")
	}
	if len(loaded.Items[0].Items[0].Items) != 1 {
		t.Error("Expected deep request")
	}
}

func TestLoadCollection_OpenAPI(t *testing.T) {
	openAPISpec := `{
		"openapi": "3.0.0",
		"info": {
			"title": "Test OpenAPI",
			"description": "Test Description",
			"version": "1.0.0"
		},
		"servers": [
			{"url": "https://api.example.com"}
		],
		"paths": {
			"/users": {
				"get": {
					"operationId": "listUsers",
					"summary": "List all users",
					"tags": ["users"]
				}
			},
			"/users/{id}": {
				"get": {
					"operationId": "getUser",
					"summary": "Get user by ID",
					"tags": ["users"],
					"parameters": [
						{
							"name": "id",
							"in": "path",
							"required": true
						}
					]
				}
			}
		}
	}`

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "openapi.json")
	if err := os.WriteFile(filePath, []byte(openAPISpec), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	parser := NewParser()
	collection, err := parser.LoadCollection(filePath)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if collection.Info.Name != "Test OpenAPI" {
		t.Errorf("Expected name 'Test OpenAPI', got '%s'", collection.Info.Name)
	}

	if collection.Info.Description != "Test Description" {
		t.Errorf("Expected description 'Test Description', got '%s'", collection.Info.Description)
	}

	stored, exists := parser.GetCollection("Test OpenAPI")
	if !exists {
		t.Error("Expected collection to be stored")
	}
	if stored == nil {
		t.Fatal("Expected stored collection to not be nil")
	}
}

func TestLoadCollection_OpenAPIWithSecurity(t *testing.T) {
	openAPISpec := `{
		"openapi": "3.0.0",
		"info": {
			"title": "Secure API",
			"version": "1.0.0"
		},
		"servers": [
			{"url": "https://secure.api.com"}
		],
		"paths": {
			"/protected": {
				"get": {
					"operationId": "getProtected",
					"security": [{"bearerAuth": []}]
				}
			}
		},
		"components": {
			"securitySchemes": {
				"bearerAuth": {
					"type": "http",
					"scheme": "bearer"
				}
			}
		}
	}`

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "secure_api.json")
	if err := os.WriteFile(filePath, []byte(openAPISpec), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	parser := NewParser()
	collection, err := parser.LoadCollection(filePath)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(collection.Items) == 0 {
		t.Fatal("Expected at least one item")
	}

	var protectedItem *Item
	for i := range collection.Items {
		if collection.Items[i].Name == "getProtected" {
			protectedItem = &collection.Items[i]
			break
		}
	}

	if protectedItem == nil {
		t.Fatal("Expected to find 'getProtected' item")
	}

	if protectedItem.Request == nil {
		t.Fatal("Expected request to be set")
	}

	authHeaderFound := false
	for _, header := range protectedItem.Request.Header {
		if header.Key == "Authorization" && header.Value == "Bearer {{bearerAuth}}" {
			authHeaderFound = true
			break
		}
	}

	if !authHeaderFound {
		t.Error("Expected Authorization header with Bearer token template")
	}
}

func TestLoadCollection_OpenAPIPetStore(t *testing.T) {
	parser := NewParser()
	collection, err := parser.LoadCollection("../../testdata/openapi_petstore.json")

	if err != nil {
		t.Fatalf("Expected no error loading petstore, got %v", err)
	}

	if collection.Info.Name != "Pet Store API" {
		t.Errorf("Expected name 'Pet Store API', got '%s'", collection.Info.Name)
	}

	if len(collection.Items) == 0 {
		t.Fatal("Expected items to be loaded")
	}

	petsFolder := findItemByName(collection.Items, "pets")
	if petsFolder == nil {
		t.Fatal("Expected to find 'pets' folder")
	}

	if len(petsFolder.Items) < 5 {
		t.Errorf("Expected at least 5 pet operations, got %d", len(petsFolder.Items))
	}

	healthCheck := findItemByName(collection.Items, "healthCheck")
	if healthCheck == nil {
		t.Fatal("Expected to find 'healthCheck' item")
	}

	if healthCheck.Request == nil {
		t.Error("Expected healthCheck to be a request")
	}
}

func findItemByName(items []Item, name string) *Item {
	for i := range items {
		if items[i].Name == name {
			return &items[i]
		}
	}
	return nil
}
