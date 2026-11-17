package script

import (
	"testing"

	"postOffice/internal/postman"
)

func scriptVariablesContext() *ExecutionContext {
	return &ExecutionContext{
		EnvironmentVars: []postman.EnvVariable{
			{Key: "persona_id", Value: "p2", Choices: []string{"p1", "p2"}, Enabled: true},
			{Key: "quoted", Value: `say "hi"` + "\n", Enabled: true},
		},
		CollectionVars: []postman.Variable{{Key: "persona_id", Value: "shadowed"}, {Key: "tenant", Value: "acme"}},
		Iteration:      Iteration{Data: IterationData{"row": 7.0}},
	}
}

func TestResolveScriptVariables(t *testing.T) {
	cases := []struct {
		name, code, want string
	}{
		{"code becomes a string literal", `const id = {{persona_id}};`, `const id = "p2";`},
		{"environment shadows collection", `x({{persona_id}}, {{tenant}})`, `x("p2", "acme")`},
		{"choice reference", `const first = {{persona_id[0]}};`, `const first = "p1";`},
		{"iteration data first", `const row = {{row}};`, `const row = "7";`},
		{"inside a double quoted string", `h("Bearer {{tenant}}")`, `h("Bearer acme")`},
		{"escaped for its quote", `log('{{quoted}}'); log("{{quoted}}")`, `log('say "hi"\n'); log("say \"hi\"\n")`},
		{"template literal", "`/p/{{persona_id}}`", "`/p/p2`"},
		{"line comment", "// uses {{tenant}}\nconst t = {{tenant}};", "// uses acme\nconst t = \"acme\";"},
		{"block comment", "/* {{tenant}} */ {{tenant}}", "/* acme */ \"acme\""},
		{"escaped quote keeps the string open", `s("a\" {{tenant}}")`, `s("a\" acme")`},
		{"unknown left as written", `const x = {{missing}};`, `const x = {{missing}};`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveScriptVariables(tc.code, scriptVariablesContext()); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestExecuteChain_ScriptSeesVariablesSetByAnEarlierScript(t *testing.T) {
	ctx := &ExecutionContext{}
	scopes := [][]postman.Event{
		{{Listen: string(PhasePreRequest), Script: postman.Script{Exec: []string{`pm.collectionVariables.set("token", "abc")`}}}},
		{{Listen: string(PhasePreRequest), Script: postman.Script{Exec: []string{`const token = {{token}};`, `pm.collectionVariables.set("seen", token)`}}}},
	}

	result := NewRuntime().ExecuteChain(PhasePreRequest, scopes, ctx)

	if len(result.Errors) > 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
	if value, _ := makeVariableGetter(&ctx.CollectionVars)("seen"); value != "abc" {
		t.Errorf("expected the resolved token, got %q", value)
	}
}

func TestRequestRun_ScriptsResolveFolderVariables(t *testing.T) {
	item := &postman.Item{Name: "r", Request: &postman.Request{}, Events: []postman.Event{{
		Listen: string(PhasePreRequest),
		Script: postman.Script{Exec: []string{
			`pm.collectionVariables.set("from_reference", {{tenant}});`,
			`pm.collectionVariables.set("from_get", pm.variables.get("tenant"));`,
			`pm.collectionVariables.set("shadowed", {{region}});`,
		}},
	}}}
	collection := &postman.Collection{
		Variables: []postman.Variable{{Key: "region", Value: "collection"}},
		Items: []postman.Item{{Name: "Accounts", Variables: []postman.Variable{
			{Key: "tenant", Value: "folder-tenant"},
			{Key: "region", Value: "folder"},
		}, Items: []postman.Item{*item}}},
	}

	result := NewRequestRun(RequestScope{Collection: collection, FolderPath: []string{"Accounts"}, Item: item}).run(PhasePreRequest, NewRequestData(item.Request), nil)

	if len(result.Errors) > 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
	get := makeVariableGetter(&collection.Variables)
	for key, want := range map[string]string{"from_reference": "folder-tenant", "from_get": "folder-tenant", "shadowed": "collection"} {
		if got, _ := get(key); got != want {
			t.Errorf("%s: got %q, want %q", key, got, want)
		}
	}
}
