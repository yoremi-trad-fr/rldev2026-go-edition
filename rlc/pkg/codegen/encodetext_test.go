package codegen

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yoremi/rldev-go/pkg/diag"
	"github.com/yoremi/rldev-go/pkg/texttransforms"
	"github.com/yoremi/rldev-go/rlc/pkg/ast"
)

// TestEncodeTextWarnsOnBadRunes is the end-to-end check for the
// module 4 fix. A translated string carrying a Hangul syllable (not
// in CP932) goes through Output.encodeText; the test asserts one
// diag.Warning is emitted with the original source Loc, mentioning
// the offending code point in OCaml wording.
func TestEncodeTextWarnsOnBadRunes(t *testing.T) {
	old := texttransforms.ForceEncode
	texttransforms.ForceEncode = true
	defer func() { texttransforms.ForceEncode = old }()

	var buf bytes.Buffer
	diag.SetOutput(&buf)
	defer diag.SetOutput(nil)
	diag.SetQuiet(false)
	diag.Reset()

	o := &Output{}
	loc := ast.Loc{File: "SEEN0001.org", Line: 42}
	_, err := o.encodeText(loc, "Hello \uACE0 World")
	if err != nil {
		t.Fatalf("encodeText errored under ForceEncode: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "Warning (SEEN0001.org line 42)") {
		t.Errorf("missing OCaml-style location prefix:\n  got: %q", got)
	}
	if !strings.Contains(got, "U+ACE0") {
		t.Errorf("missing offending code point:\n  got: %q", got)
	}
	if !strings.Contains(got, "cannot represent") {
		t.Errorf("missing OCaml wording 'cannot represent':\n  got: %q", got)
	}
	if w := diag.Warnings(); w != 1 {
		t.Errorf("expected 1 warning counted, got %d", w)
	}
}

// TestEncodeTextCleanInputSilent: pure CP932-compatible text does
// not bother the translator.
func TestEncodeTextCleanInputSilent(t *testing.T) {
	var buf bytes.Buffer
	diag.SetOutput(&buf)
	defer diag.SetOutput(nil)
	diag.Reset()

	o := &Output{}
	_, err := o.encodeText(ast.Loc{File: "x", Line: 1}, "Hello ありがとう")
	if err != nil {
		t.Fatalf("clean text errored: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("clean text produced output: %q", buf.String())
	}
	if w := diag.Warnings(); w != 0 {
		t.Errorf("clean text counted as %d warning(s)", w)
	}
}
