// Package sel implements select menu compilation for the Kepago compiler.
//
// Transposed from OCaml's rlc/select.ml (222 lines).
//
// The package name is "sel" (not "select") because "select" is a Go keyword.
//
// RealLive's select instructions present in-game menus to the player.
// Multiple select opcodes exist in module 002 (Sel):
//   - select (opcode 0)   — basic selection
//   - select_s (opcode 1) — selection with window
//   - select_w (opcode 2/10/11/12/13) — various windowed selects
//
// Each select parameter can be:
//   - Always: a simple string expression (always displayed)
//   - Special: a conditional parameter with effects (colour, title, hide, etc.)
//
// Conditional effects use single-char op codes:
//   '0' = colour, '1' = title/grey, '2' = hide, '3' = blank, '4' = cursor
//
// Bytecode layout:
//   opcode(window_expr){ param1 param2 ... }
//   where each param can have conditions: (cond_expr)op_char(normal_param)
package sel

import (
	"fmt"

	"github.com/yoremi/rldev-go/rlc/pkg/ast"
	"github.com/yoremi/rldev-go/rlc/pkg/codegen"
)

// ============================================================
// Select condition effects
// ============================================================

// EffectOp maps a condition effect name to its bytecode character.
func EffectOp(name string) (byte, error) {
	switch name {
	case "colour":         return '0', nil
	case "title", "grey":  return '1', nil
	case "hide":           return '2', nil
	case "blank":          return '3', nil
	case "cursor":         return '4', nil
	}
	return 0, fmt.Errorf("unknown effect '%s' in select condition", name)
}

// ============================================================
// Select parameter types
// ============================================================

// SelParamKind distinguishes parameter types.
type SelParamKind int

const (
	SelAlways  SelParamKind = iota // always displayed
	SelSpecial                      // conditional display
)

// CondKind distinguishes condition types within a Special parameter.
type CondKind int

const (
	CondFlag    CondKind = iota // flag only: effect_op
	CondNonCond                 // unconditional with value: effect_op + expr
	CondCond                    // conditional: (cond_expr) effect_op [value_expr]
)

// SelCond is one condition within a Special select parameter.
type SelCond struct {
	Kind   CondKind
	Effect string   // effect name ("colour", "title", "hide", "blank", "cursor")
	Expr   ast.Expr // value expression (for NonCond and optionally Cond)
	Cond   ast.Expr // condition expression (for Cond only)
}

// SelParam is one parameter in a select call.
type SelParam struct {
	Kind  SelParamKind
	Loc   ast.Loc
	Expr  ast.Expr  // the main display expression
	Conds []SelCond // conditions (for Special only)
}

// ============================================================
// Select compilation
// ============================================================

// SelectModule is the RealLive module for select functions (002 = Sel).
const SelectModule = 2

// EmitSelect emits a select instruction to the codegen Output buffer.
//
// Parameters:
//   - opcode: the select variant (0=select, 1=select_s, etc.)
//   - window: optional window expression (nil if none)
//   - dest: the destination variable expression (Store or var)
//   - params: the select parameters
//   - restrictedOpcodes: opcodes that don't allow window specifiers
func EmitSelect(out *codegen.Output, loc ast.Loc, opcode int, window ast.Expr, dest ast.Expr, params []SelParam) error {
	if len(params) == 0 {
		// Warning: select called with no options (not an error, just unusual)
	}

	// Emit kidoku + opcode header
	out.AddKidoku(loc, loc.Line)
	out.EmitOpcode(loc, 0, SelectModule, opcode, len(params), 0)

	// Emit optional window expression
	if window != nil {
		// Opcode 13 doesn't support window specifiers
		if opcode == 13 {
			return fmt.Errorf("select window specifiers are not valid for opcode %d", opcode)
		}
		out.AddCode(ast.Nowhere, []byte{'('})
		out.EmitExpr(window)
		out.AddCode(ast.Nowhere, []byte{')'})
	}

	// Open parameter block
	out.AddCode(ast.Nowhere, []byte{'{'})

	// Emit each parameter
	for _, p := range params {
		switch p.Kind {
		case SelAlways:
			emitParamExpr(out, p.Loc, p.Expr)

		case SelSpecial:
			if len(p.Conds) == 0 {
				// No conditions → same as Always
				emitParamExpr(out, p.Loc, p.Expr)
			} else {
				// Emit conditions block
				out.AddCode(ast.Nowhere, []byte{'('})
				for _, c := range p.Conds {
					op, err := EffectOp(c.Effect)
					if err != nil {
						return err
					}
					switch c.Kind {
					case CondFlag:
						out.AddCode(ast.Nowhere, []byte{op})
					case CondNonCond:
						out.AddCode(ast.Nowhere, []byte{op})
						out.EmitExpr(c.Expr)
					case CondCond:
						out.AddCode(ast.Nowhere, []byte{'('})
						out.EmitExpr(c.Cond)
						out.AddCode(ast.Nowhere, []byte{')', op})
						if c.Expr != nil {
							out.EmitExpr(c.Expr)
						}
					}
				}
				out.AddCode(ast.Nowhere, []byte{')'})
				// Emit the display expression
				emitParamExpr(out, p.Loc, p.Expr)
			}
		}
	}

	// Close parameter block
	out.AddCode(ast.Nowhere, []byte{'}'})

	// If dest is not Store, emit assignment: dest \= store
	if _, isStore := dest.(ast.StoreRef); !isStore {
		out.EmitExpr(dest)
		out.AddCode(loc, []byte{'\\', 0x1e, '$', 0xc8}) // \= $store
	}

	return nil
}

// emitParamExpr emits a parameter expression.
// For now this emits the raw expression; the full compiler would
// also handle literal strings with ###PRINT wrapping.
func emitParamExpr(out *codegen.Output, loc ast.Loc, e ast.Expr) {
	out.EmitExpr(e)
}

// ============================================================
// VWF (Variable Width Font) select helpers
// ============================================================

// IsVWFOpcode returns true if the select opcode is handled by
// the VWF (variable width font) select system.
// VWF-eligible opcodes: 0, 1, 10, 11
func IsVWFOpcode(opcode int) bool {
	return opcode == 0 || opcode == 1 || opcode == 10 || opcode == 11
}

// WindowRestrictedOpcode returns true if the opcode doesn't allow
// window specifiers (currently only opcode 13).
func WindowRestrictedOpcode(opcode int) bool {
	return opcode == 13
}
