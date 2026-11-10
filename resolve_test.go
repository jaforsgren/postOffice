package main

import (
	"errors"
	"strings"
	"testing"
)

func TestRun_ResolvesLoadedCollectionAndEnvironmentByName(t *testing.T) {
	f := newFixture(t, apiServer(t))
	writeLoadedConfig(t, []string{f.collectionPath}, []string{f.envPath})

	for _, args := range [][]string{
		{"run", "API", "Health", "-e", "Local"},
		{"run", "api", "Health", "-e", "local"},
	} {
		code, out, errOut := cli(t, args...)
		if code != exitSuccess || !strings.Contains(out, "✓ Health") {
			t.Errorf("%v: code %d, stderr %s, stdout:\n%s", args, code, errOut, out)
		}
	}
}

func TestRun_UnknownNameIsAUsageError(t *testing.T) {
	writeLoadedConfig(t, nil, nil)

	code, _, errOut := cli(t, "run", "Nope")

	if code != exitUsage || !strings.Contains(errOut, errUnknownReference.Error()) {
		t.Errorf("code %d, stderr %q", code, errOut)
	}
}

func TestResolveNamedFile_AmbiguousName(t *testing.T) {
	candidates := []namedFile{{names: []string{"SAPI"}, path: "/a/SAPI.json"}, {names: []string{"sapi"}, path: "/b/sapi.json"}}

	_, err := resolveNamedFile("sapi", collectionReference, candidates)

	if !errors.Is(err, errAmbiguousReference) || !errors.Is(err, errUsage) {
		t.Errorf("err = %v, want errAmbiguousReference", err)
	}
}

func TestFileStem(t *testing.T) {
	cases := map[string]string{
		"/x/SAPI.postman_collection.json":       "SAPI",
		"/x/Big tusks.postman_environment.json": "Big tusks.postman_environment",
		"/x/plain.json":                         "plain",
	}
	for path, want := range cases {
		if got := fileStem(path, collectionExportSuffix); got != want {
			t.Errorf("fileStem(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestWorkflowRun_ResolvesWorkflowByIDOrName(t *testing.T) {
	f := newFixture(t, apiServer(t))
	writeLoadedConfig(t, []string{f.collectionPath}, []string{f.envPath})
	saveWorkflowFor(t, f.collectionPath, healthWorkflow())

	for _, ref := range []string{"health-check", "Health check"} {
		code, out, errOut := cli(t, "workflow", "run", ref, "-e", "Local")
		if code != exitSuccess || !strings.Contains(out, "✓ Health") {
			t.Errorf("%s: code %d, stderr %s, stdout:\n%s", ref, code, errOut, out)
		}
	}
}

func TestWorkflowRun_ResolvesWorkflowNextToTheGivenCollection(t *testing.T) {
	f := newFixture(t, apiServer(t))
	writeLoadedConfig(t, nil, nil)
	saveWorkflowFor(t, f.collectionPath, healthWorkflow())

	code, out, errOut := cli(t, "workflow", "run", "health-check", "-c", f.collectionPath, "-e", f.envPath)

	if code != exitSuccess || !strings.Contains(out, "✓ Health") {
		t.Errorf("code %d, stderr %s, stdout:\n%s", code, errOut, out)
	}
}

func TestLsRequest_ResolvesCollectionByName(t *testing.T) {
	f := newFixture(t, apiServer(t))
	writeLoadedConfig(t, []string{f.collectionPath}, nil)

	code, out, errOut := cli(t, "ls", "request", "-c", "api")

	if code != exitSuccess || !strings.Contains(out, "Users/Get User") {
		t.Errorf("code %d, stderr %s, stdout:\n%s", code, errOut, out)
	}
}
