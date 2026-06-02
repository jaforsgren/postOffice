package workflow

import (
	"testing"
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

func TestBuildInitialState(t *testing.T) {
	runner := &Runner{}
	wf := &Workflow{
		ID:   "test-wf",
		Name: "Test Workflow",
		Steps: []Step{
			{ID: "step1", Request: "Folder/Req1"},
			{ID: "step2", Request: "Folder/Req2"},
		},
	}

	state := runner.buildInitialState(wf)

	if state.WorkflowID != "test-wf" {
		t.Errorf("WorkflowID = %q, want %q", state.WorkflowID, "test-wf")
	}
	if state.Status != StatusRunning {
		t.Errorf("Status = %q, want %q", state.Status, StatusRunning)
	}
	if len(state.Steps) != 2 {
		t.Fatalf("Steps length = %d, want 2", len(state.Steps))
	}
	if state.Steps[0].Status != StatusPending {
		t.Errorf("Steps[0].Status = %q, want %q", state.Steps[0].Status, StatusPending)
	}
	if state.StepIndex["step1"] == nil {
		t.Error("StepIndex missing step1")
	}
	if state.StepIndex["step2"] == nil {
		t.Error("StepIndex missing step2")
	}
}

func TestFinalize_marksRemainingStepsSkipped(t *testing.T) {
	runner := &Runner{}
	wf := &Workflow{
		ID:   "test-wf",
		Name: "Test Workflow",
		Steps: []Step{
			{ID: "step1", Request: "Folder/Req1"},
			{ID: "step2", Request: "Folder/Req2"},
		},
	}
	runner.state = runner.buildInitialState(wf)
	runner.state.Steps[0].Status = StatusSuccess

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

func TestFindRequestPath(t *testing.T) {
	runner := &Runner{
		wfSteps: []Step{
			{ID: "login", Request: "Auth/Login"},
			{ID: "list-users", Request: "Users/List"},
		},
	}

	if got := runner.findRequestPath("login"); got != "Auth/Login" {
		t.Errorf("findRequestPath(login) = %q, want %q", got, "Auth/Login")
	}
	if got := runner.findRequestPath("unknown"); got != "" {
		t.Errorf("findRequestPath(unknown) = %q, want empty", got)
	}
}
