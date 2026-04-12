package rlbabel

import (
	"testing"

	"github.com/yoremi/rldev-go/rlc/pkg/ast"
)

// ============================================================
// Token constants
// ============================================================

func TestTokenConstants(t *testing.T) {
	// Verify token bytes match rlBabel.ml definitions
	if TokenNameLeft != 0x01 { t.Error("NameLeft") }
	if TokenNameRight != 0x02 { t.Error("NameRight") }
	if TokenBreak != 0x03 { t.Error("Break") }
	if TokenSetIndent != 0x04 { t.Error("SetIndent") }
	if TokenClearIndent != 0x05 { t.Error("ClearIndent") }
	if TokenQuote != 0x08 { t.Error("Quote") }
	if TokenEmphasis != 0x09 { t.Error("Emphasis") }
	if TokenRegular != 0x0A { t.Error("Regular") }
	if TokenBeginGloss != 0x1F { t.Error("BeginGloss") }
}

// ============================================================
// VWF config
// ============================================================

func TestDefaultVWFConfig(t *testing.T) {
	cfg := DefaultVWFConfig()
	if cfg.FStart != "__vwf_TextoutStart" { t.Errorf("FStart: %q", cfg.FStart) }
	if cfg.FAppend != "__vwf_TextoutAppend" { t.Errorf("FAppend: %q", cfg.FAppend) }
	if cfg.FDisplay != "__vwf_TextoutDisplay" { t.Errorf("FDisplay: %q", cfg.FDisplay) }
}

func TestGlossVWFConfig(t *testing.T) {
	cfg := GlossVWFConfig()
	if cfg.FStart != "__vwf_GlossTextStart" { t.Errorf("FStart: %q", cfg.FStart) }
	if cfg.FAppend != "__vwf_GlossTextAppend" { t.Errorf("FAppend: %q", cfg.FAppend) }
	if cfg.FDisplay != "__vwf_GlossTextSet" { t.Errorf("FDisplay: %q", cfg.FDisplay) }
}

// ============================================================
// Element processing
// ============================================================

func TestProcessBreak(t *testing.T) {
	elts := ProcessBreak(ast.Nowhere)
	if len(elts) != 1 { t.Fatalf("got %d elements", len(elts)) }
	if elts[0].Token != TokenBreak { t.Errorf("token: 0x%02x", elts[0].Token) }
}

func TestProcessReturn(t *testing.T) {
	// \r → clear indent (0x05) + break (0x03)
	elts := ProcessReturn(ast.Nowhere)
	if len(elts) != 2 { t.Fatalf("got %d elements", len(elts)) }
	if elts[0].Token != TokenClearIndent { t.Errorf("first: 0x%02x, want 0x05", elts[0].Token) }
	if elts[1].Token != TokenBreak { t.Errorf("second: 0x%02x, want 0x03", elts[1].Token) }
}

func TestProcessDQuote(t *testing.T) {
	elt := ProcessDQuote(ast.Nowhere)
	if elt.Token != TokenQuote { t.Errorf("token: 0x%02x", elt.Token) }
	if elt.Kind != EltDQuote { t.Error("kind") }
}

func TestProcessSpeaker(t *testing.T) {
	elt := ProcessSpeaker(ast.Nowhere)
	if elt.Token != TokenNameLeft { t.Errorf("token: 0x%02x", elt.Token) }
}

func TestProcessRCur(t *testing.T) {
	elt := ProcessRCur(ast.Nowhere)
	if elt.Token != TokenNameRight { t.Errorf("token: 0x%02x", elt.Token) }
}

func TestProcessEmphasis(t *testing.T) {
	elt := ProcessEmphasis(ast.Nowhere)
	if elt.Token != TokenEmphasis { t.Errorf("token: 0x%02x", elt.Token) }
}

func TestProcessRegular(t *testing.T) {
	elt := ProcessRegular(ast.Nowhere)
	if elt.Token != TokenRegular { t.Errorf("token: 0x%02x", elt.Token) }
}

func TestProcessAsterisk(t *testing.T) {
	elt := ProcessAsterisk(ast.Nowhere)
	if !elt.FlushAfter { t.Error("should flush after asterisk") }
}

func TestProcessPercent(t *testing.T) {
	elt := ProcessPercent(ast.Nowhere)
	if !elt.FlushAfter { t.Error("should flush after percent") }
}

func TestProcessBeginGloss(t *testing.T) {
	elt := ProcessBeginGloss(ast.Nowhere)
	if elt.Token != TokenBeginGloss { t.Errorf("token: 0x%02x", elt.Token) }
}

// ============================================================
// Emoji processing
// ============================================================

func TestProcessEmojiColor(t *testing.T) {
	params := []ast.Param{ast.SimpleParam{Expr: ast.IntLit{Val: 5}}}
	r, err := ProcessEmoji("e", params)
	if err != nil { t.Fatal(err) }
	if r.EmojiMarker != 0x06 { t.Errorf("marker: 0x%02x, want 0x06", r.EmojiMarker) }
	if !r.IsConst { t.Error("should be constant") }
	if r.IndexText != "05" { t.Errorf("index: %q", r.IndexText) }
	if r.HasSize { t.Error("should not have size") }
}

func TestProcessEmojiMono(t *testing.T) {
	params := []ast.Param{ast.SimpleParam{Expr: ast.IntLit{Val: 12}}}
	r, err := ProcessEmoji("em", params)
	if err != nil { t.Fatal(err) }
	if r.EmojiMarker != 0x07 { t.Errorf("marker: 0x%02x, want 0x07", r.EmojiMarker) }
	if r.IndexText != "12" { t.Errorf("index: %q", r.IndexText) }
}

func TestProcessEmojiWithSize(t *testing.T) {
	params := []ast.Param{
		ast.SimpleParam{Expr: ast.IntLit{Val: 3}},
		ast.SimpleParam{Expr: ast.IntLit{Val: 32}},
	}
	r, err := ProcessEmoji("e", params)
	if err != nil { t.Fatal(err) }
	if !r.HasSize { t.Error("should have size") }
	if lit, ok := r.SizeExpr.(ast.IntLit); !ok || lit.Val != 32 {
		t.Errorf("size: %v", r.SizeExpr)
	}
}

func TestProcessEmojiRuntime(t *testing.T) {
	// Non-constant index → IndexExpr set, IsConst=false
	v := ast.IntVar{Bank: 0, Index: ast.IntLit{Val: 0}}
	params := []ast.Param{ast.SimpleParam{Expr: v}}
	r, err := ProcessEmoji("e", params)
	if err != nil { t.Fatal(err) }
	if r.IsConst { t.Error("should not be constant") }
	if r.IndexExpr == nil { t.Error("IndexExpr should be set") }
}

func TestProcessEmojiNoParams(t *testing.T) {
	_, err := ProcessEmoji("e", nil)
	if err == nil { t.Error("expected error with no params") }
}

// ============================================================
// Gloss flattening
// ============================================================

func TestDefaultGlossFlattening(t *testing.T) {
	gf := DefaultGlossFlattening()
	if !gf.FlattenNested { t.Error("should flatten") }
	if !gf.WrapInParens { t.Error("should wrap") }
}

// ============================================================
// Compile options
// ============================================================

func TestDefaultCompileOptions(t *testing.T) {
	opts := DefaultCompileOptions()
	if !opts.WithKidoku { t.Error("should have kidoku") }
	if opts.VWF.FStart != "__vwf_TextoutStart" { t.Error("FStart") }
}

func TestGlossCompileOptions(t *testing.T) {
	opts := GlossCompileOptions()
	if opts.WithKidoku { t.Error("gloss should not have kidoku") }
	if opts.VWF.FStart != "__vwf_GlossTextStart" { t.Error("FStart") }
}

// ============================================================
// TextEltKind coverage
// ============================================================

func TestTextEltKindRange(t *testing.T) {
	// Verify all kinds are distinct
	kinds := []TextEltKind{
		EltText, EltDQuote, EltSpace, EltSpeaker, EltRCur,
		EltAsterisk, EltPercent, EltHyphen, EltLLentic, EltRLentic,
		EltBreak, EltReturn, EltEmphasis, EltRegular,
		EltStrVar, EltIntVar, EltEmoji, EltCode, EltName,
		EltGlossRuby, EltGloss, EltAdd,
	}
	seen := make(map[TextEltKind]bool)
	for _, k := range kinds {
		if seen[k] { t.Errorf("duplicate kind: %d", k) }
		seen[k] = true
	}
	if len(kinds) != 22 { t.Errorf("expected 22 kinds, got %d", len(kinds)) }
}
