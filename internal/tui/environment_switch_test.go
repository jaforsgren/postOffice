package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"postOffice/internal/history"
	"postOffice/internal/postman"
)

func environmentsModel(t *testing.T, names ...string) Model {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	parser := postman.NewParser()
	for _, name := range names {
		if _, err := parser.LoadEnvironment(writeJSONFile(t, name+".json", postman.Environment{Name: name})); err != nil {
			t.Fatal(err)
		}
	}
	m := NewModel(parser, history.NewStore(""))
	m.mode = ModeRequests
	return m
}

func ctrlN() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyCtrlN} }

func TestNextEnvironment_CyclesThroughTheLoadedEnvironments(t *testing.T) {
	m := environmentsModel(t, "dev", "prod", "test")

	for _, want := range []string{"dev", "prod", "test", "dev"} {
		m = pressKeys(t, m, ctrlN())
		if m.environment == nil || m.environment.Name != want {
			t.Fatalf("expected %s, got %+v", want, m.environment)
		}
	}
	if m.mode != ModeRequests {
		t.Errorf("expected to stay in the requests view, got %v", m.mode)
	}
}

func TestNextEnvironment_LeavesTheCombinedEnvironment(t *testing.T) {
	m := environmentsModel(t, "dev", "prod")
	m.parser.ToggleActiveEnvironment("prod")
	m.parser.ToggleActiveEnvironment("dev")
	m.environment = m.parser.MergedEnvironment()
	m.mergedEnvironmentActive = true

	m = pressKeys(t, m, ctrlN())

	if m.isViewingMergedEnvironment() || m.environment.Name != "dev" {
		t.Errorf("expected the first single environment, got %+v", m.environment)
	}
}

func TestNextEnvironment_MakesTheSwitchedToEnvironmentTheOnlyActiveOne(t *testing.T) {
	m := environmentsModel(t, "dev", "prod")
	m.parser.ToggleActiveEnvironment("dev")
	m.environment, _ = m.parser.GetEnvironment("dev")

	m = pressKeys(t, m, ctrlN())

	if got := m.parser.ActiveEnvironments(); len(got) != 1 || got[0] != "prod" {
		t.Errorf("expected only prod active for the topbar, got %v", got)
	}
}

func TestNextEnvironment_WithoutEnvironments(t *testing.T) {
	m := environmentsModel(t)

	m = pressKeys(t, m, ctrlN())

	if m.environment != nil || m.statusMessage != "No environments loaded" {
		t.Errorf("expected nothing switched, got %+v (%s)", m.environment, m.statusMessage)
	}
}

func TestNextEnvironment_WorksFromTheResponseView(t *testing.T) {
	m := environmentsModel(t, "dev")
	m.mode = ModeResponse

	m = pressKeys(t, m, ctrlN())

	if m.environment == nil || m.mode != ModeResponse {
		t.Errorf("expected the switch in the response view, got %+v in %v", m.environment, m.mode)
	}
}
