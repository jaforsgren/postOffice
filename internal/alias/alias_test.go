package alias

import "testing"

func TestLoad_MissingFileReturnsEmptyConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Aliases) != 0 {
		t.Errorf("expected empty aliases, got %d", len(cfg.Aliases))
	}
}

func TestSaveAndLoad_RoundTrips(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg.Set("my-merge", Alias{Command: "merge", Args: []string{"a.json", "b.json", "--forceMerge"}})
	if err := cfg.Save(); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	reloaded, err := Load()
	if err != nil {
		t.Fatalf("failed to reload: %v", err)
	}

	a, exists := reloaded.Get("my-merge")
	if !exists {
		t.Fatal("expected alias my-merge to exist after reload")
	}
	if a.Command != "merge" {
		t.Errorf("expected command merge, got %s", a.Command)
	}
	if len(a.Args) != 3 || a.Args[2] != "--forceMerge" {
		t.Errorf("unexpected args: %v", a.Args)
	}
}

func TestRemove(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg, _ := Load()
	cfg.Set("temp", Alias{Command: "merge", Args: []string{"a.json", "b.json"}})

	if !cfg.Remove("temp") {
		t.Fatal("expected Remove to report the alias existed")
	}
	if _, exists := cfg.Get("temp"); exists {
		t.Fatal("expected alias to be gone after Remove")
	}
	if cfg.Remove("temp") {
		t.Fatal("expected second Remove of the same name to report false")
	}
}
