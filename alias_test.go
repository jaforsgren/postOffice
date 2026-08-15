package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"postOffice/internal/postman"
	"testing"
)

func TestAliasAdd_UnknownCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	err := aliasAdd([]string{"my-alias", "bogus", "a.json"})
	if err == nil {
		t.Fatal("expected error for unknown headless command")
	}
}

func TestAliasAdd_MissingArgs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := aliasAdd([]string{"only-name"}); err == nil {
		t.Fatal("expected error when command is missing")
	}
}

func TestAliasRemove_NotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := aliasRemove([]string{"does-not-exist"}); err == nil {
		t.Fatal("expected error removing a nonexistent alias")
	}
}

func TestAliasRun_NotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := aliasRun([]string{"does-not-exist"}); err == nil {
		t.Fatal("expected error running a nonexistent alias")
	}
}

func TestAliasAddListRemove(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := aliasAdd([]string{"my-merge", "merge", "a.json", "b.json", "--forceMerge"}); err != nil {
		t.Fatalf("unexpected error adding alias: %v", err)
	}

	if err := aliasList(); err != nil {
		t.Fatalf("unexpected error listing aliases: %v", err)
	}

	if err := aliasRemove([]string{"my-merge"}); err != nil {
		t.Fatalf("unexpected error removing alias: %v", err)
	}

	if err := aliasRun([]string{"my-merge"}); err == nil {
		t.Fatal("expected error running an alias removed earlier")
	}
}

func TestAliasRun_ExecutesMerge(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()
	file1 := filepath.Join(dir, "a.json")
	file2 := filepath.Join(dir, "b.json")
	outFile := filepath.Join(dir, "merged.json")

	col1 := `{"info":{"name":"A"},"item":[{"name":"Get Users","request":{"method":"GET","url":{"raw":"https://a.example.com/users"}}}]}`
	col2 := `{"info":{"name":"B"},"item":[{"name":"Get Orders","request":{"method":"GET","url":{"raw":"https://b.example.com/orders"}}}]}`

	if err := os.WriteFile(file1, []byte(col1), 0644); err != nil {
		t.Fatalf("failed to write file1: %v", err)
	}
	if err := os.WriteFile(file2, []byte(col2), 0644); err != nil {
		t.Fatalf("failed to write file2: %v", err)
	}

	if err := aliasAdd([]string{"test-merge", "merge", file1, file2, "--forceMerge", "-o", outFile}); err != nil {
		t.Fatalf("unexpected error adding alias: %v", err)
	}

	if err := aliasRun([]string{"test-merge"}); err != nil {
		t.Fatalf("unexpected error running alias: %v", err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read merged output: %v", err)
	}

	var merged postman.Collection
	if err := json.Unmarshal(data, &merged); err != nil {
		t.Fatalf("failed to parse merged output: %v", err)
	}
	if len(merged.Items) != 2 {
		t.Fatalf("expected 2 merged items, got %d", len(merged.Items))
	}
}

func TestRunAlias_Dispatch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := runAlias(nil); err == nil {
		t.Fatal("expected error when no subcommand is given")
	}
	if err := runAlias([]string{"bogus-subcommand"}); err == nil {
		t.Fatal("expected error for unknown alias subcommand")
	}
}
