package tui

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"postOffice/internal/postman"
)

var personaChoices = []string{"p1", "p2", "p3"}

// choiceVariablesModel is requestVariablesModel with {{persona_id}} as a
// choice variable, opened in the request variables panel on its row.
func choiceVariablesModel(t *testing.T) (Model, string) {
	t.Helper()
	m, _, envPath := requestVariablesModel(t)
	m.environment.Values[0].Choices = slices.Clone(personaChoices)
	m = pressKeys(t, m, runes(requestVariablesKey), runes("k"), runes("k"))
	return m, envPath
}

func savedPersona(t *testing.T, m Model) postman.EnvVariable {
	t.Helper()
	environment, _ := m.parser.GetEnvironment("dev")
	return environment.Values[0]
}

func TestRequestVariables_EnterOnAChoiceVariableOpensItsChoices(t *testing.T) {
	m, _ := choiceVariablesModel(t)

	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.requestVarEdit != requestVarEditChoices || m.requestVarChoiceCursor != 0 {
		t.Fatalf("expected the choices open on the chosen one, got state %v cursor %d", m.requestVarEdit, m.requestVarChoiceCursor)
	}
	view := m.renderRequestVariables(40)
	for _, choice := range personaChoices {
		if !strings.Contains(view, choice) {
			t.Errorf("expected %s listed, got %s", choice, view)
		}
	}
}

func TestRequestVariables_ChoosingSavesToTheEnvironmentFile(t *testing.T) {
	m, envPath := choiceVariablesModel(t)

	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyEnter}, runes("j"), runes("j"), tea.KeyMsg{Type: tea.KeyEnter})

	if got := savedPersona(t, m); got.Value != "p3" || !slices.Equal(got.Choices, personaChoices) {
		t.Errorf("expected p3 chosen with the choices kept, got %+v", got)
	}
	if !fileContains(t, envPath, `"p3"`) {
		t.Error("expected the environment file to hold the chosen value")
	}
	if m.requestVarEdit != requestVarEditNone {
		t.Errorf("expected the choices closed, got %v", m.requestVarEdit)
	}
}

func TestRequestVariables_AddTurnsAPlainVariableIntoChoices(t *testing.T) {
	m, _, envPath := requestVariablesModel(t)
	m = pressKeys(t, m, runes(requestVariablesKey), runes("k"), runes("k"), runes(addChoiceKey))
	m.editFieldInput.SetValue("p9")

	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if got := savedPersona(t, m); got.Value != "p9" || !slices.Equal(got.Choices, []string{"p1", "p9"}) {
		t.Errorf("expected [p1 p9] with p9 chosen, got %+v", got)
	}
	if !fileContains(t, envPath, `"choices"`) {
		t.Error("expected the environment file to hold the choices")
	}
}

func TestRequestVariables_AddFromTheChoices(t *testing.T) {
	m, _ := choiceVariablesModel(t)
	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyEnter}, runes(addChoiceKey))
	m.editFieldInput.SetValue("p4")

	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if got := savedPersona(t, m); got.Value != "p4" || len(got.Choices) != 4 {
		t.Errorf("expected p4 added and chosen, got %+v", got)
	}
}

func TestRequestVariables_EscFromANewChoiceReturnsToTheChoices(t *testing.T) {
	m, _ := choiceVariablesModel(t)

	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyEnter}, runes(addChoiceKey), tea.KeyMsg{Type: tea.KeyEsc})

	if m.requestVarEdit != requestVarEditChoices {
		t.Errorf("expected the choices again, got %v", m.requestVarEdit)
	}
}

func TestRequestVariables_DeleteChoice(t *testing.T) {
	m, _ := choiceVariablesModel(t)

	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyEnter}, runes("j"), runes(deleteChoiceKey))

	if got := savedPersona(t, m); got.Value != "p1" || !slices.Equal(got.Choices, []string{"p1", "p3"}) {
		t.Errorf("expected p2 removed, got %+v", got)
	}
	if m.requestVarEdit != requestVarEditChoices || m.requestVarChoiceCursor != 1 {
		t.Errorf("expected the choices still open on p3, got state %v cursor %d", m.requestVarEdit, m.requestVarChoiceCursor)
	}
}

func TestRequestVariables_DeletingDownToOneChoiceMakesItPlain(t *testing.T) {
	m, _ := choiceVariablesModel(t)
	m.environment.Values[0].Choices = []string{"p1", "p2"}

	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyEnter}, runes(deleteChoiceKey))

	if got := savedPersona(t, m); got.Value != "p2" || len(got.Choices) != 0 {
		t.Errorf("expected plain p2, got %+v", got)
	}
	if m.requestVarEdit != requestVarEditNone {
		t.Errorf("expected the choices closed, got %v", m.requestVarEdit)
	}
}

func TestRequestVariables_TopBarListsThePanelsShortcuts(t *testing.T) {
	m, _ := choiceVariablesModel(t)

	if shortcuts := strings.Join(m.contextualShortcuts(), " "); !strings.Contains(shortcuts, "Add choice") {
		t.Errorf("expected the panel's shortcuts, got %s", shortcuts)
	}

	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if shortcuts := strings.Join(m.contextualShortcuts(), " "); !strings.Contains(shortcuts, "Delete choice") {
		t.Errorf("expected the choices' shortcuts, got %s", shortcuts)
	}
}

func TestRequestVariables_IndexedReferenceResolvesInTheRequest(t *testing.T) {
	m, _ := choiceVariablesModel(t)

	got := postman.ResolveVariables("{{persona_id[2]}}", m.variablesAt(nil))

	if got != "p3" {
		t.Errorf("expected the third choice, got %q", got)
	}
}
