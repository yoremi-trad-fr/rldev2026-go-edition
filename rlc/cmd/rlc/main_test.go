package main

import (
	"testing"

	"github.com/yoremi/rldev-go/rlc/pkg/kfn"
)

func TestDefaultOptions(t *testing.T) {
	o := DefaultOptions()
	if o.KfnFile != "reallive.kfn" { t.Error("KfnFile") }
	if o.GameID != "LB" { t.Error("GameID") }
	if o.SrcExt != "org" { t.Error("SrcExt") }
	if o.Encoding != "CP932" { t.Error("Encoding") }
	if o.StartLine != -1 { t.Error("StartLine") }
	if o.OptLevel != 1 { t.Error("OptLevel") }
	if !o.Compress { t.Error("Compress") }
	if !o.WithRtl { t.Error("WithRtl") }
}

func TestParseFlagsBasic(t *testing.T) {
	opts, err := parseFlags([]string{"-v", "test.org"})
	if err != nil { t.Fatal(err) }
	if opts.Verbose != 1 { t.Errorf("verbose: %d", opts.Verbose) }
	if len(opts.InputFiles) != 1 || opts.InputFiles[0] != "test.org" {
		t.Errorf("input: %v", opts.InputFiles)
	}
}

func TestParseFlagsMultiVerbose(t *testing.T) {
	opts, err := parseFlags([]string{"-v", "-v", "-v", "test.org"})
	if err != nil { t.Fatal(err) }
	if opts.Verbose != 3 { t.Errorf("verbose: %d", opts.Verbose) }
}

func TestParseFlagsTarget(t *testing.T) {
	opts, err := parseFlags([]string{"-target", "Kinetic", "test.org"})
	if err != nil { t.Fatal(err) }
	if opts.Target != "Kinetic" { t.Error("target") }
	if !opts.TargetForced { t.Error("TargetForced should be true") }
}

func TestParseFlagsOutput(t *testing.T) {
	opts, err := parseFlags([]string{"-o", "out.seen", "-d", "/tmp", "test.org"})
	if err != nil { t.Fatal(err) }
	if opts.OutFile != "out.seen" { t.Errorf("OutFile: %q", opts.OutFile) }
	if opts.OutDir != "/tmp" { t.Errorf("OutDir: %q", opts.OutDir) }
}

func TestParseTarget(t *testing.T) {
	tests := []struct{ in string; want kfn.Target; err bool }{
		{"RealLive", kfn.TargetRealLive, false},
		{"reallive", kfn.TargetRealLive, false},
		{"AVG2000", kfn.TargetAVG2000, false},
		{"avg2000", kfn.TargetAVG2000, false},
		{"Kinetic", kfn.TargetKinetic, false},
		{"", kfn.TargetRealLive, false},
		{"bogus", 0, true},
	}
	for _, tt := range tests {
		got, err := parseTarget(tt.in)
		if tt.err {
			if err == nil { t.Errorf("parseTarget(%q): expected error", tt.in) }
			continue
		}
		if err != nil { t.Errorf("parseTarget(%q): %v", tt.in, err); continue }
		if got != tt.want { t.Errorf("parseTarget(%q) = %v, want %v", tt.in, got, tt.want) }
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct{ in string; want kfn.Version; err bool }{
		{"", kfn.Version{1, 2, 7, 0}, false},
		{"1", kfn.Version{1, 0, 0, 0}, false},
		{"1.2", kfn.Version{1, 2, 0, 0}, false},
		{"1.2.7", kfn.Version{1, 2, 7, 0}, false},
		{"1.2.7.5", kfn.Version{1, 2, 7, 5}, false},
		{"1.2.7.5.8", kfn.Version{}, true},
		{"x.y.z", kfn.Version{}, true},
	}
	for _, tt := range tests {
		got, err := parseVersion(tt.in)
		if tt.err {
			if err == nil { t.Errorf("parseVersion(%q): expected error", tt.in) }
			continue
		}
		if err != nil { t.Errorf("parseVersion(%q): %v", tt.in, err); continue }
		if got != tt.want { t.Errorf("parseVersion(%q) = %v, want %v", tt.in, got, tt.want) }
	}
}

func TestResolveSourcePath(t *testing.T) {
	opts := DefaultOptions()
	if resolveSourcePath(opts, "file.org") != "file.org" { t.Error("with .org") }
	if resolveSourcePath(opts, "file") != "file.org" { t.Error("no ext") }
	if resolveSourcePath(opts, "file.txt") != "file.txt" { t.Error("other ext") }
	opts.SrcExt = "kep"
	if resolveSourcePath(opts, "file") != "file.kep" { t.Error("custom ext") }
}

func TestVerboseCounter(t *testing.T) {
	var v verboseCounter = 0
	v.Set("")
	v.Set("")
	v.Set("")
	if int(v) != 3 { t.Errorf("got %d", v) }
	if !v.IsBoolFlag() { t.Error("should be bool flag") }
}
