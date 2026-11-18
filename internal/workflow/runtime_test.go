package workflow

import (
	"testing"

	"postOffice/internal/script"
)

func TestTransformScript_stripsExportDefault(t *testing.T) {
	src := `export default async function (wf, pm) {
  await wf.run("login");
}`
	got := transformScript(src)
	want := `(async function (wf, pm) {
  await wf.run("login");
})(__wf__, pm);`
	if got != want {
		t.Errorf("transformScript =\n%s\nwant:\n%s", got, want)
	}
}

func TestTransformScript_noExportDefault(t *testing.T) {
	src := `async function (wf, pm) {}`
	got := transformScript(src)
	want := "(async function (wf, pm) {})(__wf__, pm);"
	if got != want {
		t.Errorf("transformScript = %q, want %q", got, want)
	}
}

func TestTransformScript_trimsWhitespace(t *testing.T) {
	src := "  export default async function(wf, pm) {}  "
	got := transformScript(src)
	want := "(async function(wf, pm) {})(__wf__, pm);"
	if got != want {
		t.Errorf("transformScript = %q, want %q", got, want)
	}
}

func TestStepResponse_Text(t *testing.T) {
	r := &StepResponse{Body: "hello"}
	if r.Text() != "hello" {
		t.Errorf("Text() = %q, want %q", r.Text(), "hello")
	}
}

func TestStepResponse_JSON_valid(t *testing.T) {
	r := &StepResponse{Body: `{"id":42}`}
	val, err := r.JSON()
	if err != nil {
		t.Fatalf("JSON() error: %v", err)
	}
	m, ok := val.(map[string]any)
	if !ok {
		t.Fatalf("JSON() returned %T, want map", val)
	}
	id, ok := m["id"].(float64)
	if !ok || id != 42 {
		t.Errorf("id = %v, want 42", m["id"])
	}
}

func TestStepResponse_JSON_invalid(t *testing.T) {
	r := &StepResponse{Body: "not json"}
	_, err := r.JSON()
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func twoStepWorkflow() *Workflow {
	return &Workflow{
		ID:   "test-wf",
		Name: "Test Workflow",
		Steps: []Step{
			{ID: "step1", Request: "Folder/Req1"},
			{ID: "step2", Request: "Folder/Req2"},
		},
	}
}

// runnerAtIteration prepares a runner as Run does before its first iteration.
func runnerAtIteration(wf *Workflow, opts ExecutionOptions) *Runner {
	runner := &Runner{frame: &frame{steps: wf.Steps}, options: opts, ctx: &script.ExecutionContext{}}
	runner.state = ExecutionState{
		WorkflowID: wf.ID,
		Status:     StatusRunning,
		StepIndex:  make(map[string]*StepState),
		Iterations: opts.IterationCount(),
	}
	runner.beginIteration(0)
	return runner
}

func TestBeginIteration_addsPendingStepsForTheIteration(t *testing.T) {
	runner := runnerAtIteration(twoStepWorkflow(), ExecutionOptions{Iterations: 2})

	if len(runner.state.Steps) != 2 {
		t.Fatalf("Steps length = %d, want 2", len(runner.state.Steps))
	}
	if runner.state.Steps[0].Status != StatusPending || runner.state.Steps[0].Iteration != 1 {
		t.Errorf("Steps[0] = %+v, want pending in iteration 1", runner.state.Steps[0])
	}
	if runner.state.StepIndex["step1"] == nil || runner.state.StepIndex["step2"] == nil {
		t.Error("StepIndex missing a step")
	}

	runner.beginIteration(1)

	if len(runner.state.Steps) != 4 {
		t.Fatalf("Steps length = %d after the second iteration, want 4", len(runner.state.Steps))
	}
	if got := runner.state.StepIndex["step1"]; got != runner.state.Steps[2] || got.Iteration != 2 {
		t.Errorf("StepIndex should point at the latest iteration's step, got %+v", got)
	}
	if runner.ctx.Iteration.Index != 1 || runner.ctx.Iteration.Count != 2 {
		t.Errorf("script iteration = %+v, want index 1 of 2", runner.ctx.Iteration)
	}
}

func TestFinalize_marksRemainingStepsSkippedAndSummarizes(t *testing.T) {
	runner := runnerAtIteration(twoStepWorkflow(), ExecutionOptions{})
	runner.state.Steps[0].Status = StatusSuccess
	runner.countTests(&script.TestResult{Tests: []script.Test{{Passed: true}, {Passed: false}}})

	final := runner.finalize(StatusFailed, "something went wrong")

	if final.Status != StatusFailed {
		t.Errorf("Status = %q, want %q", final.Status, StatusFailed)
	}
	if final.Error != "something went wrong" {
		t.Errorf("Error = %q, want %q", final.Error, "something went wrong")
	}
	if runner.state.Steps[0].Status != StatusSuccess {
		t.Errorf("step1 should remain %q, got %q", StatusSuccess, runner.state.Steps[0].Status)
	}
	if runner.state.Steps[1].Status != StatusSkipped {
		t.Errorf("step2 should be %q, got %q", StatusSkipped, runner.state.Steps[1].Status)
	}
	want := RunSummary{StepsPassed: 1, StepsSkipped: 1, TestsPassed: 1, TestsFailed: 1}
	if final.Summary != want {
		t.Errorf("Summary = %+v, want %+v", final.Summary, want)
	}
}

func TestResolveRequest_found(t *testing.T) {
	col := buildTestCollection()
	runner := &Runner{collection: col}

	item, err := runner.resolveRequest("Auth/Login")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Name != "Login" {
		t.Errorf("Name = %q, want %q", item.Name, "Login")
	}
}

func TestResolveRequest_notFound(t *testing.T) {
	col := buildTestCollection()
	runner := &Runner{collection: col}

	_, err := runner.resolveRequest("Auth/NonExistent")
	if err == nil {
		t.Error("expected error for missing request, got nil")
	}
}

func TestResolveRequest_noCollection(t *testing.T) {
	runner := &Runner{}
	_, err := runner.resolveRequest("Auth/Login")
	if err == nil {
		t.Error("expected error when no collection loaded, got nil")
	}
}

func TestFindStep(t *testing.T) {
	runner := &Runner{frame: &frame{steps: []Step{
		{ID: "login", Request: "Auth/Login"},
		{ID: "admin", Folder: "Auth/Admin"},
	}}}

	if got := runner.findStep("admin"); got == nil || got.Folder != "Auth/Admin" {
		t.Errorf("findStep(admin) = %+v, want the Auth/Admin folder step", got)
	}
	if got := runner.findStep("unknown"); got != nil {
		t.Errorf("findStep(unknown) = %+v, want nil", got)
	}
}
