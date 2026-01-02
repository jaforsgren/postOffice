package postman

import "testing"

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
		expected CollectionFormat
	}{
		{
			name:     "Postman collection v2.1",
			jsonData: `{"info":{"schema":"https://schema.getpostman.com/json/collection/v2.1.0/collection.json"}}`,
			expected: FormatPostman,
		},
		{
			name:     "Postman collection with item array",
			jsonData: `{"info":{"name":"Test"},"item":[]}`,
			expected: FormatPostman,
		},
		{
			name:     "OpenAPI 3.0",
			jsonData: `{"openapi":"3.0.0","info":{"title":"Test"}}`,
			expected: FormatOpenAPI,
		},
		{
			name:     "OpenAPI 3.1",
			jsonData: `{"openapi":"3.1.0","info":{"title":"Test"}}`,
			expected: FormatOpenAPI,
		},
		{
			name:     "Swagger 2.0",
			jsonData: `{"swagger":"2.0","info":{"title":"Test"}}`,
			expected: FormatOpenAPI,
		},
		{
			name:     "OpenAPI with paths but no openapi field",
			jsonData: `{"info":{"title":"Test"},"paths":{"/users":{}}}`,
			expected: FormatOpenAPI,
		},
		{
			name:     "Invalid JSON",
			jsonData: `{invalid}`,
			expected: FormatUnknown,
		},
		{
			name:     "Empty JSON object",
			jsonData: `{}`,
			expected: FormatUnknown,
		},
		{
			name:     "JSON array instead of object",
			jsonData: `[]`,
			expected: FormatUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectFormat([]byte(tt.jsonData))
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}
