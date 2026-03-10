package parser

import (
	"testing"

	"github.com/yoremi/rldev-go/rlc/pkg/ast"
	"github.com/yoremi/rldev-go/rlc/pkg/lexer"
	"github.com/yoremi/rldev-go/rlc/pkg/token"
)

func parse(src string) *ast.SourceFile {
	l := lexer.New(src, "test")
	p := New(l)
	return p.ParseProgram()
}

func TestParseEmpty(t *testing.T) {
	sf := parse("")
	if len(sf.Stmts) != 0 {
		t.Errorf("empty: got %d stmts", len(sf.Stmts))
	}
}

func TestParseHalt(t *testing.T) {
	sf := parse("halt")
	if len(sf.Stmts) != 1 {
		t.Fatalf("halt: got %d stmts", len(sf.Stmts))
	}
	if _, ok := sf.Stmts[0].(ast.HaltStmt); !ok {
		t.Errorf("halt: got %T", sf.Stmts[0])
	}
}

func TestParseBreakContinue(t *testing.T) {
	sf := parse("break\ncontinue")
	if len(sf.Stmts) != 2 { t.Fatalf("got %d", len(sf.Stmts)) }
	if _, ok := sf.Stmts[0].(ast.BreakStmt); !ok { t.Error("expected BreakStmt") }
	if _, ok := sf.Stmts[1].(ast.ContinueStmt); !ok { t.Error("expected ContinueStmt") }
}

func TestParseLabel(t *testing.T) {
	sf := parse("@start")
	if len(sf.Stmts) != 1 { t.Fatalf("got %d stmts", len(sf.Stmts)) }
	ls, ok := sf.Stmts[0].(ast.LabelStmt)
	if !ok { t.Fatalf("got %T", sf.Stmts[0]) }
	if ls.Label.Ident != "start" { t.Errorf("label: got %q", ls.Label.Ident) }
}

func TestParseAssignment(t *testing.T) {
	sf := parse("intA[0] = 42")
	if len(sf.Stmts) != 1 { t.Fatalf("got %d stmts", len(sf.Stmts)) }
	as, ok := sf.Stmts[0].(ast.AssignStmt)
	if !ok { t.Fatalf("got %T", sf.Stmts[0]) }
	if as.Op != ast.AssignSet { t.Errorf("op: got %v", as.Op) }
	rhs, ok := as.Expr.(ast.IntLit)
	if !ok { t.Fatalf("rhs: got %T", as.Expr) }
	if rhs.Val != 42 { t.Errorf("rhs: got %d", rhs.Val) }
}

func TestParseCompoundAssign(t *testing.T) {
	tests := []struct{ src string; op ast.AssignOp }{
		{"intA[0] += 1", ast.AssignAdd},
		{"intA[0] -= 1", ast.AssignSub},
		{"intA[0] *= 2", ast.AssignMul},
		{"intA[0] <<= 1", ast.AssignShl},
	}
	for _, tt := range tests {
		sf := parse(tt.src)
		as, ok := sf.Stmts[0].(ast.AssignStmt)
		if !ok { t.Errorf("%s: got %T", tt.src, sf.Stmts[0]); continue }
		if as.Op != tt.op { t.Errorf("%s: op got %v, want %v", tt.src, as.Op, tt.op) }
	}
}

func TestParseExpression(t *testing.T) {
	l := lexer.New("3 + 4 * 2", "test")
	p := New(l)
	expr := p.ParseExpression()
	// Should be (3 + (4 * 2)) due to precedence
	bo, ok := expr.(ast.BinOp)
	if !ok { t.Fatalf("got %T", expr) }
	if bo.Op != ast.OpAdd { t.Errorf("top op: got %v", bo.Op) }
	rhs, ok := bo.RHS.(ast.BinOp)
	if !ok { t.Fatalf("rhs: got %T", bo.RHS) }
	if rhs.Op != ast.OpMul { t.Errorf("rhs op: got %v", rhs.Op) }
}

func TestParseComparison(t *testing.T) {
	l := lexer.New("x == 5", "test")
	p := New(l)
	expr := p.ParseExpression()
	cmp, ok := expr.(ast.CmpExpr)
	if !ok { t.Fatalf("got %T", expr) }
	if cmp.Op != ast.CmpEqu { t.Errorf("op: got %v", cmp.Op) }
}

func TestParseLogical(t *testing.T) {
	l := lexer.New("a && b || c", "test")
	p := New(l)
	expr := p.ParseExpression()
	// || has lower precedence than &&, so: (a && b) || c
	ch, ok := expr.(ast.ChainExpr)
	if !ok { t.Fatalf("got %T", expr) }
	if ch.Op != ast.ChainOr { t.Errorf("top op: got %v", ch.Op) }
	inner, ok := ch.LHS.(ast.ChainExpr)
	if !ok { t.Fatalf("lhs: got %T", ch.LHS) }
	if inner.Op != ast.ChainAnd { t.Errorf("inner op: got %v", inner.Op) }
}

func TestParseUnary(t *testing.T) {
	l := lexer.New("-5", "test")
	p := New(l)
	expr := p.ParseExpression()
	un, ok := expr.(ast.UnaryExpr)
	if !ok { t.Fatalf("got %T", expr) }
	if un.Op != ast.UnarySub { t.Errorf("op: got %v", un.Op) }
}

func TestParseFuncCall(t *testing.T) {
	sf := parse("foo(1, 2)")
	if len(sf.Stmts) != 1 { t.Fatalf("got %d stmts", len(sf.Stmts)) }
	fc, ok := sf.Stmts[0].(ast.FuncCallStmt)
	if !ok { t.Fatalf("got %T", sf.Stmts[0]) }
	if fc.Ident != "foo" { t.Errorf("ident: got %q", fc.Ident) }
	if len(fc.Params) != 2 { t.Errorf("params: got %d", len(fc.Params)) }
}

func TestParseFuncCallWithLabel(t *testing.T) {
	sf := parse("goto(1) @target")
	if len(sf.Stmts) < 1 { t.Fatalf("got %d stmts", len(sf.Stmts)) }
	// goto is parsed as expression then as return/funccall stmt
}

func TestParseIf(t *testing.T) {
	sf := parse("if 1 halt")
	if len(sf.Stmts) != 1 { t.Fatalf("got %d stmts", len(sf.Stmts)) }
	ifs, ok := sf.Stmts[0].(ast.IfStmt)
	if !ok { t.Fatalf("got %T", sf.Stmts[0]) }
	if ifs.Else != nil { t.Error("expected no else") }
}

func TestParseIfElse(t *testing.T) {
	sf := parse("if 1 halt else break")
	ifs, ok := sf.Stmts[0].(ast.IfStmt)
	if !ok { t.Fatalf("got %T", sf.Stmts[0]) }
	if ifs.Else == nil { t.Error("expected else branch") }
}

func TestParseWhile(t *testing.T) {
	sf := parse("while 1 halt")
	ws, ok := sf.Stmts[0].(ast.WhileStmt)
	if !ok { t.Fatalf("got %T", sf.Stmts[0]) }
	_ = ws
}

func TestParseRepeat(t *testing.T) {
	sf := parse("repeat halt till 0")
	rs, ok := sf.Stmts[0].(ast.RepeatStmt)
	if !ok { t.Fatalf("got %T", sf.Stmts[0]) }
	if len(rs.Body) != 1 { t.Errorf("body: got %d stmts", len(rs.Body)) }
}

func TestParseCase(t *testing.T) {
	sf := parse("case intA[0] of 1 halt of 2 break ecase")
	cs, ok := sf.Stmts[0].(ast.CaseStmt)
	if !ok { t.Fatalf("got %T", sf.Stmts[0]) }
	if len(cs.Arms) != 2 { t.Errorf("arms: got %d", len(cs.Arms)) }
}

func TestParseCaseOther(t *testing.T) {
	sf := parse("case intA[0] of 1 halt other break ecase")
	cs := sf.Stmts[0].(ast.CaseStmt)
	if len(cs.Default) != 1 { t.Errorf("default: got %d stmts", len(cs.Default)) }
}

func TestParseBlock(t *testing.T) {
	sf := parse(": halt ; break ;")
	// ":" starts a block until ";"
	bs, ok := sf.Stmts[0].(ast.BlockStmt)
	if !ok { t.Fatalf("got %T", sf.Stmts[0]) }
	if len(bs.Stmts) < 1 { t.Error("block should have stmts") }
}

func TestParseDefine(t *testing.T) {
	sf := parse("#define MAX = 100")
	ds, ok := sf.Stmts[0].(ast.DefineStmt)
	if !ok { t.Fatalf("got %T", sf.Stmts[0]) }
	if ds.Ident != "MAX" { t.Errorf("ident: got %q", ds.Ident) }
	lit, ok := ds.Value.(ast.IntLit)
	if !ok { t.Fatalf("value: got %T", ds.Value) }
	if lit.Val != 100 { t.Errorf("value: got %d", lit.Val) }
}

func TestParseDefineNoValue(t *testing.T) {
	sf := parse("#define FLAG")
	ds := sf.Stmts[0].(ast.DefineStmt)
	lit := ds.Value.(ast.IntLit)
	if lit.Val != 1 { t.Errorf("default value: got %d, want 1", lit.Val) }
}

func TestParseConst(t *testing.T) {
	sf := parse("#const PI = 3")
	cs, ok := sf.Stmts[0].(ast.DConstStmt)
	if !ok { t.Fatalf("got %T", sf.Stmts[0]) }
	if cs.Kind != ast.KindConst { t.Errorf("kind: got %v", cs.Kind) }
}

func TestParseDIf(t *testing.T) {
	sf := parse("#if 1 halt #endif")
	di, ok := sf.Stmts[0].(ast.DIfStmt)
	if !ok { t.Fatalf("got %T", sf.Stmts[0]) }
	if len(di.Body) != 1 { t.Errorf("body: got %d stmts", len(di.Body)) }
}

func TestParseDIfElse(t *testing.T) {
	sf := parse("#if 0 halt #else break #endif")
	di := sf.Stmts[0].(ast.DIfStmt)
	de, ok := di.Cont.(ast.DElseStmt)
	if !ok { t.Fatalf("cont: got %T", di.Cont) }
	if len(de.Body) != 1 { t.Errorf("else body: got %d", len(de.Body)) }
}

func TestParseTarget(t *testing.T) {
	sf := parse("#target RealLive")
	dt := sf.Stmts[0].(ast.DTargetStmt)
	if dt.Target != "RealLive" { t.Errorf("target: got %q", dt.Target) }
}

func TestParseVersion(t *testing.T) {
	sf := parse("#version 1.2.3.4")
	dv := sf.Stmts[0].(ast.DVersionStmt)
	a := dv.A.(ast.IntLit)
	b := dv.B.(ast.IntLit)
	if a.Val != 1 || b.Val != 2 { t.Errorf("version: a=%d b=%d", a.Val, b.Val) }
}

func TestParseDecl(t *testing.T) {
	sf := parse("int x = 10")
	ds, ok := sf.Stmts[0].(ast.DeclStmt)
	if !ok { t.Fatalf("got %T", sf.Stmts[0]) }
	if ds.Type.BitWidth != 32 { t.Errorf("bitwidth: got %d", ds.Type.BitWidth) }
	if len(ds.Vars) != 1 { t.Fatalf("vars: got %d", len(ds.Vars)) }
	if ds.Vars[0].Ident != "x" { t.Errorf("name: got %q", ds.Vars[0].Ident) }
}

func TestParseDeclStr(t *testing.T) {
	sf := parse("str msg = 'hello'")
	ds := sf.Stmts[0].(ast.DeclStmt)
	if !ds.Type.IsStr { t.Error("expected str type") }
}

func TestParseDeclArray(t *testing.T) {
	sf := parse("int arr[10]")
	ds := sf.Stmts[0].(ast.DeclStmt)
	sz, ok := ds.Vars[0].ArraySize.(ast.IntLit)
	if !ok { t.Fatalf("array size: got %T", ds.Vars[0].ArraySize) }
	if sz.Val != 10 { t.Errorf("size: got %d", sz.Val) }
}

func TestParseReturn(t *testing.T) {
	sf := parse("return 42")
	rs, ok := sf.Stmts[0].(ast.ReturnStmt)
	if !ok { t.Fatalf("got %T", sf.Stmts[0]) }
	if !rs.Explicit { t.Error("expected explicit return") }
}

func TestParseParens(t *testing.T) {
	l := lexer.New("(1 + 2) * 3", "test")
	p := New(l)
	expr := p.ParseExpression()
	bo, ok := expr.(ast.BinOp)
	if !ok { t.Fatalf("got %T", expr) }
	if bo.Op != ast.OpMul { t.Errorf("top: got %v", bo.Op) }
}

func TestParseComplexParam(t *testing.T) {
	sf := parse("foo({1, 2})")
	fc := sf.Stmts[0].(ast.FuncCallStmt)
	if len(fc.Params) != 1 { t.Fatalf("params: got %d", len(fc.Params)) }
	cp, ok := fc.Params[0].(ast.ComplexParam)
	if !ok { t.Fatalf("param: got %T", fc.Params[0]) }
	if len(cp.Exprs) != 2 { t.Errorf("complex exprs: got %d", len(cp.Exprs)) }
}

func TestParseRaw(t *testing.T) {
	sf := parse("raw #ff 0 endraw")
	rs, ok := sf.Stmts[0].(ast.RawCodeStmt)
	if !ok { t.Fatalf("got %T", sf.Stmts[0]) }
	if len(rs.Elts) < 1 { t.Error("expected raw elements") }
}

func TestParseMixedProgram(t *testing.T) {
	src := `#define MAX = 100
#target RealLive
#version 1.0

int counter = 0
@loop
  intA[0] += 1
  if intA[0] == MAX
    goto @done
while intA[0] < MAX halt

@done
halt`
	sf, err := ParseFile([]byte(src), "test.org")
	if err != nil { t.Fatal(err) }
	if len(sf.Stmts) < 5 {
		t.Errorf("mixed: got %d stmts, want >= 5", len(sf.Stmts))
	}
}

func TestParseSelect(t *testing.T) {
	sf := parse("select('option1', 'option2')")
	ss, ok := sf.Stmts[0].(ast.SelectStmt)
	if !ok { t.Fatalf("got %T", sf.Stmts[0]) }
	if ss.Ident != "select" { t.Errorf("ident: got %q", ss.Ident) }
	if ss.Opcode != 1 { t.Errorf("opcode: got %d", ss.Opcode) }
	if len(ss.Params) != 2 { t.Errorf("params: got %d", len(ss.Params)) }
}

func TestParseFile(t *testing.T) {
	sf, err := ParseFile([]byte("halt"), "test.org")
	if err != nil { t.Fatal(err) }
	if len(sf.Stmts) != 1 { t.Errorf("got %d stmts", len(sf.Stmts)) }

	// Verify type is correct
	_ = sf.Stmts[0].(ast.HaltStmt)
}

// Verify the lexer → parser → AST pipeline with token types
func TestParserUsesTokenPackage(t *testing.T) {
	// Just verify the imports work correctly
	_ = token.IF
	_ = token.EOF
}
