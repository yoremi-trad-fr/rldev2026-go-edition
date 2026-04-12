package intrinsic

import (
	"testing"

	"github.com/yoremi/rldev-go/rlc/pkg/ast"
	"github.com/yoremi/rldev-go/rlc/pkg/kfn"
	"github.com/yoremi/rldev-go/rlc/pkg/memory"
)

func newReg() *Registry {
	return New(memory.New())
}

func sp(e ast.Expr) ast.Param { return ast.SimpleParam{Expr: e} }
func vof(name string) ast.Expr { return ast.VarOrFunc{Ident: name} }
func ilit(v int32) ast.Expr { return ast.IntLit{Val: v} }
func slit(s string) ast.Expr { return ast.StrLit{Tokens: []ast.StrToken{ast.TextToken{Text: s}}} }

func expectInt(t *testing.T, name string, e ast.Expr, err error, want int32) {
	t.Helper()
	if err != nil { t.Fatalf("%s: %v", name, err) }
	lit, ok := e.(ast.IntLit)
	if !ok { t.Fatalf("%s: got %T, want IntLit", name, e) }
	if lit.Val != want { t.Errorf("%s: got %d, want %d", name, lit.Val, want) }
}

// --- Registry ---

func TestIsBuiltin(t *testing.T) {
	r := newReg()
	builtins := []string{"defined?", "default", "constant?", "integer?", "array?",
		"length", "__deref", "__sderef", "__variable?", "__addr", "__ident",
		"__empty_string?", "__equal_strings?", "kinetic?",
		"target_lt", "target_le", "target_gt", "target_ge"}
	for _, name := range builtins {
		if !r.IsBuiltin(name) {
			t.Errorf("%s should be builtin", name)
		}
	}
	if r.IsBuiltin("nonexistent") {
		t.Error("nonexistent should not be builtin")
	}
}

func TestNames(t *testing.T) {
	r := newReg()
	names := r.Names()
	if len(names) < 15 {
		t.Errorf("expected 15+ builtins, got %d", len(names))
	}
}

// --- defined? ---

func TestDefinedTrue(t *testing.T) {
	r := newReg()
	r.mem.Define("X", memory.Symbol{Kind: memory.KindInteger, IntVal: 1})
	e, err := r.EvalAsExpr("defined?", ast.Nowhere, []ast.Param{sp(vof("X"))})
	expectInt(t, "defined?(X)", e, err, 1)
}

func TestDefinedFalse(t *testing.T) {
	r := newReg()
	e, err := r.EvalAsExpr("defined?", ast.Nowhere, []ast.Param{sp(vof("NOPE"))})
	expectInt(t, "defined?(NOPE)", e, err, 0)
}

func TestDefinedMultiple(t *testing.T) {
	r := newReg()
	r.mem.Define("A", memory.Symbol{Kind: memory.KindInteger, IntVal: 1})
	r.mem.Define("B", memory.Symbol{Kind: memory.KindInteger, IntVal: 2})
	e, err := r.EvalAsExpr("defined?", ast.Nowhere, []ast.Param{sp(vof("A")), sp(vof("B"))})
	expectInt(t, "defined?(A,B)", e, err, 1)

	e, err = r.EvalAsExpr("defined?", ast.Nowhere, []ast.Param{sp(vof("A")), sp(vof("NOPE"))})
	expectInt(t, "defined?(A,NOPE)", e, err, 0)
}

// --- default ---

func TestDefaultDefined(t *testing.T) {
	r := newReg()
	r.mem.Define("X", memory.Symbol{Kind: memory.KindInteger, IntVal: 42})
	e, err := r.EvalAsExpr("default", ast.Nowhere, []ast.Param{sp(vof("X")), sp(ilit(99))})
	if err != nil { t.Fatal(err) }
	// Should return the VarOrFunc "X" (not the fallback 99)
	if _, ok := e.(ast.VarOrFunc); !ok {
		t.Errorf("default(X,99) when X defined: got %T", e)
	}
}

func TestDefaultUndefined(t *testing.T) {
	r := newReg()
	e, err := r.EvalAsExpr("default", ast.Nowhere, []ast.Param{sp(vof("NOPE")), sp(ilit(99))})
	if err != nil { t.Fatal(err) }
	expectInt(t, "default(NOPE,99)", e, nil, 99)
}

// --- constant? ---

func TestConstantTrue(t *testing.T) {
	r := newReg()
	e, err := r.EvalAsExpr("constant?", ast.Nowhere, []ast.Param{sp(ilit(42))})
	expectInt(t, "constant?(42)", e, err, 1)
}

func TestConstantFalse(t *testing.T) {
	r := newReg()
	e, err := r.EvalAsExpr("constant?", ast.Nowhere, []ast.Param{
		sp(ast.IntVar{Bank: 0, Index: ilit(0)}),
	})
	expectInt(t, "constant?(var)", e, err, 0)
}

// --- integer? ---

func TestIntegerTrue(t *testing.T) {
	r := newReg()
	e, err := r.EvalAsExpr("integer?", ast.Nowhere, []ast.Param{sp(ilit(1))})
	expectInt(t, "integer?(1)", e, err, 1)
}

func TestIntegerFalseStr(t *testing.T) {
	r := newReg()
	e, err := r.EvalAsExpr("integer?", ast.Nowhere, []ast.Param{sp(slit("hello"))})
	expectInt(t, "integer?('hello')", e, err, 0)
}

func TestIntegerFalseStrVar(t *testing.T) {
	r := newReg()
	e, err := r.EvalAsExpr("integer?", ast.Nowhere, []ast.Param{
		sp(ast.StrVar{Bank: 0x12, Index: ilit(0)}),
	})
	expectInt(t, "integer?(strS[0])", e, err, 0)
}

// --- array? ---

func TestArrayTrue(t *testing.T) {
	r := newReg()
	r.mem.Define("arr", memory.Symbol{Kind: memory.KindStaticVar, Var: &memory.StaticVar{ArrayLen: 10}})
	e, err := r.EvalAsExpr("array?", ast.Nowhere, []ast.Param{sp(vof("arr"))})
	expectInt(t, "array?(arr)", e, err, 1)
}

func TestArrayFalseScalar(t *testing.T) {
	r := newReg()
	r.mem.Define("x", memory.Symbol{Kind: memory.KindStaticVar, Var: &memory.StaticVar{ArrayLen: 0}})
	e, err := r.EvalAsExpr("array?", ast.Nowhere, []ast.Param{sp(vof("x"))})
	expectInt(t, "array?(x_scalar)", e, err, 0)
}

func TestArrayFalseUndefined(t *testing.T) {
	r := newReg()
	e, err := r.EvalAsExpr("array?", ast.Nowhere, []ast.Param{sp(vof("NOPE"))})
	expectInt(t, "array?(NOPE)", e, err, 0)
}

// --- length ---

func TestLength(t *testing.T) {
	r := newReg()
	r.mem.Define("arr", memory.Symbol{Kind: memory.KindStaticVar, Var: &memory.StaticVar{ArrayLen: 25}})
	e, err := r.EvalAsExpr("length", ast.Nowhere, []ast.Param{sp(vof("arr"))})
	expectInt(t, "length(arr)", e, err, 25)
}

func TestLengthNotArray(t *testing.T) {
	r := newReg()
	r.mem.Define("x", memory.Symbol{Kind: memory.KindInteger, IntVal: 5})
	_, err := r.EvalAsExpr("length", ast.Nowhere, []ast.Param{sp(vof("x"))})
	if err == nil { t.Error("expected error for non-array") }
}

// --- __deref ---

func TestDeref(t *testing.T) {
	r := newReg()
	e, err := r.EvalAsExpr("__deref", ast.Nowhere, []ast.Param{sp(ilit(0x0b)), sp(ilit(5))})
	if err != nil { t.Fatal(err) }
	iv, ok := e.(ast.IntVar)
	if !ok { t.Fatalf("got %T", e) }
	if iv.Bank != 0x0b { t.Errorf("bank: %d", iv.Bank) }
}

// --- __sderef ---

func TestSDeref(t *testing.T) {
	r := newReg()
	e, err := r.EvalAsExpr("__sderef", ast.Nowhere, []ast.Param{sp(ilit(0x12)), sp(ilit(3))})
	if err != nil { t.Fatal(err) }
	sv, ok := e.(ast.StrVar)
	if !ok { t.Fatalf("got %T", e) }
	if sv.Bank != 0x12 { t.Errorf("bank: %d", sv.Bank) }
}

// --- __variable? ---

func TestVariableIntVar(t *testing.T) {
	r := newReg()
	e, err := r.EvalAsExpr("__variable?", ast.Nowhere, []ast.Param{
		sp(ast.IntVar{Bank: 0, Index: ilit(0)}),
	})
	expectInt(t, "__variable?(intA[0])", e, err, 1)
}

func TestVariableConst(t *testing.T) {
	r := newReg()
	e, err := r.EvalAsExpr("__variable?", ast.Nowhere, []ast.Param{sp(ilit(42))})
	expectInt(t, "__variable?(42)", e, err, 0)
}

func TestVariableSymbol(t *testing.T) {
	r := newReg()
	r.mem.Define("v", memory.Symbol{Kind: memory.KindStaticVar, Var: &memory.StaticVar{TypedSpace: 0x0b, Index: 5}})
	e, err := r.EvalAsExpr("__variable?", ast.Nowhere, []ast.Param{sp(vof("v"))})
	expectInt(t, "__variable?(v)", e, err, 1)
}

// --- __addr ---

func TestAddr(t *testing.T) {
	r := newReg()
	e, err := r.EvalAsExpr("__addr", ast.Nowhere, []ast.Param{
		sp(ast.IntVar{Bank: 5, Index: ilit(10)}),
	})
	if err != nil { t.Fatal(err) }
	// Should be BinOp: 10 | (5 << 16)
	bo, ok := e.(ast.BinOp)
	if !ok { t.Fatalf("got %T", e) }
	if bo.Op != ast.OpOr { t.Errorf("op: %v", bo.Op) }
}

// --- __ident ---

func TestIdent(t *testing.T) {
	r := newReg()
	e, err := r.EvalAsExpr("__ident", ast.Nowhere, []ast.Param{sp(slit("myvar"))})
	if err != nil { t.Fatal(err) }
	vf, ok := e.(ast.VarOrFunc)
	if !ok { t.Fatalf("got %T", e) }
	if vf.Ident != "myvar" { t.Errorf("got %q", vf.Ident) }
}

// --- __empty_string? ---

func TestEmptyStringTrue(t *testing.T) {
	r := newReg()
	e, err := r.EvalAsExpr("__empty_string?", ast.Nowhere, []ast.Param{sp(slit(""))})
	expectInt(t, "empty_string?('')", e, err, 1)
}

func TestEmptyStringFalse(t *testing.T) {
	r := newReg()
	e, err := r.EvalAsExpr("__empty_string?", ast.Nowhere, []ast.Param{sp(slit("hi"))})
	expectInt(t, "empty_string?('hi')", e, err, 0)
}

// --- __equal_strings? ---

func TestEqualStringsTrue(t *testing.T) {
	r := newReg()
	e, err := r.EvalAsExpr("__equal_strings?", ast.Nowhere, []ast.Param{sp(slit("abc")), sp(slit("abc"))})
	expectInt(t, "equal_strings?('abc','abc')", e, err, 1)
}

func TestEqualStringsFalse(t *testing.T) {
	r := newReg()
	e, err := r.EvalAsExpr("__equal_strings?", ast.Nowhere, []ast.Param{sp(slit("abc")), sp(slit("xyz"))})
	expectInt(t, "equal_strings?('abc','xyz')", e, err, 0)
}

// --- kinetic? ---

func TestKineticFalse(t *testing.T) {
	r := newReg()
	e, err := r.EvalAsExpr("kinetic?", ast.Nowhere, nil)
	expectInt(t, "kinetic?(default)", e, err, 0)
}

func TestKineticTrue(t *testing.T) {
	r := newReg()
	r.SetTarget(kfn.TargetKinetic, kfn.Version{1, 0, 0, 0})
	e, err := r.EvalAsExpr("kinetic?", ast.Nowhere, nil)
	expectInt(t, "kinetic?(kinetic)", e, err, 1)
}

// --- target comparisons ---

func TestTargetLt(t *testing.T) {
	r := newReg()
	r.SetTarget(kfn.TargetRealLive, kfn.Version{1, 2, 7, 0})
	// 1.2.7 < 2.0 → true
	e, err := r.EvalAsExpr("target_lt", ast.Nowhere, []ast.Param{sp(ilit(2))})
	expectInt(t, "target_lt(2)", e, err, 1)
	// 1.2.7 < 1.0 → false
	e, err = r.EvalAsExpr("target_lt", ast.Nowhere, []ast.Param{sp(ilit(1))})
	expectInt(t, "target_lt(1)", e, err, 0)
}

func TestTargetGe(t *testing.T) {
	r := newReg()
	r.SetTarget(kfn.TargetRealLive, kfn.Version{1, 2, 7, 0})
	// 1.2.7 >= 1.2.7 → true
	e, err := r.EvalAsExpr("target_ge", ast.Nowhere, []ast.Param{sp(ilit(1)), sp(ilit(2)), sp(ilit(7))})
	expectInt(t, "target_ge(1,2,7)", e, err, 1)
	// 1.2.7 >= 1.3 → false
	e, err = r.EvalAsExpr("target_ge", ast.Nowhere, []ast.Param{sp(ilit(1)), sp(ilit(3))})
	expectInt(t, "target_ge(1,3)", e, err, 0)
}

func TestTargetCmpBadParams(t *testing.T) {
	r := newReg()
	_, err := r.EvalAsExpr("target_lt", ast.Nowhere, nil)
	if err == nil { t.Error("expected error with no params") }
}

// --- versionLess ---

func TestVersionLess(t *testing.T) {
	tests := []struct{ a, b kfn.Version; want bool }{
		{kfn.Version{1, 0, 0, 0}, kfn.Version{2, 0, 0, 0}, true},
		{kfn.Version{2, 0, 0, 0}, kfn.Version{1, 0, 0, 0}, false},
		{kfn.Version{1, 2, 3, 0}, kfn.Version{1, 2, 3, 0}, false},
		{kfn.Version{1, 2, 6, 0}, kfn.Version{1, 2, 7, 0}, true},
		{kfn.Version{1, 2, 7, 0}, kfn.Version{1, 2, 7, 1}, true},
	}
	for _, tt := range tests {
		if got := versionLess(tt.a, tt.b); got != tt.want {
			t.Errorf("versionLess(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

// --- EvalAsExpr unknown ---

func TestEvalUnknown(t *testing.T) {
	r := newReg()
	_, err := r.EvalAsExpr("nonexistent", ast.Nowhere, nil)
	if err == nil { t.Error("expected error for unknown intrinsic") }
}
