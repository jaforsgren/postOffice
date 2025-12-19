package postman

import (
	"strings"
	"testing"
)

func TestGetAllVariables_EnvironmentOnly(t *testing.T) {
	parser := NewParser()
	env := &Environment{
		Name: "Test Env",
		Values: []EnvVariable{
			{Key: "apiKey", Value: "secret123", Enabled: true},
			{Key: "baseUrl", Value: "https://api.test.com", Enabled: true},
			{Key: "disabled", Value: "value", Enabled: false},
		},
	}

	vars := parser.GetAllVariables(nil, nil, env)

	if len(vars) != 2 {
		t.Errorf("Expected 2 variables (disabled excluded), got %d", len(vars))
	}

	found := false
	for _, v := range vars {
		if v.Key == "apiKey" && v.Value == "secret123" {
			found = true
			if !strings.Contains(v.Source, "Environment") {
				t.Errorf("Expected source to contain 'Environment', got %s", v.Source)
			}
		}
	}
	if !found {
		t.Error("Expected to find apiKey variable")
	}

	for _, v := range vars {
		if v.Key == "disabled" {
			t.Error("Expected disabled variable to be excluded")
		}
	}
}

func TestGetAllVariables_CollectionOnly(t *testing.T) {
	parser := NewParser()
	collection := &Collection{
		Info: Info{Name: "Test Collection"},
		Variables: []Variable{
			{Key: "collectionVar", Value: "collectionValue"},
			{Key: "timeout", Value: "30"},
		},
	}

	vars := parser.GetAllVariables(collection, nil, nil)

	if len(vars) != 2 {
		t.Errorf("Expected 2 variables, got %d", len(vars))
	}

	found := false
	for _, v := range vars {
		if v.Key == "collectionVar" && v.Value == "collectionValue" {
			found = true
			if !strings.Contains(v.Source, "Collection") {
				t.Errorf("Expected source to contain 'Collection', got %s", v.Source)
			}
		}
	}
	if !found {
		t.Error("Expected to find collectionVar variable")
	}
}

func TestGetAllVariables_FolderVariables(t *testing.T) {
	parser := NewParser()
	collection := &Collection{
		Info: Info{Name: "Test Collection"},
		Items: []Item{
			{
				Name: "Folder1",
				Items: []Item{
					{
						Name: "Subfolder",
						Items: []Item{
							{Name: "Request"},
						},
						Variables: []Variable{
							{Key: "subfolderVar", Value: "subValue"},
						},
					},
				},
				Variables: []Variable{
					{Key: "folderVar", Value: "folderValue"},
				},
			},
		},
	}

	breadcrumb := []string{"Folder1", "Subfolder"}
	vars := parser.GetAllVariables(collection, breadcrumb, nil)

	if len(vars) != 2 {
		t.Errorf("Expected 2 folder variables, got %d", len(vars))
	}

	folderVarFound := false
	subfolderVarFound := false

	for _, v := range vars {
		if v.Key == "folderVar" && v.Value == "folderValue" {
			folderVarFound = true
			if !strings.Contains(v.Source, "Folder: Folder1") {
				t.Errorf("Expected source to contain folder path, got %s", v.Source)
			}
		}
		if v.Key == "subfolderVar" && v.Value == "subValue" {
			subfolderVarFound = true
			if !strings.Contains(v.Source, "Folder: Folder1 / Subfolder") {
				t.Errorf("Expected source to contain full folder path, got %s", v.Source)
			}
		}
	}

	if !folderVarFound {
		t.Error("Expected to find folderVar")
	}
	if !subfolderVarFound {
		t.Error("Expected to find subfolderVar")
	}
}

func TestGetAllVariables_Precedence(t *testing.T) {
	parser := NewParser()

	env := &Environment{
		Name: "Test Env",
		Values: []EnvVariable{
			{Key: "override", Value: "envValue", Enabled: true},
			{Key: "envOnly", Value: "envOnlyValue", Enabled: true},
		},
	}

	collection := &Collection{
		Info: Info{Name: "Test Collection"},
		Variables: []Variable{
			{Key: "override", Value: "collectionValue"},
			{Key: "collectionOnly", Value: "collectionOnlyValue"},
		},
		Items: []Item{
			{
				Name: "Folder",
				Items: []Item{
					{Name: "Request"},
				},
				Variables: []Variable{
					{Key: "override", Value: "folderValue"},
					{Key: "folderOnly", Value: "folderOnlyValue"},
				},
			},
		},
	}

	breadcrumb := []string{"Folder"}
	vars := parser.GetAllVariables(collection, breadcrumb, env)

	varMap := make(map[string]string)
	for _, v := range vars {
		varMap[v.Key] = v.Value
	}

	if varMap["override"] != "envValue" {
		t.Errorf("Expected environment to override, got %s", varMap["override"])
	}
	if varMap["envOnly"] != "envOnlyValue" {
		t.Error("Expected envOnly variable")
	}
	if varMap["collectionOnly"] != "collectionOnlyValue" {
		t.Error("Expected collectionOnly variable")
	}
	if varMap["folderOnly"] != "folderOnlyValue" {
		t.Error("Expected folderOnly variable")
	}
}

func TestGetAllVariables_NoBreadcrumb(t *testing.T) {
	parser := NewParser()
	collection := &Collection{
		Info: Info{Name: "Test Collection"},
		Variables: []Variable{
			{Key: "var1", Value: "value1"},
		},
		Items: []Item{
			{
				Name: "Folder",
				Items: []Item{
					{Name: "Request"},
				},
				Variables: []Variable{
					{Key: "folderVar", Value: "shouldNotAppear"},
				},
			},
		},
	}

	vars := parser.GetAllVariables(collection, nil, nil)

	if len(vars) != 1 {
		t.Errorf("Expected 1 variable (no folder vars without breadcrumb), got %d", len(vars))
	}

	for _, v := range vars {
		if v.Key == "folderVar" {
			t.Error("Expected folder variable to not appear without breadcrumb")
		}
	}
}

func TestGetAllVariables_EmptyBreadcrumb(t *testing.T) {
	parser := NewParser()
	collection := &Collection{
		Info: Info{Name: "Test Collection"},
		Variables: []Variable{
			{Key: "var1", Value: "value1"},
		},
	}

	vars := parser.GetAllVariables(collection, []string{}, nil)

	if len(vars) != 1 {
		t.Errorf("Expected 1 variable, got %d", len(vars))
	}
}

func TestGetAllVariables_InvalidBreadcrumb(t *testing.T) {
	parser := NewParser()
	collection := &Collection{
		Info: Info{Name: "Test Collection"},
		Variables: []Variable{
			{Key: "var1", Value: "value1"},
		},
		Items: []Item{
			{
				Name: "RealFolder",
				Items: []Item{
					{Name: "Request"},
				},
				Variables: []Variable{
					{Key: "folderVar", Value: "value"},
				},
			},
		},
	}

	breadcrumb := []string{"NonExistentFolder"}
	vars := parser.GetAllVariables(collection, breadcrumb, nil)

	if len(vars) != 1 {
		t.Errorf("Expected only collection variable, got %d", len(vars))
	}
}

func TestGetAllVariables_NilInputs(t *testing.T) {
	parser := NewParser()

	vars := parser.GetAllVariables(nil, nil, nil)

	if len(vars) != 0 {
		t.Errorf("Expected 0 variables for nil inputs, got %d", len(vars))
	}
}

func TestResolveVariables_Simple(t *testing.T) {
	variables := []VariableSource{
		{Key: "name", Value: "John", Source: "test"},
		{Key: "age", Value: "30", Source: "test"},
	}

	text := "Hello {{name}}, you are {{age}} years old"
	result := ResolveVariables(text, variables)

	expected := "Hello John, you are 30 years old"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestResolveVariables_NoVariables(t *testing.T) {
	variables := []VariableSource{}

	text := "Hello {{name}}"
	result := ResolveVariables(text, variables)

	if result != text {
		t.Errorf("Expected unchanged text '%s', got '%s'", text, result)
	}
}

func TestResolveVariables_UnknownVariable(t *testing.T) {
	variables := []VariableSource{
		{Key: "known", Value: "value", Source: "test"},
	}

	text := "{{known}} and {{unknown}}"
	result := ResolveVariables(text, variables)

	expected := "value and {{unknown}}"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestResolveVariables_MultipleOccurrences(t *testing.T) {
	variables := []VariableSource{
		{Key: "var", Value: "replacement", Source: "test"},
	}

	text := "{{var}} and {{var}} again {{var}}"
	result := ResolveVariables(text, variables)

	expected := "replacement and replacement again replacement"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestResolveVariables_WithWhitespace(t *testing.T) {
	variables := []VariableSource{
		{Key: "var", Value: "value", Source: "test"},
	}

	text := "{{ var }} and {{  var  }}"
	result := ResolveVariables(text, variables)

	expected := "value and value"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestResolveVariables_InURL(t *testing.T) {
	variables := []VariableSource{
		{Key: "baseUrl", Value: "https://api.example.com", Source: "test"},
		{Key: "version", Value: "v1", Source: "test"},
		{Key: "userId", Value: "123", Source: "test"},
	}

	url := "{{baseUrl}}/{{version}}/users/{{userId}}"
	result := ResolveVariables(url, variables)

	expected := "https://api.example.com/v1/users/123"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestResolveVariables_InJSON(t *testing.T) {
	variables := []VariableSource{
		{Key: "name", Value: "Alice", Source: "test"},
		{Key: "email", Value: "alice@example.com", Source: "test"},
	}

	json := `{"name":"{{name}}","email":"{{email}}"}`
	result := ResolveVariables(json, variables)

	expected := `{"name":"Alice","email":"alice@example.com"}`
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestResolveVariables_EmptyText(t *testing.T) {
	variables := []VariableSource{
		{Key: "var", Value: "value", Source: "test"},
	}

	result := ResolveVariables("", variables)

	if result != "" {
		t.Errorf("Expected empty string, got '%s'", result)
	}
}

func TestResolveVariables_NoPlaceholders(t *testing.T) {
	variables := []VariableSource{
		{Key: "var", Value: "value", Source: "test"},
	}

	text := "This is plain text without variables"
	result := ResolveVariables(text, variables)

	if result != text {
		t.Errorf("Expected unchanged text '%s', got '%s'", text, result)
	}
}

func TestResolveVariables_NestedBraces(t *testing.T) {
	variables := []VariableSource{
		{Key: "outer", Value: "replaced", Source: "test"},
	}

	text := "{{outer}}"
	result := ResolveVariables(text, variables)

	expected := "replaced"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestResolveVariables_SpecialCharacters(t *testing.T) {
	variables := []VariableSource{
		{Key: "token", Value: "abc.def.ghi", Source: "test"},
		{Key: "path", Value: "/api/v1/users", Source: "test"},
	}

	text := "Bearer {{token}} accessing {{path}}"
	result := ResolveVariables(text, variables)

	expected := "Bearer abc.def.ghi accessing /api/v1/users"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestResolveVariables_ConsecutiveVariables(t *testing.T) {
	variables := []VariableSource{
		{Key: "protocol", Value: "https", Source: "test"},
		{Key: "host", Value: "example.com", Source: "test"},
	}

	text := "{{protocol}}://{{host}}"
	result := ResolveVariables(text, variables)

	expected := "https://example.com"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestVariableSource_Structure(t *testing.T) {
	varSource := VariableSource{
		Key:    "testKey",
		Value:  "testValue",
		Source: "Test Source",
	}

	if varSource.Key != "testKey" {
		t.Errorf("Expected key 'testKey', got %s", varSource.Key)
	}
	if varSource.Value != "testValue" {
		t.Errorf("Expected value 'testValue', got %s", varSource.Value)
	}
	if varSource.Source != "Test Source" {
		t.Errorf("Expected source 'Test Source', got %s", varSource.Source)
	}
}

func TestGetAllVariables_ComplexHierarchy(t *testing.T) {
	parser := NewParser()

	env := &Environment{
		Name: "Production",
		Values: []EnvVariable{
			{Key: "apiKey", Value: "prod-key", Enabled: true},
		},
	}

	collection := &Collection{
		Info: Info{Name: "API Collection"},
		Variables: []Variable{
			{Key: "timeout", Value: "30"},
		},
		Items: []Item{
			{
				Name: "Auth",
				Variables: []Variable{
					{Key: "authPath", Value: "/auth"},
				},
				Items: []Item{
					{
						Name: "OAuth",
						Variables: []Variable{
							{Key: "grantType", Value: "client_credentials"},
						},
						Items: []Item{
							{Name: "Token Request"},
						},
					},
				},
			},
		},
	}

	breadcrumb := []string{"Auth", "OAuth"}
	vars := parser.GetAllVariables(collection, breadcrumb, env)

	varMap := make(map[string]string)
	for _, v := range vars {
		varMap[v.Key] = v.Value
	}

	if varMap["apiKey"] != "prod-key" {
		t.Error("Expected environment variable")
	}
	if varMap["timeout"] != "30" {
		t.Error("Expected collection variable")
	}
	if varMap["authPath"] != "/auth" {
		t.Error("Expected parent folder variable")
	}
	if varMap["grantType"] != "client_credentials" {
		t.Error("Expected current folder variable")
	}

	if len(vars) != 4 {
		t.Errorf("Expected 4 variables from all levels, got %d", len(vars))
	}
}

func TestResolveVariables_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		variables []VariableSource
		expected  string
	}{
		{
			name:      "Single brace",
			text:      "{not a variable}",
			variables: []VariableSource{},
			expected:  "{not a variable}",
		},
		{
			name:      "Triple braces (not matched)",
			text:      "{{{var}}}",
			variables: []VariableSource{{Key: "var", Value: "val"}},
			expected:  "{{{var}}}",
		},
		{
			name:      "Empty variable name (not matched)",
			text:      "{{}}",
			variables: []VariableSource{{Key: "", Value: "val"}},
			expected:  "{{}}",
		},
		{
			name:      "Variable with underscore",
			text:      "{{api_key}}",
			variables: []VariableSource{{Key: "api_key", Value: "secret"}},
			expected:  "secret",
		},
		{
			name:      "Variable with numbers",
			text:      "{{var123}}",
			variables: []VariableSource{{Key: "var123", Value: "value"}},
			expected:  "value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveVariables(tt.text, tt.variables)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}
