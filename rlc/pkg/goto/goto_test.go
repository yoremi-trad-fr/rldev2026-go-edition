package gotojmp

import (
	"testing"

	"github.com/yoremi/rldev-go/rlc/pkg/ast"
	"github.com/yoremi/rldev-go/rlc/pkg/codegen"
	"github.com/yoremi/rldev-go/rlc/pkg/kfn"
)

// ============================================================
// SpecialCase tests
// ============================================================

func TestSpecialCaseNonConst(t *testing.T) {
	// Variable expression → not a special case
	v := ast.IntVar{Bank: 0, Index: ast.IntLit{Val: 0}}
	handled, _, _ := SpecialCase(v, false, false)
	if handled {
		t.Error("non-constant should not be handled")
	}
}

func TestSpecialCaseGotoIfTrue(t *testing.T) {
	// goto_if(1) → should jump (neg=false, val=1)
	handled, shouldJump, ident := SpecialCase(ast.IntLit{Val: 1}, false, false)
	if !handled { t.Fatal("should be handled") }
	if !shouldJump { t.Error("goto_if(1) should jump") }
	if ident != "goto" { t.Errorf("ident: %q", ident) }
}

func TestSpecialCaseGotoIfFalse(t *testing.T) {
	// goto_if(0) → dead branch, no jump
	handled, shouldJump, _ := SpecialCase(ast.IntLit{Val: 0}, false, false)
	if !handled { t.Fatal("should be handled") }
	if shouldJump { t.Error("goto_if(0) should NOT jump") }
}

func TestSpecialCaseGotoUnlessTrue(t *testing.T) {
	// goto_unless(1) → neg=true, val=1 → no jump
	handled, shouldJump, _ := SpecialCase(ast.IntLit{Val: 1}, true, false)
	if !handled { t.Fatal("should be handled") }
	if shouldJump { t.Error("goto_unless(1) should NOT jump") }
}

func TestSpecialCaseGotoUnlessFalse(t *testing.T) {
	// goto_unless(0) → neg=true, val=0 → should jump
	handled, shouldJump, ident := SpecialCase(ast.IntLit{Val: 0}, true, false)
	if !handled { t.Fatal("should be handled") }
	if !shouldJump { t.Error("goto_unless(0) should jump") }
	if ident != "goto" { t.Errorf("ident: %q", ident) }
}

func TestSpecialCaseGosubIf(t *testing.T) {
	// gosub_if(42) → call=true, should jump
	handled, shouldJump, ident := SpecialCase(ast.IntLit{Val: 42}, false, true)
	if !handled { t.Fatal("should be handled") }
	if !shouldJump { t.Error("gosub_if(42) should jump") }
	if ident != "gosub" { t.Errorf("ident: %q", ident) }
}

func TestSpecialCaseGosubUnless(t *testing.T) {
	// gosub_unless(0) → call=true, neg=true → should jump
	handled, shouldJump, ident := SpecialCase(ast.IntLit{Val: 0}, true, true)
	if !handled { t.Fatal("should be handled") }
	if !shouldJump { t.Error("gosub_unless(0) should jump") }
	if ident != "gosub" { t.Errorf("ident: %q", ident) }
}

func TestSpecialCaseNegativeValue(t *testing.T) {
	// goto_if(-1) → nonzero, neg=false → jump
	handled, shouldJump, _ := SpecialCase(ast.IntLit{Val: -1}, false, false)
	if !handled { t.Fatal("should be handled") }
	if !shouldJump { t.Error("goto_if(-1) should jump") }
}

// ============================================================
// BuildGotoOn tests
// ============================================================

func TestBuildGotoOn(t *testing.T) {
	reg := kfn.NewRegistry()
	reg.Register(&kfn.FuncDef{Ident: "goto_on", OpType: 0, OpModule: 1, OpCode: 3})

	labels := []string{"label1", "label2", "label3"}
	result, err := BuildGotoOn(reg, "goto_on", ast.IntLit{Val: 0}, labels)
	if err != nil { t.Fatal(err) }
	if len(result.Opcode) != 8 { t.Errorf("opcode len: %d", len(result.Opcode)) }
	if result.Opcode[0] != '#' { t.Error("opcode prefix") }
	if len(result.Labels) != 3 { t.Errorf("labels: %d", len(result.Labels)) }
}

func TestBuildGotoOnFallback(t *testing.T) {
	// Unknown function → uses defaults (module=1, code=3)
	reg := kfn.NewRegistry()
	result, err := BuildGotoOn(reg, "unknown_goto", ast.IntLit{Val: 0}, []string{"a"})
	if err != nil { t.Fatal(err) }
	if result.Opcode[2] != 1 { t.Errorf("fallback module: %d", result.Opcode[2]) }
}

// ============================================================
// EmitGotoOn tests
// ============================================================

func TestEmitGotoOn(t *testing.T) {
	out := codegen.NewOutput()
	reg := kfn.NewRegistry()
	reg.Register(&kfn.FuncDef{Ident: "goto_on", OpType: 0, OpModule: 1, OpCode: 3})

	labels := []ast.Label{
		{Ident: "start", Loc: ast.Nowhere},
		{Ident: "end", Loc: ast.Nowhere},
	}

	// Define labels so resolution works
	out.AddLabel("start", ast.Nowhere)
	out.AddLabel("end", ast.Nowhere)

	EmitGotoOn(out, ast.Nowhere, reg, "goto_on", ast.IntLit{Val: 0}, labels)

	// Should have: label, label, opcode, (, expr, ), {, labelref, labelref, }
	if out.Length() < 6 { t.Errorf("IR count: %d, expected >= 6", out.Length()) }
}

func TestEmitGotoOnFallback(t *testing.T) {
	out := codegen.NewOutput()
	reg := kfn.NewRegistry()
	// No registration → uses defaults
	labels := []ast.Label{{Ident: "x", Loc: ast.Nowhere}}
	out.AddLabel("x", ast.Nowhere)
	EmitGotoOn(out, ast.Nowhere, reg, "nonexist", ast.IntLit{Val: 0}, labels)
	if out.Length() < 4 { t.Errorf("IR count: %d", out.Length()) }
}

// ============================================================
// EmitGotoCase tests
// ============================================================

func TestEmitGotoCase(t *testing.T) {
	out := codegen.NewOutput()
	reg := kfn.NewRegistry()
	reg.Register(&kfn.FuncDef{Ident: "goto_case", OpType: 0, OpModule: 1, OpCode: 4})

	out.AddLabel("one", ast.Nowhere)
	out.AddLabel("two", ast.Nowhere)
	out.AddLabel("def", ast.Nowhere)

	cases := []GotoCaseArm{
		{Expr: ast.IntLit{Val: 1}, Label: ast.Label{Ident: "one"}},
		{Expr: ast.IntLit{Val: 2}, Label: ast.Label{Ident: "two"}},
		{IsDefault: true, Label: ast.Label{Ident: "def"}},
	}

	EmitGotoCase(out, ast.Nowhere, reg, "goto_case", ast.IntLit{Val: 0}, cases)

	// Should have substantial IR: labels + opcode + ( expr ) { (match)ref (match)ref ()ref }
	if out.Length() < 10 { t.Errorf("IR count: %d, expected >= 10", out.Length()) }
}

func TestEmitGotoCaseDefaultOnly(t *testing.T) {
	out := codegen.NewOutput()
	reg := kfn.NewRegistry()

	out.AddLabel("def", ast.Nowhere)

	cases := []GotoCaseArm{
		{IsDefault: true, Label: ast.Label{Ident: "def"}},
	}

	EmitGotoCase(out, ast.Nowhere, reg, "goto_case", ast.IntLit{Val: 0}, cases)
	if out.Length() < 5 { t.Errorf("IR count: %d", out.Length()) }
}

func TestEmitGotoCaseNoDefault(t *testing.T) {
	out := codegen.NewOutput()
	reg := kfn.NewRegistry()

	out.AddLabel("a", ast.Nowhere)

	cases := []GotoCaseArm{
		{Expr: ast.IntLit{Val: 42}, Label: ast.Label{Ident: "a"}},
	}

	EmitGotoCase(out, ast.Nowhere, reg, "goto_case", ast.IntLit{Val: 0}, cases)
	if out.Length() < 5 { t.Errorf("IR count: %d", out.Length()) }
}

// ============================================================
// GotoCaseArm type
// ============================================================

func TestGotoCaseArmDefault(t *testing.T) {
	arm := GotoCaseArm{IsDefault: true, Label: ast.Label{Ident: "fallback"}}
	if !arm.IsDefault { t.Error("should be default") }
	if arm.Label.Ident != "fallback" { t.Error("label") }
}

func TestGotoCaseArmMatch(t *testing.T) {
	arm := GotoCaseArm{Expr: ast.IntLit{Val: 5}, Label: ast.Label{Ident: "five"}}
	if arm.IsDefault { t.Error("should not be default") }
	lit := arm.Expr.(ast.IntLit)
	if lit.Val != 5 { t.Error("match value") }
}
