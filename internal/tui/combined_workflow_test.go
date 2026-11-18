package tui

import (
	"errors"
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"postOffice/internal/workflow"
)

func fileExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return err == nil
}

func TestCombinedView_FolderRunUsesTheOwningCollection(t *testing.T) {
	m, pathA, pathB := combinedModel(t)
	m = cursorOn(t, openFolder(t, m, combinedB), ordersFolder)
	m = m.requestFolderRun(m.currentItems[m.cursor], m.folderPathForCursor())

	updated, cmd := m.handleConfirmationKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(confirmKey)})
	m = updated.(Model)

	if cmd == nil {
		t.Fatalf("expected the folder to run, got %q", m.statusMessage)
	}
	workflowID := workflow.FolderWorkflowID([]string{ordersFolder})
	if !fileExists(t, workflow.WorkflowPath(pathB, workflowID)) {
		t.Error("expected the workflow saved next to Collection B's file")
	}
	if fileExists(t, workflow.WorkflowPath(pathA, workflowID)) {
		t.Error("expected nothing saved next to Collection A's file")
	}
	if m.workflowCollection() == nil || m.workflowCollectionName() != combinedB {
		t.Errorf("expected the run against %s, got %q", combinedB, m.workflowCollectionName())
	}
	if want := m.requestItemID([]string{combinedB, ordersFolder}, ordersRequest); m.workflowStepItemID(m.activeWorkflow.Steps[0].Request) != want {
		t.Errorf("expected the step keyed like the request (%q), got %q", want, m.workflowStepItemID(m.activeWorkflow.Steps[0].Request))
	}
}

func TestCombinedView_FolderRunOnAGroupFolderRunsTheCollection(t *testing.T) {
	m, _, pathB := combinedModel(t)
	m = cursorOn(t, m, combinedB)
	m = m.requestFolderRun(m.currentItems[m.cursor], m.folderPathForCursor())

	_, cmd := m.handleConfirmationKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(confirmKey)})

	if cmd == nil {
		t.Fatal("expected the collection to run")
	}
	collectionB, _ := m.parser.GetCollection(combinedB)
	generated, _ := workflow.NewCollectionWorkflow(collectionB)
	if !fileExists(t, workflow.WorkflowPath(pathB, generated.ID)) {
		t.Error("expected Collection B's collection workflow saved next to its file")
	}
}

func TestCombinedView_SavedFolderWorkflowIsMarked(t *testing.T) {
	m, _, pathB := combinedModel(t)
	wf := workflow.NewWorkflow(workflow.FolderWorkflowID([]string{ordersFolder}))
	if err := workflow.SaveWorkflow(wf, workflow.WorkflowPath(pathB, wf.ID)); err != nil {
		t.Fatal(err)
	}
	m.savedWorkflowIDsCollection = ""

	m = openFolder(t, m, combinedB)

	if !m.hasSavedFolderWorkflow([]string{combinedB}, ordersFolder) {
		t.Error("expected B's folder marked as having a saved workflow")
	}
	if m.hasSavedFolderWorkflow(nil, combinedB) {
		t.Error("expected the group folder itself unmarked")
	}
}

func TestCombinedView_WorkflowsListTheCursorsCollection(t *testing.T) {
	m, pathA, _ := combinedModel(t)
	m = openFolder(t, m, combinedA)

	m, _ = m.newWorkflow("smoke")

	if !fileExists(t, workflow.WorkflowPath(pathA, "smoke")) {
		t.Fatalf("expected the workflow created next to Collection A's file (%s)", m.statusMessage)
	}
	if m.mode != ModeWorkflows || len(m.workflows) != 1 || m.workflows[0].ID != "smoke" {
		t.Errorf("expected A's workflows listed, got %d (%s)", len(m.workflows), m.statusMessage)
	}
}

func TestCombinedView_WorkflowsNeedACollectionAtTheCursor(t *testing.T) {
	m, _, _ := combinedModel(t)
	m.currentItems = nil
	m.cursor = 0

	m, _ = m.loadWorkflows()

	if m.mode == ModeWorkflows || m.statusMessage != errCombinedWorkflowScope.Error() {
		t.Errorf("expected the workflows view refused, got %v (%s)", m.mode, m.statusMessage)
	}
}
