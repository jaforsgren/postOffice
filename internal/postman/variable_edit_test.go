package postman

import (
	"slices"
	"testing"
)

func TestReferencedVariables_InOrderOfFirstUse(t *testing.T) {
	req := &Request{
		URL:    URL{Raw: "{{base_url}}/personas/{{persona_id}}"},
		Header: []Header{{Key: "Authorization", Value: "Bearer {{token}}"}, {Key: "X-Persona", Value: "{{ persona_id }}"}},
		Body: &Body{
			Raw:      `{"claim": "{{claim_id}}", "trace": "{{$guid}}"}`,
			FormData: []FormParam{{Key: "{{form_key}}", Value: "x"}},
		},
	}

	got := ReferencedVariables(req)

	want := []string{"base_url", "persona_id", "token", "claim_id", "form_key"}
	if !slices.Equal(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestReferencedVariables_NilRequest(t *testing.T) {
	if got := ReferencedVariables(nil); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func variableEditCollection() *Collection {
	return &Collection{
		Info:      Info{Name: "API"},
		Variables: []Variable{{Key: "claim_id", Value: "c1"}, {Key: "shared", Value: "collection"}},
		Items: []Item{{
			Name:      "Claims",
			Variables: []Variable{{Key: "folder_only", Value: "f1"}},
			Items: []Item{{
				Name:      "Nested",
				Variables: []Variable{{Key: "folder_only", Value: "deep"}},
				Items:     []Item{{Name: "Get", Request: &Request{}}},
			}},
		}},
	}
}

func TestSetVariable_EnvironmentWinsOverCollection(t *testing.T) {
	collection := variableEditCollection()
	env := &Environment{Name: "dev", Values: []EnvVariable{{Key: "shared", Value: "env", Enabled: true}}}

	owner := SetVariable(collection, nil, []*Environment{env}, "shared", VariableValue{Value: "new"})

	if owner != (VariableOwner{Scope: VariableScopeEnvironment, Name: "dev"}) {
		t.Errorf("expected the environment to own it, got %+v", owner)
	}
	if env.Values[0].Value != "new" || collection.Variables[1].Value != "collection" {
		t.Errorf("expected only the environment value to change, got env=%q collection=%q", env.Values[0].Value, collection.Variables[1].Value)
	}
}

func TestSetVariable_FirstEnvironmentDefiningKeyWins(t *testing.T) {
	first := &Environment{Name: "dev", Values: []EnvVariable{{Key: "other", Value: "x", Enabled: true}}}
	second := &Environment{Name: "shared", Values: []EnvVariable{{Key: "persona_id", Value: "p1", Enabled: true}}}

	owner := SetVariable(variableEditCollection(), nil, []*Environment{first, second}, "persona_id", VariableValue{Value: "p2"})

	if owner.Name != "shared" || second.Values[0].Value != "p2" {
		t.Errorf("expected the second environment to be updated, got %+v %+v", owner, second.Values)
	}
}

func TestSetVariable_DisabledEnvironmentValueIsSkipped(t *testing.T) {
	collection := variableEditCollection()
	env := &Environment{Name: "dev", Values: []EnvVariable{{Key: "claim_id", Value: "off", Enabled: false}}}

	owner := SetVariable(collection, nil, []*Environment{env}, "claim_id", VariableValue{Value: "c2"})

	if owner.Scope != VariableScopeCollection || collection.Variables[0].Value != "c2" || env.Values[0].Value != "off" {
		t.Errorf("expected the collection variable to be updated, got %+v", owner)
	}
}

func TestSetVariable_OutermostFolderDefiningKeyWins(t *testing.T) {
	collection := variableEditCollection()

	owner := SetVariable(collection, []string{"Claims", "Nested"}, nil, "folder_only", VariableValue{Value: "f2"})

	if owner != (VariableOwner{Scope: VariableScopeFolder, Name: "Claims"}) {
		t.Errorf("expected the Claims folder to own it, got %+v", owner)
	}
	if collection.Items[0].Variables[0].Value != "f2" || collection.Items[0].Items[0].Variables[0].Value != "deep" {
		t.Error("expected only the outer folder value to change")
	}
}

func TestSetVariable_UndefinedBecomesCollectionVariable(t *testing.T) {
	collection := variableEditCollection()

	owner := SetVariable(collection, nil, nil, "new_key", VariableValue{Value: "v"})

	if owner.Scope != VariableScopeCollection {
		t.Errorf("expected a collection variable, got %+v", owner)
	}
	last := collection.Variables[len(collection.Variables)-1]
	if last.Key != "new_key" || last.Value != "v" {
		t.Errorf("expected the new variable to be appended, got %+v", last)
	}
}

func TestUnresolvedVariables(t *testing.T) {
	req := &Request{
		URL:    URL{Raw: "asb://{{topic}}/tap"},
		Header: []Header{{Key: "Connection-String", Value: "{{connection}}"}},
	}

	got := UnresolvedVariables(req, []VariableSource{{Key: "topic", Value: "payments"}})

	if len(got) != 1 || got[0] != "{{connection}}" {
		t.Errorf("UnresolvedVariables() = %v, want only {{connection}}", got)
	}
}
