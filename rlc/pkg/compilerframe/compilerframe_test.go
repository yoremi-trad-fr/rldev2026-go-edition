package compilerframe

import (
	"testing"

	"github.com/yoremi/rldev-go/rlc/pkg/ast"
	"github.com/yoremi/rldev-go/rlc/pkg/ini"
	"github.com/yoremi/rldev-go/rlc/pkg/kfn"
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
	// Unimplemented statement types should produce warnings, not errors
	c := newComp()
	c.Parse([]ast.Stmt{ast.LabelStmt{}})
	if c.HasErrors() { t.Error("should not have errors, only warnings") }
	if len(c.Warnings) == 0 { t.Error("should have TODO warning") }
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
	// Multiple top-level statements
	c.Parse([]ast.Stmt{
		ast.DefineStmt{Ident: "A", Value: ast.IntLit{Val: 1}},
		ast.DefineStmt{Ident: "B", Value: ast.IntLit{Val: 2}},
		ast.HaltStmt{},
	})
	if !c.Mem.Defined("A") { t.Error("A") }
	if !c.Mem.Defined("B") { t.Error("B") }
}
