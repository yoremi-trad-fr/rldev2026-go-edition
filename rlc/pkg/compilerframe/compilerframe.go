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
	"github.com/yoremi/rldev-go/rlc/pkg/ini"
	"github.com/yoremi/rldev-go/rlc/pkg/intrinsic"
	"github.com/yoremi/rldev-go/rlc/pkg/kfn"
	"github.com/yoremi/rldev-go/rlc/pkg/memory"
	"github.com/yoremi/rldev-go/rlc/pkg/meta"
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
//
// Currently implemented:
//   - All directive statements (delegated to directive.Compile)
//   - Halt statement (emits 0x00 opcode)
//
// TODO: structure dispatch (if/while/for/switch/block), textout handling,
// expression normalization with NormNothing/NormSingle/NormMultiple
// results, and scope management for temporary variables.
func (c *Compiler) ParseElt(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case ast.DirectiveStmt,
		ast.DTargetStmt,
		ast.DefineStmt,
		ast.DConstStmt,
		ast.DInlineStmt,
		ast.DUndefStmt,
		ast.DSetStmt,
		ast.DVersionStmt:
		c.Directive.Compile(s)
	case ast.HaltStmt:
		c.Out.AddCode(ast.Nowhere, []byte{0x00})
	default:
		c.warning(ast.Nowhere, fmt.Sprintf("TODO: ParseElt not yet implemented for %T", stmt))
	}
}

// ParseNormElt processes a fully-normalized statement.
// Corresponds to parse_norm_elt (line 710) in compilerFrame.ml.
//
// TODO: full implementation. See package doc for the dispatch table.
func (c *Compiler) ParseNormElt(stmt ast.Stmt) {
	c.warning(ast.Nowhere, fmt.Sprintf("TODO: ParseNormElt not yet implemented for %T", stmt))
}

// HasErrors returns true if any errors were collected during compilation.
func (c *Compiler) HasErrors() bool { return len(c.Errors) > 0 }

func (c *Compiler) error(loc ast.Loc, msg string) {
	c.Errors = append(c.Errors, fmt.Errorf("%s: %s", loc, msg))
}

func (c *Compiler) warning(loc ast.Loc, msg string) {
	c.Warnings = append(c.Warnings, fmt.Sprintf("%s: %s", loc, msg))
}
