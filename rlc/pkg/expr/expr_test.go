package expr

import (
	"testing"

	"github.com/yoremi/rldev-go/rlc/pkg/ast"
	"github.com/yoremi/rldev-go/rlc/pkg/memory"
)

func newNorm() *Normalizer {
	return NewNormalizer(memory.New())
}

// --- Arithmetic evaluation ---

func TestApplyArith(t *testing.T) {
	tests := []struct{ a, b, want int32; op ast.ArithOp }{
		{3, 4, 7, ast.OpAdd}, {10, 3, 7, ast.OpSub}, {6, 7, 42, ast.OpMul},
		{15, 4, 3, ast.OpDiv}, {17, 5, 2, ast.OpMod},
		{0xff, 0x0f, 0x0f, ast.OpAnd}, {0xf0, 0x0f, 0xff, ast.OpOr},
		{0xff, 0x0f, 0xf0, ast.OpXor}, {1, 8, 256, ast.OpShl}, {256, 4, 16, ast.OpShr},
	}
	for _, tt := range tests {
		got := ApplyArith(tt.a, tt.b, tt.op)
		if got != tt.want {
			t.Errorf("%d %s %d = %d, want %d", tt.a, tt.op, tt.b, got, tt.want)
		}
	}
}

func TestApplyArithDivZero(t *testing.T) {
	if ApplyArith(10, 0, ast.OpDiv) != 0 { t.Error("div zero") }
	if ApplyArith(10, 0, ast.OpMod) != 0 { t.Error("mod zero") }
}

func TestApplyUnary(t *testing.T) {
	if ApplyUnary(5, ast.UnarySub) != -5 { t.Error("neg") }
	if ApplyUnary(0, ast.UnaryNot) != 1 { t.Error("not 0") }
	if ApplyUnary(42, ast.UnaryNot) != 0 { t.Error("not 42") }
	if ApplyUnary(0, ast.UnaryInv) != -1 { t.Error("inv 0") }
}

func TestApplyCmp(t *testing.T) {
	if ApplyCmp(5, 5, ast.CmpEqu) != 1 { t.Error("5==5") }
	if ApplyCmp(5, 5, ast.CmpNeq) != 0 { t.Error("5!=5") }
	if ApplyCmp(3, 7, ast.CmpLtn) != 1 { t.Error("3<7") }
	if ApplyCmp(7, 3, ast.CmpLtn) != 0 { t.Error("7<3") }
	if ApplyCmp(3, 3, ast.CmpLte) != 1 { t.Error("3<=3") }
	if ApplyCmp(7, 3, ast.CmpGtn) != 1 { t.Error("7>3") }
	if ApplyCmp(3, 3, ast.CmpGte) != 1 { t.Error("3>=3") }
}

func TestReverseCmp(t *testing.T) {
	pairs := [][2]ast.CmpOp{
		{ast.CmpEqu, ast.CmpNeq}, {ast.CmpLtn, ast.CmpGte},
		{ast.CmpLte, ast.CmpGtn}, {ast.CmpGtn, ast.CmpLte},
	}
	for _, p := range pairs {
		if ReverseCmp(p[0]) != p[1] { t.Errorf("reverse(%d)=%d, want %d", p[0], ReverseCmp(p[0]), p[1]) }
		if ReverseCmp(p[1]) != p[0] { t.Errorf("reverse(%d)=%d, want %d", p[1], ReverseCmp(p[1]), p[0]) }
	}
}

func TestPrec(t *testing.T) {
	if Prec(ast.OpAdd) != 10 { t.Error("add prec") }
	if Prec(ast.OpSub) != 10 { t.Error("sub prec") }
	if Prec(ast.OpMul) != 20 { t.Error("mul prec") }
	if Prec(ast.OpAnd) != 20 { t.Error("and prec") }
}

// --- Constant folding ---

func TestConstFoldLiteral(t *testing.T) {
	n := newNorm()
	e := n.ConstFold(ast.IntLit{Val: 42})
	if lit, ok := e.(ast.IntLit); !ok || lit.Val != 42 {
		t.Errorf("literal: got %v", e)
	}
}

func TestConstFoldBinOp(t *testing.T) {
	n := newNorm()
	e := n.ConstFold(ast.BinOp{LHS: ast.IntLit{Val: 3}, Op: ast.OpMul, RHS: ast.IntLit{Val: 7}})
	if lit, ok := e.(ast.IntLit); !ok || lit.Val != 21 {
		t.Errorf("3*7: got %v", e)
	}
}

func TestConstFoldNested(t *testing.T) {
	n := newNorm()
	// (2 + 3) * 4 = 20
	e := n.ConstFold(ast.BinOp{
		LHS: ast.BinOp{LHS: ast.IntLit{Val: 2}, Op: ast.OpAdd, RHS: ast.IntLit{Val: 3}},
		Op: ast.OpMul,
		RHS: ast.IntLit{Val: 4},
	})
	if lit, ok := e.(ast.IntLit); !ok || lit.Val != 20 {
		t.Errorf("(2+3)*4: got %v", e)
	}
}

func TestConstFoldUnary(t *testing.T) {
	n := newNorm()
	e := n.ConstFold(ast.UnaryExpr{Op: ast.UnarySub, Val: ast.IntLit{Val: 5}})
	if lit, ok := e.(ast.IntLit); !ok || lit.Val != -5 {
		t.Errorf("-5: got %v", e)
	}
}

func TestConstFoldCmp(t *testing.T) {
	n := newNorm()
	e := n.ConstFold(ast.CmpExpr{LHS: ast.IntLit{Val: 5}, Op: ast.CmpEqu, RHS: ast.IntLit{Val: 5}})
	if lit, ok := e.(ast.IntLit); !ok || lit.Val != 1 {
		t.Errorf("5==5: got %v", e)
	}
}

func TestConstFoldChain(t *testing.T) {
	n := newNorm()
	// true && false = 0
	e := n.ConstFold(ast.ChainExpr{LHS: ast.IntLit{Val: 1}, Op: ast.ChainAnd, RHS: ast.IntLit{Val: 0}})
	if lit, ok := e.(ast.IntLit); !ok || lit.Val != 0 {
		t.Errorf("1&&0: got %v", e)
	}
	// false || 42 = 1 (boolean true, not value 42 — Kepago || is boolean)
	e = n.ConstFold(ast.ChainExpr{LHS: ast.IntLit{Val: 0}, Op: ast.ChainOr, RHS: ast.IntLit{Val: 42}})
	if lit, ok := e.(ast.IntLit); !ok || lit.Val != 1 {
		t.Errorf("0||42: got %v", e)
	}
}

// --- Algebraic simplification ---

func TestSimplifyAddZero(t *testing.T) {
	n := newNorm()
	v := ast.IntVar{Bank: 0, Index: ast.IntLit{Val: 0}}
	e := n.ConstFold(ast.BinOp{LHS: v, Op: ast.OpAdd, RHS: ast.IntLit{Val: 0}})
	if _, ok := e.(ast.IntVar); !ok {
		t.Errorf("x+0: got %T, want IntVar", e)
	}
}

func TestSimplifyMulOne(t *testing.T) {
	n := newNorm()
	v := ast.IntVar{Bank: 0, Index: ast.IntLit{Val: 0}}
	e := n.ConstFold(ast.BinOp{LHS: v, Op: ast.OpMul, RHS: ast.IntLit{Val: 1}})
	if _, ok := e.(ast.IntVar); !ok {
		t.Errorf("x*1: got %T", e)
	}
}

func TestSimplifyMulZero(t *testing.T) {
	n := newNorm()
	v := ast.IntVar{Bank: 0, Index: ast.IntLit{Val: 0}}
	e := n.ConstFold(ast.BinOp{LHS: v, Op: ast.OpMul, RHS: ast.IntLit{Val: 0}})
	if lit, ok := e.(ast.IntLit); !ok || lit.Val != 0 {
		t.Errorf("x*0: got %v", e)
	}
}

func TestSimplifySubSelf(t *testing.T) {
	n := newNorm()
	v := ast.IntVar{Bank: 0, Index: ast.IntLit{Val: 5}}
	e := n.ConstFold(ast.BinOp{LHS: v, Op: ast.OpSub, RHS: v})
	if lit, ok := e.(ast.IntLit); !ok || lit.Val != 0 {
		t.Errorf("x-x: got %v", e)
	}
}

func TestSimplifyDivSelf(t *testing.T) {
	n := newNorm()
	v := ast.IntVar{Bank: 0, Index: ast.IntLit{Val: 5}}
	e := n.ConstFold(ast.BinOp{LHS: v, Op: ast.OpDiv, RHS: v})
	if lit, ok := e.(ast.IntLit); !ok || lit.Val != 1 {
		t.Errorf("x/x: got %v", e)
	}
}

func TestSimplifyAddNeg(t *testing.T) {
	n := newNorm()
	v := ast.IntVar{Bank: 0, Index: ast.IntLit{Val: 0}}
	e := n.ConstFold(ast.BinOp{LHS: v, Op: ast.OpAdd, RHS: ast.IntLit{Val: -5}})
	if bo, ok := e.(ast.BinOp); !ok || bo.Op != ast.OpSub {
		t.Errorf("x+(-5): got %T op=%v, want BinOp Sub", e, bo.Op)
	}
}

func TestSimplifyEqualSelf(t *testing.T) {
	n := newNorm()
	v := ast.IntVar{Bank: 0, Index: ast.IntLit{Val: 0}}
	e := n.ConstFold(ast.CmpExpr{LHS: v, Op: ast.CmpEqu, RHS: v})
	if lit, ok := e.(ast.IntLit); !ok || lit.Val != 1 {
		t.Errorf("x==x: got %v", e)
	}
}

func TestSimplifyAndSelf(t *testing.T) {
	n := newNorm()
	v := ast.IntVar{Bank: 0, Index: ast.IntLit{Val: 0}}
	e := n.ConstFold(ast.ChainExpr{LHS: v, Op: ast.ChainAnd, RHS: v})
	if _, ok := e.(ast.IntVar); !ok {
		t.Errorf("x&&x: got %T", e)
	}
}

// --- Double negation ---

func TestDoubleNeg(t *testing.T) {
	n := newNorm()
	v := ast.IntVar{Bank: 0, Index: ast.IntLit{Val: 0}}
	e := n.ConstFold(ast.UnaryExpr{Op: ast.UnarySub, Val: ast.UnaryExpr{Op: ast.UnarySub, Val: v}})
	if _, ok := e.(ast.IntVar); !ok {
		t.Errorf("--x: got %T", e)
	}
}

// --- Symbol resolution ---

func TestDisambiguateDefine(t *testing.T) {
	n := newNorm()
	n.Mem.Define("MAX", memory.Symbol{Kind: memory.KindInteger, IntVal: 100})
	e := n.ConstFold(ast.VarOrFunc{Ident: "MAX"})
	if lit, ok := e.(ast.IntLit); !ok || lit.Val != 100 {
		t.Errorf("MAX: got %v", e)
	}
}

func TestDisambiguateMacro(t *testing.T) {
	n := newNorm()
	n.Mem.Define("EXPR", memory.Symbol{
		Kind: memory.KindMacro,
		Expr: ast.BinOp{LHS: ast.IntLit{Val: 10}, Op: ast.OpAdd, RHS: ast.IntLit{Val: 20}},
	})
	e := n.ConstFold(ast.VarOrFunc{Ident: "EXPR"})
	if lit, ok := e.(ast.IntLit); !ok || lit.Val != 30 {
		t.Errorf("EXPR (10+20): got %v", e)
	}
}

func TestDisambiguateVar(t *testing.T) {
	n := newNorm()
	n.Mem.Define("myvar", memory.Symbol{
		Kind: memory.KindStaticVar,
		Var:  &memory.StaticVar{TypedSpace: 0x0b, Index: 5},
	})
	e := n.ConstFold(ast.VarOrFunc{Ident: "myvar"})
	if iv, ok := e.(ast.IntVar); !ok || iv.Bank != 0x0b {
		t.Errorf("myvar: got %T", e)
	}
}

func TestDisambiguateUndefined(t *testing.T) {
	n := newNorm()
	e := n.ConstFold(ast.VarOrFunc{Ident: "UNKNOWN"})
	// Should return unchanged
	if vf, ok := e.(ast.VarOrFunc); !ok || vf.Ident != "UNKNOWN" {
		t.Errorf("UNKNOWN: got %T", e)
	}
}

// --- Condition normalization ---

func TestConditionalUnitInt(t *testing.T) {
	e := ConditionalUnit(ast.IntLit{Val: 5})
	if cmp, ok := e.(ast.CmpExpr); !ok || cmp.Op != ast.CmpNeq {
		t.Errorf("cond(5): got %T", e)
	}
}

func TestConditionalUnitCmp(t *testing.T) {
	cmp := ast.CmpExpr{LHS: ast.IntLit{Val: 1}, Op: ast.CmpEqu, RHS: ast.IntLit{Val: 2}}
	e := ConditionalUnit(cmp)
	// Should pass through unchanged
	if _, ok := e.(ast.CmpExpr); !ok {
		t.Errorf("cond(cmp): got %T", e)
	}
}

func TestConditionalUnitNot(t *testing.T) {
	inner := ast.CmpExpr{LHS: ast.IntLit{Val: 1}, Op: ast.CmpEqu, RHS: ast.IntLit{Val: 2}}
	e := ConditionalUnit(ast.UnaryExpr{Op: ast.UnaryNot, Val: inner})
	// !(a==b) → a!=b
	if cmp, ok := e.(ast.CmpExpr); !ok || cmp.Op != ast.CmpNeq {
		t.Errorf("cond(!cmp): got %T", e)
	}
}

// --- Expression equality ---

func TestEqualExpr(t *testing.T) {
	if !EqualExpr(ast.IntLit{Val: 5}, ast.IntLit{Val: 5}) { t.Error("5==5") }
	if EqualExpr(ast.IntLit{Val: 5}, ast.IntLit{Val: 6}) { t.Error("5!=6") }
	if !EqualExpr(ast.StoreRef{}, ast.StoreRef{}) { t.Error("store==store") }
	v1 := ast.IntVar{Bank: 0, Index: ast.IntLit{Val: 3}}
	v2 := ast.IntVar{Bank: 0, Index: ast.IntLit{Val: 3}}
	v3 := ast.IntVar{Bank: 1, Index: ast.IntLit{Val: 3}}
	if !EqualExpr(v1, v2) { t.Error("intA[3]==intA[3]") }
	if EqualExpr(v1, v3) { t.Error("intA[3]!=intB[3]") }
	// Parens are transparent
	if !EqualExpr(ast.ParenExpr{Expr: ast.IntLit{Val: 5}}, ast.IntLit{Val: 5}) { t.Error("(5)==5") }
}

func TestEqualExprComplex(t *testing.T) {
	e1 := ast.BinOp{LHS: ast.IntLit{Val: 1}, Op: ast.OpAdd, RHS: ast.IntLit{Val: 2}}
	e2 := ast.BinOp{LHS: ast.IntLit{Val: 1}, Op: ast.OpAdd, RHS: ast.IntLit{Val: 2}}
	e3 := ast.BinOp{LHS: ast.IntLit{Val: 1}, Op: ast.OpSub, RHS: ast.IntLit{Val: 2}}
	if !EqualExpr(e1, e2) { t.Error("equal binops") }
	if EqualExpr(e1, e3) { t.Error("diff binops") }
}

// --- High-level entry points ---

func TestNormalizeAndGetConst(t *testing.T) {
	n := newNorm()
	n.Mem.Define("X", memory.Symbol{Kind: memory.KindInteger, IntVal: 42})
	v, ok := n.NormalizeAndGetConst(ast.VarOrFunc{Ident: "X"})
	if !ok || v != 42 { t.Errorf("got %d, %v", v, ok) }
}

func TestNormalizeAndGetInt(t *testing.T) {
	n := newNorm()
	v, err := n.NormalizeAndGetInt(ast.BinOp{LHS: ast.IntLit{Val: 10}, Op: ast.OpMul, RHS: ast.IntLit{Val: 3}})
	if err != nil { t.Fatal(err) }
	if v != 30 { t.Errorf("got %d", v) }
}

func TestNormalizeAndGetIntNonConst(t *testing.T) {
	n := newNorm()
	_, err := n.NormalizeAndGetInt(ast.IntVar{Bank: 0, Index: ast.IntLit{Val: 0}})
	if err == nil { t.Error("expected error for non-constant") }
}

func TestEvalAsBool(t *testing.T) {
	n := newNorm()
	if !n.EvalAsBool(ast.IntLit{Val: 1}) { t.Error("1 should be true") }
	if n.EvalAsBool(ast.IntLit{Val: 0}) { t.Error("0 should be false") }
	if !n.EvalAsBool(ast.BinOp{LHS: ast.IntLit{Val: 2}, Op: ast.OpAdd, RHS: ast.IntLit{Val: 3}}) {
		t.Error("2+3 should be true")
	}
}

func TestNormalizeParams(t *testing.T) {
	n := newNorm()
	n.Mem.Define("C", memory.Symbol{Kind: memory.KindInteger, IntVal: 10})
	params := []ast.Param{
		ast.SimpleParam{Expr: ast.VarOrFunc{Ident: "C"}},
		ast.SimpleParam{Expr: ast.BinOp{LHS: ast.IntLit{Val: 1}, Op: ast.OpAdd, RHS: ast.IntLit{Val: 2}}},
	}
	result := n.NormalizeParams(params)
	sp0 := result[0].(ast.SimpleParam)
	if lit, ok := sp0.Expr.(ast.IntLit); !ok || lit.Val != 10 {
		t.Errorf("param 0: got %v", sp0.Expr)
	}
	sp1 := result[1].(ast.SimpleParam)
	if lit, ok := sp1.Expr.(ast.IntLit); !ok || lit.Val != 3 {
		t.Errorf("param 1: got %v", sp1.Expr)
	}
}

// --- Paren stripping ---

func TestConstFoldStripParens(t *testing.T) {
	n := newNorm()
	e := n.ConstFold(ast.ParenExpr{Expr: ast.IntLit{Val: 7}})
	if _, ok := e.(ast.IntLit); !ok {
		t.Errorf("(7): got %T, want IntLit", e)
	}
}

func TestFoldDivZeroError(t *testing.T) {
	n := newNorm()
	n.ConstFold(ast.BinOp{LHS: ast.IntLit{Val: 10}, Op: ast.OpDiv, RHS: ast.IntLit{Val: 0}})
	if len(n.Errors) == 0 {
		t.Error("expected division by zero error")
	}
}

func TestInvToXor(t *testing.T) {
	n := newNorm()
	v := ast.IntVar{Bank: 0, Index: ast.IntLit{Val: 0}}
	e := n.ConstFold(ast.UnaryExpr{Op: ast.UnaryInv, Val: v})
	// ~x → x ^ -1
	if bo, ok := e.(ast.BinOp); !ok || bo.Op != ast.OpXor {
		t.Errorf("~x: got %T", e)
	}
}

func TestNotToEqZero(t *testing.T) {
	n := newNorm()
	v := ast.IntVar{Bank: 0, Index: ast.IntLit{Val: 0}}
	e := n.ConstFold(ast.UnaryExpr{Op: ast.UnaryNot, Val: v})
	// !x → x == 0
	if cmp, ok := e.(ast.CmpExpr); !ok || cmp.Op != ast.CmpEqu {
		t.Errorf("!x: got %T", e)
	}
}
