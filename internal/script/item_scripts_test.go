package script

import (
	"testing"

	"postOffice/internal/postman"
)

func itemWithScript(listen string, lines ...string) *postman.Item {
	return &postman.Item{
		Name:   "request",
		Events: []postman.Event{{Listen: listen, Script: postman.Script{Type: "text/javascript", Exec: lines}}},
	}
}

func TestRunItemPreRequestScripts_WritesVariablesBack(t *testing.T) {
	collection := &postman.Collection{}
	environment := &postman.Environment{}
	item := itemWithScript("prerequest",
		"pm.collectionVariables.set('from_collection', 'c');",
		"pm.environment.set('from_env', 'e');",
	)

	errors := RunItemPreRequestScripts(item, collection, environment)

	if len(errors) != 0 {
		t.Fatalf("unexpected errors: %v", errors)
	}
	if len(collection.Variables) != 1 || collection.Variables[0].Value != "c" {
		t.Errorf("collection variables = %+v", collection.Variables)
	}
	if len(environment.Values) != 1 || environment.Values[0].Value != "e" {
		t.Errorf("environment values = %+v", environment.Values)
	}
}

func TestRunItemPreRequestScripts_ReturnsScriptErrors(t *testing.T) {
	errors := RunItemPreRequestScripts(itemWithScript("prerequest", "throw new Error('boom');"), nil, nil)

	if len(errors) != 1 {
		t.Errorf("errors = %v, want one", errors)
	}
}

func TestRunItemTestScripts_ReadsResponseAndWritesVariablesBack(t *testing.T) {
	environment := &postman.Environment{}
	item := itemWithScript("test",
		"pm.environment.set('claim_id', pm.response.json().claimId);",
		"pm.test('ok', function () { pm.response.to.have.status(200); });",
	)

	result := RunItemTestScripts(item, nil, environment, &ResponseData{StatusCode: 200, Body: `{"claimId":"LF-1"}`})

	if len(result.Tests) != 1 || !result.Tests[0].Passed {
		t.Errorf("tests = %+v, want one passing test", result.Tests)
	}
	if len(environment.Values) != 1 || environment.Values[0].Value != "LF-1" {
		t.Errorf("environment values = %+v", environment.Values)
	}
}

func TestRunItemTestScripts_NilItem(t *testing.T) {
	if result := RunItemTestScripts(nil, nil, nil, &ResponseData{}); result != nil {
		t.Errorf("result = %+v, want nil", result)
	}
}
