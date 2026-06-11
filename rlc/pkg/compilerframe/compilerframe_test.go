package compilerframe

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/yoremi/rldev-go/pkg/encoding"
	"github.com/yoremi/rldev-go/pkg/texttransforms"
	"github.com/yoremi/rldev-go/rlc/pkg/ast"
	"github.com/yoremi/rldev-go/rlc/pkg/codegen"
	"github.com/yoremi/rldev-go/rlc/pkg/ini"
	"github.com/yoremi/rldev-go/rlc/pkg/kfn"
	"github.com/yoremi/rldev-go/rlc/pkg/memory"
)

func newComp() *Compiler {
	return New(kfn.NewRegistry(), ini.NewTable())
}

func TestNew(t *testing.T) {
	c := newComp()
	if c.Mem == nil {
		t.Error("Mem")
	}
	if c.Out == nil {
		t.Error("Out")
	}
	if c.Norm == nil {
		t.Error("Norm")
	}
	if c.Directive == nil {
		t.Error("Directive")
	}
	if c.Intrin == nil {
		t.Error("Intrin")
	}
	if c.Reg == nil {
		t.Error("Reg")
	}
	if c.Ini == nil {
		t.Error("Ini")
	}
	if c.State == nil {
		t.Error("State")
	}
}

func TestMetaCallbackWired(t *testing.T) {
	c := newComp()
	// meta.State.CompileStatements should delegate to c.Parse
	if c.State.CompileStatements == nil {
		t.Error("callback not wired")
	}
	// Trigger via meta
	c.State.ParseOne(ast.HaltStmt{})
	// Halt should have been emitted
	if c.Out.Length() == 0 {
		t.Error("halt not emitted via meta callback")
	}
}

func TestParseEmpty(t *testing.T) {
	c := newComp()
	c.Parse(nil)
	if c.HasErrors() {
		t.Error("should have no errors")
	}
}

func TestParseHalt(t *testing.T) {
	c := newComp()
	c.Parse([]ast.Stmt{ast.HaltStmt{}})
	if c.Out.Length() == 0 {
		t.Error("halt not emitted")
	}
}

func TestParseEofThenHaltPreservesTrailerAndHalt(t *testing.T) {
	c := newComp()
	c.EmitEOFMarkers = true
	c.Parse([]ast.Stmt{
		ast.EOFStmt{Loc: ast.Loc{File: "t", Line: 10}},
		ast.HaltStmt{Loc: ast.Loc{File: "t", Line: 11}},
	})

	if !c.SeenEndEmitted {
		t.Fatal("eof marker was not recorded")
	}
	if len(c.Out.IR) != 2 {
		t.Fatalf("IR length = %d, want trailer + halt", len(c.Out.IR))
	}
	if !bytes.Equal(c.Out.IR[0].Bytes, SeenEndTrailerBytes()) {
		t.Fatalf("first IR is not SeenEnd trailer")
	}
	if !bytes.Equal(c.Out.IR[1].Bytes, []byte{0x00}) {
		t.Fatalf("second IR = % x, want halt", c.Out.IR[1].Bytes)
	}
}

func TestParseDirective(t *testing.T) {
	c := newComp()
	c.Parse([]ast.Stmt{ast.DefineStmt{Ident: "X", Value: ast.IntLit{Val: 42}}})
	if !c.Mem.Defined("X") {
		t.Error("X not defined")
	}
}

func TestBareOpLiteralCompilesWithoutUnresolvedWarning(t *testing.T) {
	c := newComp()
	c.ParseElt(ast.VarOrFuncStmt{
		Loc:   ast.Loc{File: "SEEN8005.ke", Line: 2645},
		Ident: "op<1:060:00111,0>",
	})

	if c.HasErrors() {
		t.Fatalf("bare op literal should not error: %v", c.Errors)
	}
	if len(c.Warnings) != 0 {
		t.Fatalf("bare op literal should not warn: %v", c.Warnings)
	}
	if findCodeIR(c, codegen.EncodeOpcode(1, 60, 111, 0, 0)) < 0 {
		t.Fatalf("bare op literal did not emit raw opcode, IR=%#v", c.Out.IR)
	}
}

func TestParseLineDirectiveEmitsForcedLineRef(t *testing.T) {
	c := newComp()
	c.Parse([]ast.Stmt{
		ast.DirectiveStmt{Name: "line", Value: ast.IntLit{Val: 128}},
		ast.HaltStmt{Loc: ast.Loc{Line: 128}},
	})

	if len(c.Out.IR) < 2 {
		t.Fatalf("IR length = %d, want at least 2", len(c.Out.IR))
	}
	if c.Out.IR[0].Type != codegen.IRLineref || c.Out.IR[0].Index != 128 {
		t.Fatalf("first IR = %#v, want forced line 128", c.Out.IR[0])
	}
	if c.Out.IR[1].Type != codegen.IRCode || !bytes.Equal(c.Out.IR[1].Bytes, []byte{0x00}) {
		t.Fatalf("second IR = %#v, want halt code", c.Out.IR[1])
	}
}

func TestParseStandaloneKidokuDirectiveEmitsMarker(t *testing.T) {
	c := newComp()
	c.Parse([]ast.Stmt{
		ast.DirectiveStmt{Loc: ast.Loc{File: "t", Line: 22}, Name: "line", Value: ast.IntLit{Val: 23}},
		ast.DirectiveStmt{Loc: ast.Loc{File: "t", Line: 23}, Name: "kidoku", Value: ast.IntLit{Val: 1}},
		ast.DirectiveStmt{Loc: ast.Loc{File: "t", Line: 23}, Name: "line", Value: ast.IntLit{Val: 24}},
		ast.HaltStmt{Loc: ast.Loc{File: "t", Line: 24}},
	})

	kidoku := kidokuIRs(c)
	if len(kidoku) != 1 {
		t.Fatalf("kidoku count = %d, want 1", len(kidoku))
	}
	if kidoku[0].Index != 23 {
		t.Fatalf("kidoku line = %d, want 23", kidoku[0].Index)
	}
	kidokuPos := -1
	for i, ir := range c.Out.IR {
		if ir.Type == codegen.IRKidoku {
			kidokuPos = i
			break
		}
	}
	if kidokuPos < 0 || kidokuPos+1 >= len(c.Out.IR) ||
		c.Out.IR[kidokuPos+1].Type != codegen.IRCode ||
		!bytes.Equal(c.Out.IR[kidokuPos+1].Bytes, []byte{'"', '"'}) {
		t.Fatalf("standalone kidoku should emit an empty text run after the marker: IR=%#v", c.Out.IR)
	}
}

func TestParseKidokuDirectiveBeforeTextoutDoesNotDuplicate(t *testing.T) {
	c := newComp()
	c.Parse([]ast.Stmt{
		ast.DirectiveStmt{Loc: ast.Loc{File: "t", Line: 41}, Name: "line", Value: ast.IntLit{Val: 42}},
		ast.DirectiveStmt{Loc: ast.Loc{File: "t", Line: 42}, Name: "kidoku", Value: ast.IntLit{Val: 1}},
		ast.ReturnStmt{
			Loc: ast.Loc{File: "t", Line: 42},
			Expr: ast.StrLit{Loc: ast.Loc{File: "t", Line: 42}, Tokens: []ast.StrToken{
				ast.TextToken{Loc: ast.Loc{File: "t", Line: 42}, Text: "hello"},
			}},
		},
	})

	kidoku := kidokuIRs(c)
	if len(kidoku) != 1 {
		t.Fatalf("kidoku count = %d, want 1", len(kidoku))
	}
	if kidoku[0].Index != 42 {
		t.Fatalf("kidoku line = %d, want 42", kidoku[0].Index)
	}
}

func TestParseKidokuLineDirectiveUsesExplicitLine(t *testing.T) {
	c := newComp()
	c.Parse([]ast.Stmt{
		ast.DirectiveStmt{Loc: ast.Loc{File: "t", Line: 7}, Name: "kidoku_line", Value: ast.IntLit{Val: 129}},
		ast.ReturnStmt{
			Loc: ast.Loc{File: "t", Line: 8},
			Expr: ast.StrLit{Loc: ast.Loc{File: "t", Line: 8}, Tokens: []ast.StrToken{
				ast.TextToken{Loc: ast.Loc{File: "t", Line: 8}, Text: "hello"},
			}},
		},
	})

	kidoku := kidokuIRs(c)
	if len(kidoku) != 1 {
		t.Fatalf("kidoku count = %d, want 1", len(kidoku))
	}
	if kidoku[0].Index != 129 {
		t.Fatalf("kidoku line = %d, want explicit line 129", kidoku[0].Index)
	}
}

func TestParseCompactLineSuppressesPhysicalLineRefs(t *testing.T) {
	c := newComp()
	c.Parse([]ast.Stmt{
		ast.DirectiveStmt{Loc: ast.Loc{File: "t", Line: 7}, Name: "line_compact", Value: ast.IntLit{Val: 129}},
		ast.HaltStmt{Loc: ast.Loc{File: "t", Line: 8}},
	})
	if len(c.Out.IR) != 2 {
		t.Fatalf("IR length = %d, want compact line + halt only: %#v", len(c.Out.IR), c.Out.IR)
	}
	if c.Out.IR[0].Type != codegen.IRLineref || c.Out.IR[0].Index != 129 {
		t.Fatalf("first IR = %#v, want compact line 129", c.Out.IR[0])
	}
	if c.Out.IR[1].Type != codegen.IRCode || !bytes.Equal(c.Out.IR[1].Bytes, []byte{0x00}) {
		t.Fatalf("second IR = %#v, want halt code", c.Out.IR[1])
	}
}

func TestParseExplicitKidokuModeSuppressesUnannotatedTextout(t *testing.T) {
	c := newComp()
	c.Parse([]ast.Stmt{
		ast.DirectiveStmt{Loc: ast.Loc{File: "t", Line: 23}, Name: "kidoku", Value: ast.IntLit{Val: 1}},
		ast.ReturnStmt{
			Loc: ast.Loc{File: "t", Line: 56},
			Expr: ast.StrLit{Loc: ast.Loc{File: "t", Line: 56}, Tokens: []ast.StrToken{
				ast.TextToken{Loc: ast.Loc{File: "t", Line: 56}, Text: "no marker here"},
			}},
		},
	})

	kidoku := kidokuIRs(c)
	if len(kidoku) != 1 {
		t.Fatalf("kidoku count = %d, want only the explicit marker", len(kidoku))
	}
	if kidoku[0].Index != 23 {
		t.Fatalf("kidoku line = %d, want 23", kidoku[0].Index)
	}
}

func TestParseTODO(t *testing.T) {
	// Partially-implemented statement types should produce warnings, not errors
	c := newComp()
	c.Parse([]ast.Stmt{ast.LoadFileStmt{Loc: ast.Loc{File: "t", Line: 1}, Path: ast.StrLit{
		Tokens: []ast.StrToken{ast.TextToken{Text: "test.kh"}},
	}}})
	if c.HasErrors() {
		t.Error("should not have errors, only warnings")
	}
	if len(c.Warnings) == 0 {
		t.Error("should have pending warning")
	}
}

func TestCompileMergesDirectiveDiagnostics(t *testing.T) {
	c := newComp()
	// Trigger a directive error by undefining something that doesn't exist
	c.Compile([]ast.Stmt{
		ast.DUndefStmt{Loc: ast.Loc{File: "t", Line: 1}, Idents: []string{"NOPE"}},
	})
	if !c.HasErrors() {
		t.Error("should have merged directive errors")
	}
}

func TestHasErrors(t *testing.T) {
	c := newComp()
	if c.HasErrors() {
		t.Error("fresh compiler should have no errors")
	}
	c.error(ast.Nowhere, "test")
	if !c.HasErrors() {
		t.Error("should have errors")
	}
}

func TestRecursiveParse(t *testing.T) {
	c := newComp()
	c.Parse([]ast.Stmt{
		ast.DefineStmt{Ident: "A", Value: ast.IntLit{Val: 1}},
		ast.DefineStmt{Ident: "B", Value: ast.IntLit{Val: 2}},
		ast.HaltStmt{},
	})
	if !c.Mem.Defined("A") {
		t.Error("A")
	}
	if !c.Mem.Defined("B") {
		t.Error("B")
	}
}

// --- Structure tests ---

func TestParseSeq(t *testing.T) {
	c := newComp()
	c.ParseElt(ast.SeqStmt{Stmts: []ast.Stmt{
		ast.DefineStmt{Ident: "S1", Value: ast.IntLit{Val: 1}},
		ast.HaltStmt{},
	}})
	if !c.Mem.Defined("S1") {
		t.Error("S1 not defined in seq")
	}
}

func TestParseBlock(t *testing.T) {
	c := newComp()
	c.ParseElt(ast.BlockStmt{Stmts: []ast.Stmt{
		ast.DefineStmt{Ident: "BLK", Value: ast.IntLit{Val: 1}},
	}})
	// BLK should be undefined after block scope closes
	// (depends on scoped define implementation)
}

func TestParseIfConstTrue(t *testing.T) {
	c := newComp()
	// if(1) then define X
	c.ParseElt(ast.IfStmt{
		Cond: ast.IntLit{Val: 1},
		Then: ast.DefineStmt{Ident: "X_TRUE", Value: ast.IntLit{Val: 1}},
	})
	if !c.Mem.Defined("X_TRUE") {
		t.Error("X_TRUE should be defined (const-true branch)")
	}
}

func TestParseIfConstFalse(t *testing.T) {
	c := newComp()
	// if(0) then define X else define Y
	c.ParseElt(ast.IfStmt{
		Cond: ast.IntLit{Val: 0},
		Then: ast.DefineStmt{Ident: "IF_THEN", Value: ast.IntLit{Val: 1}},
		Else: ast.DefineStmt{Ident: "IF_ELSE", Value: ast.IntLit{Val: 2}},
	})
	if c.Mem.Defined("IF_THEN") {
		t.Error("IF_THEN should NOT be defined (const-false)")
	}
	if !c.Mem.Defined("IF_ELSE") {
		t.Error("IF_ELSE should be defined (else branch)")
	}
}

func TestParseWhileConstFalse(t *testing.T) {
	c := newComp()
	// while(0) define X → should not define X
	c.ParseElt(ast.WhileStmt{
		Cond: ast.IntLit{Val: 0},
		Body: ast.DefineStmt{Ident: "W_BODY", Value: ast.IntLit{Val: 1}},
	})
	if c.Mem.Defined("W_BODY") {
		t.Error("W_BODY should NOT be defined (while(0))")
	}
}

func TestBreakOutsideLoop(t *testing.T) {
	c := newComp()
	c.ParseNormElt(ast.BreakStmt{Loc: ast.Loc{File: "t", Line: 1}})
	if !c.HasErrors() {
		t.Error("break outside loop should error")
	}
}

func TestContinueOutsideLoop(t *testing.T) {
	c := newComp()
	c.ParseNormElt(ast.ContinueStmt{Loc: ast.Loc{File: "t", Line: 1}})
	if !c.HasErrors() {
		t.Error("continue outside loop should error")
	}
}

func TestLabelEmit(t *testing.T) {
	c := newComp()
	c.ParseNormElt(ast.LabelStmt{Label: ast.Label{Ident: "mytest"}})
	if c.Out.Length() == 0 {
		t.Error("label should be emitted")
	}
}

func TestAssignEmit(t *testing.T) {
	c := newComp()
	c.ParseNormElt(ast.AssignStmt{
		Dest: ast.StoreRef{},
		Op:   ast.AssignSet,
		Expr: ast.IntLit{Val: 42},
	})
	if c.Out.Length() == 0 {
		t.Error("assignment should be emitted")
	}
}

func TestCompileTopLevelControlCode(t *testing.T) {
	c := newComp()
	c.Reg.Register(&kfn.FuncDef{
		Ident:    "wait",
		CCStr:    "wait",
		OpType:   1,
		OpModule: 0,
		OpCode:   100,
		Prototypes: []kfn.Prototype{{
			Defined: true,
			Params:  []kfn.Parameter{{Type: kfn.PInt}},
		}},
	})
	c.ParseNormElt(ast.FuncCallStmt{
		Loc:   ast.Loc{File: "t", Line: 1},
		Ident: `\wait`,
		Params: []ast.Param{ast.SimpleParam{
			Loc:  ast.Loc{File: "t", Line: 1},
			Expr: ast.IntLit{Loc: ast.Loc{File: "t", Line: 1}, Val: 3000},
		}},
	})
	if c.HasErrors() {
		t.Fatalf("control code compile errors: %v", c.Errors)
	}
	if c.Out.Length() == 0 {
		t.Fatal("control code should emit bytecode")
	}
}

func TestNormalizeCondParamNestedNot(t *testing.T) {
	loc := ast.Loc{File: "t", Line: 1}
	param := ast.SimpleParam{Loc: loc, Expr: ast.CmpExpr{
		Loc: loc,
		LHS: ast.UnaryExpr{
			Loc: loc,
			Op:  ast.UnaryNot,
			Val: ast.IntVar{Loc: loc, Bank: 0, Index: ast.IntLit{Loc: loc, Val: 3}},
		},
		Op:  ast.CmpEqu,
		RHS: ast.IntLit{Loc: loc, Val: 218},
	}}
	out := normalizeCondParam(param).(ast.SimpleParam)
	cmp := out.Expr.(ast.CmpExpr)
	if _, ok := cmp.LHS.(ast.CmpExpr); !ok {
		t.Fatalf("nested ! should lower directly to a comparison, got %T", cmp.LHS)
	}
}

func TestNormalizeCondParamBareExprBecomesBooleanCompare(t *testing.T) {
	loc := ast.Loc{File: "t", Line: 1}
	param := ast.SimpleParam{Loc: loc, Expr: ast.IntVar{
		Loc:   loc,
		Bank:  5,
		Index: ast.IntLit{Loc: loc, Val: 1013},
	}}
	out := normalizeCondParam(param).(ast.SimpleParam)
	cmp, ok := out.Expr.(ast.CmpExpr)
	if !ok {
		t.Fatalf("bare condition should become comparison, got %T", out.Expr)
	}
	if cmp.Op != ast.CmpNeq {
		t.Fatalf("condition op = %v, want !=", cmp.Op)
	}
	if rhs, ok := cmp.RHS.(ast.IntLit); !ok || rhs.Val != 0 {
		t.Fatalf("condition rhs = %#v, want 0", cmp.RHS)
	}
	if _, ok := cmp.LHS.(ast.IntVar); !ok {
		t.Fatalf("condition lhs = %T, want IntVar", cmp.LHS)
	}
}

func TestConditionalParamSetOnlyConditionTags(t *testing.T) {
	fd := &kfn.FuncDef{Prototypes: []kfn.Prototype{{
		Defined: true,
		Params: []kfn.Parameter{
			{Type: kfn.PStrC, Flags: []kfn.ParamFlag{kfn.FUncount, kfn.FTagged}, Tag: "filename"},
			{Type: kfn.PIntC, Flags: []kfn.ParamFlag{kfn.FUncount, kfn.FTagged}, Tag: "condition"},
			{Type: kfn.PIntC, Flags: []kfn.ParamFlag{kfn.FUncount, kfn.FTagged}, Tag: "conditional"},
		},
	}}}
	uncounted := uncountParamSet(fd, 0, 3)
	for i, got := range uncounted {
		if !got {
			t.Fatalf("uncount param %d = false, want true", i)
		}
	}
	conds := conditionalParamSet(fd, 0, 3)
	if conds[0] {
		t.Fatal("uncounted filename parameter should not be normalized as a condition")
	}
	if !conds[1] || !conds[2] {
		t.Fatalf("condition tags = %v, want only condition/conditional true", conds)
	}
}

func TestUnquotedStringParamAfterIntegerGetsComma(t *testing.T) {
	c := newComp()
	c.Reg.Register(&kfn.FuncDef{
		Ident:    "objOfFile",
		OpType:   1,
		OpModule: 71,
		OpCode:   1000,
		Prototypes: []kfn.Prototype{{
			Defined: true,
			Params: []kfn.Parameter{
				{Type: kfn.PIntC},
				{Type: kfn.PStrC},
			},
		}},
	})
	loc := ast.Loc{File: "t", Line: 1}
	c.Parse([]ast.Stmt{ast.FuncCallStmt{
		Loc:   loc,
		Ident: "objOfFile",
		Params: []ast.Param{
			ast.SimpleParam{Loc: loc, Expr: ast.IntLit{Loc: loc, Val: 0}},
			ast.SimpleParam{Loc: loc, Expr: ast.StrLit{Loc: loc, Tokens: []ast.StrToken{
				ast.TextToken{Loc: loc, Text: "SIROS"},
			}}},
		},
	}})
	if c.HasErrors() {
		t.Fatalf("compile errors: %v", c.Errors)
	}
	want := append([]byte{'('}, codegen.EncodeInt32(0)...)
	want = append(want, ',')
	want = append(want, []byte("SIROS)")...)
	if !bytes.Contains(compilerOutputBytes(c), want) {
		t.Fatalf("compiled call missing comma before unquoted string; want sequence % x in % x", want, compilerOutputBytes(c))
	}
}

func TestExplicitReturnParamChoosesFullOverload(t *testing.T) {
	c := newComp()
	registerItoa(c)
	loc := ast.Loc{File: "t", Line: 1}
	c.Parse([]ast.Stmt{ast.FuncCallStmt{
		Loc:   loc,
		Ident: "itoa",
		Params: []ast.Param{
			ast.SimpleParam{Loc: loc, Expr: ast.IntVar{Loc: loc, Bank: 5, Index: ast.IntLit{Loc: loc, Val: 3}}},
			ast.SimpleParam{Loc: loc, Expr: ast.StrVar{Loc: loc, Bank: 18, Index: ast.IntLit{Loc: loc, Val: 0}}},
			ast.SimpleParam{Loc: loc, Expr: ast.IntLit{Loc: loc, Val: 2}},
		},
	}})
	if c.HasErrors() {
		t.Fatalf("compile errors: %v", c.Errors)
	}
	if findCodeIR(c, codegen.EncodeOpcode(1, 10, 17, 3, 1)) < 0 {
		t.Fatal("itoa with explicit return parameter should use overload 1")
	}
}

func TestLegacyRealLiveItoaLengthUsesKFNOverload(t *testing.T) {
	c := newComp()
	c.Reg.Version = kfn.Version{1, 2, 3, 5}
	registerItoa(c)
	loc := ast.Loc{File: "t", Line: 1}
	c.Parse([]ast.Stmt{ast.FuncCallStmt{
		Loc:   loc,
		Ident: "itoa",
		Params: []ast.Param{
			ast.SimpleParam{Loc: loc, Expr: ast.IntVar{Loc: loc, Bank: 5, Index: ast.IntLit{Loc: loc, Val: 100}}},
			ast.SimpleParam{Loc: loc, Expr: ast.StrVar{Loc: loc, Bank: 18, Index: ast.IntLit{Loc: loc, Val: 1}}},
			ast.SimpleParam{Loc: loc, Expr: ast.IntLit{Loc: loc, Val: 4}},
		},
	}})
	if c.HasErrors() {
		t.Fatalf("compile errors: %v", c.Errors)
	}
	if findCodeIR(c, codegen.EncodeOpcode(1, 10, 17, 3, 1)) < 0 {
		t.Fatal("RealLive 1.2.3 itoa length form should use the KFN overload")
	}
}

func TestAirRealLiveItoaAssignmentKeepsOverloadOne(t *testing.T) {
	c := newComp()
	c.Reg.Version = kfn.Version{1, 2, 9, 5}
	registerItoa(c)
	loc := ast.Loc{File: "t", Line: 1}
	c.Parse([]ast.Stmt{ast.AssignStmt{
		Loc:  loc,
		Dest: ast.StrVar{Loc: loc, Bank: 18, Index: ast.IntLit{Loc: loc, Val: 0}},
		Op:   ast.AssignSet,
		Expr: ast.FuncCall{
			Loc:   loc,
			Ident: "itoa",
			Params: []ast.Param{
				ast.SimpleParam{Loc: loc, Expr: ast.IntVar{Loc: loc, Bank: 5, Index: ast.IntLit{Loc: loc, Val: 3}}},
				ast.SimpleParam{Loc: loc, Expr: ast.IntLit{Loc: loc, Val: 2}},
			},
		},
	}})
	if c.HasErrors() {
		t.Fatalf("compile errors: %v", c.Errors)
	}
	if findCodeIR(c, codegen.EncodeOpcode(1, 10, 17, 3, 1)) < 0 {
		t.Fatal("RealLive 1.2.9 itoa assignment should keep overload 1")
	}
}

func TestReturnAssignmentInjectsDestinationBeforeStringRewrite(t *testing.T) {
	c := newComp()
	registerItoa(c)
	loc := ast.Loc{File: "t", Line: 1}
	c.Parse([]ast.Stmt{ast.AssignStmt{
		Loc:  loc,
		Dest: ast.StrVar{Loc: loc, Bank: 18, Index: ast.IntLit{Loc: loc, Val: 0}},
		Op:   ast.AssignSet,
		Expr: ast.FuncCall{
			Loc:   loc,
			Ident: "itoa",
			Params: []ast.Param{
				ast.SimpleParam{Loc: loc, Expr: ast.IntVar{Loc: loc, Bank: 5, Index: ast.IntLit{Loc: loc, Val: 3}}},
				ast.SimpleParam{Loc: loc, Expr: ast.IntLit{Loc: loc, Val: 2}},
			},
		},
	}})
	if c.HasErrors() {
		t.Fatalf("compile errors: %v", c.Errors)
	}
	if findCodeIR(c, codegen.EncodeOpcode(1, 10, 17, 3, 1)) < 0 {
		t.Fatal("itoa assignment should inject destination and use overload 1")
	}
}

func TestStrsubThreeArgUsesShortOverload(t *testing.T) {
	c := newComp()
	registerStrsub(c)
	loc := ast.Loc{File: "t", Line: 1}
	c.Parse([]ast.Stmt{ast.FuncCallStmt{
		Loc:   loc,
		Ident: "strsub",
		Params: []ast.Param{
			ast.SimpleParam{Loc: loc, Expr: ast.StrVar{Loc: loc, Bank: 18, Index: ast.IntLit{Loc: loc, Val: 1}}},
			ast.SimpleParam{Loc: loc, Expr: ast.StrVar{Loc: loc, Bank: 18, Index: ast.IntLit{Loc: loc, Val: 1004}}},
			ast.SimpleParam{Loc: loc, Expr: ast.IntLit{Loc: loc, Val: 3}},
		},
	}})
	if c.HasErrors() {
		t.Fatalf("compile errors: %v", c.Errors)
	}
	if findCodeIR(c, codegen.EncodeOpcode(1, 10, 5, 3, 0)) < 0 {
		t.Fatal("three-argument strsub should use overload 0")
	}
}

func TestStrsubAssignmentInjectsDestinationWithShortOverload(t *testing.T) {
	c := newComp()
	registerStrsub(c)
	loc := ast.Loc{File: "t", Line: 1}
	c.Parse([]ast.Stmt{ast.AssignStmt{
		Loc:  loc,
		Dest: ast.StrVar{Loc: loc, Bank: 18, Index: ast.IntLit{Loc: loc, Val: 1}},
		Op:   ast.AssignSet,
		Expr: ast.FuncCall{
			Loc:   loc,
			Ident: "strsub",
			Params: []ast.Param{
				ast.SimpleParam{Loc: loc, Expr: ast.StrVar{Loc: loc, Bank: 18, Index: ast.IntLit{Loc: loc, Val: 1004}}},
				ast.SimpleParam{Loc: loc, Expr: ast.IntLit{Loc: loc, Val: 4}},
			},
		},
	}})
	if c.HasErrors() {
		t.Fatalf("compile errors: %v", c.Errors)
	}
	if findCodeIR(c, codegen.EncodeOpcode(1, 10, 5, 3, 0)) < 0 {
		t.Fatal("strsub assignment should inject destination and use overload 0")
	}
}

func TestStrsubFourArgUsesLongOverload(t *testing.T) {
	c := newComp()
	registerStrsub(c)
	loc := ast.Loc{File: "t", Line: 1}
	c.Parse([]ast.Stmt{ast.FuncCallStmt{
		Loc:   loc,
		Ident: "strsub",
		Params: []ast.Param{
			ast.SimpleParam{Loc: loc, Expr: ast.StrVar{Loc: loc, Bank: 18, Index: ast.IntLit{Loc: loc, Val: 0}}},
			ast.SimpleParam{Loc: loc, Expr: ast.StrVar{Loc: loc, Bank: 18, Index: ast.IntLit{Loc: loc, Val: 1011}}},
			ast.SimpleParam{Loc: loc, Expr: ast.IntVar{Loc: loc, Bank: 10, Index: ast.IntLit{Loc: loc, Val: 0}}},
			ast.SimpleParam{Loc: loc, Expr: ast.IntLit{Loc: loc, Val: 1}},
		},
	}})
	if c.HasErrors() {
		t.Fatalf("compile errors: %v", c.Errors)
	}
	if findCodeIR(c, codegen.EncodeOpcode(1, 10, 5, 4, 1)) < 0 {
		t.Fatal("four-argument strsub should use overload 1")
	}
}

func TestSameArityOverloadUsesStringTypes(t *testing.T) {
	c := newComp()
	c.Reg.Register(&kfn.FuncDef{
		Ident:    "TypedPair",
		OpType:   0,
		OpModule: 4,
		OpCode:   1999,
		Prototypes: []kfn.Prototype{
			{Defined: true, Params: []kfn.Parameter{{Type: kfn.PIntC}}},
			{Defined: true, Params: []kfn.Parameter{{Type: kfn.PIntC}, {Type: kfn.PIntC}}},
			{Defined: true, Params: []kfn.Parameter{{Type: kfn.PStr}, {Type: kfn.PStr}}},
		},
	})
	loc := ast.Loc{File: "t", Line: 1}
	c.ParseElt(ast.FuncCallStmt{
		Loc:   loc,
		Ident: "TypedPair",
		Params: []ast.Param{
			ast.SimpleParam{Loc: loc, Expr: ast.StrVar{Loc: loc, Bank: 10, Index: ast.IntLit{Loc: loc, Val: 1011}}},
			ast.SimpleParam{Loc: loc, Expr: ast.StrVar{Loc: loc, Bank: 10, Index: ast.IntLit{Loc: loc, Val: 1012}}},
		},
	})
	if c.HasErrors() {
		t.Fatalf("compile errors: %v", c.Errors)
	}
	if findCodeIR(c, codegen.EncodeOpcode(0, 4, 1999, 2, 2)) < 0 {
		t.Fatal("same-arity string overload should use the string prototype")
	}
	if findCodeIR(c, codegen.EncodeOpcode(0, 4, 1999, 2, 1)) >= 0 {
		t.Fatal("same-arity string overload used the integer prototype")
	}
}

func TestPlanetarianLocalFlagExcopyStringUsesOverload3(t *testing.T) {
	c := newComp()
	c.Reg.Register(&kfn.FuncDef{
		Ident:    "CCOM_LOCAL_FLAG_EXCOPY",
		OpType:   0,
		OpModule: 4,
		OpCode:   2000,
		Prototypes: []kfn.Prototype{
			{Defined: true, Params: []kfn.Parameter{{Type: kfn.PInt}}},
			{Defined: true, Params: []kfn.Parameter{{Type: kfn.PInt}, {Type: kfn.PInt}}},
			{Defined: true, Params: []kfn.Parameter{{Type: kfn.PStr}, {Type: kfn.PStr}}},
		},
	})
	loc := ast.Loc{File: "t", Line: 1}
	c.ParseElt(ast.FuncCallStmt{
		Loc:   loc,
		Ident: "CCOM_LOCAL_FLAG_EXCOPY",
		Params: []ast.Param{
			ast.SimpleParam{Loc: loc, Expr: ast.StrVar{Loc: loc, Bank: 10, Index: ast.IntLit{Loc: loc, Val: 1011}}},
			ast.SimpleParam{Loc: loc, Expr: ast.StrVar{Loc: loc, Bank: 10, Index: ast.IntLit{Loc: loc, Val: 1011}}},
		},
	})
	if c.HasErrors() {
		t.Fatalf("compile errors: %v", c.Errors)
	}
	if findCodeIR(c, codegen.EncodeOpcode(0, 4, 2000, 2, 3)) < 0 {
		t.Fatal("Planetarian CCOM_LOCAL_FLAG_EXCOPY(str,str) should emit overload 3")
	}
	if findCodeIR(c, codegen.EncodeOpcode(0, 4, 2000, 2, 1)) >= 0 {
		t.Fatal("Planetarian CCOM_LOCAL_FLAG_EXCOPY(str,str) used the integer-pair overload")
	}
	if findCodeIR(c, codegen.EncodeOpcode(0, 4, 2000, 2, 2)) >= 0 {
		t.Fatal("Planetarian CCOM_LOCAL_FLAG_EXCOPY(str,str) used the internal prototype index as the bytecode overload")
	}
}

func registerSetLocalNameForTest(c *Compiler) {
	c.Reg.Register(&kfn.FuncDef{
		Ident:    "SetLocalName",
		OpType:   1,
		OpModule: 13,
		OpCode:   11,
		Prototypes: []kfn.Prototype{{
			Defined: true,
			Params: []kfn.Parameter{
				{Type: kfn.PIntC, Flags: []kfn.ParamFlag{kfn.FTagged}, Tag: "index"},
				{Type: kfn.PStrC, Flags: []kfn.ParamFlag{kfn.FTagged}, Tag: "name"},
			},
		}},
	})
}

func TestFunctionCallDoesNotSeparateIntThenUnquotedStringParam(t *testing.T) {
	c := newComp()
	registerSetLocalNameForTest(c)
	loc := ast.Loc{File: "t", Line: 1}
	name := "《渚の名前》"
	c.Parse([]ast.Stmt{ast.FuncCallStmt{
		Loc:   loc,
		Ident: "SetLocalName",
		Params: []ast.Param{
			ast.SimpleParam{Loc: loc, Expr: ast.IntLit{Loc: loc, Val: 0}},
			ast.SimpleParam{Loc: loc, Expr: ast.StrLit{Loc: loc, Tokens: []ast.StrToken{
				ast.TextToken{Loc: loc, Text: name},
			}}},
		},
	}})
	if c.HasErrors() {
		t.Fatalf("compile errors: %v", c.Errors)
	}
	encodedName, err := encoding.UTF8ToSJS(name)
	if err != nil {
		t.Fatalf("encode name: %v", err)
	}
	args := append([]byte{'('}, codegen.EncodeInt32(0)...)
	args = append(args, encodedName...)
	args = append(args, ')')
	want := append(codegen.EncodeOpcode(1, 13, 11, 2, 0), args...)
	got := compilerOutputBytes(c)
	if !bytes.Contains(got, want) {
		t.Fatalf("SetLocalName unquoted args changed:\n got  % x\n want % x", got, want)
	}
	forbiddenArgs := append([]byte{'('}, codegen.EncodeInt32(0)...)
	forbiddenArgs = append(forbiddenArgs, ',')
	forbiddenArgs = append(forbiddenArgs, encodedName...)
	forbiddenArgs = append(forbiddenArgs, ')')
	forbidden := append(codegen.EncodeOpcode(1, 13, 11, 2, 0), forbiddenArgs...)
	if bytes.Contains(got, forbidden) {
		t.Fatalf("SetLocalName unquoted args should not have separator:\n got       % x\n forbidden % x", got, forbidden)
	}
}

func TestFunctionCallDoesNotSeparateQuotedStringParam(t *testing.T) {
	c := newComp()
	registerSetLocalNameForTest(c)
	loc := ast.Loc{File: "t", Line: 1}
	c.Parse([]ast.Stmt{ast.FuncCallStmt{
		Loc:   loc,
		Ident: "SetLocalName",
		Params: []ast.Param{
			ast.SimpleParam{Loc: loc, Expr: ast.IntLit{Loc: loc, Val: 0}},
			ast.SimpleParam{Loc: loc, Expr: ast.StrLit{Loc: loc, Tokens: []ast.StrToken{
				ast.TextToken{Loc: loc, Text: "Girl"},
			}}},
		},
	}})
	if c.HasErrors() {
		t.Fatalf("compile errors: %v", c.Errors)
	}
	got := compilerOutputBytes(c)
	want := append(codegen.EncodeOpcode(1, 13, 11, 2, 0), []byte{'(', '$', 0xff, 0, 0, 0, 0, '"', 'G', 'i', 'r', 'l', '"', ')'}...)
	if !bytes.Contains(got, want) {
		t.Fatalf("SetLocalName quoted args changed:\n got  % x\n want % x", got, want)
	}
	forbidden := append(codegen.EncodeOpcode(1, 13, 11, 2, 0), []byte{'(', '$', 0xff, 0, 0, 0, 0, ',', '"', 'G', 'i', 'r', 'l', '"', ')'}...)
	if bytes.Contains(got, forbidden) {
		t.Fatalf("SetLocalName quoted args should not have separator:\n got       % x\n forbidden % x", got, forbidden)
	}
}

func TestFunctionCallSeparatesStringVarThenASCIIStringParam(t *testing.T) {
	c := newComp()
	c.Reg.Register(&kfn.FuncDef{
		Ident:    "CompareForTest",
		OpType:   1,
		OpModule: 10,
		OpCode:   4,
		Prototypes: []kfn.Prototype{{
			Defined: true,
			Params: []kfn.Parameter{
				{Type: kfn.PStrC, Flags: []kfn.ParamFlag{kfn.FTagged}, Tag: "lhs"},
				{Type: kfn.PStrC, Flags: []kfn.ParamFlag{kfn.FTagged}, Tag: "rhs"},
			},
		}},
	})
	loc := ast.Loc{File: "t", Line: 1}
	c.Parse([]ast.Stmt{ast.FuncCallStmt{
		Loc:   loc,
		Ident: "CompareForTest",
		Params: []ast.Param{
			ast.SimpleParam{Loc: loc, Expr: ast.StrVar{Loc: loc, Bank: 11, Index: ast.IntLit{Loc: loc, Val: 1}}},
			ast.SimpleParam{Loc: loc, Expr: ast.StrLit{Loc: loc, Tokens: []ast.StrToken{
				ast.TextToken{Loc: loc, Text: "A"},
			}}},
		},
	}})
	if c.HasErrors() {
		t.Fatalf("compile errors: %v", c.Errors)
	}
	strVar := append([]byte{'$', 11, '['}, codegen.EncodeInt32(1)...)
	strVar = append(strVar, ']')
	wantArgs := append([]byte{'('}, strVar...)
	wantArgs = append(wantArgs, ',', 'A', ')')
	want := append(codegen.EncodeOpcode(1, 10, 4, 2, 0), wantArgs...)
	got := compilerOutputBytes(c)
	if !bytes.Contains(got, want) {
		t.Fatalf("string-var + ASCII string args changed:\n got  % x\n want % x", got, want)
	}
	forbiddenArgs := append([]byte{'('}, strVar...)
	forbiddenArgs = append(forbiddenArgs, 'A', ')')
	forbidden := append(codegen.EncodeOpcode(1, 10, 4, 2, 0), forbiddenArgs...)
	if bytes.Contains(got, forbidden) {
		t.Fatalf("string-var + ASCII string args should have separator:\n got       % x\n forbidden % x", got, forbidden)
	}
}

func TestFunctionCallSeparatesIntThenUnaryParam(t *testing.T) {
	c := newComp()
	c.Reg.Register(&kfn.FuncDef{
		Ident:    "UnaryParamForTest",
		OpType:   1,
		OpModule: 10,
		OpCode:   6,
		Prototypes: []kfn.Prototype{{
			Defined: true,
			Params: []kfn.Parameter{
				{Type: kfn.PIntC, Flags: []kfn.ParamFlag{kfn.FTagged}, Tag: "first"},
				{Type: kfn.PIntC, Flags: []kfn.ParamFlag{kfn.FTagged}, Tag: "second"},
			},
		}},
	})
	loc := ast.Loc{File: "t", Line: 1}
	c.Parse([]ast.Stmt{ast.FuncCallStmt{
		Loc:   loc,
		Ident: "UnaryParamForTest",
		Params: []ast.Param{
			ast.SimpleParam{Loc: loc, Expr: ast.IntLit{Loc: loc, Val: 7}},
			ast.SimpleParam{Loc: loc, Expr: ast.UnaryExpr{Loc: loc, Op: ast.UnarySub, Val: ast.IntVar{Loc: loc, Bank: 0, Index: ast.IntLit{Loc: loc, Val: 1}}}},
		},
	}})
	if c.HasErrors() {
		t.Fatalf("compile errors: %v", c.Errors)
	}
	wantArgs := append([]byte{'('}, codegen.EncodeInt32(7)...)
	wantArgs = append(wantArgs, ',')
	wantArgs = append(wantArgs, '\\', codegen.OpCode(ast.OpSub))
	wantArgs = append(wantArgs, '$', 0x00, '[')
	wantArgs = append(wantArgs, codegen.EncodeInt32(1)...)
	wantArgs = append(wantArgs, ']')
	wantArgs = append(wantArgs, ')')
	want := append(codegen.EncodeOpcode(1, 10, 6, 2, 0), wantArgs...)
	got := compilerOutputBytes(c)
	if !bytes.Contains(got, want) {
		t.Fatalf("int + unary args missing separator:\n got  % x\n want % x", got, want)
	}
}

func TestComplexParamPreservesOmittedSlotBeforeNegativeLiteral(t *testing.T) {
	c := newComp()
	c.Reg.Register(&kfn.FuncDef{
		Ident:    "InitExFramesForTest",
		OpType:   1,
		OpModule: 4,
		OpCode:   620,
		Prototypes: []kfn.Prototype{{
			Defined: true,
			Params:  []kfn.Parameter{{Type: kfn.PComplex}},
		}},
	})
	loc := ast.Loc{File: "t", Line: 1}
	c.Parse([]ast.Stmt{ast.FuncCallStmt{
		Loc:   loc,
		Ident: "InitExFramesForTest",
		Params: []ast.Param{
			ast.ComplexParam{Loc: loc, Exprs: []ast.Expr{
				ast.IntLit{Loc: loc, Val: 0},
				ast.OmittedExpr{Loc: loc},
				ast.UnaryExpr{Loc: loc, Op: ast.UnarySub, Val: ast.IntLit{Loc: loc, Val: 880}},
				ast.IntLit{Loc: loc, Val: 0},
				ast.IntLit{Loc: loc, Val: 14000},
			}},
		},
	}})
	if c.HasErrors() {
		t.Fatalf("compile errors: %v", c.Errors)
	}

	wantArgs := []byte{'(', '('}
	wantArgs = append(wantArgs, codegen.EncodeInt32(0)...)
	wantArgs = append(wantArgs, ',', '\\', codegen.OpCode(ast.OpSub))
	wantArgs = append(wantArgs, codegen.EncodeInt32(880)...)
	wantArgs = append(wantArgs, codegen.EncodeInt32(0)...)
	wantArgs = append(wantArgs, codegen.EncodeInt32(14000)...)
	wantArgs = append(wantArgs, ')', ')')
	want := append(codegen.EncodeOpcode(1, 4, 620, 1, 0), wantArgs...)
	got := compilerOutputBytes(c)
	if !bytes.Contains(got, want) {
		t.Fatalf("omitted tuple slot was not preserved:\n got  % x\n want % x", got, want)
	}

	forbiddenArgs := []byte{'(', '('}
	forbiddenArgs = append(forbiddenArgs, codegen.EncodeInt32(0)...)
	forbiddenArgs = append(forbiddenArgs, codegen.EncodeInt32(0)...)
	forbiddenArgs = append(forbiddenArgs, codegen.EncodeInt32(-880)...)
	forbiddenArgs = append(forbiddenArgs, codegen.EncodeInt32(0)...)
	forbiddenArgs = append(forbiddenArgs, codegen.EncodeInt32(14000)...)
	forbiddenArgs = append(forbiddenArgs, ')', ')')
	if bytes.Contains(got, forbiddenArgs) {
		t.Fatalf("omitted tuple slot was compiled as literal zero:\n got       % x\n forbidden % x", got, forbiddenArgs)
	}
}

// --- Compile-time structures ---

func TestDIfTrue(t *testing.T) {
	c := newComp()
	c.ParseElt(ast.DIfStmt{
		Cond: ast.IntLit{Val: 1},
		Body: []ast.Stmt{ast.DefineStmt{Ident: "DIF_T", Value: ast.IntLit{Val: 1}}},
		Cont: ast.DEndifStmt{},
	})
	if !c.Mem.Defined("DIF_T") {
		t.Error("DIF_T should be defined (#if true)")
	}
}

func TestDIfFalse(t *testing.T) {
	c := newComp()
	c.ParseElt(ast.DIfStmt{
		Cond: ast.IntLit{Val: 0},
		Body: []ast.Stmt{ast.DefineStmt{Ident: "DIF_SKIP", Value: ast.IntLit{Val: 1}}},
		Cont: ast.DEndifStmt{},
	})
	if c.Mem.Defined("DIF_SKIP") {
		t.Error("DIF_SKIP should NOT be defined (#if false)")
	}
}

func TestDIfElse(t *testing.T) {
	c := newComp()
	c.ParseElt(ast.DIfStmt{
		Cond: ast.IntLit{Val: 0},
		Body: []ast.Stmt{ast.DefineStmt{Ident: "SKIP", Value: ast.IntLit{Val: 1}}},
		Cont: ast.DElseStmt{
			Body: []ast.Stmt{ast.DefineStmt{Ident: "ELSE_HIT", Value: ast.IntLit{Val: 2}}},
		},
	})
	if c.Mem.Defined("SKIP") {
		t.Error("SKIP should not be defined")
	}
	if !c.Mem.Defined("ELSE_HIT") {
		t.Error("ELSE_HIT should be defined (#else)")
	}
}

func TestDFor(t *testing.T) {
	c := newComp()
	// #for i = 0 to 2: define body + halt
	c.ParseElt(ast.DForStmt{
		Ident: "i",
		From:  ast.IntLit{Val: 0},
		To:    ast.IntLit{Val: 2},
		Body:  ast.HaltStmt{},
	})
	// Should have emitted 3 halt instructions
	if c.Out.Length() < 3 {
		t.Errorf("expected 3 halts, IR length: %d", c.Out.Length())
	}
}

func TestDForReverse(t *testing.T) {
	c := newComp()
	c.ParseElt(ast.DForStmt{
		Ident: "j",
		From:  ast.IntLit{Val: 5},
		To:    ast.IntLit{Val: 3},
		Body:  ast.HaltStmt{},
	})
	// 5,4,3 = 3 iterations
	if c.Out.Length() < 3 {
		t.Errorf("expected 3 halts (reverse), IR length: %d", c.Out.Length())
	}
}

func TestHiding(t *testing.T) {
	c := newComp()
	c.ParseElt(ast.HidingStmt{
		Loc:   ast.Loc{File: "test.org", Line: 10},
		Ident: "myfunc",
		Body:  ast.HaltStmt{},
	})
	// __INLINE_CALL__ should be undefined after hiding completes
	if c.Mem.Defined("__INLINE_CALL__") {
		t.Error("__INLINE_CALL__ should be cleaned up")
	}
}

func TestCaseDegenerate(t *testing.T) {
	c := newComp()
	// case(expr) with no arms but a default
	c.ParseElt(ast.CaseStmt{
		Expr:    ast.IntLit{Val: 42},
		Arms:    nil,
		Default: []ast.Stmt{ast.HaltStmt{}},
	})
	if c.HasErrors() {
		t.Errorf("degenerate case should not error: %v", c.Errors)
	}
}

func TestRawCodeBytes(t *testing.T) {
	c := newComp()
	c.ParseNormElt(ast.RawCodeStmt{
		Elts: []ast.RawElt{
			{Kind: "bytes", Str: "\x01\x02\x03"},
		},
	})
	if c.Out.Length() == 0 {
		t.Error("raw bytes should be emitted")
	}
}

func TestRawCodeHex(t *testing.T) {
	c := newComp()
	c.ParseNormElt(ast.RawCodeStmt{
		Elts: []ast.RawElt{
			{Kind: "ident", Str: "#FF00"},
		},
	})
	if c.HasErrors() {
		t.Errorf("hex raw should not error: %v", c.Errors)
	}
}

// ============================================================
// Text compilation tests
// ============================================================

func TestTextStubASCII(t *testing.T) {
	c := newComp()
	c.ParseElt(ast.ReturnStmt{
		Loc: ast.Loc{Line: 1},
		Expr: ast.StrLit{Tokens: []ast.StrToken{
			ast.TextToken{Text: "Hello"},
		}},
	})
	// Should emit kidoku + quoted text
	if c.Out.Length() == 0 {
		t.Error("no output")
	}
	if c.HasErrors() {
		t.Errorf("errors: %v", c.Errors)
	}
}

func TestTextStubDQuote(t *testing.T) {
	c := newComp()
	c.ParseElt(ast.ReturnStmt{
		Loc: ast.Loc{Line: 1},
		Expr: ast.StrLit{Tokens: []ast.StrToken{
			ast.TextToken{Text: "say "},
			ast.DQuoteToken{},
			ast.TextToken{Text: "hi"},
			ast.DQuoteToken{},
		}},
	})
	if c.HasErrors() {
		t.Errorf("errors: %v", c.Errors)
	}
}

func TestTextStubSpeaker(t *testing.T) {
	c := newComp()
	c.ParseElt(ast.ReturnStmt{
		Loc: ast.Loc{Line: 1},
		Expr: ast.StrLit{Tokens: []ast.StrToken{
			ast.SpeakerToken{},
			ast.TextToken{Text: "Tomoya"},
			ast.RCurToken{},
			ast.TextToken{Text: "Hello!"},
		}},
	})
	if c.HasErrors() {
		t.Errorf("errors: %v", c.Errors)
	}
}

func TestTextStubSpeakerKeepsDefaultQuoteRun(t *testing.T) {
	c := newComp()
	c.ParseElt(ast.ReturnStmt{
		Loc: ast.Loc{Line: 1},
		Expr: ast.StrLit{Tokens: []ast.StrToken{
			ast.SpeakerToken{},
			ast.TextToken{Text: "Tomoya"},
			ast.RCurToken{},
			ast.TextToken{Text: "Hello!"},
		}},
	})
	if c.HasErrors() {
		t.Fatalf("errors: %v", c.Errors)
	}
	got := compilerOutputBytes(c)
	want := []byte{'"', '"', 0x81, 0x79, '"', 'T', 'o', 'm', 'o', 'y', 'a', '"', 0x81, 0x7a, '"', 'H', 'e', 'l', 'l', 'o', '!', '"'}
	if !bytes.Contains(got, want) {
		t.Fatalf("speaker line bytes: got % x, want contains % x", got, want)
	}
}

func TestTextStubNativeSpeakerTagsSkipEmptyQuoteRun(t *testing.T) {
	c := newComp()
	c.Out.NativeSpeakerTags = true
	c.ParseElt(ast.ReturnStmt{
		Loc: ast.Loc{Line: 1},
		Expr: ast.StrLit{Tokens: []ast.StrToken{
			ast.SpeakerToken{},
			ast.TextToken{Text: "Tomoya"},
			ast.RCurToken{},
			ast.TextToken{Text: "Hello!"},
		}},
	})
	if c.HasErrors() {
		t.Fatalf("errors: %v", c.Errors)
	}
	got := compilerOutputBytes(c)
	if bytes.Contains(got, []byte{'"', '"', 0x81, 0x79}) {
		t.Fatalf("native speaker line started with an empty quote run: % x", got)
	}
	want := []byte{0x81, 0x79, 'T', 'o', 'm', 'o', 'y', 'a', 0x81, 0x7a, '"', 'H', 'e', 'l', 'l', 'o', '!', '"'}
	if !bytes.Contains(got, want) {
		t.Fatalf("native speaker line bytes: got % x, want contains % x", got, want)
	}
}

func TestTextStubSpace(t *testing.T) {
	c := newComp()
	c.ParseElt(ast.ReturnStmt{
		Loc: ast.Loc{Line: 1},
		Expr: ast.StrLit{Tokens: []ast.StrToken{
			ast.TextToken{Text: "a"},
			ast.SpaceToken{Count: 3},
			ast.TextToken{Text: "b"},
		}},
	})
	if c.HasErrors() {
		t.Errorf("errors: %v", c.Errors)
	}
}

func TestTextStubSpecialChars(t *testing.T) {
	c := newComp()
	c.ParseElt(ast.ReturnStmt{
		Loc: ast.Loc{Line: 1},
		Expr: ast.StrLit{Tokens: []ast.StrToken{
			ast.AsteriskToken{},
			ast.PercentToken{},
			ast.HyphenToken{},
			ast.LLenticToken{},
			ast.RLenticToken{},
		}},
	})
	if c.HasErrors() {
		t.Errorf("errors: %v", c.Errors)
	}
}

func TestTextStubEmoji(t *testing.T) {
	c := newComp()
	c.ParseElt(ast.ReturnStmt{
		Loc: ast.Loc{Line: 1},
		Expr: ast.StrLit{Tokens: []ast.StrToken{
			ast.CodeToken{Ident: "e", Params: []ast.Param{
				ast.SimpleParam{Expr: ast.IntLit{Val: 5}},
			}},
		}},
	})
	if c.HasErrors() {
		t.Errorf("errors: %v", c.Errors)
	}
}

func TestTextStubName(t *testing.T) {
	c := newComp()
	c.ParseElt(ast.ReturnStmt{
		Loc: ast.Loc{Line: 1},
		Expr: ast.StrLit{Tokens: []ast.StrToken{
			ast.NameToken{Index: ast.IntLit{Val: 3}, Global: false},
		}},
	})
	if c.HasErrors() {
		t.Errorf("errors: %v", c.Errors)
	}
}

func TestTextStubResourceNameMarkerLetters(t *testing.T) {
	c := newComp()
	c.Out.ResolveRes = func(key string) (string, bool) {
		if key == "0000" {
			return `\{\m{B}}`, true
		}
		return "", false
	}
	c.ParseElt(ast.ReturnStmt{
		Loc:  ast.Loc{Line: 1},
		Expr: ast.ResRef{Key: "0000"},
	})
	var got []byte
	for _, ir := range c.Out.IR {
		got = append(got, ir.Bytes...)
	}
	if !bytes.Contains(got, []byte{0x81, 0x96, 0x82, 0x61}) {
		t.Fatalf("resource name marker should preserve B, got % x", got)
	}
	if bytes.Contains(got, []byte{0x81, 0x96, 0x82, 0x50}) {
		t.Fatalf("resource name marker compiled B as digit 1: % x", got)
	}
}

func TestTextStubUsesWesternTextTransform(t *testing.T) {
	oldMode := texttransforms.GetMode()
	oldForce := texttransforms.ForceEncode
	texttransforms.SetMode(texttransforms.EncWestern)
	texttransforms.ForceEncode = false
	defer func() {
		texttransforms.SetMode(oldMode)
		texttransforms.ForceEncode = oldForce
	}()

	c := newComp()
	c.ParseElt(ast.ReturnStmt{
		Loc: ast.Loc{Line: 1},
		Expr: ast.StrLit{Tokens: []ast.StrToken{
			ast.TextToken{Text: "de"},
			ast.TextToken{Text: "teste"},
			ast.TextToken{Text: "é"},
		}},
	})
	var got []byte
	for _, ir := range c.Out.IR {
		got = append(got, ir.Bytes...)
	}
	if !bytes.Contains(got, []byte{0xca}) {
		t.Fatalf("western transform should encode é as 0xCA, got % x", got)
	}
	if bytes.Contains(got, []byte(" ")) {
		t.Fatalf("western transform should not replace é with a space: % x", got)
	}
}

func TestScanWaitControl(t *testing.T) {
	value, consumed, ok := scanWaitControl([]rune(`...\wait{800} suite`), 3)
	if !ok {
		t.Fatal("wait control was not recognised")
	}
	if value != 800 {
		t.Fatalf("value: got %d", value)
	}
	if consumed != len(`\wait{800}`) {
		t.Fatalf("consumed: got %d", consumed)
	}
	if _, _, ok := scanWaitControl([]rune(`\wait{x}`), 0); ok {
		t.Fatal("non-numeric wait should not be recognised")
	}
}

func TestScanResourceControl(t *testing.T) {
	src, consumed, ok := scanResourceControl([]rune(`\s{strS[1016]} suite`), 0)
	if !ok {
		t.Fatal("resource control was not recognised")
	}
	if src != `\s{strS[1016]}` {
		t.Fatalf("src: got %q", src)
	}
	if consumed != len(`\s{strS[1016]}`) {
		t.Fatalf("consumed: got %d", consumed)
	}

	stmt, ok := parseResourceControl(src, ast.Loc{File: "t", Line: 1})
	if !ok {
		t.Fatal("resource control did not parse")
	}
	if stmt.Ident != `\s` || len(stmt.Params) != 1 {
		t.Fatalf("stmt: got ident=%q params=%d", stmt.Ident, len(stmt.Params))
	}
}

func TestScanResourceControlPreservesParamlessSpacing(t *testing.T) {
	src, consumed, ok := scanResourceControl([]rune(`\r  \{Misuzu}`), 0)
	if !ok {
		t.Fatal("resource control was not recognised")
	}
	if src != `\r` {
		t.Fatalf("src: got %q", src)
	}
	if consumed != len(`\r`) {
		t.Fatalf("consumed: got %d", consumed)
	}
}

func TestCompileResTextStroutControl(t *testing.T) {
	c := newComp()
	c.Reg.Register(&kfn.FuncDef{
		Ident:    "strout",
		CCStr:    "s",
		Flags:    []kfn.FuncFlag{kfn.FlagIsTextout},
		OpType:   1,
		OpModule: 1,
		OpCode:   100,
		Prototypes: []kfn.Prototype{{
			Defined: true,
			Params:  []kfn.Parameter{{Type: kfn.PStrV}},
		}},
	})

	tc := &textCompiler{c: c, loc: ast.Loc{File: "t", Line: 1}}
	tc.compileResText(`\s{strS[1016]}`)
	tc.flush()

	if c.HasErrors() {
		t.Fatalf("compile errors: %v", c.Errors)
	}
	got := compilerOutputBytes(c)
	if bytes.Contains(got, []byte(`\s`)) {
		t.Fatalf("resource control was emitted as literal text: % x", got)
	}
	if !bytes.Contains(got, []byte{'#', 1, 1, 100, 0}) {
		t.Fatalf("strout opcode was not emitted: % x", got)
	}
}

func TestCompileResTextControlKeepsTextOrder(t *testing.T) {
	c := newComp()
	c.Reg.Register(&kfn.FuncDef{
		Ident:    "shake",
		CCStr:    "shake",
		OpType:   1,
		OpModule: 2,
		OpCode:   3,
		Prototypes: []kfn.Prototype{{
			Defined: true,
			Params:  []kfn.Parameter{{Type: kfn.PInt}},
		}},
	})

	tc := &textCompiler{c: c, loc: ast.Loc{File: "t", Line: 1}}
	tc.compileResText(`before\shake{1}after`)
	tc.flush()

	if c.HasErrors() {
		t.Fatalf("compile errors: %v", c.Errors)
	}
	got := compilerOutputBytes(c)
	textPos := bytes.Index(got, []byte("before"))
	opPos := bytes.Index(got, []byte{'#', 1, 2, 3, 0})
	afterPos := bytes.Index(got, []byte("after"))
	if textPos < 0 || opPos < 0 || afterPos < 0 {
		t.Fatalf("missing expected text/opcode in % x", got)
	}
	if !(textPos < opPos && opPos < afterPos) {
		t.Fatalf("resource control order changed: before=%d opcode=%d after=%d bytes=% x", textPos, opPos, afterPos, got)
	}
}

func TestCompileResTextEscapedLeadingSpace(t *testing.T) {
	c := newComp()
	tc := &textCompiler{c: c, loc: ast.Loc{File: "t", Line: 1}}
	tc.compileResText(`\             combo`)
	tc.flush()

	if c.HasErrors() {
		t.Fatalf("compile errors: %v", c.Errors)
	}
	got := compilerOutputBytes(c)
	if bytes.Contains(got, []byte{'\\'}) {
		t.Fatalf("escaped leading space kept a literal backslash: % x", got)
	}
	if !bytes.Contains(got, []byte("             combo")) {
		t.Fatalf("escaped leading spaces were not preserved: % x", got)
	}
}

func TestCompileResTextRawByteEscapes(t *testing.T) {
	c := newComp()
	tc := &textCompiler{c: c, loc: ast.Loc{File: "t", Line: 1}}
	tc.compileResText(`\x{84}\x{02}`)
	tc.flush()

	if c.HasErrors() {
		t.Fatalf("compile errors: %v", c.Errors)
	}
	got := compilerOutputBytes(c)
	if !bytes.Equal(got, []byte{0x84, 0x02}) {
		t.Fatalf("raw byte escapes: got % x, want 84 02", got)
	}
}

func TestTextStubPlainJapaneseResourceTextoutIsBare(t *testing.T) {
	c := newComp()
	c.Out.SuppressAutoKidoku = true
	c.Out.ResolveRes = func(key string) (string, bool) {
		if key == "0000" {
			return "俺は息を詰め、引き金を絞った。", true
		}
		return "", false
	}
	loc := ast.Loc{File: "t", Line: 1}
	c.ParseElt(ast.ReturnStmt{Loc: loc, Expr: ast.ResRef{Loc: loc, Key: "0000"}})
	if c.HasErrors() {
		t.Fatalf("compile errors: %v", c.Errors)
	}
	want, err := encoding.UTF8ToSJS("俺は息を詰め、引き金を絞った。")
	if err != nil {
		t.Fatal(err)
	}
	got := compilerOutputBytes(c)
	if !bytes.Equal(got, want) {
		t.Fatalf("plain Japanese textout resource should be bare:\n got  % x\n want % x", got, want)
	}
	if len(got) > 0 && got[0] == '"' {
		t.Fatalf("plain Japanese textout resource started with quote: % x", got)
	}
}

func TestTextStubSpeakerResourceTextoutIsBare(t *testing.T) {
	c := newComp()
	c.Out.SuppressAutoKidoku = true
	c.Out.ResolveRes = func(key string) (string, bool) {
		if key == "0000" {
			return `\{ゆめみ}「ようこそ、プラネタリウムへ。」`, true
		}
		return "", false
	}
	loc := ast.Loc{File: "t", Line: 1}
	c.ParseElt(ast.ReturnStmt{Loc: loc, Expr: ast.ResRef{Loc: loc, Key: "0000"}})
	if c.HasErrors() {
		t.Fatalf("compile errors: %v", c.Errors)
	}
	name, err := encoding.UTF8ToSJS("ゆめみ")
	if err != nil {
		t.Fatal(err)
	}
	body, err := encoding.UTF8ToSJS("「ようこそ、プラネタリウムへ。」")
	if err != nil {
		t.Fatal(err)
	}
	want := append([]byte{0x81, 0x79}, name...)
	want = append(want, 0x81, 0x7A)
	want = append(want, body...)
	got := compilerOutputBytes(c)
	if !bytes.Equal(got, want) {
		t.Fatalf("speaker resource textout should be bare:\n got  % x\n want % x", got, want)
	}
	if bytes.Contains(got, []byte{'"'}) {
		t.Fatalf("speaker resource textout contained a quote byte: % x", got)
	}
}

func TestTextStubResourceRawByteEscapesSkipEmptyQuoteRun(t *testing.T) {
	c := newComp()
	c.Out.SuppressAutoKidoku = true
	c.Out.ResolveRes = func(key string) (string, bool) {
		if key == "0000" {
			return `\x{84}\x{02}`, true
		}
		return "", false
	}
	c.ParseElt(ast.ReturnStmt{
		Loc:  ast.Loc{File: "t", Line: 1},
		Expr: ast.ResRef{Loc: ast.Loc{File: "t", Line: 1}, Key: "0000"},
	})

	if c.HasErrors() {
		t.Fatalf("compile errors: %v", c.Errors)
	}
	got := compilerOutputBytes(c)
	if !bytes.Equal(got, []byte{0x84, 0x02}) {
		t.Fatalf("resource raw byte escapes: got % x, want 84 02", got)
	}
}

func TestCompileResTextSpeakerNameUsesWesternTransformByDefault(t *testing.T) {
	prev := texttransforms.GetMode()
	prevForce := texttransforms.ForceEncode
	texttransforms.SetMode(texttransforms.EncWestern)
	texttransforms.ForceEncode = true
	defer func() {
		texttransforms.SetMode(prev)
		texttransforms.ForceEncode = prevForce
	}()

	c := newComp()
	c.Out.ResolveRes = func(key string) (string, bool) {
		if key == "0000" {
			return `\{Père}Salut`, true
		}
		return "", false
	}
	c.ParseElt(ast.ReturnStmt{
		Loc:  ast.Loc{File: "t", Line: 1},
		Expr: ast.ResRef{Key: "0000"},
	})

	if c.HasErrors() {
		t.Fatalf("compile errors: %v", c.Errors)
	}
	got := compilerOutputBytes(c)
	want := []byte{'"', '"', 0x81, 0x79, '"', 'P', 0xc9, 'r', 'e', '"', 0x81, 0x7a, '"', 'S', 'a', 'l', 'u', 't', '"'}
	if !bytes.Contains(got, want) {
		t.Fatalf("speaker name should use Western transform by default: got % x, want contains % x", got, want)
	}
}

func TestCompileResTextNativeSpeakerTagsBypassWesternTransform(t *testing.T) {
	prev := texttransforms.GetMode()
	prevForce := texttransforms.ForceEncode
	texttransforms.SetMode(texttransforms.EncWestern)
	texttransforms.ForceEncode = true
	defer func() {
		texttransforms.SetMode(prev)
		texttransforms.ForceEncode = prevForce
	}()

	c := newComp()
	c.Out.NativeSpeakerTags = true
	c.Out.ResolveRes = func(key string) (string, bool) {
		if key == "0000" {
			return `\{声}*Sigh*`, true
		}
		return "", false
	}
	c.ParseElt(ast.ReturnStmt{
		Loc:  ast.Loc{File: "t", Line: 1},
		Expr: ast.ResRef{Key: "0000"},
	})

	if c.HasErrors() {
		t.Fatalf("compile errors: %v", c.Errors)
	}
	wantName, err := encoding.UTF8ToSJS(string(rune(0x58f0)))
	if err != nil {
		t.Fatal(err)
	}
	got := compilerOutputBytes(c)
	want := append([]byte{0x81, 0x79}, wantName...)
	want = append(want, 0x81, 0x7a, '"', '*', 'S', 'i', 'g', 'h', '*', '"')
	if !bytes.Contains(got, want) {
		t.Fatalf("native speaker text bytes: got % x, want contains % x", got, want)
	}
	if bytes.Contains(got, []byte{'"', '"', 0x81, 0x79}) {
		t.Fatalf("native speaker line started with an empty quote run: % x", got)
	}
}

func TestCompileResTextKeepsSpacesAfterParamlessControl(t *testing.T) {
	c := newComp()
	c.Reg.Register(&kfn.FuncDef{
		Ident:    "par",
		CCStr:    "r",
		OpType:   0,
		OpModule: 0,
		OpCode:   3,
		Prototypes: []kfn.Prototype{{
			Defined: true,
		}},
	})

	tc := &textCompiler{c: c, loc: ast.Loc{File: "t", Line: 1}}
	tc.compileResText(`before\r  after`)
	tc.flush()

	if c.HasErrors() {
		t.Fatalf("compile errors: %v", c.Errors)
	}
	got := compilerOutputBytes(c)
	if !bytes.Contains(got, []byte("before")) {
		t.Fatalf("text before control was not emitted: % x", got)
	}
	if !bytes.Contains(got, []byte("  after")) {
		t.Fatalf("spaces after paramless control were not preserved: % x", got)
	}
}

func TestCompileResTextAcceptsLegacyAttachedPause(t *testing.T) {
	c := newComp()
	c.Reg.Register(&kfn.FuncDef{
		Ident:    "spause",
		CCStr:    "p",
		OpType:   0,
		OpModule: 0,
		OpCode:   205,
		Prototypes: []kfn.Prototype{{
			Defined: true,
		}},
	})

	tc := &textCompiler{c: c, loc: ast.Loc{File: "t", Line: 1}}
	tc.compileResText(`avant\papres`)
	tc.flush()

	if c.HasErrors() {
		t.Fatalf("compile errors: %v", c.Errors)
	}
	got := compilerOutputBytes(c)
	if !bytes.Contains(got, []byte("avant")) || !bytes.Contains(got, []byte("apres")) {
		t.Fatalf("attached pause changed surrounding text: % x", got)
	}
	if !bytes.Contains(got, codegen.EncodeOpcode(0, 0, 205, 0, 0)) {
		t.Fatalf("pause opcode was not emitted: % x", got)
	}
	if bytes.Contains(got, []byte{'\\'}) {
		t.Fatalf("legacy attached pause left a literal backslash: % x", got)
	}
}

func TestTextoutSuppressesLineBeforeFollowingPause(t *testing.T) {
	c := newComp()
	registerPause(c)
	c.Parse([]ast.Stmt{
		ast.ReturnStmt{
			Loc: ast.Loc{File: "t", Line: 10},
			Expr: ast.StrLit{Loc: ast.Loc{File: "t", Line: 10}, Tokens: []ast.StrToken{
				ast.TextToken{Loc: ast.Loc{File: "t", Line: 10}, Text: "hello"},
			}},
		},
		ast.FuncCallStmt{Loc: ast.Loc{File: "t", Line: 11}, Ident: "pause"},
	})

	pauseIdx := findCodeIR(c, codegen.EncodeOpcode(0, 0, 17, 0, 0))
	if pauseIdx < 0 {
		t.Fatal("pause opcode was not emitted")
	}
	if pauseIdx > 0 && c.Out.IR[pauseIdx-1].Type == codegen.IRLineref && c.Out.IR[pauseIdx-1].Index == 11 {
		t.Fatalf("pause immediately after textout should not get its own line marker")
	}
}

func TestTextoutPauseLineSuppressionClearsOnInterveningStmt(t *testing.T) {
	c := newComp()
	registerPause(c)
	c.Reg.Register(&kfn.FuncDef{
		Ident:    "msgHide",
		OpType:   0,
		OpModule: 0,
		OpCode:   151,
		Prototypes: []kfn.Prototype{{
			Defined: true,
		}},
	})
	c.Parse([]ast.Stmt{
		ast.ReturnStmt{
			Loc: ast.Loc{File: "t", Line: 10},
			Expr: ast.StrLit{Loc: ast.Loc{File: "t", Line: 10}, Tokens: []ast.StrToken{
				ast.TextToken{Loc: ast.Loc{File: "t", Line: 10}, Text: "hello"},
			}},
		},
		ast.FuncCallStmt{Loc: ast.Loc{File: "t", Line: 11}, Ident: "msgHide"},
		ast.FuncCallStmt{Loc: ast.Loc{File: "t", Line: 12}, Ident: "pause"},
	})

	pauseIdx := findCodeIR(c, codegen.EncodeOpcode(0, 0, 17, 0, 0))
	if pauseIdx < 0 {
		t.Fatal("pause opcode was not emitted")
	}
	if pauseIdx == 0 || c.Out.IR[pauseIdx-1].Type != codegen.IRLineref || c.Out.IR[pauseIdx-1].Index != 12 {
		t.Fatalf("non-immediate pause should keep its line marker")
	}
}

func registerPause(c *Compiler) {
	c.Reg.Register(&kfn.FuncDef{
		Ident:    "pause",
		OpType:   0,
		OpModule: 0,
		OpCode:   17,
		Prototypes: []kfn.Prototype{{
			Defined: true,
		}},
	})
}

func registerItoa(c *Compiler) {
	c.Reg.Register(&kfn.FuncDef{
		Ident:    "itoa",
		OpType:   1,
		OpModule: 10,
		OpCode:   17,
		Prototypes: []kfn.Prototype{
			{
				Defined: true,
				Params: []kfn.Parameter{
					{Type: kfn.PIntC, Flags: []kfn.ParamFlag{kfn.FTagged}, Tag: "num"},
					{Type: kfn.PStr, Flags: []kfn.ParamFlag{kfn.FReturn, kfn.FTagged}, Tag: "buf"},
				},
			},
			{
				Defined: true,
				Params: []kfn.Parameter{
					{Type: kfn.PIntC, Flags: []kfn.ParamFlag{kfn.FTagged}, Tag: "num"},
					{Type: kfn.PStr, Flags: []kfn.ParamFlag{kfn.FReturn, kfn.FTagged}, Tag: "buf"},
					{Type: kfn.PIntC, Flags: []kfn.ParamFlag{kfn.FTagged}, Tag: "length"},
				},
			},
		},
	})
}

func registerStrsub(c *Compiler) {
	c.Reg.Register(&kfn.FuncDef{
		Ident:    "strsub",
		OpType:   1,
		OpModule: 10,
		OpCode:   5,
		Prototypes: []kfn.Prototype{
			{
				Defined: true,
				Params: []kfn.Parameter{
					{Type: kfn.PStr, Flags: []kfn.ParamFlag{kfn.FReturn, kfn.FTagged}, Tag: "dst"},
					{Type: kfn.PStrC, Flags: []kfn.ParamFlag{kfn.FTagged}, Tag: "src"},
					{Type: kfn.PIntC, Flags: []kfn.ParamFlag{kfn.FTagged}, Tag: "offset"},
				},
			},
			{
				Defined: true,
				Params: []kfn.Parameter{
					{Type: kfn.PStr, Flags: []kfn.ParamFlag{kfn.FReturn, kfn.FTagged}, Tag: "dst"},
					{Type: kfn.PStrC, Flags: []kfn.ParamFlag{kfn.FTagged}, Tag: "src"},
					{Type: kfn.PIntC, Flags: []kfn.ParamFlag{kfn.FTagged}, Tag: "offset"},
					{Type: kfn.PIntC, Flags: []kfn.ParamFlag{kfn.FTagged}, Tag: "len"},
				},
			},
		},
	})
}

func findCodeIR(c *Compiler, want []byte) int {
	for i, ir := range c.Out.IR {
		if ir.Type == codegen.IRCode && bytes.Equal(ir.Bytes, want) {
			return i
		}
	}
	return -1
}

func compilerOutputBytes(c *Compiler) []byte {
	var got []byte
	for _, ir := range c.Out.IR {
		got = append(got, ir.Bytes...)
	}
	return got
}

func kidokuIRs(c *Compiler) []codegen.IR {
	var got []codegen.IR
	for _, ir := range c.Out.IR {
		if ir.Type == codegen.IRKidoku {
			got = append(got, ir)
		}
	}
	return got
}

func registerFarcallWith(c *Compiler) {
	c.Reg.Register(&kfn.FuncDef{
		Ident:    "farcall_with",
		Flags:    []kfn.FuncFlag{kfn.FlagPushStore, kfn.FlagIsCall},
		OpType:   0,
		OpModule: 1,
		OpCode:   18,
		Prototypes: []kfn.Prototype{{
			Defined: true,
			Params: []kfn.Parameter{
				{Type: kfn.PIntC, Flags: []kfn.ParamFlag{kfn.FUncount, kfn.FTagged}, Tag: "scenario"},
				{Type: kfn.PIntC, Flags: []kfn.ParamFlag{kfn.FUncount, kfn.FTagged}, Tag: "entrypoint"},
				{
					Type:  kfn.PSpecial,
					Flags: []kfn.ParamFlag{kfn.FArgc},
					Specials: []kfn.SpecialDef{
						{ID: 0, Params: []kfn.Parameter{{Type: kfn.PIntC}}, Flags: []kfn.SpecialFlag{kfn.SFNoParens}},
						{ID: 1, Params: []kfn.Parameter{{Type: kfn.PStrC}}, Flags: []kfn.SpecialFlag{kfn.SFNoParens}},
					},
				},
			},
		}},
	})
}

func TestAngleSpecialParamEmitsInlineNoParens(t *testing.T) {
	c := newComp()
	registerFarcallWith(c)
	c.ParseElt(ast.FuncCallStmt{
		Loc:   ast.Loc{Line: 1},
		Ident: "farcall_with",
		Params: []ast.Param{
			ast.SimpleParam{Expr: ast.IntLit{Val: 9600}},
			ast.SimpleParam{Expr: ast.IntLit{Val: 0}},
			ast.SpecialParam{Tag: 0, Exprs: []ast.Expr{ast.IntLit{Val: 41400000}}, NoParens: true},
			ast.SpecialParam{Tag: 0, Exprs: []ast.Expr{ast.IntLit{Val: 0}}, NoParens: true},
		},
	})
	if c.HasErrors() {
		t.Fatalf("errors: %v", c.Errors)
	}
	got := compilerOutputBytes(c)
	want := []byte{
		0x23, 0x00, 0x01, 0x12, 0x00, 0x02, 0x00, 0x00,
		'(',
		0x24, 0xff, 0x80, 0x25, 0x00, 0x00,
		0x24, 0xff, 0x00, 0x00, 0x00, 0x00,
		0x61, 0x00, 0x24, 0xff, 0xc0, 0xb6, 0x77, 0x02,
		0x61, 0x00, 0x24, 0xff, 0x00, 0x00, 0x00, 0x00,
		')',
	}
	if !bytes.Contains(got, want) {
		t.Fatalf("farcall special params should be inline:\n got  % x\n want % x", got, want)
	}
	if bytes.Contains(got, []byte{0x61, 0x00, '('}) {
		t.Fatalf("inline special was emitted as parenthesised __special form: % x", got)
	}
}

func TestLegacyInlineSpecialParamCoercesSimpleArgs(t *testing.T) {
	c := newComp()
	registerFarcallWith(c)
	c.ParseElt(ast.FuncCallStmt{
		Loc:   ast.Loc{Line: 1},
		Ident: "farcall_with",
		Params: []ast.Param{
			ast.SimpleParam{Expr: ast.IntLit{Val: 9600}},
			ast.SimpleParam{Expr: ast.IntLit{Val: 0}},
			ast.SimpleParam{Expr: ast.IntLit{Val: 41400000}},
			ast.SimpleParam{Expr: ast.IntLit{Val: 0}},
		},
	})
	if c.HasErrors() {
		t.Fatalf("errors: %v", c.Errors)
	}
	got := compilerOutputBytes(c)
	want := []byte{
		0x23, 0x00, 0x01, 0x12, 0x00, 0x02, 0x00, 0x00,
		'(',
		0x24, 0xff, 0x80, 0x25, 0x00, 0x00,
		0x24, 0xff, 0x00, 0x00, 0x00, 0x00,
		0x61, 0x00, 0x24, 0xff, 0xc0, 0xb6, 0x77, 0x02,
		0x61, 0x00, 0x24, 0xff, 0x00, 0x00, 0x00, 0x00,
		')',
	}
	if !bytes.Contains(got, want) {
		t.Fatalf("legacy simple args should become inline specials:\n got  % x\n want % x", got, want)
	}
	if bytes.Contains(got, []byte{0x61, 0x00, '('}) {
		t.Fatalf("legacy inline special was emitted with parens: % x", got)
	}
}

func TestVariadicNestedSpecialParamsCountAndEmitInnerSpecial(t *testing.T) {
	c := newComp()
	c.Reg.Register(&kfn.FuncDef{
		Ident:  "TIMETABLE2",
		Flags:  []kfn.FuncFlag{kfn.FlagPushStore},
		OpType: 1, OpModule: 0, OpCode: 810,
		Prototypes: []kfn.Prototype{{
			Defined: true,
			Params: []kfn.Parameter{
				{Type: kfn.PIntC, Flags: []kfn.ParamFlag{kfn.FUncount}},
				{Type: kfn.PIntC, Flags: []kfn.ParamFlag{kfn.FUncount}},
				{Type: kfn.PIntC, Flags: []kfn.ParamFlag{kfn.FUncount}},
				{Type: kfn.PIntC, Flags: []kfn.ParamFlag{kfn.FUncount}},
				{Type: kfn.PSpecial, Flags: []kfn.ParamFlag{kfn.FArgc}},
			},
		}},
	})

	nested := func(x int32) ast.SpecialParam {
		return ast.SpecialParam{
			Tag:      48,
			NoParens: true,
			Exprs: []ast.Expr{ast.FuncCall{
				Ident: "__special",
				Params: []ast.Param{
					ast.SimpleParam{Expr: ast.IntLit{Val: 1}},
					ast.SimpleParam{Expr: ast.IntLit{Val: x}},
					ast.SimpleParam{Expr: ast.IntLit{Val: 433}},
					ast.SimpleParam{Expr: ast.IntLit{Val: 0}},
				},
			}},
		}
	}

	c.ParseElt(ast.FuncCallStmt{
		Loc:   ast.Loc{Line: 1},
		Ident: "TIMETABLE2",
		Dest:  ast.IntVar{Bank: 0, Index: ast.IntLit{Val: 3}},
		Params: []ast.Param{
			ast.SimpleParam{Expr: ast.IntVar{Bank: 5, Index: ast.IntLit{Val: 5}}},
			ast.SimpleParam{Expr: ast.IntLit{Val: 0}},
			ast.SimpleParam{Expr: ast.IntLit{Val: 0}},
			ast.SimpleParam{Expr: ast.IntLit{Val: 400}},
			nested(217),
			nested(434),
		},
	})
	if c.HasErrors() {
		t.Fatalf("errors: %v", c.Errors)
	}
	got := compilerOutputBytes(c)
	if !bytes.Contains(got, codegen.EncodeOpcode(1, 0, 810, 2, 0)) {
		t.Fatalf("TIMETABLE2 argc should count only repeated specials:\n got % x", got)
	}
	wantNested := []byte{0x61, 48, 0x61, 1, '('}
	if count := bytes.Count(got, wantNested); count != 2 {
		t.Fatalf("nested __special markers = %d, want 2:\n got % x", count, got)
	}
}

func TestBracketSpecialParamKeepsParens(t *testing.T) {
	c := newComp()
	c.Reg.Register(&kfn.FuncDef{
		Ident:    "grpMulti",
		OpType:   1,
		OpModule: 70,
		OpCode:   75,
		Prototypes: []kfn.Prototype{{
			Defined: true,
			Params: []kfn.Parameter{
				{Type: kfn.PStrC},
				{Type: kfn.PIntC},
				{Type: kfn.PSpecial, Flags: []kfn.ParamFlag{kfn.FArgc}},
			},
		}},
	})
	c.ParseElt(ast.FuncCallStmt{
		Loc:   ast.Loc{Line: 1},
		Ident: "grpMulti",
		Params: []ast.Param{
			ast.SimpleParam{Expr: ast.StrLit{Tokens: []ast.StrToken{ast.TextToken{Text: "KURO"}}}},
			ast.SimpleParam{Expr: ast.IntLit{Val: 4}},
			ast.SpecialParam{Tag: 4, Exprs: []ast.Expr{
				ast.StrLit{Tokens: []ast.StrToken{ast.TextToken{Text: "CGKY12"}}},
				ast.IntLit{Val: 0},
				ast.IntLit{Val: 0},
			}},
		},
	})
	if c.HasErrors() {
		t.Fatalf("errors: %v", c.Errors)
	}
	got := compilerOutputBytes(c)
	if !bytes.Contains(got, []byte{0x61, 0x04, '('}) {
		t.Fatalf("__special form lost its parameter list parens: % x", got)
	}
}

func TestBracketSpecialParamSeparatesNegativeArg(t *testing.T) {
	c := newComp()
	c.Reg.Register(&kfn.FuncDef{
		Ident:    "index_series",
		OpType:   1,
		OpModule: 0,
		OpCode:   800,
		Prototypes: []kfn.Prototype{{
			Defined: true,
			Params: []kfn.Parameter{
				{Type: kfn.PIntC},
				{Type: kfn.PIntC},
				{Type: kfn.PIntC},
				{Type: kfn.PSpecial, Flags: []kfn.ParamFlag{kfn.FArgc}},
			},
		}},
	})
	c.ParseElt(ast.FuncCallStmt{
		Loc:   ast.Loc{Line: 1},
		Ident: "index_series",
		Params: []ast.Param{
			ast.SimpleParam{Expr: ast.IntVar{Bank: 2, Index: ast.IntLit{Val: 1}}},
			ast.SimpleParam{Expr: ast.IntLit{Val: 0}},
			ast.SimpleParam{Expr: ast.IntLit{Val: 0}},
			ast.SpecialParam{Tag: 2, Exprs: []ast.Expr{
				ast.IntLit{Val: 0},
				ast.IntLit{Val: 10000},
				ast.UnaryExpr{Op: ast.UnarySub, Val: ast.IntLit{Val: 800}},
				ast.IntLit{Val: 0},
			}},
		},
	})
	if c.HasErrors() {
		t.Fatalf("errors: %v", c.Errors)
	}
	got := compilerOutputBytes(c)
	want := append([]byte{0x24, 0xff, 0x10, 0x27, 0x00, 0x00}, codegen.EncodeInt32(-800)...)
	if !bytes.Contains(got, want) {
		t.Fatalf("negative special arg should be direct int literal:\n got  % x\n want % x", got, want)
	}
}

func TestLegacyBraceSpecialParamCoercesByKFNArity(t *testing.T) {
	c := newComp()
	c.Reg.Register(&kfn.FuncDef{
		Ident:    "index_series",
		Flags:    []kfn.FuncFlag{kfn.FlagPushStore},
		OpType:   1,
		OpModule: 0,
		OpCode:   800,
		Prototypes: []kfn.Prototype{{
			Defined: true,
			Params: []kfn.Parameter{
				{Type: kfn.PIntC},
				{Type: kfn.PIntC},
				{Type: kfn.PIntC},
				{
					Type:  kfn.PSpecial,
					Flags: []kfn.ParamFlag{kfn.FArgc},
					Specials: []kfn.SpecialDef{
						{ID: 0, Params: []kfn.Parameter{{Type: kfn.PIntC}}},
						{ID: 1, Params: []kfn.Parameter{{Type: kfn.PIntC}, {Type: kfn.PIntC}, {Type: kfn.PIntC}}},
						{ID: 2, Params: []kfn.Parameter{{Type: kfn.PIntC}, {Type: kfn.PIntC}, {Type: kfn.PIntC}, {Type: kfn.PIntC}}},
					},
				},
			},
		}},
	})
	c.ParseElt(ast.FuncCallStmt{
		Loc:   ast.Loc{Line: 1},
		Ident: "index_series",
		Dest:  ast.IntVar{Bank: 2, Index: ast.IntLit{Val: 0}},
		Params: []ast.Param{
			ast.SimpleParam{Expr: ast.IntVar{Bank: 2, Index: ast.IntLit{Val: 1}}},
			ast.SimpleParam{Expr: ast.IntLit{Val: 0}},
			ast.SimpleParam{Expr: ast.IntLit{Val: 255}},
			ast.ComplexParam{Exprs: []ast.Expr{
				ast.IntLit{Val: 0},
				ast.IntLit{Val: 10000},
				ast.IntLit{Val: 0},
				ast.IntLit{Val: 0},
			}},
		},
	})
	if c.HasErrors() {
		t.Fatalf("errors: %v", c.Errors)
	}
	got := compilerOutputBytes(c)
	if !bytes.Contains(got, []byte{0x61, 0x02, '('}) {
		t.Fatalf("legacy brace special should emit tag 2, not a tuple:\n got % x", got)
	}
}

func TestLegacyBraceSpecialParamChoosesTypedKFNCase(t *testing.T) {
	c := newComp()
	c.Reg.Register(&kfn.FuncDef{
		Ident:    "GetSaveFlag",
		Flags:    []kfn.FuncFlag{kfn.FlagPushStore},
		OpType:   1,
		OpModule: 0,
		OpCode:   1414,
		Prototypes: []kfn.Prototype{{
			Defined: true,
			Params: []kfn.Parameter{
				{Type: kfn.PIntC},
				{
					Type:  kfn.PSpecial,
					Flags: []kfn.ParamFlag{kfn.FArgc},
					Specials: []kfn.SpecialDef{
						{ID: 0, Params: []kfn.Parameter{{Type: kfn.PInt}, {Type: kfn.PInt}, {Type: kfn.PIntC}}},
						{ID: 1, Params: []kfn.Parameter{{Type: kfn.PStr}, {Type: kfn.PStr}, {Type: kfn.PIntC}}},
					},
				},
			},
		}},
	})
	c.ParseElt(ast.FuncCallStmt{
		Loc:   ast.Loc{Line: 1},
		Ident: "GetSaveFlag",
		Params: []ast.Param{
			ast.SimpleParam{Expr: ast.IntLit{Val: 30}},
			ast.ComplexParam{Exprs: []ast.Expr{
				ast.IntVar{Bank: 5, Index: ast.IntLit{Val: 0}},
				ast.IntVar{Bank: 2, Index: ast.IntLit{Val: 10}},
				ast.IntLit{Val: 1},
			}},
		},
	})
	if c.HasErrors() {
		t.Fatalf("errors: %v", c.Errors)
	}
	got := compilerOutputBytes(c)
	if !bytes.Contains(got, []byte{0x61, 0x00, '('}) {
		t.Fatalf("legacy typed brace special should emit int tag 0:\n got % x", got)
	}
}

func TestTextStubNilExpr(t *testing.T) {
	c := newComp()
	c.ParseElt(ast.ReturnStmt{Loc: ast.Loc{Line: 1}})
	// Should do nothing
	if c.HasErrors() {
		t.Errorf("errors: %v", c.Errors)
	}
}

func TestTextStubDynLinNotDefined(t *testing.T) {
	c := newComp()
	// Default: __DynamicLineation__ not defined → stub mode
	c.ParseElt(ast.ReturnStmt{
		Loc: ast.Loc{Line: 1},
		Expr: ast.StrLit{Tokens: []ast.StrToken{
			ast.TextToken{Text: "test"},
		}},
	})
	if c.HasErrors() {
		t.Errorf("errors: %v", c.Errors)
	}
}

func TestTextStubDynLinError(t *testing.T) {
	c := newComp()
	// Define __DynamicLineation__ = 1 but no library → error
	c.Mem.Define("__DynamicLineation__", memory.Symbol{Kind: memory.KindInteger, IntVal: 1})
	c.ParseElt(ast.ReturnStmt{
		Loc:  ast.Loc{Line: 1},
		Expr: ast.StrLit{Tokens: []ast.StrToken{ast.TextToken{Text: "x"}}},
	})
	if !c.HasErrors() {
		t.Error("should error when dynlin=1 but no library")
	}
}

func TestLoadRLBabelModuleDefinesFlags(t *testing.T) {
	c := newComp()
	c.Parse([]ast.Stmt{ast.LoadFileStmt{Loc: ast.Loc{File: "t", Line: 1}, Path: ast.StrLit{
		Loc: ast.Loc{File: "t", Line: 1},
		Tokens: []ast.StrToken{
			ast.TextToken{Loc: ast.Loc{File: "t", Line: 1}, Text: "rlBabel"},
		},
	}}})
	if c.HasErrors() {
		t.Fatalf("errors: %v", c.Errors)
	}
	for _, name := range []string{"__RLBABEL_KH__", "__DynamicLineation__", "rlBabelDLL"} {
		if !c.Mem.Defined(name) {
			t.Fatalf("%s was not defined", name)
		}
	}
}

func TestRLBabelTextEmitsCallDLLRuntime(t *testing.T) {
	c := newComp()
	registerRLBabelRuntimeKFN(c)
	c.Compile([]ast.Stmt{
		ast.LoadFileStmt{Loc: ast.Loc{File: "t", Line: 1}, Path: ast.StrLit{
			Loc: ast.Loc{File: "t", Line: 1},
			Tokens: []ast.StrToken{
				ast.TextToken{Loc: ast.Loc{File: "t", Line: 1}, Text: "rlBabel"},
			},
		}},
		ast.ReturnStmt{Loc: ast.Loc{File: "t", Line: 2}, Expr: ast.StrLit{
			Loc: ast.Loc{File: "t", Line: 2},
			Tokens: []ast.StrToken{
				ast.SpeakerToken{Loc: ast.Loc{File: "t", Line: 2}},
				ast.TextToken{Loc: ast.Loc{File: "t", Line: 2}, Text: "Name"},
				ast.RCurToken{Loc: ast.Loc{File: "t", Line: 2}},
				ast.SpaceToken{Loc: ast.Loc{File: "t", Line: 2}, Count: 1},
				ast.TextToken{Loc: ast.Loc{File: "t", Line: 2}, Text: "Hello"},
				ast.CodeToken{Loc: ast.Loc{File: "t", Line: 2}, Ident: "n"},
				ast.TextToken{Loc: ast.Loc{File: "t", Line: 2}, Text: "World"},
			},
		}},
	})
	if c.HasErrors() {
		t.Fatalf("errors: %v", c.Errors)
	}
	got := compilerOutputBytes(c)
	if !bytes.Contains(got, []byte{'#', 2, 0, 12, 0}) {
		t.Fatalf("CallDLL opcode was not emitted: % x", got)
	}
	if !hasLabel(c, rlBabelDisplayLabel) {
		t.Fatalf("runtime label %s was not emitted", rlBabelDisplayLabel)
	}
	if !c.rlBabelRuntimeDone {
		t.Fatal("Babel runtime was not marked emitted")
	}
}

func TestRLBabelResourceNameMarkerLetters(t *testing.T) {
	oldMode := texttransforms.GetMode()
	oldForce := texttransforms.ForceEncode
	texttransforms.SetMode(texttransforms.EncWestern)
	texttransforms.ForceEncode = true
	defer func() {
		texttransforms.SetMode(oldMode)
		texttransforms.ForceEncode = oldForce
	}()

	c := newComp()
	registerRLBabelRuntimeKFN(c)
	c.Out.ResolveRes = func(key string) (string, bool) {
		if key == "0000" {
			return `\{\m{B}}Hello`, true
		}
		return "", false
	}
	c.Compile([]ast.Stmt{
		ast.LoadFileStmt{Loc: ast.Loc{File: "t", Line: 1}, Path: ast.StrLit{
			Tokens: []ast.StrToken{ast.TextToken{Text: "rlBabel"}},
		}},
		ast.ReturnStmt{Loc: ast.Loc{File: "t", Line: 2}, Expr: ast.ResRef{Loc: ast.Loc{File: "t", Line: 2}, Key: "0000"}},
	})
	if c.HasErrors() {
		t.Fatalf("errors: %v", c.Errors)
	}
	got := compilerOutputBytes(c)
	want := []byte{0x01, 0x81, 0x96, 0x82, 0x61, 0x02}
	if !bytes.Contains(got, want) {
		t.Fatalf("Babel resource name marker should preserve \\m{B}: got % x, want contains % x", got, want)
	}
	if bytes.Contains(got, []byte(`\m{B}`)) {
		t.Fatalf("Babel resource name marker leaked as plain text: % x", got)
	}
}

func TestRLBabelRuntimeEmitsBeforeEOF(t *testing.T) {
	c := newComp()
	registerRLBabelRuntimeKFN(c)
	c.EmitEOFMarkers = true
	c.Compile([]ast.Stmt{
		ast.LoadFileStmt{Loc: ast.Loc{File: "t", Line: 1}, Path: ast.StrLit{
			Tokens: []ast.StrToken{ast.TextToken{Text: "rlBabel"}},
		}},
		ast.ReturnStmt{Loc: ast.Loc{File: "t", Line: 2}, Expr: ast.StrLit{
			Tokens: []ast.StrToken{ast.TextToken{Text: "Hello"}},
		}},
		ast.EOFStmt{Loc: ast.Loc{File: "t", Line: 3}},
	})
	if c.HasErrors() {
		t.Fatalf("errors: %v", c.Errors)
	}
	runtimeIdx := labelIRIndex(c, rlBabelDisplayLabel)
	trailerIdx := codeIRIndex(c, SeenEndTrailerBytes())
	if runtimeIdx < 0 || trailerIdx < 0 {
		t.Fatalf("runtimeIdx=%d trailerIdx=%d", runtimeIdx, trailerIdx)
	}
	if runtimeIdx > trailerIdx {
		t.Fatalf("runtime label should be emitted before SeenEnd trailer")
	}
}

func TestRLBabelCompilesWithRealKFN(t *testing.T) {
	kfnPath := filepath.Join("..", "..", "..", "KFN", "reallive.kfn")
	reg, err := kfn.ParseFile(kfnPath)
	if err != nil {
		t.Fatal(err)
	}
	c := New(reg, ini.NewTable())
	c.Compile([]ast.Stmt{
		ast.LoadFileStmt{Loc: ast.Loc{File: "t", Line: 1}, Path: ast.StrLit{
			Tokens: []ast.StrToken{ast.TextToken{Text: "rlBabel"}},
		}},
		ast.ReturnStmt{Loc: ast.Loc{File: "t", Line: 2}, Expr: ast.StrLit{
			Tokens: []ast.StrToken{ast.TextToken{Text: "Hello"}},
		}},
	})
	if c.HasErrors() {
		t.Fatalf("errors: %v", c.Errors)
	}
	if !hasLabel(c, rlBabelDisplayLabel) {
		t.Fatalf("runtime label %s was not emitted", rlBabelDisplayLabel)
	}
}

func hasLabel(c *Compiler, label string) bool {
	return labelIRIndex(c, label) >= 0
}

func labelIRIndex(c *Compiler, label string) int {
	for i, ir := range c.Out.IR {
		if ir.Type == codegen.IRLabel && ir.Label == label {
			return i
		}
	}
	return -1
}

func codeIRIndex(c *Compiler, want []byte) int {
	for i, ir := range c.Out.IR {
		if ir.Type == codegen.IRCode && bytes.Equal(ir.Bytes, want) {
			return i
		}
	}
	return -1
}

func registerRLBabelRuntimeKFN(c *Compiler) {
	reg := c.Reg
	reg.Register(&kfn.FuncDef{Ident: "CallDLL", Flags: []kfn.FuncFlag{kfn.FlagPushStore}, OpType: 2, OpModule: 0, OpCode: 12})
	reg.Register(&kfn.FuncDef{Ident: "LoadDLL", Flags: []kfn.FuncFlag{kfn.FlagPushStore}, OpType: 2, OpModule: 0, OpCode: 10})
	reg.Register(&kfn.FuncDef{Ident: "strout", OpType: 1, OpModule: 10, OpCode: 100})
	reg.Register(&kfn.FuncDef{Ident: "strcpy", OpType: 1, OpModule: 10, OpCode: 0})
	reg.Register(&kfn.FuncDef{Ident: "itoa", OpType: 1, OpModule: 10, OpCode: 17})
	reg.Register(&kfn.FuncDef{Ident: "FontSize", OpType: 0, OpModule: 0, OpCode: 101})
	reg.Register(&kfn.FuncDef{Ident: "DisableAutoSavepoints", OpType: 1, OpModule: 0, OpCode: 3502})
	reg.Register(&kfn.FuncDef{Ident: "EnableAutoSavepoints", OpType: 1, OpModule: 0, OpCode: 3501})
	reg.Register(&kfn.FuncDef{Ident: "TextPos", OpType: 0, OpModule: 0, OpCode: 310})
	reg.Register(&kfn.FuncDef{Ident: "TextPosX", OpType: 0, OpModule: 0, OpCode: 311})
	reg.Register(&kfn.FuncDef{Ident: "SetIndent", OpType: 0, OpModule: 0, OpCode: 300})
	reg.Register(&kfn.FuncDef{Ident: "ClearIndent", OpType: 0, OpModule: 0, OpCode: 301})
	reg.Register(&kfn.FuncDef{Ident: "br", OpType: 0, OpModule: 0, OpCode: 201})
	reg.Register(&kfn.FuncDef{Ident: "page", OpType: 0, OpModule: 0, OpCode: 210})
	reg.Register(&kfn.FuncDef{Ident: "goto", OpType: 0, OpModule: 0, OpCode: 0})
	reg.Register(&kfn.FuncDef{Ident: "gosub", OpType: 0, OpModule: 0, OpCode: 5})
	reg.Register(&kfn.FuncDef{Ident: "goto_case", OpType: 0, OpModule: 0, OpCode: 4})
	reg.Register(&kfn.FuncDef{Ident: "ret", OpType: 0, OpModule: 0, OpCode: 10})
}
