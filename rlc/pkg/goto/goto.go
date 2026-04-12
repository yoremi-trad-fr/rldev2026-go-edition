// Package gotojmp implements goto/gosub compilation for the Kepago compiler.
//
// Transposed from OCaml's rlc/goto.ml (261 lines).
//
// RealLive has several jump instructions:
//   - goto/gosub:           unconditional jump/call to a label
//   - goto_if/gosub_if:     conditional jump if expression != 0
//   - goto_unless/gosub_unless: conditional jump if expression == 0
//   - goto_on/gosub_on:     computed jump to one of N labels by index
//   - goto_case/gosub_case: computed jump by matching expression against values
//
// This package handles:
//   1. SpecialCase: optimizes constant conditionals — when the condition
//      is a compile-time constant, the conditional is replaced with either
//      nothing (dead branch) or an unconditional goto/gosub.
//   2. GotoOn: emits opcode(expr){label1, label2, ...}
//   3. GotoCase: emits opcode(expr){(match1)label1, (match2)label2, ()default}
//
// The package name is "gotojmp" (not "goto") because "goto" is a Go keyword.
package gotojmp

import (
	"github.com/yoremi/rldev-go/rlc/pkg/ast"
	"github.com/yoremi/rldev-go/rlc/pkg/codegen"
	"github.com/yoremi/rldev-go/rlc/pkg/kfn"
)

// ============================================================
// SpecialCase: constant condition optimization
// (from goto.ml lines 216-230)
// ============================================================

// SpecialCase checks if a conditional goto/gosub has a constant condition
// and can be optimized. Returns true if the case was handled (either
// eliminated as dead code or converted to unconditional jump).
//
// Parameters:
//   - param: the condition expression (must be already normalized)
//   - neg: true if this is a negated conditional (goto_unless/gosub_unless)
//   - call: true for gosub, false for goto
//
// Logic (matching OCaml):
//
//	if param is constant:
//	  if (param==0 && neg) || (param!=0 && !neg):
//	    emit unconditional goto/gosub
//	  else:
//	    do nothing (dead branch)
//	  return true (handled)
//	else:
//	  return false (not a special case)
func SpecialCase(param ast.Expr, neg bool, call bool) (handled bool, shouldJump bool, ident string) {
	lit, ok := param.(ast.IntLit)
	if !ok {
		return false, false, ""
	}

	// Determine if we should actually jump
	isZero := lit.Val == 0
	if isZero {
		shouldJump = neg // goto_unless(0) = true, goto_if(0) = false
	} else {
		shouldJump = !neg // goto_if(nonzero) = true, goto_unless(nonzero) = false
	}

	if call {
		ident = "gosub"
	} else {
		ident = "goto"
	}

	return true, shouldJump, ident
}

// ============================================================
// GotoOn: computed jump by index
// (from goto.ml lines 233-240)
// ============================================================

// GotoOnResult holds the bytecode components for a goto_on/gosub_on.
type GotoOnResult struct {
	Opcode []byte   // opcode header bytes
	Expr   ast.Expr // the index expression
	Labels []string // label names to reference
}

// BuildGotoOn constructs a goto_on/gosub_on instruction.
// The function name (e.g. "goto_on", "gosub_on") is used to look up the
// opcode in the KFN registry.
//
// Bytecode layout: opcode(expr){labelref1, labelref2, ...}
func BuildGotoOn(reg *kfn.Registry, funcName string, expr ast.Expr, labels []string) (*GotoOnResult, error) {
	fn, ok := reg.Lookup(funcName)
	if !ok {
		// Fallback: use default opcodes
		fn = &kfn.FuncDef{OpType: 0, OpModule: 1, OpCode: 3}
	}

	opcode := codegen.EncodeOpcode(0, fn.OpModule, fn.OpCode, len(labels), 0)

	return &GotoOnResult{
		Opcode: opcode,
		Expr:   expr,
		Labels: labels,
	}, nil
}

// EmitGotoOn emits a goto_on instruction to the codegen Output buffer.
func EmitGotoOn(out *codegen.Output, loc ast.Loc, reg *kfn.Registry, funcName string, expr ast.Expr, labels []ast.Label) {
	fn, ok := reg.Lookup(funcName)
	opMod, opCode := 1, 3 // defaults for goto_on
	if ok {
		opMod = fn.OpModule
		opCode = fn.OpCode
	}

	out.EmitOpcode(loc, 0, opMod, opCode, len(labels), 0)
	out.AddCode(loc, []byte{'('})
	out.EmitExpr(expr)
	out.AddCode(loc, []byte{')', '{'})
	for _, lbl := range labels {
		out.AddLabelRef(lbl.Ident, lbl.Loc)
	}
	out.AddCode(ast.Nowhere, []byte{'}'})
}

// ============================================================
// GotoCase: computed jump by value matching
// (from goto.ml lines 243-261)
// ============================================================

// GotoCaseArm is one arm of a goto_case.
type GotoCaseArm struct {
	IsDefault bool     // true for the default case (no match expression)
	Expr      ast.Expr // match expression (nil for default)
	Label     ast.Label
}

// EmitGotoCase emits a goto_case instruction to the codegen Output buffer.
//
// Bytecode layout: opcode(expr){ (match1)labelref1, (match2)labelref2, ()defaultref }
func EmitGotoCase(out *codegen.Output, loc ast.Loc, reg *kfn.Registry, funcName string, expr ast.Expr, cases []GotoCaseArm) {
	fn, ok := reg.Lookup(funcName)
	opMod, opCode := 1, 4 // defaults for goto_case
	if ok {
		opMod = fn.OpModule
		opCode = fn.OpCode
	}

	out.EmitOpcode(loc, 0, opMod, opCode, len(cases), 0)
	out.AddCode(loc, []byte{'('})
	out.EmitExpr(expr)
	out.AddCode(loc, []byte{')', '{'})

	for _, arm := range cases {
		if arm.IsDefault {
			out.AddCode(ast.Nowhere, []byte{'(', ')'})
		} else {
			out.AddCode(ast.Nowhere, []byte{'('})
			out.EmitExpr(arm.Expr)
			out.AddCode(ast.Nowhere, []byte{')'})
		}
		out.AddLabelRef(arm.Label.Ident, arm.Label.Loc)
	}

	out.AddCode(ast.Nowhere, []byte{'}'})
}
