package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"postOffice/internal/postman"
	"postOffice/internal/workflow"
)

const stagingEnvironment = "Big tusks staging"

// modelWithEnvironments returns a test model whose parser has the given environments loaded.
func modelWithEnvironments(t *testing.T, names ...string) Model {
	t.Helper()
	m := createTestModel()
	dir := t.TempDir()
	for _, name := range names {
		path := filepath.Join(dir, strings.ReplaceAll(name, " ", "_")+".json")
		content := `{"name":"` + name + `","values":[{"key":"base","value":"` + name + `","enabled":true}]}`
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := m.parser.LoadEnvironment(path); err != nil {
			t.Fatalf("failed to load environment %q: %v", name, err)
		}
	}
	return m
}

func writeRunDataFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestApplyRunOption(t *testing.T) {
	m := modelWithEnvironments(t, stagingEnvironment)
	dataFile := writeRunDataFile(t, "rows.csv", "user\nalice\nbob\n")

	successes := []struct {
		arg  string
		want runSettings
	}{
		{"stop on", runSettings{stopOnFailure: true}},
		{"iterations 3", runSettings{iterations: 3}},
		{"iterations 0", runSettings{}},
		{"env " + stagingEnvironment, runSettings{environmentName: stagingEnvironment}},
		{"env active", runSettings{}},
		{"data " + dataFile, runSettings{dataPath: dataFile}},
		{"data off", runSettings{}},
		{"reset", runSettings{}},
	}
	for _, tc := range successes {
		got, err := m.applyRunOption(tc.arg)
		if err != nil || got != tc.want {
			t.Errorf("%q: got %+v, %v; want %+v", tc.arg, got, err, tc.want)
		}
	}

	failures := []struct {
		arg  string
		want error
	}{
		{"stop maybe", errInvalidRunValue},
		{"iterations -1", errInvalidRunValue},
		{"iterations many", errInvalidRunValue},
		{"env", errInvalidRunValue},
		{"env Production", errUnknownRunEnvName},
		{"data", errInvalidRunValue},
		{"data " + writeRunDataFile(t, "rows.txt", "x"), workflow.ErrUnsupportedDataFormat},
		{"colour blue", errUnknownRunOption},
	}
	for _, tc := range failures {
		if _, err := m.applyRunOption(tc.arg); !errors.Is(err, tc.want) {
			t.Errorf("%q: expected %v, got %v", tc.arg, tc.want, err)
		}
	}
}

func TestHandleRunOptsCommand_AppliesAndReports(t *testing.T) {
	m := modelWithEnvironments(t, stagingEnvironment)

	m, _ = handleRunOptsCommand(m, []string{"iterations 2"})
	m, _ = handleRunOptsCommand(m, []string{"stop on"})

	if m.runSettings != (runSettings{iterations: 2, stopOnFailure: true}) {
		t.Errorf("unexpected settings %+v", m.runSettings)
	}
	if m.statusMessage != "Run options: 2 iterations · stop on failure" {
		t.Errorf("unexpected status %q", m.statusMessage)
	}

	m, _ = handleRunOptsCommand(m, []string{"bogus"})
	if !m.statusIsError || !strings.Contains(m.statusMessage, runOptsUsage) {
		t.Errorf("expected an error with usage, got %q", m.statusMessage)
	}
	if m.runSettings != (runSettings{iterations: 2, stopOnFailure: true}) {
		t.Error("expected a bad option to leave the settings unchanged")
	}

	m, _ = handleRunOptsCommand(m, nil)
	if !strings.HasPrefix(m.statusMessage, "Run options: 2 iterations") {
		t.Errorf("expected :runopts alone to show the settings, got %q", m.statusMessage)
	}
}

func TestRunConfig(t *testing.T) {
	m := modelWithEnvironments(t, stagingEnvironment)
	active, _ := m.parser.GetEnvironment(stagingEnvironment)
	m.environment = &postman.Environment{Name: "active"}

	t.Run("defaults use the active environment", func(t *testing.T) {
		environment, opts, err := m.runConfig()
		if err != nil || environment != m.environment || opts.IterationCount() != 1 {
			t.Errorf("got %v, %+v, %v", environment, opts, err)
		}
	})

	t.Run("selected environment and data file", func(t *testing.T) {
		selected := m
		selected.runSettings = runSettings{
			environmentName: stagingEnvironment,
			dataPath:        writeRunDataFile(t, "rows.json", `[{"user":"a"},{"user":"b"},{"user":"c"}]`),
			stopOnFailure:   true,
		}
		environment, opts, err := selected.runConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if environment != active {
			t.Errorf("expected the selected environment, got %v", environment)
		}
		if opts.IterationCount() != 3 || !opts.StopOnFailure {
			t.Errorf("unexpected options %+v", opts)
		}
		if m.environment.Name != "active" {
			t.Error("expected the active environment to stay unchanged")
		}
	})

	t.Run("environment unloaded since it was chosen", func(t *testing.T) {
		gone := m
		gone.runSettings = runSettings{environmentName: "Unloaded"}
		if _, _, err := gone.runConfig(); !errors.Is(err, errUnknownRunEnvName) {
			t.Errorf("expected errUnknownRunEnvName, got %v", err)
		}
	})
}

func TestStartWorkflow_RefusesWhenRunConfigFails(t *testing.T) {
	m := createTestModel()
	m.runSettings = runSettings{dataPath: filepath.Join(t.TempDir(), "deleted.csv")}

	m, cmd := m.startWorkflow(&workflow.Workflow{ID: "wf", Name: "wf"})

	if cmd != nil || m.workflowChan != nil {
		t.Error("expected no run to start")
	}
	if !m.statusIsError || !strings.Contains(m.statusMessage, "Cannot run workflow wf") {
		t.Errorf("unexpected status %q", m.statusMessage)
	}
}

func TestFolderRunPrompt_ShowsNonDefaultOptions(t *testing.T) {
	path := []string{"policies", "TEST"}

	if prompt := folderRunPrompt(path, runSettings{}); strings.Contains(prompt, "[") {
		t.Errorf("expected no options in the default prompt, got %q", prompt)
	}
	prompt := folderRunPrompt(path, runSettings{iterations: 2, stopOnFailure: true, dataPath: "/tmp/users.csv"})
	if !strings.HasSuffix(prompt, "[2 iterations · stop on failure · data: users.csv]") {
		t.Errorf("unexpected prompt %q", prompt)
	}
}

func TestRunReportLines_IterationsAndSummary(t *testing.T) {
	state := workflow.ExecutionState{
		WorkflowName: "policies/TEST",
		Status:       workflow.StatusFailed,
		Iteration:    2,
		Iterations:   2,
		Steps: []*workflow.StepState{
			{ID: "login", Status: workflow.StatusSuccess, Iteration: 1},
			{ID: "login", Status: workflow.StatusFailed, Iteration: 2},
		},
		Summary: workflow.RunSummary{StepsPassed: 1, StepsFailed: 1, TestsPassed: 3, TestsFailed: 1},
	}

	report := strings.Join(runReportLines(state, plainRunStyler), "\n")

	for _, want := range []string{
		"policies/TEST [failed · iteration 2/2]",
		"steps: 1 passed, 1 failed, 0 skipped · tests: 3 passed, 1 failed",
		"✓ #1 login",
		"✗ #2 login",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("expected %q in report:\n%s", want, report)
		}
	}

	single := strings.Join(runReportLines(workflow.ExecutionState{WorkflowName: "x", Status: workflow.StatusRunning, Iterations: 1,
		Steps: []*workflow.StepState{{ID: "login", Status: workflow.StatusRunning, Iteration: 1}}}, plainRunStyler), "\n")
	if strings.Contains(single, "#1") || strings.Contains(single, "steps:") {
		t.Errorf("expected no iteration prefix or summary for a running single pass:\n%s", single)
	}
}
