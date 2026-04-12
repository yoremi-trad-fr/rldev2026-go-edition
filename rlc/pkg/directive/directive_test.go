package directive

import (
	"testing"

	"github.com/yoremi/rldev-go/rlc/pkg/ast"
	"github.com/yoremi/rldev-go/rlc/pkg/codegen"
	"github.com/yoremi/rldev-go/rlc/pkg/expr"
	"github.com/yoremi/rldev-go/rlc/pkg/ini"
	"github.com/yoremi/rldev-go/rlc/pkg/kfn"
	"github.com/yoremi/rldev-go/rlc/pkg/memory"
	"github.com/yoremi/rldev-go/rlc/pkg/meta"
)

func newCompiler() *Compiler {
	mem := memory.New()
	return &Compiler{
		Mem:     mem,
		Norm:    expr.NewNormalizer(mem),
		Output:  codegen.NewOutput(),
		Ini:     ini.NewTable(),
		State:   meta.NewState(),
		Target:  new(kfn.Target),
		Version: &kfn.Version{1, 2, 7, 0},
	}
}

// ============================================================
// #define
// ============================================================

func TestDefine(t *testing.T) {
	c := newCompiler()
	c.Compile(ast.DefineStmt{Ident: "MAX", Value: ast.IntLit{Val: 100}})
	if !c.Mem.Defined("MAX") { t.Error("MAX not defined") }
	sym, _ := c.Mem.Get("MAX")
	if sym.Kind != memory.KindMacro { t.Error("should be macro") }
}

// ============================================================
// #const
// ============================================================

func TestConstInt(t *testing.T) {
	c := newCompiler()
	c.Compile(ast.DConstStmt{Ident: "PI", Kind: ast.KindConst, Value: ast.IntLit{Val: 3}})
	sym, ok := c.Mem.Get("PI")
	if !ok { t.Fatal("PI not defined") }
	if sym.Kind != memory.KindInteger || sym.IntVal != 3 {
		t.Errorf("PI: kind=%d val=%d", sym.Kind, sym.IntVal)
	}
}

func TestConstExpr(t *testing.T) {
	c := newCompiler()
	// #const X = 10 + 20 → should fold to 30
	c.Compile(ast.DConstStmt{Ident: "X", Kind: ast.KindConst,
		Value: ast.BinOp{LHS: ast.IntLit{Val: 10}, Op: ast.OpAdd, RHS: ast.IntLit{Val: 20}}})
	sym, _ := c.Mem.Get("X")
	if sym.IntVal != 30 { t.Errorf("X: %d, want 30", sym.IntVal) }
}

// ============================================================
// #inline
// ============================================================

func TestInline(t *testing.T) {
	c := newCompiler()
	c.Compile(ast.DInlineStmt{
		Ident:  "myfunc",
		Params: []ast.InlineParam{{Ident: "x"}},
		Body:   ast.HaltStmt{},
	})
	sym, ok := c.Mem.Get("myfunc")
	if !ok { t.Fatal("myfunc not defined") }
	if sym.Kind != memory.KindInline { t.Error("should be inline") }
}

// ============================================================
// #undef
// ============================================================

func TestUndef(t *testing.T) {
	c := newCompiler()
	c.Mem.Define("X", memory.Symbol{Kind: memory.KindInteger, IntVal: 1})
	c.Compile(ast.DUndefStmt{Idents: []string{"X"}})
	if c.Mem.Defined("X") { t.Error("X should be undefined") }
}

func TestUndefNotFound(t *testing.T) {
	c := newCompiler()
	c.Compile(ast.DUndefStmt{Loc: ast.Loc{File: "test", Line: 1}, Idents: []string{"NOPE"}})
	if len(c.Errors) == 0 { t.Error("expected error for undefined") }
}

// ============================================================
// #set
// ============================================================

func TestSet(t *testing.T) {
	c := newCompiler()
	c.Mem.Define("X", memory.Symbol{Kind: memory.KindInteger, IntVal: 1})
	c.Compile(ast.DSetStmt{Ident: "X", Value: ast.IntLit{Val: 42}, ReadOnly: true})
	sym, _ := c.Mem.Get("X")
	if sym.IntVal != 42 { t.Errorf("X after set: %d", sym.IntVal) }
}

func TestSetUndefined(t *testing.T) {
	c := newCompiler()
	c.Compile(ast.DSetStmt{Loc: ast.Loc{File: "test", Line: 1}, Ident: "NOPE", Value: ast.IntLit{Val: 1}})
	if len(c.Errors) == 0 { t.Error("expected error") }
}

// ============================================================
// #target
// ============================================================

func TestTarget(t *testing.T) {
	c := newCompiler()
	c.Compile(ast.DTargetStmt{Target: "AVG2000"})
	if *c.Target != kfn.TargetAVG2000 { t.Errorf("target: %v", *c.Target) }
}

func TestTargetForced(t *testing.T) {
	c := newCompiler()
	c.TargetForced = true
	*c.Target = kfn.TargetRealLive
	c.Compile(ast.DTargetStmt{Target: "AVG2000"})
	if *c.Target != kfn.TargetRealLive { t.Error("forced target should not change") }
	if len(c.Warnings) == 0 { t.Error("expected warning") }
}

// ============================================================
// #version
// ============================================================

func TestVersion(t *testing.T) {
	c := newCompiler()
	c.Compile(ast.DVersionStmt{
		A: ast.IntLit{Val: 1}, B: ast.IntLit{Val: 6},
		C: ast.IntLit{Val: 3}, D: ast.IntLit{Val: 0},
	})
	if *c.Version != (kfn.Version{1, 6, 3, 0}) {
		t.Errorf("version: %v", *c.Version)
	}
}

// ============================================================
// Generic directives
// ============================================================

func TestDirectiveWarn(t *testing.T) {
	c := newCompiler()
	c.Compile(ast.DirectiveStmt{Name: "warn", Value: ast.StrLit{
		Tokens: []ast.StrToken{ast.TextToken{Text: "test warning"}},
	}})
	if len(c.Warnings) == 0 { t.Error("expected warning") }
}

func TestDirectiveError(t *testing.T) {
	c := newCompiler()
	c.Compile(ast.DirectiveStmt{Name: "error", Value: ast.StrLit{
		Tokens: []ast.StrToken{ast.TextToken{Text: "test error"}},
	}})
	if len(c.Errors) == 0 { t.Error("expected error") }
}

func TestDirectiveEntrypoint(t *testing.T) {
	c := newCompiler()
	c.Compile(ast.DirectiveStmt{Name: "entrypoint", Value: ast.IntLit{Val: 5}})
	// Should have added an entrypoint to the output
	if c.Output.Length() == 0 { t.Error("expected entrypoint in output") }
}

func TestDirectiveEntrypointInvalid(t *testing.T) {
	c := newCompiler()
	c.Compile(ast.DirectiveStmt{Loc: ast.Loc{File: "test", Line: 1},
		Name: "entrypoint", Value: ast.IntLit{Val: 200}})
	if len(c.Errors) == 0 { t.Error("expected error for invalid entrypoint") }
}

func TestDirectiveVal0x2c(t *testing.T) {
	c := newCompiler()
	c.Compile(ast.DirectiveStmt{Name: "val_0x2c", Value: ast.IntLit{Val: 42}})
	if c.State.Val0x2C != 42 { t.Errorf("val_0x2c: %d", c.State.Val0x2C) }
}

func TestDirectiveCharacter(t *testing.T) {
	c := newCompiler()
	c.Compile(ast.DirectiveStmt{Name: "character", Value: ast.StrLit{
		Tokens: []ast.StrToken{ast.TextToken{Text: "Nagisa"}},
	}})
	if len(c.State.DramatisPersonae) != 1 || c.State.DramatisPersonae[0] != "Nagisa" {
		t.Error("character")
	}
}

func TestDirectiveKidokuType(t *testing.T) {
	c := newCompiler()
	c.Compile(ast.DirectiveStmt{Name: "kidoku_type", Value: ast.IntLit{Val: 2}})
	if c.Ini.GetInt("KIDOKU_TYPE", 0) != 2 { t.Error("kidoku_type") }
}

func TestDirectiveFile(t *testing.T) {
	c := newCompiler()
	c.Compile(ast.DirectiveStmt{Name: "file", Value: ast.StrLit{
		Tokens: []ast.StrToken{ast.TextToken{Text: "SEEN0001"}},
	}})
	if c.OutFile != "SEEN0001" { t.Errorf("file: %q", c.OutFile) }
}

func TestDirectiveFileNotOverwrite(t *testing.T) {
	c := newCompiler()
	c.OutFile = "existing"
	c.Compile(ast.DirectiveStmt{Name: "file", Value: ast.StrLit{
		Tokens: []ast.StrToken{ast.TextToken{Text: "new"}},
	}})
	if c.OutFile != "existing" { t.Error("should not overwrite existing filename") }
}
