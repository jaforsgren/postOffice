package main

import (
	"errors"
	"strings"
	"testing"

	"postOffice/internal/postman"
)

func selectedFixture(t *testing.T) fixture {
	t.Helper()
	f := newFixture(t, apiServer(t))
	writeLoadedConfig(t, []string{f.collectionPath}, []string{f.envPath})
	return f
}

func TestSelectionFlagsWithoutACommandRunUse(t *testing.T) {
	f := selectedFixture(t)

	code, out, errOut := cli(t, "-c", "API", "-e", "Local")

	if code != exitSuccess || !strings.Contains(out, f.collectionPath) || !strings.Contains(out, f.envPath) {
		t.Fatalf("code %d, stderr %s, stdout:\n%s", code, errOut, out)
	}
	if got := postman.NewParser().LoadCLISelection(); got.CollectionPath != f.collectionPath || got.EnvironmentPath != f.envPath {
		t.Errorf("expected the selection saved, got %+v", got)
	}
	if isTUIInvocation([]string{"-e", "Local"}) {
		t.Error("expected -e to select, not start the TUI")
	}
}

func TestUse_EmptyValueUnselects(t *testing.T) {
	selectedFixture(t)
	cli(t, "use", "-c", "API", "-e", "Local")

	code, out, _ := cli(t, "use", "-e", "")

	got := postman.NewParser().LoadCLISelection()
	if code != exitSuccess || got.EnvironmentPath != "" || got.CollectionPath == "" {
		t.Errorf("expected only the environment unselected, got %+v (%s)", got, out)
	}
}

func TestUse_Clear(t *testing.T) {
	selectedFixture(t)
	cli(t, "use", "-c", "API", "-e", "Local")

	cli(t, "use", "--clear")

	if got := postman.NewParser().LoadCLISelection(); *got != (postman.CLISelection{}) {
		t.Errorf("expected nothing selected, got %+v", got)
	}
}

func TestUse_UnknownNameIsAUsageError(t *testing.T) {
	selectedFixture(t)

	if code, _, errOut := cli(t, "use", "-c", "Nope"); code != exitUsage || !strings.Contains(errOut, errUnknownReference.Error()) {
		t.Errorf("code %d, stderr %q", code, errOut)
	}
}

func TestRun_UsesTheSelectedCollectionAndEnvironment(t *testing.T) {
	selectedFixture(t)
	cli(t, "-c", "API", "-e", "Local")

	code, out, errOut := cli(t, "run", "Health")

	if code != exitSuccess || !strings.Contains(out, "✓ Health") || !strings.Contains(out, "Local") {
		t.Errorf("code %d, stderr %s, stdout:\n%s", code, errOut, out)
	}
}

func TestRun_WithoutArgumentsRunsTheSelectedCollection(t *testing.T) {
	selectedFixture(t)
	cli(t, "-c", "API", "-e", "Local")

	code, out, errOut := cli(t, "run")

	if code != exitRunFailed || !strings.Contains(out, "✓ Health") || !strings.Contains(out, "Missing User") {
		t.Errorf("code %d, stderr %s, stdout:\n%s", code, errOut, out)
	}
}

func TestRun_ExplicitEnvironmentOverridesTheSelection(t *testing.T) {
	f := selectedFixture(t)
	other := writeJSONFile(t, f.dir, "other.postman_environment.json", postman.Environment{Name: "Other"})
	cli(t, "-c", "API", "-e", other)

	code, out, errOut := cli(t, "run", "Health", "-e", f.envPath)

	if code != exitSuccess || !strings.Contains(out, "Local") {
		t.Errorf("code %d, stderr %s, stdout:\n%s", code, errOut, out)
	}
}

func TestRun_NothingGivenOrSelected(t *testing.T) {
	selectedFixture(t)

	_, err := withSelectedCollection(runOptions{})

	if !errors.Is(err, errNothingSelected) || !errors.Is(err, errUsage) {
		t.Errorf("err = %v, want errNothingSelected", err)
	}
}

func TestWorkflowRun_UsesTheSelectedWorkflow(t *testing.T) {
	f := selectedFixture(t)
	saveWorkflowFor(t, f.collectionPath, healthWorkflow())
	cli(t, "-w", "health-check", "-e", "Local")

	code, out, errOut := cli(t, "workflow", "run")

	if code != exitSuccess || !strings.Contains(out, "✓ Health") {
		t.Errorf("code %d, stderr %s, stdout:\n%s", code, errOut, out)
	}
}

func TestWorkflowRun_NothingGivenOrSelected(t *testing.T) {
	selectedFixture(t)

	if code, _, errOut := cli(t, "workflow", "run"); code != exitUsage || !strings.Contains(errOut, errNoWorkflowSelected.Error()) {
		t.Errorf("code %d, stderr %q", code, errOut)
	}
}

func TestLs_MarksTheSelection(t *testing.T) {
	f := selectedFixture(t)
	saveWorkflowFor(t, f.collectionPath, healthWorkflow())
	cli(t, "-c", "API", "-e", "Local", "-w", "health-check")

	code, out, _ := cli(t, "ls")

	for _, line := range []string{selectedPrefix + "API", selectedPrefix + "Local", selectedPrefix + "health-check"} {
		if code != exitSuccess || !strings.Contains(out, line) {
			t.Errorf("expected %q marked, got:\n%s", line, out)
		}
	}
}
