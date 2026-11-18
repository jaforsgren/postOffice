package tui

import (
	"errors"
	"os"
	"slices"
	"strings"
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

func TestCombinedView_NewWorkflowGoesToTheCursorsCollection(t *testing.T) {
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

// saveWorkflowsOfBoth saves workflow aID next to collection A and bID next to B.
func saveWorkflowsOfBoth(t *testing.T, pathA, pathB, aID, bID string) {
	t.Helper()
	for path, id := range map[string]string{pathA: aID, pathB: bID} {
		if err := workflow.SaveWorkflow(workflow.NewWorkflow(id), workflow.WorkflowPath(path, id)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCombinedView_WorkflowsListEveryActiveCollection(t *testing.T) {
	m, pathA, pathB := combinedModel(t)
	saveWorkflowsOfBoth(t, pathA, pathB, "a-flow", "b-flow")
	// As on startup: the cursor is on no collection's item.
	m.currentItems = nil
	m.cursor = 0

	m, _ = m.loadWorkflows()

	if m.mode != ModeWorkflows || len(m.workflows) != 2 {
		t.Fatalf("expected both collections' workflows listed, got %d (%s)", len(m.workflows), m.statusMessage)
	}
	owners := map[string]string{}
	for _, wf := range m.workflows {
		owners[wf.ID] = m.workflowOwners[wf]
	}
	if owners["a-flow"] != combinedA || owners["b-flow"] != combinedB {
		t.Errorf("owners = %v, want each workflow owned by its collection", owners)
	}
	if lines := strings.Join(m.buildWorkflowListLines(splitTestHeight), "\n"); !strings.Contains(lines, "["+combinedB+"]") {
		t.Errorf("list should name each workflow's collection:\n%s", lines)
	}
}

func TestCombinedView_OpeningAListedWorkflowScopesToItsCollection(t *testing.T) {
	m, pathA, pathB := combinedModel(t)
	saveWorkflowsOfBoth(t, pathA, pathB, "a-flow", "b-flow")
	m = openFolder(t, m, combinedA)
	m, _ = m.loadWorkflows()
	wf, ok := m.findListedWorkflow(combinedB, "b-flow")
	if !ok {
		t.Fatal("b-flow not listed")
	}
	m.workflowCursor = slices.Index(m.workflows, wf)

	m, _ = m.openWorkflowDetail(wf)

	if m.activeWorkflowCollection != combinedB || m.workflowCollectionName() != combinedB {
		t.Errorf("active collection %q, scope %q, want %s", m.activeWorkflowCollection, m.workflowCollectionName(), combinedB)
	}
}

func TestCombinedView_RunWorkflowByIDFindsItInAnyCollection(t *testing.T) {
	m, pathA, pathB := combinedModel(t)
	saveWorkflowsOfBoth(t, pathA, pathB, "a-flow", "b-flow")
	m = openFolder(t, m, combinedA)

	m, cmd := m.runWorkflowByID("b-flow")

	if cmd == nil || m.activeWorkflowCollection != combinedB {
		t.Errorf("expected b-flow run against %s, got %q (%s)", combinedB, m.activeWorkflowCollection, m.statusMessage)
	}
	if m.cancelWorkflowRun != nil {
		m.cancelWorkflowRun()
	}
}
