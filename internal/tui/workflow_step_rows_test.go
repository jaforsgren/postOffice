package tui

import (
	"slices"
	"strings"
	"testing"

	"postOffice/internal/workflow"

	tea "github.com/charmbracelet/bubbletea"
)

var (
	expandStepKeyMsg     = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(expandFolderKey)}
	expandAllStepsKeyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(expandAllFoldersKey)}
	downKeyMsg           = tea.KeyMsg{Type: tea.KeyDown}
)

// folderWorkflowDetailModel opens the generated "policies/TEST" workflow (setup, the "invite"
// folder step, cleanup) in the workflow detail view, with the cursor on setup.
func folderWorkflowDetailModel(t *testing.T) Model {
	t.Helper()
	m := folderRunTestModel(t, writeFolderRunCollection(t))
	wf, err := workflow.NewFolderPathWorkflow(m.collection, testFolderPath)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.openWorkflowDetail(wf)
	return m
}

func rowRunIDs(m Model) []string {
	var ids []string
	for _, row := range m.workflowStepRows() {
		ids = append(ids, row.runID)
	}
	return ids
}

func cursorOnStep(t *testing.T, m Model, runID string) Model {
	t.Helper()
	index := slices.Index(rowRunIDs(m), runID)
	if index < 0 {
		t.Fatalf("no row %q in %v", runID, rowRunIDs(m))
	}
	m.workflowStepCursor = index
	return m
}

func TestWorkflowStepRows_folderStepsStartCollapsed(t *testing.T) {
	m := folderWorkflowDetailModel(t)

	if got := rowRunIDs(m); !slices.Equal(got, []string{"setup", "invite", "cleanup"}) {
		t.Errorf("rows = %v, want only the workflow's own steps", got)
	}
}

func TestWorkflowStepRows_expandingANotRunFolderStepShowsItsWorkflow(t *testing.T) {
	m := cursorOnStep(t, folderWorkflowDetailModel(t), "invite")

	m, _ = pressKey(m, expandStepKeyMsg)

	if got := rowRunIDs(m); !slices.Equal(got, []string{"setup", "invite", "invite/redeem", "cleanup"}) {
		t.Fatalf("rows = %v, want invite's sub-step under it", got)
	}
	nested := m.workflowStepRows()[2]
	if nested.isTopLevel() || nested.step.Request != "policies/TEST/invite/redeem" || !slices.Equal(nested.groups, []string{"invite"}) {
		t.Errorf("nested row = %+v, want the redeem request grouped under invite", nested)
	}

	m, _ = pressKey(m, expandStepKeyMsg)
	if got := rowRunIDs(m); !slices.Equal(got, []string{"setup", "invite", "cleanup"}) {
		t.Errorf("rows after collapsing = %v", got)
	}
}

func TestWorkflowStepRows_expandingARunFolderStepShowsItsRun(t *testing.T) {
	m := cursorOnStep(t, folderWorkflowDetailModel(t), "invite")
	redeem := &workflow.StepState{ID: "invite/redeem", Request: "policies/TEST/invite/redeem", Groups: []string{"invite"}, Status: workflow.StatusFailed, Iteration: 1}
	m.workflowState = workflow.ExecutionState{
		WorkflowID: m.activeWorkflow.ID,
		Iteration:  1,
		Steps:      []*workflow.StepState{{ID: "invite", Folder: "policies/TEST/invite", Status: workflow.StatusFailed, Iteration: 1}, redeem},
		StepIndex:  map[string]*workflow.StepState{"invite/redeem": redeem},
	}

	m, _ = pressKey(m, expandStepKeyMsg)
	m, _ = pressKey(m, downKeyMsg)

	if ss := m.selectedRunStepState(); ss != redeem {
		t.Fatalf("selected run state = %+v, want the sub-step's own run", ss)
	}
	if lines := strings.Join(m.buildStepPanelLines(80), "\n"); !strings.Contains(lines, "invite/redeem") || !strings.Contains(lines, workflow.StatusFailed) {
		t.Errorf("step panel = %q, want the sub-step's run", lines)
	}
}

func TestWorkflowStepRows_expandAllReachesNestedFolderSteps(t *testing.T) {
	m := folderWorkflowDetailModel(t)
	m.activeWorkflow.Steps = append(m.activeWorkflow.Steps, workflow.Step{ID: "policies", Folder: "policies"})

	m, _ = pressKey(m, expandAllStepsKeyMsg)

	ids := rowRunIDs(m)
	if !slices.Contains(ids, "policies/test/invite/redeem") {
		t.Fatalf("rows = %v, want the deepest sub-step policies → TEST → invite → redeem", ids)
	}

	m, _ = pressKey(m, expandAllStepsKeyMsg)
	if got := rowRunIDs(m); len(got) != len(m.activeWorkflow.Steps) {
		t.Errorf("rows after collapsing all = %v", got)
	}
}

func TestWorkflowStepRows_cursorMovesThroughExpandedRows(t *testing.T) {
	m := cursorOnStep(t, folderWorkflowDetailModel(t), "invite")
	m, _ = pressKey(m, expandStepKeyMsg)

	for range len(m.workflowStepRows()) + 1 {
		m, _ = pressKey(m, downKeyMsg)
	}

	if row, _ := m.selectedWorkflowRow(); row.runID != "cleanup" {
		t.Errorf("cursor on %q, want it to stop on the last row", row.runID)
	}
}

func TestWorkflowStepRows_nestedStepCannotBeDeleted(t *testing.T) {
	m := cursorOnStep(t, folderWorkflowDetailModel(t), "invite")
	m, _ = pressKey(m, expandStepKeyMsg)
	m = cursorOnStep(t, m, "invite/redeem")
	steps := slices.Clone(m.activeWorkflow.Steps)

	deleted, _ := m.deleteWorkflowStep()

	if !slices.Equal(deleted.activeWorkflow.Steps, steps) || !strings.Contains(deleted.statusMessage, errNestedWorkflowStep.Error()) {
		t.Errorf("steps %v, status %q, want nothing deleted and the reason", deleted.activeWorkflow.Steps, deleted.statusMessage)
	}
}

func TestWorkflowStepRows_runStepOnANestedStepRunsIt(t *testing.T) {
	m := cursorOnStep(t, folderWorkflowDetailModel(t), "invite")
	m, _ = pressKey(m, expandStepKeyMsg)
	m = cursorOnStep(t, m, "invite/redeem")

	m, cmd := handleWorkflowRunStepKey(m)

	if cmd == nil || !strings.Contains(m.statusMessage, "step: invite/redeem") {
		t.Errorf("status %q, want a run of invite/redeem started", m.statusMessage)
	}
	if m.cancelWorkflowRun != nil {
		m.cancelWorkflowRun()
	}
}

func TestWorkflowStepRows_expandingARequestStepIsRefused(t *testing.T) {
	m, _ := pressKey(folderWorkflowDetailModel(t), expandStepKeyMsg)

	if len(m.workflowStepRows()) != len(m.activeWorkflow.Steps) || m.statusMessage != "Only folder steps can be expanded" {
		t.Errorf("rows %v, status %q", rowRunIDs(m), m.statusMessage)
	}
}

func TestRenderWorkflowDetail_marksFolderStepsAndShowsSubSteps(t *testing.T) {
	m := cursorOnStep(t, folderWorkflowDetailModel(t), "invite")
	m.height = 40

	collapsed := m.renderWorkflowDetail()
	m, _ = pressKey(m, expandStepKeyMsg)
	expanded := m.renderWorkflowDetail()

	if !strings.Contains(collapsed, folderStepCollapsedMarker+"invite") {
		t.Errorf("collapsed view should mark invite with %q", folderStepCollapsedMarker)
	}
	if !strings.Contains(expanded, folderStepExpandedMarker) || !strings.Contains(expanded, "redeem") {
		t.Errorf("expanded view should mark invite with %q and list redeem", folderStepExpandedMarker)
	}
}

func TestWorkflowStepRows_expansionBelongsToOneWorkflow(t *testing.T) {
	m := cursorOnStep(t, folderWorkflowDetailModel(t), "invite")
	m, _ = pressKey(m, expandStepKeyMsg)

	other := *m.activeWorkflow
	other.ID = "another-workflow"
	m.activeWorkflow = &other

	if got := rowRunIDs(m); slices.Contains(got, "invite/redeem") {
		t.Errorf("rows = %v, want another workflow's expansion not to carry over", got)
	}
}

func TestWorkflowStepRows_expansionResolvesInTheWorkflowsOwnCollectionAfterRescoping(t *testing.T) {
	m, _, _ := combinedModel(t)
	m, err := m.scopeWorkflowsTo(combinedB)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := m.parser.GetCollection(combinedB)
	wf, err := workflow.NewCollectionWorkflow(b)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.openWorkflowDetail(wf)
	// The combined view rescopes the workflow commands to the cursor's collection, e.g. on :wf.
	if m, err = m.scopeWorkflowsTo(combinedA); err != nil {
		t.Fatal(err)
	}

	m = cursorOnStep(t, m, workflow.FolderWorkflowID([]string{ordersFolder}))
	m, _ = pressKey(m, expandStepKeyMsg)

	if len(m.workflowStepRows()) != len(wf.Steps)+1 || m.statusIsError {
		t.Errorf("rows %v, status %q, want the folder expanded from collection B", rowRunIDs(m), m.statusMessage)
	}
}
