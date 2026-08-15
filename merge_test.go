package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"postOffice/internal/postman"
	"testing"
)

func TestParseMergeArgs(t *testing.T) {
	t.Run("valid forceMerge", func(t *testing.T) {
		opts, err := parseMergeArgs([]string{"a.json", "b.json", "--forceMerge"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if opts.file1 != "a.json" || opts.file2 != "b.json" {
			t.Errorf("expected files a.json/b.json, got %s/%s", opts.file1, opts.file2)
		}
		if opts.strategy != postman.MergeForce {
			t.Errorf("expected MergeForce strategy")
		}
		if opts.output != "" {
			t.Errorf("expected empty output, got %s", opts.output)
		}
	})

	t.Run("valid keep-original with output flag", func(t *testing.T) {
		opts, err := parseMergeArgs([]string{"a.json", "b.json", "--keep-original", "-o", "merged.json"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if opts.strategy != postman.MergeKeepOriginal {
			t.Errorf("expected MergeKeepOriginal strategy")
		}
		if opts.output != "merged.json" {
			t.Errorf("expected output merged.json, got %s", opts.output)
		}
	})

	t.Run("flags interleaved with files", func(t *testing.T) {
		opts, err := parseMergeArgs([]string{"--forceMerge", "a.json", "-o", "out.json", "b.json"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if opts.file1 != "a.json" || opts.file2 != "b.json" {
			t.Errorf("expected files a.json/b.json, got %s/%s", opts.file1, opts.file2)
		}
	})

	t.Run("missing files", func(t *testing.T) {
		if _, err := parseMergeArgs([]string{"--forceMerge"}); err == nil {
			t.Fatal("expected error for missing files")
		}
	})

	t.Run("too many files", func(t *testing.T) {
		if _, err := parseMergeArgs([]string{"a.json", "b.json", "c.json", "--forceMerge"}); err == nil {
			t.Fatal("expected error for too many files")
		}
	})

	t.Run("neither strategy flag specified", func(t *testing.T) {
		if _, err := parseMergeArgs([]string{"a.json", "b.json"}); err == nil {
			t.Fatal("expected error when neither --forceMerge nor --keep-original is given")
		}
	})

	t.Run("both strategy flags specified", func(t *testing.T) {
		if _, err := parseMergeArgs([]string{"a.json", "b.json", "--forceMerge", "--keep-original"}); err == nil {
			t.Fatal("expected error when both --forceMerge and --keep-original are given")
		}
	})

	t.Run("unknown flag", func(t *testing.T) {
		if _, err := parseMergeArgs([]string{"a.json", "b.json", "--forceMerge", "--bogus"}); err == nil {
			t.Fatal("expected error for unknown flag")
		}
	})

	t.Run("output flag missing value", func(t *testing.T) {
		if _, err := parseMergeArgs([]string{"a.json", "b.json", "--forceMerge", "-o"}); err == nil {
			t.Fatal("expected error for -o with no value")
		}
	})
}

func TestRunMerge_Collections(t *testing.T) {
	dir := t.TempDir()
	file1 := filepath.Join(dir, "a.json")
	file2 := filepath.Join(dir, "b.json")

	col1 := `{"info":{"name":"A","schema":"https://schema.getpostman.com/json/collection/v2.1.0/collection.json"},"item":[{"name":"Get Users","request":{"method":"GET","url":{"raw":"https://a.example.com/users"}}}]}`
	col2 := `{"info":{"name":"B","schema":"https://schema.getpostman.com/json/collection/v2.1.0/collection.json"},"item":[{"name":"Get Orders","request":{"method":"GET","url":{"raw":"https://b.example.com/orders"}}}]}`

	if err := os.WriteFile(file1, []byte(col1), 0644); err != nil {
		t.Fatalf("failed to write file1: %v", err)
	}
	if err := os.WriteFile(file2, []byte(col2), 0644); err != nil {
		t.Fatalf("failed to write file2: %v", err)
	}

	if err := runMerge([]string{file1, file2, "--forceMerge"}); err != nil {
		t.Fatalf("runMerge failed: %v", err)
	}

	data, err := os.ReadFile(file1)
	if err != nil {
		t.Fatalf("failed to read merged output: %v", err)
	}

	var merged postman.Collection
	if err := json.Unmarshal(data, &merged); err != nil {
		t.Fatalf("failed to parse merged output: %v", err)
	}

	if merged.Info.Name != "A" {
		t.Errorf("expected merged collection to keep dst name A, got %s", merged.Info.Name)
	}
	if len(merged.Items) != 2 {
		t.Fatalf("expected 2 merged items, got %d", len(merged.Items))
	}
}

func TestRunMerge_Environments(t *testing.T) {
	dir := t.TempDir()
	file1 := filepath.Join(dir, "env1.json")
	file2 := filepath.Join(dir, "env2.json")
	outFile := filepath.Join(dir, "merged-env.json")

	env1 := `{"id":"1","name":"Dst Env","values":[{"key":"host","value":"dst-host","enabled":true}]}`
	env2 := `{"id":"2","name":"Src Env","values":[{"key":"host","value":"src-host","enabled":true},{"key":"token","value":"abc","enabled":true}]}`

	if err := os.WriteFile(file1, []byte(env1), 0644); err != nil {
		t.Fatalf("failed to write file1: %v", err)
	}
	if err := os.WriteFile(file2, []byte(env2), 0644); err != nil {
		t.Fatalf("failed to write file2: %v", err)
	}

	if err := runMerge([]string{file1, file2, "--keep-original", "-o", outFile}); err != nil {
		t.Fatalf("runMerge failed: %v", err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read merged output: %v", err)
	}

	var merged postman.Environment
	if err := json.Unmarshal(data, &merged); err != nil {
		t.Fatalf("failed to parse merged output: %v", err)
	}

	if merged.Name != "Dst Env" {
		t.Errorf("expected merged environment to keep dst name, got %s", merged.Name)
	}
	if len(merged.Values) != 2 {
		t.Fatalf("expected 2 merged values, got %d", len(merged.Values))
	}
	if merged.Values[0].Value != "dst-host" {
		t.Errorf("expected keep-original to preserve dst-host, got %s", merged.Values[0].Value)
	}

	if _, err := os.Stat(file1); err != nil {
		t.Errorf("expected original file1 to remain untouched: %v", err)
	}
}

func TestRunMerge_MismatchedTypes(t *testing.T) {
	dir := t.TempDir()
	collectionFile := filepath.Join(dir, "col.json")
	envFile := filepath.Join(dir, "env.json")

	col := `{"info":{"name":"A"},"item":[]}`
	env := `{"id":"1","name":"Env","values":[]}`

	if err := os.WriteFile(collectionFile, []byte(col), 0644); err != nil {
		t.Fatalf("failed to write collection file: %v", err)
	}
	if err := os.WriteFile(envFile, []byte(env), 0644); err != nil {
		t.Fatalf("failed to write env file: %v", err)
	}

	err := runMerge([]string{collectionFile, envFile, "--forceMerge"})
	if err == nil {
		t.Fatal("expected error when merging a collection with an environment")
	}
}
