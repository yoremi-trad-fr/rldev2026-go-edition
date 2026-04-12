// Package meta implements global compiler state and compile-time
// code generation utilities for the Kepago compiler.
//
// Transposed from OCaml:
//   - rlc/global.ml (95 lines) — global mutable state, resources, unique IDs
//   - rlc/meta.ml (45 lines)   — compile-time AST generation helpers
//
// The meta package serves two purposes:
//
// 1. Global state (from global.ml): holds resources (#res strings),
//    dramatis personae (debug character list), the val_0x2c header field,
//    a unique label counter, and function references for breaking circular
//    dependencies between compiler modules.
//
// 2. Code generation helpers (from meta.ml): utility functions for
//    generating AST nodes at compile time — used by the compiler to emit
//    runtime function calls, assignments, gotos, etc. without going
//    through the parser.
package meta

import (
	"fmt"
	"sync/atomic"

	"github.com/yoremi/rldev-go/rlc/pkg/ast"
)

// ============================================================
// Global state (from global.ml)
// ============================================================

// State holds all mutable global compiler state.
type State struct {
	// Header data
	DramatisPersonae []string // debug: character name list
	Val0x2C          int      // header field at offset 0x2c (#Z-1)

	// Resources (#res strings)
	Resources map[string]Resource // key → resource string + location
	BaseRes   map[string]Resource // base resource strings

	// Unique label counter
	uniqueCounter int64

	// Gloss counter
	GlossCount int

	// Compile callbacks (for breaking circular dependencies)
	// These are set by the compiler package after initialization.
	CompileStatements func(stmts []ast.Stmt)
}

// Resource is a stored #res string with its source location.
type Resource struct {
	Text string
	Loc  ast.Loc
}

// NewState creates a fresh global state.
func NewState() *State {
	return &State{
		Resources: make(map[string]Resource),
		BaseRes:   make(map[string]Resource),
	}
}

// Unique returns a monotonically increasing unique ID for label generation.
func (s *State) Unique() int64 {
	return atomic.AddInt64(&s.uniqueCounter, 1) - 1
}

// UniqueLabel generates a unique label name at a given location.
func (s *State) UniqueLabel(loc ast.Loc) ast.Label {
	id := s.Unique()
	name := fmt.Sprintf("__auto@_%d__", id)
	return ast.Label{Loc: loc, Ident: name}
}

// GetResource looks up a resource string by key.
func (s *State) GetResource(key string) (Resource, error) {
	r, ok := s.Resources[key]
	if !ok {
		return Resource{}, fmt.Errorf("undefined resource string '%s'", key)
	}
	return r, nil
}

// GetBaseResource looks up a base resource string by key.
func (s *State) GetBaseResource(key string) (Resource, error) {
	r, ok := s.BaseRes[key]
	if !ok {
		return Resource{}, fmt.Errorf("undefined base resource string '%s'", key)
	}
	return r, nil
}

// SetResource stores a resource string.
func (s *State) SetResource(key string, text string, loc ast.Loc) {
	s.Resources[key] = Resource{Text: text, Loc: loc}
}

// SetBaseResource stores a base resource string.
func (s *State) SetBaseResource(key string, text string, loc ast.Loc) {
	s.BaseRes[key] = Resource{Text: text, Loc: loc}
}

// AddCharacter adds a character name to the dramatis personae list.
func (s *State) AddCharacter(name string) {
	s.DramatisPersonae = append(s.DramatisPersonae, name)
}

// ============================================================
// Compile-time integer padding (from global.ml int32_to_string_padded)
// ============================================================

// Int32PaddedString converts an int32 to a zero-padded string.
// If the result is shorter than width, it's left-padded with '0'.
func Int32PaddedString(width int, value int32) string {
	s := fmt.Sprintf("%d", value)
	if d := width - len(s); d > 0 {
		pad := make([]byte, d)
		for i := range pad {
			pad[i] = '0'
		}
		return string(pad) + s
	}
	return s
}

// ============================================================
// AST generation helpers (from meta.ml)
// ============================================================

// IntExpr creates an integer literal expression.
func IntExpr(v int) ast.Expr {
	return ast.IntLit{Loc: ast.Nowhere, Val: int32(v)}
}

// ZeroExpr is the integer literal 0.
func ZeroExpr() ast.Expr {
	return ast.IntLit{Loc: ast.Nowhere, Val: 0}
}

// MakeAssign creates an assignment statement.
func MakeAssign(dest ast.Expr, op ast.AssignOp, rhs ast.Expr) ast.Stmt {
	return ast.AssignStmt{Loc: ast.Nowhere, Dest: dest, Op: op, Expr: rhs}
}

// MakeCall creates a function call statement.
func MakeCall(funcName string, args []ast.Expr, label *ast.Label) ast.Stmt {
	params := make([]ast.Param, len(args))
	for i, a := range args {
		params[i] = ast.SimpleParam{Loc: ast.Nowhere, Expr: a}
	}
	return ast.FuncCallStmt{
		Loc:    ast.Nowhere,
		Ident:  funcName,
		Params: params,
		Label:  label,
	}
}

// MakeGoto creates a goto statement.
func MakeGoto(label ast.Label) ast.Stmt {
	return MakeCall("goto", nil, &label)
}

// MakeGosub creates a gosub statement.
func MakeGosub(label ast.Label) ast.Stmt {
	return MakeCall("gosub", nil, &label)
}

// MakeGotoUnless creates a goto_unless(cond) @label statement.
func MakeGotoUnless(cond ast.Expr, label ast.Label) ast.Stmt {
	return MakeCall("goto_unless", []ast.Expr{cond}, &label)
}

// MakeReturn creates a return statement.
func MakeReturn(expr ast.Expr) ast.Stmt {
	return ast.ReturnStmt{Loc: ast.Nowhere, Explicit: false, Expr: expr}
}

// MakeVarOrFunc creates a VarOrFunc expression from a name.
func MakeVarOrFunc(name string) ast.Expr {
	return ast.VarOrFunc{Loc: ast.Nowhere, Ident: name}
}

// Parse compiles a list of statements through the compiler pipeline.
// This is the meta.ml parse function — it delegates to the compiler.
func (s *State) Parse(stmts []ast.Stmt) {
	if s.CompileStatements != nil {
		s.CompileStatements(stmts)
	}
}

// ParseOne compiles a single statement.
func (s *State) ParseOne(stmt ast.Stmt) {
	s.Parse([]ast.Stmt{stmt})
}
