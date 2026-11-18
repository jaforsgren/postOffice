package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"postOffice/internal/workflow"
)

const deleteRequestKey = "d"

func TestConfirmation_ConfirmKeyRunsTheAction(t *testing.T) {
	ran := false
	m := Model{}.askConfirmation("Go?", "Stayed", func(m Model) (Model, tea.Cmd) {
		ran = true
		return m, nil
	})

	m = pressKeys(t, m, runes(confirmKey))

	if !ran {
		t.Error("expected the action to run on the confirm key")
	}
	if m.pendingConfirmation != nil {
		t.Error("expected the prompt cleared")
	}
}

func TestConfirmation_AnyOtherKeyCancels(t *testing.T) {
	for name, key := range map[string]tea.KeyMsg{
		"n":     runes(cancelKey),
		"esc":   {Type: tea.KeyEsc},
		"other": runes("j"),
	} {
		ran := false
		m := Model{}.askConfirmation("Go?", "Stayed", func(m Model) (Model, tea.Cmd) {
			ran = true
			return m, nil
		})

		m = pressKeys(t, m, key)

		if ran {
			t.Errorf("%s: the action must not run", name)
		}
		if m.pendingConfirmation != nil || m.statusMessage != "Stayed" {
			t.Errorf("%s: expected the prompt cleared with the cancelled status, got %q", name, m.statusMessage)
		}
	}
}

func TestDeleteRequestKey_AsksBeforeDeleting(t *testing.T) {
	m, pathA, _ := combinedModel(t)
	m = cursorOn(t, openFolder(t, m, combinedA), usersRequest)

	m = pressKeys(t, m, runes(deleteRequestKey))

	wantPrompt := "Delete request '" + usersRequest + "'? (y/n)"
	if m.statusMessage != wantPrompt {
		t.Errorf("statusMessage = %q, want %q", m.statusMessage, wantPrompt)
	}
	if !strings.Contains(m.renderCommandBar(), wantPrompt) {
		t.Errorf("command bar should show the prompt, got %q", m.renderCommandBar())
	}
	if !fileContains(t, pathA, usersRequest) {
		t.Error("nothing may be deleted before confirmation")
	}
}

func TestDeleteRequestKey_CancelKeepsTheRequest(t *testing.T) {
	m, pathA, _ := combinedModel(t)
	m = cursorOn(t, openFolder(t, m, combinedA), usersRequest)

	m = pressKeys(t, m, runes(deleteRequestKey), runes(cancelKey))

	if !fileContains(t, pathA, usersRequest) || len(m.currentItems) != 1 {
		t.Error("expected the request kept after answering no")
	}
}

func TestDeleteRequestKey_ConfirmDeletesTheRequest(t *testing.T) {
	m, pathA, _ := combinedModel(t)
	m = cursorOn(t, openFolder(t, m, combinedA), usersRequest)

	m = pressKeys(t, m, runes(deleteRequestKey), runes(confirmKey))

	if fileContains(t, pathA, usersRequest) {
		t.Errorf("expected the request deleted after answering yes (%s)", m.statusMessage)
	}
}

func TestDeleteWorkflowStepKey_AsksBeforeDeleting(t *testing.T) {
	m := createTestModel()
	m.mode = ModeWorkflowDetail
	m.activeWorkflow = &workflow.Workflow{ID: "wf", Steps: []workflow.Step{{ID: "login"}, {ID: "fetch"}}}

	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyCtrlD})

	if m.statusMessage != `Delete step "login"? (y/n)` {
		t.Errorf("statusMessage = %q, want the delete prompt", m.statusMessage)
	}

	m = pressKeys(t, m, runes(cancelKey))

	if len(m.activeWorkflow.Steps) != 2 {
		t.Errorf("expected both steps kept after answering no, got %d", len(m.activeWorkflow.Steps))
	}
}
