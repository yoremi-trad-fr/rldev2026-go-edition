// Package expr implements expression normalization and transformation
// for the Kepago compiler.
//
// Transposed from OCaml's rlc/expr.ml (~1383 lines).
//
// The expression pipeline transforms raw parsed AST expressions into
// normalized forms suitable for bytecode generation. Key operations:
//
//   - Constant folding (arithmetic, comparison, boolean, unary)
//   - Algebraic simplification (x+0→x, x*1→x, x&-1→x, x-x→0, etc.)
//   - Symbol resolution (VarOrFunc → variable/macro/intrinsic)
//   - Condition normalization (expr → LogOp(expr, Neq, 0))
//   - Operator precedence parenthesization for bytecode
//   - Type checking (int vs string context)
//
// The normalizer is used by the compiler for:
//   - Assignment RHS normalization
//   - Function call parameter normalization
//   - #if / #const / #define constant evaluation
//   - Goto/select expression normalization
package expr

import (
	"fmt"

	"github.com/yoremi/rldev-go/rlc/pkg/ast"
	"github.com/yoremi/rldev-go/rlc/pkg/memory"
)

// ============================================================
// Operator utilities
// ============================================================

// Prec returns the RealLive bytecode precedence of an arithmetic operator.
// Add/Sub = 10, everything else = 20.
func Prec(op ast.ArithOp) int {
	switch op {
	case ast.OpAdd, ast.OpSub:
		return 10
	}
	return 20
}

// ApplyArith evaluates a binary arithmetic operation on constants.
func ApplyArith(a, b int32, op ast.ArithOp) int32 {
	switch op {
	case ast.OpAdd: return a + b
	case ast.OpSub: return a - b
	case ast.OpMul: return a * b
	case ast.OpDiv:
		if b != 0 { return a / b }
		return 0
	case ast.OpMod:
		if b != 0 { return a % b }
		return 0
	case ast.OpAnd: return a & b
	case ast.OpOr:  return a | b
	case ast.OpXor: return a ^ b
	case ast.OpShl: return a << uint(b)
	case ast.OpShr: return a >> uint(b)
	}
	return 0
}

// ApplyUnary evaluates a unary operation on a constant.
func ApplyUnary(val int32, op ast.UnaryOp) int32 {
	switch op {
	case ast.UnarySub: return -val
	case ast.UnaryNot:
		if val == 0 { return 1 }
		return 0
	case ast.UnaryInv: return ^val
	}
	return val
}

// ApplyCmp evaluates a comparison on constants.
func ApplyCmp(a, b int32, op ast.CmpOp) int32 {
	var result bool
	switch op {
	case ast.CmpEqu: result = a == b
	case ast.CmpNeq: result = a != b
	case ast.CmpLtn: result = a < b
	case ast.CmpLte: result = a <= b
	case ast.CmpGtn: result = a > b
	case ast.CmpGte: result = a >= b
	}
	if result { return 1 }
	return 0
}

// ReverseCmp inverts a comparison operator.
func ReverseCmp(op ast.CmpOp) ast.CmpOp {
	switch op {
	case ast.CmpEqu: return ast.CmpNeq
	case ast.CmpNeq: return ast.CmpEqu
	case ast.CmpLtn: return ast.CmpGte
	case ast.CmpLte: return ast.CmpGtn
	case ast.CmpGtn: return ast.CmpLte
	case ast.CmpGte: return ast.CmpLtn
	}
	return op
}

// ============================================================
// Expression type classification
// ============================================================

// RejectType controls type checking during normalization.
type RejectType int

const (
	RejectNone RejectType = iota // accept both int and string
	RejectInt                     // reject integers (string context)
	RejectStr                     // reject strings (integer context)
)

// ============================================================
// Normalizer — the main expression transformation engine
// ============================================================

// Normalizer transforms expressions using the symbol table.
type Normalizer struct {
	Mem     *memory.Memory
	Errors  []error
	labelID int
}

// NewNormalizer creates a normalizer with a symbol table.
func NewNormalizer(mem *memory.Memory) *Normalizer {
	return &Normalizer{Mem: mem}
}

func (n *Normalizer) error(loc ast.Loc, msg string) {
	n.Errors = append(n.Errors, fmt.Errorf("%s: %s", loc, msg))
}

func (n *Normalizer) errorf(loc ast.Loc, format string, args ...interface{}) {
	n.Errors = append(n.Errors, fmt.Errorf("%s: %s", loc, fmt.Sprintf(format, args...)))
}

// ============================================================
// Symbol resolution (expr_disambiguate)
// ============================================================

// Disambiguate resolves a VarOrFunc or Deref through the symbol table.
// Returns the resolved expression, or the original if unresolved.
func (n *Normalizer) Disambiguate(e ast.Expr) ast.Expr {
	switch x := e.(type) {
	case ast.VarOrFunc:
		if !n.Mem.Defined(x.Ident) {
			return e // leave for later resolution (might be a function)
		}
		resolved, err := n.Mem.GetAsExpr(x.Ident, x.Loc)
		if err != nil {
			return e
		}
		return resolved

	case ast.Deref:
		if !n.Mem.Defined(x.Ident) {
			return e
		}
		resolved, err := n.Mem.GetDerefAsExpr(x.Ident, x.Index, x.Loc)
		if err != nil {
			n.error(x.Loc, err.Error())
			return e
		}
		return resolved
	}
	return e
}

// ============================================================
// Constant folding
// ============================================================

// ConstFold attempts full constant evaluation of an expression.
// Resolves symbols, folds arithmetic, and simplifies.
func (n *Normalizer) ConstFold(e ast.Expr) ast.Expr {
	switch x := e.(type) {
	case ast.IntLit:
		return x
	case ast.StrLit:
		return x

	case ast.VarOrFunc:
		resolved := n.Disambiguate(x)
		if resolved != e {
			return n.ConstFold(resolved)
		}
		return e

	case ast.Deref:
		resolved := n.Disambiguate(x)
		if resolved != e {
			return n.ConstFold(resolved)
		}
		return e

	case ast.BinOp:
		lhs := n.ConstFold(x.LHS)
		rhs := n.ConstFold(x.RHS)
		return n.foldBinOp(x.Loc, lhs, x.Op, rhs)

	case ast.CmpExpr:
		lhs := n.ConstFold(x.LHS)
		rhs := n.ConstFold(x.RHS)
		return n.foldCmpExpr(x.Loc, lhs, x.Op, rhs)

	case ast.ChainExpr:
		lhs := n.ConstFold(x.LHS)
		rhs := n.ConstFold(x.RHS)
		return n.foldChainExpr(x.Loc, lhs, x.Op, rhs)

	case ast.UnaryExpr:
		val := n.ConstFold(x.Val)
		return n.foldUnaryExpr(x.Loc, x.Op, val)

	case ast.ParenExpr:
		inner := n.ConstFold(x.Expr)
		// Remove superfluous parens around atoms
		switch inner.(type) {
		case ast.IntLit, ast.StoreRef, ast.IntVar, ast.StrVar, ast.StrLit:
			return inner
		}
		return ast.ParenExpr{Loc: x.Loc, Expr: inner}

	case ast.FuncCall:
		// Fold parameters
		params := make([]ast.Param, len(x.Params))
		for i, p := range x.Params {
			params[i] = n.foldParam(p)
		}
		return ast.FuncCall{Loc: x.Loc, Ident: x.Ident, Params: params, Label: x.Label}
	}
	return e
}

// foldParam folds constants in a parameter.
func (n *Normalizer) foldParam(p ast.Param) ast.Param {
	switch x := p.(type) {
	case ast.SimpleParam:
		return ast.SimpleParam{Loc: x.Loc, Expr: n.ConstFold(x.Expr)}
	case ast.ComplexParam:
		exprs := make([]ast.Expr, len(x.Exprs))
		for i, e := range x.Exprs {
			exprs[i] = n.ConstFold(e)
		}
		return ast.ComplexParam{Loc: x.Loc, Exprs: exprs}
	}
	return p
}

// --- Binary operation folding with algebraic simplification ---

func (n *Normalizer) foldBinOp(loc ast.Loc, lhs ast.Expr, op ast.ArithOp, rhs ast.Expr) ast.Expr {
	li, lok := lhs.(ast.IntLit)
	ri, rok := rhs.(ast.IntLit)

	// Both constant → evaluate
	if lok && rok {
		// Division by zero check
		if (op == ast.OpDiv || op == ast.OpMod) && ri.Val == 0 {
			n.error(loc, "division by zero")
			return ast.IntLit{Loc: loc, Val: 0}
		}
		return ast.IntLit{Loc: loc, Val: ApplyArith(li.Val, ri.Val, op)}
	}

	// --- Algebraic simplifications (from OCaml expr.ml lines 424-451) ---

	// x & -1 → x, -1 & x → x
	if op == ast.OpAnd {
		if rok && ri.Val == -1 { return lhs }
		if lok && li.Val == -1 { return rhs }
	}

	// x | 0 → x, 0 | x → x, x ^ 0 → x, 0 ^ x → x
	if op == ast.OpOr || op == ast.OpXor {
		if rok && ri.Val == 0 { return lhs }
		if lok && li.Val == 0 { return rhs }
	}

	// x + 0 → x, 0 + x → x, x - 0 → x
	if (op == ast.OpAdd || op == ast.OpSub) && rok && ri.Val == 0 { return lhs }
	if op == ast.OpAdd && lok && li.Val == 0 { return rhs }

	// x * 1 → x, 1 * x → x, x / 1 → x
	if (op == ast.OpMul || op == ast.OpDiv) && rok && ri.Val == 1 { return lhs }
	if op == ast.OpMul && lok && li.Val == 1 { return rhs }

	// x & 0 → 0, 0 & x → 0, x * 0 → 0, 0 * x → 0, 0 / x → 0, 0 % x → 0
	if (op == ast.OpAnd || op == ast.OpMul) && rok && ri.Val == 0 { return ast.IntLit{Loc: loc, Val: 0} }
	if (op == ast.OpAnd || op == ast.OpMul || op == ast.OpDiv || op == ast.OpMod) && lok && li.Val == 0 {
		return ast.IntLit{Loc: loc, Val: 0}
	}

	// x & x → x, x | x → x
	if (op == ast.OpAnd || op == ast.OpOr) && EqualExpr(lhs, rhs) { return lhs }
	// x - x → 0, x ^ x → 0, x % x → 0
	if (op == ast.OpSub || op == ast.OpXor || op == ast.OpMod) && EqualExpr(lhs, rhs) {
		return ast.IntLit{Loc: loc, Val: 0}
	}
	// x / x → 1
	if op == ast.OpDiv && EqualExpr(lhs, rhs) {
		return ast.IntLit{Loc: loc, Val: 1}
	}

	// x + (-n) → x - n, x - (-n) → x + n
	if rok && ri.Val < 0 {
		if op == ast.OpAdd {
			return ast.BinOp{Loc: loc, LHS: lhs, Op: ast.OpSub, RHS: ast.IntLit{Loc: ri.Loc, Val: -ri.Val}}
		}
		if op == ast.OpSub {
			return ast.BinOp{Loc: loc, LHS: lhs, Op: ast.OpAdd, RHS: ast.IntLit{Loc: ri.Loc, Val: -ri.Val}}
		}
	}

	// Parenthesize for bytecode precedence
	a := lhs
	if bo, ok := lhs.(ast.BinOp); ok && Prec(bo.Op) < Prec(op) {
		a = ast.ParenExpr{Loc: loc, Expr: lhs}
	}
	b := rhs
	if bo, ok := rhs.(ast.BinOp); ok && Prec(bo.Op) <= Prec(op) {
		b = ast.ParenExpr{Loc: loc, Expr: rhs}
	}
	return ast.BinOp{Loc: loc, LHS: a, Op: op, RHS: b}
}

// --- Comparison folding ---

func (n *Normalizer) foldCmpExpr(loc ast.Loc, lhs ast.Expr, op ast.CmpOp, rhs ast.Expr) ast.Expr {
	li, lok := lhs.(ast.IntLit)
	ri, rok := rhs.(ast.IntLit)
	if lok && rok {
		return ast.IntLit{Loc: loc, Val: ApplyCmp(li.Val, ri.Val, op)}
	}
	// x == x → 1, x >= x → 1, x <= x → 1
	if (op == ast.CmpEqu || op == ast.CmpGte || op == ast.CmpLte) && EqualExpr(lhs, rhs) {
		return ast.IntLit{Loc: loc, Val: 1}
	}
	// x != x → 0, x > x → 0, x < x → 0
	if (op == ast.CmpNeq || op == ast.CmpGtn || op == ast.CmpLtn) && EqualExpr(lhs, rhs) {
		return ast.IntLit{Loc: loc, Val: 0}
	}
	return ast.CmpExpr{Loc: loc, LHS: lhs, Op: op, RHS: rhs}
}

// --- Chain (&&/||) folding ---

func (n *Normalizer) foldChainExpr(loc ast.Loc, lhs ast.Expr, op ast.ChainOp, rhs ast.Expr) ast.Expr {
	li, lok := lhs.(ast.IntLit)
	ri, rok := rhs.(ast.IntLit)

	if lok && rok {
		if op == ast.ChainAnd {
			if li.Val != 0 && ri.Val != 0 { return ast.IntLit{Loc: loc, Val: 1} }
			return ast.IntLit{Loc: loc, Val: 0}
		}
		// ChainOr
		if li.Val != 0 || ri.Val != 0 { return ast.IntLit{Loc: loc, Val: 1} }
		return ast.IntLit{Loc: loc, Val: 0}
	}

	// Short-circuit: true && x → x, false && x → 0
	if op == ast.ChainAnd {
		if lok && li.Val != 0 { return rhs }
		if lok && li.Val == 0 { return ast.IntLit{Loc: loc, Val: 0} }
		if rok && ri.Val != 0 { return lhs }
		if rok && ri.Val == 0 { return ast.IntLit{Loc: loc, Val: 0} }
	}
	// Short-circuit: false || x → x, true || x → 1
	if op == ast.ChainOr {
		if lok && li.Val == 0 { return rhs }
		if lok && li.Val != 0 { return ast.IntLit{Loc: loc, Val: 1} }
		if rok && ri.Val == 0 { return lhs }
		if rok && ri.Val != 0 { return ast.IntLit{Loc: loc, Val: 1} }
	}

	// x && x → x, x || x → x
	if EqualExpr(lhs, rhs) { return lhs }

	// Parenthesize && inside || for correct bytecode precedence
	a := lhs
	if ch, ok := lhs.(ast.ChainExpr); ok && ch.Op == ast.ChainAnd && op == ast.ChainOr {
		a = ast.ParenExpr{Loc: loc, Expr: lhs}
	}
	b := rhs
	if _, ok := rhs.(ast.ChainExpr); ok && op == ast.ChainOr {
		b = ast.ParenExpr{Loc: loc, Expr: rhs}
	}
	return ast.ChainExpr{Loc: loc, LHS: a, Op: op, RHS: b}
}

// --- Unary folding ---

func (n *Normalizer) foldUnaryExpr(loc ast.Loc, op ast.UnaryOp, val ast.Expr) ast.Expr {
	// Constant folding
	if lit, ok := val.(ast.IntLit); ok {
		return ast.IntLit{Loc: loc, Val: ApplyUnary(lit.Val, op)}
	}

	// Double negation: --x → x, !!x → x, ~~x → x
	if un, ok := val.(ast.UnaryExpr); ok && un.Op == op {
		return un.Val
	}

	switch op {
	case ast.UnarySub:
		// Parenthesize compound expressions under unary minus
		if _, ok := val.(ast.BinOp); ok {
			val = ast.ParenExpr{Loc: loc, Expr: val}
		}
		return ast.UnaryExpr{Loc: loc, Op: ast.UnarySub, Val: val}

	case ast.UnaryInv:
		// ~x → x ^ -1
		b := val
		if bo, ok := val.(ast.BinOp); ok && Prec(bo.Op) < Prec(ast.OpXor) {
			b = ast.ParenExpr{Loc: loc, Expr: val}
		}
		return ast.BinOp{Loc: loc, LHS: b, Op: ast.OpXor, RHS: ast.IntLit{Loc: loc, Val: -1}}

	case ast.UnaryNot:
		// !x → (x == 0)
		return ast.CmpExpr{Loc: loc, LHS: val, Op: ast.CmpEqu, RHS: ast.IntLit{Loc: loc, Val: 0}}
	}
	return ast.UnaryExpr{Loc: loc, Op: op, Val: val}
}

// ============================================================
// Condition normalization
// ============================================================

// ConditionalUnit ensures an expression is a proper boolean condition.
// Wraps non-boolean expressions as (expr != 0).
func ConditionalUnit(e ast.Expr) ast.Expr {
	switch x := e.(type) {
	case ast.CmpExpr, ast.ChainExpr:
		return e // already boolean
	case ast.UnaryExpr:
		if x.Op == ast.UnaryNot {
			return UnaryToLogOp(x.Loc, ConditionalUnit(x.Val))
		}
	case ast.ParenExpr:
		return ast.ParenExpr{Loc: x.Loc, Expr: ConditionalUnit(x.Expr)}
	}
	// Default: expr != 0
	loc := e.ExprLoc()
	return ast.CmpExpr{Loc: loc, LHS: e, Op: ast.CmpNeq, RHS: ast.IntLit{Loc: loc, Val: 0}}
}

// UnaryToLogOp converts a negated condition to its logical inverse.
// Used when processing !condition in boolean context.
func UnaryToLogOp(loc ast.Loc, e ast.Expr) ast.Expr {
	switch x := e.(type) {
	case ast.CmpExpr:
		// !(a == b) → a != b
		return ast.CmpExpr{Loc: x.Loc, LHS: x.LHS, Op: ReverseCmp(x.Op), RHS: x.RHS}
	case ast.UnaryExpr:
		if x.Op == ast.UnaryNot {
			return ConditionalUnit(x.Val)
		}
	}
	// !(expr) → (expr == 0)
	return ast.CmpExpr{Loc: loc, LHS: e, Op: ast.CmpEqu, RHS: ast.IntLit{Loc: loc, Val: 0}}
}

// ============================================================
// Expression equality (for simplification)
// ============================================================

// EqualExpr tests structural equality of two expressions (ignoring locations).
func EqualExpr(a, b ast.Expr) bool {
	switch x := a.(type) {
	case ast.IntLit:
		if y, ok := b.(ast.IntLit); ok { return x.Val == y.Val }
	case ast.StoreRef:
		_, ok := b.(ast.StoreRef); return ok
	case ast.IntVar:
		if y, ok := b.(ast.IntVar); ok { return x.Bank == y.Bank && EqualExpr(x.Index, y.Index) }
	case ast.StrVar:
		if y, ok := b.(ast.StrVar); ok { return x.Bank == y.Bank && EqualExpr(x.Index, y.Index) }
	case ast.BinOp:
		if y, ok := b.(ast.BinOp); ok { return x.Op == y.Op && EqualExpr(x.LHS, y.LHS) && EqualExpr(x.RHS, y.RHS) }
	case ast.CmpExpr:
		if y, ok := b.(ast.CmpExpr); ok { return x.Op == y.Op && EqualExpr(x.LHS, y.LHS) && EqualExpr(x.RHS, y.RHS) }
	case ast.ChainExpr:
		if y, ok := b.(ast.ChainExpr); ok { return x.Op == y.Op && EqualExpr(x.LHS, y.LHS) && EqualExpr(x.RHS, y.RHS) }
	case ast.UnaryExpr:
		if y, ok := b.(ast.UnaryExpr); ok { return x.Op == y.Op && EqualExpr(x.Val, y.Val) }
	case ast.ParenExpr:
		if y, ok := b.(ast.ParenExpr); ok { return EqualExpr(x.Expr, y.Expr) }
		return EqualExpr(x.Expr, b) // parens are transparent
	case ast.VarOrFunc:
		if y, ok := b.(ast.VarOrFunc); ok { return x.Ident == y.Ident }
	}
	// Check if b is ParenExpr wrapping something equal to a
	if y, ok := b.(ast.ParenExpr); ok {
		return EqualExpr(a, y.Expr)
	}
	return false
}

// ============================================================
// High-level normalization entry points
// ============================================================

// NormalizeExpr normalizes a single expression for codegen.
// Resolves symbols, folds constants, simplifies operations.
func (n *Normalizer) NormalizeExpr(e ast.Expr) ast.Expr {
	return n.ConstFold(e)
}

// NormalizeCond normalizes a condition expression.
// Ensures the result is a proper boolean (LogOp or ChainExpr).
func (n *Normalizer) NormalizeCond(e ast.Expr) ast.Expr {
	e = n.ConstFold(e)
	return ConditionalUnit(e)
}

// NormalizeAndGetConst evaluates an expression at compile time.
// Returns (intVal, true) for integer constants, (0, false) otherwise.
func (n *Normalizer) NormalizeAndGetConst(e ast.Expr) (int32, bool) {
	folded := n.ConstFold(e)
	if lit, ok := folded.(ast.IntLit); ok {
		return lit.Val, true
	}
	return 0, false
}

// NormalizeAndGetInt evaluates an expression as an integer constant.
// Returns an error if not a constant integer.
func (n *Normalizer) NormalizeAndGetInt(e ast.Expr) (int32, error) {
	v, ok := n.NormalizeAndGetConst(e)
	if !ok {
		return 0, fmt.Errorf("expected constant integer expression")
	}
	return v, nil
}

// NormalizeAndGetStr evaluates an expression as a string constant.
func (n *Normalizer) NormalizeAndGetStr(e ast.Expr) (string, error) {
	folded := n.ConstFold(e)
	if slit, ok := folded.(ast.StrLit); ok {
		if len(slit.Tokens) > 0 {
			if tt, ok := slit.Tokens[0].(ast.TextToken); ok {
				return tt.Text, nil
			}
		}
		return "", nil
	}
	return "", fmt.Errorf("expected constant string expression")
}

// EvalAsBool evaluates an expression as a boolean for #if directives.
func (n *Normalizer) EvalAsBool(e ast.Expr) bool {
	v, ok := n.NormalizeAndGetConst(e)
	if !ok {
		return false
	}
	return v != 0
}

// NormalizeAssignment normalizes the RHS of an assignment.
// Returns the simplified expression.
func (n *Normalizer) NormalizeAssignment(dest ast.Expr, op ast.AssignOp, e ast.Expr) ast.Expr {
	e = n.ConstFold(e)
	// Strip unnecessary outer parens
	if pe, ok := e.(ast.ParenExpr); ok {
		e = pe.Expr
	}
	return e
}

// NormalizeParams normalizes all parameters in a function call.
func (n *Normalizer) NormalizeParams(params []ast.Param) []ast.Param {
	result := make([]ast.Param, len(params))
	for i, p := range params {
		result[i] = n.foldParam(p)
	}
	return result
}
