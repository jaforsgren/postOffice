package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"postOffice/internal/postman"
)

const variablesRequestName = "Get Claim"

func writeJSONFile(t *testing.T, name string, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// requestVariablesModel has a request using {{persona_id}} from the
// environment, {{claim_id}} from the collection and an undefined {{trace}},
// with both files on disk so saves can be checked.
func requestVariablesModel(t *testing.T) (Model, string, string) {
	t.Helper()
	parser := postman.NewParser()
	collectionPath := writeJSONFile(t, "collection.json", postman.Collection{
		Info:      postman.Info{Name: "Claims API"},
		Variables: []postman.Variable{{Key: "claim_id", Value: "c1"}},
		Items: []postman.Item{{
			Name:    variablesRequestName,
			Request: &postman.Request{Method: "GET", URL: postman.URL{Raw: "https://x/{{persona_id}}/claims/{{claim_id}}?t={{trace}}"}},
		}},
	})
	envPath := writeJSONFile(t, "env.json", postman.Environment{
		Name:   "dev",
		Values: []postman.EnvVariable{{Key: "persona_id", Value: "p1", Enabled: true}},
	})
	collection, err := parser.LoadCollection(collectionPath)
	if err != nil {
		t.Fatal(err)
	}
	environment, err := parser.LoadEnvironment(envPath)
	if err != nil {
		t.Fatal(err)
	}

	m := NewModel(parser)
	m.collection = collection
	m.environment = environment
	m.mode = ModeRequests
	m = m.loadRequestsList()
	return m, collectionPath, envPath
}

func pressKeys(t *testing.T, m Model, keys ...tea.KeyMsg) Model {
	t.Helper()
	for _, key := range keys {
		updated, _ := m.Update(key)
		m = updated.(Model)
	}
	return m
}

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestRequestVariables_ListsWhatTheRequestUses(t *testing.T) {
	m, _, _ := requestVariablesModel(t)

	m = pressKeys(t, m, runes(requestVariablesKey))

	if m.mode != ModeRequestVariables {
		t.Fatalf("expected the request variables view, got mode %v", m.mode)
	}
	rows := m.requestVariableRows()
	if len(rows) != 3 {
		t.Fatalf("expected 3 variables, got %+v", rows)
	}
	if rows[0].Key != "persona_id" || !strings.HasPrefix(rows[0].Source, "Environment") {
		t.Errorf("expected persona_id from the environment, got %+v", rows[0])
	}
	if rows[1].Key != "claim_id" || !strings.HasPrefix(rows[1].Source, "Collection") {
		t.Errorf("expected claim_id from the collection, got %+v", rows[1])
	}
	if rows[2].Defined {
		t.Errorf("expected trace to be undefined, got %+v", rows[2])
	}
}

func TestRequestVariables_EditSavesToTheEnvironmentFile(t *testing.T) {
	m, _, envPath := requestVariablesModel(t)
	m = pressKeys(t, m, runes(requestVariablesKey), runes("e"))
	m.editFieldInput.SetValue("p2")

	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	data, _ := os.ReadFile(envPath)
	if !strings.Contains(string(data), `"p2"`) {
		t.Errorf("expected the environment file to hold the new value, got %s", data)
	}
	if got := m.requestVariableRows()[0].Value; got != "p2" {
		t.Errorf("expected the view to show the new value, got %q", got)
	}
}

func TestRequestVariables_EditSavesToTheCollectionFile(t *testing.T) {
	m, collectionPath, _ := requestVariablesModel(t)
	m = pressKeys(t, m, runes(requestVariablesKey), runes("j"), tea.KeyMsg{Type: tea.KeyEnter})
	m.editFieldInput.SetValue("c2")

	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	data, _ := os.ReadFile(collectionPath)
	if !strings.Contains(string(data), `"c2"`) {
		t.Errorf("expected the collection file to hold the new value, got %s", data)
	}
	if m.statusIsError {
		t.Errorf("expected no error, got %q", m.statusMessage)
	}
}

func TestRequestVariables_EscCancelsEditWithoutSaving(t *testing.T) {
	m, _, envPath := requestVariablesModel(t)
	m = pressKeys(t, m, runes(requestVariablesKey), runes("e"))
	m.editFieldInput.SetValue("p2")

	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	data, _ := os.ReadFile(envPath)
	if strings.Contains(string(data), `"p2"`) {
		t.Error("expected the cancelled edit not to be saved")
	}
	if m.mode != ModeRequestVariables {
		t.Error("expected esc to stay in the variables view while editing")
	}

	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ModeRequests {
		t.Errorf("expected a second esc to return to the requests view, got %v", m.mode)
	}
}

func TestRequestVariables_MergedEnvironmentsSaveToTheDefiningFile(t *testing.T) {
	m, _, envPath := requestVariablesModel(t)
	otherPath := writeJSONFile(t, "other.json", postman.Environment{Name: "other", Values: []postman.EnvVariable{{Key: "unrelated", Value: "x", Enabled: true}}})
	if _, err := m.parser.LoadEnvironment(otherPath); err != nil {
		t.Fatal(err)
	}
	m.parser.ToggleActiveEnvironment("other")
	m.parser.ToggleActiveEnvironment("dev")
	m.environment = m.parser.MergedEnvironment()
	m.mergedEnvironmentActive = true

	m = pressKeys(t, m, runes(requestVariablesKey), runes("e"))
	m.editFieldInput.SetValue("p3")
	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	data, _ := os.ReadFile(envPath)
	if !strings.Contains(string(data), `"p3"`) {
		t.Errorf("expected the dev environment file to be updated, got %s", data)
	}
	if got := m.requestVariableRows()[0].Value; got != "p3" {
		t.Errorf("expected the merged view to show the new value, got %q", got)
	}
}

func TestRequestVariables_FolderIsRejected(t *testing.T) {
	m := createTestModel()
	m.cursor = 0

	m = pressKeys(t, m, runes(requestVariablesKey))

	if m.mode == ModeRequestVariables {
		t.Error("expected a folder not to open the variables view")
	}
}
