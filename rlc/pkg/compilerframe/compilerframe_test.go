package compilerframe

import (
	"bytes"
	"testing"

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

func TestParseDirective(t *testing.T) {
	c := newComp()
	c.Parse([]ast.Stmt{ast.DefineStmt{Ident: "X", Value: ast.IntLit{Val: 42}}})
	if !c.Mem.Defined("X") {
		t.Error("X not defined")
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

func TestLegacyRealLiveItoaLengthUsesOverloadZero(t *testing.T) {
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
	if findCodeIR(c, codegen.EncodeOpcode(1, 10, 17, 3, 0)) < 0 {
		t.Fatal("RealLive 1.2.3 itoa length form should use legacy overload 0")
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

func TestStrsubThreeArgUsesObservedOverload(t *testing.T) {
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
	if findCodeIR(c, codegen.EncodeOpcode(1, 10, 5, 3, 1)) < 0 {
		t.Fatal("three-argument strsub should use overload 1")
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
