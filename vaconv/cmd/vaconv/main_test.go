package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandInputFilesFiltersDirectoryByFormat(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.g00", "b.G00", "c.png"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "nested.g00"), 0755); err != nil {
		t.Fatal(err)
	}

	files, err := expandInputFiles([]string{dir}, "g00")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 G00 files, got %d: %v", len(files), files)
	}
	for _, file := range files {
		if !strings.EqualFold(filepath.Ext(file), ".g00") {
			t.Fatalf("expected only .g00 files, got %s", file)
		}
	}
}

func TestExpandInputFilesReportsEmptyDirectoryForFormat(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.png"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := expandInputFiles([]string{dir}, "g00")
	if err == nil {
		t.Fatal("expected error for directory without matching files")
	}
	if !strings.Contains(err.Error(), ".g00") {
		t.Fatalf("expected error to mention .g00, got %v", err)
	}
}

func TestBatchJobsClampsRequestedValue(t *testing.T) {
	if got := batchJobs(3, 10); got != 3 {
		t.Fatalf("batchJobs clamps to file count = %d, want 3", got)
	}
	if got := batchJobs(3, 1); got != 1 {
		t.Fatalf("batchJobs explicit sequential = %d, want 1", got)
	}
	if got := batchJobs(1, 0); got != 1 {
		t.Fatalf("batchJobs single file = %d, want 1", got)
	}
}
