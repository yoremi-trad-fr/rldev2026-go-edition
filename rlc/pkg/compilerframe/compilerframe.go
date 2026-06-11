// Package compilerframe implements the main compiler orchestration for Kepago.
//
// Transposed from OCaml's rlc/compilerFrame.ml (1315 lines).
//
// This is the top-level driver tying together all backend packages:
//
//	lexer/parser → AST → compilerframe → codegen → .seen bytecode
//
// Entry points:
//   - New(reg, ini)    — create a fully-wired compiler
//   - Compile(stmts)   — compile a program
//   - Parse(stmts)     — process statements (also called recursively via meta)
//   - ParseElt(stmt)   — dispatch one statement
//   - ParseNormElt(s)  — dispatch one normalized statement
package compilerframe

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/yoremi/rldev-go/pkg/diag"
	"github.com/yoremi/rldev-go/pkg/encoding"
	"github.com/yoremi/rldev-go/pkg/text"
	"github.com/yoremi/rldev-go/pkg/texttransforms"
	"github.com/yoremi/rldev-go/rlc/pkg/ast"
	"github.com/yoremi/rldev-go/rlc/pkg/codegen"
	"github.com/yoremi/rldev-go/rlc/pkg/directive"
	"github.com/yoremi/rldev-go/rlc/pkg/expr"
	fn "github.com/yoremi/rldev-go/rlc/pkg/function"
	gotojmp "github.com/yoremi/rldev-go/rlc/pkg/goto"
	"github.com/yoremi/rldev-go/rlc/pkg/ini"
	"github.com/yoremi/rldev-go/rlc/pkg/intrinsic"
	"github.com/yoremi/rldev-go/rlc/pkg/kfn"
	"github.com/yoremi/rldev-go/rlc/pkg/lexer"
	"github.com/yoremi/rldev-go/rlc/pkg/memory"
	"github.com/yoremi/rldev-go/rlc/pkg/meta"
	"github.com/yoremi/rldev-go/rlc/pkg/parser"
	"github.com/yoremi/rldev-go/rlc/pkg/sel"
)

// Compiler holds all compiler state and wires the sub-packages together.
type Compiler struct {
	Mem       *memory.Memory
	Out       *codegen.Output
	Norm      *expr.Normalizer
	Directive *directive.Compiler
	Intrin    *intrinsic.Registry
	Reg       *kfn.Registry
	Ini       *ini.Table
	State     *meta.State

	breakStack         []string // break targets (label idents)
	continueStack      []string // continue targets (label idents)
	noLineForNextPause bool
	afterEOFMarker     bool
	rlBabelLoaded      bool
	rlBabelUsed        bool
	rlBabelRuntimeDone bool
	rlBabelInitDone    bool
	rlBabelVars        *rlBabelRuntimeVars

	// EmitEOFMarkers controls whether the source-level `eof` marker writes
	// the SeenEnd trailer. The CLI wires this to debug-info generation.
	EmitEOFMarkers bool
	SeenEndEmitted bool

	Errors   []error
	Warnings []string
	Verbose  int
}

const seenEndTrailer = "" +
	"\x82\x72\x82\x85\x82\x85\x82\x8e\x82\x64\x82\x8e\x82\x84" +
	"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff" +
	"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff"

// SeenEndTrailerBytes returns the canonical RealLive SeenEnd marker. Disasm
// writes this marker as `eof`; keeping it source-addressable preserves scripts
// that intentionally place a final `halt` after the trailer.
func SeenEndTrailerBytes() []byte {
	return []byte(seenEndTrailer)
}

// New creates a fully-wired Compiler.
func New(reg *kfn.Registry, iniTable *ini.Table) *Compiler {
	mem := memory.New()
	out := codegen.NewOutput()
	norm := expr.NewNormalizer(mem)
	state := meta.NewState()

	c := &Compiler{
		Mem: mem, Out: out, Norm: norm,
		Intrin: intrinsic.New(mem),
		Reg:    reg, Ini: iniTable, State: state,
		EmitEOFMarkers: true,
	}
	out.NativeSpeakerTags = iniTable.HasNamae()
	c.Directive = &directive.Compiler{
		Mem: mem, Norm: norm, Output: out,
		Ini: iniTable, State: state,
	}
	// Wire meta callback (OCaml: Global.compilerFrame__parse)
	state.CompileStatements = c.Parse
	return c
}

// ============================================================
// Top-level entry points
// ============================================================

// Compile compiles a full program and merges sub-compiler diagnostics.
func (c *Compiler) Compile(stmts []ast.Stmt) {
	c.Parse(stmts)
	if c.rlBabelUsed && !c.rlBabelRuntimeDone {
		c.emitRLBabelRuntime(ast.Nowhere)
	}
	if c.Directive != nil {
		c.Errors = append(c.Errors, c.Directive.Errors...)
		c.Warnings = append(c.Warnings, c.Directive.Warnings...)
	}
}

// Parse processes a batch of statements. Called recursively from
// meta.State.Parse via the CompileStatements callback.
func (c *Compiler) Parse(stmts []ast.Stmt) {
	if containsKidokuDirective(stmts) {
		c.Out.SuppressAutoKidoku = true
	}
	if containsCompactLineDirective(stmts) {
		c.Out.SuppressAutoLineRefs = true
	}
	for i, s := range stmts {
		if isKidokuDirective(s) {
			c.ParseElt(s)
			if !nextStmtConsumesKidoku(stmts, i+1) {
				c.Out.AddCodeRaw(s.StmtLoc(), []byte{'"', '"'})
			}
			continue
		}
		c.ParseElt(s)
	}
}

func isDirectiveNamed(stmt ast.Stmt, name string) bool {
	d, ok := stmt.(ast.DirectiveStmt)
	return ok && d.Name == name
}

func isKidokuDirective(stmt ast.Stmt) bool {
	d, ok := stmt.(ast.DirectiveStmt)
	return ok && (d.Name == "kidoku" || d.Name == "kidoku_line")
}

func isLineDirective(stmt ast.Stmt) bool {
	return isDirectiveNamed(stmt, "line") || isDirectiveNamed(stmt, "line_compact")
}

func nextStmtConsumesKidoku(stmts []ast.Stmt, start int) bool {
	for i := start; i < len(stmts); i++ {
		if isLineDirective(stmts[i]) {
			continue
		}
		switch s := stmts[i].(type) {
		case ast.ReturnStmt:
			return !s.Explicit && s.Expr != nil
		case ast.SelectStmt:
			return true
		default:
			return false
		}
	}
	return false
}

func containsKidokuDirective(stmts []ast.Stmt) bool {
	for _, s := range stmts {
		if stmtContainsKidokuDirective(s) {
			return true
		}
	}
	return false
}

func containsCompactLineDirective(stmts []ast.Stmt) bool {
	for _, s := range stmts {
		if stmtContainsCompactLineDirective(s) {
			return true
		}
	}
	return false
}

func stmtContainsKidokuDirective(stmt ast.Stmt) bool {
	switch s := stmt.(type) {
	case nil:
		return false
	case ast.DirectiveStmt:
		return s.Name == "kidoku" || s.Name == "kidoku_line"
	case ast.IfStmt:
		return stmtContainsKidokuDirective(s.Then) || stmtContainsKidokuDirective(s.Else)
	case ast.WhileStmt:
		return stmtContainsKidokuDirective(s.Body)
	case ast.RepeatStmt:
		return containsKidokuDirective(s.Body)
	case ast.ForStmt:
		return containsKidokuDirective(s.Init) ||
			containsKidokuDirective(s.Step) ||
			stmtContainsKidokuDirective(s.Body)
	case ast.CaseStmt:
		if containsKidokuDirective(s.Default) {
			return true
		}
		for _, arm := range s.Arms {
			if containsKidokuDirective(arm.Body) {
				return true
			}
		}
	case ast.BlockStmt:
		return containsKidokuDirective(s.Stmts)
	case ast.SeqStmt:
		return containsKidokuDirective(s.Stmts)
	case ast.HidingStmt:
		return stmtContainsKidokuDirective(s.Body)
	case ast.DInlineStmt:
		return stmtContainsKidokuDirective(s.Body)
	case ast.DForStmt:
		return stmtContainsKidokuDirective(s.Body)
	case ast.DIfStmt:
		if containsKidokuDirective(s.Body) {
			return true
		}
		switch cont := s.Cont.(type) {
		case ast.DIfStmt:
			return stmtContainsKidokuDirective(cont)
		case ast.DElseStmt:
			return containsKidokuDirective(cont.Body)
		}
	}
	return false
}

func stmtContainsCompactLineDirective(stmt ast.Stmt) bool {
	switch s := stmt.(type) {
	case nil:
		return false
	case ast.DirectiveStmt:
		return s.Name == "line_compact"
	case ast.IfStmt:
		return stmtContainsCompactLineDirective(s.Then) || stmtContainsCompactLineDirective(s.Else)
	case ast.WhileStmt:
		return stmtContainsCompactLineDirective(s.Body)
	case ast.RepeatStmt:
		return containsCompactLineDirective(s.Body)
	case ast.ForStmt:
		return containsCompactLineDirective(s.Init) ||
			containsCompactLineDirective(s.Step) ||
			stmtContainsCompactLineDirective(s.Body)
	case ast.CaseStmt:
		if containsCompactLineDirective(s.Default) {
			return true
		}
		for _, arm := range s.Arms {
			if containsCompactLineDirective(arm.Body) {
				return true
			}
		}
	case ast.BlockStmt:
		return containsCompactLineDirective(s.Stmts)
	case ast.SeqStmt:
		return containsCompactLineDirective(s.Stmts)
	case ast.HidingStmt:
		return stmtContainsCompactLineDirective(s.Body)
	case ast.DInlineStmt:
		return stmtContainsCompactLineDirective(s.Body)
	case ast.DForStmt:
		return stmtContainsCompactLineDirective(s.Body)
	case ast.DIfStmt:
		if containsCompactLineDirective(s.Body) {
			return true
		}
		switch cont := s.Cont.(type) {
		case ast.DIfStmt:
			return stmtContainsCompactLineDirective(cont)
		case ast.DElseStmt:
			return containsCompactLineDirective(cont.Body)
		}
	}
	return false
}

// HasErrors returns true if any errors were collected.
func (c *Compiler) HasErrors() bool { return len(c.Errors) > 0 }

// ============================================================
// ParseElt — main per-statement dispatch (OCaml parse_elt L682)
// ============================================================

// ParseElt dispatches a single statement.
func (c *Compiler) ParseElt(stmt ast.Stmt) {
	if c.noLineForNextPause && !isPauseStmt(stmt) {
		c.noLineForNextPause = false
	}
	if c.afterEOFMarker {
		if _, ok := stmt.(ast.HaltStmt); !ok {
			c.afterEOFMarker = false
		}
	}

	// 1. Structures: if/while/for/repeat/case/block/seq/hiding/dif/dfor
	switch stmt.(type) {
	case ast.IfStmt, ast.WhileStmt, ast.ForStmt, ast.RepeatStmt,
		ast.CaseStmt, ast.BlockStmt, ast.SeqStmt, ast.HidingStmt,
		ast.DIfStmt, ast.DForStmt:
		c.parseStruct(stmt)
		return
	}

	// 2. VarOrFuncStmt: disambiguate into function call or textout
	if vf, ok := stmt.(ast.VarOrFuncStmt); ok {
		// If it's a known function, treat as FuncCall with no params
		if _, found := c.Reg.Lookup(vf.Ident); found {
			c.ParseNormElt(ast.FuncCallStmt{
				Loc: vf.Loc, Ident: vf.Ident,
			})
		} else if strings.HasPrefix(vf.Ident, "op<") {
			if _, err := fn.LookupFuncDef(c.Reg, vf.Ident, nil, false); err == nil {
				c.ParseNormElt(ast.FuncCallStmt{
					Loc: vf.Loc, Ident: vf.Ident,
				})
				return
			}
			// Unknown identifier — treat as textout if defined, else error
			c.warning(vf.Loc, fmt.Sprintf("unresolved identifier '%s'", vf.Ident))
		} else {
			// Unknown identifier — treat as textout if defined, else error
			c.warning(vf.Loc, fmt.Sprintf("unresolved identifier '%s'", vf.Ident))
		}
		return
	}

	// 3. Implicit return = textout
	if ret, ok := stmt.(ast.ReturnStmt); ok && !ret.Explicit {
		c.handleTextout(ret)
		return
	}

	// 4. Everything else: normalize then dispatch
	// Full normalization would call Expr.normalise returning
	// Nothing/Single/Multiple, but we dispatch directly since
	// our normalizer handles folding inline.
	c.ParseNormElt(stmt)
}

// handleTextout handles implicit return (text output) statements.
// OCaml: lines 652-681 — dispatches to textout, rlbabel, or stub mode
// based on __DynamicLineation__ and library flags.
func (c *Compiler) handleTextout(ret ast.ReturnStmt) {
	dynLin := "__DynamicLineation__"
	textoutKH := "__TEXTOUT_KH__"
	rlbabelKH := "__RLBABEL_KH__"

	if !c.Mem.Defined(dynLin) {
		c.compileTextStub(ret)
	} else {
		val, ok := c.Norm.NormalizeAndGetConst(c.getSymExpr(dynLin))
		if ok && val == 0 {
			c.compileTextStub(ret)
		} else if c.Mem.Defined(textoutKH) {
			// TODO: full Textout.compile with DTO tokens
			c.compileTextStub(ret)
		} else if c.Mem.Defined(rlbabelKH) {
			c.compileRLBabelText(ret)
		} else {
			c.error(ret.Loc, "__DynamicLineation__ defined, but no recognised dynamic lineation library loaded")
		}
	}
}

// compileTextStub compiles text output in "stub" (static) mode.
// This walks the text token stream and emits Shift-JIS bytecode directly.
// Corresponds to OCaml textout.ml compile_stub (lines 319-425).
func (c *Compiler) compileTextStub(ret ast.ReturnStmt) {
	if ret.Expr == nil {
		return
	}
	slit, ok := ret.Expr.(ast.StrLit)
	if !ok {
		// A textout statement can also be a bare `#res<KEY>` directly
		// (the disassembler emits these for every resource entry it
		// pulled out — the .org for SEEN0001 has lines like:
		//   `#res<0000>`
		// standalone, meaning "play this resource text as a textout").
		// Wrap them in a synthetic StrLit so the textCompiler runs the
		// resource-aware text path: plain Japanese text can stay bare
		// like original RealLive bytecode, while `\{Name}「…」`-
		// style content still goes through the quoted/unquoted marker
		// state machine.
		if rr, isRes := ret.Expr.(ast.ResRef); isRes {
			slit = ast.StrLit{
				Loc:    rr.Loc,
				Tokens: []ast.StrToken{ast.ResRefToken{Loc: rr.Loc, Key: rr.Key}},
			}
		} else {
			// Non-string non-res expression → just emit as-is
			c.addImplicitKidoku(ret.Loc)
			c.Out.EmitExprRaw(ret.Expr)
			return
		}
	}

	c.addImplicitKidoku(ret.Loc)
	tc := &textCompiler{
		c:              c,
		loc:            ret.Loc,
		buf:            make([]byte, 0, 256),
		quoted:         false,
		ignoreOneSpace: false,
	}
	tc.setQuotes(true)
	for _, tok := range slit.Tokens {
		tc.compileToken(tok)
	}
	tc.flush()
	c.noLineForNextPause = true
}

func (c *Compiler) addImplicitKidoku(loc ast.Loc) {
	if c.Out.SuppressAutoKidoku {
		return
	}
	c.Out.AddKidoku(loc, loc.Line)
}

func isPauseStmt(stmt ast.Stmt) bool {
	switch s := stmt.(type) {
	case ast.FuncCallStmt:
		return strings.TrimPrefix(s.Ident, "\\") == "pause"
	case ast.VarOrFuncStmt:
		return s.Ident == "pause"
	}
	return false
}

// textCompiler handles the state machine for static text compilation.
// Mirrors the OCaml compile_stub local state (quoted, ignore_one_space, buffer).
type textCompiler struct {
	c              *Compiler
	loc            ast.Loc
	buf            []byte
	quoted         bool
	ignoreOneSpace bool
	inSpeakerName  bool
}

// setQuotes transitions between quoted and unquoted mode.
// In quoted mode, text bytes are accumulated in the buffer;
// transitions emit opening/closing quote marks (0x22 = ").
func (tc *textCompiler) setQuotes(q bool) {
	if tc.quoted != q {
		tc.quoted = q
		tc.buf = append(tc.buf, '"') // 0x22
	}
}

func (tc *textCompiler) cancelInitialEmptyQuoteRun() bool {
	if tc.quoted && len(tc.buf) == 1 && tc.buf[0] == '"' {
		tc.quoted = false
		tc.buf = tc.buf[:0]
		return true
	}
	return false
}

func (tc *textCompiler) nativeSpeakerTags() bool {
	return tc.c != nil && tc.c.Out != nil && tc.c.Out.NativeSpeakerTags
}

// flush writes accumulated buffer to codegen output and clears it.
func (tc *textCompiler) flush() {
	tc.setQuotes(false)
	if len(tc.buf) > 0 {
		chunk := append([]byte(nil), tc.buf...)
		tc.c.Out.AddCode(tc.loc, chunk)
		tc.buf = tc.buf[:0]
	}
}

// compileToken handles one StrToken in the text stream.
func (tc *textCompiler) compileToken(tok ast.StrToken) {
	if tc.ignoreOneSpace {
		if _, isSpace := tok.(ast.SpaceToken); !isSpace {
			tc.ignoreOneSpace = false
		}
	}

	switch t := tok.(type) {
	case ast.TextToken:
		if tc.inSpeakerName && tc.nativeSpeakerTags() {
			tc.setQuotes(false)
			tc.buf = append(tc.buf, tc.textToNativeBytecode(t.Text)...)
		} else {
			tc.setQuotes(true)
			tc.buf = append(tc.buf, tc.textToBytecode(t.Text)...)
		}

	case ast.SpaceToken:
		count := t.Count
		if count > 0 && tc.ignoreOneSpace {
			tc.ignoreOneSpace = false
			count--
		}
		for i := 0; i < count; i++ {
			tc.buf = append(tc.buf, ' ')
		}

	case ast.DQuoteToken:
		tc.setQuotes(true)
		tc.buf = append(tc.buf, '\\', '"') // escaped quote in text

	case ast.SpeakerToken:
		if !tc.nativeSpeakerTags() || !tc.cancelInitialEmptyQuoteRun() {
			tc.setQuotes(false)
		}
		tc.buf = append(tc.buf, 0x81, 0x79) // 【
		tc.inSpeakerName = true

	case ast.RCurToken:
		tc.setQuotes(false)
		tc.buf = append(tc.buf, 0x81, 0x7A) // 】
		tc.ignoreOneSpace = true
		tc.inSpeakerName = false

	case ast.LLenticToken:
		tc.setQuotes(true)
		tc.buf = append(tc.buf, 0x81, 0x79) // 【

	case ast.RLenticToken:
		tc.buf = append(tc.buf, 0x81, 0x7A) // 】

	case ast.AsteriskToken:
		tc.setQuotes(true)
		tc.buf = append(tc.buf, 0x81, 0x96) // ＊

	case ast.PercentToken:
		tc.setQuotes(true)
		tc.buf = append(tc.buf, 0x81, 0x93) // ％

	case ast.HyphenToken:
		tc.setQuotes(true)
		tc.buf = append(tc.buf, '-')

	case ast.NameToken:
		tc.setQuotes(false)
		tc.compileNameToken(t)

	case ast.CodeToken:
		if t.Ident == "e" || t.Ident == "em" {
			tc.compileEmojiCode(t)
		} else {
			// Other control codes: flush and compile as function call
			tc.flush()
			tc.c.ParseElt(ast.FuncCallStmt{
				Loc: t.Loc, Ident: t.Ident, Params: t.Params,
			})
		}

	case ast.GlossToken:
		if t.IsRuby {
			// \ruby{base}{gloss} → flush + __doruby call
			tc.flush()
			tc.c.ParseElt(ast.FuncCallStmt{Loc: t.Loc, Ident: "__doruby"})
			for _, bt := range t.Base {
				tc.compileToken(bt)
			}
			tc.flush()
			// Second __doruby call with gloss text
			glossStr := ast.StrLit{Loc: t.Loc, Tokens: t.Gloss}
			tc.c.ParseElt(ast.FuncCallStmt{
				Loc:    t.Loc,
				Ident:  "__doruby",
				Params: []ast.Param{ast.SimpleParam{Loc: t.Loc, Expr: glossStr}},
			})
		} else {
			// \g{} — gloss not supported in stub mode, compile base only
			tc.c.warning(t.Loc, "\\g{} is not implemented in unformatted text")
			for _, bt := range t.Base {
				tc.compileToken(bt)
			}
		}

	case ast.DeleteToken:
		// \d → skip entire string (handled by do_compile wrapper)

	case ast.AddToken:
		// \a{key} → queued for processing after main text
		// (simplified: just emit a warning for now)
		tc.c.warning(t.Loc, "\\a{} additional string: TODO")

	case ast.RewriteToken:
		// \f{} → rewrite transformation (handled by do_compile wrapper)

	case ast.ResRefToken:
		// Resolve the resource text from the .utf companion and replay
		// it through this textCompiler so all markers (\{Name}, \m{X},
		// \l{X}, 【 】, ＊, ％) and plain text go through the same
		// quoted/unquoted state machine the engine expects. Without
		// this, a standalone `#res<0000>` line in a .org came out as
		// just the raw SJIS bytes with no framing quotes — the engine
		// read the following opcode as text and the menu froze on the
		// first speaker line (Clannad "Aw" / missing window title).
		if tc.c.Out.ResolveRes != nil {
			if raw, ok := tc.c.Out.ResolveRes(t.Key); ok {
				tc.compileResText(raw)
			}
		}

	default:
		tc.c.warning(tc.loc, fmt.Sprintf("unknown text token type: %T", tok))
	}
}

// textToBytecode converts a UTF-8 string through the active RLdev text
// transformation. This matches OCaml textout.ml's TextTransforms.to_bytecode
// path, including warning collection for characters that cannot be represented.
func (tc *textCompiler) textToBytecode(s string) []byte {
	texttransforms.ResetBadChars()
	result, err := texttransforms.ToBytecode(text.Text([]rune(s)))
	for _, r := range texttransforms.BadRunes() {
		diag.Warning(diag.Loc{File: tc.loc.File, Line: tc.loc.Line},
			"cannot represent U+%04X %q in RealLive bytecode with %s",
			r, string(r), texttransforms.Describe())
	}
	if err != nil {
		tc.c.warning(tc.loc, err.Error())
	}
	return result
}

func (tc *textCompiler) textToNativeBytecode(s string) []byte {
	b, err := encoding.UTF8ToSJS(s)
	if err == nil {
		return b
	}
	var result []byte
	for _, r := range s {
		if r <= 0x7f {
			result = append(result, byte(r))
			continue
		}
		sb, encErr := encoding.UTF8ToSJS(string(r))
		if encErr != nil {
			diag.Warning(diag.Loc{File: tc.loc.File, Line: tc.loc.Line},
				"cannot represent U+%04X %q in RealLive bytecode with native CP932 speaker-name encoding",
				r, string(r))
			if texttransforms.ForceEncode {
				result = append(result, ' ')
			}
			continue
		}
		result = append(result, sb...)
	}
	return result
}

// compileNameToken handles \l{idx} and \m{idx} name variable references.
// Emits SJIS-encoded name reference: 【lg idx 】
func (tc *textCompiler) compileNameToken(t ast.NameToken) {
	// Determine name marker: \l = local (％), \m = global (＊)
	var marker []byte
	if t.Global {
		marker = []byte{0x81, 0x96} // ＊
	} else {
		marker = []byte{0x81, 0x93} // ％
	}

	// Get constant index
	idx := 0
	if v, ok := tc.c.Norm.NormalizeAndGetConst(t.Index); ok {
		idx = int(v)
	} else {
		tc.c.error(t.Loc, "name index must be constant in static text")
		return
	}

	// Build SJIS name: marker + fullwidth digits
	tc.buf = append(tc.buf, marker...)
	// Fullwidth digits: 0x82 (0x4F + digit)
	if idx >= 10 {
		tc.buf = append(tc.buf, 0x82, byte(0x4F+idx/10))
	}
	tc.buf = append(tc.buf, 0x82, byte(0x4F+idx%10))
}

// compileEmojiCode handles \e{idx} and \em{idx} in static text.
// Emits SJIS emoji marker: 0x81 0x94 0x82 <type> 0x82 <d1> 0x82 <d2>
func (tc *textCompiler) compileEmojiCode(t ast.CodeToken) {
	if len(t.Params) < 1 {
		tc.c.error(t.Loc, fmt.Sprintf("\\%s{} requires at least one parameter", t.Ident))
		return
	}
	sp, ok := t.Params[0].(ast.SimpleParam)
	if !ok {
		tc.c.error(t.Loc, fmt.Sprintf("\\%s{} parameter must be a simple expression", t.Ident))
		return
	}
	v, ok := tc.c.Norm.NormalizeAndGetConst(sp.Expr)
	if !ok {
		tc.c.error(t.Loc, "emoji index must be constant in static text")
		return
	}
	idx := int(v)

	// Optional size parameter: flush + FontSize call
	var sizeExpr ast.Expr
	if len(t.Params) >= 2 {
		if sp2, ok := t.Params[1].(ast.SimpleParam); ok {
			sizeExpr = sp2.Expr
		}
	}
	if sizeExpr != nil {
		tc.flush()
		tc.c.ParseElt(ast.FuncCallStmt{
			Loc: t.Loc, Ident: "FontSize",
			Params: []ast.Param{ast.SimpleParam{Loc: t.Loc, Expr: sizeExpr}},
		})
	}

	tc.setQuotes(false)
	// Emoji type: \e → 0x60, \em → 0x61
	emojiType := byte(0x60)
	if t.Ident == "em" {
		emojiType = 0x61
	}
	tc.buf = append(tc.buf, 0x81, 0x94, 0x82, emojiType)
	// Two-digit index as fullwidth chars
	tc.buf = append(tc.buf, 0x82, byte(0x4F+idx/10))
	tc.buf = append(tc.buf, 0x82, byte(0x4F+idx%10))

	if sizeExpr != nil {
		tc.flush()
		tc.c.ParseElt(ast.FuncCallStmt{Loc: t.Loc, Ident: "FontSize"})
	}
}

// getSymExpr retrieves a symbol's expression, or returns IntLit(0) if missing.
func (c *Compiler) getSymExpr(name string) ast.Expr {
	e, err := c.Mem.GetAsExpr(name, ast.Nowhere)
	if err != nil {
		return ast.IntLit{Val: 0}
	}
	return e
}

// ============================================================
// ParseNormElt — normalized statement dispatch (OCaml L710)
// ============================================================

// ParseNormElt processes a fully-normalized statement.
func (c *Compiler) ParseNormElt(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case ast.ReturnStmt:
		// return _ == nop at the normalized level (L717)

	case ast.VarOrFuncStmt:
		// Should have been converted by ParseElt (L718)
		c.error(s.Loc, "internal: VarOrFuncStmt reached ParseNormElt")

	case ast.DeclStmt:
		c.compileDeclStmt(s)

	// --- Directives ---
	case ast.DirectiveStmt:
		if s.Name == "line" || s.Name == "line_compact" {
			if v, ok := s.Value.(ast.IntLit); ok {
				c.Out.AddLine(ast.Loc{File: s.Loc.File, Line: int(v.Val)})
			} else {
				c.warning(s.Loc, "#line expects an integer literal")
			}
			return
		}
		if s.Name == "kidoku" || s.Name == "kidoku_line" {
			line := s.Loc.Line
			if s.Name == "kidoku_line" {
				if v, ok := s.Value.(ast.IntLit); ok {
					line = int(v.Val)
				} else {
					c.warning(s.Loc, "kidoku_line expects an integer literal")
				}
			}
			c.Out.AddKidoku(s.Loc, line)
			return
		}
		c.Directive.Compile(s)

	case ast.DTargetStmt, ast.DefineStmt, ast.DConstStmt,
		ast.DInlineStmt, ast.DUndefStmt, ast.DSetStmt, ast.DVersionStmt:
		c.Directive.Compile(s)

	case ast.HaltStmt:
		if c.afterEOFMarker {
			c.Out.AddCodeRaw(s.Loc, []byte{0x00})
			c.afterEOFMarker = false
		} else {
			c.Out.AddCode(s.Loc, []byte{0x00}) // L722
		}

	case ast.EOFStmt:
		if c.rlBabelUsed && !c.rlBabelRuntimeDone {
			c.emitRLBabelRuntime(s.Loc)
		}
		if c.EmitEOFMarkers {
			c.Out.AddCodeRaw(s.Loc, SeenEndTrailerBytes())
		}
		c.SeenEndEmitted = true
		c.afterEOFMarker = true

	case ast.BreakStmt:
		if len(c.breakStack) == 0 {
			c.error(s.Loc, "break outside breakable structure")
			return
		}
		lbl := ast.Label{Ident: c.breakStack[len(c.breakStack)-1]}
		c.ParseElt(meta.MakeGoto(lbl)) // L723

	case ast.ContinueStmt:
		if len(c.continueStack) == 0 {
			c.error(s.Loc, "continue outside loop")
			return
		}
		lbl := ast.Label{Ident: c.continueStack[len(c.continueStack)-1]}
		c.ParseElt(meta.MakeGoto(lbl)) // L724

	case ast.LabelStmt:
		c.Out.AddLabel(s.Label.Ident, s.Loc) // L725

	case ast.GotoOnStmt:
		gotojmp.EmitGotoOn(c.Out, s.Loc, c.Reg, s.Ident, s.Expr, s.Labels) // L729

	case ast.GotoCaseStmt:
		arms := make([]gotojmp.GotoCaseArm, len(s.Cases))
		for i, a := range s.Cases {
			arms[i] = gotojmp.GotoCaseArm{
				IsDefault: a.IsDefault, Expr: a.Expr, Label: a.Label,
			}
		}
		gotojmp.EmitGotoCase(c.Out, s.Loc, c.Reg, s.Ident, s.Expr, arms) // L730

	case ast.AssignStmt:
		c.compileAssign(s) // L731 — with strcpy/strcat desugaring + store lift

	case ast.FuncCallStmt:
		c.compileFuncCall(s) // L749-762

	case ast.SelectStmt:
		c.compileSelect(s) // L764-772

	case ast.UnknownOpStmt:
		// Emit raw opcode for unknown ops (L773)
		c.Out.EmitOpcode(s.Loc, s.OpType, s.OpModule, s.OpCode, 0, 0)

	case ast.LoadFileStmt:
		c.compileLoadFile(s) // L774-784

	case ast.RawCodeStmt:
		c.compileRawCode(s) // L785-811

	default:
		c.warning(ast.Nowhere, fmt.Sprintf("unhandled normalized stmt: %T", stmt))
	}
}

// ============================================================
// Statement compilation helpers
// ============================================================

// compileDeclStmt allocates variables via Memory.AllocVar. (OCaml L720)
func (c *Compiler) compileDeclStmt(s ast.DeclStmt) {
	for _, v := range s.Vars {
		isStr := s.Type.IsStr
		arrLen := 0
		if v.ArraySize != nil {
			if sz, ok := c.Norm.NormalizeAndGetConst(v.ArraySize); ok {
				arrLen = int(sz)
			} else if v.AutoArray {
				arrLen = len(v.ArrayInit)
			}
		}

		var fixed *[2]int
		if v.AddrFrom != nil && v.AddrTo != nil {
			from, _ := c.Norm.NormalizeAndGetConst(v.AddrFrom)
			to, _ := c.Norm.NormalizeAndGetConst(v.AddrTo)
			f := [2]int{int(from), int(to)}
			fixed = &f
		}

		vt := memory.VarType{IsStr: isStr, BitWidth: s.Type.BitWidth}
		if !isStr && s.Type.BitWidth == 0 {
			vt.BitWidth = 32 // default to full int
		}

		sv, err := c.Mem.AllocVar(v.Ident, vt, arrLen, fixed)
		if err != nil {
			c.error(v.Loc, err.Error())
			continue
		}

		// Emit zero-initialization if requested
		for _, dir := range s.Dirs {
			if dir == ast.DirZero && sv != nil {
				zeroExpr := ast.IntVar{
					Loc: v.Loc, Bank: sv.TypedSpace,
					Index: ast.IntLit{Val: int32(sv.Index)},
				}
				c.Out.EmitAssignment(v.Loc, zeroExpr, ast.AssignSet, ast.IntLit{Val: 0})
			}
		}

		// Emit scalar initialization
		if v.Init != nil {
			varExpr := ast.IntVar{
				Loc: v.Loc, Bank: sv.TypedSpace,
				Index: ast.IntLit{Val: int32(sv.Index)},
			}
			if isStr {
				varExpr = ast.IntVar{} // wrong type — use StrVar
				svar := ast.StrVar{
					Loc: v.Loc, Bank: sv.TypedSpace,
					Index: ast.IntLit{Val: int32(sv.Index)},
				}
				c.Out.EmitAssignment(v.Loc, svar, ast.AssignSet, v.Init)
			} else {
				c.Out.EmitAssignment(v.Loc, varExpr, ast.AssignSet, v.Init)
			}
		}
	}
}

// compileFuncCall handles function call statements. (OCaml L749-762)
// compileAssign compiles an assignment statement.
//
// Two special cases are handled before falling through to EmitAssignment:
//
//  1. String-to-string assignment is desugared to a strcpy/strcat call.
//     RealLive has no native bytecode opcode for `strS[x] = 'foo'`; the
//     OCaml compiler (expr.ml L963-974) rewrites these as
//     `strcpy(dest, src)` for `=` and `strcat(dest, src)` for `+=`.
//     Without this, EmitExpr had no case for ast.StrLit in expression
//     position and every `strS[…] = '…'` produced invalid bytecode.
//
//  2. RHS that is a store-emitting function reference is lifted to a
//     statement call with `dest = store`. See expr.ml L295-318.
//     Covers both bare identifiers (`intA[3] = Timer`) and parenthesised
//     calls (`intA[1000] = strcmp(strS[1004], 'NONE')`).
//
// Otherwise the assignment is encoded normally as `<dest> \op <expr>`.
func (c *Compiler) compileAssign(s ast.AssignStmt) {
	// Function return assignment must be handled before the generic
	// string-copy rewrite. KFN `>` parameters are encoded as real opcode
	// arguments, so `strS[0] = itoa(n, 2)` becomes `itoa(n, strS[0], 2)`.
	if s.Op == ast.AssignSet {
		switch rhs := s.Expr.(type) {
		case ast.VarOrFunc:
			if !c.Mem.Defined(rhs.Ident) {
				if fd, ok := c.Reg.Lookup(rhs.Ident); ok && (fd.HasFlag(kfn.FlagPushStore) || funcHasReturnParam(fd)) {
					c.compileFuncCall(ast.FuncCallStmt{
						Loc:    s.Loc,
						Ident:  rhs.Ident,
						Params: nil,
						Dest:   s.Dest,
					})
					return
				}
			}
		case ast.FuncCall:
			if fd, err := fn.LookupFuncDef(c.Reg, rhs.Ident, rhs.Params, false); err == nil &&
				(fd.HasFlag(kfn.FlagPushStore) || funcHasReturnParam(fd)) {
				c.compileFuncCall(ast.FuncCallStmt{
					Loc:    s.Loc,
					Ident:  rhs.Ident,
					Params: rhs.Params,
					Label:  rhs.Label,
					Dest:   s.Dest,
				})
				return
			}
		case ast.SelFuncCall:
			c.compileSelect(ast.SelectStmt{
				Loc:    s.Loc,
				Dest:   s.Dest,
				Ident:  rhs.Ident,
				Opcode: rhs.Opcode,
				Window: rhs.Window,
				Params: rhs.Params,
			})
			return
		}
	}

	// (1) String destination → desugar to strcpy/strcat.
	if _, isStrVar := s.Dest.(ast.StrVar); isStrVar {
		var fnName string
		switch s.Op {
		case ast.AssignSet:
			fnName = "strcpy"
		case ast.AssignAdd:
			fnName = "strcat"
		default:
			c.error(s.Loc, fmt.Sprintf("assignment operator %s is not valid for strings", s.Op))
			return
		}
		c.compileFuncCall(ast.FuncCallStmt{
			Loc:   s.Loc,
			Ident: fnName,
			Params: []ast.Param{
				ast.SimpleParam{Loc: s.Loc, Expr: s.Dest},
				ast.SimpleParam{Loc: s.Loc, Expr: s.Expr},
			},
		})
		return
	}

	// (2) RHS that is a store-emitting function call.
	switch rhs := s.Expr.(type) {
	case ast.VarOrFunc:
		if !c.Mem.Defined(rhs.Ident) {
			if fd, ok := c.Reg.Lookup(rhs.Ident); ok && fd.HasFlag(kfn.FlagPushStore) {
				c.compileFuncCall(ast.FuncCallStmt{
					Loc:    s.Loc,
					Ident:  rhs.Ident,
					Params: nil,
					Dest:   s.Dest,
				})
				return
			}
		}

	case ast.FuncCall:
		if fd, ok := c.Reg.Lookup(rhs.Ident); ok && fd.HasFlag(kfn.FlagPushStore) {
			c.compileFuncCall(ast.FuncCallStmt{
				Loc:    s.Loc,
				Ident:  rhs.Ident,
				Params: rhs.Params,
				Label:  rhs.Label,
				Dest:   s.Dest,
			})
			return
		}
	}

	c.Out.EmitAssignment(s.Loc, s.Dest, s.Op, s.Expr)
}

func (c *Compiler) compileFuncCall(s ast.FuncCallStmt) {
	ident := s.Ident
	ctrlCode := false
	if strings.HasPrefix(ident, "\\") {
		ctrlCode = true
		ident = strings.TrimPrefix(ident, "\\")
	}

	if !ctrlCode && c.Intrin.IsBuiltin(ident) {
		result, err := c.Intrin.EvalAsExpr(ident, s.Loc, s.Params)
		if err != nil {
			c.error(s.Loc, err.Error())
			return
		}
		// Wrap the result expression as a return statement and recurse
		if result != nil {
			c.ParseElt(ast.ReturnStmt{Loc: s.Loc, Expr: result, Explicit: false})
		}
		return
	}

	// Regular function: lookup + overload + emit via opcode + params
	fd, err := fn.LookupFuncDef(c.Reg, ident, s.Params, ctrlCode)
	if err != nil {
		c.error(s.Loc, err.Error())
		return
	}
	params := s.Params
	overload, err := chooseOverloadForCall(fd, params, s.Dest != nil, c.Reg)
	if err != nil {
		c.error(s.Loc, err.Error())
		return
	}
	if fd.SyntheticOverload != 0 {
		// Synthetic FuncDef built from an op<...> literal: use its
		// captured overload directly.
		overload = fd.SyntheticOverload
	}
	if fd.SyntheticOverload == 0 && overload >= 0 && overload < len(fd.Prototypes) {
		if stripped, ok := fn.StripFakeParams(fd.Prototypes[overload], params); ok {
			params = stripped
		}
	}
	if s.Dest != nil {
		var injected bool
		params, injected, err = injectReturnParam(fd, overload, params, s.Dest, s.Loc)
		if err != nil {
			c.error(s.Loc, err.Error())
			return
		}
		if injected {
			// Re-check overload using the encoded parameter list. This keeps
			// argc/overload in sync with Go-disassembled sources that already
			// contain the `>` return argument explicitly.
			overload, err = chooseOverloadForCall(fd, params, false, c.Reg)
			if err != nil {
				c.error(s.Loc, err.Error())
				return
			}
		}
	}
	params = coerceLegacySpecialParams(fd, overload, params)

	// Determine which params carry the `<` (FUncount) flag in the
	// chosen prototype. The corresponding arguments are still emitted
	// in the bytecode, but they don't count toward the opcode's `argc`
	// field — this is how OCaml encodes condition arguments to
	// goto_if/goto_unless (KFN proto: `(<'condition')`). The original
	// bytecode at SEEN0414 offset 0x1eb confirms this: a goto_unless
	// with one condition argument has argc=0 yet still carries
	// `( $intA[1000] \= $0 )` in the parameter list.
	uncountIdx := uncountParamSet(fd, overload, len(params))
	argc := 0
	for i := range params {
		if !uncountIdx[i] {
			argc++
		}
	}

	// In a conditional parameter position, `!expr` must be normalised
	// to `expr == 0` so the binary `\=` operator is emitted instead of
	// a (silently-dropped) unary `!`. OCaml expr.ml L214-236
	// (`unary_to_logop` / `conditional_unit`) does the same lowering.
	emitParams := make([]ast.Param, len(params))
	condIdx := conditionalParamSet(fd, overload, len(params))
	for i, p := range params {
		if condIdx[i] {
			emitParams[i] = normalizeCondParam(p)
		} else {
			emitParams[i] = p
		}
	}

	// Emit opcode header. OCaml does not emit a fresh `#line` marker for
	// the `pause` that immediately follows a static textout; the preceding
	// kidoku/text line covers that whole display step. Extra debug-line
	// markers are invisible in a normal redump but present in the bytecode,
	// and AIR is sensitive to the larger/debug-heavier stream at route start.
	encodedOverload := encodedOverloadForCall(fd, overload)
	if c.noLineForNextPause && ident == "pause" && len(params) == 0 {
		c.Out.AddCodeRaw(s.Loc, codegen.EncodeOpcode(fd.OpType, fd.OpModule, fd.OpCode, argc, encodedOverload))
		c.noLineForNextPause = false
	} else {
		c.noLineForNextPause = false
		c.Out.EmitOpcode(s.Loc, fd.OpType, fd.OpModule, fd.OpCode, argc, encodedOverload)
	}

	// Emit params wrapped in parens. The OCaml emitter (codegen.ml
	// `compile_arglist`) handles three Param shapes:
	//
	//   SimpleParam   — emit the expression as-is.
	//   ComplexParam  — emit `( v1 v2 v3 … )` (one level of nested
	//                   parens); the engine sees the outer `(` (opened
	//                   below), then this inner `(...)`, then the
	//                   outer `)`. This is how complex(...)+ tuples
	//                   like `ReadExFrames((0, intC[1]))` are encoded.
	//   SpecialParam  — emit `<0x61 tag> ( v1 v2 … )` for the general
	//                   `__special[N](args)` shape, or `<0x61 tag> v`
	//                   for KFN `NoParens` specials represented by the
	//                   disassembler as `special<N>(v)`.
	//
	// Skipping non-SimpleParam values — as the previous implementation
	// did — strips the entire argument from the bytecode, producing
	// `(  )` instead of `( ( v1 v2 ) )`. The engine then reads garbage
	// off the stack for that opcode (e.g. ReadExFrames in SEEN9031),
	// leaves the window in an undefined state and ends up on a black
	// screen.
	if len(emitParams) > 0 {
		c.Out.AddCodeRaw(s.Loc, []byte{'('})
		// Track whether at least one parameter has already been emitted.
		// Unquoted string arguments have no self-delimiting marker, so
		// OCaml emits an ASCII comma before a later unquoted string.
		prevParam := emittedParamNone
		for _, p := range emitParams {
			switch pp := p.(type) {
			case ast.SimpleParam:
				// OCaml's expression AST does NOT carry an explicit
				// ParenExpr node — parens around a sub-expression are
				// purely grammatical and produce no bytecode of their
				// own. The Go parser, in contrast, keeps an
				// ast.ParenExpr to preserve source-level grouping. If
				// we feed that ParenExpr to EmitExprRaw, it dutifully
				// emits `(` and `)` bytes around the inner expression —
				// but the parameter list itself ALREADY supplies the
				// outer `(`/`)`, so we end up with `((cond))` in cases
				// like `goto_unless (intA[1000] == 1) @2`. The engine
				// reads the extra paren as part of the condition, the
				// label lookup desyncs, and Clannad freezes after the
				// splash. Strip top-level ParenExpr to match OCaml.
				inner := pp.Expr
				for {
					pe, ok := inner.(ast.ParenExpr)
					if !ok {
						break
					}
					inner = pe.Expr
				}
				if needsCommaBeforeParam(c.Out, prevParam, inner) {
					c.Out.AddCodeRaw(s.Loc, []byte{','})
				}
				if _, omitted := inner.(ast.OmittedExpr); omitted {
					prevParam = emittedParamOmitted
					continue
				}
				c.Out.EmitExprRaw(inner)
				prevParam = classifyEmittedParam(inner)
			case ast.ComplexParam:
				c.Out.AddCodeRaw(s.Loc, []byte{'('})
				emitExprListWithSeparators(c.Out, s.Loc, pp.Exprs)
				c.Out.AddCodeRaw(s.Loc, []byte{')'})
				prevParam = emittedParamList
			case ast.SpecialParam:
				emitSpecialParam(c.Out, s.Loc, pp)
				if pp.NoParens {
					prevParam = emittedParamSpecialNoParens
				} else {
					prevParam = emittedParamSpecialParens
				}
			}
		}
		c.Out.AddCodeRaw(s.Loc, []byte{')'})
	}

	// Emit label reference if present
	if s.Label != nil {
		c.Out.AddLabelRef(s.Label.Ident, s.Label.Loc)
	}

	// Handle return value destination
	if s.Dest != nil {
		if _, isStore := s.Dest.(ast.StoreRef); !isStore {
			if fd.HasFlag(kfn.FlagPushStore) {
				// Simulate: dest \= store
				c.Out.EmitExprRaw(s.Dest)
				c.Out.AddCodeRaw(s.Loc, []byte{'\\', 0x1e, '$', 0xc8})
			}
		}
	}
}

// compileSelect compiles a select statement. (OCaml L764-772)
func (c *Compiler) compileSelect(s ast.SelectStmt) {
	dynLin := "__DynamicLineation__"
	rlbabelKH := "__RLBABEL_KH__"

	// Choose between standard and VWF select
	useVWF := false
	if c.Mem.Defined(dynLin) && !c.Mem.Defined("__TEXTOUT_KH__") {
		val, ok := c.Norm.NormalizeAndGetConst(c.getSymExpr(dynLin))
		if !ok || val != 0 {
			if c.Mem.Defined(rlbabelKH) && sel.IsVWFOpcode(s.Opcode) {
				useVWF = true
			}
		}
	}
	_ = useVWF // TODO: use for VWF select dispatch

	// Build sel.SelParam list from ast.SelParam
	params := make([]sel.SelParam, len(s.Params))
	for i, p := range s.Params {
		switch sp := p.(type) {
		case ast.AlwaysSelParam:
			params[i] = sel.SelParam{Kind: sel.SelAlways, Loc: sp.Loc, Expr: sp.Expr}
		case ast.CondSelParam:
			conds := make([]sel.SelCond, len(sp.Conds))
			for j, sc := range sp.Conds {
				conds[j] = sel.SelCond{Effect: sc.Ident, Expr: sc.Arg, Cond: sc.Cond}
				if sc.Cond != nil {
					conds[j].Kind = sel.CondCond
				} else if sc.Arg != nil {
					conds[j].Kind = sel.CondNonCond
				} else {
					conds[j].Kind = sel.CondFlag
				}
			}
			params[i] = sel.SelParam{Kind: sel.SelSpecial, Loc: sp.Loc, Expr: sp.Expr, Conds: conds}
		}
	}

	if err := sel.EmitSelect(c.Out, s.Loc, s.Opcode, s.Window, s.Dest, params); err != nil {
		c.error(s.Loc, err.Error())
	}
}

// compileLoadFile handles #load file inclusion. (OCaml L774-784)
func (c *Compiler) compileLoadFile(s ast.LoadFileStmt) {
	path, err := c.Norm.NormalizeAndGetStr(s.Path)
	if err != nil {
		c.error(s.Loc, fmt.Sprintf("#load: cannot evaluate path: %v", err))
		return
	}
	if c.compileKnownLoadFile(s.Loc, path) {
		return
	}
	// TODO: read file, lex, parse, and call c.ParseElt on the resulting AST.
	// Search order: path, path.kh, prefix/path, prefix/path.kh
	c.warning(s.Loc, fmt.Sprintf("#load \"%s\": file inclusion pending", path))
}

// compileRawCode handles raw { ... } blocks. (OCaml L785-811)
func (c *Compiler) compileRawCode(s ast.RawCodeStmt) {
	for _, elt := range s.Elts {
		switch elt.Kind {
		case "bytes":
			c.Out.AddCode(s.Loc, []byte(elt.Str))
		case "int":
			c.Out.AddCode(s.Loc, codegen.EncodeInt32(elt.Int))
		case "ident":
			if len(elt.Str) > 0 && elt.Str[0] == '#' {
				// Hex literal: #AB or #0ABC
				hex := elt.Str[1:]
				if len(hex)%2 != 0 {
					hex = "0" + hex
				}
				out := make([]byte, len(hex)/2)
				for i := 0; i < len(out); i++ {
					var b byte
					if _, err := fmt.Sscanf(hex[i*2:i*2+2], "%02x", &b); err != nil {
						c.error(s.Loc, "syntax error in raw block: not hex")
						return
					}
					out[i] = b
				}
				c.Out.AddCode(s.Loc, out)
			} else if len(elt.Str) > 0 && elt.Str[0] == '?' {
				// Text transform — TODO: TextTransforms.to_bytecode
				c.warning(s.Loc, "raw block '?' text transform: pending")
			} else {
				c.error(s.Loc, "not implemented: identifiers in raw blocks")
			}
		}
	}
}

// ============================================================
// Structure compilation (OCaml parse_struct L813)
// ============================================================

func (c *Compiler) parseStruct(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case ast.SeqStmt:
		c.Parse(s.Stmts) // L825

	case ast.BlockStmt:
		c.Mem.OpenScope()
		c.Parse(s.Stmts)
		c.Mem.CloseScope() // L842-845

	case ast.HidingStmt:
		c.compileHiding(s) // L827-841

	// --- If statement (OCaml L847-884) ---
	case ast.IfStmt:
		c.compileIf(s)

	// --- While loop (OCaml L885-896) ---
	case ast.WhileStmt:
		// Check for constant-false condition → skip entirely
		if v, ok := c.Norm.NormalizeAndGetConst(s.Cond); ok && v == 0 {
			return
		}
		loop := c.State.UniqueLabel(s.Loc)
		skip := c.State.UniqueLabel(s.Loc)
		c.pushLoop(skip.Ident, loop.Ident)
		c.emitLabel(s.Loc, loop)
		c.ParseElt(meta.MakeGotoUnless(s.Cond, skip))
		c.ParseElt(s.Body)
		c.ParseElt(meta.MakeGoto(loop))
		c.emitLabel(s.Loc, skip)
		c.popLoop()

	// --- Repeat/until loop (OCaml L897-909) ---
	case ast.RepeatStmt:
		loop := c.State.UniqueLabel(s.Loc)
		skip := c.State.UniqueLabel(s.Loc)
		cont := c.State.UniqueLabel(s.Loc)
		c.pushLoop(skip.Ident, cont.Ident)
		c.Mem.OpenScope()
		c.emitLabel(s.Loc, loop)
		c.Parse(s.Body)
		c.emitLabel(s.Loc, cont)
		c.ParseElt(meta.MakeGotoUnless(s.Cond, loop))
		c.emitLabel(s.Loc, skip)
		c.Mem.CloseScope()
		c.popLoop()

	// --- For loop (OCaml L910-924) ---
	case ast.ForStmt:
		loop := c.State.UniqueLabel(s.Loc)
		skip := c.State.UniqueLabel(s.Loc)
		c.pushLoop(skip.Ident, loop.Ident)
		c.Mem.OpenScope()
		c.Parse(s.Init)
		c.emitLabel(s.Loc, loop)
		if s.Cond != nil {
			c.ParseElt(meta.MakeGotoUnless(s.Cond, skip))
		}
		// Unwrap Block body to avoid double scoping (OCaml L919)
		if blk, ok := s.Body.(ast.BlockStmt); ok {
			c.Parse(blk.Stmts)
		} else {
			c.ParseElt(s.Body)
		}
		c.Parse(s.Step)
		c.ParseElt(meta.MakeGoto(loop))
		c.emitLabel(s.Loc, skip)
		c.Mem.CloseScope()
		c.popLoop()

	// --- Case/switch (OCaml L925-1066) ---
	case ast.CaseStmt:
		c.compileCase(s)

	// --- Compile-time #if (OCaml L1067-1075) ---
	case ast.DIfStmt:
		c.compileDIf(s)

	// --- Compile-time #for (OCaml L1076-1086) ---
	case ast.DForStmt:
		c.compileDFor(s)

	default:
		c.warning(ast.Nowhere, fmt.Sprintf("unknown structure: %T", stmt))
	}
}

// compileIf handles if/else. (OCaml L847-884)
// Special cases:
//   - if const goto/gosub → emit goto_if/gosub_if directly
//   - if const break/continue → emit goto_if to break/continue target
//   - if const-true → emit only then branch
//   - if const-false → emit only else branch
func (c *Compiler) compileIf(s ast.IfStmt) {
	// Try constant folding the condition
	if v, ok := c.Norm.NormalizeAndGetConst(s.Cond); ok {
		if v != 0 {
			c.ParseElt(s.Then)
		} else if s.Else != nil {
			c.ParseElt(s.Else)
		}
		return
	}

	// Special case: if(cond) goto/gosub @label (OCaml L847-852)
	if fc, ok := s.Then.(ast.FuncCallStmt); ok && s.Else == nil && fc.Label != nil {
		if fc.Ident == "goto" || fc.Ident == "gosub" {
			newIdent := fc.Ident + "_if"
			params := []ast.Param{ast.SimpleParam{Loc: s.Loc, Expr: s.Cond}}
			c.ParseElt(ast.FuncCallStmt{
				Loc: s.Loc, Ident: newIdent, Params: params, Label: fc.Label,
			})
			return
		}
	}

	// Special case: if(cond) break/continue (OCaml L854-864)
	if _, ok := s.Then.(ast.BreakStmt); ok && s.Else == nil && len(c.breakStack) > 0 {
		lbl := ast.Label{Ident: c.breakStack[len(c.breakStack)-1]}
		params := []ast.Param{ast.SimpleParam{Loc: s.Loc, Expr: s.Cond}}
		c.ParseElt(ast.FuncCallStmt{Loc: s.Loc, Ident: "goto_if", Params: params, Label: &lbl})
		return
	}
	if _, ok := s.Then.(ast.ContinueStmt); ok && s.Else == nil && len(c.continueStack) > 0 {
		lbl := ast.Label{Ident: c.continueStack[len(c.continueStack)-1]}
		params := []ast.Param{ast.SimpleParam{Loc: s.Loc, Expr: s.Cond}}
		c.ParseElt(ast.FuncCallStmt{Loc: s.Loc, Ident: "goto_if", Params: params, Label: &lbl})
		return
	}

	// General case (OCaml L866-884)
	endLbl := c.State.UniqueLabel(s.Loc)
	if s.Else != nil {
		elseLbl := c.State.UniqueLabel(s.Loc)
		c.ParseElt(meta.MakeGotoUnless(s.Cond, elseLbl))
		c.ParseElt(s.Then)
		c.ParseElt(meta.MakeGoto(endLbl))
		c.emitLabel(s.Loc, elseLbl)
		c.ParseElt(s.Else)
	} else {
		c.ParseElt(meta.MakeGotoUnless(s.Cond, endLbl))
		c.ParseElt(s.Then)
	}
	c.emitLabel(s.Loc, endLbl)
}

// compileCase handles case/switch statements. (OCaml L925-1066)
// Tries:
//  1. Compile-time case selection (constant expression → pick matching arm)
//  2. goto_on (consecutive integer cases → efficient jump table)
//  3. goto_case (general fallback)
func (c *Compiler) compileCase(s ast.CaseStmt) {
	skip := c.State.UniqueLabel(s.Loc)
	c.breakStack = append(c.breakStack, skip.Ident)
	defer func() { c.breakStack = c.breakStack[:len(c.breakStack)-1] }()

	// Degenerate case: no arms (OCaml L925-935)
	if len(s.Arms) == 0 {
		c.ParseElt(ast.AssignStmt{Loc: s.Loc, Dest: ast.StoreRef{}, Op: ast.AssignSet, Expr: s.Expr})
		if len(s.Default) > 0 {
			c.Mem.Define("__ConstantCase__", memory.Symbol{Kind: memory.KindInteger, IntVal: 1})
			c.Parse(s.Default)
			c.Mem.Undefine("__ConstantCase__")
		}
		c.emitLabel(s.Loc, skip)
		return
	}

	// Try compile-time case selection
	if val, ok := c.Norm.NormalizeAndGetConst(s.Expr); ok {
		if c.tryConstantCase(s, val, skip) {
			return
		}
	}

	// Runtime case: build goto_case
	c.compileRuntimeCase(s, skip)
}

// tryConstantCase attempts to select a case arm at compile time.
func (c *Compiler) tryConstantCase(s ast.CaseStmt, val int32, skip ast.Label) bool {
	c.Mem.Define("__ConstantCase__", memory.Symbol{Kind: memory.KindInteger, IntVal: 1})
	defer c.Mem.Undefine("__ConstantCase__")

	// Find matching arm
	for idx, arm := range s.Arms {
		armVal, ok := c.Norm.NormalizeAndGetConst(arm.Cond)
		if !ok {
			return false // non-constant arm → fall through to runtime
		}
		if armVal == val {
			// Found match — emit this arm and fall through until break
			for i := idx; i < len(s.Arms); i++ {
				body := s.Arms[i].Body
				if len(body) > 0 {
					if _, isBrk := body[len(body)-1].(ast.BreakStmt); isBrk {
						c.Parse(body[:len(body)-1])
						break
					}
				}
				c.Parse(body)
			}
			c.emitLabel(s.Loc, skip)
			return true
		}
	}

	// No match — use default clause
	if len(s.Default) > 0 {
		c.Parse(s.Default)
	}
	c.emitLabel(s.Loc, skip)
	return true
}

// compileRuntimeCase emits goto_case for non-constant switch. (OCaml L1031-1063)
func (c *Compiler) compileRuntimeCase(s ast.CaseStmt, skip ast.Label) {
	c.Mem.Define("__ConstantCase__", memory.Symbol{Kind: memory.KindInteger, IntVal: 0})
	defer c.Mem.Undefine("__ConstantCase__")

	olbl := skip
	if len(s.Default) > 0 {
		olbl = c.State.UniqueLabel(s.Loc)
	}

	// Build arms: each gets a unique label
	type caseEntry struct {
		armExpr ast.Expr
		lbl     ast.Label
		body    []ast.Stmt
	}
	var entries []caseEntry
	for _, arm := range s.Arms {
		lbl := c.State.UniqueLabel(s.Loc)
		entries = append(entries, caseEntry{armExpr: arm.Cond, lbl: lbl, body: arm.Body})
	}

	// Build GotoCaseStmt
	gcCases := make([]ast.GotoCaseArm, 0, len(entries)+1)
	// Default first
	gcCases = append(gcCases, ast.GotoCaseArm{IsDefault: true, Label: olbl})
	for _, e := range entries {
		gcCases = append(gcCases, ast.GotoCaseArm{Expr: e.armExpr, Label: e.lbl})
	}

	c.ParseNormElt(ast.GotoCaseStmt{
		Loc: s.Loc, Ident: "goto_case", Expr: s.Expr, Cases: gcCases,
	})

	// Emit bodies
	if len(s.Default) > 0 {
		c.emitLabel(s.Loc, olbl)
		c.Parse(s.Default)
	}
	for _, e := range entries {
		c.emitLabel(s.Loc, e.lbl)
		c.Parse(e.body)
	}

	c.emitLabel(s.Loc, skip)
}

// compileHiding handles inline call hiding. (OCaml L827-841)
func (c *Compiler) compileHiding(s ast.HidingStmt) {
	// Save existing symbol, define inline-call markers, execute body, restore
	c.Mem.Define("__INLINE_CALL__", memory.Symbol{Kind: memory.KindInteger, IntVal: 0})
	c.Mem.Define("__CALLER_FILE__", memory.Symbol{
		Kind: memory.KindString, StrVal: s.Loc.File,
	})
	c.Mem.Define("__CALLER_LINE__", memory.Symbol{
		Kind: memory.KindInteger, IntVal: int32(s.Loc.Line),
	})

	c.ParseElt(s.Body)

	c.Mem.Undefine("__INLINE_CALL__")
	c.Mem.Undefine("__CALLER_FILE__")
	c.Mem.Undefine("__CALLER_LINE__")
}

// compileDIf handles compile-time #if/#elseif/#else/#endif. (OCaml L1067-1075)
func (c *Compiler) compileDIf(s ast.DIfStmt) {
	val, ok := c.Norm.NormalizeAndGetConst(s.Cond)
	if !ok {
		c.error(s.Loc, "#if condition must be a compile-time constant")
		return
	}

	if val != 0 {
		// Condition true → process body
		c.Parse(s.Body)
	} else {
		// Condition false → process continuation
		switch cont := s.Cont.(type) {
		case ast.DEndifStmt:
			// Nothing to do
		case ast.DElseStmt:
			c.Parse(cont.Body)
		case ast.DIfStmt:
			// Nested #elseif — recurse
			c.compileDIf(cont)
		}
	}
}

// compileDFor handles compile-time #for loops. (OCaml L1076-1086)
// Unrolls the loop at compile time: for each integer in [from..to],
// defines the symbol, parses the body, then undefines.
func (c *Compiler) compileDFor(s ast.DForStmt) {
	start, err := c.Norm.NormalizeAndGetInt(s.From)
	if err != nil {
		c.error(s.Loc, fmt.Sprintf("#for start: %v", err))
		return
	}
	finish, err := c.Norm.NormalizeAndGetInt(s.To)
	if err != nil {
		c.error(s.Loc, fmt.Sprintf("#for end: %v", err))
		return
	}

	if finish >= start {
		for i := start; i <= finish; i++ {
			c.Mem.Define(s.Ident, memory.Symbol{Kind: memory.KindInteger, IntVal: i})
			c.ParseElt(s.Body)
			c.Mem.Undefine(s.Ident)
		}
	} else {
		for i := start; i >= finish; i-- {
			c.Mem.Define(s.Ident, memory.Symbol{Kind: memory.KindInteger, IntVal: i})
			c.ParseElt(s.Body)
			c.Mem.Undefine(s.Ident)
		}
	}
}

// ============================================================
// Helpers
// ============================================================

func (c *Compiler) emitLabel(loc ast.Loc, lbl ast.Label) {
	c.Out.AddLabel(lbl.Ident, loc)
}

func (c *Compiler) pushLoop(breakLbl, contLbl string) {
	c.breakStack = append(c.breakStack, breakLbl)
	c.continueStack = append(c.continueStack, contLbl)
}

func (c *Compiler) popLoop() {
	c.breakStack = c.breakStack[:len(c.breakStack)-1]
	c.continueStack = c.continueStack[:len(c.continueStack)-1]
}

// error records a fatal compiler diagnostic at loc. The message is
// pushed into c.Errors (consumed by tests and the driver's
// HasErrors() check) AND emitted on stderr via diag.Errorf with
// the OCaml wording "Error (file line N): msg" — the driver
// formerly flushed the slice in its own format ("error: <loc>:
// <msg>"), but unifying through diag gives the translator a single,
// well-localised diagnostic per problem and lets -Wfatal / -q
// behave consistently.
func (c *Compiler) error(loc ast.Loc, msg string) {
	c.Errors = append(c.Errors, fmt.Errorf("%s: %s", loc, msg))
	diag.Errorf(diag.Loc{File: loc.File, Line: loc.Line}, "%s", msg)
}

// warning is the non-fatal twin of error. Same dual emission: the
// slice keeps the existing programmatic interface intact, diag.Warning
// gives the user the OCaml-formatted line on stderr and bumps the
// warning counter that drives Summary() and -Wfatal.
func (c *Compiler) warning(loc ast.Loc, msg string) {
	c.Warnings = append(c.Warnings, fmt.Sprintf("%s: %s", loc, msg))
	diag.Warning(diag.Loc{File: loc.File, Line: loc.Line}, "%s", msg)
}

// uncountParamSet builds the set of parameter indices whose KFN
// prototype carries the FUncount (`<`) flag, given a chosen overload
// for a function definition. Returns a boolean slice of length nParams
// where entry i is true iff that parameter is uncounted.
//
// If the prototype information is missing or the overload is out of
// range, every position defaults to "counted" — preserving the legacy
// argc = len(Params) behaviour for normal opcodes.
func uncountParamSet(fd *kfn.FuncDef, overload, nParams int) []bool {
	out := make([]bool, nParams)
	if fd == nil || overload < 0 || overload >= len(fd.Prototypes) {
		return out
	}
	proto := fd.Prototypes[overload]
	if !proto.Defined {
		return out
	}
	defs := fn.BuildParamDefs(proto.Params, nParams)
	for i := 0; i < nParams && i < len(defs); i++ {
		if defs[i].HasFlag(kfn.FUncount) {
			out[i] = true
		}
	}
	return out
}

func chooseOverloadForCall(fd *kfn.FuncDef, params []ast.Param, hasReturnDest bool, reg *kfn.Registry) (int, error) {
	if fd.SyntheticOverload != 0 {
		return fd.SyntheticOverload, nil
	}
	var selected int
	var err error
	if hasReturnDest {
		selected, err = fn.ChooseOverloadByParams(fd.Prototypes, params)
	} else if funcHasReturnParam(fd) {
		if overload, ok := chooseOverloadByFullParams(fd, params); ok {
			selected = overload
		} else {
			selected, err = fn.ChooseOverloadByParams(fd.Prototypes, params)
		}
	} else {
		if overload, ok := chooseOverloadByFullParams(fd, params); ok {
			selected = overload
		} else {
			selected, err = fn.ChooseOverloadByParams(fd.Prototypes, params)
		}
	}
	if err != nil {
		return 0, err
	}
	return selected, nil
}

func encodedOverloadForCall(fd *kfn.FuncDef, overload int) int {
	if fd == nil {
		return overload
	}
	if fd.OpType == 0 && fd.OpModule == 4 && fd.OpCode == 2000 && overload == 2 {
		return 3
	}
	return overload
}

func knownRegistryVersion(reg *kfn.Registry) (kfn.Version, bool) {
	if reg == nil || reg.Version == (kfn.Version{}) {
		return kfn.Version{}, false
	}
	return reg.Version, true
}

func funcHasReturnParam(fd *kfn.FuncDef) bool {
	if fd == nil {
		return false
	}
	for _, proto := range fd.Prototypes {
		if !proto.Defined {
			continue
		}
		for _, p := range proto.Params {
			if p.HasFlag(kfn.FReturn) {
				return true
			}
		}
	}
	return false
}

func chooseOverloadByFullParams(fd *kfn.FuncDef, params []ast.Param) (int, bool) {
	for i, proto := range fd.Prototypes {
		if !proto.Defined || !prototypeFullArityMatches(proto, len(params)) {
			continue
		}
		if prototypeFullParamsMatch(proto, params) {
			return i, true
		}
	}
	return 0, false
}

func prototypeFullArityMatches(proto kfn.Prototype, nParams int) bool {
	min, max := 0, 0
	arbitrary := false
	for _, p := range proto.Params {
		if p.HasFlag(kfn.FArgc) {
			arbitrary = true
		}
		if !p.HasFlag(kfn.FOptional) {
			min++
		}
		max++
	}
	if nParams < min {
		return false
	}
	if arbitrary {
		return true
	}
	return nParams <= max
}

func prototypeFullParamsMatch(proto kfn.Prototype, params []ast.Param) bool {
	if len(proto.Params) == 0 {
		return len(params) == 0
	}
	for i, param := range params {
		defIdx := i
		if defIdx >= len(proto.Params) {
			defIdx = len(proto.Params) - 1
			if !proto.Params[defIdx].HasFlag(kfn.FArgc) {
				return false
			}
		}
		if !prototypeParamMatches(proto.Params[defIdx], param) {
			return false
		}
	}
	return true
}

func prototypeParamMatches(def kfn.Parameter, param ast.Param) bool {
	switch p := param.(type) {
	case ast.SimpleParam:
		if def.Type == kfn.PSpecial {
			_, ok := specialParamFromLegacySimple(p, def)
			return ok
		}
		return fn.CheckParamType(def.Type, fn.ClassifyExpr(p.Expr)) == ""
	case ast.ComplexParam:
		switch def.Type {
		case kfn.PComplex:
			return true
		case kfn.PSpecial:
			_, ok := specialParamFromLegacyComplex(p, def)
			return ok
		default:
			return false
		}
	case ast.SpecialParam:
		return def.Type == kfn.PSpecial && specialTagExists(def.Specials, p.Tag)
	default:
		return false
	}
}

func specialTagExists(defs []kfn.SpecialDef, tag int) bool {
	for _, def := range defs {
		if def.ID == tag {
			return true
		}
	}
	return false
}

func injectReturnParam(fd *kfn.FuncDef, overload int, params []ast.Param, dest ast.Expr, loc ast.Loc) ([]ast.Param, bool, error) {
	if fd == nil || overload < 0 || overload >= len(fd.Prototypes) || !fd.Prototypes[overload].Defined {
		return params, false, nil
	}
	rvPos := -1
	for i, p := range fd.Prototypes[overload].Params {
		if p.HasFlag(kfn.FReturn) {
			rvPos = i
			break
		}
	}
	if rvPos < 0 {
		return params, false, nil
	}
	if rvPos > len(params) {
		return nil, false, fmt.Errorf("return value for function '%s' cannot be placed at parameter %d", fd.Ident, rvPos)
	}
	out := make([]ast.Param, 0, len(params)+1)
	out = append(out, params[:rvPos]...)
	out = append(out, ast.SimpleParam{Loc: loc, Expr: dest})
	out = append(out, params[rvPos:]...)
	return out, true, nil
}

func coerceLegacySpecialParams(fd *kfn.FuncDef, overload int, params []ast.Param) []ast.Param {
	if fd == nil || overload < 0 || overload >= len(fd.Prototypes) || !fd.Prototypes[overload].Defined {
		return params
	}
	defs := fn.BuildParamDefs(fd.Prototypes[overload].Params, len(params))
	if len(defs) == 0 {
		return params
	}
	var out []ast.Param
	for i, p := range params {
		if i >= len(defs) || defs[i].Type != kfn.PSpecial {
			continue
		}
		sp, ok := specialParamFromLegacyParam(p, defs[i])
		if !ok {
			continue
		}
		if out == nil {
			out = append([]ast.Param(nil), params...)
		}
		out[i] = sp
	}
	if out == nil {
		return params
	}
	return out
}

func specialParamFromLegacyParam(p ast.Param, def kfn.Parameter) (ast.SpecialParam, bool) {
	switch pp := p.(type) {
	case ast.ComplexParam:
		return specialParamFromLegacyComplex(pp, def)
	case ast.SimpleParam:
		return specialParamFromLegacySimple(pp, def)
	default:
		return ast.SpecialParam{}, false
	}
}

func specialParamFromLegacyComplex(cp ast.ComplexParam, def kfn.Parameter) (ast.SpecialParam, bool) {
	spec, ok := selectSpecialDef(def.Specials, cp.Exprs)
	if !ok {
		return ast.SpecialParam{}, false
	}
	return ast.SpecialParam{
		Loc:      cp.Loc,
		Tag:      spec.ID,
		Exprs:    cp.Exprs,
		NoParens: spec.HasFlag(kfn.SFNoParens),
	}, true
}

func specialParamFromLegacySimple(sp ast.SimpleParam, def kfn.Parameter) (ast.SpecialParam, bool) {
	spec, ok := selectSpecialDef(def.Specials, []ast.Expr{sp.Expr})
	if !ok || !spec.HasFlag(kfn.SFNoParens) {
		return ast.SpecialParam{}, false
	}
	return ast.SpecialParam{
		Loc:      sp.Loc,
		Tag:      spec.ID,
		Exprs:    []ast.Expr{sp.Expr},
		NoParens: true,
	}, true
}

func selectSpecialDef(defs []kfn.SpecialDef, exprs []ast.Expr) (kfn.SpecialDef, bool) {
	for _, def := range defs {
		if len(def.Params) != len(exprs) {
			continue
		}
		if specialDefMatchesExprs(def, exprs) {
			return def, true
		}
	}
	return kfn.SpecialDef{}, false
}

func specialDefMatchesExprs(def kfn.SpecialDef, exprs []ast.Expr) bool {
	for i, param := range def.Params {
		if msg := fn.CheckParamType(param.Type, fn.ClassifyExpr(stripTopLevelParens(exprs[i]))); msg != "" {
			return false
		}
	}
	return true
}

func conditionalParamSet(fd *kfn.FuncDef, overload, nParams int) []bool {
	out := make([]bool, nParams)
	if fd == nil || overload < 0 || overload >= len(fd.Prototypes) {
		return out
	}
	proto := fd.Prototypes[overload]
	if !proto.Defined {
		return out
	}
	for i := 0; i < nParams && i < len(proto.Params); i++ {
		p := proto.Params[i]
		if !p.HasFlag(kfn.FUncount) {
			continue
		}
		tag := strings.ToLower(p.Tag)
		out[i] = tag == "condition" || tag == "conditional"
	}
	return out
}

// normalizeCondParam rewrites a conditional parameter so that any
// outer unary `!` is lowered to a comparison against 0.
//
// `goto_unless (!intA[x]) @label` → `goto_unless (intA[x] == 0) @label`
//
// Mirrors OCaml expr.ml's `unary_to_logop` (L214-236): the engine has
// no `!` opcode at the bytecode level, so a leading not must become a
// logical compare in any context where the expression is consumed as
// a truth value. Parens around the inner expression are preserved.
func normalizeCondParam(p ast.Param) ast.Param {
	sp, ok := p.(ast.SimpleParam)
	if !ok {
		return p
	}
	return ast.SimpleParam{Loc: sp.Loc, Expr: expr.ConditionalUnit(liftNot(sp.Expr))}
}

// liftNot recursively rewrites `!e` as `e == 0` so the binary compare
// can be emitted by EmitExpr. RealLive has no unary-not bytecode, and
// the disassembler can emit shapes like `!intA[3] == 218`; leaving the
// nested UnaryNot in place drops the left operand and corrupts the
// following opcode stream.
func liftNot(e ast.Expr) ast.Expr {
	return liftNotInExpr(e)
}

func liftNotInExpr(e ast.Expr) ast.Expr {
	switch x := e.(type) {
	case ast.UnaryExpr:
		if x.Op == ast.UnaryNot {
			return ast.CmpExpr{
				Loc: x.Loc,
				LHS: liftNotInExpr(x.Val),
				Op:  ast.CmpEqu,
				RHS: ast.IntLit{Loc: x.Loc, Val: 0},
			}
		}
		return ast.UnaryExpr{Loc: x.Loc, Op: x.Op, Val: liftNotInExpr(x.Val)}
	case ast.ParenExpr:
		return ast.ParenExpr{Loc: x.Loc, Expr: liftNotInExpr(x.Expr)}
	case ast.CmpExpr:
		return ast.CmpExpr{
			Loc: x.Loc,
			LHS: liftNotInExpr(x.LHS),
			Op:  x.Op,
			RHS: liftNotInExpr(x.RHS),
		}
	case ast.ChainExpr:
		return ast.ChainExpr{
			Loc: x.Loc,
			LHS: liftNotInExpr(x.LHS),
			Op:  x.Op,
			RHS: liftNotInExpr(x.RHS),
		}
	case ast.BinOp:
		return ast.BinOp{
			Loc: x.Loc,
			LHS: liftNotInExpr(x.LHS),
			Op:  x.Op,
			RHS: liftNotInExpr(x.RHS),
		}
	}
	return e
}

type emittedParamKind int

const (
	emittedParamNone emittedParamKind = iota
	emittedParamOther
	emittedParamInteger
	emittedParamStringLiteral
	emittedParamList
	emittedParamSpecialParens
	emittedParamSpecialNoParens
	emittedParamOmitted
)

func needsCommaBeforeParam(out *codegen.Output, prev emittedParamKind, e ast.Expr) bool {
	if prev == emittedParamNone {
		return false
	}
	if prev == emittedParamOmitted {
		return true
	}
	if simpleParamNeedsSeparator(e) {
		switch prev {
		case emittedParamStringLiteral, emittedParamList, emittedParamSpecialParens:
			return false
		default:
			return true
		}
	}
	if out.StringExprNeedsSeparator(e) {
		switch prev {
		case emittedParamList, emittedParamSpecialParens:
			return false
		default:
			return true
		}
	}
	switch prev {
	case emittedParamStringLiteral, emittedParamSpecialNoParens:
		_, ok := out.EncodeStringExpr(e)
		return ok
	}
	return false
}

func emitExprListWithSeparators(out *codegen.Output, loc ast.Loc, exprs []ast.Expr) {
	prev := emittedParamNone
	omitted := 0
	for _, e := range exprs {
		inner := stripTopLevelParens(e)
		if _, ok := inner.(ast.OmittedExpr); ok {
			omitted++
			prev = emittedParamOmitted
			continue
		}
		if omitted > 0 {
			for i := 0; i < omitted; i++ {
				out.AddCodeRaw(loc, []byte{','})
			}
			emitExprAfterOmitted(out, loc, inner)
			omitted = 0
			prev = classifyEmittedParam(inner)
			continue
		}
		if needsCommaBeforeParam(out, prev, inner) {
			out.AddCodeRaw(loc, []byte{','})
		}
		out.EmitExprRaw(inner)
		prev = classifyEmittedParam(inner)
	}
	for i := 0; i < omitted; i++ {
		out.AddCodeRaw(loc, []byte{','})
	}
}

func emitSpecialParam(out *codegen.Output, loc ast.Loc, p ast.SpecialParam) {
	out.AddCodeRaw(loc, []byte{0x61, byte(p.Tag)})
	if !p.NoParens {
		out.AddCodeRaw(loc, []byte{'('})
	}
	emitSpecialExprListWithSeparators(out, loc, p.Exprs)
	if !p.NoParens {
		out.AddCodeRaw(loc, []byte{')'})
	}
}

func emitSpecialExprListWithSeparators(out *codegen.Output, loc ast.Loc, exprs []ast.Expr) {
	prev := emittedParamNone
	omitted := 0
	for _, e := range exprs {
		inner := stripTopLevelParens(e)
		if _, ok := inner.(ast.OmittedExpr); ok {
			omitted++
			prev = emittedParamOmitted
			continue
		}
		if omitted > 0 {
			for i := 0; i < omitted; i++ {
				out.AddCodeRaw(loc, []byte{','})
			}
			if emitBracketSpecialFuncCall(out, loc, inner) {
				omitted = 0
				prev = emittedParamSpecialParens
				continue
			}
			emitExprAfterOmitted(out, loc, inner)
			omitted = 0
			prev = classifyEmittedParam(inner)
			continue
		}
		if needsCommaBeforeParam(out, prev, inner) {
			out.AddCodeRaw(loc, []byte{','})
		}
		if emitBracketSpecialFuncCall(out, loc, inner) {
			prev = emittedParamSpecialParens
			continue
		}
		out.EmitExprRaw(inner)
		prev = classifyEmittedParam(inner)
	}
	for i := 0; i < omitted; i++ {
		out.AddCodeRaw(loc, []byte{','})
	}
}

func emitExprAfterOmitted(out *codegen.Output, loc ast.Loc, e ast.Expr) {
	if x, ok := e.(ast.UnaryExpr); ok && x.Op == ast.UnarySub {
		if _, isLit := x.Val.(ast.IntLit); isLit {
			out.AddCodeRaw(loc, []byte{'\\', codegen.OpCode(ast.OpSub)})
			out.EmitExprRaw(x.Val)
			return
		}
	}
	out.EmitExprRaw(e)
}

func emitBracketSpecialFuncCall(out *codegen.Output, loc ast.Loc, e ast.Expr) bool {
	call, ok := e.(ast.FuncCall)
	if !ok || call.Ident != "__special" || len(call.Params) == 0 {
		return false
	}
	tagParam, ok := call.Params[0].(ast.SimpleParam)
	if !ok {
		return false
	}
	tagLit, ok := stripTopLevelParens(tagParam.Expr).(ast.IntLit)
	if !ok {
		return false
	}
	out.AddCodeRaw(loc, []byte{0x61, byte(tagLit.Val)})
	out.AddCodeRaw(loc, []byte{'('})
	emitParamListWithSeparators(out, loc, call.Params[1:])
	out.AddCodeRaw(loc, []byte{')'})
	return true
}

func emitParamListWithSeparators(out *codegen.Output, loc ast.Loc, params []ast.Param) {
	prev := emittedParamNone
	for _, p := range params {
		switch pp := p.(type) {
		case ast.SimpleParam:
			inner := stripTopLevelParens(pp.Expr)
			if needsCommaBeforeParam(out, prev, inner) {
				out.AddCodeRaw(loc, []byte{','})
			}
			if emitBracketSpecialFuncCall(out, loc, inner) {
				prev = emittedParamSpecialParens
				continue
			}
			out.EmitExprRaw(inner)
			prev = classifyEmittedParam(inner)
		case ast.ComplexParam:
			out.AddCodeRaw(loc, []byte{'('})
			emitExprListWithSeparators(out, loc, pp.Exprs)
			out.AddCodeRaw(loc, []byte{')'})
			prev = emittedParamList
		case ast.SpecialParam:
			emitSpecialParam(out, loc, pp)
			if pp.NoParens {
				prev = emittedParamSpecialNoParens
			} else {
				prev = emittedParamSpecialParens
			}
		}
	}
}

func stripTopLevelParens(e ast.Expr) ast.Expr {
	for {
		pe, ok := e.(ast.ParenExpr)
		if !ok {
			return e
		}
		e = pe.Expr
	}
}

func simpleParamNeedsSeparator(e ast.Expr) bool {
	switch x := e.(type) {
	case ast.UnaryExpr:
		if x.Op == ast.UnarySub {
			if _, ok := x.Val.(ast.IntLit); ok {
				return false
			}
		}
		return true
	}
	return false
}

func classifyEmittedParam(e ast.Expr) emittedParamKind {
	switch e.(type) {
	case ast.StrLit, ast.ResRef:
		return emittedParamStringLiteral
	}
	switch fn.ClassifyExpr(e) {
	case fn.ETInt:
		return emittedParamInteger
	case fn.ETLiteral:
		return emittedParamStringLiteral
	}
	return emittedParamOther
}

// compileResText lexes a `.utf` resource string (the raw text the
// disassembler wrote for a given <NNNN> entry) and replays its markers
// and text through this textCompiler's state machine.
//
// We can't reuse the lexResourceText helper in codegen because that
// returns its own internal token type. We re-tokenise here so each
// marker is dispatched to the appropriate compileToken case, ensuring
// every `\{Name}「…」` block produced by the disassembler keeps its
// speaker name as native CP932 while the following dialogue text still
// gets the quoting and transform handling expected by RealLive. Anything
// Go-specific that compileTextStub already does (kidoku,
// ignoreOneSpace tracking, etc.) is preserved because we go through the
// same compileToken dispatcher.
func (tc *textCompiler) compileResText(s string) {
	if tc.tryCompileInitialBareResourceText(s) {
		return
	}
	if tc.tryCompileInitialBareSpeakerResourceText(s) {
		return
	}

	r := []rune(s)
	i := 0
	var textBuf []rune
	flushText := func() {
		if len(textBuf) > 0 {
			tc.compileToken(ast.TextToken{Loc: tc.loc, Text: string(textBuf)})
			textBuf = textBuf[:0]
		}
	}
	for i < len(r) {
		c := r[i]
		// Backslash escapes
		if c == '\\' && i+1 < len(r) {
			if value, consumed, ok := scanRawByteEscape(r, i); ok {
				flushText()
				if !tc.cancelInitialEmptyQuoteRun() {
					tc.setQuotes(false)
				}
				tc.buf = append(tc.buf, value)
				i += consumed
				continue
			}
			if value, consumed, ok := scanWaitControl(r, i); ok {
				flushText()
				tc.compileWaitControl(value)
				i += consumed
				continue
			}
			next := r[i+1]
			if next == '{' {
				flushText()
				tc.compileToken(ast.SpeakerToken{Loc: tc.loc})
				i += 2
				continue
			}
			if (next == 'l' || next == 'm') && i+2 < len(r) && r[i+2] == '{' {
				// \l{X} / \m{X} / \l{XX} / \l{X,N}
				end := i + 3
				for end < len(r) && r[end] != '}' {
					end++
				}
				if end < len(r) {
					inside := string(r[i+3 : end])
					// Parse letters (1-2 uppercase A-Z) and optional ", N"
					p := 0
					var letters []byte
					for p < len(inside) && inside[p] >= 'A' && inside[p] <= 'Z' && len(letters) < 2 {
						letters = append(letters, inside[p])
						p++
					}
					hasCharID := false
					charID := 0
					if p < len(inside) && inside[p] == ',' {
						p++
						for p < len(inside) && inside[p] == ' ' {
							p++
						}
						if p < len(inside) && inside[p] >= '0' && inside[p] <= '9' {
							charID = int(inside[p] - '0')
							hasCharID = true
							p++
						}
					}
					if len(letters) > 0 && p == len(inside) {
						flushText()
						tc.compileResourceNameMarker(next == 'm', letters, hasCharID, charID)
						i = end + 1
						continue
					}
				}
			}
			if src, consumed, ok := scanResourceControl(r, i); ok {
				if stmt, ok := parseResourceControl(src, tc.loc); ok && tc.isKnownResourceControl(stmt.Ident) {
					flushText()
					tc.flush()
					tc.c.ParseElt(stmt)
					i += consumed
					continue
				}
				if src, consumed, ok := scanLegacyAttachedPauseControl(r, i); ok {
					if stmt, ok := parseResourceControl(src, tc.loc); ok && tc.isKnownResourceControl(stmt.Ident) {
						flushText()
						tc.flush()
						tc.c.ParseElt(stmt)
						i += consumed
						continue
					}
				}
			}
			if !isResourceControlStart(next) {
				textBuf = append(textBuf, next)
				i += 2
				continue
			}
			// Unknown escape — emit literal '\'
			textBuf = append(textBuf, c)
			i++
			continue
		}
		switch c {
		case '"':
			flushText()
			tc.compileToken(ast.DQuoteToken{Loc: tc.loc})
			i++
		case '}':
			flushText()
			tc.compileToken(ast.RCurToken{Loc: tc.loc})
			i++
		case 0x3010: // 【
			flushText()
			tc.compileToken(ast.LLenticToken{Loc: tc.loc})
			i++
		case 0x3011: // 】
			flushText()
			tc.compileToken(ast.RLenticToken{Loc: tc.loc})
			i++
		case 0xff0a: // ＊
			flushText()
			tc.compileToken(ast.AsteriskToken{Loc: tc.loc})
			i++
		case 0xff05: // ％
			flushText()
			tc.compileToken(ast.PercentToken{Loc: tc.loc})
			i++
		default:
			textBuf = append(textBuf, c)
			i++
		}
	}
	flushText()
}

func (tc *textCompiler) tryCompileInitialBareResourceText(s string) bool {
	if !plainNonASCIIResourceText(s) {
		return false
	}
	if !tc.cancelInitialEmptyQuoteRun() {
		return false
	}
	tc.buf = append(tc.buf, tc.textToBytecode(s)...)
	return true
}

func (tc *textCompiler) tryCompileInitialBareSpeakerResourceText(s string) bool {
	name, body, ok := splitPlainSpeakerResourceText(s)
	if !ok {
		return false
	}
	if !tc.cancelInitialEmptyQuoteRun() {
		return false
	}
	tc.buf = append(tc.buf, 0x81, 0x79) // 【
	tc.buf = append(tc.buf, tc.textToBytecode(name)...)
	tc.buf = append(tc.buf, 0x81, 0x7A) // 】
	tc.buf = append(tc.buf, tc.textToBytecode(body)...)
	tc.inSpeakerName = false
	tc.ignoreOneSpace = false
	return true
}

func splitPlainSpeakerResourceText(s string) (string, string, bool) {
	if !strings.HasPrefix(s, `\{`) {
		return "", "", false
	}
	endName := strings.IndexRune(s[2:], '}')
	if endName < 0 {
		return "", "", false
	}
	endName += 2
	name := s[2:endName]
	body := s[endName+1:]
	if name == "" || body == "" {
		return "", "", false
	}
	if strings.ContainsAny(name, `\{}"`) || strings.ContainsAny(body, `\{}"`) {
		return "", "", false
	}
	if !plainNonASCIIResourceText(name) || !plainNonASCIIResourceText(body) {
		return "", "", false
	}
	return name, body, true
}

func plainNonASCIIResourceText(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r <= 0x7f {
			return false
		}
	}
	return true
}

func scanRawByteEscape(r []rune, start int) (byte, int, bool) {
	if start+3 >= len(r) || r[start] != '\\' || r[start+1] != 'x' || r[start+2] != '{' {
		return 0, 0, false
	}
	pos := start + 3
	hexStart := pos
	for pos < len(r) && r[pos] != '}' {
		pos++
	}
	if pos >= len(r) || pos == hexStart {
		return 0, 0, false
	}
	raw := string(r[hexStart:pos])
	value, err := strconv.ParseUint(raw, 16, 8)
	if err != nil {
		return 0, 0, false
	}
	return byte(value), pos - start + 1, true
}

func (tc *textCompiler) isKnownResourceControl(ident string) bool {
	if !strings.HasPrefix(ident, "\\") {
		return false
	}
	_, ok := tc.c.Reg.LookupCtrlCode(strings.TrimPrefix(ident, "\\"))
	return ok
}

func scanWaitControl(r []rune, start int) (int32, int, bool) {
	prefix := []rune(`\wait{`)
	if start+len(prefix) >= len(r) {
		return 0, 0, false
	}
	for i, c := range prefix {
		if r[start+i] != c {
			return 0, 0, false
		}
	}
	pos := start + len(prefix)
	argStart := pos
	for pos < len(r) && r[pos] != '}' {
		pos++
	}
	if pos >= len(r) || pos == argStart {
		return 0, 0, false
	}
	raw := strings.TrimSpace(string(r[argStart:pos]))
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, 0, false
	}
	return int32(value), pos - start + 1, true
}

func scanResourceControl(r []rune, start int) (string, int, bool) {
	if start+1 >= len(r) || r[start] != '\\' || !isResourceControlStart(r[start+1]) {
		return "", 0, false
	}

	pos := start + 2
	for pos < len(r) && isResourceControlPart(r[pos]) {
		pos++
	}
	identEnd := pos
	for pos < len(r) && (r[pos] == ' ' || r[pos] == '\t') {
		pos++
	}
	if pos >= len(r) || r[pos] != '{' {
		return string(r[start:identEnd]), identEnd - start, true
	}

	depth := 0
	for pos < len(r) {
		switch r[pos] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				pos++
				return string(r[start:pos]), pos - start, true
			}
		case '\n', '\r':
			return "", 0, false
		}
		pos++
	}
	return "", 0, false
}

func scanLegacyAttachedPauseControl(r []rune, start int) (string, int, bool) {
	if start+2 >= len(r) || r[start] != '\\' || r[start+1] != 'p' {
		return "", 0, false
	}
	if !isResourceControlPart(r[start+2]) {
		return "", 0, false
	}
	return `\p`, len(`\p`), true
}

func parseResourceControl(src string, loc ast.Loc) (stmt ast.FuncCallStmt, ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	p := parser.New(lexer.New(src, loc.File))
	fc, ok := p.ParseExpression().(ast.FuncCall)
	if !ok {
		return ast.FuncCallStmt{}, false
	}
	return ast.FuncCallStmt{
		Loc:    loc,
		Ident:  fc.Ident,
		Params: fc.Params,
		Label:  fc.Label,
	}, true
}

func isResourceControlStart(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_'
}

func isResourceControlPart(r rune) bool {
	return isResourceControlStart(r) || (r >= '0' && r <= '9')
}

func (tc *textCompiler) compileWaitControl(value int32) {
	tc.flush()
	tc.c.ParseElt(ast.FuncCallStmt{
		Loc:   tc.loc,
		Ident: `\wait`,
		Params: []ast.Param{
			ast.SimpleParam{Loc: tc.loc, Expr: ast.IntLit{Loc: tc.loc, Val: value}},
		},
	})
}

func (tc *textCompiler) compileResourceNameMarker(global bool, letters []byte, hasCharID bool, charID int) {
	tc.setQuotes(false)
	if global {
		tc.buf = append(tc.buf, 0x81, 0x96)
	} else {
		tc.buf = append(tc.buf, 0x81, 0x93)
	}
	for _, c := range letters {
		tc.buf = append(tc.buf, 0x82, c-'A'+0x60)
	}
	if hasCharID {
		tc.buf = append(tc.buf, 0x82, byte(charID)+0x4f)
	}
}
