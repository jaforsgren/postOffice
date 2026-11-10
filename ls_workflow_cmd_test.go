package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"postOffice/internal/postman"
	"postOffice/internal/workflow"
)

const configFileName = ".postoffice_collections.json"

// writeLoadedConfig makes collectionPaths and envPaths the "loaded" files in a fresh HOME.
func writeLoadedConfig(t *testing.T, collectionPaths, envPaths []string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeJSONFile(t, home, configFileName, postman.PersistenceConfig{CollectionPaths: collectionPaths, EnvironmentPaths: envPaths})
}

func saveWorkflowFor(t *testing.T, collectionPath string, wf *workflow.Workflow) string {
	t.Helper()
	path := workflow.WorkflowPath(collectionPath, wf.ID)
	if err := workflow.SaveWorkflow(wf, path); err != nil {
		t.Fatal(err)
	}
	return path
}

func healthWorkflow() *workflow.Workflow {
	steps := []workflow.Step{{ID: "health", Request: "Health"}}
	return &workflow.Workflow{ID: "health-check", Name: "Health check", Steps: steps, Script: workflow.PartialScript(steps, 0, 0)}
}

func TestWorkflowRun_FindsSiblingCollection(t *testing.T) {
	writeLoadedConfig(t, nil, nil)
	f := newFixture(t, apiServer(t))
	wfPath := saveWorkflowFor(t, f.collectionPath, healthWorkflow())

	code, out, errOut := cli(t, "workflow", "run", wfPath, "-e", f.envPath)

	if code != exitSuccess || !strings.Contains(out, `Run "Health check" (collection API`) || !strings.Contains(out, "✓ Health") {
		t.Errorf("code %d, stderr %s, stdout:\n%s", code, errOut, out)
	}
}

func TestFindWorkflowCollection(t *testing.T) {
	t.Run("prefers the loaded collection owning the workflows directory", func(t *testing.T) {
		f := newFixture(t, apiServer(t))
		other := writeJSONFile(t, f.dir, "other.postman_collection.json", apiCollection())
		writeLoadedConfig(t, []string{other}, nil)

		got, err := findWorkflowCollection(saveWorkflowFor(t, other, healthWorkflow()))
		if err != nil || got != other {
			t.Errorf("got %q, %v, want %q", got, err, other)
		}
	})

	t.Run("several sibling collections are ambiguous", func(t *testing.T) {
		writeLoadedConfig(t, nil, nil)
		f := newFixture(t, apiServer(t))
		writeJSONFile(t, f.dir, "other.postman_collection.json", apiCollection())

		_, err := findWorkflowCollection(saveWorkflowFor(t, f.collectionPath, healthWorkflow()))
		if !errors.Is(err, errAmbiguousCollection) || !errors.Is(err, errUsage) {
			t.Errorf("err = %v, want errAmbiguousCollection", err)
		}
	})

	t.Run("no collection found", func(t *testing.T) {
		writeLoadedConfig(t, nil, nil)
		wfPath := filepath.Join(t.TempDir(), "workflows", "lonely.yaml")
		if err := workflow.SaveWorkflow(healthWorkflow(), wfPath); err != nil {
			t.Fatal(err)
		}

		_, err := findWorkflowCollection(wfPath)
		if !errors.Is(err, errCollectionNotFound) {
			t.Errorf("err = %v, want errCollectionNotFound", err)
		}
	})
}

func TestWorkflowRun_ExplicitCollectionAndErrors(t *testing.T) {
	writeLoadedConfig(t, nil, nil)
	f := newFixture(t, apiServer(t))
	elsewhere := filepath.Join(t.TempDir(), "health.yaml")
	if err := workflow.SaveWorkflow(healthWorkflow(), elsewhere); err != nil {
		t.Fatal(err)
	}

	if code, out, _ := cli(t, "workflow", "run", elsewhere, "-c", f.collectionPath, "-e", f.envPath); code != exitSuccess {
		t.Errorf("explicit -c: code %d:\n%s", code, out)
	}
	for name, args := range map[string][]string{
		"no subcommand":     {"workflow"},
		"wrong subcommand":  {"workflow", "start", elsewhere},
		"missing file":      {"workflow", "run", filepath.Join(f.dir, "nope.yaml"), "-c", f.collectionPath},
		"no collection":     {"workflow", "run", elsewhere},
		"two workflow args": {"workflow", "run", elsewhere, elsewhere},
	} {
		if code, _, _ := cli(t, args...); code != exitUsage {
			t.Errorf("%s: code %d, want %d", name, code, exitUsage)
		}
	}
}

func TestWorkflowRun_ReportsScriptLogsAndUnrunnableSteps(t *testing.T) {
	writeLoadedConfig(t, nil, nil)
	f := newFixture(t, apiServer(t))
	steps := []workflow.Step{{ID: "ghost", Request: "Users/Ghost"}}
	wfPath := saveWorkflowFor(t, f.collectionPath, &workflow.Workflow{
		ID:     "ghost",
		Name:   "Ghost",
		Steps:  steps,
		Script: "export default async function (wf, pm) {\n  console.log('starting');\n  await wf.run('ghost');\n}\n",
	})

	code, out, _ := cli(t, "workflow", "run", wfPath, "--output", "json")

	if code != exitRunFailed {
		t.Errorf("code %d, want %d", code, exitRunFailed)
	}
	var report runReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(report.Logs) != 1 || report.Logs[0] != "starting" {
		t.Errorf("logs = %v, want only the workflow script's own output", report.Logs)
	}
	ghost := report.Requests[0]
	if ghost.Status != workflow.StatusFailed || len(ghost.Errors) == 0 || !strings.Contains(ghost.Errors[0], "Users/Ghost") {
		t.Errorf("request = %+v, want the engine's resolve error", ghost)
	}
}

func TestLs_Overview(t *testing.T) {
	f := newFixture(t, apiServer(t))
	writeLoadedConfig(t, []string{f.collectionPath}, []string{f.envPath})
	saveWorkflowFor(t, f.collectionPath, healthWorkflow())

	code, out, _ := cli(t, "ls")

	if code != exitSuccess {
		t.Fatalf("code %d", code)
	}
	for _, want := range []string{"Collections:", "API", f.collectionPath, "Environments:", "Local", f.envPath, "Workflows:", "health-check", "Health check"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestLs_OverviewWhenNothingIsLoaded(t *testing.T) {
	writeLoadedConfig(t, nil, nil)

	_, out, _ := cli(t, "ls")

	if strings.Count(out, strings.TrimSpace(noneListed)) != 3 {
		t.Errorf("expected (none) for collections, environments and workflows:\n%s", out)
	}
}

func TestLs_Variables(t *testing.T) {
	f := newFixture(t, apiServer(t))
	collection := apiCollection()
	collection.Variables = []postman.Variable{{Key: "userId", Value: "u0"}}
	collectionPath := writeJSONFile(t, f.dir, "vars.postman_collection.json", collection)
	writeLoadedConfig(t, []string{collectionPath}, []string{f.envPath})

	for _, args := range [][]string{{"ls", "var"}, {"ls", "vars", "-c", collectionPath, "-e", f.envPath}, {"ls", "variables"}} {
		code, out, _ := cli(t, args...)
		if code != exitSuccess {
			t.Fatalf("%v: code %d", args, code)
		}
		for _, want := range []string{"KEY", "userId", "u0", "Collection: API", "base", "Environment: Local"} {
			if !strings.Contains(out, want) {
				t.Errorf("%v: output missing %q:\n%s", args, want, out)
			}
		}
	}
}

func TestLs_RequestsAreRunnablePaths(t *testing.T) {
	writeLoadedConfig(t, nil, nil)
	server := apiServer(t)
	f := newFixture(t, server)
	collection := apiCollection()
	collection.Items = append(collection.Items, apiRequest("get → 403/404", okPath))
	collectionPath := writeJSONFile(t, f.dir, "slash.postman_collection.json", collection)

	for _, topic := range []string{"request", "requests", "r"} {
		code, out, _ := cli(t, "ls", topic, "-c", collectionPath)
		if code != exitSuccess {
			t.Fatalf("%s: code %d", topic, code)
		}
		for _, want := range []string{"API:", "GET", "Users/Get User", "Users/Missing User", "Health", `get → 403\/404`} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: output missing %q:\n%s", topic, want, out)
			}
		}
	}

	for _, path := range []string{"Users/Get User", `get → 403\/404`} {
		if code, out, _ := cli(t, "run", collectionPath, path, "-e", f.envPath); code != exitSuccess {
			t.Errorf("listed path %q must run: code %d:\n%s", path, code, out)
		}
	}
}

func TestRun_FolderNameWithSeparator(t *testing.T) {
	writeLoadedConfig(t, nil, nil)
	server := apiServer(t)
	f := newFixture(t, server)
	collection := apiCollection()
	collection.Items = append(collection.Items, postman.Item{Name: "403/404", Items: []postman.Item{apiRequest("inside", okPath)}})
	collectionPath := writeJSONFile(t, f.dir, "slash-folder.postman_collection.json", collection)

	code, out, _ := cli(t, "run", collectionPath, "--folder", `403\/404`, "-e", f.envPath)

	if code != exitSuccess || !strings.Contains(out, "inside") {
		t.Errorf("escaped folder must run: code %d:\n%s", code, out)
	}
}

func TestLs_UsageErrors(t *testing.T) {
	writeLoadedConfig(t, nil, nil)
	for _, args := range [][]string{{"ls", "things"}, {"ls", "var", "extra"}, {"ls", "request", "-e", "x.json"}} {
		if code, _, _ := cli(t, args...); code != exitUsage {
			t.Errorf("%v: code %d, want %d", args, code, exitUsage)
		}
	}
}

func TestAlias_ReplaysHeadlessRun(t *testing.T) {
	writeLoadedConfig(t, nil, nil)
	f := newFixture(t, apiServer(t))

	if code, _, errOut := cli(t, "alias", "add", "smoke", "run", f.collectionPath, "Health", "-e", f.envPath); code != exitSuccess {
		t.Fatalf("alias add: code %d: %s", code, errOut)
	}
	code, out, _ := cli(t, "alias", "run", "smoke")
	if code != exitSuccess || !strings.Contains(out, "✓ Health") {
		t.Errorf("alias run: code %d:\n%s", code, out)
	}

	if code, _, _ := cli(t, "alias", "add", "nope", "help"); code != exitUsage {
		t.Errorf("help must not be aliasable, code %d", code)
	}
}

func TestMergeViaCommandName(t *testing.T) {
	dir := t.TempDir()
	first := writeJSONFile(t, dir, "a.json", apiCollection())
	second := writeJSONFile(t, dir, "b.json", apiCollection())

	for _, name := range []string{"merge", "-m", "--merge"} {
		if code, out, errOut := cli(t, name, first, second, "--forceMerge", "-o", filepath.Join(dir, "out.json")); code != exitSuccess || !strings.Contains(out, "Merged") {
			t.Errorf("%s: code %d, stdout %q, stderr %q", name, code, out, errOut)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "out.json")); err != nil {
		t.Errorf("expected merged output: %v", err)
	}
}
