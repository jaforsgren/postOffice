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
