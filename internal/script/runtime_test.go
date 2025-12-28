package script

import (
	"postOffice/internal/postman"
	"testing"
)

func TestNewRuntime(t *testing.T) {
	runtime := NewRuntime()
	if runtime == nil {
		t.Fatal("Expected runtime to be created")
	}
	if runtime.vm == nil {
		t.Error("Expected VM to be initialized")
	}
}

func TestExecuteTestScript_SimplePass(t *testing.T) {
	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"pm.test('simple test', () => {",
			"    pm.response.to.have.status(200);",
			"});",
		},
	}

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 200,
			Body:       `{"status":"ok"}`,
		},
	}

	result := runtime.ExecuteTestScript(script, ctx)

	if result == nil {
		t.Fatal("Expected result")
	}
	if len(result.Tests) != 1 {
		t.Fatalf("Expected 1 test, got %d", len(result.Tests))
	}
	if !result.Tests[0].Passed {
		t.Error("Expected test to pass")
	}
	if result.Tests[0].Name != "simple test" {
		t.Errorf("Expected test name 'simple test', got '%s'", result.Tests[0].Name)
	}
}

func TestExecuteTestScript_SimpleFail(t *testing.T) {
	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"pm.test('failing test', () => {",
			"    pm.response.to.have.status(200);",
			"});",
		},
	}

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 404,
			Body:       `{"error":"not found"}`,
		},
	}

	result := runtime.ExecuteTestScript(script, ctx)

	if result == nil {
		t.Fatal("Expected result")
	}
	if len(result.Tests) != 1 {
		t.Fatalf("Expected 1 test, got %d", len(result.Tests))
	}
	if result.Tests[0].Passed {
		t.Error("Expected test to fail")
	}
	if result.Tests[0].Error == "" {
		t.Error("Expected error message")
	}
}

func TestExecuteTestScript_MultipleTests(t *testing.T) {
	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"pm.test('first test', () => {",
			"    pm.response.to.have.status(200);",
			"});",
			"pm.test('second test', () => {",
			"    pm.response.to.have.status(200);",
			"});",
		},
	}

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 200,
			Body:       "test",
		},
	}

	result := runtime.ExecuteTestScript(script, ctx)

	if len(result.Tests) != 2 {
		t.Fatalf("Expected 2 tests, got %d", len(result.Tests))
	}
	for i, test := range result.Tests {
		if !test.Passed {
			t.Errorf("Test %d failed: %s", i, test.Error)
		}
	}
}

func TestExecuteTestScript_CollectionVariableSet(t *testing.T) {
	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"pm.collectionVariables.set('testKey', 'testValue');",
		},
	}

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 200,
			Body:       "",
		},
		CollectionVars: []postman.Variable{},
	}

	result := runtime.ExecuteTestScript(script, ctx)

	if len(result.Errors) > 0 {
		t.Errorf("Expected no errors, got: %v", result.Errors)
	}
	if len(ctx.CollectionVars) != 1 {
		t.Fatalf("Expected 1 collection variable, got %d", len(ctx.CollectionVars))
	}
	if ctx.CollectionVars[0].Key != "testKey" {
		t.Errorf("Expected key 'testKey', got '%s'", ctx.CollectionVars[0].Key)
	}
	if ctx.CollectionVars[0].Value != "testValue" {
		t.Errorf("Expected value 'testValue', got '%s'", ctx.CollectionVars[0].Value)
	}
}

func TestExecuteTestScript_EnvironmentVariableSet(t *testing.T) {
	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"pm.environmentVariables.set('envKey', 'envValue');",
		},
	}

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 200,
			Body:       "",
		},
		EnvironmentVars: []postman.EnvVariable{},
	}

	result := runtime.ExecuteTestScript(script, ctx)

	if len(result.Errors) > 0 {
		t.Errorf("Expected no errors, got: %v", result.Errors)
	}
	if len(ctx.EnvironmentVars) != 1 {
		t.Fatalf("Expected 1 environment variable, got %d", len(ctx.EnvironmentVars))
	}
	if ctx.EnvironmentVars[0].Key != "envKey" {
		t.Errorf("Expected key 'envKey', got '%s'", ctx.EnvironmentVars[0].Key)
	}
	if ctx.EnvironmentVars[0].Value != "envValue" {
		t.Errorf("Expected value 'envValue', got '%s'", ctx.EnvironmentVars[0].Value)
	}
}

func TestExecuteTestScript_ResponseBodyAccess(t *testing.T) {
	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"pm.test('response body check', () => {",
			"    if (responseBody !== 'test body') {",
			"        throw new Error('Body mismatch');",
			"    }",
			"});",
		},
	}

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 200,
			Body:       "test body",
		},
	}

	result := runtime.ExecuteTestScript(script, ctx)

	if len(result.Tests) != 1 {
		t.Fatalf("Expected 1 test, got %d", len(result.Tests))
	}
	if !result.Tests[0].Passed {
		t.Errorf("Expected test to pass, error: %s", result.Tests[0].Error)
	}
}

func TestExecuteTestScript_EmptyScript(t *testing.T) {
	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{},
	}

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 200,
			Body:       "",
		},
	}

	result := runtime.ExecuteTestScript(script, ctx)

	if len(result.Tests) != 0 {
		t.Errorf("Expected 0 tests, got %d", len(result.Tests))
	}
	if len(result.Errors) != 0 {
		t.Errorf("Expected 0 errors, got %d", len(result.Errors))
	}
}

func TestExecuteTestScripts_MultipleEvents(t *testing.T) {
	events := []postman.Event{
		{
			Listen: "test",
			Script: postman.Script{
				Type: "text/javascript",
				Exec: []string{
					"pm.test('test 1', () => {",
					"    pm.response.to.have.status(200);",
					"});",
				},
			},
		},
		{
			Listen: "test",
			Script: postman.Script{
				Type: "text/javascript",
				Exec: []string{
					"pm.test('test 2', () => {",
					"    pm.response.to.have.status(200);",
					"});",
				},
			},
		},
	}

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 200,
			Body:       "",
		},
	}

	result := ExecuteTestScripts(events, ctx)

	if len(result.Tests) != 2 {
		t.Fatalf("Expected 2 tests, got %d", len(result.Tests))
	}
}

func TestExecuteTestScripts_IgnoreNonTestEvents(t *testing.T) {
	events := []postman.Event{
		{
			Listen: "prerequest",
			Script: postman.Script{
				Type: "text/javascript",
				Exec: []string{
					"console.log('pre-request');",
				},
			},
		},
		{
			Listen: "test",
			Script: postman.Script{
				Type: "text/javascript",
				Exec: []string{
					"pm.test('only test', () => {",
					"    pm.response.to.have.status(200);",
					"});",
				},
			},
		},
	}

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 200,
			Body:       "",
		},
	}

	result := ExecuteTestScripts(events, ctx)

	if len(result.Tests) != 1 {
		t.Fatalf("Expected 1 test (pre-request should be ignored), got %d", len(result.Tests))
	}
}

func TestExecutePreRequestScripts_Simple(t *testing.T) {
	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"pm.collectionVariables.set('preReqVar', 'setValue');",
		},
	}

	ctx := &ExecutionContext{
		CollectionVars: []postman.Variable{},
	}

	result := runtime.ExecutePreRequestScript(script, ctx)

	if len(result.Errors) > 0 {
		t.Errorf("Expected no errors, got: %v", result.Errors)
	}
	if len(ctx.CollectionVars) != 1 {
		t.Fatalf("Expected 1 collection variable, got %d", len(ctx.CollectionVars))
	}
	if ctx.CollectionVars[0].Key != "preReqVar" {
		t.Errorf("Expected key 'preReqVar', got '%s'", ctx.CollectionVars[0].Key)
	}
	if ctx.CollectionVars[0].Value != "setValue" {
		t.Errorf("Expected value 'setValue', got '%s'", ctx.CollectionVars[0].Value)
	}
}

func TestExecutePreRequestScripts_Multiple(t *testing.T) {
	events := []postman.Event{
		{
			Listen: "prerequest",
			Script: postman.Script{
				Type: "text/javascript",
				Exec: []string{
					"pm.environmentVariables.set('token', 'abc123');",
				},
			},
		},
		{
			Listen: "prerequest",
			Script: postman.Script{
				Type: "text/javascript",
				Exec: []string{
					"pm.collectionVariables.set('requestId', '456');",
				},
			},
		},
		{
			Listen: "test",
			Script: postman.Script{
				Type: "text/javascript",
				Exec: []string{
					"pm.test('should be ignored', () => {});",
				},
			},
		},
	}

	ctx := &ExecutionContext{
		CollectionVars:  []postman.Variable{},
		EnvironmentVars: []postman.EnvVariable{},
	}

	errors := ExecutePreRequestScripts(events, ctx)

	if len(errors) > 0 {
		t.Errorf("Expected no errors, got: %v", errors)
	}
	if len(ctx.EnvironmentVars) != 1 {
		t.Fatalf("Expected 1 environment variable, got %d", len(ctx.EnvironmentVars))
	}
	if len(ctx.CollectionVars) != 1 {
		t.Fatalf("Expected 1 collection variable, got %d", len(ctx.CollectionVars))
	}
}

func TestExecutePreRequestScripts_Empty(t *testing.T) {
	events := []postman.Event{}
	ctx := &ExecutionContext{}

	errors := ExecutePreRequestScripts(events, ctx)

	if len(errors) != 0 {
		t.Errorf("Expected no errors, got: %v", errors)
	}
}

func TestExecuteTestScript_ResponseJSON(t *testing.T) {
	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"const data = pm.response.json();",
			"pm.test('has id property', () => {",
			"    if (!data.id) {",
			"        throw new Error('Missing id property');",
			"    }",
			"});",
			"pm.test('id is 123', () => {",
			"    if (data.id !== 123) {",
			"        throw new Error('Expected id 123, got ' + data.id);",
			"    }",
			"});",
			"pm.collectionVariables.set('userId', data.id.toString());",
		},
	}

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 200,
			Body:       `{"id": 123, "name": "test user", "active": true}`,
		},
		CollectionVars: []postman.Variable{},
	}

	result := runtime.ExecuteTestScript(script, ctx)

	if len(result.Errors) > 0 {
		t.Errorf("Expected no errors, got: %v", result.Errors)
	}
	if len(result.Tests) != 2 {
		t.Fatalf("Expected 2 tests, got %d", len(result.Tests))
	}
	if !result.Tests[0].Passed {
		t.Errorf("Expected first test to pass, error: %s", result.Tests[0].Error)
	}
	if !result.Tests[1].Passed {
		t.Errorf("Expected second test to pass, error: %s", result.Tests[1].Error)
	}
	if len(ctx.CollectionVars) != 1 {
		t.Fatalf("Expected 1 collection variable, got %d", len(ctx.CollectionVars))
	}
	if ctx.CollectionVars[0].Key != "userId" {
		t.Errorf("Expected key 'userId', got '%s'", ctx.CollectionVars[0].Key)
	}
	if ctx.CollectionVars[0].Value != "123" {
		t.Errorf("Expected value '123', got '%s'", ctx.CollectionVars[0].Value)
	}
}

func TestExecuteTestScript_ResponseJSON_Invalid(t *testing.T) {
	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"const data = pm.response.json();",
		},
	}

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 200,
			Body:       `not valid json`,
		},
	}

	result := runtime.ExecuteTestScript(script, ctx)

	if len(result.Errors) == 0 {
		t.Error("Expected error for invalid JSON")
	}
	if len(result.Errors) > 0 && !contains(result.Errors[0], "failed to parse JSON") {
		t.Errorf("Expected 'failed to parse JSON' error, got: %s", result.Errors[0])
	}
}

func TestExecuteTestScript_ResponseJSON_NestedObjects(t *testing.T) {
	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"const data = pm.response.json();",
			"pm.test('can access nested properties', () => {",
			"    if (data.user.name !== 'Alice') {",
			"        throw new Error('Expected name Alice');",
			"    }",
			"    if (data.user.address.city !== 'NYC') {",
			"        throw new Error('Expected city NYC');",
			"    }",
			"    if (data.tags[0] !== 'admin') {",
			"        throw new Error('Expected first tag admin');",
			"    }",
			"});",
		},
	}

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 200,
			Body: `{
				"user": {
					"name": "Alice",
					"address": {
						"city": "NYC",
						"zip": "10001"
					}
				},
				"tags": ["admin", "user"]
			}`,
		},
	}

	result := runtime.ExecuteTestScript(script, ctx)

	if len(result.Errors) > 0 {
		t.Errorf("Expected no errors, got: %v", result.Errors)
	}
	if len(result.Tests) != 1 {
		t.Fatalf("Expected 1 test, got %d", len(result.Tests))
	}
	if !result.Tests[0].Passed {
		t.Errorf("Expected test to pass, error: %s", result.Tests[0].Error)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestExecuteTestScript_CollectionVariableGet(t *testing.T) {
	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"const value = pm.collectionVariables.get('existingKey');",
			"pm.test('can retrieve existing variable', () => {",
			"    if (value !== 'existingValue') {",
			"        throw new Error('Expected existingValue, got ' + value);",
			"    }",
			"});",
			"const missing = pm.collectionVariables.get('nonExistent');",
			"pm.test('missing variable returns undefined', () => {",
			"    if (missing !== undefined) {",
			"        throw new Error('Expected undefined, got ' + missing);",
			"    }",
			"});",
		},
	}

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 200,
			Body:       "",
		},
		CollectionVars: []postman.Variable{
			{Key: "existingKey", Value: "existingValue"},
		},
	}

	result := runtime.ExecuteTestScript(script, ctx)

	if len(result.Errors) > 0 {
		t.Errorf("Expected no errors, got: %v", result.Errors)
	}
	if len(result.Tests) != 2 {
		t.Fatalf("Expected 2 tests, got %d", len(result.Tests))
	}
	if !result.Tests[0].Passed {
		t.Errorf("Expected first test to pass, error: %s", result.Tests[0].Error)
	}
	if !result.Tests[1].Passed {
		t.Errorf("Expected second test to pass, error: %s", result.Tests[1].Error)
	}
}

func TestExecuteTestScript_EnvironmentVariableGet(t *testing.T) {
	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"const token = pm.environmentVariables.get('apiToken');",
			"pm.test('can retrieve environment variable', () => {",
			"    if (token !== 'secret123') {",
			"        throw new Error('Expected secret123, got ' + token);",
			"    }",
			"});",
		},
	}

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 200,
			Body:       "",
		},
		EnvironmentVars: []postman.EnvVariable{
			{Key: "apiToken", Value: "secret123", Enabled: true},
		},
	}

	result := runtime.ExecuteTestScript(script, ctx)

	if len(result.Errors) > 0 {
		t.Errorf("Expected no errors, got: %v", result.Errors)
	}
	if len(result.Tests) != 1 {
		t.Fatalf("Expected 1 test, got %d", len(result.Tests))
	}
	if !result.Tests[0].Passed {
		t.Errorf("Expected test to pass, error: %s", result.Tests[0].Error)
	}
}

func TestExecuteTestScript_VariablesGetPrecedence(t *testing.T) {
	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"const envOverride = pm.variables.get('sharedKey');",
			"pm.test('env variable takes precedence', () => {",
			"    if (envOverride !== 'envValue') {",
			"        throw new Error('Expected envValue, got ' + envOverride);",
			"    }",
			"});",
			"const collectionOnly = pm.variables.get('collectionKey');",
			"pm.test('falls back to collection variable', () => {",
			"    if (collectionOnly !== 'collectionValue') {",
			"        throw new Error('Expected collectionValue, got ' + collectionOnly);",
			"    }",
			"});",
			"const envOnly = pm.variables.get('envKey');",
			"pm.test('can get env-only variable', () => {",
			"    if (envOnly !== 'envOnlyValue') {",
			"        throw new Error('Expected envOnlyValue, got ' + envOnly);",
			"    }",
			"});",
		},
	}

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 200,
			Body:       "",
		},
		CollectionVars: []postman.Variable{
			{Key: "sharedKey", Value: "collectionValue"},
			{Key: "collectionKey", Value: "collectionValue"},
		},
		EnvironmentVars: []postman.EnvVariable{
			{Key: "sharedKey", Value: "envValue", Enabled: true},
			{Key: "envKey", Value: "envOnlyValue", Enabled: true},
		},
	}

	result := runtime.ExecuteTestScript(script, ctx)

	if len(result.Errors) > 0 {
		t.Errorf("Expected no errors, got: %v", result.Errors)
	}
	if len(result.Tests) != 3 {
		t.Fatalf("Expected 3 tests, got %d", len(result.Tests))
	}
	for i, test := range result.Tests {
		if !test.Passed {
			t.Errorf("Test %d (%s) failed: %s", i, test.Name, test.Error)
		}
	}
}

func TestExecuteTestScript_VariablesSet(t *testing.T) {
	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"pm.variables.set('newVar', 'newValue');",
			"const retrieved = pm.collectionVariables.get('newVar');",
			"pm.test('pm.variables.set() sets in collection scope', () => {",
			"    if (retrieved !== 'newValue') {",
			"        throw new Error('Expected newValue, got ' + retrieved);",
			"    }",
			"});",
		},
	}

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 200,
			Body:       "",
		},
		CollectionVars:  []postman.Variable{},
		EnvironmentVars: []postman.EnvVariable{},
	}

	result := runtime.ExecuteTestScript(script, ctx)

	if len(result.Errors) > 0 {
		t.Errorf("Expected no errors, got: %v", result.Errors)
	}
	if len(result.Tests) != 1 {
		t.Fatalf("Expected 1 test, got %d", len(result.Tests))
	}
	if !result.Tests[0].Passed {
		t.Errorf("Expected test to pass, error: %s", result.Tests[0].Error)
	}
	if len(ctx.CollectionVars) != 1 {
		t.Fatalf("Expected 1 collection variable, got %d", len(ctx.CollectionVars))
	}
	if ctx.CollectionVars[0].Key != "newVar" {
		t.Errorf("Expected key 'newVar', got '%s'", ctx.CollectionVars[0].Key)
	}
	if ctx.CollectionVars[0].Value != "newValue" {
		t.Errorf("Expected value 'newValue', got '%s'", ctx.CollectionVars[0].Value)
	}
}
