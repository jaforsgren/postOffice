package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"postOffice/internal/postman"
	"postOffice/internal/script"
)

const (
	okPath       = "ok"
	missingPath  = "missing"
	passingTest  = "pm.test('is 200', () => pm.response.to.have.status(200));"
	failingTest  = "pm.test('is 201', () => pm.expect(pm.response.code).to.eql(201));"
	setsVariable = "pm.collectionVariables.set('userId', pm.response.json().id);"
)

// apiServer answers okPath with 200 {"id":"u1"} and everything else with 404.
func apiServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimPrefix(r.URL.Path, "/") == okPath {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"u1"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	return server
}

func apiRequest(name, path string, tests ...string) postman.Item {
	item := postman.Item{
		Name:    name,
		Request: &postman.Request{Method: "GET", URL: postman.URL{Raw: "{{base}}/" + path}},
	}
	if len(tests) > 0 {
		item.Events = []postman.Event{{Listen: string(script.PhaseTest), Script: postman.Script{Exec: tests}}}
	}
	return item
}

// apiCollection has Users/Get User (passes), Users/Missing User (fails) and Health (passes).
func apiCollection() *postman.Collection {
	return &postman.Collection{
		Info: postman.Info{Name: "API", Schema: "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"},
		Items: []postman.Item{
			{Name: "Users", Items: []postman.Item{
				apiRequest("Get User", okPath, passingTest, setsVariable),
				apiRequest("Missing User", missingPath, passingTest),
			}},
			apiRequest("Health", okPath, passingTest),
		},
	}
}

func writeJSONFile(t *testing.T, dir, name string, value any) string {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

type fixture struct {
	dir            string
	collectionPath string
	envPath        string
}

// newFixture writes the API collection and an environment pointing "base" at server.
func newFixture(t *testing.T, server *httptest.Server) fixture {
	t.Helper()
	dir := t.TempDir()
	return fixture{
		dir:            dir,
		collectionPath: writeJSONFile(t, dir, "api.postman_collection.json", apiCollection()),
		envPath: writeJSONFile(t, dir, "local.postman_environment.json", postman.Environment{
			Name:   "Local",
			Values: []postman.EnvVariable{{Key: "base", Value: server.URL, Enabled: true}},
		}),
	}
}

func cli(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runCLI(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestIsTUIInvocation(t *testing.T) {
	cases := map[string]bool{
		"":           true,
		"--log x":    true,
		"-log x":     true,
		"run a.json": false,
		"-m a b":     false,
		"--help":     false,
		"-h":         false,
		"bogus":      false,
	}
	for line, want := range cases {
		if got := isTUIInvocation(strings.Fields(line)); got != want {
			t.Errorf("isTUIInvocation(%q) = %v, want %v", line, got, want)
		}
	}
}

func TestParseFlags(t *testing.T) {
	flagA := flagSpec{names: []string{"--alpha", "-a"}, takesValue: true}
	flagB := flagSpec{names: []string{"--beta"}}

	parsed, err := parseFlags([]string{"one", "-a", "x", "--beta", "two", "--alpha=y"}, flagA, flagB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(parsed.positional, ",") != "one,two" || parsed.value(flagA) != "y" || !parsed.has(flagB) {
		t.Errorf("got %+v", parsed)
	}

	for _, args := range [][]string{{"--gamma"}, {"--alpha"}, {"--beta=1"}} {
		if _, err := parseFlags(args, flagA, flagB); !errors.Is(err, errUsage) {
			t.Errorf("%v: err = %v, want errUsage", args, err)
		}
	}
}

func TestParseRunArgs(t *testing.T) {
	opts, err := parseRunArgs([]string{"c.json", "Users/Get User", "-e", "env.json", "--output", "junit", "--report", "r.xml", "--persist"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := executionOptions{environmentPath: "env.json", output: outputJUnit, reportPath: "r.xml", persist: true}
	if opts.collectionPath != "c.json" || opts.requestPath != "Users/Get User" || !reflect.DeepEqual(opts.execution, want) {
		t.Errorf("got %+v", opts)
	}

	folder, err := parseRunArgs([]string{"c.json", "--folder", "Users/Admin"})
	if err != nil || strings.Join(folder.folderPath, "|") != "Users|Admin" || folder.execution.output != outputText {
		t.Errorf("folder: got %+v, %v", folder, err)
	}

	for _, args := range [][]string{
		{"c.json", "a", "b"},
		{"c.json", "Users/Get User", "--folder", "Users"},
		{"c.json", "--folder", ""},
		{"c.json", "--output", "xml"},
		{"c.json", "--report", "r.json"},
		{"c.json", "--repeat", "3"},
	} {
		if _, err := parseRunArgs(args); !errors.Is(err, errUsage) {
			t.Errorf("%v: err = %v, want errUsage", args, err)
		}
	}
}

func TestHelp(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"help"}} {
		code, out, _ := cli(t, args...)
		if code != exitSuccess || !strings.Contains(out, "Commands:") || !strings.Contains(out, "Exit codes:") {
			t.Errorf("%v: code %d, output:\n%s", args, code, out)
		}
		for _, cmd := range cliCommands {
			if !strings.Contains(out, cmd.name) {
				t.Errorf("%v: usage does not list %q", args, cmd.name)
			}
		}
	}

	for _, args := range [][]string{{"run", "--help"}, {"help", "run"}, {"workflow", "-h"}, {"ls", "--help"}, {"merge", "--help"}, {"-m", "--help"}, {"alias", "--help"}} {
		code, out, _ := cli(t, args...)
		if code != exitSuccess || !strings.HasPrefix(out, "Usage:") {
			t.Errorf("%v: code %d, output:\n%s", args, code, out)
		}
	}
}

func TestUnknownCommandPrintsUsageAndExits2(t *testing.T) {
	code, _, errOut := cli(t, "bogus")
	if code != exitUsage || !strings.Contains(errOut, `unknown command "bogus"`) || !strings.Contains(errOut, "Commands:") {
		t.Errorf("code %d, stderr:\n%s", code, errOut)
	}

	code, _, errOut = cli(t, "help", "bogus")
	if code != exitUsage || !strings.Contains(errOut, `unknown command "bogus"`) {
		t.Errorf("help bogus: code %d, stderr:\n%s", code, errOut)
	}
}

func TestUsageErrorShowsMessageThenCommandUsage(t *testing.T) {
	code, _, errOut := cli(t, "run", "c.json", "--output", "xml")

	if code != exitUsage || !strings.HasPrefix(errOut, `Error: --output must be text, json or junit, got "xml"`) || !strings.Contains(errOut, runCommand.usage) {
		t.Errorf("code %d, stderr:\n%s", code, errOut)
	}
}

func TestRun_SingleRequestPasses(t *testing.T) {
	f := newFixture(t, apiServer(t))

	code, out, errOut := cli(t, "run", f.collectionPath, "Users/Get User", "-e", f.envPath)

	if code != exitSuccess {
		t.Fatalf("code %d, stderr %s, stdout:\n%s", code, errOut, out)
	}
	for _, want := range []string{`Run "Users/Get User" (collection API, environment Local)`, "✓ Users/Get User  200 OK", "    ✓ is 200", "1 requests: 1 passed, 0 failed, 0 skipped", "1 tests: 1 passed, 0 failed", "PASSED"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRun_FolderAndCollection(t *testing.T) {
	f := newFixture(t, apiServer(t))

	code, out, _ := cli(t, "run", f.collectionPath, "--folder", "Users", "-e", f.envPath)
	if code != exitRunFailed {
		t.Errorf("folder: code %d, want %d:\n%s", code, exitRunFailed, out)
	}
	for _, want := range []string{"✓ Users/Get User", "✗ Users/Missing User  404 Not Found", "expected status 200 but got 404", "2 requests: 1 passed, 1 failed", "FAILED: one or more steps failed"} {
		if !strings.Contains(out, want) {
			t.Errorf("folder output missing %q:\n%s", want, out)
		}
	}

	code, out, _ = cli(t, "run", f.collectionPath, "-e", f.envPath)
	if code != exitRunFailed || !strings.Contains(out, "3 requests: 2 passed, 1 failed") || !strings.Contains(out, "✓ Health") {
		t.Errorf("collection: code %d:\n%s", code, out)
	}
}

func TestRun_InputErrorsExit2(t *testing.T) {
	f := newFixture(t, apiServer(t))
	invalid := filepath.Join(f.dir, "invalid.json")
	if err := os.WriteFile(invalid, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}

	cases := map[string][]string{
		"missing collection":  {"run", filepath.Join(f.dir, "nope.json")},
		"invalid collection":  {"run", invalid},
		"missing environment": {"run", f.collectionPath, "-e", filepath.Join(f.dir, "nope.json")},
		"unknown request":     {"run", f.collectionPath, "Users/Nobody"},
		"unknown folder":      {"run", f.collectionPath, "--folder", "Admins"},
		"bad flag":            {"run", f.collectionPath, "--retries"},
	}
	for name, args := range cases {
		if code, _, errOut := cli(t, args...); code != exitUsage || !strings.HasPrefix(errOut, "Error: ") {
			t.Errorf("%s: code %d, stderr %q", name, code, errOut)
		}
	}
}

func TestRun_JSONOutput(t *testing.T) {
	f := newFixture(t, apiServer(t))

	code, out, _ := cli(t, "run", f.collectionPath, "--folder", "Users", "-e", f.envPath, "--output", "json")
	if code != exitRunFailed {
		t.Errorf("code %d", code)
	}

	var report runReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if report.Passed || report.Collection != "API" || report.Environment != "Local" {
		t.Errorf("unexpected report header %+v", report)
	}
	wantSummary := reportSummary{Requests: 2, Passed: 1, Failed: 1, Tests: 2, TestsPassed: 1, TestsFailed: 1}
	if report.Summary != wantSummary {
		t.Errorf("summary = %+v, want %+v", report.Summary, wantSummary)
	}
	missing := report.Requests[1]
	if missing.Request != "Users/Missing User" || missing.HTTPStatus != "404 Not Found" || missing.Tests[0].Passed {
		t.Errorf("unexpected request report %+v", missing)
	}
}

func TestRun_JUnitReportFile(t *testing.T) {
	f := newFixture(t, apiServer(t))
	reportPath := filepath.Join(f.dir, "report.xml")

	code, out, _ := cli(t, "run", f.collectionPath, "-e", f.envPath, "--output", "junit", "--report", reportPath)
	if code != exitRunFailed {
		t.Errorf("code %d", code)
	}
	if !strings.Contains(out, "PASSED") && !strings.Contains(out, "FAILED") {
		t.Errorf("expected the text report on stdout:\n%s", out)
	}

	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	var suites junitTestSuites
	if err := xml.Unmarshal(data, &suites); err != nil {
		t.Fatalf("invalid JUnit XML: %v\n%s", err, data)
	}
	if len(suites.Suites) != 3 || suites.Tests != 3 || suites.Failures != 1 {
		t.Errorf("suites=%d tests=%d failures=%d, want 3/3/1:\n%s", len(suites.Suites), suites.Tests, suites.Failures, data)
	}
	failed := suites.Suites[1]
	if failed.Name != "Users/Missing User" || failed.Cases[0].Name != "is 200" || failed.Cases[0].Failure == nil {
		t.Errorf("unexpected failing suite %+v", failed)
	}
}

func TestRun_PersistIsOptIn(t *testing.T) {
	f := newFixture(t, apiServer(t))
	before, err := os.ReadFile(f.collectionPath)
	if err != nil {
		t.Fatal(err)
	}

	cli(t, "run", f.collectionPath, "Users/Get User", "-e", f.envPath)
	after, _ := os.ReadFile(f.collectionPath)
	if !bytes.Equal(before, after) {
		t.Fatal("expected the collection file untouched without --persist")
	}

	cli(t, "run", f.collectionPath, "Users/Get User", "-e", f.envPath, "--persist")
	saved, err := postman.NewParser().LoadCollection(f.collectionPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Variables) != 1 || saved.Variables[0].Key != "userId" || saved.Variables[0].Value != "u1" {
		t.Errorf("expected userId persisted, got %+v", saved.Variables)
	}
}

func TestRun_PersistRefusesNonPostmanCollections(t *testing.T) {
	dir := t.TempDir()
	spec := writeJSONFile(t, dir, "openapi.json", map[string]any{"openapi": "3.0.0", "info": map[string]any{"title": "Spec"}, "paths": map[string]any{}})

	code, _, errOut := cli(t, "run", spec, "--persist")
	if code != exitUsage || !strings.Contains(errOut, errPersistUnsupported.Error()) {
		t.Errorf("code %d, stderr %q", code, errOut)
	}
}
