package sel

import (
	"testing"

	"github.com/yoremi/rldev-go/rlc/pkg/ast"
	"github.com/yoremi/rldev-go/rlc/pkg/codegen"
)

// ============================================================
// EffectOp tests
// ============================================================

func TestEffectOp(t *testing.T) {
	tests := []struct{ name string; want byte }{
		{"colour", '0'},
		{"title", '1'},
		{"grey", '1'},
		{"hide", '2'},
		{"blank", '3'},
		{"cursor", '4'},
	}
	for _, tt := range tests {
		got, err := EffectOp(tt.name)
		if err != nil { t.Errorf("%s: %v", tt.name, err); continue }
		if got != tt.want { t.Errorf("EffectOp(%q) = %c, want %c", tt.name, got, tt.want) }
	}
}

func TestEffectOpUnknown(t *testing.T) {
	_, err := EffectOp("nonexistent")
	if err == nil { t.Error("expected error for unknown effect") }
}

// ============================================================
// EmitSelect tests
// ============================================================

func TestEmitSelectBasic(t *testing.T) {
	out := codegen.NewOutput()
	params := []SelParam{
		{Kind: SelAlways, Expr: ast.StrLit{Tokens: []ast.StrToken{ast.TextToken{Text: "option1"}}}},
		{Kind: SelAlways, Expr: ast.StrLit{Tokens: []ast.StrToken{ast.TextToken{Text: "option2"}}}},
	}
	err := EmitSelect(out, ast.Loc{Line: 1}, 0, nil, ast.StoreRef{}, params)
	if err != nil { t.Fatal(err) }
	// Should have: kidoku + opcode + { + param1 + param2 + }
	if out.Length() < 4 { t.Errorf("IR count: %d, expected >= 4", out.Length()) }
}

func TestEmitSelectWithWindow(t *testing.T) {
	out := codegen.NewOutput()
	params := []SelParam{
		{Kind: SelAlways, Expr: ast.IntLit{Val: 1}},
	}
	window := ast.IntLit{Val: 3}
	err := EmitSelect(out, ast.Loc{Line: 1}, 1, window, ast.StoreRef{}, params)
	if err != nil { t.Fatal(err) }
	// Should have window expression in parentheses
	if out.Length() < 6 { t.Errorf("IR count: %d", out.Length()) }
}

func TestEmitSelectWindowRestricted(t *testing.T) {
	out := codegen.NewOutput()
	params := []SelParam{
		{Kind: SelAlways, Expr: ast.IntLit{Val: 1}},
	}
	// Opcode 13 doesn't allow window specifiers
	err := EmitSelect(out, ast.Nowhere, 13, ast.IntLit{Val: 0}, ast.StoreRef{}, params)
	if err == nil { t.Error("expected error for opcode 13 with window") }
}

func TestEmitSelectNoWindow(t *testing.T) {
	out := codegen.NewOutput()
	params := []SelParam{
		{Kind: SelAlways, Expr: ast.IntLit{Val: 1}},
	}
	err := EmitSelect(out, ast.Loc{Line: 1}, 0, nil, ast.StoreRef{}, params)
	if err != nil { t.Fatal(err) }
}

func TestEmitSelectEmpty(t *testing.T) {
	out := codegen.NewOutput()
	err := EmitSelect(out, ast.Loc{Line: 1}, 0, nil, ast.StoreRef{}, nil)
	if err != nil { t.Fatal(err) }
	// Empty select is valid (though warns in OCaml)
}

func TestEmitSelectWithDest(t *testing.T) {
	out := codegen.NewOutput()
	params := []SelParam{
		{Kind: SelAlways, Expr: ast.IntLit{Val: 1}},
	}
	dest := ast.IntVar{Bank: 0x0b, Index: ast.IntLit{Val: 5}}
	err := EmitSelect(out, ast.Loc{Line: 1}, 0, nil, dest, params)
	if err != nil { t.Fatal(err) }
	// Should have extra assignment at the end: dest \= store
	if out.Length() < 6 { t.Errorf("IR count: %d, expected >= 6 (with dest assign)", out.Length()) }
}

func TestEmitSelectStoreDest(t *testing.T) {
	out := codegen.NewOutput()
	params := []SelParam{
		{Kind: SelAlways, Expr: ast.IntLit{Val: 1}},
	}
	// Store dest → no extra assignment
	err := EmitSelect(out, ast.Loc{Line: 1}, 0, nil, ast.StoreRef{}, params)
	if err != nil { t.Fatal(err) }
}

// ============================================================
// Special (conditional) parameters
// ============================================================

func TestEmitSelectSpecialFlag(t *testing.T) {
	out := codegen.NewOutput()
	params := []SelParam{
		{Kind: SelSpecial, Expr: ast.IntLit{Val: 1}, Conds: []SelCond{
			{Kind: CondFlag, Effect: "hide"},
		}},
	}
	err := EmitSelect(out, ast.Loc{Line: 1}, 0, nil, ast.StoreRef{}, params)
	if err != nil { t.Fatal(err) }
}

func TestEmitSelectSpecialNonCond(t *testing.T) {
	out := codegen.NewOutput()
	params := []SelParam{
		{Kind: SelSpecial, Expr: ast.IntLit{Val: 1}, Conds: []SelCond{
			{Kind: CondNonCond, Effect: "colour", Expr: ast.IntLit{Val: 128}},
		}},
	}
	err := EmitSelect(out, ast.Loc{Line: 1}, 0, nil, ast.StoreRef{}, params)
	if err != nil { t.Fatal(err) }
}

func TestEmitSelectSpecialCond(t *testing.T) {
	out := codegen.NewOutput()
	params := []SelParam{
		{Kind: SelSpecial, Expr: ast.IntLit{Val: 1}, Conds: []SelCond{
			{Kind: CondCond, Effect: "colour", Expr: ast.IntLit{Val: 200}, Cond: ast.IntLit{Val: 1}},
		}},
	}
	err := EmitSelect(out, ast.Loc{Line: 1}, 0, nil, ast.StoreRef{}, params)
	if err != nil { t.Fatal(err) }
}

func TestEmitSelectSpecialNoConds(t *testing.T) {
	// Special with empty conditions → treated as Always
	out := codegen.NewOutput()
	params := []SelParam{
		{Kind: SelSpecial, Expr: ast.IntLit{Val: 1}, Conds: nil},
	}
	err := EmitSelect(out, ast.Loc{Line: 1}, 0, nil, ast.StoreRef{}, params)
	if err != nil { t.Fatal(err) }
}

func TestEmitSelectBadEffect(t *testing.T) {
	out := codegen.NewOutput()
	params := []SelParam{
		{Kind: SelSpecial, Expr: ast.IntLit{Val: 1}, Conds: []SelCond{
			{Kind: CondFlag, Effect: "badeffect"},
		}},
	}
	err := EmitSelect(out, ast.Nowhere, 0, nil, ast.StoreRef{}, params)
	if err == nil { t.Error("expected error for bad effect") }
}

func TestEmitSelectMixed(t *testing.T) {
	out := codegen.NewOutput()
	params := []SelParam{
		{Kind: SelAlways, Expr: ast.IntLit{Val: 1}},
		{Kind: SelSpecial, Expr: ast.IntLit{Val: 2}, Conds: []SelCond{
			{Kind: CondFlag, Effect: "title"},
		}},
		{Kind: SelAlways, Expr: ast.IntLit{Val: 3}},
	}
	err := EmitSelect(out, ast.Loc{Line: 1}, 0, nil, ast.StoreRef{}, params)
	if err != nil { t.Fatal(err) }
	if out.Length() < 6 { t.Errorf("IR count: %d", out.Length()) }
}

// ============================================================
// VWF helpers
// ============================================================

func TestIsVWFOpcode(t *testing.T) {
	vwf := []int{0, 1, 10, 11}
	for _, op := range vwf {
		if !IsVWFOpcode(op) { t.Errorf("opcode %d should be VWF", op) }
	}
	nonVwf := []int{2, 3, 4, 12, 13}
	for _, op := range nonVwf {
		if IsVWFOpcode(op) { t.Errorf("opcode %d should NOT be VWF", op) }
	}
}

func TestWindowRestrictedOpcode(t *testing.T) {
	if !WindowRestrictedOpcode(13) { t.Error("opcode 13 should be restricted") }
	if WindowRestrictedOpcode(0) { t.Error("opcode 0 should not be restricted") }
	if WindowRestrictedOpcode(1) { t.Error("opcode 1 should not be restricted") }
}

// ============================================================
// Select module constant
// ============================================================

func TestSelectModule(t *testing.T) {
	if SelectModule != 2 { t.Errorf("SelectModule = %d, want 2", SelectModule) }
}
