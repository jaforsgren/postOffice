package dependency

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
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
	want := Node{Requires: []string{loginPath}, Produces: []string{userIDVar}}
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

func TestSaveLoad_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.dependencies.yaml")
	// Empty lists are written as [] so a scaffolded entry shows both keys; they load back non-nil.
	original := &Graph{Requests: map[string]Node{
		loginPath:   {Requires: []string{}, Produces: []string{tokenVar}},
		getUserPath: {Requires: []string{loginPath}, Produces: []string{}},
	}}
	if err := Save(original, path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	g, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(g, original) {
		t.Errorf("round trip mismatch: got %+v", g)
	}
}
