package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveBabelDLLName(t *testing.T) {
	tests := []struct {
		version string
		mode    string
		want    string
	}{
		{version: "1.2.4.8", mode: "auto", want: "rlBabelF.dll"},
		{version: "1.2.5.4", mode: "auto", want: "rlBabel.dll"},
		{version: "1.3.1.0", mode: "old", want: "rlBabelF.dll"},
		{version: "1.2.3.0", mode: "new", want: "rlBabel.dll"},
	}
	for _, tt := range tests {
		if got := resolveBabelDLLName(tt.version, tt.mode); got != tt.want {
			t.Fatalf("resolveBabelDLLName(%q, %q) = %q, want %q", tt.version, tt.mode, got, tt.want)
		}
	}
}

func TestUpdateBabelGameexeAddsDLLAndNameEnc(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "GAMEEXE.INI")
	original := "#DLL.000 = \"Existing\"\r\n#NAME_ENC = 1\r\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	if err := updateBabelGameexe(path, "rlBabel.dll", "western"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "#DLL.001 = \"rlBabel\"") {
		t.Fatalf("expected rlBabel DLL slot, got:\n%s", text)
	}
	if !strings.Contains(text, "#NAME_ENC = 2") {
		t.Fatalf("expected western NAME_ENC, got:\n%s", text)
	}

	backups, err := filepath.Glob(filepath.Join(dir, "GAMEEXE.INI.babel-*.bak"))
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("expected one backup, got %d", len(backups))
	}
}

func TestUpdateBabelGameexeOldDLLDoesNotAddDLLSlot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "GAMEEXE.INI")
	if err := os.WriteFile(path, []byte("#DLL.000 = \"Existing\"\r\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := updateBabelGameexe(path, "rlBabelF.dll", "none"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "rlBabel") {
		t.Fatalf("old DLL mode should not add a #DLL line, got:\n%s", data)
	}
}

func TestRldevBabelPrepareRuntimeCopiesFiles(t *testing.T) {
	dir := t.TempDir()
	babelRoot := filepath.Join(dir, "BABEL")
	rtlDir := filepath.Join(babelRoot, "rtl")
	if err := os.MkdirAll(rtlDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"rlBabel.dll", "rlBabelF.dll", "1.3.1.0.map"} {
		if err := os.WriteFile(filepath.Join(rtlDir, name), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}

	gameDir := filepath.Join(dir, "game")
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		t.Fatal(err)
	}
	gameexe := filepath.Join(gameDir, "GAMEEXE.INI")
	if err := os.WriteFile(gameexe, []byte("#DLL.000 = \"Existing\"\r\n"), 0644); err != nil {
		t.Fatal(err)
	}

	app := NewApp()
	if got := app.RldevBabelPrepareRuntime(babelRoot, gameDir, "1.3.1.0", "auto", "western", true); got != "" {
		t.Fatalf("RldevBabelPrepareRuntime returned %q", got)
	}
	for _, name := range []string{"rlBabel.dll", "1.3.1.0.map"} {
		if _, err := os.Stat(filepath.Join(gameDir, name)); err != nil {
			t.Fatalf("expected copied %s: %v", name, err)
		}
	}
	data, err := os.ReadFile(gameexe)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "#DLL.001 = \"rlBabel\"") || !strings.Contains(text, "#NAME_ENC = 2") {
		t.Fatalf("expected Babel GAMEEXE lines, got:\n%s", text)
	}
}

func TestRldevBabelWriteHeader(t *testing.T) {
	dir := t.TempDir()
	app := NewApp()
	if got := app.RldevBabelWriteHeader(dir, true); got != "" {
		t.Fatalf("RldevBabelWriteHeader returned %q", got)
	}
	data, err := os.ReadFile(filepath.Join(dir, "global.kh"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"#define __DynamicLineation__ = 1", "#define __EnableGlosses__", "#load 'rlBabel'"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in global.kh, got:\n%s", want, text)
		}
	}
}
