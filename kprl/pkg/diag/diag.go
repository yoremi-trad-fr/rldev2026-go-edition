// Package diag is the rldev-go diagnostic reporter.
//
// PLACEMENT NOTE — read before moving this file.
// The rldev-go repository currently has two go.mod files both
// declaring `module github.com/yoremi/rldev-go`: one at common/
// (the "future" layout) and one at kprl/ (the legacy layout, with
// duplicates of binarray, bytecode, encoding, etc.). Only one of
// the two can be listed in go.work without a module collision,
// and historically that's kprl/. As a consequence every other
// package in the workspace — rlc, rlxml, vaconv — resolves any
// import of `github.com/yoremi/rldev-go/pkg/X` to **kprl/pkg/X**.
// So this file lives at kprl/pkg/diag/ even though it is logically
// a common/ package. When kprl/pkg/* eventually consolidates into
// common/pkg/* this file should move with the rest.
//
// ─────────────────────────────────────────────────────────────────
//
// diag is the Go transposition of the OCaml rldev reporting layer,
// scattered across two files in the original:
//
//   - rlc/keTypes.ml         — error / warning / info (with a Loc)
//   - common/optpp.ml        — cliError / cliWarning / sysError /
//                              sysWarning / sysInfo  (no Loc)
//
// The OCaml compiler was systematically helpful: every recoverable
// glitch ("cannot represent U+2605", "stray byte 0x83 in UTF-8 file",
// "unresolved resource #res<...>") produced a line on stderr telling
// the translator the file, the line, and the offending value. The
// first Go port silently swallowed almost every one of these — a
// failed string encode emitted empty quotes, a stray Shift-JIS byte
// in a UTF-8 file degraded to U+FFFD, an unresolved #res<> emitted
// nothing — so a SEEN.TXT could "compile successfully" yet be just
// wrong enough that the engine refused to boot, with nothing in the
// log to explain it.
//
// This package restores that visibility. Every diagnostic goes to
// stderr (which the AIO GUI console captures line by line) with the
// exact OCaml wording so existing rldev documentation still applies:
//
//	Error   (SEEN0001.org line 42): invalid character 0x83
//	Warning (SEEN0428.org line 13): cannot represent U+2605 in RealLive bytecode
//	Warning: unable to locate 'gameexe.ini': using default values.
//
// All output funnels through a single io.Writer (default os.Stderr)
// so tests can capture it via SetOutput. The counters (Warnings,
// Errors) survive across calls until Reset, so the caller — the rlc
// driver loop in cmd/rlc/main.go, or the kprl archiver — can decide
// per-file whether to abort, continue, or upgrade warnings to errors
// (-Wfatal, wired in module 2).
package diag

import (
	"fmt"
	"io"
	"os"
	"sync"
)

// Loc is a source position. It mirrors OCaml keTypes.ml
//
//	type location = { file: string; line: int }
//
// kept minimal on purpose: every package in rldev-go (rlc/ast.Loc,
// kprl disassembler RlTypes.line) can convert to it with two field
// copies, and the WarnAt / ErrorAt helpers below accept raw
// (file, line) pairs for callers who don't want to allocate a Loc.
type Loc struct {
	File string
	Line int
}

// Nowhere is the synthetic location used for diagnostics emitted on
// generated code that has no real source position. Matches OCaml
// keTypes.ml `nowhere = { file = "generated code"; line = -1 }`.
var Nowhere = Loc{File: "generated code", Line: -1}

// String renders a Loc the way both OCaml functions do:
// "file line N", or just "generated code" when the line is unset.
func (l Loc) String() string {
	if l.File == "" {
		return "generated code"
	}
	if l.Line < 0 {
		return l.File
	}
	return fmt.Sprintf("%s line %d", l.File, l.Line)
}

// ────────────────────────────────────────────────────────────────
// Configuration (one process-wide reporter; matches OCaml globals)
// ────────────────────────────────────────────────────────────────

var (
	mu       sync.Mutex
	out      io.Writer = os.Stderr
	quiet    bool
	verbose  bool
	wFatal   bool // -Wfatal: treat warnings as errors (module 2)
	warnings int
	errors   int
)

// SetOutput redirects every diagnostic to w. Defaults to os.Stderr.
// Used by tests to capture output; the GUI keeps the default since
// it already pipes our stderr into its console line by line.
func SetOutput(w io.Writer) {
	mu.Lock()
	defer mu.Unlock()
	if w == nil {
		out = os.Stderr
	} else {
		out = w
	}
}

// SetQuiet suppresses Warning / Info / Phase / Sys* output. Counters
// still advance so Summary remains accurate. Errors are NEVER
// silenced — they always print and always count. Wired to rlc's -q.
func SetQuiet(q bool) {
	mu.Lock()
	defer mu.Unlock()
	quiet = q
}

// SetVerbose enables Phase logging. Wired to rlc's -v flag. Has no
// effect on Warning / Error / Info, which print regardless.
func SetVerbose(v bool) {
	mu.Lock()
	defer mu.Unlock()
	verbose = v
}

// SetWarningsFatal makes every Warning bump the error counter as
// well, so callers that check Errors() at end-of-file abort the
// compilation. Wired to rlc's -Wfatal in module 2. Off by default —
// the OCaml compiler tolerated warnings.
func SetWarningsFatal(b bool) {
	mu.Lock()
	defer mu.Unlock()
	wFatal = b
}

// Reset zeroes the counters. Call once per input file so per-file
// Summary() is accurate. Does NOT touch quiet / verbose / wFatal.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	warnings, errors = 0, 0
}

// Warnings returns the number of warnings reported since the last
// Reset. With -Wfatal active a warning also increments errors, but
// the warning counter still reflects the raw warning count.
func Warnings() int {
	mu.Lock()
	defer mu.Unlock()
	return warnings
}

// Errors returns the number of errors reported since the last
// Reset. The rlc driver loop uses this to decide the exit code.
func Errors() int {
	mu.Lock()
	defer mu.Unlock()
	return errors
}

// ────────────────────────────────────────────────────────────────
// Located diagnostics (OCaml keTypes.ml: error / warning / info)
// ────────────────────────────────────────────────────────────────

// Warning reports a non-fatal problem at a known source position and
// increments the warning counter. Compilation continues. Mirrors
// OCaml keTypes.ml `warning`:
//
//	Warning (file line N): message.
//
// With -Wfatal the error counter is bumped too so the file aborts.
func Warning(l Loc, format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	warnings++
	if wFatal {
		errors++
	}
	if quiet {
		return
	}
	fmt.Fprintf(out, "Warning (%s): %s.\n", l, fmt.Sprintf(format, args...))
}

// Errorf builds a fatal diagnostic at a known source position,
// increments the error counter, and returns the diagnostic as an
// error value so the caller can propagate it up. Mirrors OCaml
// keTypes.ml `error`, which raises:
//
//	Error (file line N): message
//
// Use the returned error with `return diag.Errorf(...)` so the
// current file aborts cleanly.
func Errorf(l Loc, format string, args ...any) error {
	mu.Lock()
	defer mu.Unlock()
	errors++
	msg := fmt.Sprintf("Error (%s): %s", l, fmt.Sprintf(format, args...))
	// Errors are never silenced — matches OCaml `cliError`, which
	// raises an exception, so the message always reaches the user.
	fmt.Fprintln(out, msg)
	return fmt.Errorf("%s", msg)
}

// Info prints a low-priority note at a known source position. No
// counter is touched. Mirrors OCaml keTypes.ml `info`:
//
//	file line N: message
func Info(l Loc, format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if quiet {
		return
	}
	fmt.Fprintf(out, "%s: %s\n", l, fmt.Sprintf(format, args...))
}

// ────────────────────────────────────────────────────────────────
// (file, line) helpers — same as Warning / Errorf / Info but accept
// raw position fields. Convenient for callers that don't already
// have a Loc on hand (lexer, encoding pass, etc.).
// ────────────────────────────────────────────────────────────────

// WarnAt is Warning for callers that hold a raw (file, line) pair.
func WarnAt(file string, line int, format string, args ...any) {
	Warning(Loc{File: file, Line: line}, format, args...)
}

// ErrorAt is Errorf for callers that hold a raw (file, line) pair.
func ErrorAt(file string, line int, format string, args ...any) error {
	return Errorf(Loc{File: file, Line: line}, format, args...)
}

// InfoAt is Info for callers that hold a raw (file, line) pair.
func InfoAt(file string, line int, format string, args ...any) {
	Info(Loc{File: file, Line: line}, format, args...)
}

// ────────────────────────────────────────────────────────────────
// Unlocated diagnostics (OCaml optpp.ml: sysError / sysWarning /
// sysInfo). Used for top-level issues that have no source position
// — missing INI file, unreadable KFN, CLI usage, etc.
// ────────────────────────────────────────────────────────────────

// SysWarning reports a global warning with no source location.
// Mirrors OCaml optpp.ml `sysWarning`:
//
//	Warning: message.
func SysWarning(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	warnings++
	if wFatal {
		errors++
	}
	if quiet {
		return
	}
	fmt.Fprintf(out, "Warning: %s.\n", fmt.Sprintf(format, args...))
}

// SysError builds a fatal global diagnostic, increments the error
// counter, and returns it as an error value. Mirrors OCaml
// optpp.ml `sysError`:
//
//	Error: message.
func SysError(format string, args ...any) error {
	mu.Lock()
	defer mu.Unlock()
	errors++
	msg := fmt.Sprintf("Error: %s", fmt.Sprintf(format, args...))
	// Errors are never silenced (see Errorf).
	fmt.Fprintln(out, msg)
	return fmt.Errorf("%s", msg)
}

// SysInfo prints a global informational message. Mirrors OCaml
// optpp.ml `sysInfo` (which itself is an alias of cliWarning):
//
//	message
func SysInfo(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if quiet {
		return
	}
	fmt.Fprintf(out, "%s\n", fmt.Sprintf(format, args...))
}

// ────────────────────────────────────────────────────────────────
// Compile-phase tracing (verbose only). Has no OCaml equivalent
// per se — OCaml used scattered `if !verbose then sysInfo "..."`.
// We centralise it here so module 2 can wire `-v` once.
// ────────────────────────────────────────────────────────────────

// Phase prints a compile-phase note ONLY when SetVerbose(true) is
// active. Typical use:
//
//	diag.Phase("lexing %s (%d bytes, encoding %s)", path, n, enc)
//	diag.Phase("parsed %d statements", len(prog.Stmts))
//	diag.Phase("generated %d bytes of bytecode", len(bc))
func Phase(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if !verbose || quiet {
		return
	}
	fmt.Fprintf(out, "  %s\n", fmt.Sprintf(format, args...))
}

// ────────────────────────────────────────────────────────────────
// End-of-file summary
// ────────────────────────────────────────────────────────────────

// Summary prints a one-line tally for the file just processed and
// returns true when at least one warning or error was reported.
// Silent (and returns false) when both counters are zero.
//
//	SEEN0001.org: 2 warning(s), 0 error(s)
//
// Callers reset counters with Reset() before the next file.
func Summary(file string) bool {
	mu.Lock()
	defer mu.Unlock()
	if warnings == 0 && errors == 0 {
		return false
	}
	if !quiet {
		fmt.Fprintf(out, "%s: %d warning(s), %d error(s)\n", file, warnings, errors)
	}
	return true
}
