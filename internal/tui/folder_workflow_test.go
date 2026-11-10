package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"postOffice/internal/postman"
	"postOffice/internal/workflow"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const folderRunTestCollectionName = "Folder Run Collection"

var (
	ctrlEKey   = tea.KeyMsg{Type: tea.KeyCtrlE}
	confirmKey = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(confirmFolderRunKey)}
	cancelKey  = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(cancelFolderRunKey)}
)

func folderRunTestRequest(name string) postman.Item {
	return postman.Item{
		Name:    name,
		Request: &postman.Request{Method: "GET", URL: postman.URL{Raw: "https://example.com/" + name}},
	}
}

// writeFolderRunCollection writes a collection with a "policies/TEST" folder (holding a nested
// subfolder) and a plain "Other" folder to a temp dir, returning the collection file path.
func writeFolderRunCollection(t *testing.T) string {
	t.Helper()
	collection := postman.Collection{
		Info: postman.Info{Name: folderRunTestCollectionName},
		Items: []postman.Item{
			{
				Name: "policies",
				Items: []postman.Item{
					{
						Name: "TEST",
						Items: []postman.Item{
							folderRunTestRequest("setup"),
							{Name: "invite", Items: []postman.Item{folderRunTestRequest("redeem")}},
							folderRunTestRequest("cleanup"),
						},
					},
				},
			},
			{Name: "Other", Items: []postman.Item{folderRunTestRequest("ping")}},
		},
	}
	data, err := json.Marshal(collection)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "collection.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// folderRunTestModel opens the collection at path inside its "policies" folder, so the cursor
// starts on the "TEST" folder.
func folderRunTestModel(t *testing.T, path string) Model {
	t.Helper()
	parser := createMockParser()
	collection, err := parser.LoadCollection(path)
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(parser)
	m.width = 200
	m.collection = collection
	m.mode = ModeRequests
	m = m.loadRequestsList()
	m = m.navigateInto(collection.Items[0])
	m.cursor = 0
	return m
}

func pressKey(m Model, key tea.KeyMsg) (Model, tea.Cmd) {
	updated, cmd := m.Update(key)
	return updated.(Model), cmd
}

var testFolderPath = []string{"policies", "TEST"}

func TestCtrlE_OnFolderAsksForConfirmation(t *testing.T) {
	path := writeFolderRunCollection(t)
	m := folderRunTestModel(t, path)

	m, cmd := pressKey(m, ctrlEKey)

	if cmd != nil {
		t.Error("ctrl+e on a folder must not start anything before confirmation")
	}
	if m.pendingFolderRun == nil {
		t.Fatal("expected a pending folder run after ctrl+e on a folder")
	}
	wantPrompt := "Run folder 'policies/TEST' as workflow? (y/n)"
	if m.statusMessage != wantPrompt {
		t.Errorf("statusMessage = %q, want %q", m.statusMessage, wantPrompt)
	}
	if !strings.Contains(m.renderCommandBar(), wantPrompt) {
		t.Errorf("command bar should show the prompt, got %q", m.renderCommandBar())
	}
	if _, err := os.Stat(workflow.WorkflowPath(path, workflow.FolderWorkflowID(testFolderPath))); !os.IsNotExist(err) {
		t.Error("no workflow must be saved before confirmation")
	}
}

func TestCtrlE_OnRequestStillExecutesIt(t *testing.T) {
	m := folderRunTestModel(t, writeFolderRunCollection(t))
	m = m.navigateInto(m.currentItems[0])
	m.cursor = 0

	m, cmd := pressKey(m, ctrlEKey)

	if m.pendingFolderRun != nil {
		t.Error("ctrl+e on a request must not ask to run a folder")
	}
	if cmd == nil {
		t.Error("ctrl+e on a request should start executing it")
	}
}

func TestFolderRunConfirm_CancelKeysAbort(t *testing.T) {
	for name, key := range map[string]tea.KeyMsg{
		"n":     cancelKey,
		"esc":   {Type: tea.KeyEsc},
		"other": {Type: tea.KeyRunes, Runes: []rune("j")},
	} {
		path := writeFolderRunCollection(t)
		m := folderRunTestModel(t, path)
		m, _ = pressKey(m, ctrlEKey)

		m, cmd := pressKey(m, key)

		if m.pendingFolderRun != nil {
			t.Errorf("%s: prompt should be cleared", name)
		}
		if cmd != nil {
			t.Errorf("%s: nothing should run after cancelling", name)
		}
		if m.mode != ModeRequests || m.cursor != 0 {
			t.Errorf("%s: the cancelling key must not also navigate (mode %v, cursor %d)", name, m.mode, m.cursor)
		}
		if _, err := os.Stat(workflow.WorkflowsDir(path)); !os.IsNotExist(err) {
			t.Errorf("%s: no workflow must be saved after cancelling", name)
		}
	}
}

func TestFolderRunConfirm_SavesAndRunsGeneratedWorkflow(t *testing.T) {
	path := writeFolderRunCollection(t)
	m := folderRunTestModel(t, path)
	m, _ = pressKey(m, ctrlEKey)

	m, cmd := pressKey(m, confirmKey)

	if cmd == nil {
		t.Fatal("confirming should start the workflow run")
	}
	if m.mode != ModeWorkflowDetail {
		t.Errorf("mode = %v, want ModeWorkflowDetail (the workflow progress view)", m.mode)
	}

	saved, err := workflow.LoadWorkflow(workflow.WorkflowPath(path, "policies-test"))
	if err != nil {
		t.Fatalf("expected the folder workflow to be saved: %v", err)
	}
	if saved.Name != "policies/TEST" {
		t.Errorf("saved Name = %q, want %q", saved.Name, "policies/TEST")
	}
	wantRequests := []string{"policies/TEST/setup", "policies/TEST/invite/redeem", "policies/TEST/cleanup"}
	if len(saved.Steps) != len(wantRequests) {
		t.Fatalf("saved %d steps, want %d", len(saved.Steps), len(wantRequests))
	}
	for i, request := range wantRequests {
		if saved.Steps[i].Request != request {
			t.Errorf("Steps[%d].Request = %q, want %q", i, saved.Steps[i].Request, request)
		}
	}

	if m.activeWorkflow == nil || m.activeWorkflow.ID != "policies-test" {
		t.Errorf("activeWorkflow = %v, want the folder workflow", m.activeWorkflow)
	}
	if findWorkflowByID(m.workflows, "policies-test") == nil {
		t.Error("the saved workflow should be in the workflow list")
	}
	if m.previousMode != ModeRequests {
		t.Errorf("previousMode = %v, want ModeRequests so esc returns to the folder", m.previousMode)
	}
}

func TestFolderRunConfirm_ReusesExistingSavedWorkflow(t *testing.T) {
	path := writeFolderRunCollection(t)
	m := folderRunTestModel(t, path)
	folder := m.currentItems[0]

	existing, err := workflow.NewFolderWorkflow(folder, testFolderPath)
	if err != nil {
		t.Fatal(err)
	}
	existing.Script = "export default async function (wf, pm) {\n  await wf.run(\"cleanup\");\n}\n"
	wfPath := workflow.WorkflowPath(path, existing.ID)
	if err := workflow.SaveWorkflow(existing, wfPath); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(wfPath)
	if err != nil {
		t.Fatal(err)
	}

	m, _ = pressKey(m, ctrlEKey)
	m, _ = pressKey(m, confirmKey)

	after, err := os.ReadFile(wfPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Error("an existing folder workflow must be run, not regenerated")
	}
	if m.activeWorkflow == nil || m.activeWorkflow.Script != existing.Script {
		t.Error("expected the saved workflow (with its own script) to be the one running")
	}
	entries, err := os.ReadDir(workflow.WorkflowsDir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("workflows dir holds %d files, want 1 (no duplicate)", len(entries))
	}
}

func TestFolderRunConfirm_RejectsWhileWorkflowRunning(t *testing.T) {
	m := folderRunTestModel(t, writeFolderRunCollection(t))
	m.workflowChan = make(chan workflow.ExecutionState)

	m, _ = pressKey(m, ctrlEKey)

	if m.pendingFolderRun != nil {
		t.Error("no prompt should be shown while a workflow is already running")
	}
	if m.statusMessage != "Workflow already running" {
		t.Errorf("statusMessage = %q, want %q", m.statusMessage, "Workflow already running")
	}
}

func folderLine(t *testing.T, m Model, name string) string {
	t.Helper()
	for i, item := range m.currentItems {
		if item.Name == name {
			return ansi.Strip(m.formatItemLine(i, itemNameWidth))
		}
	}
	t.Fatalf("no row for %q", name)
	return ""
}

func TestFolderWorkflowIndicator_ShownOnceSaved(t *testing.T) {
	m := folderRunTestModel(t, writeFolderRunCollection(t))
	if strings.Contains(folderLine(t, m, "TEST"), folderWorkflowIndicator) {
		t.Fatal("folder without a saved workflow must not show the indicator")
	}

	m, _ = pressKey(m, ctrlEKey)
	m, _ = pressKey(m, confirmKey)
	escKey := tea.KeyMsg{Type: tea.KeyEsc}
	m, _ = pressKey(m, escKey) // workflow progress view → workflow list
	if findWorkflowByID(m.workflows, "policies-test") == nil {
		t.Error("the workflow list should show the saved folder workflow")
	}
	m, _ = pressKey(m, escKey) // workflow list → requests view

	if m.mode != ModeRequests {
		t.Fatalf("mode = %v, want ModeRequests after leaving the workflow views", m.mode)
	}
	if !strings.Contains(folderLine(t, m, "TEST"), folderWorkflowIndicator) {
		t.Error("folder should show the indicator as soon as its workflow is saved")
	}
}

func TestFolderWorkflowIndicator_ShownOnStartupForWorkflowOnDisk(t *testing.T) {
	path := writeFolderRunCollection(t)
	seed := folderRunTestModel(t, path)
	wf, err := workflow.NewFolderWorkflow(seed.currentItems[0], testFolderPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := workflow.SaveWorkflow(wf, workflow.WorkflowPath(path, wf.ID)); err != nil {
		t.Fatal(err)
	}

	m := folderRunTestModel(t, path)

	if !strings.Contains(folderLine(t, m, "TEST"), folderWorkflowIndicator) {
		t.Error("folder with a workflow on disk should show the indicator on startup")
	}
}

func TestFolderWorkflowIndicator_KeepsColumnAlignment(t *testing.T) {
	path := writeFolderRunCollection(t)
	m := folderRunTestModel(t, path)
	m = m.navigateUp()
	m.savedWorkflowIDs[workflow.FolderWorkflowID([]string{"policies"})] = true

	marked := folderLine(t, m, "policies")
	plain := folderLine(t, m, "Other")

	if !strings.Contains(marked, folderWorkflowIndicator) {
		t.Fatalf("expected indicator on %q", marked)
	}
	if ansi.StringWidth(marked) != ansi.StringWidth(plain) {
		t.Errorf("rows differ in display width (%d vs %d): %q / %q",
			ansi.StringWidth(marked), ansi.StringWidth(plain), marked, plain)
	}
}
