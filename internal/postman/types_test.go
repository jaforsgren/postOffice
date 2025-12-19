package postman

import (
	"testing"
)

func TestItem_IsFolder(t *testing.T) {
	tests := []struct {
		name     string
		item     Item
		expected bool
	}{
		{
			name: "Folder with items",
			item: Item{
				Name:    "Test Folder",
				Request: nil,
				Items: []Item{
					{Name: "Child Item"},
				},
			},
			expected: true,
		},
		{
			name: "Empty folder",
			item: Item{
				Name:    "Empty Folder",
				Request: nil,
				Items:   []Item{},
			},
			expected: false,
		},
		{
			name: "Request item",
			item: Item{
				Name: "Test Request",
				Request: &Request{
					Method: "GET",
				},
				Items: []Item{},
			},
			expected: false,
		},
		{
			name: "Folder with empty items slice",
			item: Item{
				Name:    "Folder",
				Request: nil,
				Items:   nil,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.item.IsFolder()
			if result != tt.expected {
				t.Errorf("Expected IsFolder() = %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestItem_IsRequest(t *testing.T) {
	tests := []struct {
		name     string
		item     Item
		expected bool
	}{
		{
			name: "Valid request",
			item: Item{
				Name: "Test Request",
				Request: &Request{
					Method: "GET",
					URL: URL{
						Raw: "https://example.com",
					},
				},
			},
			expected: true,
		},
		{
			name: "Folder item",
			item: Item{
				Name:    "Test Folder",
				Request: nil,
				Items: []Item{
					{Name: "Child"},
				},
			},
			expected: false,
		},
		{
			name: "Empty item",
			item: Item{
				Name:    "Empty",
				Request: nil,
				Items:   nil,
			},
			expected: false,
		},
		{
			name: "Request with items (should still be request)",
			item: Item{
				Name: "Hybrid",
				Request: &Request{
					Method: "POST",
				},
				Items: []Item{{Name: "Child"}},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.item.IsRequest()
			if result != tt.expected {
				t.Errorf("Expected IsRequest() = %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestItem_BothMethods(t *testing.T) {
	folder := Item{
		Name:    "Folder",
		Request: nil,
		Items:   []Item{{Name: "Child"}},
	}

	if !folder.IsFolder() {
		t.Error("Expected item to be a folder")
	}
	if folder.IsRequest() {
		t.Error("Expected item to not be a request")
	}

	request := Item{
		Name: "Request",
		Request: &Request{
			Method: "DELETE",
		},
	}

	if request.IsFolder() {
		t.Error("Expected item to not be a folder")
	}
	if !request.IsRequest() {
		t.Error("Expected item to be a request")
	}
}

func TestCollection_Structure(t *testing.T) {
	collection := Collection{
		Info: Info{
			Name:        "Test Collection",
			Description: "Test Description",
			Schema:      "https://schema.getpostman.com/json/collection/v2.1.0/collection.json",
		},
		Items: []Item{
			{
				Name: "Request 1",
				Request: &Request{
					Method: "GET",
					URL: URL{
						Raw: "https://api.example.com/users",
					},
				},
			},
		},
		Variables: []Variable{
			{
				Key:   "baseUrl",
				Value: "https://example.com",
				Type:  "string",
			},
		},
	}

	if collection.Info.Name != "Test Collection" {
		t.Errorf("Expected name 'Test Collection', got %s", collection.Info.Name)
	}
	if len(collection.Items) != 1 {
		t.Errorf("Expected 1 item, got %d", len(collection.Items))
	}
	if len(collection.Variables) != 1 {
		t.Errorf("Expected 1 variable, got %d", len(collection.Variables))
	}
}

func TestRequest_Structure(t *testing.T) {
	request := Request{
		Method: "POST",
		Header: []Header{
			{Key: "Content-Type", Value: "application/json"},
			{Key: "Authorization", Value: "Bearer token"},
		},
		Body: &Body{
			Mode: "raw",
			Raw:  `{"name":"test"}`,
		},
		URL: URL{
			Raw:  "https://api.example.com/users",
			Host: []string{"api", "example", "com"},
			Path: []string{"users"},
		},
	}

	if request.Method != "POST" {
		t.Errorf("Expected method POST, got %s", request.Method)
	}
	if len(request.Header) != 2 {
		t.Errorf("Expected 2 headers, got %d", len(request.Header))
	}
	if request.Body == nil {
		t.Error("Expected body to be set")
	}
	if request.Body != nil && request.Body.Mode != "raw" {
		t.Errorf("Expected body mode 'raw', got %s", request.Body.Mode)
	}
}

func TestHeader_Structure(t *testing.T) {
	header := Header{
		Key:   "X-Custom-Header",
		Value: "custom-value",
		Type:  "text",
	}

	if header.Key != "X-Custom-Header" {
		t.Errorf("Expected key 'X-Custom-Header', got %s", header.Key)
	}
	if header.Value != "custom-value" {
		t.Errorf("Expected value 'custom-value', got %s", header.Value)
	}
}

func TestBody_Structure(t *testing.T) {
	body := Body{
		Mode: "raw",
		Raw:  `{"key":"value"}`,
	}

	if body.Mode != "raw" {
		t.Errorf("Expected mode 'raw', got %s", body.Mode)
	}
	if body.Raw != `{"key":"value"}` {
		t.Errorf("Expected raw body, got %s", body.Raw)
	}
}

func TestURL_Structure(t *testing.T) {
	url := URL{
		Raw:  "https://api.example.com/v1/users/123",
		Host: []string{"api", "example", "com"},
		Path: []string{"v1", "users", "123"},
	}

	if url.Raw != "https://api.example.com/v1/users/123" {
		t.Errorf("Expected specific raw URL, got %s", url.Raw)
	}
	if len(url.Host) != 3 {
		t.Errorf("Expected 3 host parts, got %d", len(url.Host))
	}
	if len(url.Path) != 3 {
		t.Errorf("Expected 3 path parts, got %d", len(url.Path))
	}
}

func TestEnvironment_Structure(t *testing.T) {
	env := Environment{
		ID:   "env-123",
		Name: "Test Environment",
		Values: []EnvVariable{
			{
				Key:     "apiKey",
				Value:   "secret",
				Enabled: true,
				Type:    "secret",
			},
			{
				Key:     "baseUrl",
				Value:   "https://api.example.com",
				Enabled: true,
				Type:    "default",
			},
		},
	}

	if env.ID != "env-123" {
		t.Errorf("Expected ID 'env-123', got %s", env.ID)
	}
	if env.Name != "Test Environment" {
		t.Errorf("Expected name 'Test Environment', got %s", env.Name)
	}
	if len(env.Values) != 2 {
		t.Errorf("Expected 2 values, got %d", len(env.Values))
	}
}

func TestEnvVariable_Structure(t *testing.T) {
	envVar := EnvVariable{
		Key:     "testKey",
		Value:   "testValue",
		Enabled: true,
		Type:    "default",
	}

	if envVar.Key != "testKey" {
		t.Errorf("Expected key 'testKey', got %s", envVar.Key)
	}
	if !envVar.Enabled {
		t.Error("Expected variable to be enabled")
	}
}

func TestVariable_Structure(t *testing.T) {
	variable := Variable{
		Key:     "var1",
		Value:   "value1",
		Type:    "string",
		Enabled: true,
	}

	if variable.Key != "var1" {
		t.Errorf("Expected key 'var1', got %s", variable.Key)
	}
	if variable.Value != "value1" {
		t.Errorf("Expected value 'value1', got %s", variable.Value)
	}
}

func TestNestedItems(t *testing.T) {
	collection := Collection{
		Info: Info{Name: "Root"},
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
								},
							},
						},
					},
				},
			},
		},
	}

	if len(collection.Items) != 1 {
		t.Fatalf("Expected 1 top-level item, got %d", len(collection.Items))
	}

	folder1 := collection.Items[0]
	if !folder1.IsFolder() {
		t.Error("Expected Folder 1 to be a folder")
	}
	if len(folder1.Items) != 1 {
		t.Fatalf("Expected 1 item in Folder 1, got %d", len(folder1.Items))
	}

	subfolder := folder1.Items[0]
	if !subfolder.IsFolder() {
		t.Error("Expected Subfolder to be a folder")
	}
	if len(subfolder.Items) != 1 {
		t.Fatalf("Expected 1 item in Subfolder, got %d", len(subfolder.Items))
	}

	deepRequest := subfolder.Items[0]
	if !deepRequest.IsRequest() {
		t.Error("Expected Deep Request to be a request")
	}
}

func TestItem_WithVariables(t *testing.T) {
	item := Item{
		Name: "Folder with vars",
		Items: []Item{
			{Name: "Child"},
		},
		Variables: []Variable{
			{Key: "folderVar", Value: "folderValue"},
		},
	}

	if !item.IsFolder() {
		t.Error("Expected item to be a folder")
	}
	if len(item.Variables) != 1 {
		t.Errorf("Expected 1 variable, got %d", len(item.Variables))
	}
	if item.Variables[0].Key != "folderVar" {
		t.Errorf("Expected variable key 'folderVar', got %s", item.Variables[0].Key)
	}
}

func TestItem_WithDescription(t *testing.T) {
	item := Item{
		Name:        "Documented Request",
		Description: "This is a test request for documentation",
		Request: &Request{
			Method: "GET",
		},
	}

	if item.Description != "This is a test request for documentation" {
		t.Errorf("Expected specific description, got %s", item.Description)
	}
	if !item.IsRequest() {
		t.Error("Expected item to be a request")
	}
}

func TestMultipleHeadersInRequest(t *testing.T) {
	request := Request{
		Method: "GET",
		Header: []Header{
			{Key: "Accept", Value: "application/json"},
			{Key: "Accept-Language", Value: "en-US"},
			{Key: "Cache-Control", Value: "no-cache"},
			{Key: "Authorization", Value: "Bearer token"},
		},
	}

	if len(request.Header) != 4 {
		t.Errorf("Expected 4 headers, got %d", len(request.Header))
	}

	authFound := false
	for _, h := range request.Header {
		if h.Key == "Authorization" && h.Value == "Bearer token" {
			authFound = true
			break
		}
	}
	if !authFound {
		t.Error("Expected to find Authorization header")
	}
}

func TestEmptyStructures(t *testing.T) {
	emptyCollection := Collection{}
	if emptyCollection.Info.Name != "" {
		t.Error("Expected empty name for uninitialized collection")
	}
	if len(emptyCollection.Items) != 0 {
		t.Error("Expected no items in empty collection")
	}

	emptyItem := Item{}
	if emptyItem.IsFolder() {
		t.Error("Expected empty item to not be a folder")
	}
	if emptyItem.IsRequest() {
		t.Error("Expected empty item to not be a request")
	}

	emptyEnv := Environment{}
	if len(emptyEnv.Values) != 0 {
		t.Error("Expected no values in empty environment")
	}
}
