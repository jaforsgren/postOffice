package script

import (
	"postOffice/internal/postman"
	"testing"
)

func TestExecuteTestScript_ResponseText(t *testing.T) {
	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"const text = pm.response.text();",
			"pm.test('text() returns body', () => {",
			"    if (text !== 'test response body') {",
			"        throw new Error('Expected test response body');",
			"    }",
			"});",
		},
	}

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 200,
			Body:       "test response body",
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

func TestExecuteTestScript_ResponseMetadata(t *testing.T) {
	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"pm.test('response metadata is accessible', () => {",
			"    if (pm.response.code !== 201) {",
			"        throw new Error('Expected code 201, got ' + pm.response.code);",
			"    }",
			"    if (pm.response.status !== '201 Created') {",
			"        throw new Error('Expected status 201 Created');",
			"    }",
			"    if (pm.response.responseTime !== 150) {",
			"        throw new Error('Expected responseTime 150');",
			"    }",
			"});",
		},
	}

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode:   201,
			Status:       "201 Created",
			Body:         "",
			ResponseTime: 150,
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

func TestExecuteTestScript_ResponseHeaders(t *testing.T) {
	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"pm.test('can access headers map', () => {",
			"    const contentType = pm.response.headers['Content-Type'];",
			"    if (!contentType || contentType[0] !== 'application/json') {",
			"        throw new Error('Expected Content-Type application/json');",
			"    }",
			"});",
		},
	}

	headers := make(map[string][]string)
	headers["Content-Type"] = []string{"application/json"}
	headers["X-Custom"] = []string{"test-value"}

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 200,
			Body:       "",
			Headers:    headers,
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

func TestExecuteTestScript_ResponseToHaveHeader(t *testing.T) {
	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"pm.test('header exists', () => {",
			"    pm.response.to.have.header('Content-Type');",
			"});",
			"pm.test('header has expected value', () => {",
			"    pm.response.to.have.header('Content-Type', 'application/json');",
			"});",
		},
	}

	headers := make(map[string][]string)
	headers["Content-Type"] = []string{"application/json"}

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 200,
			Body:       "",
			Headers:    headers,
		},
	}

	result := runtime.ExecuteTestScript(script, ctx)

	if len(result.Errors) > 0 {
		t.Errorf("Expected no errors, got: %v", result.Errors)
	}
	if len(result.Tests) != 2 {
		t.Fatalf("Expected 2 tests, got %d", len(result.Tests))
	}
	for i, test := range result.Tests {
		if !test.Passed {
			t.Errorf("Test %d failed: %s", i, test.Error)
		}
	}
}

func TestExecuteTestScript_ResponseToHaveHeader_NotFound(t *testing.T) {
	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"pm.test('missing header fails', () => {",
			"    pm.response.to.have.header('X-Missing');",
			"});",
		},
	}

	headers := make(map[string][]string)

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 200,
			Body:       "",
			Headers:    headers,
		},
	}

	result := runtime.ExecuteTestScript(script, ctx)

	if len(result.Tests) != 1 {
		t.Fatalf("Expected 1 test, got %d", len(result.Tests))
	}
	if result.Tests[0].Passed {
		t.Error("Expected test to fail for missing header")
	}
	if !contains(result.Tests[0].Error, "not found") {
		t.Errorf("Expected 'not found' in error, got: %s", result.Tests[0].Error)
	}
}

func TestExecuteTestScript_ResponseToBeOk(t *testing.T) {
	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"pm.test('2xx is ok', () => {",
			"    pm.response.to.be.ok;",
			"});",
		},
	}

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 200,
			Body:       "",
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

func TestExecuteTestScript_ResponseToBeOk_Fail(t *testing.T) {
	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"pm.test('4xx is not ok', () => {",
			"    pm.response.to.be.ok;",
			"});",
		},
	}

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 404,
			Body:       "",
		},
	}

	result := runtime.ExecuteTestScript(script, ctx)

	if len(result.Tests) != 1 {
		t.Fatalf("Expected 1 test, got %d", len(result.Tests))
	}
	if result.Tests[0].Passed {
		t.Error("Expected test to fail for 404 status")
	}
}

func TestExecuteTestScript_ResponseToBeError(t *testing.T) {
	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"pm.test('4xx is error', () => {",
			"    pm.response.to.be.error;",
			"});",
		},
	}

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 400,
			Body:       "",
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

func TestExecuteTestScript_ResponseToHaveJsonBody(t *testing.T) {
	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"pm.test('JSONPath exists', () => {",
			"    pm.response.to.have.jsonBody('user.name');",
			"});",
			"pm.test('JSONPath has value', () => {",
			"    pm.response.to.have.jsonBody('user.name', 'Alice');",
			"});",
			"pm.test('nested JSONPath', () => {",
			"    pm.response.to.have.jsonBody('user.address.city', 'NYC');",
			"});",
			"pm.test('array access', () => {",
			"    pm.response.to.have.jsonBody('tags.0', 'admin');",
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
						"city": "NYC"
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
	if len(result.Tests) != 4 {
		t.Fatalf("Expected 4 tests, got %d", len(result.Tests))
	}
	for i, test := range result.Tests {
		if !test.Passed {
			t.Errorf("Test %d (%s) failed: %s", i, test.Name, test.Error)
		}
	}
}

func TestExecuteTestScript_ResponseToHaveJsonBody_NotFound(t *testing.T) {
	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"pm.test('missing path fails', () => {",
			"    pm.response.to.have.jsonBody('missing.path');",
			"});",
		},
	}

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 200,
			Body:       `{"existing": "value"}`,
		},
	}

	result := runtime.ExecuteTestScript(script, ctx)

	if len(result.Tests) != 1 {
		t.Fatalf("Expected 1 test, got %d", len(result.Tests))
	}
	if result.Tests[0].Passed {
		t.Error("Expected test to fail for missing JSONPath")
	}
	if !contains(result.Tests[0].Error, "not found") {
		t.Errorf("Expected 'not found' in error, got: %s", result.Tests[0].Error)
	}
}
