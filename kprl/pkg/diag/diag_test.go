package diag

import (
	"bytes"
	"strings"
	"testing"
)

// captureOutput swaps the package writer for a buffer and returns a
// restore func. All tests use it instead of touching os.Stderr.
func captureOutput(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()
	buf := &bytes.Buffer{}
	SetOutput(buf)
	SetQuiet(false)
	SetVerbose(false)
	SetWarningsFatal(false)
	Reset()
	return buf, func() {
		SetOutput(nil) // restore os.Stderr
		SetQuiet(false)
		SetVerbose(false)
		SetWarningsFatal(false)
		Reset()
	}
}

// TestLocatedFormat checks the exact OCaml wording of error / warning
// / info. The wording is part of the diag contract — translators and
// existing rldev documentation rely on it.
func TestLocatedFormat(t *testing.T) {
	buf, restore := captureOutput(t)
	defer restore()

	l := Loc{File: "SEEN0001.org", Line: 42}
	Warning(l, "cannot represent U+%04X in RealLive bytecode", 0x2605)
	if got := buf.String(); got != "Warning (SEEN0001.org line 42): cannot represent U+2605 in RealLive bytecode.\n" {
		t.Errorf("Warning format wrong:\n  got: %q", got)
	}

	buf.Reset()
	err := Errorf(l, "invalid character 0x%02X", 0x83)
	want := "Error (SEEN0001.org line 42): invalid character 0x83"
	if got := buf.String(); got != want+"\n" {
		t.Errorf("Errorf stderr wrong:\n  got: %q\n want: %q", got, want+"\n")
	}
	if err == nil || err.Error() != want {
		t.Errorf("Errorf return value wrong:\n  got: %v\n want: %q", err, want)
	}

	buf.Reset()
	Info(l, "skipped 3 lines")
	if got := buf.String(); got != "SEEN0001.org line 42: skipped 3 lines\n" {
		t.Errorf("Info format wrong:\n  got: %q", got)
	}
}

func TestNowhereFormat(t *testing.T) {
	buf, restore := captureOutput(t)
	defer restore()
	Warning(Nowhere, "synthetic node")
	if got := buf.String(); got != "Warning (generated code): synthetic node.\n" {
		t.Errorf("Nowhere format wrong:\n  got: %q", got)
	}
}

// TestSysFormat checks the optpp.ml wording: no location.
func TestSysFormat(t *testing.T) {
	buf, restore := captureOutput(t)
	defer restore()

	SysWarning("unable to locate 'gameexe.ini': using default values")
	if got := buf.String(); got != "Warning: unable to locate 'gameexe.ini': using default values.\n" {
		t.Errorf("SysWarning format wrong:\n  got: %q", got)
	}

	buf.Reset()
	err := SysError("not implemented")
	want := "Error: not implemented"
	if got := buf.String(); got != want+"\n" {
		t.Errorf("SysError stderr wrong:\n  got: %q", got)
	}
	if err == nil || err.Error() != want {
		t.Errorf("SysError return wrong: %v", err)
	}

	buf.Reset()
	SysInfo("Reading INI: gameexe.ini")
	if got := buf.String(); got != "Reading INI: gameexe.ini\n" {
		t.Errorf("SysInfo format wrong:\n  got: %q", got)
	}
}

// TestCounters checks that Warning / Errorf / SysWarning / SysError
// each bump exactly one counter and Reset zeroes them.
func TestCounters(t *testing.T) {
	_, restore := captureOutput(t)
	defer restore()

	Warning(Loc{File: "a", Line: 1}, "w1")
	WarnAt("b", 2, "w2")
	SysWarning("w3")
	_ = Errorf(Loc{File: "c", Line: 3}, "e1")
	_ = ErrorAt("d", 4, "e2")
	_ = SysError("e3")

	if w, e := Warnings(), Errors(); w != 3 || e != 3 {
		t.Errorf("counters wrong: warnings=%d errors=%d (want 3,3)", w, e)
	}
	Reset()
	if w, e := Warnings(), Errors(); w != 0 || e != 0 {
		t.Errorf("Reset did not zero counters: w=%d e=%d", w, e)
	}
}

// TestQuiet: -q hides Warning / Info / SysInfo / Phase, but Errors
// still print and counters still advance.
func TestQuiet(t *testing.T) {
	buf, restore := captureOutput(t)
	defer restore()

	SetQuiet(true)
	Warning(Loc{File: "x", Line: 1}, "hidden")
	Info(Loc{File: "x", Line: 1}, "hidden")
	SysWarning("hidden")
	SysInfo("hidden")
	SetVerbose(true)
	Phase("hidden")

	if buf.Len() != 0 {
		t.Errorf("quiet mode leaked output: %q", buf.String())
	}
	if Warnings() != 2 {
		t.Errorf("quiet must still count warnings: got %d", Warnings())
	}

	_ = Errorf(Loc{File: "x", Line: 2}, "visible")
	if !strings.Contains(buf.String(), "Error (x line 2): visible") {
		t.Errorf("quiet should NOT silence errors, got: %q", buf.String())
	}
}

// TestWFatal: -Wfatal escalates warnings to error-counted, output
// still says "Warning" so the translator sees the actual problem.
func TestWFatal(t *testing.T) {
	buf, restore := captureOutput(t)
	defer restore()

	SetWarningsFatal(true)
	Warning(Loc{File: "x", Line: 1}, "treat as error")
	SysWarning("ditto")

	if !strings.Contains(buf.String(), "Warning (x line 1)") {
		t.Errorf("-Wfatal must keep Warning wording, got: %q", buf.String())
	}
	if Warnings() != 2 {
		t.Errorf("warnings counter must reflect raw count: %d", Warnings())
	}
	if Errors() != 2 {
		t.Errorf("-Wfatal must escalate to error count: %d", Errors())
	}
}

// TestPhaseVerbose: Phase prints only with -v, indented 2 spaces.
func TestPhaseVerbose(t *testing.T) {
	buf, restore := captureOutput(t)
	defer restore()

	Phase("hidden without -v")
	if buf.Len() != 0 {
		t.Errorf("Phase leaked without verbose: %q", buf.String())
	}

	SetVerbose(true)
	Phase("lexing %s (%d bytes)", "SEEN0001.org", 1234)
	if got := buf.String(); got != "  lexing SEEN0001.org (1234 bytes)\n" {
		t.Errorf("Phase format wrong:\n  got: %q", got)
	}
}

// TestSummary prints only when something happened.
func TestSummary(t *testing.T) {
	buf, restore := captureOutput(t)
	defer restore()

	if Summary("clean.org") {
		t.Errorf("Summary returned true with zero counters")
	}
	if buf.Len() != 0 {
		t.Errorf("Summary printed with zero counters: %q", buf.String())
	}

	Warning(Loc{File: "x", Line: 1}, "w")
	_ = Errorf(Loc{File: "x", Line: 2}, "e")
	buf.Reset()
	if !Summary("SEEN0001.org") {
		t.Errorf("Summary returned false with non-zero counters")
	}
	if got := buf.String(); got != "SEEN0001.org: 1 warning(s), 1 error(s)\n" {
		t.Errorf("Summary format wrong: %q", got)
	}
}
