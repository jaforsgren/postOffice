package script

import (
	"encoding/json"
	"os"
	"path/filepath"
	"postOffice/internal/postman"
	"testing"
	"time"
)

func TestIntegration_VariableGetWithPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	collectionPath := filepath.Join(tmpDir, "test_collection.json")
	envPath := filepath.Join(tmpDir, "test_env.json")

	collection := &postman.Collection{
		Info: postman.Info{
			Name: "Test Collection",
		},
		Variables: []postman.Variable{
			{Key: "baseUrl", Value: "https://api.example.com"},
		},
	}

	environment := &postman.Environment{
		Name: "Test Environment",
		Values: []postman.EnvVariable{
			{Key: "apiKey", Value: "initial_key", Enabled: true},
		},
	}

	collectionData, _ := json.MarshalIndent(collection, "", "  ")
	os.WriteFile(collectionPath, collectionData, 0644)

	envData, _ := json.MarshalIndent(environment, "", "  ")
	os.WriteFile(envPath, envData, 0644)

	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"const baseUrl = pm.variables.get('baseUrl');",
			"const apiKey = pm.variables.get('apiKey');",
			"pm.test('can read variables', () => {",
			"    if (!baseUrl || !apiKey) {",
			"        throw new Error('Variables not found');",
			"    }",
			"});",
			"pm.collectionVariables.set('timestamp', Date.now().toString());",
			"pm.environmentVariables.set('apiKey', 'updated_key');",
		},
	}

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 200,
			Body:       "",
		},
		CollectionVars:  collection.Variables,
		EnvironmentVars: environment.Values,
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

	collection.Variables = ctx.CollectionVars
	environment.Values = ctx.EnvironmentVars

	collectionData, _ = json.MarshalIndent(collection, "", "  ")
	os.WriteFile(collectionPath, collectionData, 0644)

	envData, _ = json.MarshalIndent(environment, "", "  ")
	os.WriteFile(envPath, envData, 0644)

	var reloadedCollection postman.Collection
	collectionBytes, _ := os.ReadFile(collectionPath)
	json.Unmarshal(collectionBytes, &reloadedCollection)

	var reloadedEnv postman.Environment
	envBytes, _ := os.ReadFile(envPath)
	json.Unmarshal(envBytes, &reloadedEnv)

	timestampFound := false
	for _, v := range reloadedCollection.Variables {
		if v.Key == "timestamp" {
			timestampFound = true
			break
		}
	}
	if !timestampFound {
		t.Error("Expected timestamp variable to be persisted")
	}

	apiKeyUpdated := false
	for _, v := range reloadedEnv.Values {
		if v.Key == "apiKey" && v.Value == "updated_key" {
			apiKeyUpdated = true
			break
		}
	}
	if !apiKeyUpdated {
		t.Error("Expected apiKey to be updated and persisted")
	}
}

func TestIntegration_TimeoutProtection(t *testing.T) {
	runtime := NewRuntimeWithTimeout(200 * time.Millisecond)

	maliciousScript := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"while(true) { var x = 1; }",
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

	start := time.Now()
	result := runtime.ExecuteTestScript(maliciousScript, ctx)
	duration := time.Since(start)

	if len(result.Errors) == 0 {
		t.Error("Expected timeout error for infinite loop")
	}

	if !contains(result.Errors[0], "timeout") {
		t.Errorf("Expected timeout error, got: %s", result.Errors[0])
	}

	if duration > 500*time.Millisecond {
		t.Errorf("Timeout took too long: %v (expected ~200ms)", duration)
	}

	if len(ctx.CollectionVars) > 0 || len(ctx.EnvironmentVars) > 0 {
		t.Error("Expected no variables to be modified after timeout")
	}
}

func TestIntegration_FullScriptLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	collectionPath := filepath.Join(tmpDir, "lifecycle_collection.json")

	collection := &postman.Collection{
		Info: postman.Info{
			Name: "Lifecycle Test",
		},
		Variables: []postman.Variable{},
	}

	collectionData, _ := json.MarshalIndent(collection, "", "  ")
	os.WriteFile(collectionPath, collectionData, 0644)

	preRequestScript := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"pm.collectionVariables.set('requestTime', Date.now().toString());",
			"pm.collectionVariables.set('requestId', 'req_' + Math.random());",
		},
	}

	preReqCtx := &ExecutionContext{
		CollectionVars:  collection.Variables,
		EnvironmentVars: []postman.EnvVariable{},
	}

	runtime := NewRuntime()
	preReqResult := runtime.ExecutePreRequestScript(preRequestScript, preReqCtx)

	if len(preReqResult.Errors) > 0 {
		t.Errorf("Pre-request script errors: %v", preReqResult.Errors)
	}

	collection.Variables = preReqCtx.CollectionVars

	if len(collection.Variables) != 2 {
		t.Fatalf("Expected 2 variables after pre-request, got %d", len(collection.Variables))
	}

	testScript := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"const requestTime = pm.collectionVariables.get('requestTime');",
			"const requestId = pm.collectionVariables.get('requestId');",
			"pm.test('variables set in pre-request are available', () => {",
			"    if (!requestTime || !requestId) {",
			"        throw new Error('Pre-request variables not found');",
			"    }",
			"});",
			"const responseData = pm.response.json();",
			"pm.test('response has expected structure', () => {",
			"    if (!responseData.status) {",
			"        throw new Error('Missing status');",
			"    }",
			"});",
			"pm.collectionVariables.set('lastResponseStatus', responseData.status);",
		},
	}

	testCtx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 200,
			Body:       `{"status": "success", "data": {"id": 123}}`,
		},
		CollectionVars:  collection.Variables,
		EnvironmentVars: []postman.EnvVariable{},
	}

	testResult := runtime.ExecuteTestScript(testScript, testCtx)

	if len(testResult.Errors) > 0 {
		t.Errorf("Test script errors: %v", testResult.Errors)
	}
	if len(testResult.Tests) != 2 {
		t.Fatalf("Expected 2 tests, got %d", len(testResult.Tests))
	}

	for i, test := range testResult.Tests {
		if !test.Passed {
			t.Errorf("Test %d (%s) failed: %s", i, test.Name, test.Error)
		}
	}

	collection.Variables = testCtx.CollectionVars

	if len(collection.Variables) != 3 {
		t.Fatalf("Expected 3 variables after test, got %d", len(collection.Variables))
	}

	collectionData, _ = json.MarshalIndent(collection, "", "  ")
	os.WriteFile(collectionPath, collectionData, 0644)

	var reloadedCollection postman.Collection
	collectionBytes, _ := os.ReadFile(collectionPath)
	json.Unmarshal(collectionBytes, &reloadedCollection)

	expectedVars := map[string]bool{
		"requestTime":        false,
		"requestId":          false,
		"lastResponseStatus": false,
	}

	for _, v := range reloadedCollection.Variables {
		if _, ok := expectedVars[v.Key]; ok {
			expectedVars[v.Key] = true
		}
	}

	for key, found := range expectedVars {
		if !found {
			t.Errorf("Expected variable '%s' to be persisted", key)
		}
	}

	var statusFound bool
	for _, v := range reloadedCollection.Variables {
		if v.Key == "lastResponseStatus" && v.Value == "success" {
			statusFound = true
			break
		}
	}
	if !statusFound {
		t.Error("Expected lastResponseStatus='success' to be persisted")
	}
}

func TestIntegration_TimeoutDoesNotCorruptVariables(t *testing.T) {
	runtime := NewRuntimeWithTimeout(100 * time.Millisecond)

	collection := &postman.Collection{
		Variables: []postman.Variable{
			{Key: "existingVar", Value: "existingValue"},
		},
	}

	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"pm.collectionVariables.set('newVar', 'newValue');",
			"while(true) {}",
		},
	}

	ctx := &ExecutionContext{
		Response: &ResponseData{
			StatusCode: 200,
			Body:       "",
		},
		CollectionVars: collection.Variables,
	}

	result := runtime.ExecuteTestScript(script, ctx)

	if len(result.Errors) == 0 {
		t.Error("Expected timeout error")
	}

	if len(ctx.CollectionVars) != 2 {
		t.Errorf("Expected 2 variables (newVar set before timeout), got %d", len(ctx.CollectionVars))
	}

	existingVarFound := false
	for _, v := range ctx.CollectionVars {
		if v.Key == "existingVar" && v.Value == "existingValue" {
			existingVarFound = true
			break
		}
	}
	if !existingVarFound {
		t.Error("Expected existing variable to remain intact after timeout")
	}
}
