package meta

import (
	"testing"

	"github.com/yoremi/rldev-go/rlc/pkg/ast"
)

func TestNewState(t *testing.T) {
	s := NewState()
	if s.Val0x2C != 0 { t.Error("Val0x2C") }
	if len(s.DramatisPersonae) != 0 { t.Error("DramatisPersonae") }
	if len(s.Resources) != 0 { t.Error("Resources") }
	if s.GlossCount != 0 { t.Error("GlossCount") }
}

func TestUnique(t *testing.T) {
	s := NewState()
	a := s.Unique()
	b := s.Unique()
	c := s.Unique()
	if a == b || b == c || a == c {
		t.Errorf("unique values should be distinct: %d %d %d", a, b, c)
	}
	if b != a+1 || c != a+2 {
		t.Errorf("should be sequential: %d %d %d", a, b, c)
	}
}

func TestUniqueLabel(t *testing.T) {
	s := NewState()
	l1 := s.UniqueLabel(ast.Nowhere)
	l2 := s.UniqueLabel(ast.Nowhere)
	if l1.Ident == l2.Ident {
		t.Error("labels should be distinct")
	}
	if l1.Ident[:8] != "__auto@_" {
		t.Errorf("label format: %q", l1.Ident)
	}
}

func TestResources(t *testing.T) {
	s := NewState()
	s.SetResource("greeting", "Hello!", ast.Loc{File: "test", Line: 1})
	r, err := s.GetResource("greeting")
	if err != nil { t.Fatal(err) }
	if r.Text != "Hello!" { t.Errorf("text: %q", r.Text) }
	if r.Loc.Line != 1 { t.Error("loc") }
}

func TestResourceNotFound(t *testing.T) {
	s := NewState()
	_, err := s.GetResource("nonexist")
	if err == nil { t.Error("expected error") }
}

func TestBaseResources(t *testing.T) {
	s := NewState()
	s.SetBaseResource("key", "value", ast.Nowhere)
	r, err := s.GetBaseResource("key")
	if err != nil { t.Fatal(err) }
	if r.Text != "value" { t.Errorf("text: %q", r.Text) }
}

func TestAddCharacter(t *testing.T) {
	s := NewState()
	s.AddCharacter("Nagisa")
	s.AddCharacter("Tomoya")
	if len(s.DramatisPersonae) != 2 { t.Errorf("count: %d", len(s.DramatisPersonae)) }
	if s.DramatisPersonae[0] != "Nagisa" { t.Error("first") }
}

func TestInt32PaddedString(t *testing.T) {
	tests := []struct{ width int; val int32; want string }{
		{0, 42, "42"},
		{4, 42, "0042"},
		{2, 42, "42"},
		{6, 1, "000001"},
		{3, -5, "0-5"},
		{1, 0, "0"},
	}
	for _, tt := range tests {
		got := Int32PaddedString(tt.width, tt.val)
		if got != tt.want {
			t.Errorf("Int32PaddedString(%d, %d) = %q, want %q", tt.width, tt.val, got, tt.want)
		}
	}
}

// --- AST generation helpers ---

func TestIntExpr(t *testing.T) {
	e := IntExpr(42)
	lit, ok := e.(ast.IntLit)
	if !ok { t.Fatalf("got %T", e) }
	if lit.Val != 42 { t.Errorf("val: %d", lit.Val) }
}

func TestZeroExpr(t *testing.T) {
	e := ZeroExpr()
	lit := e.(ast.IntLit)
	if lit.Val != 0 { t.Errorf("val: %d", lit.Val) }
}

func TestMakeAssign(t *testing.T) {
	s := MakeAssign(ast.StoreRef{}, ast.AssignSet, ast.IntLit{Val: 5})
	as, ok := s.(ast.AssignStmt)
	if !ok { t.Fatalf("got %T", s) }
	if as.Op != ast.AssignSet { t.Error("op") }
}

func TestMakeCall(t *testing.T) {
	s := MakeCall("goto", nil, nil)
	fc, ok := s.(ast.FuncCallStmt)
	if !ok { t.Fatalf("got %T", s) }
	if fc.Ident != "goto" { t.Errorf("ident: %q", fc.Ident) }
}

func TestMakeCallWithArgs(t *testing.T) {
	s := MakeCall("strcpy", []ast.Expr{ast.IntLit{Val: 1}, ast.IntLit{Val: 2}}, nil)
	fc := s.(ast.FuncCallStmt)
	if len(fc.Params) != 2 { t.Errorf("params: %d", len(fc.Params)) }
}

func TestMakeGoto(t *testing.T) {
	lbl := ast.Label{Ident: "start"}
	s := MakeGoto(lbl)
	fc := s.(ast.FuncCallStmt)
	if fc.Ident != "goto" { t.Error("ident") }
	if fc.Label == nil || fc.Label.Ident != "start" { t.Error("label") }
}

func TestMakeGosub(t *testing.T) {
	lbl := ast.Label{Ident: "sub"}
	s := MakeGosub(lbl)
	fc := s.(ast.FuncCallStmt)
	if fc.Ident != "gosub" { t.Error("ident") }
}

func TestMakeGotoUnless(t *testing.T) {
	lbl := ast.Label{Ident: "skip"}
	s := MakeGotoUnless(ast.IntLit{Val: 0}, lbl)
	fc := s.(ast.FuncCallStmt)
	if fc.Ident != "goto_unless" { t.Error("ident") }
	if len(fc.Params) != 1 { t.Error("params") }
}

func TestMakeReturn(t *testing.T) {
	s := MakeReturn(ast.IntLit{Val: 42})
	rs, ok := s.(ast.ReturnStmt)
	if !ok { t.Fatalf("got %T", s) }
	if rs.Explicit { t.Error("should not be explicit") }
}

func TestMakeVarOrFunc(t *testing.T) {
	e := MakeVarOrFunc("myvar")
	vf, ok := e.(ast.VarOrFunc)
	if !ok { t.Fatalf("got %T", e) }
	if vf.Ident != "myvar" { t.Errorf("ident: %q", vf.Ident) }
}

func TestParseCallback(t *testing.T) {
	s := NewState()
	called := false
	s.CompileStatements = func(stmts []ast.Stmt) { called = true }
	s.ParseOne(ast.HaltStmt{})
	if !called { t.Error("callback not called") }
}

func TestParseNoCallback(t *testing.T) {
	s := NewState()
	// Should not panic even without callback
	s.ParseOne(ast.HaltStmt{})
}
