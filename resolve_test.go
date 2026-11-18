package main

import (
	"errors"
	"slices"
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

// sharedWorkflowsFixture loads two collections from one directory with one
// legacy workflow saved directly in their shared workflows directory.
func sharedWorkflowsFixture(t *testing.T) (f fixture, other string) {
	t.Helper()
	f = newFixture(t, apiServer(t))
	otherCollection := apiCollection()
	otherCollection.Info.Name = "Other"
	other = writeJSONFile(t, f.dir, "other.postman_collection.json", otherCollection)
	writeLoadedConfig(t, []string{f.collectionPath, other}, []string{f.envPath})
	saveSharedWorkflow(t, f.collectionPath, healthWorkflow())
	return f, other
}

func TestLs_ListsAWorkflowSharedByCollectionsOnce(t *testing.T) {
	sharedWorkflowsFixture(t)

	code, out, _ := cli(t, "ls")

	if code != exitSuccess || strings.Count(out, "health-check") != 1 || !strings.Contains(out, "API"+collectionNameSeparator+"Other") {
		t.Errorf("code %d, want health-check listed once for both collections:\n%s", code, out)
	}
}

func TestWorkflowRun_SharedWorkflowRunsAgainstTheSelectedCollection(t *testing.T) {
	_, other := sharedWorkflowsFixture(t)
	cli(t, "-c", other)

	code, out, errOut := cli(t, "workflow", "run", "health-check", "-e", "Local")

	if code != exitSuccess || !strings.Contains(out, `(collection Other`) || !strings.Contains(out, "✓ Health") {
		t.Errorf("code %d, stderr %s, stdout:\n%s", code, errOut, out)
	}
}

func TestWorkflowRun_SharedWorkflowWithoutASelectedOwnerIsAmbiguous(t *testing.T) {
	sharedWorkflowsFixture(t)

	code, _, errOut := cli(t, "workflow", "run", "health-check", "-e", "Local")

	if code != exitUsage || !strings.Contains(errOut, errAmbiguousCollection.Error()) {
		t.Errorf("code %d, stderr %q, want errAmbiguousCollection", code, errOut)
	}
}

func TestWorkflowsSavedNextTo_OwnWorkflowsBelongToOneCollection(t *testing.T) {
	f := newFixture(t, apiServer(t))
	other := writeJSONFile(t, f.dir, "other.postman_collection.json", apiCollection())
	saveWorkflowFor(t, f.collectionPath, healthWorkflow())

	saved, err := workflowsSavedNextTo([]string{f.collectionPath, other})

	if err != nil || len(saved) != 1 || !slices.Equal(saved[0].collectionPaths, []string{f.collectionPath}) {
		t.Errorf("got %+v, %v, want the workflow owned by its own collection only", saved, err)
	}
}

func TestWorkflowsSavedNextTo_CollectionsSharingADirectory(t *testing.T) {
	f, other := sharedWorkflowsFixture(t)

	saved, err := workflowsSavedNextTo([]string{f.collectionPath, other})

	if err != nil || len(saved) != 1 || len(saved[0].collectionPaths) != 2 {
		t.Errorf("got %+v, %v, want one workflow owned by both collections", saved, err)
	}
}
