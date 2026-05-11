// Package compilerframe implements the main compiler orchestration for Kepago.
//
// Transposed from OCaml's rlc/compilerFrame.ml (1315 lines).
//
// This is the top-level driver tying together all backend packages:
//   lexer/parser → AST → compilerframe → codegen → .seen bytecode
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

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"

	"github.com/yoremi/rldev-go/rlc/pkg/ast"
	"github.com/yoremi/rldev-go/rlc/pkg/codegen"
	"github.com/yoremi/rldev-go/rlc/pkg/directive"
	"github.com/yoremi/rldev-go/rlc/pkg/expr"
	fn "github.com/yoremi/rldev-go/rlc/pkg/function"
	gotojmp "github.com/yoremi/rldev-go/rlc/pkg/goto"
	"github.com/yoremi/rldev-go/rlc/pkg/ini"
	"github.com/yoremi/rldev-go/rlc/pkg/intrinsic"
	"github.com/yoremi/rldev-go/rlc/pkg/kfn"
	"github.com/yoremi/rldev-go/rlc/pkg/memory"
	"github.com/yoremi/rldev-go/rlc/pkg/meta"
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

	breakStack    []string // break targets (label idents)
	continueStack []string // continue targets (label idents)

	Errors   []error
	Warnings []string
	Verbose  int
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
		Reg: reg, Ini: iniTable, State: state,
	}
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
	if c.Directive != nil {
		c.Errors = append(c.Errors, c.Directive.Errors...)
		c.Warnings = append(c.Warnings, c.Directive.Warnings...)
	}
}

// Parse processes a batch of statements. Called recursively from
// meta.State.Parse via the CompileStatements callback.
func (c *Compiler) Parse(stmts []ast.Stmt) {
	for _, s := range stmts {
		c.ParseElt(s)
	}
}

// HasErrors returns true if any errors were collected.
func (c *Compiler) HasErrors() bool { return len(c.Errors) > 0 }

// ============================================================
// ParseElt — main per-statement dispatch (OCaml parse_elt L682)
// ============================================================

// ParseElt dispatches a single statement.
func (c *Compiler) ParseElt(stmt ast.Stmt) {
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
			// TODO: full RlBabel.compile with VWF tokens
			c.compileTextStub(ret)
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
		// Non-string expression → just emit as-is
		c.Out.AddKidoku(ret.Loc, ret.Loc.Line)
		c.Out.EmitExpr(ret.Expr)
		return
	}

	c.Out.AddKidoku(ret.Loc, ret.Loc.Line)
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
}

// textCompiler handles the state machine for static text compilation.
// Mirrors the OCaml compile_stub local state (quoted, ignore_one_space, buffer).
type textCompiler struct {
	c              *Compiler
	loc            ast.Loc
	buf            []byte
	quoted         bool
	ignoreOneSpace bool
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

// flush writes accumulated buffer to codegen output and clears it.
func (tc *textCompiler) flush() {
	tc.setQuotes(false)
	if len(tc.buf) > 0 {
		tc.c.Out.AddCode(tc.loc, tc.buf)
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
		tc.setQuotes(true)
		tc.buf = append(tc.buf, tc.textToSJIS(t.Text)...)

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
		tc.setQuotes(false)
		tc.buf = append(tc.buf, 0x81, 0x79) // 【

	case ast.RCurToken:
		tc.setQuotes(false)
		tc.buf = append(tc.buf, 0x81, 0x7A) // 】
		tc.ignoreOneSpace = true

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
		// Resource reference — should have been resolved earlier

	default:
		tc.c.warning(tc.loc, fmt.Sprintf("unknown text token type: %T", tok))
	}
}

// textToSJIS converts a UTF-8 string to Shift-JIS bytes for bytecode.
// Uses golang.org/x/text/encoding for the conversion.
func (tc *textCompiler) textToSJIS(s string) []byte {
	encoder := japanese.ShiftJIS.NewEncoder()
	result, _, err := transform.Bytes(encoder, []byte(s))
	if err != nil {
		// Fallback: try character by character, replacing failures with spaces
		var out []byte
		for _, r := range s {
			rb, _, err := transform.Bytes(encoder, []byte(string(r)))
			if err != nil {
				out = append(out, ' ')
				tc.c.warning(tc.loc, fmt.Sprintf("cannot represent U+%04X in Shift-JIS", r))
			} else {
				out = append(out, rb...)
			}
		}
		return out
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
	case ast.DirectiveStmt, ast.DTargetStmt, ast.DefineStmt, ast.DConstStmt,
		ast.DInlineStmt, ast.DUndefStmt, ast.DSetStmt, ast.DVersionStmt:
		c.Directive.Compile(s)

	case ast.HaltStmt:
		c.Out.AddCode(s.Loc, []byte{0x00}) // L722

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
		c.Out.EmitAssignment(s.Loc, s.Dest, s.Op, s.Expr) // L731

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
func (c *Compiler) compileFuncCall(s ast.FuncCallStmt) {
	if c.Intrin.IsBuiltin(s.Ident) {
		result, err := c.Intrin.EvalAsExpr(s.Ident, s.Loc, s.Params)
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
	fd, err := fn.LookupFuncDef(c.Reg, s.Ident, s.Params, false)
	if err != nil {
		c.error(s.Loc, err.Error())
		return
	}
	overload, _ := fn.ChooseOverloadByParams(fd.Prototypes, s.Params)
	if fd.SyntheticOverload != 0 {
		// Synthetic FuncDef built from an op<...> literal: use its
		// captured overload directly.
		overload = fd.SyntheticOverload
	}

	// Emit opcode header
	c.Out.EmitOpcode(s.Loc, fd.OpType, fd.OpModule, fd.OpCode, len(s.Params), overload)

	// Emit params wrapped in parens
	if len(s.Params) > 0 {
		c.Out.AddCode(s.Loc, []byte{'('})
		for _, p := range s.Params {
			if sp, ok := p.(ast.SimpleParam); ok {
				c.Out.EmitExpr(sp.Expr)
			}
		}
		c.Out.AddCode(s.Loc, []byte{')'})
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
				c.Out.EmitExpr(s.Dest)
				c.Out.AddCode(s.Loc, []byte{'\\', 0x1e, '$', 0xc8})
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
				conds[j] = sel.SelCond{Effect: sc.Ident, Expr: sc.Arg}
				if sc.Arg != nil {
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
//   1. Compile-time case selection (constant expression → pick matching arm)
//   2. goto_on (consecutive integer cases → efficient jump table)
//   3. goto_case (general fallback)
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

func (c *Compiler) error(loc ast.Loc, msg string) {
	c.Errors = append(c.Errors, fmt.Errorf("%s: %s", loc, msg))
}

func (c *Compiler) warning(loc ast.Loc, msg string) {
	c.Warnings = append(c.Warnings, fmt.Sprintf("%s: %s", loc, msg))
}

