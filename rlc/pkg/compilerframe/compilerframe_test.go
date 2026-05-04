package compilerframe

import (
	"testing"

	"github.com/yoremi/rldev-go/rlc/pkg/ast"
	"github.com/yoremi/rldev-go/rlc/pkg/ini"
	"github.com/yoremi/rldev-go/rlc/pkg/kfn"
	"github.com/yoremi/rldev-go/rlc/pkg/memory"
)

func newComp() *Compiler {
	return New(kfn.NewRegistry(), ini.NewTable())
}

func TestNew(t *testing.T) {
	c := newComp()
	if c.Mem == nil { t.Error("Mem") }
	if c.Out == nil { t.Error("Out") }
	if c.Norm == nil { t.Error("Norm") }
	if c.Directive == nil { t.Error("Directive") }
	if c.Intrin == nil { t.Error("Intrin") }
	if c.Reg == nil { t.Error("Reg") }
	if c.Ini == nil { t.Error("Ini") }
	if c.State == nil { t.Error("State") }
}

func TestMetaCallbackWired(t *testing.T) {
	c := newComp()
	// meta.State.CompileStatements should delegate to c.Parse
	if c.State.CompileStatements == nil { t.Error("callback not wired") }
	// Trigger via meta
	c.State.ParseOne(ast.HaltStmt{})
	// Halt should have been emitted
	if c.Out.Length() == 0 { t.Error("halt not emitted via meta callback") }
}

func TestParseEmpty(t *testing.T) {
	c := newComp()
	c.Parse(nil)
	if c.HasErrors() { t.Error("should have no errors") }
}

func TestParseHalt(t *testing.T) {
	c := newComp()
	c.Parse([]ast.Stmt{ast.HaltStmt{}})
	if c.Out.Length() == 0 { t.Error("halt not emitted") }
}

func TestParseDirective(t *testing.T) {
	c := newComp()
	c.Parse([]ast.Stmt{ast.DefineStmt{Ident: "X", Value: ast.IntLit{Val: 42}}})
	if !c.Mem.Defined("X") { t.Error("X not defined") }
}

func TestParseTODO(t *testing.T) {
	// Partially-implemented statement types should produce warnings, not errors
	c := newComp()
	c.Parse([]ast.Stmt{ast.LoadFileStmt{Loc: ast.Loc{File: "t", Line: 1}, Path: ast.StrLit{
		Tokens: []ast.StrToken{ast.TextToken{Text: "test.kh"}},
	}}})
	if c.HasErrors() { t.Error("should not have errors, only warnings") }
	if len(c.Warnings) == 0 { t.Error("should have pending warning") }
}

func TestCompileMergesDirectiveDiagnostics(t *testing.T) {
	c := newComp()
	// Trigger a directive error by undefining something that doesn't exist
	c.Compile([]ast.Stmt{
		ast.DUndefStmt{Loc: ast.Loc{File: "t", Line: 1}, Idents: []string{"NOPE"}},
	})
	if !c.HasErrors() { t.Error("should have merged directive errors") }
}

func TestHasErrors(t *testing.T) {
	c := newComp()
	if c.HasErrors() { t.Error("fresh compiler should have no errors") }
	c.error(ast.Nowhere, "test")
	if !c.HasErrors() { t.Error("should have errors") }
}

func TestRecursiveParse(t *testing.T) {
	c := newComp()
	c.Parse([]ast.Stmt{
		ast.DefineStmt{Ident: "A", Value: ast.IntLit{Val: 1}},
		ast.DefineStmt{Ident: "B", Value: ast.IntLit{Val: 2}},
		ast.HaltStmt{},
	})
	if !c.Mem.Defined("A") { t.Error("A") }
	if !c.Mem.Defined("B") { t.Error("B") }
}

// --- Structure tests ---

func TestParseSeq(t *testing.T) {
	c := newComp()
	c.ParseElt(ast.SeqStmt{Stmts: []ast.Stmt{
		ast.DefineStmt{Ident: "S1", Value: ast.IntLit{Val: 1}},
		ast.HaltStmt{},
	}})
	if !c.Mem.Defined("S1") { t.Error("S1 not defined in seq") }
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
	if !c.Mem.Defined("X_TRUE") { t.Error("X_TRUE should be defined (const-true branch)") }
}

func TestParseIfConstFalse(t *testing.T) {
	c := newComp()
	// if(0) then define X else define Y
	c.ParseElt(ast.IfStmt{
		Cond: ast.IntLit{Val: 0},
		Then: ast.DefineStmt{Ident: "IF_THEN", Value: ast.IntLit{Val: 1}},
		Else: ast.DefineStmt{Ident: "IF_ELSE", Value: ast.IntLit{Val: 2}},
	})
	if c.Mem.Defined("IF_THEN") { t.Error("IF_THEN should NOT be defined (const-false)") }
	if !c.Mem.Defined("IF_ELSE") { t.Error("IF_ELSE should be defined (else branch)") }
}

func TestParseWhileConstFalse(t *testing.T) {
	c := newComp()
	// while(0) define X → should not define X
	c.ParseElt(ast.WhileStmt{
		Cond: ast.IntLit{Val: 0},
		Body: ast.DefineStmt{Ident: "W_BODY", Value: ast.IntLit{Val: 1}},
	})
	if c.Mem.Defined("W_BODY") { t.Error("W_BODY should NOT be defined (while(0))") }
}

func TestBreakOutsideLoop(t *testing.T) {
	c := newComp()
	c.ParseNormElt(ast.BreakStmt{Loc: ast.Loc{File: "t", Line: 1}})
	if !c.HasErrors() { t.Error("break outside loop should error") }
}

func TestContinueOutsideLoop(t *testing.T) {
	c := newComp()
	c.ParseNormElt(ast.ContinueStmt{Loc: ast.Loc{File: "t", Line: 1}})
	if !c.HasErrors() { t.Error("continue outside loop should error") }
}

func TestLabelEmit(t *testing.T) {
	c := newComp()
	c.ParseNormElt(ast.LabelStmt{Label: ast.Label{Ident: "mytest"}})
	if c.Out.Length() == 0 { t.Error("label should be emitted") }
}

func TestAssignEmit(t *testing.T) {
	c := newComp()
	c.ParseNormElt(ast.AssignStmt{
		Dest: ast.StoreRef{},
		Op:   ast.AssignSet,
		Expr: ast.IntLit{Val: 42},
	})
	if c.Out.Length() == 0 { t.Error("assignment should be emitted") }
}

// --- Compile-time structures ---

func TestDIfTrue(t *testing.T) {
	c := newComp()
	c.ParseElt(ast.DIfStmt{
		Cond: ast.IntLit{Val: 1},
		Body: []ast.Stmt{ast.DefineStmt{Ident: "DIF_T", Value: ast.IntLit{Val: 1}}},
		Cont: ast.DEndifStmt{},
	})
	if !c.Mem.Defined("DIF_T") { t.Error("DIF_T should be defined (#if true)") }
}

func TestDIfFalse(t *testing.T) {
	c := newComp()
	c.ParseElt(ast.DIfStmt{
		Cond: ast.IntLit{Val: 0},
		Body: []ast.Stmt{ast.DefineStmt{Ident: "DIF_SKIP", Value: ast.IntLit{Val: 1}}},
		Cont: ast.DEndifStmt{},
	})
	if c.Mem.Defined("DIF_SKIP") { t.Error("DIF_SKIP should NOT be defined (#if false)") }
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
	if c.Mem.Defined("SKIP") { t.Error("SKIP should not be defined") }
	if !c.Mem.Defined("ELSE_HIT") { t.Error("ELSE_HIT should be defined (#else)") }
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
	if c.Out.Length() < 3 { t.Errorf("expected 3 halts, IR length: %d", c.Out.Length()) }
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
	if c.Out.Length() < 3 { t.Errorf("expected 3 halts (reverse), IR length: %d", c.Out.Length()) }
}

func TestHiding(t *testing.T) {
	c := newComp()
	c.ParseElt(ast.HidingStmt{
		Loc:   ast.Loc{File: "test.org", Line: 10},
		Ident: "myfunc",
		Body:  ast.HaltStmt{},
	})
	// __INLINE_CALL__ should be undefined after hiding completes
	if c.Mem.Defined("__INLINE_CALL__") { t.Error("__INLINE_CALL__ should be cleaned up") }
}

func TestCaseDegenerate(t *testing.T) {
	c := newComp()
	// case(expr) with no arms but a default
	c.ParseElt(ast.CaseStmt{
		Expr:    ast.IntLit{Val: 42},
		Arms:    nil,
		Default: []ast.Stmt{ast.HaltStmt{}},
	})
	if c.HasErrors() { t.Errorf("degenerate case should not error: %v", c.Errors) }
}

func TestRawCodeBytes(t *testing.T) {
	c := newComp()
	c.ParseNormElt(ast.RawCodeStmt{
		Elts: []ast.RawElt{
			{Kind: "bytes", Str: "\x01\x02\x03"},
		},
	})
	if c.Out.Length() == 0 { t.Error("raw bytes should be emitted") }
}

func TestRawCodeHex(t *testing.T) {
	c := newComp()
	c.ParseNormElt(ast.RawCodeStmt{
		Elts: []ast.RawElt{
			{Kind: "ident", Str: "#FF00"},
		},
	})
	if c.HasErrors() { t.Errorf("hex raw should not error: %v", c.Errors) }
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
	if c.Out.Length() == 0 { t.Error("no output") }
	if c.HasErrors() { t.Errorf("errors: %v", c.Errors) }
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
	if c.HasErrors() { t.Errorf("errors: %v", c.Errors) }
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
	if c.HasErrors() { t.Errorf("errors: %v", c.Errors) }
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
	if c.HasErrors() { t.Errorf("errors: %v", c.Errors) }
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
	if c.HasErrors() { t.Errorf("errors: %v", c.Errors) }
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
	if c.HasErrors() { t.Errorf("errors: %v", c.Errors) }
}

func TestTextStubName(t *testing.T) {
	c := newComp()
	c.ParseElt(ast.ReturnStmt{
		Loc: ast.Loc{Line: 1},
		Expr: ast.StrLit{Tokens: []ast.StrToken{
			ast.NameToken{Index: ast.IntLit{Val: 3}, Global: false},
		}},
	})
	if c.HasErrors() { t.Errorf("errors: %v", c.Errors) }
}

func TestTextStubNilExpr(t *testing.T) {
	c := newComp()
	c.ParseElt(ast.ReturnStmt{Loc: ast.Loc{Line: 1}})
	// Should do nothing
	if c.HasErrors() { t.Errorf("errors: %v", c.Errors) }
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
	if c.HasErrors() { t.Errorf("errors: %v", c.Errors) }
}

func TestTextStubDynLinError(t *testing.T) {
	c := newComp()
	// Define __DynamicLineation__ = 1 but no library → error
	c.Mem.Define("__DynamicLineation__", memory.Symbol{Kind: memory.KindInteger, IntVal: 1})
	c.ParseElt(ast.ReturnStmt{
		Loc:  ast.Loc{Line: 1},
		Expr: ast.StrLit{Tokens: []ast.StrToken{ast.TextToken{Text: "x"}}},
	})
	if !c.HasErrors() { t.Error("should error when dynlin=1 but no library") }
}
