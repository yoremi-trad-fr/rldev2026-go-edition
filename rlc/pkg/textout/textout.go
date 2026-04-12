// Package textout implements text output compilation for the Kepago compiler.
//
// Transposed from OCaml's rlc/textout.ml (506 lines).
//
// RealLive has two text compilation modes:
//
//   1. Static (compile_stub): text is encoded directly as Shift_JIS bytecode
//      strings with embedded control codes. This is the default for standard
//      RealLive games.
//
//   2. Dynamic (compile/textout library): text is tokenized into a compact
//      array of packed integers (DTO = Dynamic Text Output), enabling runtime
//      lineation, formatting, and variable substitution. Used with the textout
//      library (textout.kh).
//
// Control codes supported: \r (CR), \n (LF), \p (page), \i{n} (integer),
// \s{var} (string variable), \c{fg,bg} (color), \size{n}, \wait{n},
// \e{idx}/\em{idx} (emoji), \mv*{}/\pos*{} (move/position),
// \ruby{base}{gloss} (furigana), \g{base}{gloss}, \name{}/\{}...{}
package textout

import (
	"fmt"

	"github.com/yoremi/rldev-go/rlc/pkg/ast"
)

// ============================================================
// Token IDs (from textout.kh / textout.ml lines 33-46)
// ============================================================

// Token IDs for the Dynamic Text Output (DTO) system.
// Each token is packed as: (arg1 << 12) | id | (arg2 << 4)
const (
	IDSText int32 = 0  // text followed by single space: offset, length
	IDNText int32 = 1  // text not followed by space: offset, length
	IDDText int32 = 2  // DBCS characters: offset, length
	IDDQuot int32 = 3  // double quote mark
	IDSpace int32 = 4  // whitespace: length
	IDNameV int32 = 5  // name variable: index, loc/glb, char_index_follows
	IDFSize int32 = 6  // font size: size, has_size
	IDVrble int32 = 7  // string variable: index, space
	IDCCode int32 = 8  // control code: args, code_id
	IDWName int32 = 9  // name block: 0=enter/1=leave, bracket_type
	IDRuby  int32 = 10 // furigana/gloss: depends, 0-2=ruby / 3-4=gloss
	IDEmoji int32 = 11 // emoji: index, use_colour
	IDMove  int32 = 12 // \mv*/\pos*: which(1=x,2=y,3=both), 0=mv/1=pos
)

// ControlCode IDs used in the code_id field of IDCCode tokens.
const (
	CCReturn = 0x0d // \r — carriage return
	CCNewline = 0x0a // \n — newline
	CCPage    = 0x16 // \p — page break
	CCIntVar  = 0x69 // \i{} — integer variable
	CCColor   = 0x63 // \c{} — color
	CCWait    = 0x77 // \wait{} — wait
)

// ============================================================
// Token encoding (from textout.ml make_token / make_token_nconst)
// ============================================================

// MakeToken builds a packed DTO token from constant arguments.
// Encoding: (arg1 << 12) | id | (arg2 << 4)
// Constraints: id < 16, arg1 < 1_048_575, arg2 < 256
func MakeToken(id int32, arg1, arg2 int) ast.Expr {
	return MakeTokenExpr(id, ast.IntLit{Val: int32(arg1)}, ast.IntLit{Val: int32(arg2)})
}

// MakeTokenExpr builds a DTO token from expression arguments.
// Same encoding but arguments can be runtime expressions.
func MakeTokenExpr(id int32, arg1, arg2 ast.Expr) ast.Expr {
	nw := ast.Nowhere
	twelve := ast.IntLit{Loc: nw, Val: 12}
	four := ast.IntLit{Loc: nw, Val: 4}
	idLit := ast.IntLit{Loc: nw, Val: id}

	// (arg1 << 12) | id | (arg2 << 4)
	return ast.BinOp{Loc: nw, Op: ast.OpOr,
		LHS: ast.ParenExpr{Loc: nw, Expr: ast.BinOp{Loc: nw, Op: ast.OpShl, LHS: arg1, RHS: twelve}},
		RHS: ast.BinOp{Loc: nw, Op: ast.OpOr,
			LHS: idLit,
			RHS: ast.ParenExpr{Loc: nw, Expr: ast.BinOp{Loc: nw, Op: ast.OpShl, LHS: arg2, RHS: four}},
		},
	}
}

// ============================================================
// Control code processing (from textout.ml get_code_tokens)
// ============================================================

// CodeResult holds the result of processing a control code.
type CodeResult struct {
	Tokens []ast.Expr // DTO tokens to emit
	Text   string     // if non-empty, text to add instead (e.g. \i{const})
}

// ProcessControlCode processes a single control code (e.g. \r, \n, \i{}, \s{}).
// Returns either a list of DTO tokens or literal text to inject.
func ProcessControlCode(code string, lenOpt ast.Expr, params []ast.Param) (CodeResult, error) {
	switch code {
	case "r":
		return CodeResult{Tokens: []ast.Expr{MakeToken(IDCCode, 0, CCReturn)}}, nil
	case "n":
		return CodeResult{Tokens: []ast.Expr{MakeToken(IDCCode, 0, CCNewline)}}, nil
	case "p":
		return CodeResult{Tokens: []ast.Expr{MakeToken(IDCCode, 0, CCPage)}}, nil

	case "i":
		param := oneSimpleParam(params)
		if param == nil {
			return CodeResult{}, errorf("\\i{} must have exactly one parameter")
		}
		if lenOpt == nil {
			// No width specifier
			if lit, ok := param.(ast.IntLit); ok {
				// Constant → expand to text
				return CodeResult{Text: int32ToString(lit.Val)}, nil
			}
			return CodeResult{Tokens: []ast.Expr{
				MakeToken(IDCCode, 0, CCIntVar),
				param,
			}}, nil
		}
		// With width specifier
		return CodeResult{Tokens: []ast.Expr{
			MakeTokenExpr(IDCCode, lenOpt, ast.IntLit{Val: CCIntVar}),
			param,
		}}, nil

	case "s":
		param := oneSimpleParam(params)
		if param == nil {
			return CodeResult{}, errorf("\\s{} must have exactly one parameter")
		}
		sv, ok := param.(ast.StrVar)
		if !ok {
			return CodeResult{}, errorf("\\s{} parameter must be a string variable")
		}
		return CodeResult{Tokens: []ast.Expr{
			MakeTokenExpr(IDVrble, sv.Index, ast.IntLit{Val: int32(sv.Bank)}),
		}}, nil

	case "c":
		fg := ast.IntLit{Val: 0}
		bg := ast.IntLit{Val: 0}
		argc := 0
		if len(params) >= 1 {
			if sp, ok := params[0].(ast.SimpleParam); ok {
				fg = asExprOrZero(sp.Expr)
				argc = 1
			}
		}
		if len(params) >= 2 {
			if sp, ok := params[1].(ast.SimpleParam); ok {
				bg = asExprOrZero(sp.Expr)
				argc = 2
			}
		}
		nw := ast.Nowhere
		// args = (fg << 2) | (bg << 10) | argc
		args := ast.BinOp{Loc: nw, Op: ast.OpOr,
			LHS: ast.ParenExpr{Loc: nw, Expr: ast.BinOp{Loc: nw, Op: ast.OpShl,
				LHS: fg, RHS: ast.IntLit{Val: 2}}},
			RHS: ast.BinOp{Loc: nw, Op: ast.OpOr,
				LHS: ast.ParenExpr{Loc: nw, Expr: ast.BinOp{Loc: nw, Op: ast.OpShl,
					LHS: bg, RHS: ast.IntLit{Val: 10}}},
				RHS: ast.IntLit{Val: int32(argc)},
			},
		}
		return CodeResult{Tokens: []ast.Expr{
			MakeTokenExpr(IDCCode, args, ast.IntLit{Val: CCColor}),
		}}, nil

	case "size":
		hasSz := 0
		sz := ast.Expr(ast.IntLit{Val: 0})
		if len(params) == 1 {
			if sp, ok := params[0].(ast.SimpleParam); ok {
				hasSz = 1
				sz = sp.Expr
			}
		}
		return CodeResult{Tokens: []ast.Expr{
			MakeTokenExpr(IDFSize, sz, ast.IntLit{Val: int32(hasSz)}),
		}}, nil

	case "wait":
		param := oneSimpleParam(params)
		if param == nil {
			return CodeResult{}, errorf("\\wait{} must have exactly one parameter")
		}
		return CodeResult{Tokens: []ast.Expr{
			MakeTokenExpr(IDCCode, param, ast.IntLit{Val: CCWait}),
		}}, nil

	case "e", "em":
		if len(params) < 1 {
			return CodeResult{}, errorf("\\%s{} requires at least one parameter", code)
		}
		idx := oneSimpleParam(params[:1])
		useColor := 1
		if code == "em" {
			useColor = 0
		}
		ecode := MakeTokenExpr(IDEmoji, idx, ast.IntLit{Val: int32(useColor)})
		if len(params) >= 2 {
			if sp, ok := params[1].(ast.SimpleParam); ok {
				// With font size: size_on, emoji, size_off
				return CodeResult{Tokens: []ast.Expr{
					MakeTokenExpr(IDFSize, sp.Expr, ast.IntLit{Val: 1}),
					ecode,
					MakeToken(IDFSize, 0, 0),
				}}, nil
			}
		}
		return CodeResult{Tokens: []ast.Expr{ecode}}, nil

	case "mv", "mvx", "mvy", "pos", "posx", "posy":
		isPos := 0
		if code[0] == 'p' {
			isPos = 1
		}
		which := 0
		var tokenParams []ast.Expr
		switch code {
		case "mv", "pos":
			if len(params) < 2 {
				return CodeResult{}, errorf("\\%s{} requires x and y parameters", code)
			}
			which = 0b11
			tokenParams = extractSimpleExprs(params[:2])
		case "mvy", "posy":
			if len(params) < 1 {
				return CodeResult{}, errorf("\\%s{} requires a parameter", code)
			}
			which = 0b10
			tokenParams = extractSimpleExprs(params[:1])
		default: // mvx, posx
			if len(params) < 1 {
				return CodeResult{}, errorf("\\%s{} requires a parameter", code)
			}
			which = 0b01
			tokenParams = extractSimpleExprs(params[:1])
		}
		result := []ast.Expr{MakeToken(IDMove, which, isPos)}
		result = append(result, tokenParams...)
		return CodeResult{Tokens: result}, nil
	}

	return CodeResult{}, errorf("unknown control code '\\%s{}'", code)
}

// ============================================================
// Pause/page types
// ============================================================

// PauseType indicates what follows a text output.
type PauseType int

const (
	PauseNone  PauseType = iota // no pause
	PausePause                   // \pause
	PausePage                    // \page
)

// ============================================================
// Static text byte encoding constants
// ============================================================

// Shift_JIS special characters used in static text output.
const (
	SJISLeftBracket  = "\x81\x79" // 【
	SJISRightBracket = "\x81\x7a" // 】
	SJISAsterisk     = "\x81\x96" // ＊
	SJISPercent      = "\x81\x93" // ％
)

// ============================================================
// Helpers
// ============================================================

func oneSimpleParam(params []ast.Param) ast.Expr {
	if len(params) != 1 {
		return nil
	}
	sp, ok := params[0].(ast.SimpleParam)
	if !ok {
		return nil
	}
	return sp.Expr
}

func asExprOrZero(e ast.Expr) ast.IntLit {
	if lit, ok := e.(ast.IntLit); ok {
		return lit
	}
	return ast.IntLit{Val: 0}
}

func extractSimpleExprs(params []ast.Param) []ast.Expr {
	var result []ast.Expr
	for _, p := range params {
		if sp, ok := p.(ast.SimpleParam); ok {
			result = append(result, sp.Expr)
		}
	}
	return result
}

func int32ToString(v int32) string {
	// Simple int32 to decimal string
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	digits := make([]byte, 0, 12)
	for v > 0 {
		digits = append(digits, byte('0'+v%10))
		v /= 10
	}
	if neg {
		digits = append(digits, '-')
	}
	// Reverse
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(digits)
}

func errorf(format string, args ...interface{}) error {
	return fmt.Errorf(format, args...)
}
