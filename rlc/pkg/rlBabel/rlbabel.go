// Package rlbabel implements the rlBabel text compilation system for
// multi-language RealLive games.
//
// Transposed from OCaml's rlc/rlBabel.ml (260 lines).
//
// rlBabel is an alternative text output system that supports:
//   - Variable-width font rendering (VWF) via __vwf_* runtime functions
//   - Multi-language text encoding (Latin, CJK, etc.)
//   - Dynamic glosses (hover text / tooltips)
//   - Emoji with optional font size changes
//   - Text emphasis (\b, \u) and indentation control
//
// Unlike the standard textout system which encodes text as raw Shift_JIS
// bytes, rlBabel builds text as a series of string tokens passed to VWF
// runtime functions:
//   - __vwf_TextoutStart(str): begin a new text block
//   - __vwf_TextoutAppend(str): append to current text block
//   - __vwf_TextoutDisplay(str): display the text block
//
// Special token bytes embedded in the text strings:
//   0x01 = name block open (left bracket)
//   0x02 = name block close (right bracket)
//   0x03 = line break
//   0x04 = set indent
//   0x05 = clear indent
//   0x06-0x07 = exfont markers (reserved)
//   0x08 = double quote
//   0x09 = emphasis on
//   0x0A = emphasis off (regular)
//   0x1F = begin gloss
package rlbabel

import (
	"fmt"

	"github.com/yoremi/rldev-go/rlc/pkg/ast"
)

// ============================================================
// Token bytes (from rlBabel.ml lines 29-39)
// ============================================================

// Special token bytes embedded in rlBabel text strings.
const (
	TokenNameLeft    byte = 0x01 // \{} name block open
	TokenNameRight   byte = 0x02 // } name block close
	TokenBreak       byte = 0x03 // \n line break
	TokenSetIndent   byte = 0x04 // set indent position
	TokenClearIndent byte = 0x05 // \r clear indent + break
	// 0x06, 0x07 reserved for exfont markers
	TokenQuote       byte = 0x08 // " double quote
	TokenEmphasis    byte = 0x09 // \b emphasis on
	TokenRegular     byte = 0x0A // \u emphasis off
	// 0x0B-0x1E available
	TokenBeginGloss  byte = 0x1F // \g{} begin gloss
)

// ============================================================
// VWF function names (from rlBabel.ml compile defaults)
// ============================================================

// VWFConfig holds the runtime function names for VWF text output.
type VWFConfig struct {
	FStart   string // function to begin a text block
	FAppend  string // function to append to text block
	FDisplay string // function to display text block
}

// DefaultVWFConfig returns the standard VWF function names.
func DefaultVWFConfig() VWFConfig {
	return VWFConfig{
		FStart:   "__vwf_TextoutStart",
		FAppend:  "__vwf_TextoutAppend",
		FDisplay: "__vwf_TextoutDisplay",
	}
}

// GlossVWFConfig returns VWF function names for gloss (tooltip) text.
func GlossVWFConfig() VWFConfig {
	return VWFConfig{
		FStart:   "__vwf_GlossTextStart",
		FAppend:  "__vwf_GlossTextAppend",
		FDisplay: "__vwf_GlossTextSet",
	}
}

// ============================================================
// Text element classification
// ============================================================

// TextEltKind classifies a text element for rlBabel compilation.
type TextEltKind int

const (
	EltText      TextEltKind = iota // plain text, pass through
	EltDQuote                        // " → token 0x08
	EltSpace                         // whitespace
	EltSpeaker                       // \{} name open → token 0x01
	EltRCur                          // } name close → token 0x02
	EltAsterisk                      // ＊ → flush after
	EltPercent                       // ％ → flush after
	EltHyphen                        // - pass through
	EltLLentic                       // 【 pass through
	EltRLentic                       // 】 pass through
	EltBreak                         // \n → token 0x03
	EltReturn                        // \r → tokens 0x05 + 0x03
	EltEmphasis                      // \b → token 0x09
	EltRegular                       // \u → token 0x0A
	EltStrVar                        // \s{var} → flush + append var
	EltIntVar                        // \i{expr} → flush + itoa + append
	EltEmoji                         // \e{idx} / \em{idx}
	EltCode                          // other control code → flush + compile
	EltName                          // name reference
	EltGlossRuby                     // \ruby{} (warning: not implemented)
	EltGloss                         // \g{}{} gloss/tooltip
	EltAdd                           // \a{} additional string
)

// ============================================================
// Compiled text element
// ============================================================

// CompiledElt represents one processed text element in the rlBabel pipeline.
type CompiledElt struct {
	Kind     TextEltKind
	Loc      ast.Loc
	Token    byte      // for token-based elements
	Text     string    // for plain text
	Expr     ast.Expr  // for variable references
	FlushAfter bool    // element requires a flush after emission
}

// ============================================================
// rlBabel text processing
// ============================================================

// ProcessBreak creates the rlBabel elements for \n (line break).
// Emits token 0x03 (break).
func ProcessBreak(loc ast.Loc) []CompiledElt {
	return []CompiledElt{
		{Kind: EltBreak, Loc: loc, Token: TokenBreak},
	}
}

// ProcessReturn creates the rlBabel elements for \r (return).
// Emits token 0x05 (clear indent) then 0x03 (break).
func ProcessReturn(loc ast.Loc) []CompiledElt {
	return []CompiledElt{
		{Kind: EltReturn, Loc: loc, Token: TokenClearIndent},
		{Kind: EltBreak, Loc: loc, Token: TokenBreak},
	}
}

// ProcessDQuote creates the rlBabel element for a double quote.
// Emits token 0x08.
func ProcessDQuote(loc ast.Loc) CompiledElt {
	return CompiledElt{Kind: EltDQuote, Loc: loc, Token: TokenQuote}
}

// ProcessSpeaker creates the rlBabel element for \{} (name block open).
// Emits token 0x01.
func ProcessSpeaker(loc ast.Loc) CompiledElt {
	return CompiledElt{Kind: EltSpeaker, Loc: loc, Token: TokenNameLeft}
}

// ProcessRCur creates the rlBabel element for } (name block close).
// Emits token 0x02.
func ProcessRCur(loc ast.Loc) CompiledElt {
	return CompiledElt{Kind: EltRCur, Loc: loc, Token: TokenNameRight}
}

// ProcessEmphasis creates the rlBabel element for \b (emphasis on).
// Emits token 0x09.
func ProcessEmphasis(loc ast.Loc) CompiledElt {
	return CompiledElt{Kind: EltEmphasis, Loc: loc, Token: TokenEmphasis}
}

// ProcessRegular creates the rlBabel element for \u (emphasis off).
// Emits token 0x0A.
func ProcessRegular(loc ast.Loc) CompiledElt {
	return CompiledElt{Kind: EltRegular, Loc: loc, Token: TokenRegular}
}

// ProcessAsterisk creates the rlBabel element for ＊.
// Needs flush after to prevent next char from forming a name.
func ProcessAsterisk(loc ast.Loc) CompiledElt {
	return CompiledElt{Kind: EltAsterisk, Loc: loc, FlushAfter: true}
}

// ProcessPercent creates the rlBabel element for ％.
// Needs flush after to prevent next char from forming a name.
func ProcessPercent(loc ast.Loc) CompiledElt {
	return CompiledElt{Kind: EltPercent, Loc: loc, FlushAfter: true}
}

// ProcessBeginGloss creates the token for starting a gloss block.
// Emits token 0x1F.
func ProcessBeginGloss(loc ast.Loc) CompiledElt {
	return CompiledElt{Kind: EltGloss, Loc: loc, Token: TokenBeginGloss}
}

// ============================================================
// Emoji processing (from rlBabel.ml lines 146-168)
// ============================================================

// EmojiResult holds the processing result for \e{} / \em{}.
type EmojiResult struct {
	// EmojiMarker is the DBCS marker byte: 0x06 for \e (colour), 0x07 for \em (mono)
	EmojiMarker byte
	// IndexText is the 2-digit index string (for constant indices)
	IndexText string
	// IndexExpr is the index expression (for runtime indices)
	IndexExpr ast.Expr
	// IsConst is true if the index was folded to a constant
	IsConst bool
	// HasSize is true if a font size parameter was provided
	HasSize bool
	// SizeExpr is the font size expression (if HasSize)
	SizeExpr ast.Expr
}

// ProcessEmoji processes an \e{idx} or \em{idx} control code.
// The emoji marker byte is 0x06 for \e (with colour) or 0x07 for \em (mono).
// This matches the OCaml: Text.length id = 1 → '\x60' (0x06), else '\x61' (0x07)
// which encodes as DBCS char (5 + length of identifier).
func ProcessEmoji(code string, params []ast.Param) (*EmojiResult, error) {
	if len(params) < 1 {
		return nil, fmt.Errorf("\\%s{} requires at least one parameter", code)
	}
	result := &EmojiResult{}

	// \e → marker 0x06 (5 + len("e")=1 = 6), \em → marker 0x07 (5 + len("em")=2 = 7)
	if code == "e" {
		result.EmojiMarker = 0x06
	} else {
		result.EmojiMarker = 0x07
	}

	// Extract index
	if sp, ok := params[0].(ast.SimpleParam); ok {
		if lit, ok := sp.Expr.(ast.IntLit); ok {
			result.IsConst = true
			result.IndexText = fmt.Sprintf("%02d", lit.Val)
		} else {
			result.IndexExpr = sp.Expr
		}
	}

	// Extract optional size
	if len(params) >= 2 {
		if sp, ok := params[1].(ast.SimpleParam); ok {
			result.HasSize = true
			result.SizeExpr = sp.Expr
		}
	}

	return result, nil
}

// ============================================================
// Gloss flattening (from rlBabel.ml lines 41-58)
// ============================================================

// FlattenNestedGlosses handles the case where glosses are nested.
// Nested glosses are flattened with brackets:
//
//	\g{foo}={bar \g{baz}={yomuna} quux}
//	  → \g{foo}={bar baz (yomuna) quux}
//
// This is necessary because the dynamic gloss runtime code doesn't
// handle nested glosses. The tokens are modified in-place.
//
// In the Go version, this is represented as a description of the
// transformation rather than operating on DynArrays directly,
// since the full text token types aren't available here.
type GlossFlattening struct {
	// FlattenNested indicates that nested \g{} should be flattened
	FlattenNested bool
	// WrapInParens indicates nested gloss content should be wrapped in ()
	WrapInParens bool
}

// DefaultGlossFlattening returns the standard flattening config.
func DefaultGlossFlattening() GlossFlattening {
	return GlossFlattening{
		FlattenNested: true,
		WrapInParens:  true,
	}
}

// ============================================================
// Compile options
// ============================================================

// CompileOptions controls rlBabel text compilation behavior.
type CompileOptions struct {
	WithKidoku bool      // emit kidoku marker (true for top-level, false for gloss)
	VWF        VWFConfig // VWF runtime function names
}

// DefaultCompileOptions returns options for standard text compilation.
func DefaultCompileOptions() CompileOptions {
	return CompileOptions{
		WithKidoku: true,
		VWF:        DefaultVWFConfig(),
	}
}

// GlossCompileOptions returns options for gloss text compilation.
func GlossCompileOptions() CompileOptions {
	return CompileOptions{
		WithKidoku: false,
		VWF:        GlossVWFConfig(),
	}
}


