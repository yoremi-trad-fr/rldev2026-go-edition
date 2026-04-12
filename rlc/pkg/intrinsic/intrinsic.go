// Package intrinsic implements compile-time builtin functions for Kepago.
//
// Transposed from OCaml's rlc/intrinsic.ml (410 lines).
//
// Intrinsic functions are evaluated at compile time, not at runtime.
// They provide metaprogramming capabilities:
//
//   - defined?(sym)       — test if a symbol is defined
//   - default(sym, expr)  — return sym if defined, else expr
//   - constant?(expr)     — test if expression is a compile-time constant
//   - integer?(expr)      — test if expression is integer-typed
//   - array?(sym)         — test if symbol is an array variable
//   - length(sym)         — get array length
//   - __deref(space, idx) — raw integer variable access: intN[idx]
//   - __sderef(space, idx)— raw string variable access: strN[idx]
//   - __variable?(expr)   — test if expression resolves to a variable
//   - __addr(var)         — get encoded address (space|index) of a variable
//   - __ident(str)        — convert string constant to an identifier
//   - __empty_string?(s)  — test if string constant is empty
//   - __equal_strings?(a,b) — compare two string constants
//   - kinetic?()          — test if target is Kinetic
//   - target_lt/le/gt/ge(v...) — compare current version
//   - gameexe(key[,idx[,default]]) — read from GAMEEXE.INI
//   - at(file, line, expr) — evaluate expression with different location
//   - rlc_parse_string(s) — parse string as Kepago code
package intrinsic

import (
	"fmt"

	"github.com/yoremi/rldev-go/rlc/pkg/ast"
	"github.com/yoremi/rldev-go/rlc/pkg/kfn"
	"github.com/yoremi/rldev-go/rlc/pkg/memory"
)

// ============================================================
// Registry
// ============================================================

// ExprFunc evaluates an intrinsic as an expression.
type ExprFunc func(loc ast.Loc, params []ast.Param) (ast.Expr, error)

// Builtin represents one intrinsic function.
type Builtin struct {
	Name     string
	EvalExpr ExprFunc
}

// Registry holds all registered intrinsic functions.
type Registry struct {
	builtins map[string]*Builtin
	mem      *memory.Memory
	target   kfn.Target
	version  kfn.Version
}

// New creates a registry with all standard intrinsics registered.
func New(mem *memory.Memory) *Registry {
	r := &Registry{
		builtins: make(map[string]*Builtin),
		mem:      mem,
		target:   kfn.TargetDefault,
		version:  kfn.Version{1, 2, 7, 0},
	}
	r.registerAll()
	return r
}

// SetTarget configures the target engine and version.
func (r *Registry) SetTarget(t kfn.Target, v kfn.Version) {
	r.target = t
	r.version = v
}

// IsBuiltin returns true if the identifier is an intrinsic function.
func (r *Registry) IsBuiltin(name string) bool {
	_, ok := r.builtins[name]
	return ok
}

// EvalAsExpr evaluates an intrinsic as an expression.
func (r *Registry) EvalAsExpr(name string, loc ast.Loc, params []ast.Param) (ast.Expr, error) {
	b, ok := r.builtins[name]
	if !ok {
		return nil, fmt.Errorf("unknown intrinsic '%s'", name)
	}
	return b.EvalExpr(loc, params)
}

func (r *Registry) register(name string, fn ExprFunc) {
	r.builtins[name] = &Builtin{Name: name, EvalExpr: fn}
}

// ============================================================
// Intrinsic implementations
// ============================================================

func (r *Registry) registerAll() {
	r.register("defined?", r.builtinDefined)
	r.register("default", r.builtinDefault)
	r.register("constant?", r.builtinConstant)
	r.register("integer?", r.builtinInteger)
	r.register("array?", r.builtinArray)
	r.register("length", r.builtinLength)
	r.register("__deref", r.builtinDeref)
	r.register("__sderef", r.builtinSDeref)
	r.register("__variable?", r.builtinVariable)
	r.register("__addr", r.builtinAddr)
	r.register("__ident", r.builtinIdent)
	r.register("__empty_string?", r.builtinEmptyString)
	r.register("__equal_strings?", r.builtinEqualStrings)
	r.register("kinetic?", r.builtinKinetic)
	r.register("target_lt", r.makeTargetCmp("target_lt", func(cur, ref kfn.Version) bool { return versionLess(cur, ref) }))
	r.register("target_le", r.makeTargetCmp("target_le", func(cur, ref kfn.Version) bool { return !versionLess(ref, cur) }))
	r.register("target_gt", r.makeTargetCmp("target_gt", func(cur, ref kfn.Version) bool { return versionLess(ref, cur) }))
	r.register("target_ge", r.makeTargetCmp("target_ge", func(cur, ref kfn.Version) bool { return !versionLess(cur, ref) }))
}

// --- defined?(sym1, sym2, ...) ---
// Returns 1 if ALL symbols are defined, 0 otherwise.
func (r *Registry) builtinDefined(loc ast.Loc, params []ast.Param) (ast.Expr, error) {
	for _, p := range params {
		sp, ok := p.(ast.SimpleParam)
		if !ok {
			return nil, fmt.Errorf("%s: defined? must be passed only simple identifiers", loc)
		}
		vf, ok := sp.Expr.(ast.VarOrFunc)
		if !ok {
			return nil, fmt.Errorf("%s: defined? must be passed only simple identifiers", loc)
		}
		if !r.mem.Defined(vf.Ident) {
			return ast.IntLit{Loc: loc, Val: 0}, nil
		}
	}
	return ast.IntLit{Loc: loc, Val: 1}, nil
}

// --- default(sym, fallback) ---
// Returns sym if defined, else fallback.
func (r *Registry) builtinDefault(loc ast.Loc, params []ast.Param) (ast.Expr, error) {
	if len(params) != 2 {
		return nil, fmt.Errorf("%s: default must be passed a symbol and an expression", loc)
	}
	sp0, ok0 := params[0].(ast.SimpleParam)
	sp1, ok1 := params[1].(ast.SimpleParam)
	if !ok0 || !ok1 {
		return nil, fmt.Errorf("%s: default must be passed a symbol and an expression", loc)
	}
	if vf, ok := sp0.Expr.(ast.VarOrFunc); ok && r.mem.Defined(vf.Ident) {
		return sp0.Expr, nil
	}
	return sp1.Expr, nil
}

// --- constant?(expr1, expr2, ...) ---
// Returns 1 if ALL expressions are compile-time integer constants.
func (r *Registry) builtinConstant(loc ast.Loc, params []ast.Param) (ast.Expr, error) {
	for _, p := range params {
		sp, ok := p.(ast.SimpleParam)
		if !ok {
			return ast.IntLit{Loc: loc, Val: 0}, nil
		}
		if _, ok := sp.Expr.(ast.IntLit); !ok {
			return ast.IntLit{Loc: loc, Val: 0}, nil
		}
	}
	return ast.IntLit{Loc: loc, Val: 1}, nil
}

// --- integer?(expr1, ...) ---
// Returns 1 if ALL expressions are integer-typed.
func (r *Registry) builtinInteger(loc ast.Loc, params []ast.Param) (ast.Expr, error) {
	for _, p := range params {
		sp, ok := p.(ast.SimpleParam)
		if !ok {
			return ast.IntLit{Loc: loc, Val: 0}, nil
		}
		if isStrTyped(sp.Expr) {
			return ast.IntLit{Loc: loc, Val: 0}, nil
		}
	}
	return ast.IntLit{Loc: loc, Val: 1}, nil
}

func isStrTyped(e ast.Expr) bool {
	switch x := e.(type) {
	case ast.StrLit, ast.StrVar:
		return true
	case ast.BinOp:
		return isStrTyped(x.LHS)
	case ast.ParenExpr:
		return isStrTyped(x.Expr)
	}
	return false
}

// --- array?(sym1, ...) ---
// Returns 1 if ALL symbols are arrays.
func (r *Registry) builtinArray(loc ast.Loc, params []ast.Param) (ast.Expr, error) {
	for _, p := range params {
		sp, ok := p.(ast.SimpleParam)
		if !ok {
			return nil, fmt.Errorf("%s: array? must be passed only simple identifiers", loc)
		}
		vf, ok := sp.Expr.(ast.VarOrFunc)
		if !ok {
			return nil, fmt.Errorf("%s: array? must be passed only simple identifiers", loc)
		}
		sym, ok := r.mem.Get(vf.Ident)
		if !ok || sym.Kind != memory.KindStaticVar || sym.Var == nil || sym.Var.ArrayLen <= 0 {
			return ast.IntLit{Loc: loc, Val: 0}, nil
		}
	}
	return ast.IntLit{Loc: loc, Val: 1}, nil
}

// --- length(sym) ---
// Returns the array length of a symbol.
func (r *Registry) builtinLength(loc ast.Loc, params []ast.Param) (ast.Expr, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("%s: length must be passed a single array variable", loc)
	}
	sp, ok := params[0].(ast.SimpleParam)
	if !ok {
		return nil, fmt.Errorf("%s: length must be passed a single array variable", loc)
	}
	vf, ok := sp.Expr.(ast.VarOrFunc)
	if !ok {
		return nil, fmt.Errorf("%s: length must be passed a single identifier", loc)
	}
	sym, ok := r.mem.Get(vf.Ident)
	if !ok || sym.Kind != memory.KindStaticVar || sym.Var == nil || sym.Var.ArrayLen <= 0 {
		return nil, fmt.Errorf("%s: '%s' is not an array", loc, vf.Ident)
	}
	return ast.IntLit{Loc: loc, Val: int32(sym.Var.ArrayLen)}, nil
}

// --- __deref(space, expr) ---
// Raw integer variable access: intN[expr].
func (r *Registry) builtinDeref(loc ast.Loc, params []ast.Param) (ast.Expr, error) {
	if len(params) != 2 {
		return nil, fmt.Errorf("%s: __deref must be passed an integer constant and an expression", loc)
	}
	sp0, ok0 := params[0].(ast.SimpleParam)
	sp1, ok1 := params[1].(ast.SimpleParam)
	if !ok0 || !ok1 {
		return nil, fmt.Errorf("%s: __deref must be passed an integer constant and an expression", loc)
	}
	spaceLit, ok := sp0.Expr.(ast.IntLit)
	if !ok {
		return nil, fmt.Errorf("%s: __deref first argument must be an integer constant", loc)
	}
	return ast.IntVar{Loc: loc, Bank: int(spaceLit.Val), Index: sp1.Expr}, nil
}

// --- __sderef(space, expr) ---
// Raw string variable access: strN[expr].
func (r *Registry) builtinSDeref(loc ast.Loc, params []ast.Param) (ast.Expr, error) {
	if len(params) != 2 {
		return nil, fmt.Errorf("%s: __sderef must be passed an integer constant and an expression", loc)
	}
	sp0, ok0 := params[0].(ast.SimpleParam)
	sp1, ok1 := params[1].(ast.SimpleParam)
	if !ok0 || !ok1 {
		return nil, fmt.Errorf("%s: __sderef must be passed simple expressions", loc)
	}
	spaceLit, ok := sp0.Expr.(ast.IntLit)
	if !ok {
		return nil, fmt.Errorf("%s: __sderef first argument must be an integer constant", loc)
	}
	return ast.StrVar{Loc: loc, Bank: int(spaceLit.Val), Index: sp1.Expr}, nil
}

// --- __variable?(expr) ---
// Returns 1 if the expression resolves to a variable (IVar or SVar).
func (r *Registry) builtinVariable(loc ast.Loc, params []ast.Param) (ast.Expr, error) {
	if len(params) != 1 {
		return ast.IntLit{Loc: loc, Val: 0}, nil
	}
	sp, ok := params[0].(ast.SimpleParam)
	if !ok {
		return ast.IntLit{Loc: loc, Val: 0}, nil
	}
	switch sp.Expr.(type) {
	case ast.IntVar, ast.StrVar:
		return ast.IntLit{Loc: loc, Val: 1}, nil
	case ast.VarOrFunc:
		vf := sp.Expr.(ast.VarOrFunc)
		expr, err := r.mem.GetAsExpr(vf.Ident, loc)
		if err != nil {
			return ast.IntLit{Loc: loc, Val: 0}, nil
		}
		switch expr.(type) {
		case ast.IntVar, ast.StrVar:
			return ast.IntLit{Loc: loc, Val: 1}, nil
		}
	case ast.Deref:
		d := sp.Expr.(ast.Deref)
		expr, err := r.mem.GetDerefAsExpr(d.Ident, d.Index, loc)
		if err != nil {
			return ast.IntLit{Loc: loc, Val: 0}, nil
		}
		switch expr.(type) {
		case ast.IntVar, ast.StrVar:
			return ast.IntLit{Loc: loc, Val: 1}, nil
		}
	}
	return ast.IntLit{Loc: loc, Val: 0}, nil
}

// --- __addr(var) ---
// Returns (index | (space << 16)) for a variable.
func (r *Registry) builtinAddr(loc ast.Loc, params []ast.Param) (ast.Expr, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("%s: __addr must be passed a single variable", loc)
	}
	sp, ok := params[0].(ast.SimpleParam)
	if !ok {
		return nil, fmt.Errorf("%s: __addr must be passed a single variable", loc)
	}
	space, indexExpr, err := r.resolveVarSpaceIndex(loc, sp.Expr)
	if err != nil {
		return nil, err
	}
	// index | (space << 16)
	return ast.BinOp{
		Loc: loc, Op: ast.OpOr,
		LHS: indexExpr,
		RHS: ast.BinOp{
			Loc: loc, Op: ast.OpShl,
			LHS: ast.IntLit{Loc: loc, Val: int32(space)},
			RHS: ast.IntLit{Loc: loc, Val: 16},
		},
	}, nil
}

func (r *Registry) resolveVarSpaceIndex(loc ast.Loc, e ast.Expr) (int, ast.Expr, error) {
	switch x := e.(type) {
	case ast.IntVar:
		return x.Bank, x.Index, nil
	case ast.StrVar:
		return x.Bank, x.Index, nil
	case ast.VarOrFunc:
		resolved, err := r.mem.GetAsExpr(x.Ident, loc)
		if err != nil {
			return 0, nil, fmt.Errorf("%s: __addr: '%s' is not a variable", loc, x.Ident)
		}
		return r.resolveVarSpaceIndex(loc, resolved)
	case ast.Deref:
		resolved, err := r.mem.GetDerefAsExpr(x.Ident, x.Index, loc)
		if err != nil {
			return 0, nil, err
		}
		return r.resolveVarSpaceIndex(loc, resolved)
	}
	return 0, nil, fmt.Errorf("%s: __addr must be passed a variable", loc)
}

// --- __ident(str) ---
// Converts a string constant to an identifier reference.
func (r *Registry) builtinIdent(loc ast.Loc, params []ast.Param) (ast.Expr, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("%s: __ident must be passed a single string constant", loc)
	}
	sp, ok := params[0].(ast.SimpleParam)
	if !ok {
		return nil, fmt.Errorf("%s: __ident must be passed a single string constant", loc)
	}
	slit, ok := sp.Expr.(ast.StrLit)
	if !ok || len(slit.Tokens) == 0 {
		return nil, fmt.Errorf("%s: __ident must be passed a string constant", loc)
	}
	tt, ok := slit.Tokens[0].(ast.TextToken)
	if !ok {
		return nil, fmt.Errorf("%s: __ident string must be a simple text", loc)
	}
	return ast.VarOrFunc{Loc: loc, Ident: tt.Text}, nil
}

// --- __empty_string?(str) ---
// Returns 1 if the string constant is empty.
func (r *Registry) builtinEmptyString(loc ast.Loc, params []ast.Param) (ast.Expr, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("%s: __empty_string? must be passed a single string constant", loc)
	}
	sp, ok := params[0].(ast.SimpleParam)
	if !ok {
		return nil, fmt.Errorf("%s: __empty_string? must be passed a single string constant", loc)
	}
	slit, ok := sp.Expr.(ast.StrLit)
	if !ok {
		return nil, fmt.Errorf("%s: __empty_string? must be passed a string constant", loc)
	}
	isEmpty := len(slit.Tokens) == 0
	if !isEmpty && len(slit.Tokens) == 1 {
		if tt, ok := slit.Tokens[0].(ast.TextToken); ok {
			isEmpty = tt.Text == ""
		}
	}
	if isEmpty {
		return ast.IntLit{Loc: loc, Val: 1}, nil
	}
	return ast.IntLit{Loc: loc, Val: 0}, nil
}

// --- __equal_strings?(s1, s2) ---
// Compares two string constants. Returns 1 if equal.
func (r *Registry) builtinEqualStrings(loc ast.Loc, params []ast.Param) (ast.Expr, error) {
	if len(params) != 2 {
		return nil, fmt.Errorf("%s: __equal_strings? must be passed two string constants", loc)
	}
	s1 := extractStrConst(params[0])
	s2 := extractStrConst(params[1])
	if s1 == s2 {
		return ast.IntLit{Loc: loc, Val: 1}, nil
	}
	return ast.IntLit{Loc: loc, Val: 0}, nil
}

func extractStrConst(p ast.Param) string {
	sp, ok := p.(ast.SimpleParam)
	if !ok {
		return "\x00__not_a_string__"
	}
	slit, ok := sp.Expr.(ast.StrLit)
	if !ok || len(slit.Tokens) == 0 {
		return ""
	}
	if tt, ok := slit.Tokens[0].(ast.TextToken); ok {
		return tt.Text
	}
	return "\x00__complex_token__"
}

// --- kinetic?() ---
// Returns 1 if the current target is Kinetic.
func (r *Registry) builtinKinetic(loc ast.Loc, params []ast.Param) (ast.Expr, error) {
	if len(params) > 0 {
		return nil, fmt.Errorf("%s: kinetic? takes no parameters", loc)
	}
	if r.target == kfn.TargetKinetic {
		return ast.IntLit{Loc: loc, Val: 1}, nil
	}
	return ast.IntLit{Loc: loc, Val: 0}, nil
}

// --- target_lt/le/gt/ge(major[, minor[, patch[, build]]]) ---

func (r *Registry) makeTargetCmp(name string, cmp func(kfn.Version, kfn.Version) bool) ExprFunc {
	return func(loc ast.Loc, params []ast.Param) (ast.Expr, error) {
		ref, err := extractVersionArgs(loc, name, params)
		if err != nil {
			return nil, err
		}
		if cmp(r.version, ref) {
			return ast.IntLit{Loc: loc, Val: 1}, nil
		}
		return ast.IntLit{Loc: loc, Val: 0}, nil
	}
}

func extractVersionArgs(loc ast.Loc, name string, params []ast.Param) (kfn.Version, error) {
	if len(params) < 1 || len(params) > 4 {
		return kfn.Version{}, fmt.Errorf("%s: %s must be passed 1 to 4 parameters", loc, name)
	}
	var v kfn.Version
	for i, p := range params {
		sp, ok := p.(ast.SimpleParam)
		if !ok {
			return kfn.Version{}, fmt.Errorf("%s: %s parameters must be simple expressions", loc, name)
		}
		lit, ok := sp.Expr.(ast.IntLit)
		if !ok {
			return kfn.Version{}, fmt.Errorf("%s: %s parameters must evaluate to integer constants", loc, name)
		}
		v[i] = int(lit.Val)
	}
	return v, nil
}

func versionLess(a, b kfn.Version) bool {
	for i := 0; i < 4; i++ {
		if a[i] < b[i] {
			return true
		}
		if a[i] > b[i] {
			return false
		}
	}
	return false
}

// Names returns the list of all registered intrinsic names.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.builtins))
	for name := range r.builtins {
		names = append(names, name)
	}
	return names
}
