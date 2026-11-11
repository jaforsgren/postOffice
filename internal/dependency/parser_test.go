package dependency

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestFilePath(t *testing.T) {
	got := FilePath("/path/to/My.postman_collection.json")
	if want := "/path/to/My.postman_collection.dependencies.yaml"; got != want {
		t.Errorf("FilePath = %q, want %q", got, want)
	}
}

func TestLoad_MissingFileYieldsEmptyGraph(t *testing.T) {
	g, err := Load(filepath.Join(t.TempDir(), "absent.dependencies.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.Requests == nil || len(g.Requests) != 0 {
		t.Errorf("expected empty initialised graph, got %+v", g)
	}
}

func TestLoad_Parses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.dependencies.yaml")
	content := "requests:\n  Users/Get User:\n    requires:\n      - Auth/Login\n    produces:\n      - userId\n"
	if err := os.WriteFile(path, []byte(content), filePermissions); err != nil {
		t.Fatal(err)
	}

	g, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Node{Requires: requires(loginPath), Produces: []string{userIDVar}}
	if got := g.Node(getUserPath); !reflect.DeepEqual(got, want) {
		t.Errorf("Node = %+v, want %+v", got, want)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.dependencies.yaml")
	if err := os.WriteFile(path, []byte("requests: [unclosed"), filePermissions); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("expected parse error")
	}
}

func TestLoad_EmptyFileYieldsEmptyGraph(t *testing.T) {
	path := writeDependencyFile(t, "")
	g, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.Requests == nil || len(g.Requests) != 0 {
		t.Errorf("expected empty initialised graph, got %+v", g)
	}
}

func TestLoad_ParsesRequirementRules(t *testing.T) {
	path := writeDependencyFile(t, `requests:
  Users/Get User:
    requires:
      - Users/Profile
      - request: Auth/Login
        max_age: 15m
        refresh_on: [401, 403]
      - request: Users/Create User
        when_missing: [userId]
`)

	g, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []Requirement{
		{Request: profilePath},
		{Request: loginPath, MaxAge: 15 * time.Minute, RefreshOn: []int{401, 403}},
		{Request: "Users/Create User", WhenMissing: []string{userIDVar}},
	}
	if got := g.Node(getUserPath).Requires; !reflect.DeepEqual(got, want) {
		t.Errorf("Requires = %+v, want %+v", got, want)
	}
}

func TestLoad_RejectsInvalidRequirements(t *testing.T) {
	tests := []struct {
		name        string
		requirement string
		want        string
	}{
		{"unknown key", "{request: Auth/Login, max-age: 15m}", `line 4: invalid requires entry: unknown key "max-age"`},
		{"missing request", "{max_age: 15m}", "request is required"},
		{"bad duration", "{request: Auth/Login, max_age: 15x}", `max_age "15x" is not a duration`},
		{"non-positive duration", "{request: Auth/Login, max_age: 0s}", "max_age must be positive"},
		{"status out of range", "{request: Auth/Login, refresh_on: [42]}", "refresh_on status 42 is not between 100 and 599"},
		{"max_age with when_missing", "{request: Auth/Login, max_age: 1h, when_missing: [token]}", "max_age and when_missing cannot be combined"},
		{"sequence entry", "[Auth/Login]", "expected a request path or a mapping"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeDependencyFile(t, "requests:\n  Users/Get User:\n    requires:\n      - "+tt.requirement+"\n")
			_, err := Load(path)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Load error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestLoad_RejectsUnknownNodeKey(t *testing.T) {
	path := writeDependencyFile(t, "requests:\n  Users/Get User:\n    require: [Auth/Login]\n")
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "field require not found") {
		t.Errorf("expected unknown field error, got %v", err)
	}
}

func TestAddRequest_CreatesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.dependencies.yaml")
	if err := AddRequest(path, getUserPath); err != nil {
		t.Fatalf("AddRequest: %v", err)
	}
	assertFileContent(t, path, "requests:\n  Users/Get User:\n    requires: []\n    produces: []\n")
}

func TestAddRequest_KeepsCommentsAndExistingEntries(t *testing.T) {
	original := `# Login before anything else.
requests:
  Auth/Login:
    produces: [token] # minted locally
`
	path := writeDependencyFile(t, original)
	if err := AddRequest(path, getUserPath); err != nil {
		t.Fatalf("AddRequest: %v", err)
	}
	assertFileContent(t, path, original+"  Users/Get User:\n    requires: []\n    produces: []\n")
}

func TestAddRequest_DeclaredRequestIsUnchanged(t *testing.T) {
	original := "requests:\n  Users/Get User:\n    requires: [Auth/Login]\n"
	path := writeDependencyFile(t, original)
	if err := AddRequest(path, getUserPath); err != nil {
		t.Fatalf("AddRequest: %v", err)
	}
	assertFileContent(t, path, original)
}

func TestAddRequest_FillsEmptyRequestsKey(t *testing.T) {
	path := writeDependencyFile(t, "requests:\n")
	if err := AddRequest(path, getUserPath); err != nil {
		t.Fatalf("AddRequest: %v", err)
	}
	assertFileContent(t, path, "requests:\n  Users/Get User:\n    requires: []\n    produces: []\n")
}

func writeDependencyFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "c.dependencies.yaml")
	if err := os.WriteFile(path, []byte(content), filePermissions); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("file content:\n%s\nwant:\n%s", got, want)
	}
}

func TestLoad_ResolvesAnchoredRequirements(t *testing.T) {
	path := writeDependencyFile(t, `requests:
  Users/Get User:
    requires:
      - &login {request: Auth/Login, max_age: 1h, refresh_on: [401]}
  Users/Profile:
    requires: [*login]
`)

	g, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []Requirement{{Request: loginPath, MaxAge: time.Hour, RefreshOn: []int{401}}}
	if got := g.Node(profilePath).Requires; !reflect.DeepEqual(got, want) {
		t.Errorf("Requires = %+v, want %+v", got, want)
	}
}
