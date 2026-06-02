package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadWorkflow_valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "login.yaml")
	content := `
id: login-flow
name: Login Flow
description: Test login
version: 1
steps:
  - id: login
    request: Auth/Login
script: |
  export default async function(wf, pm) {}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	wf, err := LoadWorkflow(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if wf.ID != "login-flow" {
		t.Errorf("ID = %q, want %q", wf.ID, "login-flow")
	}
	if wf.Name != "Login Flow" {
		t.Errorf("Name = %q, want %q", wf.Name, "Login Flow")
	}
	if len(wf.Steps) != 1 {
		t.Fatalf("Steps length = %d, want 1", len(wf.Steps))
	}
	if wf.Steps[0].ID != "login" {
		t.Errorf("Steps[0].ID = %q, want %q", wf.Steps[0].ID, "login")
	}
	if wf.Steps[0].Request != "Auth/Login" {
		t.Errorf("Steps[0].Request = %q, want %q", wf.Steps[0].Request, "Auth/Login")
	}
}

func TestLoadWorkflow_missingID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("name: No ID Workflow\nsteps: []\nscript: |\n  export default async function(wf,pm){}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadWorkflow(path)
	if err == nil {
		t.Fatal("expected error for missing id, got nil")
	}
}

func TestLoadWorkflow_fileNotFound(t *testing.T) {
	_, err := LoadWorkflow("/nonexistent/path/workflow.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestDiscoverWorkflows_emptyDir(t *testing.T) {
	dir := t.TempDir()
	workflows, err := DiscoverWorkflows(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(workflows) != 0 {
		t.Errorf("expected 0 workflows, got %d", len(workflows))
	}
}

func TestDiscoverWorkflows_nonexistentDir(t *testing.T) {
	workflows, err := DiscoverWorkflows("/nonexistent/workflows")
	if err != nil {
		t.Fatalf("expected nil error for missing dir, got: %v", err)
	}
	if workflows != nil {
		t.Errorf("expected nil slice for missing dir, got %v", workflows)
	}
}

func TestDiscoverWorkflows_multipleFiles(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"alpha.yaml": "id: alpha\nname: Alpha\nsteps: []\nscript: |\n  export default async function(wf,pm){}\n",
		"beta.yml":   "id: beta\nname: Beta\nsteps: []\nscript: |\n  export default async function(wf,pm){}\n",
		"readme.txt": "not a workflow",
		"bad.yaml":   "name: Missing ID\nsteps: []\n",
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	workflows, err := DiscoverWorkflows(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(workflows) != 2 {
		t.Errorf("expected 2 workflows (skipping txt and bad.yaml), got %d", len(workflows))
	}
}

func TestWorkflowsDir(t *testing.T) {
	got := WorkflowsDir("/home/user/collections/my.json")
	want := "/home/user/collections/workflows"
	if got != want {
		t.Errorf("WorkflowsDir = %q, want %q", got, want)
	}
}

func TestWorkflowPath(t *testing.T) {
	got := WorkflowPath("/home/user/collections/my.json", "login-flow")
	want := "/home/user/collections/workflows/login-flow.yaml"
	if got != want {
		t.Errorf("WorkflowPath = %q, want %q", got, want)
	}
}

func TestNewWorkflow(t *testing.T) {
	wf := NewWorkflow("login-and-fetch-users")
	if wf.ID != "login-and-fetch-users" {
		t.Errorf("ID = %q, want %q", wf.ID, "login-and-fetch-users")
	}
	if wf.Name != "Login And Fetch Users" {
		t.Errorf("Name = %q, want %q", wf.Name, "Login And Fetch Users")
	}
	if wf.Script == "" {
		t.Error("Script should not be empty")
	}
	if wf.Steps == nil {
		t.Error("Steps should be initialized (not nil)")
	}
}

func TestPartialScript_singleStep(t *testing.T) {
	steps := []Step{
		{ID: "login", Request: "Auth/Login"},
		{ID: "get-user", Request: "Users/Get User"},
		{ID: "list-users", Request: "Users/List"},
	}

	got := PartialScript(steps, 1, 1)
	if !strings.Contains(got, `wf.run("get-user")`) {
		t.Errorf("expected get-user in script, got:\n%s", got)
	}
	if strings.Contains(got, `wf.run("login")`) {
		t.Errorf("login should not appear in single-step script, got:\n%s", got)
	}
	if strings.Contains(got, `wf.run("list-users")`) {
		t.Errorf("list-users should not appear in single-step script, got:\n%s", got)
	}
}

func TestPartialScript_range(t *testing.T) {
	steps := []Step{
		{ID: "a", Request: "Folder/A"},
		{ID: "b", Request: "Folder/B"},
		{ID: "c", Request: "Folder/C"},
		{ID: "d", Request: "Folder/D"},
	}

	got := PartialScript(steps, 1, 2)
	if !strings.Contains(got, `wf.run("b")`) {
		t.Errorf("b missing: %s", got)
	}
	if !strings.Contains(got, `wf.run("c")`) {
		t.Errorf("c missing: %s", got)
	}
	if strings.Contains(got, `wf.run("a")`) {
		t.Errorf("a should be excluded: %s", got)
	}
	if strings.Contains(got, `wf.run("d")`) {
		t.Errorf("d should be excluded: %s", got)
	}
}

func TestPartialScript_allSteps(t *testing.T) {
	steps := []Step{
		{ID: "step1", Request: "F/S1"},
		{ID: "step2", Request: "F/S2"},
	}
	got := PartialScript(steps, 0, len(steps)-1)
	if !strings.Contains(got, `wf.run("step1")`) || !strings.Contains(got, `wf.run("step2")`) {
		t.Errorf("both steps should be present: %s", got)
	}
}

func TestPartialScript_isValidWorkflowScript(t *testing.T) {
	steps := []Step{{ID: "login", Request: "Auth/Login"}}
	got := PartialScript(steps, 0, 0)
	if !strings.HasPrefix(strings.TrimSpace(got), "export default async function") {
		t.Errorf("script should start with export default async function, got: %s", got)
	}
}

func TestSaveWorkflow_roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workflows", "my-flow.yaml")

	original := &Workflow{
		ID:          "my-flow",
		Name:        "My Flow",
		Description: "Round-trip test",
		Version:     2,
		Steps: []Step{
			{ID: "step1", Request: "Folder/Request"},
		},
		Script: "export default async function(wf, pm) {}\n",
	}

	if err := SaveWorkflow(original, path); err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}

	loaded, err := LoadWorkflow(path)
	if err != nil {
		t.Fatalf("LoadWorkflow: %v", err)
	}

	if loaded.ID != original.ID {
		t.Errorf("ID = %q, want %q", loaded.ID, original.ID)
	}
	if loaded.Name != original.Name {
		t.Errorf("Name = %q, want %q", loaded.Name, original.Name)
	}
	if loaded.Version != original.Version {
		t.Errorf("Version = %d, want %d", loaded.Version, original.Version)
	}
	if len(loaded.Steps) != 1 || loaded.Steps[0].Request != "Folder/Request" {
		t.Errorf("Steps not preserved correctly: %+v", loaded.Steps)
	}
}
