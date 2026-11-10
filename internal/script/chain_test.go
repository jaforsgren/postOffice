package script

import (
	"slices"
	"strings"
	"testing"

	"postOffice/internal/postman"
)

func jsScript(lines ...string) postman.Script {
	return postman.Script{Type: "text/javascript", Exec: lines}
}

func event(phase Phase, lines ...string) postman.Event {
	return postman.Event{Listen: string(phase), Script: jsScript(lines...)}
}

func logOrder(name string) string {
	return "pm.collectionVariables.set('order', (pm.collectionVariables.get('order') || '') + '" + name + ",');"
}

func chainCollection() *postman.Collection {
	return &postman.Collection{
		Info:   postman.Info{Name: "Chain"},
		Events: []postman.Event{event(PhasePreRequest, logOrder("collection")), event(PhaseTest, logOrder("collection-test"))},
		Items: []postman.Item{
			{
				Name:   "outer",
				Events: []postman.Event{event(PhasePreRequest, logOrder("outer"))},
				Items: []postman.Item{
					{
						Name:   "inner",
						Events: []postman.Event{event(PhasePreRequest, logOrder("inner"))},
						Items: []postman.Item{
							{
								Name:    "request",
								Request: &postman.Request{Method: "GET", URL: postman.URL{Raw: "{{base}}/things"}},
								Events:  []postman.Event{event(PhasePreRequest, logOrder("request"))},
							},
						},
					},
				},
			},
		},
	}
}

func collectionVar(c *postman.Collection, key string) string {
	for _, v := range c.Variables {
		if v.Key == key {
			return v.Value
		}
	}
	return ""
}

func TestRequestRun_RunsCollectionFoldersThenRequest(t *testing.T) {
	collection := chainCollection()
	item := collection.Items[0].Items[0].Items[0]
	scope := RequestScope{Collection: collection, FolderPath: []string{"outer", "inner"}, Item: &item}

	result := NewRequestRun(scope).run(PhasePreRequest, NewRequestData(item.Request), nil)

	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if got := collectionVar(collection, "order"); got != "collection,outer,inner,request," {
		t.Errorf("expected collection → folders → request order, got %q", got)
	}
}

func TestRequestRun_OnlyRunsScriptsOfThePhase(t *testing.T) {
	collection := chainCollection()
	item := collection.Items[0].Items[0].Items[0]
	scope := RequestScope{Collection: collection, FolderPath: []string{"outer", "inner"}, Item: &item}

	NewRequestRun(scope).Test(nil, &ResponseData{StatusCode: 200})

	if got := collectionVar(collection, "order"); got != "collection-test," {
		t.Errorf("expected only the collection test script, got %q", got)
	}
}

func TestRequestRun_UnknownFolderStopsFolderWalk(t *testing.T) {
	collection := chainCollection()
	item := collection.Items[0].Items[0].Items[0]
	scope := RequestScope{Collection: collection, FolderPath: []string{"missing", "inner"}, Item: &item}

	NewRequestRun(scope).PreRequest(item.Request)

	if got := collectionVar(collection, "order"); got != "collection,request," {
		t.Errorf("expected folder scripts below an unknown folder to be skipped, got %q", got)
	}
}

func TestExecuteChain_ImplicitGlobalsCarryAcrossScripts(t *testing.T) {
	scopes := [][]postman.Event{
		{event(PhaseTest, "helpers = { double: n => n * 2 };")},
		{event(PhaseTest, "pm.test('uses collection helper', () => pm.expect(helpers.double(2)).to.eql(4));")},
	}

	result := NewRuntime().ExecuteChain(PhaseTest, scopes, &ExecutionContext{Response: &ResponseData{StatusCode: 200}})

	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(result.Tests) != 1 || !result.Tests[0].Passed {
		t.Fatalf("expected helper test to pass, got %+v", result.Tests)
	}
}

func TestExecuteChain_SameConstInEachScriptIsNotARedeclaration(t *testing.T) {
	scopes := [][]postman.Event{
		{event(PhaseTest, "const CLIENT_ERROR = 400;")},
		{event(PhaseTest, "const CLIENT_ERROR = 400;", "pm.test('ran', () => {});")},
	}

	result := NewRuntime().ExecuteChain(PhaseTest, scopes, &ExecutionContext{Response: &ResponseData{StatusCode: 200}})

	if len(result.Errors) > 0 {
		t.Fatalf("expected per-script scopes, got errors: %v", result.Errors)
	}
	if len(result.Tests) != 1 {
		t.Fatalf("expected request script to run, got %d tests", len(result.Tests))
	}
}

func TestExecuteChain_SkipStopsRemainingPreRequestScripts(t *testing.T) {
	ctx := &ExecutionContext{}
	scopes := [][]postman.Event{
		{event(PhasePreRequest, "pm.execution.skipRequest();", "pm.collectionVariables.set('afterSkipSameScript', 'yes');")},
		{event(PhasePreRequest, "pm.collectionVariables.set('nextScript', 'ran');")},
	}

	result := NewRuntime().ExecuteChain(PhasePreRequest, scopes, ctx)

	if !result.Skipped {
		t.Fatal("expected request to be skipped")
	}
	keys := make([]string, len(ctx.CollectionVars))
	for i, v := range ctx.CollectionVars {
		keys[i] = v.Key
	}
	if !slices.Contains(keys, "afterSkipSameScript") {
		t.Error("expected the skipping script itself to run to completion")
	}
	if slices.Contains(keys, "nextScript") {
		t.Error("expected scripts after the skip not to run")
	}
}

func TestExecuteChain_ScriptErrorDoesNotStopLaterScripts(t *testing.T) {
	scopes := [][]postman.Event{
		{event(PhaseTest, "undefinedFunction();")},
		{event(PhaseTest, "pm.test('still runs', () => {});")},
	}

	result := NewRuntime().ExecuteChain(PhaseTest, scopes, &ExecutionContext{Response: &ResponseData{StatusCode: 200}})

	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0], "undefinedFunction") {
		t.Errorf("expected the first script's error, got %v", result.Errors)
	}
	if len(result.Tests) != 1 {
		t.Errorf("expected the second script to run, got %d tests", len(result.Tests))
	}
}

func TestRequestScopeInfo(t *testing.T) {
	t.Run("uses the item id when present", func(t *testing.T) {
		scope := RequestScope{FolderPath: []string{"a"}, Item: &postman.Item{ID: "abc-123", Name: "get"}}
		if info := scope.info(); info.RequestID != "abc-123" || info.RequestName != "get" {
			t.Errorf("unexpected info %+v", info)
		}
	})
	t.Run("falls back to the folder path", func(t *testing.T) {
		scope := RequestScope{FolderPath: []string{"a", "b"}, Item: &postman.Item{Name: "get"}}
		if info := scope.info(); info.RequestID != "a/b/get" {
			t.Errorf("expected path id, got %q", info.RequestID)
		}
	})
}

func TestRequestRun_ExposesEnvironmentNameAndWritesBackVariables(t *testing.T) {
	collection := &postman.Collection{Variables: []postman.Variable{{Key: "stale", Value: "1"}}}
	environment := &postman.Environment{Name: "Big tusks staging", Values: []postman.EnvVariable{{Key: "token", Value: "old"}}}
	item := &postman.Item{
		Name: "req",
		Events: []postman.Event{event(PhasePreRequest,
			"pm.collectionVariables.set('envName', pm.environment.name);",
			"pm.collectionVariables.unset('stale');",
			"pm.environment.set('token', 'new');",
		)},
	}

	NewRequestRun(RequestScope{Collection: collection, Environment: environment, Item: item}).PreRequest(&postman.Request{})

	if got := collectionVar(collection, "envName"); got != "Big tusks staging" {
		t.Errorf("expected environment name, got %q", got)
	}
	if collectionVar(collection, "stale") != "" {
		t.Error("expected unset variable to be removed from the collection")
	}
	if environment.Values[0].Value != "new" {
		t.Errorf("expected environment write-back, got %q", environment.Values[0].Value)
	}
}

func TestRequestRun_PreRequestAppliesMutationsWithoutTouchingOriginal(t *testing.T) {
	original := &postman.Request{
		Method: "POST",
		URL:    postman.URL{Raw: "{{base}}/offers"},
		Header: []postman.Header{{Key: "Accept", Value: "application/json"}},
		Body:   &postman.Body{Mode: "raw", Raw: `{"a":1}`},
	}
	item := &postman.Item{Name: "offer", Request: original, Events: []postman.Event{event(PhasePreRequest,
		"pm.request.body.update(JSON.stringify({ a: 2 }));",
		"pm.request.headers.upsert({ key: 'X-Trace', value: 'on' });",
	)}}

	sent, result := NewRequestRun(RequestScope{Item: item}).PreRequest(original)

	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if sent.Body.Raw != `{"a":2}` {
		t.Errorf("expected updated body, got %q", sent.Body.Raw)
	}
	if len(sent.Header) != 2 || sent.Header[1].Key != "X-Trace" {
		t.Errorf("expected added header, got %+v", sent.Header)
	}
	if original.Body.Raw != `{"a":1}` || len(original.Header) != 1 {
		t.Error("expected the collection's request to stay untouched")
	}
}

func TestRequestRun_GlobalsFromPreRequestSurviveIntoTestPhase(t *testing.T) {
	item := &postman.Item{Name: "offer", Events: []postman.Event{
		event(PhasePreRequest, "sapi = { isoDate: () => '2026-11-10' };"),
		event(PhaseTest, "pm.test('helper still defined', () => pm.expect(sapi.isoDate()).to.eql('2026-11-10'));"),
	}}
	run := NewRequestRun(RequestScope{Item: item})

	run.PreRequest(&postman.Request{})
	result := run.Test(nil, &ResponseData{StatusCode: 200})

	if len(result.Errors) > 0 || len(result.Tests) != 1 || !result.Tests[0].Passed {
		t.Fatalf("expected the pre-request global in the test phase, got %+v", result)
	}
}
