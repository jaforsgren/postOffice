package script

import (
	"postOffice/internal/postman"
	"testing"
)

func TestVariableUpdatesPersistInPointer(t *testing.T) {
	collection := &postman.Collection{
		Info: postman.Info{
			Name: "Test Collection",
		},
		Variables: []postman.Variable{
			{Key: "existingVar", Value: "initialValue"},
		},
	}

	originalPointer := collection
	originalVarsPointer := &collection.Variables

	runtime := NewRuntime()
	script := postman.Script{
		Type: "text/javascript",
		Exec: []string{
			"pm.collectionVariables.set('newVar', 'newValue');",
			"pm.collectionVariables.set('existingVar', 'updatedValue');",
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

	if len(result.Errors) > 0 {
		t.Errorf("Expected no errors, got: %v", result.Errors)
	}

	collection.Variables = ctx.CollectionVars

	if collection != originalPointer {
		t.Error("Collection pointer changed!")
	}

	if len(collection.Variables) != 2 {
		t.Errorf("Expected 2 variables, got %d", len(collection.Variables))
	}

	foundNew := false
	foundUpdated := false

	for _, v := range collection.Variables {
		if v.Key == "newVar" && v.Value == "newValue" {
			foundNew = true
		}
		if v.Key == "existingVar" && v.Value == "updatedValue" {
			foundUpdated = true
		}
	}

	if !foundNew {
		t.Error("Expected new variable 'newVar' to be added")
	}
	if !foundUpdated {
		t.Error("Expected 'existingVar' to be updated to 'updatedValue'")
	}

	t.Logf("Original vars pointer: %p", originalVarsPointer)
	t.Logf("Current vars pointer: %p", &collection.Variables)
	t.Logf("Variables: %+v", collection.Variables)
}
