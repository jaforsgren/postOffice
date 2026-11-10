package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func decodeReport(t *testing.T, out string) runReport {
	t.Helper()
	var report runReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON report: %v\n%s", err, out)
	}
	return report
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseRunArgs_RunnerOptions(t *testing.T) {
	opts, err := parseRunArgs([]string{"c.json", "--folder", "Users", "-n", "3", "--bail", "--data", "rows.csv"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.execution.runner.Iterations != 3 || !opts.execution.runner.StopOnFailure || opts.execution.dataPath != "rows.csv" {
		t.Errorf("got %+v", opts.execution)
	}

	for _, args := range [][]string{
		{"c.json", "--iterations", "many"},
		{"c.json", "--iterations", "-1"},
		{"c.json", "--data"},
	} {
		if _, err := parseRunArgs(args); err == nil {
			t.Errorf("%v: expected a usage error", args)
		}
	}
}

func TestRun_DataDrivenIterations(t *testing.T) {
	server := apiServer(t)
	f := newFixture(t, server)
	collection := apiCollection()
	collection.Items = append(collection.Items, apiRequest("By Path", "{{path}}", "pm.test('data row', () => pm.expect(pm.iterationData.get('path')).to.eql('"+okPath+"'));"))
	collectionPath := writeJSONFile(t, f.dir, "data.postman_collection.json", collection)
	rows := writeFile(t, f.dir, "rows.csv", "path\n"+okPath+"\n"+okPath+"\n")

	t.Run("one iteration per row", func(t *testing.T) {
		code, out, errOut := cli(t, "run", collectionPath, "By Path", "-e", f.envPath, "--data", rows, "--output", "json")
		if code != exitSuccess {
			t.Fatalf("code %d: %s%s", code, out, errOut)
		}
		report := decodeReport(t, out)
		if report.Iterations != 2 || len(report.Requests) != 2 || report.Requests[1].Iteration != 2 || report.Summary.TestsPassed != 2 {
			t.Errorf("unexpected report %+v", report)
		}
	})

	t.Run("explicit count reuses the last row", func(t *testing.T) {
		code, out, _ := cli(t, "run", collectionPath, "By Path", "-e", f.envPath, "--data", rows, "-n", "3")
		if code != exitSuccess || !strings.Contains(out, "3 iterations") || !strings.Contains(out, "#3 By Path") {
			t.Errorf("code %d:\n%s", code, out)
		}
	})
}

func TestRun_DataFileErrorsExit2(t *testing.T) {
	server := apiServer(t)
	f := newFixture(t, server)
	unsupported := writeFile(t, f.dir, "rows.txt", "path\nok\n")
	empty := writeFile(t, f.dir, "empty.csv", "path\n")

	for _, data := range []string{filepath.Join(f.dir, "missing.csv"), unsupported, empty} {
		if code, _, errOut := cli(t, "run", f.collectionPath, "-e", f.envPath, "--data", data); code != exitUsage || !strings.HasPrefix(errOut, "Error: ") {
			t.Errorf("%s: code %d, stderr %q", filepath.Base(data), code, errOut)
		}
	}
}

func TestRun_BailStopsAtFirstFailure(t *testing.T) {
	server := apiServer(t)
	f := newFixture(t, server)

	statusOf := func(report runReport, request string) string {
		for _, r := range report.Requests {
			if r.Request == request {
				return r.Status
			}
		}
		return ""
	}

	_, withoutBail, _ := cli(t, "run", f.collectionPath, "-e", f.envPath, "--output", "json")
	code, withBail, _ := cli(t, "run", f.collectionPath, "-e", f.envPath, "--output", "json", "--bail")

	if code != exitRunFailed {
		t.Errorf("expected exit %d with --bail, got %d", exitRunFailed, code)
	}
	if got := statusOf(decodeReport(t, withoutBail), "Health"); got != "success" {
		t.Errorf("without --bail Health should run, got %q", got)
	}
	if got := statusOf(decodeReport(t, withBail), "Health"); got != "skipped" {
		t.Errorf("with --bail Health should be skipped after Users/Missing User fails, got %q", got)
	}
}

func TestWorkflowRun_AcceptsRunnerOptions(t *testing.T) {
	opts, err := parseWorkflowRunArgs([]string{"run", "wf.yaml", "-c", "c.json", "--bail", "-n", "2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.execution.runner.StopOnFailure || opts.execution.runner.Iterations != 2 {
		t.Errorf("got %+v", opts.execution)
	}
}
