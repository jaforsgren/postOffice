package tui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	internalhttp "postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/script"
)

const scriptSetToken = "fresh-token"

// completeScriptedRequest finishes a request whose script set {{token}} in
// the environment the request ran against.
func completeScriptedRequest(t *testing.T, m Model) Model {
	t.Helper()
	ran := m.environment
	ran.Values = append(ran.Values, postman.EnvVariable{Key: "token", Value: scriptSetToken, Enabled: true})
	updated, _ := m.Update(RequestCompleteMsg{
		ItemName:    "Login",
		Response:    &internalhttp.Response{},
		TestResult:  &script.TestResult{},
		Environment: ran,
	})
	return updated.(Model)
}

func TestRequestComplete_ScriptVariablesInAMergedEnvironmentSurviveAVariableEdit(t *testing.T) {
	m, _, envPath := requestVariablesModel(t)
	m.parser.ToggleActiveEnvironment("dev")
	m.environment = m.parser.MergedEnvironment()
	m.mergedEnvironmentActive = true

	m = completeScriptedRequest(t, m)
	m = pressKeys(t, m, runes(requestVariablesKey), runes("k"), tea.KeyMsg{Type: tea.KeyEnter})
	m.editFieldInput.SetValue("c2")
	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if got, ok := lookupEnvValue(m.environment, "token"); !ok || got != scriptSetToken {
		t.Errorf("expected the script's token after the edit, got %q (defined %v)", got, ok)
	}
	if data, _ := os.ReadFile(envPath); !strings.Contains(string(data), scriptSetToken) {
		t.Errorf("expected the token saved to the environment file, got %s", data)
	}
}

func TestRequestComplete_ScriptVariablesInALoadedEnvironmentAreSaved(t *testing.T) {
	m, _, envPath := requestVariablesModel(t)

	m = completeScriptedRequest(t, m)

	if data, _ := os.ReadFile(envPath); !strings.Contains(string(data), scriptSetToken) {
		t.Errorf("expected the token saved to the environment file, got %s", data)
	}
}

func lookupEnvValue(environment *postman.Environment, key string) (string, bool) {
	for _, variable := range environment.Values {
		if variable.Key == key {
			return variable.Value, true
		}
	}
	return "", false
}
