package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func ctrlC() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyCtrlC} }

func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestCtrlC_QuitsFromEveryView(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m, _, _ := combinedModel(t)
	m = cursorOn(t, openFolder(t, m, combinedA), usersRequest)
	editing := m.enterEditMode(m.currentItems[m.cursor], m.folderPathForCursor())
	commandMode := m
	commandMode.commandMode = true

	for name, model := range map[string]Model{"requests": m, "edit form": editing, "command bar": commandMode} {
		if _, cmd := model.Update(ctrlC()); !quits(cmd) {
			t.Errorf("%s: expected ctrl+c to quit", name)
		}
	}
}

func TestQ_BacksOutOfTheListsAndQuitsFromCollections(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m, _, _ := combinedModel(t)
	m = openFolder(t, m, combinedA)

	m = pressKeys(t, m, runes("q"))
	if m.mode != ModeRequests || len(m.breadcrumb) != 0 {
		t.Fatalf("expected q to go up a folder, got %v at %v", m.mode, m.breadcrumb)
	}
	m = pressKeys(t, m, runes("q"))
	if m.mode != ModeCollections {
		t.Fatalf("expected q at the top to open the collections list, got %v", m.mode)
	}
	if _, cmd := m.Update(runes("q")); !quits(cmd) {
		t.Error("expected q to quit from the collections list")
	}
}

func TestQ_LeavesEnvironmentsForCollections(t *testing.T) {
	m := environmentsModel(t, "dev")
	m.mode = ModeEnvironments

	m = pressKeys(t, m, runes("q"))

	if m.mode != ModeCollections {
		t.Errorf("expected the collections list, got %v", m.mode)
	}
}

func TestNumberKeys_SwitchViews(t *testing.T) {
	m, _, _ := combinedModel(t)
	m.mode = ModeResponse

	for _, step := range []struct {
		key  string
		want ViewMode
	}{{environmentsViewKey, ModeEnvironments}, {variablesViewKey, ModeVariables}, {collectionsViewKey, ModeCollections}, {requestsViewKey, ModeRequests}} {
		m = pressKeys(t, m, runes(step.key))
		if m.mode != step.want {
			t.Fatalf("%s: expected %v, got %v", step.key, step.want, m.mode)
		}
	}
}

func TestWorkflowDetail_PartialRunsAreRepeatable(t *testing.T) {
	m := workflowKeysTestModel()
	m, _ = pressKey(m, runes(runStepOnlyKey))
	m.workflowChan = nil

	m, _ = pressKey(m, runes(repeatLastActionKey))

	if m.workflowChan == nil {
		t.Errorf("expected . to run the step again (%s)", m.statusMessage)
	}
}

func TestWorkflowDetail_RefreshKeepsTheOpenWorkflow(t *testing.T) {
	m := workflowKeysTestModel()
	open := m.activeWorkflow

	m, _ = pressKey(m, runes(refreshWorkflowsKey))

	if m.mode != ModeWorkflowDetail || m.activeWorkflow != open {
		t.Errorf("expected to stay in the open workflow, got %v (%s)", m.mode, m.statusMessage)
	}
}
