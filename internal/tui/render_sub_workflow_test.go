package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"postOffice/internal/workflow"
)

func subWorkflowRunState() workflow.ExecutionState {
	return workflow.ExecutionState{
		WorkflowName: "Runner",
		Status:       workflow.StatusSuccess,
		Steps: []*workflow.StepState{
			{ID: "top", Request: "top", Status: workflow.StatusSuccess, Iteration: 1},
			{ID: "outer", Folder: "outer", Status: workflow.StatusSuccess, Iteration: 1},
			{ID: "outer/b", Request: "outer/b", Groups: []string{"outer"}, Status: workflow.StatusSuccess, Iteration: 1},
			{ID: "outer/inner", Folder: "outer/inner", Groups: []string{"outer"}, Status: workflow.StatusSuccess, Iteration: 1},
			{ID: "outer/inner/c", Request: "outer/inner/c", Groups: []string{"outer", "outer/inner"}, Status: workflow.StatusSuccess, Iteration: 1},
			{ID: "other", Folder: "other", Status: workflow.StatusSuccess, Iteration: 1},
		},
		Logs: []workflow.LogEntry{{StepID: "outer/inner/c", Iteration: 1, Message: "inside c"}},
	}
}

// numberedBars renders each bar as its colour index, so tests can see which bar is which.
var numberedBars = runStyler{
	title:    identity,
	heading:  identity,
	status:   func(_, text string) string { return text },
	subtle:   identity,
	failure:  identity,
	groupBar: func(colorIndex int) string { return fmt.Sprintf("[%d]", colorIndex) },
}

func hasLineWithPrefix(lines []string, prefix string) bool {
	return slices.ContainsFunc(lines, func(line string) bool { return strings.HasPrefix(line, prefix) })
}

func TestRunReport_barsMarkEachSubWorkflowInItsOwnColour(t *testing.T) {
	lines := runReportLines(subWorkflowRunState(), numberedBars)

	for _, want := range []string{
		runReportStepIndent + "✓ top ",
		runReportStepIndent + "[0]✓ outer ",
		runReportStepIndent + "[0]✓ b ",
		runReportStepIndent + "[0][1]✓ inner ",
		runReportStepIndent + "[0][1]✓ c ",
		runReportStepIndent + "[0][1]" + runReportLogOffset + "inside c",
		runReportStepIndent + "[2]✓ other ",
	} {
		if !hasLineWithPrefix(lines, want) {
			t.Errorf("no line starts with %q in\n%s", want, strings.Join(lines, "\n"))
		}
	}
	if !strings.Contains(strings.Join(lines, "\n"), folderStepLabel+"outer/inner") {
		t.Error("folder step lines should name the folder they run")
	}
}

func TestRunReport_plainExportIndentsSubWorkflowSteps(t *testing.T) {
	lines := runReportLines(subWorkflowRunState(), plainRunStyler)

	want := runReportStepIndent + strings.Repeat(subWorkflowIndent, 2) + "✓ c "
	if !hasLineWithPrefix(lines, want) {
		t.Errorf("no line starts with %q in\n%s", want, strings.Join(lines, "\n"))
	}
}

func TestStepTargetLabel(t *testing.T) {
	if got := stepTargetLabel(workflow.Step{Request: "Auth/Login"}); got != "Auth/Login" {
		t.Errorf("request step label = %q", got)
	}
	if got := stepTargetLabel(workflow.Step{Folder: "Auth/Admin"}); got != folderStepLabel+"Auth/Admin" {
		t.Errorf("folder step label = %q", got)
	}
}
