package workflow

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"postOffice/internal/postman"
)

func folderRequest(name string) postman.Item {
	return postman.Item{
		Name:    name,
		Request: &postman.Request{Method: "GET", URL: postman.URL{Raw: "https://example.com/" + name}},
	}
}

// buildNestedFolder mirrors a folder like "policies/TEST" with requests before, inside and after
// nested subfolders, so ordering has to be depth-first in collection order.
func buildNestedFolder() postman.Item {
	return postman.Item{
		Name: "TEST",
		Items: []postman.Item{
			folderRequest("setup"),
			{
				Name: "invite",
				Items: []postman.Item{
					folderRequest("create invite"),
					{
						Name:  "deep",
						Items: []postman.Item{folderRequest("redeem")},
					},
				},
			},
			folderRequest("cleanup"),
		},
	}
}

func TestNewFolderWorkflow_stepsAreDepthFirstInCollectionOrder(t *testing.T) {
	wf, err := NewFolderWorkflow(buildNestedFolder(), []string{"policies", "TEST"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{
		"policies/TEST/setup",
		"policies/TEST/invite/create invite",
		"policies/TEST/invite/deep/redeem",
		"policies/TEST/cleanup",
	}
	if len(wf.Steps) != len(want) {
		t.Fatalf("Steps length = %d, want %d", len(wf.Steps), len(want))
	}
	for i, path := range want {
		if wf.Steps[i].Request != path {
			t.Errorf("Steps[%d].Request = %q, want %q", i, wf.Steps[i].Request, path)
		}
	}
}

func TestNewFolderWorkflow_nameIsFullFolderPathAndIDIsItsSlug(t *testing.T) {
	wf, err := NewFolderWorkflow(buildNestedFolder(), []string{"policies", "TEST"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wf.Name != "policies/TEST" {
		t.Errorf("Name = %q, want %q", wf.Name, "policies/TEST")
	}
	if wf.ID != "policies-test" {
		t.Errorf("ID = %q, want %q", wf.ID, "policies-test")
	}
}

func TestNewFolderWorkflow_scriptListsEveryStepInOrder(t *testing.T) {
	wf, err := NewFolderWorkflow(buildNestedFolder(), []string{"policies", "TEST"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `const steps = ["setup", "create-invite", "redeem", "cleanup"];`
	if !strings.Contains(wf.Script, want) {
		t.Errorf("Script =\n%s\nwant it to contain %s", wf.Script, want)
	}
}

func TestResolveFolderWorkflow_upgradesUnmodifiedLegacyScript(t *testing.T) {
	saved := generateFolderWorkflow(t, buildNestedFolder())
	saved.Script = legacyFolderRunScript(saved.Steps)
	generated := generateFolderWorkflow(t, buildNestedFolder())

	wf, resolution := ResolveFolderWorkflow(saved, generated)
	if wf != generated || resolution != FolderWorkflowRegenerated {
		t.Errorf("got (%p, %v), want generated workflow and FolderWorkflowRegenerated", wf, resolution)
	}
}

func TestNewFolderWorkflow_duplicateRequestNamesGetUniqueStepIDs(t *testing.T) {
	folder := postman.Item{
		Name: "Users",
		Items: []postman.Item{
			folderRequest("get"),
			{Name: "Admins", Items: []postman.Item{folderRequest("get")}},
			folderRequest("Get!"),
		},
	}

	wf, err := NewFolderWorkflow(folder, []string{"Users"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"get", "get-2", "get-3"}
	for i, id := range want {
		if wf.Steps[i].ID != id {
			t.Errorf("Steps[%d].ID = %q, want %q", i, wf.Steps[i].ID, id)
		}
	}
}

func TestNewFolderWorkflow_noRequests(t *testing.T) {
	folder := postman.Item{Name: "Empty", Items: []postman.Item{{Name: "Nested"}}}

	_, err := NewFolderWorkflow(folder, []string{"Empty"})
	if !errors.Is(err, ErrFolderHasNoRequests) {
		t.Fatalf("err = %v, want ErrFolderHasNoRequests", err)
	}
}

func TestNewFolderWorkflow_nameWithPathSeparatorIsEscaped(t *testing.T) {
	folder := postman.Item{Name: "access", Items: []postman.Item{folderRequest("get policy → 403/404")}}

	wf, err := NewFolderWorkflow(folder, []string{"access"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := wf.Steps[0].Request, `access/get policy → 403\/404`; got != want {
		t.Errorf("step request = %q, want %q", got, want)
	}
}

func TestFolderWorkflowID(t *testing.T) {
	tests := []struct {
		path []string
		want string
	}{
		{[]string{"policies", "TEST"}, "policies-test"},
		{[]string{"a", "b", "c"}, "a-b-c"},
		{[]string{"Claim Documentation", "claim – created!"}, "claim-documentation-claim-created"},
		{[]string{"Försäkring"}, "försäkring"},
		{[]string{"→"}, fallbackFolderSlug},
	}
	for _, tt := range tests {
		if got := FolderWorkflowID(tt.path); got != tt.want {
			t.Errorf("FolderWorkflowID(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestFolderWorkflowName_nested(t *testing.T) {
	if got := FolderWorkflowName([]string{"a", "b", "c"}); got != "a/b/c" {
		t.Errorf("FolderWorkflowName = %q, want %q", got, "a/b/c")
	}
}

func generateFolderWorkflow(t *testing.T, folder postman.Item) *Workflow {
	t.Helper()
	wf, err := NewFolderWorkflow(folder, []string{"policies", folder.Name})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return wf
}

func TestResolveFolderWorkflow_createsWhenNoneSaved(t *testing.T) {
	generated := generateFolderWorkflow(t, buildNestedFolder())

	wf, resolution := ResolveFolderWorkflow(nil, generated)
	if wf != generated || resolution != FolderWorkflowCreated {
		t.Errorf("got (%p, %v), want generated workflow and FolderWorkflowCreated", wf, resolution)
	}
	if !resolution.NeedsSave() {
		t.Error("a created workflow must be saved")
	}
}

func TestResolveFolderWorkflow_reusesSavedWhenRequestsUnchanged(t *testing.T) {
	saved := generateFolderWorkflow(t, buildNestedFolder())
	saved.Script = "export default async function (wf, pm) { await wf.run(\"setup\"); }"
	generated := generateFolderWorkflow(t, buildNestedFolder())

	wf, resolution := ResolveFolderWorkflow(saved, generated)
	if wf != saved || resolution != FolderWorkflowReused {
		t.Errorf("got (%p, %v), want saved workflow and FolderWorkflowReused", wf, resolution)
	}
	if resolution.NeedsSave() {
		t.Error("a reused workflow must not be saved")
	}
}

func TestResolveFolderWorkflow_reusesWorkflowLoadedFromDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policies-test.yaml")
	if err := SaveWorkflow(generateFolderWorkflow(t, buildNestedFolder()), path); err != nil {
		t.Fatal(err)
	}
	saved, err := LoadWorkflow(path)
	if err != nil {
		t.Fatal(err)
	}

	_, resolution := ResolveFolderWorkflow(saved, generateFolderWorkflow(t, buildNestedFolder()))
	if resolution != FolderWorkflowReused {
		t.Errorf("resolution = %v, want FolderWorkflowReused", resolution)
	}
}

func TestResolveFolderWorkflow_regeneratesUnmodifiedWhenRequestsChanged(t *testing.T) {
	saved := generateFolderWorkflow(t, buildNestedFolder())
	changed := buildNestedFolder()
	changed.Items = append(changed.Items, folderRequest("verify"))
	generated := generateFolderWorkflow(t, changed)

	wf, resolution := ResolveFolderWorkflow(saved, generated)
	if wf != generated || resolution != FolderWorkflowRegenerated {
		t.Errorf("got (%p, %v), want generated workflow and FolderWorkflowRegenerated", wf, resolution)
	}
	if !resolution.NeedsSave() {
		t.Error("a regenerated workflow must be saved")
	}
}

func TestResolveFolderWorkflow_keepsHandEditedWhenRequestsChanged(t *testing.T) {
	changed := buildNestedFolder()
	changed.Items = append(changed.Items, folderRequest("verify"))

	editedScript := generateFolderWorkflow(t, buildNestedFolder())
	editedScript.Script = "export default async function (wf, pm) { await wf.run(\"cleanup\"); }"

	editedStep := generateFolderWorkflow(t, buildNestedFolder())
	editedStep.Steps[0].PostScript = "console.log(step.response.code);"

	for name, saved := range map[string]*Workflow{"script": editedScript, "step script": editedStep} {
		wf, resolution := ResolveFolderWorkflow(saved, generateFolderWorkflow(t, changed))
		if wf != saved || resolution != FolderWorkflowCustomizedStale {
			t.Errorf("%s edited: got (%p, %v), want saved workflow and FolderWorkflowCustomizedStale", name, wf, resolution)
		}
		if resolution.NeedsSave() {
			t.Errorf("%s edited: a hand-edited workflow must not be overwritten", name)
		}
	}
}
