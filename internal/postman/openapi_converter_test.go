package postman

import (
	"strings"
	"testing"
)

func TestConvertInfo(t *testing.T) {
	oaInfo := OpenAPIInfo{
		Title:       "Pet Store API",
		Description: "A sample pet store",
		Version:     "1.0.0",
	}

	info := convertInfo(oaInfo)

	if info.Name != "Pet Store API" {
		t.Errorf("Expected name 'Pet Store API', got '%s'", info.Name)
	}
	if info.Description != "A sample pet store" {
		t.Errorf("Expected description 'A sample pet store', got '%s'", info.Description)
	}
	if info.Schema != postmanSchemaURL {
		t.Errorf("Expected schema '%s', got '%s'", postmanSchemaURL, info.Schema)
	}
}

func TestGetBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		servers  []OpenAPIServer
		expected string
	}{
		{
			name:     "Single server",
			servers:  []OpenAPIServer{{URL: "https://api.example.com"}},
			expected: "https://api.example.com",
		},
		{
			name:     "Multiple servers uses first",
			servers:  []OpenAPIServer{{URL: "https://api.example.com"}, {URL: "https://api2.example.com"}},
			expected: "https://api.example.com",
		},
		{
			name:     "Empty servers",
			servers:  []OpenAPIServer{},
			expected: defaultBaseURL,
		},
		{
			name:     "Server with variables",
			servers:  []OpenAPIServer{{URL: "https://{environment}.example.com"}},
			expected: "https://{{environment}}.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getBaseURL(tt.servers)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestBuildURL_PathParameters(t *testing.T) {
	tests := []struct {
		name       string
		baseURL    string
		path       string
		pathParams []OpenAPIParameter
		expected   string
	}{
		{
			name:    "Single path param",
			baseURL: "https://api.example.com",
			path:    "/users/{id}",
			pathParams: []OpenAPIParameter{
				{Name: "id", In: "path"},
			},
			expected: "https://api.example.com/users/{{id}}",
		},
		{
			name:    "Multiple path params",
			baseURL: "https://api.example.com",
			path:    "/users/{userId}/posts/{postId}",
			pathParams: []OpenAPIParameter{
				{Name: "userId", In: "path"},
				{Name: "postId", In: "path"},
			},
			expected: "https://api.example.com/users/{{userId}}/posts/{{postId}}",
		},
		{
			name:       "No path params",
			baseURL:    "https://api.example.com",
			path:       "/users",
			pathParams: nil,
			expected:   "https://api.example.com/users",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := buildURL(tt.baseURL, tt.path, tt.pathParams, nil)
			if url.Raw != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, url.Raw)
			}
		})
	}
}

func TestBuildURL_QueryParameters(t *testing.T) {
	baseURL := "https://api.example.com"
	path := "/search"
	queryParams := []OpenAPIParameter{
		{Name: "q", In: "query"},
		{Name: "limit", In: "query"},
	}

	url := buildURL(baseURL, path, nil, queryParams)

	if !strings.Contains(url.Raw, "q={{q}}") {
		t.Error("Expected query parameter template for 'q'")
	}
	if !strings.Contains(url.Raw, "limit={{limit}}") {
		t.Error("Expected query parameter template for 'limit'")
	}
	if !strings.HasPrefix(url.Raw, "https://api.example.com/search?") {
		t.Errorf("Expected URL to start with base + path + ?, got '%s'", url.Raw)
	}
}

func TestCategorizeParameters(t *testing.T) {
	params := []OpenAPIParameter{
		{Name: "id", In: "path"},
		{Name: "q", In: "query"},
		{Name: "X-API-Key", In: "header"},
		{Name: "userId", In: "path"},
		{Name: "limit", In: "query"},
	}

	pathParams, queryParams, headerParams := categorizeParameters(params)

	if len(pathParams) != 2 {
		t.Errorf("Expected 2 path params, got %d", len(pathParams))
	}
	if len(queryParams) != 2 {
		t.Errorf("Expected 2 query params, got %d", len(queryParams))
	}
	if len(headerParams) != 1 {
		t.Errorf("Expected 1 header param, got %d", len(headerParams))
	}
}

func TestConvertParametersToHeaders(t *testing.T) {
	headerParams := []OpenAPIParameter{
		{Name: "X-API-Key", In: "header"},
		{Name: "X-Request-ID", In: "header"},
	}

	headers := convertParametersToHeaders(headerParams)

	if len(headers) != 2 {
		t.Fatalf("Expected 2 headers, got %d", len(headers))
	}

	if headers[0].Key != "X-API-Key" || headers[0].Value != "{{X-API-Key}}" {
		t.Errorf("Expected X-API-Key: {{X-API-Key}}, got %s: %s", headers[0].Key, headers[0].Value)
	}

	if headers[1].Key != "X-Request-ID" || headers[1].Value != "{{X-Request-ID}}" {
		t.Errorf("Expected X-Request-ID: {{X-Request-ID}}, got %s: %s", headers[1].Key, headers[1].Value)
	}
}

func TestConvertSecurity_ApiKey(t *testing.T) {
	securitySchemes := map[string]OpenAPISecurityScheme{
		"apiKey": {
			Type: "apiKey",
			Name: "X-API-Key",
			In:   "header",
		},
	}

	opSecurity := []map[string][]string{
		{"apiKey": {}},
	}

	headers := convertSecurity(opSecurity, securitySchemes)

	if len(headers) != 1 {
		t.Fatalf("Expected 1 header, got %d", len(headers))
	}

	if headers[0].Key != "X-API-Key" {
		t.Errorf("Expected key 'X-API-Key', got '%s'", headers[0].Key)
	}

	if headers[0].Value != "{{apiKey}}" {
		t.Errorf("Expected value '{{apiKey}}', got '%s'", headers[0].Value)
	}
}

func TestConvertSecurity_BearerToken(t *testing.T) {
	securitySchemes := map[string]OpenAPISecurityScheme{
		"bearerAuth": {
			Type:   "http",
			Scheme: "bearer",
		},
	}

	opSecurity := []map[string][]string{
		{"bearerAuth": {}},
	}

	headers := convertSecurity(opSecurity, securitySchemes)

	if len(headers) != 1 {
		t.Fatalf("Expected 1 header, got %d", len(headers))
	}

	if headers[0].Key != "Authorization" {
		t.Errorf("Expected key 'Authorization', got '%s'", headers[0].Key)
	}

	if headers[0].Value != "Bearer {{bearerAuth}}" {
		t.Errorf("Expected value 'Bearer {{bearerAuth}}', got '%s'", headers[0].Value)
	}
}

func TestConvertSecurity_BasicAuth(t *testing.T) {
	securitySchemes := map[string]OpenAPISecurityScheme{
		"basicAuth": {
			Type:   "http",
			Scheme: "basic",
		},
	}

	opSecurity := []map[string][]string{
		{"basicAuth": {}},
	}

	headers := convertSecurity(opSecurity, securitySchemes)

	if len(headers) != 1 {
		t.Fatalf("Expected 1 header, got %d", len(headers))
	}

	if headers[0].Key != "Authorization" {
		t.Errorf("Expected key 'Authorization', got '%s'", headers[0].Key)
	}

	if headers[0].Value != "Basic {{basicAuth}}" {
		t.Errorf("Expected value 'Basic {{basicAuth}}', got '%s'", headers[0].Value)
	}
}

func TestConvertRequestBody(t *testing.T) {
	tests := []struct {
		name     string
		reqBody  *OpenAPIRequestBody
		expected string
	}{
		{
			name: "JSON with example",
			reqBody: &OpenAPIRequestBody{
				Required: true,
				Content: map[string]OpenAPIMediaType{
					"application/json": {
						Example: map[string]interface{}{
							"name": "John",
							"age":  30,
						},
					},
				},
			},
			expected: "John",
		},
		{
			name: "JSON with schema example",
			reqBody: &OpenAPIRequestBody{
				Content: map[string]OpenAPIMediaType{
					"application/json": {
						Schema: &OpenAPISchema{
							Type: "object",
							Example: map[string]interface{}{
								"email": "test@example.com",
							},
						},
					},
				},
			},
			expected: "test@example.com",
		},
		{
			name: "No example",
			reqBody: &OpenAPIRequestBody{
				Content: map[string]OpenAPIMediaType{
					"application/json": {
						Schema: &OpenAPISchema{
							Type: "object",
						},
					},
				},
			},
			expected: "{}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := convertRequestBody(tt.reqBody)

			if body == nil {
				t.Fatal("Expected body to be converted")
			}

			if body.Mode != "raw" {
				t.Errorf("Expected mode 'raw', got '%s'", body.Mode)
			}

			if !strings.Contains(body.Raw, tt.expected) {
				t.Errorf("Expected body to contain '%s', got '%s'", tt.expected, body.Raw)
			}
		})
	}
}

func TestConvertOpenAPIToCollection_Complete(t *testing.T) {
	spec := &OpenAPISpec{
		OpenAPI: "3.0.0",
		Info: OpenAPIInfo{
			Title:       "Test API",
			Description: "Test Description",
			Version:     "1.0.0",
		},
		Servers: []OpenAPIServer{
			{URL: "https://api.test.com"},
		},
		Paths: map[string]OpenAPIPathItem{
			"/users/{id}": {
				Get: &OpenAPIOperation{
					OperationID: "getUser",
					Summary:     "Get user by ID",
					Parameters: []OpenAPIParameter{
						{Name: "id", In: "path", Required: true},
					},
				},
			},
		},
	}

	collection, err := ConvertOpenAPIToCollection(spec)

	if err != nil {
		t.Fatalf("Conversion failed: %v", err)
	}

	if collection.Info.Name != "Test API" {
		t.Errorf("Expected name 'Test API', got '%s'", collection.Info.Name)
	}

	if len(collection.Items) == 0 {
		t.Fatal("Expected at least one item")
	}

	var getUserItem *Item
	for i := range collection.Items {
		if collection.Items[i].Name == "getUser" {
			getUserItem = &collection.Items[i]
			break
		}
	}

	if getUserItem == nil {
		t.Fatal("Expected to find 'getUser' item")
	}

	if getUserItem.Request == nil {
		t.Fatal("Expected request to be set")
	}

	if getUserItem.Request.Method != "GET" {
		t.Errorf("Expected method GET, got %s", getUserItem.Request.Method)
	}

	if !strings.Contains(getUserItem.Request.URL.Raw, "{{id}}") {
		t.Errorf("Expected URL to contain path parameter template {{id}}, got '%s'", getUserItem.Request.URL.Raw)
	}

	if !strings.Contains(getUserItem.Request.URL.Raw, "https://api.test.com") {
		t.Errorf("Expected URL to contain base URL, got '%s'", getUserItem.Request.URL.Raw)
	}
}

func TestOrganizeByTags(t *testing.T) {
	items := []Item{
		{
			Name:        "listPets",
			Description: "[tag:pets] List all pets",
			Request:     &Request{Method: "GET"},
		},
		{
			Name:        "createPet",
			Description: "[tag:pets] Create a pet",
			Request:     &Request{Method: "POST"},
		},
		{
			Name:        "listUsers",
			Description: "[tag:users] List all users",
			Request:     &Request{Method: "GET"},
		},
		{
			Name:        "healthCheck",
			Description: "Check API health",
			Request:     &Request{Method: "GET"},
		},
	}

	organized := organizeByTags(items)

	if len(organized) != 3 {
		t.Errorf("Expected 3 top-level items (2 folders + 1 untagged), got %d", len(organized))
	}

	var petsFolder *Item
	var usersFolder *Item
	var untaggedItem *Item

	for i := range organized {
		if organized[i].Name == "pets" {
			petsFolder = &organized[i]
		} else if organized[i].Name == "users" {
			usersFolder = &organized[i]
		} else if organized[i].Name == "healthCheck" {
			untaggedItem = &organized[i]
		}
	}

	if petsFolder == nil {
		t.Fatal("Expected to find 'pets' folder")
	}
	if len(petsFolder.Items) != 2 {
		t.Errorf("Expected 2 items in pets folder, got %d", len(petsFolder.Items))
	}

	if usersFolder == nil {
		t.Fatal("Expected to find 'users' folder")
	}
	if len(usersFolder.Items) != 1 {
		t.Errorf("Expected 1 item in users folder, got %d", len(usersFolder.Items))
	}

	if untaggedItem == nil {
		t.Fatal("Expected to find untagged item 'healthCheck'")
	}
	if untaggedItem.Request == nil {
		t.Error("Expected untagged item to be a request, not a folder")
	}
}

func TestConvertOperation_GeneratesNameFromMethod(t *testing.T) {
	op := &OpenAPIOperation{
		Summary: "",
	}

	item := convertOperation("POST", "/users", op, nil, "https://api.test.com", nil, nil)

	if item.Name != "POST /users" {
		t.Errorf("Expected name 'POST /users', got '%s'", item.Name)
	}
}

func TestConvertOperation_UsesSummaryWhenNoOperationID(t *testing.T) {
	op := &OpenAPIOperation{
		Summary: "Create a new user",
	}

	item := convertOperation("POST", "/users", op, nil, "https://api.test.com", nil, nil)

	if item.Name != "Create a new user" {
		t.Errorf("Expected name 'Create a new user', got '%s'", item.Name)
	}
}

func TestTemplateServerVariables(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "https://{environment}.example.com",
			expected: "https://{{environment}}.example.com",
		},
		{
			input:    "https://api.example.com",
			expected: "https://api.example.com",
		},
		{
			input:    "https://{env}.{region}.example.com",
			expected: "https://{{env}}.{{region}}.example.com",
		},
	}

	for _, tt := range tests {
		result := templateServerVariables(tt.input)
		if result != tt.expected {
			t.Errorf("Expected '%s', got '%s'", tt.expected, result)
		}
	}
}
