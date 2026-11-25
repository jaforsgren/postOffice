package tui

import (
	"slices"
	"strings"
	"testing"

	"postOffice/internal/http"
	"postOffice/internal/workflow"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	panelTestWorkflowID = "panel-wf"
	loginStepID         = "login"
	listStepID          = "list"
	loginPassLog        = workflow.LogPassMarker + " logged in"
	listFailLog         = workflow.LogFailMarker + " has users: expected 0 to be above 0"
	workflowScriptLog   = "finished"
	loginRequestURL     = "https://example.com/login"
	loginResponseBody   = `{"token":"abc"}`
	manyLogLines        = 200
)

func panelTestState() workflow.ExecutionState {
	login := &workflow.StepState{
		ID: loginStepID, Request: "Auth/Login", Status: workflow.StatusSuccess,
		LastResponse: &http.Response{
			StatusCode: 200, Status: "200 OK", Body: loginResponseBody,
			RequestMethod: "POST", RequestURL: loginRequestURL,
		},
	}
	list := &workflow.StepState{ID: listStepID, Request: "Users/List", Status: workflow.StatusFailed}
	return workflow.ExecutionState{
		WorkflowID:   panelTestWorkflowID,
		WorkflowName: "Panel",
		Status:       workflow.StatusFailed,
		Steps:        []*workflow.StepState{login, list},
		StepIndex:    map[string]*workflow.StepState{loginStepID: login, listStepID: list},
		Logs: []workflow.LogEntry{
			{StepID: loginStepID, Message: loginPassLog},
			{StepID: listStepID, Message: listFailLog},
			{StepID: "", Message: workflowScriptLog},
		},
	}
}

func panelTestModel() Model {
	m := createTestModel()
	m.width = splitTestWidth
	m.height = splitTestHeight
	m.mode = ModeWorkflowDetail
	m.activeWorkflow = &workflow.Workflow{
		ID:   panelTestWorkflowID,
		Name: "Panel",
		Steps: []workflow.Step{
			{ID: loginStepID, Request: "Auth/Login"},
			{ID: listStepID, Request: "Users/List"},
		},
	}
	m.workflowState = panelTestState()
	return m
}

func plainPanelLines(m Model) []string {
	var lines []string
	for _, line := range m.buildStepPanelLines(splitTestWidth) {
		lines = append(lines, strings.TrimRight(ansi.Strip(line), " "))
	}
	return lines
}

func TestRunReportLines_GroupsLogsUnderTheirStep(t *testing.T) {
	lines := runReportLines(panelTestState(), plainRunStyler)

	loginLine := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, loginStepID+" ") })
	listLine := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, listStepID+" ") })
	if loginLine < 0 || listLine < 0 {
		t.Fatalf("step lines missing from report:\n%s", strings.Join(lines, "\n"))
	}
	if lines[loginLine+1] != runReportLogIndent+loginPassLog {
		t.Errorf("line after login step = %q, want its log", lines[loginLine+1])
	}
	if lines[listLine+1] != runReportLogIndent+listFailLog {
		t.Errorf("line after list step = %q, want its log", lines[listLine+1])
	}

	workflowHeading := slices.Index(lines, "Workflow logs:")
	if workflowHeading < listLine {
		t.Fatalf("workflow logs should follow the steps:\n%s", strings.Join(lines, "\n"))
	}
	if lines[workflowHeading+1] != runReportStepIndent+workflowScriptLog {
		t.Errorf("workflow log line = %q, want %q", lines[workflowHeading+1], workflowScriptLog)
	}
}

func TestGroupLogsByStep_UnlistedStepLogsGoToWorkflowLogs(t *testing.T) {
	state := panelTestState()
	state.Logs = append(state.Logs, workflow.LogEntry{StepID: "unlisted", Message: "orphan"})

	stepLogs, workflowLogs := groupLogsByStep(state)

	if !slices.Equal(stepLogs[stepRunKey{stepID: loginStepID}], []string{loginPassLog}) {
		t.Errorf("login logs = %v", stepLogs[stepRunKey{stepID: loginStepID}])
	}
	if !slices.Equal(workflowLogs, []string{workflowScriptLog, "orphan"}) {
		t.Errorf("workflow logs = %v", workflowLogs)
	}
}

func TestGroupLogsByStep_KeepsIterationsApart(t *testing.T) {
	first := &workflow.StepState{ID: loginStepID, Status: workflow.StatusSuccess, Iteration: 1}
	second := &workflow.StepState{ID: loginStepID, Status: workflow.StatusFailed, Iteration: 2}
	state := workflow.ExecutionState{
		Iteration:  2,
		Iterations: 2,
		Steps:      []*workflow.StepState{first, second},
		StepIndex:  map[string]*workflow.StepState{loginStepID: second},
		Logs: []workflow.LogEntry{
			{StepID: loginStepID, Iteration: 1, Message: loginPassLog},
			{StepID: loginStepID, Iteration: 2, Message: listFailLog},
		},
	}

	stepLogs, _ := groupLogsByStep(state)

	if !slices.Equal(stepLogs[stepRunKeyOf(first)], []string{loginPassLog}) || !slices.Equal(stepLogs[stepRunKeyOf(second)], []string{listFailLog}) {
		t.Errorf("expected each iteration's logs under its own step run, got %v", stepLogs)
	}

	report := strings.Join(runReportLines(state, plainRunStyler), "\n")
	firstAt, secondAt := strings.Index(report, "#1 "+loginStepID), strings.Index(report, "#2 "+loginStepID)
	if firstAt < 0 || secondAt < firstAt || !strings.Contains(report[firstAt:secondAt], loginPassLog) || !strings.Contains(report[secondAt:], listFailLog) {
		t.Errorf("expected each iteration's logs folded under its step line:\n%s", report)
	}
}

func TestStepPanel_TabTogglesInWorkflowDetail(t *testing.T) {
	m := panelTestModel()

	m, _ = pressKey(m, tea.KeyMsg{Type: tea.KeyTab})
	if !m.workflowStepPanelOpen {
		t.Fatal("expected tab to open the step panel")
	}

	m, _ = pressKey(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.workflowStepPanelOpen {
		t.Fatal("expected a second tab to close the step panel")
	}
}

func TestStepPanel_EscClosesPanelBeforeLeavingDetail(t *testing.T) {
	m := panelTestModel()
	m.workflowStepPanelOpen = true

	m, _ = pressKey(m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.workflowStepPanelOpen {
		t.Error("expected esc to close the step panel")
	}
	if m.mode != ModeWorkflowDetail {
		t.Errorf("mode = %v, want to stay in workflow detail", m.mode)
	}
}

func TestStepPanel_ShowsSelectedStepLogsRequestAndResponse(t *testing.T) {
	m := panelTestModel()

	lines := plainPanelLines(m)

	for _, want := range []string{loginPassLog, "POST " + loginRequestURL, "Status: 200 OK"} {
		if !slices.Contains(lines, want) {
			t.Errorf("panel missing %q:\n%s", want, strings.Join(lines, "\n"))
		}
	}
	if slices.Contains(lines, listFailLog) {
		t.Error("panel shows another step's log")
	}
}

func TestStepPanel_FollowsCursorAndExplainsMissingResponse(t *testing.T) {
	m := panelTestModel()
	m.workflowStepPanelOpen = true

	m, _ = pressKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	lines := plainPanelLines(m)

	if !slices.Contains(lines, listFailLog) {
		t.Errorf("panel should show the list step's log:\n%s", strings.Join(lines, "\n"))
	}
	if !slices.Contains(lines, stepPanelNoResponseMessage) {
		t.Errorf("panel should explain the step sent no request:\n%s", strings.Join(lines, "\n"))
	}
}

func TestStepPanel_StepNotRunYet(t *testing.T) {
	m := panelTestModel()
	m.workflowState = workflow.ExecutionState{}

	if !slices.Contains(plainPanelLines(m), stepPanelNotRunMessage) {
		t.Error("expected the not-run message before the workflow has run")
	}
}

func TestStepPanel_ScrollStopsAtLastPage(t *testing.T) {
	m := panelTestModel()
	m.workflowStepPanelOpen = true
	for range manyLogLines {
		m.workflowState.Logs = append(m.workflowState.Logs, workflow.LogEntry{StepID: loginStepID, Message: "line"})
	}
	maxScroll := m.maxSidePanelScroll(m.buildStepPanelLines)
	if maxScroll == 0 {
		t.Fatal("expected the panel to overflow")
	}

	for range maxScroll + 5 {
		m, _ = pressKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(stepPanelDownKey)})
	}
	if m.workflowStepPanelScroll != maxScroll {
		t.Errorf("scroll = %d, want it to stop at %d", m.workflowStepPanelScroll, maxScroll)
	}

	m, _ = pressKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(stepPanelUpKey)})
	if m.workflowStepPanelScroll != maxScroll-1 {
		t.Errorf("scroll after [ = %d, want %d", m.workflowStepPanelScroll, maxScroll-1)
	}

	m, _ = pressKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.workflowStepPanelScroll != 0 {
		t.Errorf("moving to another step should reset the scroll, got %d", m.workflowStepPanelScroll)
	}
}

func TestRenderWorkflowDetailWithStepPanel_FitsMainWindow(t *testing.T) {
	m := panelTestModel()
	m.workflowStepPanelOpen = true

	split := m.renderWorkflowDetailWithStepPanel()
	single := m.renderWorkflowDetail()

	if lipgloss.Width(split) != lipgloss.Width(single) || lipgloss.Height(split) != lipgloss.Height(single) {
		t.Errorf("split %dx%d, want %dx%d like the single pane",
			lipgloss.Width(split), lipgloss.Height(split), lipgloss.Width(single), lipgloss.Height(single))
	}
}

func TestShowWorkflowStepInfo_ListsTheStepRunLogs(t *testing.T) {
	m := panelTestModel()
	m.activeWorkflow.Steps[0].Request = formRequestName
	m.workflowStepCursor = 0

	m, _ = m.showWorkflowStepInfo()

	if m.mode != ModeInfo {
		t.Fatalf("expected the info view, got mode %v (%s)", m.mode, m.statusMessage)
	}
	info := ansi.Strip(m.infoViewport.View())
	if !strings.Contains(info, formRequestName) || !strings.Contains(info, "LOGS") || !strings.Contains(info, loginPassLog) {
		t.Errorf("expected the request info followed by the step's logs, got:\n%s", info)
	}
	if strings.Contains(info, listFailLog) || strings.Contains(info, workflowScriptLog) {
		t.Errorf("expected only this step's logs, got:\n%s", info)
	}
}

func TestShowWorkflowStepInfo_ShowsTheStepRunResponse(t *testing.T) {
	m := panelTestModel()
	m.activeWorkflow.Steps[0].Request = formRequestName
	m.workflowStepCursor = 0
	m.workflowState.StepIndex[loginStepID].LastResponse.Headers = map[string][]string{"X-Trace": {"t-1"}}

	m, _ = m.showWorkflowStepInfo()

	m.infoViewport.Height = manyLogLines
	info := ansi.Strip(m.infoViewport.View())
	for _, want := range []string{"RESPONSE", "Status: 200 OK", "X-Trace: t-1", "\"token\": \"abc\""} {
		if !strings.Contains(info, want) {
			t.Errorf("info view lacks %q:\n%s", want, info)
		}
	}
}

func TestShowWorkflowStepInfo_NotRunStepHasNoLogsSection(t *testing.T) {
	m := panelTestModel()
	m.activeWorkflow.Steps[0].Request = formRequestName
	m.workflowState = workflow.ExecutionState{}

	m, _ = m.showWorkflowStepInfo()

	if info := ansi.Strip(m.infoViewport.View()); strings.Contains(info, "LOGS") {
		t.Errorf("expected no logs section before the step has run, got:\n%s", info)
	}
}

func TestHandleEnterKey_FolderStepOpensItsInfoWithTheFailure(t *testing.T) {
	const folderStepID = "folder"
	const folderErrorLog = "[ERROR] cannot resolve folder \"Test Folder\": folder contains no requests"
	m := panelTestModel()
	m.activeWorkflow.Steps = []workflow.Step{{ID: folderStepID, Folder: "Test Folder"}}
	folder := &workflow.StepState{ID: folderStepID, Folder: "Test Folder", Status: workflow.StatusFailed}
	m.workflowState.Steps = []*workflow.StepState{folder}
	m.workflowState.StepIndex = map[string]*workflow.StepState{folderStepID: folder}
	m.workflowState.Logs = []workflow.LogEntry{{StepID: folderStepID, Message: folderErrorLog}}
	m.workflowStepCursor = 0

	m, _ = handleEnterKey(m)

	if m.mode != ModeInfo {
		t.Fatalf("expected the info view, got mode %v (%s)", m.mode, m.statusMessage)
	}
	m.infoViewport.Height = manyLogLines
	if info := ansi.Strip(m.infoViewport.View()); !strings.Contains(info, folderErrorLog) {
		t.Errorf("expected the folder step's failure in its info, got:\n%s", info)
	}
}

func TestHandleEnterKey_RequestStepOpensItsResponse(t *testing.T) {
	m := panelTestModel()
	m.workflowStepCursor = 0

	m, _ = handleEnterKey(m)

	if m.mode != ModeResponse {
		t.Fatalf("expected the response view, got mode %v (%s)", m.mode, m.statusMessage)
	}
	if m.lastResponse == nil || m.lastResponse.Body != loginResponseBody {
		t.Errorf("expected the login step's response, got %+v", m.lastResponse)
	}
}

func TestWorkflowStepInfoReport_HasTheStepRunLogsAndResponse(t *testing.T) {
	m := panelTestModel()
	m.activeWorkflow.Steps[0].Request = formRequestName
	m.workflowStepCursor = 0
	m, _ = m.showWorkflowStepInfo()

	target, err := m.currentYankTarget()
	if err != nil {
		t.Fatal(err)
	}
	report, err := target.markdownReport()
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"# " + formRequestName,
		"## Logs\n\n```text\n" + loginPassLog + "\n```",
		"POST " + loginRequestURL,
		"\"token\": \"abc\"",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("expected the report to contain %q, got:\n%s", want, report)
		}
	}
	if strings.Contains(report, listFailLog) || strings.Contains(report, workflowScriptLog) {
		t.Errorf("expected only this step's logs, got:\n%s", report)
	}
}
