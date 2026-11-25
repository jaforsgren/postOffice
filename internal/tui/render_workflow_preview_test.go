package tui

import (
	"fmt"
	"strings"
	"testing"

	"postOffice/internal/workflow"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const (
	manyWorkflows        = 100
	manyStepsPerWorkflow = 200
	otherWorkflowID      = "other-wf"
)

var (
	tabKey        = tea.KeyMsg{Type: tea.KeyTab}
	escKey        = tea.KeyMsg{Type: tea.KeyEsc}
	previewDown   = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(workflowPreviewDownKey)}
	previewUp     = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(workflowPreviewUpKey)}
	cursorDownKey = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}
)

func workflowWithSteps(id string, stepCount int) *workflow.Workflow {
	wf := &workflow.Workflow{ID: id, Name: id}
	for i := range stepCount {
		wf.Steps = append(wf.Steps, workflow.Step{ID: fmt.Sprintf("%s-step-%d", id, i), Request: "Folder/Request"})
	}
	return wf
}

func workflowsListTestModel() Model {
	m := panelTestModel()
	m.mode = ModeWorkflows
	m.workflows = []*workflow.Workflow{m.activeWorkflow, workflowWithSteps(otherWorkflowID, manyStepsPerWorkflow)}
	return m
}

func TestWorkflowsList_ShowsOneLinePerWorkflowWithoutInliningSteps(t *testing.T) {
	m := workflowsListTestModel()
	m.workflowCursor = 1

	view := ansi.Strip(m.renderWorkflowsList())

	if strings.Contains(view, otherWorkflowID+"-step-0") {
		t.Errorf("expected the selected workflow's steps not to be listed inline:\n%s", view)
	}
	if !strings.Contains(view, stepCountLabel(manyStepsPerWorkflow)) {
		t.Errorf("expected the step count %q in the list:\n%s", stepCountLabel(manyStepsPerWorkflow), view)
	}
}

func TestWorkflowsList_ScrollsToKeepCursorVisible(t *testing.T) {
	m := workflowsListTestModel()
	m.workflows = nil
	for i := range manyWorkflows {
		m.workflows = append(m.workflows, workflowWithSteps(fmt.Sprintf("wf-%03d", i), 1))
	}
	m.workflowCursor = manyWorkflows - 1
	height := m.calculateLayout().contentHeight

	lines := m.buildWorkflowListLines(height)

	if len(lines) > height {
		t.Errorf("list has %d lines, want at most %d", len(lines), height)
	}
	if last := ansi.Strip(lines[len(lines)-1]); !strings.Contains(last, m.workflows[manyWorkflows-1].Name) {
		t.Errorf("last line = %q, want the selected workflow", last)
	}
}

func TestWorkflowsList_ShowsLastRunStatusOnlyForTheWorkflowThatRan(t *testing.T) {
	m := workflowsListTestModel()

	ran := ansi.Strip(m.workflowListLine(m.workflows[0], false))
	notRun := ansi.Strip(m.workflowListLine(m.workflows[1], false))

	if !strings.Contains(ran, workflow.StatusIcon(workflow.StatusFailed)) {
		t.Errorf("line = %q, want the failed run icon", ran)
	}
	if !strings.Contains(notRun, workflow.StatusIcon(workflow.StatusPending)) {
		t.Errorf("line = %q, want the pending icon", notRun)
	}
}

func TestWorkflowPreview_TabTogglesInWorkflowsList(t *testing.T) {
	m := workflowsListTestModel()

	m, _ = pressKey(m, tabKey)
	if !m.workflowPreviewOpen {
		t.Fatal("expected tab to open the workflow preview")
	}
	if m.workflowStepPanelOpen {
		t.Error("expected the detail's step panel to stay closed")
	}

	m, _ = pressKey(m, tabKey)
	if m.workflowPreviewOpen {
		t.Fatal("expected a second tab to close the workflow preview")
	}
}

func TestWorkflowPreview_EscClosesPreviewBeforeLeavingList(t *testing.T) {
	m := workflowsListTestModel()
	m.workflowPreviewOpen = true

	m, _ = pressKey(m, escKey)

	if m.workflowPreviewOpen {
		t.Error("expected esc to close the workflow preview")
	}
	if m.mode != ModeWorkflows {
		t.Errorf("mode = %v, want to stay in the workflows list", m.mode)
	}
}

func TestWorkflowPreview_ShowsSelectedWorkflowStepsWithLastRunStatus(t *testing.T) {
	m := workflowsListTestModel()
	m.workflowPreviewOpen = true

	preview := ansi.Strip(strings.Join(m.buildWorkflowPreviewLines(splitTestWidth), "\n"))

	for _, want := range []string{
		workflowPreviewStepsTitle,
		workflow.StatusIcon(workflow.StatusSuccess) + " " + loginStepID,
		workflow.StatusIcon(workflow.StatusFailed) + " " + listStepID,
	} {
		if !strings.Contains(preview, want) {
			t.Errorf("expected preview to contain %q:\n%s", want, preview)
		}
	}
}

func TestWorkflowPreview_ShowsPendingStepsForWorkflowThatDidNotRun(t *testing.T) {
	m := workflowsListTestModel()
	m.workflowCursor = 1

	preview := ansi.Strip(strings.Join(m.buildWorkflowPreviewLines(splitTestWidth), "\n"))

	if !strings.Contains(preview, workflow.StatusIcon(workflow.StatusPending)+" "+otherWorkflowID+"-step-0") {
		t.Errorf("expected pending steps for a workflow that has not run:\n%s", preview)
	}
}

func TestWorkflowPreview_ScrollKeysAreCappedAndCursorMoveResetsScroll(t *testing.T) {
	m := workflowsListTestModel()
	m.workflowCursor = 1
	m.workflowPreviewOpen = true
	maxScroll := m.maxSidePanelScroll(m.buildWorkflowPreviewLines)
	if maxScroll == 0 {
		t.Fatal("expected the preview of a long workflow to scroll")
	}

	for range maxScroll + 1 {
		m, _ = pressKey(m, previewDown)
	}
	if m.workflowPreviewScroll != maxScroll {
		t.Errorf("scroll = %d, want capped at %d", m.workflowPreviewScroll, maxScroll)
	}

	m, _ = pressKey(m, previewUp)
	if m.workflowPreviewScroll != maxScroll-1 {
		t.Errorf("scroll = %d, want %d after scrolling up", m.workflowPreviewScroll, maxScroll-1)
	}

	m.workflowCursor = 0
	m, _ = pressKey(m, cursorDownKey)
	if m.workflowPreviewScroll != 0 {
		t.Errorf("scroll = %d, want reset after moving the cursor", m.workflowPreviewScroll)
	}
}

func TestWorkflowRunIndicator_ShowsRunningOnlyForTheRunningWorkflow(t *testing.T) {
	m := workflowsListTestModel()
	m.workflowChan = make(chan WorkflowMsg)

	if indicator := ansi.Strip(m.workflowRunIndicator(m.workflows[1])); strings.Contains(indicator, workflow.StatusRunning) {
		t.Errorf("indicator = %q, want no running marker for a workflow that is not running", indicator)
	}
	if indicator := ansi.Strip(m.workflowRunIndicator(m.workflows[0])); !strings.Contains(indicator, workflow.StatusRunning) {
		t.Errorf("indicator = %q, want the running marker", indicator)
	}
}
