package memory

import (
	"testing"

	"github.com/yoremi/rldev-go/rlc/pkg/ast"
)

func TestNewMemory(t *testing.T) {
	m := New()
	if m.ScopeDepth() != 1 {
		t.Errorf("initial depth: got %d, want 1", m.ScopeDepth())
	}
}

func TestDefineAndGet(t *testing.T) {
	m := New()
	m.Define("X", Symbol{Kind: KindInteger, IntVal: 42})
	sym, ok := m.Get("X")
	if !ok { t.Fatal("X not found") }
	if sym.Kind != KindInteger || sym.IntVal != 42 {
		t.Errorf("X: got kind=%d val=%d", sym.Kind, sym.IntVal)
	}
}

func TestDefined(t *testing.T) {
	m := New()
	if m.Defined("X") { t.Error("X should not be defined") }
	m.Define("X", Symbol{Kind: KindInteger, IntVal: 1})
	if !m.Defined("X") { t.Error("X should be defined") }
}

func TestDefineGlobal(t *testing.T) {
	m := New()
	m.DefineGlobal("G", Symbol{Kind: KindInteger, IntVal: 99})
	sym, ok := m.Get("G")
	if !ok { t.Fatal("G not found") }
	if sym.IntVal != 99 { t.Errorf("G: got %d", sym.IntVal) }
}

func TestScoping(t *testing.T) {
	m := New()
	m.Define("X", Symbol{Kind: KindInteger, IntVal: 1})

	m.OpenScope()
	m.Define("X", Symbol{Kind: KindInteger, IntVal: 2})
	sym, _ := m.Get("X")
	if sym.IntVal != 2 { t.Errorf("inner X: got %d, want 2", sym.IntVal) }

	m.CloseScope()
	sym, _ = m.Get("X")
	if sym.IntVal != 1 { t.Errorf("outer X: got %d, want 1", sym.IntVal) }
}

func TestScopingDeep(t *testing.T) {
	m := New()
	m.Define("A", Symbol{Kind: KindInteger, IntVal: 10})

	m.OpenScope()
	m.Define("B", Symbol{Kind: KindInteger, IntVal: 20})
	if m.ScopeDepth() != 2 { t.Errorf("depth: got %d, want 2", m.ScopeDepth()) }

	m.OpenScope()
	m.Define("C", Symbol{Kind: KindInteger, IntVal: 30})
	if m.ScopeDepth() != 3 { t.Errorf("depth: got %d, want 3", m.ScopeDepth()) }

	m.CloseScope()
	if m.Defined("C") { t.Error("C should be gone after close") }
	if !m.Defined("B") { t.Error("B should still exist") }
	if !m.Defined("A") { t.Error("A should still exist") }

	m.CloseScope()
	if m.Defined("B") { t.Error("B should be gone") }
	if !m.Defined("A") { t.Error("A should still exist") }
}

func TestUndefine(t *testing.T) {
	m := New()
	m.Define("X", Symbol{Kind: KindInteger, IntVal: 1})
	err := m.Undefine("X")
	if err != nil { t.Fatal(err) }
	if m.Defined("X") { t.Error("X should be undefined") }

	err = m.Undefine("NONEXIST")
	if err == nil { t.Error("expected error for undefined symbol") }
}

func TestMutate(t *testing.T) {
	m := New()
	m.Define("X", Symbol{Kind: KindInteger, IntVal: 1})
	err := m.Mutate("X", Symbol{Kind: KindInteger, IntVal: 99})
	if err != nil { t.Fatal(err) }
	sym, _ := m.Get("X")
	if sym.IntVal != 99 { t.Errorf("mutated X: got %d, want 99", sym.IntVal) }
}

func TestDescribe(t *testing.T) {
	m := New()
	if m.Describe("X") != "undeclared identifier" { t.Error("undefined") }

	m.Define("M", Symbol{Kind: KindMacro})
	if m.Describe("M") != "macro" { t.Error("macro") }

	m.Define("I", Symbol{Kind: KindInteger})
	if m.Describe("I") != "integer constant" { t.Error("integer") }

	m.Define("S", Symbol{Kind: KindString})
	if m.Describe("S") != "string constant" { t.Error("string") }

	m.Define("V", Symbol{Kind: KindStaticVar, Var: &StaticVar{IsStr: false, ArrayLen: 0}})
	if m.Describe("V") != "integer variable" { t.Error("int var") }

	m.Define("SV", Symbol{Kind: KindStaticVar, Var: &StaticVar{IsStr: true, ArrayLen: 0}})
	if m.Describe("SV") != "string variable" { t.Error("str var") }

	m.Define("A", Symbol{Kind: KindStaticVar, Var: &StaticVar{IsStr: false, ArrayLen: 10}})
	if m.Describe("A") != "integer array" { t.Error("int array") }

	m.Define("SA", Symbol{Kind: KindStaticVar, Var: &StaticVar{IsStr: true, ArrayLen: 5}})
	if m.Describe("SA") != "string array" { t.Error("str array") }
}

func TestGetAsExpr(t *testing.T) {
	m := New()

	// Integer constant
	m.Define("N", Symbol{Kind: KindInteger, IntVal: 42})
	expr, err := m.GetAsExpr("N", ast.Nowhere)
	if err != nil { t.Fatal(err) }
	lit, ok := expr.(ast.IntLit)
	if !ok { t.Fatalf("N: got %T", expr) }
	if lit.Val != 42 { t.Errorf("N: got %d", lit.Val) }

	// String constant
	m.Define("S", Symbol{Kind: KindString, StrVal: "hello"})
	expr, err = m.GetAsExpr("S", ast.Nowhere)
	if err != nil { t.Fatal(err) }
	if _, ok := expr.(ast.StrLit); !ok { t.Fatalf("S: got %T", expr) }

	// Static int var
	m.Define("V", Symbol{Kind: KindStaticVar, Var: &StaticVar{TypedSpace: 0x0b, Index: 5}})
	expr, err = m.GetAsExpr("V", ast.Nowhere)
	if err != nil { t.Fatal(err) }
	iv, ok := expr.(ast.IntVar)
	if !ok { t.Fatalf("V: got %T", expr) }
	if iv.Bank != 0x0b { t.Errorf("V bank: got 0x%02x", iv.Bank) }

	// Macro
	m.Define("MAC", Symbol{Kind: KindMacro, Expr: ast.IntLit{Val: 99}})
	expr, err = m.GetAsExpr("MAC", ast.Nowhere)
	if err != nil { t.Fatal(err) }
	if lit, ok := expr.(ast.IntLit); !ok || lit.Val != 99 {
		t.Errorf("MAC: got %v", expr)
	}

	// Undefined
	_, err = m.GetAsExpr("NOPE", ast.Nowhere)
	if err == nil { t.Error("expected error for undefined") }
}

func TestGetDerefAsExpr(t *testing.T) {
	m := New()
	m.Define("arr", Symbol{Kind: KindStaticVar, Var: &StaticVar{
		TypedSpace: 0x0b, Index: 10, ArrayLen: 5,
	}})
	expr, err := m.GetDerefAsExpr("arr", ast.IntLit{Val: 2}, ast.Nowhere)
	if err != nil { t.Fatal(err) }
	iv, ok := expr.(ast.IntVar)
	if !ok { t.Fatalf("deref: got %T", expr) }
	// Index should be 10 + 2
	bo, ok := iv.Index.(ast.BinOp)
	if !ok { t.Fatalf("deref index: got %T", iv.Index) }
	if bo.Op != ast.OpAdd { t.Errorf("deref op: got %v", bo.Op) }
}

func TestVarIdx(t *testing.T) {
	tests := []struct{ bank, want int }{
		{0, 0}, {1, 1}, {6, 6}, {25, 7}, {18, 8}, {12, 9}, {10, 10}, {11, 11},
	}
	for _, tt := range tests {
		got, err := varIdx(tt.bank)
		if err != nil { t.Errorf("bank %d: %v", tt.bank, err); continue }
		if got != tt.want { t.Errorf("varIdx(%d) = %d, want %d", tt.bank, got, tt.want) }
	}
	// Invalid bank
	_, err := varIdx(99)
	if err == nil { t.Error("bank 99 should error") }
}

func TestAllocatorFindUnused(t *testing.T) {
	a := newAllocator()
	// Empty → should find 0
	idx, err := a.FindUnusedIndex(0, 0, 100)
	if err != nil { t.Fatal(err) }
	if idx != 0 { t.Errorf("got %d, want 0", idx) }

	// Mark 0 as used → should find 1
	a.staticVars[0][0] = 1
	idx, err = a.FindUnusedIndex(0, 0, 100)
	if err != nil { t.Fatal(err) }
	if idx != 1 { t.Errorf("got %d, want 1", idx) }
}

func TestAllocatorFindBlock(t *testing.T) {
	a := newAllocator()
	// Mark slots 0-4 as used
	for i := 0; i < 5; i++ {
		a.staticVars[0][i] = 1
	}
	// Block of 3 should start at 5
	idx, err := a.FindUnusedBlock(0, 0, 3)
	if err != nil { t.Fatal(err) }
	if idx != 5 { t.Errorf("got %d, want 5", idx) }
}

func TestAllocVar(t *testing.T) {
	m := New()
	sv, err := m.AllocVar("myvar", VarType{BitWidth: 32}, 0, nil)
	if err != nil { t.Fatal(err) }
	if sv == nil { t.Fatal("nil StaticVar") }
	if sv.IsStr { t.Error("should not be string") }
	if sv.AllocLen != 1 { t.Errorf("alloc len: got %d", sv.AllocLen) }

	// Should be defined
	if !m.Defined("myvar") { t.Error("myvar should be defined") }
	expr, err := m.GetAsExpr("myvar", ast.Nowhere)
	if err != nil { t.Fatal(err) }
	if _, ok := expr.(ast.IntVar); !ok { t.Errorf("myvar expr: got %T", expr) }
}

func TestAllocVarStr(t *testing.T) {
	m := New()
	sv, err := m.AllocVar("msg", VarType{IsStr: true}, 0, nil)
	if err != nil { t.Fatal(err) }
	if !sv.IsStr { t.Error("should be string") }
	expr, _ := m.GetAsExpr("msg", ast.Nowhere)
	if _, ok := expr.(ast.StrVar); !ok { t.Errorf("msg: got %T", expr) }
}

func TestAllocVarArray(t *testing.T) {
	m := New()
	sv, err := m.AllocVar("arr", VarType{BitWidth: 32}, 10, nil)
	if err != nil { t.Fatal(err) }
	if sv.ArrayLen != 10 { t.Errorf("array len: got %d", sv.ArrayLen) }
	if sv.AllocLen < 10 { t.Errorf("alloc len: got %d, want >= 10", sv.AllocLen) }
}

func TestAllocVarSubInt(t *testing.T) {
	m := New()
	// bit (1-bit) — 32 bits pack into 1 int32 slot
	sv, err := m.AllocVar("flags", VarType{BitWidth: 1}, 32, nil)
	if err != nil { t.Fatal(err) }
	if sv.AllocLen != 1 { t.Errorf("bit alloc: got %d slots, want 1", sv.AllocLen) }
	// Typed space should be offset by 26
	if sv.TypedSpace != m.IntAllocSpace+26 {
		t.Errorf("bit typed space: got %d, want %d", sv.TypedSpace, m.IntAllocSpace+26)
	}
}

func TestAllocTempInt(t *testing.T) {
	m := New()
	expr, err := m.AllocTempInt()
	if err != nil { t.Fatal(err) }
	iv, ok := expr.(ast.IntVar)
	if !ok { t.Fatalf("got %T", expr) }
	if iv.Bank != m.IntAllocSpace { t.Errorf("bank: got %d", iv.Bank) }
}

func TestAllocTempStr(t *testing.T) {
	m := New()
	expr, err := m.AllocTempStr()
	if err != nil { t.Fatal(err) }
	sv, ok := expr.(ast.StrVar)
	if !ok { t.Fatalf("got %T", expr) }
	if sv.Bank != m.StrAllocSpace { t.Errorf("bank: got %d", sv.Bank) }
}

func TestGetOrAllocTemp(t *testing.T) {
	m := New()
	expr1, err := m.GetOrAllocTemp("__dto_token", false)
	if err != nil { t.Fatal(err) }
	// Should be cached on second call
	expr2, err := m.GetOrAllocTemp("__dto_token", false)
	if err != nil { t.Fatal(err) }
	iv1 := expr1.(ast.IntVar)
	iv2 := expr2.(ast.IntVar)
	if iv1.Bank != iv2.Bank { t.Error("cached temp should return same bank") }
}

func TestScopeFreesStaticVars(t *testing.T) {
	m := New()
	m.OpenScope()
	sv, err := m.AllocVar("local", VarType{BitWidth: 32}, 0, nil)
	if err != nil { t.Fatal(err) }
	allocIdx := sv.AllocIndex
	allocSpace := sv.AllocSpace
	// Slot should be in use
	if m.alloc.staticVars[allocSpace][allocIdx] <= 0 {
		t.Error("slot should be allocated")
	}
	m.CloseScope()
	// Slot should be freed
	if m.alloc.staticVars[allocSpace][allocIdx] > 0 {
		t.Error("slot should be freed after scope close")
	}
	if m.Defined("local") { t.Error("local should be undefined after scope close") }
}

func TestGetRealAddress(t *testing.T) {
	// 32-bit int → same space
	ts, ai := getRealAddress(VarType{BitWidth: 32}, 5, 10)
	if ts != 5 || ai != 10 { t.Errorf("int32: got %d,%d", ts, ai) }

	// 1-bit → space+26, index*32
	ts, ai = getRealAddress(VarType{BitWidth: 1}, 5, 10)
	if ts != 31 || ai != 320 { t.Errorf("bit: got %d,%d", ts, ai) }

	// 8-bit (byte) → space+104, index*4
	ts, ai = getRealAddress(VarType{BitWidth: 8}, 5, 10)
	if ts != 109 || ai != 40 { t.Errorf("byte: got %d,%d", ts, ai) }

	// string → same
	ts, ai = getRealAddress(VarType{IsStr: true}, 12, 5)
	if ts != 12 || ai != 5 { t.Errorf("str: got %d,%d", ts, ai) }
}
