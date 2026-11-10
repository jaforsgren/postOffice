package postman

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

var personaChoices = []string{"01", "02", "03"}

func choiceVariables() []VariableSource {
	return []VariableSource{{Key: "persona_id", Value: "02", Choices: personaChoices}}
}

func TestResolveVariables_ChoiceVariableResolvesToTheChosenValue(t *testing.T) {
	if got := ResolveVariables("/p/{{persona_id}}", choiceVariables()); got != "/p/02" {
		t.Errorf("expected the chosen value, got %q", got)
	}
}

func TestResolveVariables_IndexedChoiceReference(t *testing.T) {
	if got := ResolveVariables("/p/{{persona_id[2]}}/{{ persona_id[0] }}", choiceVariables()); got != "/p/03/01" {
		t.Errorf("expected the indexed choices, got %q", got)
	}
}

func TestResolveVariables_OutOfRangeChoiceStaysUnresolved(t *testing.T) {
	if got := ResolveVariables("{{persona_id[3]}}", choiceVariables()); got != "{{persona_id[3]}}" {
		t.Errorf("expected the reference left as is, got %q", got)
	}
}

func TestReferencedVariables_IndexedChoiceCountsAsTheVariable(t *testing.T) {
	req := &Request{URL: URL{Raw: "https://x/{{persona_id[1]}}/{{persona_id}}"}}

	if got := ReferencedVariables(req); !slices.Equal(got, []string{"persona_id"}) {
		t.Errorf("expected only persona_id, got %v", got)
	}
}

func TestGetAllVariables_CarriesChoices(t *testing.T) {
	environment := &Environment{Name: "dev", Values: []EnvVariable{{Key: "persona_id", Value: "02", Choices: personaChoices, Enabled: true}}}
	collection := &Collection{Variables: []Variable{{Key: "tenant", Value: "a", Choices: []string{"a", "b"}}}}

	variables := GetAllVariables(collection, nil, environment)

	if len(variables) != 2 || !slices.Equal(variables[0].Choices, personaChoices) || len(variables[1].Choices) != 2 {
		t.Errorf("expected both variables' choices, got %+v", variables)
	}
}

func TestVariableValue_WithChoiceKeepsThePlainValueAsAChoice(t *testing.T) {
	got := VariableValue{Value: "01"}.WithChoice("02")

	if got.Value != "02" || !slices.Equal(got.Choices, []string{"01", "02"}) {
		t.Errorf("expected [01 02] with 02 chosen, got %+v", got)
	}
}

func TestVariableValue_WithChoiceDoesNotDuplicate(t *testing.T) {
	got := VariableValue{Value: "02", Choices: personaChoices}.WithChoice("01")

	if got.Value != "01" || !slices.Equal(got.Choices, personaChoices) {
		t.Errorf("expected the choices unchanged with 01 chosen, got %+v", got)
	}
}

func TestVariableValue_WithChosen(t *testing.T) {
	value := VariableValue{Value: "02", Choices: personaChoices}

	if got := value.WithChosen(2); got.Value != "03" || got.ChoiceIndex() != 2 {
		t.Errorf("expected 03 chosen, got %+v", got)
	}
	if got := value.WithChosen(len(personaChoices)); got.Value != "02" {
		t.Errorf("expected an out of range index ignored, got %+v", got)
	}
}

func TestVariableValue_WithoutTheChosenChoiceChoosesTheFirst(t *testing.T) {
	got := VariableValue{Value: "02", Choices: personaChoices}.WithoutChoice(1)

	if got.Value != "01" || !slices.Equal(got.Choices, []string{"01", "03"}) {
		t.Errorf("expected [01 03] with 01 chosen, got %+v", got)
	}
}

func TestVariableValue_WithoutChoiceDownToOneIsPlain(t *testing.T) {
	got := VariableValue{Value: "01", Choices: []string{"01", "02"}}.WithoutChoice(0)

	if got.Value != "02" || got.IsChoice() {
		t.Errorf("expected plain 02, got %+v", got)
	}
}

func TestSetVariable_StoresChoicesInTheEnvironment(t *testing.T) {
	environment := &Environment{Name: "dev", Values: []EnvVariable{{Key: "persona_id", Value: "01", Enabled: true}}}

	SetVariable(&Collection{}, nil, []*Environment{environment}, "persona_id", VariableValue{Value: "03", Choices: personaChoices})

	if got := environment.Values[0]; got.Value != "03" || !slices.Equal(got.Choices, personaChoices) {
		t.Errorf("expected 03 with the choices stored, got %+v", got)
	}
}

func TestEnvVariable_ChoicesRoundTripThroughJSON(t *testing.T) {
	data, err := json.Marshal(EnvVariable{Key: "persona_id", Value: "02", Choices: personaChoices, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"choices":["01","02","03"]`) {
		t.Errorf("expected the choices written, got %s", data)
	}

	var decoded EnvVariable
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(decoded.Choices, personaChoices) {
		t.Errorf("expected the choices read back, got %+v", decoded)
	}

	plain, _ := json.Marshal(EnvVariable{Key: "k", Value: "v"})
	if strings.Contains(string(plain), "choices") {
		t.Errorf("expected no choices key on a plain variable, got %s", plain)
	}
}
