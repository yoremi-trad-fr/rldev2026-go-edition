// Package compilerframe implements the main compiler orchestration for Kepago.
//
// Transposed from OCaml's rlc/compilerFrame.ml (1315 lines).
//
// This package is the top-level driver that orchestrates all the rlc backend
// packages. It exposes a clean public API (New, Compile, Parse, ParseElt)
// and wires meta's CompileStatements callback so other packages can recurse
// into the compiler without creating import cycles.
//
// # Status: SKELETON
//
// The public API, dependency wiring, and dispatch structure are in place.
// The dispatch cases for most statement types are TODOs, documented below
// with the OCaml line numbers to port from. Control-flow structures
// (if/while/for/do-while) and switch are the main remaining work.
//
// # Architecture
//
//	Source .org  ->  lexer.Lex  ->  tokens  ->  parser.Parse  ->  AST
//	   |
//	   v
//	compilerframe.Compile(stmts)   <-- this package
//	   |
//	   +-- ParseElt (per-statement dispatch, parse_elt line 682)
//	   |   +-- disambiguate (line 523)
//	   |   +-- parseStruct (line 813): if/while/for/do-while/switch/block
//	   |   +-- handleTextout (line 652)
//	   |   +-- Norm.NormalizeStmt -> ParseNormElt
//	   +-- ParseNormElt (normalized dispatch, parse_norm_elt line 710)
//	       +-- Directive.Compile   for #define/#const/etc.
//	       +-- Halt -> emit 0x00
//	       +-- Break/Continue -> goto with break_stack/continue_stack top
//	       +-- Label -> Output.AddLabel
//	       +-- GotoOn -> gotojmp.EmitGotoOn
//	       +-- GotoCase -> gotojmp.EmitGotoCase
//	       +-- Assign -> emit assignment bytecode
//	       +-- FuncCall -> Intrinsic or Function.Assemble
//	       +-- Select -> sel.EmitSelect / EmitSelectVWF
//	       +-- RawCode -> emit raw bytes/ints/idents (hex decoding)
//	       +-- LoadFile -> recursive parse (get_ast_of_file)
//	   |
//	   v
//	Output.Build -> .seen bytecode
package compilerframe

import (
	"fmt"

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

// Compiler holds all the state needed to compile a Kepago program.
// It wires memory, codegen, expression normalization, directive compiler,
// intrinsics registry, KFN function registry, INI table, and meta state.
type Compiler struct {
	Mem       *memory.Memory
	Out       *codegen.Output
	Norm      *expr.Normalizer
	Directive *directive.Compiler
	Intrin    *intrinsic.Registry
	Reg       *kfn.Registry
	Ini       *ini.Table
	State     *meta.State

	// Runtime control flow stacks (populated by while/for/do-while/switch)
	breakStack    []string
	continueStack []string

	Errors   []error
	Warnings []string

	Verbose int
}

// New creates a fully-wired Compiler instance.
// The registry can be an empty kfn.Registry if no KFN file is loaded.
// The ini table can be empty if no GAMEEXE.INI is loaded.
func New(reg *kfn.Registry, iniTable *ini.Table) *Compiler {
	mem := memory.New()
	out := codegen.NewOutput()
	norm := expr.NewNormalizer(mem)
	state := meta.NewState()

	c := &Compiler{
		Mem:    mem,
		Out:    out,
		Norm:   norm,
		Intrin: intrinsic.New(mem),
		Reg:    reg,
		Ini:    iniTable,
		State:  state,
	}
	c.Directive = &directive.Compiler{
		Mem:    mem,
		Norm:   norm,
		Output: out,
		Ini:    iniTable,
		State:  state,
	}

	// Wire meta's Parse callback back to our Parse method. This corresponds
	// to OCaml's Global.compilerFrame__parse reference — how the meta
	// package invokes the compiler recursively without creating an import
	// cycle.
	state.CompileStatements = c.Parse

	return c
}

// Compile compiles a full program and merges any errors/warnings from
// sub-compilers. This is the main entry point.
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

// ParseElt dispatches a single statement.
// Corresponds to parse_elt (line 682) in compilerFrame.ml.
func (c *Compiler) ParseElt(stmt ast.Stmt) {
	// 1. Structures: if/while/for/repeat/case/block/seq/hiding
	switch stmt.(type) {
	case ast.IfStmt, ast.WhileStmt, ast.ForStmt, ast.RepeatStmt,
		ast.CaseStmt, ast.BlockStmt, ast.SeqStmt, ast.HidingStmt:
		c.parseStruct(stmt)
		return
	}
	// 2. Implicit return = textout (TODO: full textout dispatch)
	if ret, ok := stmt.(ast.ReturnStmt); ok && !ret.Explicit {
		// TODO: handleTextout — dispatch to textout/rlbabel based on __DynamicLineation__
		_ = ret
		return
	}
	// 3. Everything else: normalize then dispatch via ParseNormElt
	// TODO: full normalization (Expr.normalise returning Nothing/Single/Multiple)
	// For now, dispatch directly.
	c.ParseNormElt(stmt)
}

// ParseNormElt processes a fully-normalized statement.
// Corresponds to parse_norm_elt (line 710) in compilerFrame.ml.
func (c *Compiler) ParseNormElt(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case ast.ReturnStmt:
		// return _ == nop at the normalized level

	case ast.DeclStmt:
		// Variable allocation — TODO: Variables.allocate(s)
		_ = s

	// --- Directives ---
	case ast.DirectiveStmt, ast.DTargetStmt, ast.DefineStmt, ast.DConstStmt,
		ast.DInlineStmt, ast.DUndefStmt, ast.DSetStmt, ast.DVersionStmt:
		c.Directive.Compile(s)

	case ast.HaltStmt:
		c.Out.AddCode(s.Loc, []byte{0x00})

	case ast.BreakStmt:
		if len(c.breakStack) == 0 {
			c.error(s.Loc, "break outside breakable structure")
			return
		}
		lbl := ast.Label{Ident: c.breakStack[len(c.breakStack)-1]}
		c.ParseElt(meta.MakeGoto(lbl))

	case ast.ContinueStmt:
		if len(c.continueStack) == 0 {
			c.error(s.Loc, "continue outside loop")
			return
		}
		lbl := ast.Label{Ident: c.continueStack[len(c.continueStack)-1]}
		c.ParseElt(meta.MakeGoto(lbl))

	case ast.LabelStmt:
		c.Out.AddLabel(s.Label.Ident, s.Loc)

	case ast.GotoOnStmt:
		gotojmp.EmitGotoOn(c.Out, s.Loc, c.Reg, s.Ident, s.Expr, s.Labels)

	case ast.GotoCaseStmt:
		arms := make([]gotojmp.GotoCaseArm, len(s.Cases))
		for i, a := range s.Cases {
			arms[i] = gotojmp.GotoCaseArm{
				IsDefault: a.IsDefault, Expr: a.Expr, Label: a.Label,
			}
		}
		gotojmp.EmitGotoCase(c.Out, s.Loc, c.Reg, s.Ident, s.Expr, arms)

	case ast.AssignStmt:
		c.Out.EmitAssignment(s.Loc, s.Dest, s.Op, s.Expr)

	case ast.FuncCallStmt:
		if c.Intrin.IsBuiltin(s.Ident) {
			// Intrinsic: evaluate as code and recurse
			result, err := c.Intrin.EvalAsExpr(s.Ident, s.Loc, s.Params)
			if err != nil {
				c.error(s.Loc, err.Error())
			}
			_ = result
			// TODO: wrap result as a statement and recurse via ParseElt
		} else {
			// Regular function call via Function.compile
			fd, err := fn.LookupFuncDef(c.Reg, s.Ident, s.Params, false)
			if err != nil {
				c.error(s.Loc, err.Error())
				return
			}
			overload, _ := fn.ChooseOverloadByParams(fd.Prototypes, s.Params)
			// Build assembled params (simplified)
			asmParams := make([]fn.AsmParam, 0, len(s.Params))
			for _, p := range s.Params {
				if _, ok := p.(ast.SimpleParam); ok {
					asmParams = append(asmParams, fn.AsmParam{
						Kind: fn.AsmUnknown,
						Code: "", // TODO: c.Out.EncodeExpr(sp.Expr)
					})
				}
			}
			result, err := fn.Assemble(fd, asmParams, overload, "")
			if err != nil {
				c.error(s.Loc, err.Error())
				return
			}
			c.Out.AddCode(s.Loc, result.Code)
			if result.Append != nil {
				c.Out.AddCode(s.Loc, result.Append)
			}
			if s.Label != nil {
				c.Out.AddLabelRef(s.Label.Ident, s.Label.Loc)
			}
		}

	case ast.SelectStmt:
		// TODO: VWF dispatch based on __DynamicLineation__ / __RLBABEL_KH__
		params := make([]sel.SelParam, len(s.Params))
		for i, p := range s.Params {
			if ap, ok := p.(ast.AlwaysSelParam); ok {
				params[i] = sel.SelParam{Kind: sel.SelAlways, Loc: ap.Loc, Expr: ap.Expr}
			}
		}
		if err := sel.EmitSelect(c.Out, s.Loc, s.Opcode, s.Window, s.Dest, params); err != nil {
			c.error(s.Loc, err.Error())
		}

	case ast.UnknownOpStmt:
		// TODO: Function.compile_unknown
		c.warning(s.Loc, "unknown opcode: TODO")

	case ast.LoadFileStmt:
		// TODO: recursive file loading
		c.warning(s.Loc, "#load: TODO — recursive file loading")

	case ast.RawCodeStmt:
		for _, elt := range s.Elts {
			switch elt.Kind {
			case "bytes":
				c.Out.AddCode(s.Loc, []byte(elt.Str))
			case "int":
				c.Out.AddCode(s.Loc, codegen.EncodeInt32(elt.Int))
			case "ident":
				if len(elt.Str) > 0 && elt.Str[0] == '#' {
					hex := elt.Str[1:]
					if len(hex)%2 != 0 {
						hex = "0" + hex
					}
					out := make([]byte, len(hex)/2)
					for i := 0; i < len(out); i++ {
						var b byte
						fmt.Sscanf(hex[i*2:i*2+2], "%02x", &b)
						out[i] = b
					}
					c.Out.AddCode(s.Loc, out)
				} else {
					c.error(s.Loc, "unsupported raw ident")
				}
			}
		}

	default:
		c.warning(ast.Nowhere, fmt.Sprintf("unhandled normalized stmt: %T", stmt))
	}
}

// ============================================================
// Structure compilation (parse_struct, line 813)
// ============================================================

func (c *Compiler) parseStruct(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case ast.SeqStmt:
		c.Parse(s.Stmts)

	case ast.BlockStmt:
		c.Mem.OpenScope()
		c.Parse(s.Stmts)
		c.Mem.CloseScope()

	case ast.IfStmt:
		endLbl := c.State.UniqueLabel(s.Loc)
		if s.Else != nil {
			elseLbl := c.State.UniqueLabel(s.Loc)
			c.ParseElt(meta.MakeGotoUnless(s.Cond, elseLbl))
			c.ParseElt(s.Then)
			c.ParseElt(meta.MakeGoto(endLbl))
			c.ParseNormElt(ast.LabelStmt{Loc: s.Loc, Label: elseLbl})
			c.ParseElt(s.Else)
		} else {
			c.ParseElt(meta.MakeGotoUnless(s.Cond, endLbl))
			c.ParseElt(s.Then)
		}
		c.ParseNormElt(ast.LabelStmt{Loc: s.Loc, Label: endLbl})

	case ast.WhileStmt:
		loop := c.State.UniqueLabel(s.Loc)
		skip := c.State.UniqueLabel(s.Loc)
		c.breakStack = append(c.breakStack, skip.Ident)
		c.continueStack = append(c.continueStack, loop.Ident)
		c.ParseNormElt(ast.LabelStmt{Loc: s.Loc, Label: loop})
		c.ParseElt(meta.MakeGotoUnless(s.Cond, skip))
		c.ParseElt(s.Body)
		c.ParseElt(meta.MakeGoto(loop))
		c.ParseNormElt(ast.LabelStmt{Loc: s.Loc, Label: skip})
		c.breakStack = c.breakStack[:len(c.breakStack)-1]
		c.continueStack = c.continueStack[:len(c.continueStack)-1]

	case ast.RepeatStmt:
		loop := c.State.UniqueLabel(s.Loc)
		skip := c.State.UniqueLabel(s.Loc)
		cont := c.State.UniqueLabel(s.Loc)
		c.breakStack = append(c.breakStack, skip.Ident)
		c.continueStack = append(c.continueStack, cont.Ident)
		c.Mem.OpenScope()
		c.ParseNormElt(ast.LabelStmt{Loc: s.Loc, Label: loop})
		c.Parse(s.Body)
		c.ParseNormElt(ast.LabelStmt{Loc: s.Loc, Label: cont})
		c.ParseElt(meta.MakeGotoUnless(s.Cond, loop))
		c.ParseNormElt(ast.LabelStmt{Loc: s.Loc, Label: skip})
		c.Mem.CloseScope()
		c.breakStack = c.breakStack[:len(c.breakStack)-1]
		c.continueStack = c.continueStack[:len(c.continueStack)-1]

	case ast.ForStmt:
		loop := c.State.UniqueLabel(s.Loc)
		skip := c.State.UniqueLabel(s.Loc)
		c.breakStack = append(c.breakStack, skip.Ident)
		c.continueStack = append(c.continueStack, loop.Ident)
		c.Mem.OpenScope()
		c.Parse(s.Init)
		c.ParseNormElt(ast.LabelStmt{Loc: s.Loc, Label: loop})
		c.ParseElt(meta.MakeGotoUnless(s.Cond, skip))
		c.ParseElt(s.Body)
		c.Parse(s.Step)
		c.ParseElt(meta.MakeGoto(loop))
		c.ParseNormElt(ast.LabelStmt{Loc: s.Loc, Label: skip})
		c.Mem.CloseScope()
		c.breakStack = c.breakStack[:len(c.breakStack)-1]
		c.continueStack = c.continueStack[:len(c.continueStack)-1]

	case ast.CaseStmt:
		// TODO: full case/switch — compile-time case selection + goto_on/goto_case fallback
		c.warning(s.Loc, "case statement: TODO — full port pending")

	case ast.HidingStmt:
		// TODO: inline call hiding
		c.ParseElt(s.Body)

	default:
		c.warning(ast.Nowhere, fmt.Sprintf("unknown structure: %T", stmt))
	}
}

// HasErrors returns true if any errors were collected during compilation.
func (c *Compiler) HasErrors() bool { return len(c.Errors) > 0 }

func (c *Compiler) error(loc ast.Loc, msg string) {
	c.Errors = append(c.Errors, fmt.Errorf("%s: %s", loc, msg))
}

func (c *Compiler) warning(loc ast.Loc, msg string) {
	c.Warnings = append(c.Warnings, fmt.Sprintf("%s: %s", loc, msg))
}
