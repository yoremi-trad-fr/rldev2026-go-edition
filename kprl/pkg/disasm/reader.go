package disasm

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/yoremi/rldev-go/pkg/binarray"
	"github.com/yoremi/rldev-go/pkg/bytecode"
)

// Reader reads bytecodes sequentially from a buffer.
//
// The reader carries optional pointers to the disassembly result and
// the active Options. When set (via SetContext), string-collecting
// helpers can route Japanese strings to the resource list automatically
// (matching OCaml's --separate-all behavior on sjs1-prefixed strings).
type Reader struct {
	data   []byte
	pos    int
	origin int // data_offset (start of code section)
	limit  int // end of data
	mode   EngineMode

	// Optional context for resource emission.
	result *DisassemblyResult
	opts   *Options
}

// NewReader creates a bytecode reader starting at origin.
func NewReader(data []byte, origin, limit int, mode EngineMode) *Reader {
	return &Reader{
		data:   data,
		pos:    origin,
		origin: origin,
		limit:  limit,
		mode:   mode,
	}
}

// SetContext attaches the result and options to the reader so that
// helpers can emit resources transparently. Passing nil disables that.
func (r *Reader) SetContext(result *DisassemblyResult, opts *Options) {
	r.result = result
	r.opts = opts
}

// Pos returns the current read position.
func (r *Reader) Pos() int { return r.pos }

// RelPos returns the position relative to the code origin.
func (r *Reader) RelPos() int { return r.pos - r.origin }

// AtEnd returns true if the reader has reached the limit.
func (r *Reader) AtEnd() bool { return r.pos >= r.limit }

// Next reads the next byte and advances.
func (r *Reader) Next() (byte, error) {
	if r.pos >= r.limit {
		return 0, fmt.Errorf("end of data at offset 0x%x", r.pos)
	}
	b := r.data[r.pos]
	r.pos++
	return b, nil
}

// Peek returns the next byte without advancing.
func (r *Reader) Peek() (byte, error) {
	if r.pos >= r.limit {
		return 0, fmt.Errorf("end of data at offset 0x%x", r.pos)
	}
	return r.data[r.pos], nil
}

// ReadInt32 reads a little-endian 32-bit integer.
func (r *Reader) ReadInt32() (int32, error) {
	if r.pos+4 > r.limit {
		return 0, fmt.Errorf("not enough data for int32 at 0x%x", r.pos)
	}
	v := int32(binary.LittleEndian.Uint32(r.data[r.pos:]))
	r.pos += 4
	return v, nil
}

// ReadUint32 reads a little-endian 32-bit unsigned integer.
func (r *Reader) ReadUint32() (uint32, error) {
	if r.pos+4 > r.limit {
		return 0, fmt.Errorf("not enough data for uint32 at 0x%x", r.pos)
	}
	v := binary.LittleEndian.Uint32(r.data[r.pos:])
	r.pos += 4
	return v, nil
}

// ReadInt16 reads a little-endian 16-bit integer.
func (r *Reader) ReadInt16() (int16, error) {
	if r.pos+2 > r.limit {
		return 0, fmt.Errorf("not enough data for int16 at 0x%x", r.pos)
	}
	v := int16(binary.LittleEndian.Uint16(r.data[r.pos:]))
	r.pos += 2
	return v, nil
}

// ReadUint16 reads a little-endian 16-bit unsigned integer.
func (r *Reader) ReadUint16() (uint16, error) {
	if r.pos+2 > r.limit {
		return 0, fmt.Errorf("not enough data for uint16 at 0x%x", r.pos)
	}
	v := binary.LittleEndian.Uint16(r.data[r.pos:])
	r.pos += 2
	return v, nil
}

// GetInt reads a 32-bit signed integer (used for data values).
func (r *Reader) GetInt() (int, error) {
	v, err := r.ReadInt32()
	return int(v), err
}

// GetInt16 reads a 16-bit unsigned integer as int.
func (r *Reader) GetInt16() (int, error) {
	v, err := r.ReadUint16()
	return int(v), err
}

// GetIntForMode reads an int appropriate for the engine mode.
// AVG2000 uses 32-bit, RealLive uses 16-bit for some fields.
func (r *Reader) GetIntForMode() (int, error) {
	if r.mode == ModeAvg2000 {
		return r.GetInt()
	}
	return r.GetInt16()
}

// Expect reads the next byte and checks it matches the expected value.
func (r *Reader) Expect(expected byte, context string) error {
	b, err := r.Next()
	if err != nil {
		return fmt.Errorf("%s: expected '%c' (0x%02x), got EOF", context, expected, expected)
	}
	if b != expected {
		return fmt.Errorf("%s: expected '%c' (0x%02x), got 0x%02x at offset 0x%x",
			context, expected, expected, b, r.pos-1)
	}
	return nil
}

// Rollback moves the position back by n bytes.
func (r *Reader) Rollback(n int) {
	r.pos -= n
	if r.pos < r.origin {
		r.pos = r.origin
	}
}

// Skip advances the position by n bytes.
func (r *Reader) Skip(n int) {
	r.pos += n
}

// ReadBytes reads n bytes.
func (r *Reader) ReadBytes(n int) ([]byte, error) {
	if r.pos+n > r.limit {
		return nil, fmt.Errorf("not enough data for %d bytes at 0x%x", n, r.pos)
	}
	data := make([]byte, n)
	copy(data, r.data[r.pos:r.pos+n])
	r.pos += n
	return data, nil
}

// --- Expression parser ---
// Translates the OCaml get_expression/get_expr_term/get_expr_arith/etc.

// GetExpression reads an expression from the bytecode, returning its string representation.
//
// OCaml reference: get_expression / get_expr_bool (disassembler.ml ~L526, L512).
// CRITICAL: every binary/unary operator in RealLive bytecode is prefixed by
// 0x5C ('\\'). All four expression layers below must consume two bytes per
// operator: 0x5C followed by the actual op byte. Reading a single byte will
// desynchronize the entire reader after the first expression.
func (r *Reader) GetExpression() (string, error) {
	return r.getExprBool()
}

// peek2 returns true and the second byte if the next two bytes match (b0, *).
// Does NOT advance position.
func (r *Reader) peek2() (byte, byte, bool) {
	if r.pos+1 >= r.limit {
		return 0, 0, false
	}
	return r.data[r.pos], r.data[r.pos+1], true
}

// matchBackslashOp checks if the next two bytes are 0x5C followed by an op
// byte in [lo, hi]. Returns the op byte and true if matched (and consumes
// both bytes); otherwise returns 0, false and does NOT advance.
func (r *Reader) matchBackslashOp(lo, hi byte) (byte, bool) {
	b0, b1, ok := r.peek2()
	if !ok {
		return 0, false
	}
	if b0 != 0x5c {
		return 0, false
	}
	if b1 < lo || b1 > hi {
		return 0, false
	}
	r.pos += 2
	return b1, true
}

// getExprBool — OCaml get_expr_bool (disassembler.ml L512).
// Handles "\<" (0x5C 0x3C) for && and "\=" (0x5C 0x3D) for ||.
func (r *Reader) getExprBool() (string, error) {
	left, err := r.getExprCond()
	if err != nil {
		return "", err
	}
	for {
		op, ok := r.matchBackslashOp(0x3c, 0x3d)
		if !ok {
			return left, nil
		}
		right, err := r.getExprCond()
		if err != nil {
			return "", err
		}
		switch op {
		case 0x3c:
			left = left + " && " + right
		case 0x3d:
			left = left + " || " + right
		}
	}
}

// getExprCond — OCaml get_expr_cond (disassembler.ml L496).
// Handles "\(" (0x28) "==", "\)" (0x29) "!=", etc., all 0x5C-prefixed.
func (r *Reader) getExprCond() (string, error) {
	left, err := r.getExprArith()
	if err != nil {
		return "", err
	}
	for {
		op, ok := r.matchBackslashOp(0x28, 0x2d)
		if !ok {
			return left, nil
		}
		right, err := r.getExprArith()
		if err != nil {
			return "", err
		}
		var s string
		switch op {
		case 0x28:
			s = "=="
		case 0x29:
			s = "!="
		case 0x2a:
			s = "<="
		case 0x2b:
			s = "<"
		case 0x2c:
			s = ">="
		case 0x2d:
			s = ">"
		}
		left = left + " " + s + " " + right
	}
}

// op_string mapping shared between assignment and arithmetic.
// Index by (op_byte - 0x14) for assignment, or directly by op_byte for arith.
// OCaml: let op_string = [| "+"; "-"; "*"; "/"; "%"; "&"; "|"; "^"; "<<"; ">>"; "" |]
var opStringTable = [11]string{
	"+", "-", "*", "/", "%", "&", "|", "^", "<<", ">>", "",
}

// getExprArith — OCaml get_expr_arith (disassembler.ml L471).
// Two-pass for precedence:
//   - high prec ops "\*" through "\>>" (0x02-0x09)
//   - low prec ops "\+" / "\-"          (0x00-0x01)
func (r *Reader) getExprArith() (string, error) {
	loopHi := func(left string) (string, error) {
		for {
			op, ok := r.matchBackslashOp(0x02, 0x09)
			if !ok {
				return left, nil
			}
			right, err := r.getExprTerm()
			if err != nil {
				return "", err
			}
			left = left + " " + opStringTable[op] + " " + right
		}
	}
	term, err := r.getExprTerm()
	if err != nil {
		return "", err
	}
	left, err := loopHi(term)
	if err != nil {
		return "", err
	}
	for {
		op, ok := r.matchBackslashOp(0x00, 0x01)
		if !ok {
			return left, nil
		}
		rhsTerm, err := r.getExprTerm()
		if err != nil {
			return "", err
		}
		rhs, err := loopHi(rhsTerm)
		if err != nil {
			return "", err
		}
		left = left + " " + opStringTable[op] + " " + rhs
	}
}

// getExprTerm — OCaml get_expr_term (disassembler.ml L450).
// Valid term starts:
//   - "$"          → variable token (handled via get_expr_token)
//   - "\\\000"     → unary plus (ignored, just descends)
//   - "\\\001"     → unary minus
//   - "("          → parenthesized expression
//
// IMPORTANT: 0xff (immediate int) and 0xc8 (store) are NOT top-level term
// starts — they live inside the get_expr_token reader, which is invoked
// after the leading "$" via readIntVar.
func (r *Reader) getExprTerm() (string, error) {
	b, err := r.Peek()
	if err != nil {
		return "", err
	}

	switch b {
	case '$':
		r.Next()
		return r.readExprToken()

	case '(':
		r.Next()
		expr, err := r.GetExpression()
		if err != nil {
			return "", err
		}
		if perr := r.Expect(')', "getExprTerm/paren"); perr != nil {
			// continue best-effort
			_ = perr
		}
		return "(" + expr + ")", nil

	case 0x5c:
		// Two-byte form: 0x5C 0x00 (unary plus, ignored) or 0x5C 0x01 (unary minus).
		_, b1, ok := r.peek2()
		if !ok {
			return "", fmt.Errorf("EOF after 0x5c at expr term offset 0x%x", r.pos)
		}
		switch b1 {
		case 0x00:
			r.pos += 2
			return r.getExprTerm()
		case 0x01:
			r.pos += 2
			term, err := r.getExprTerm()
			if err != nil {
				return "", err
			}
			return "-" + term, nil
		default:
			return "", fmt.Errorf("unexpected 0x5c %02x in expr term at offset 0x%x", b1, r.pos)
		}

	default:
		return "", fmt.Errorf("expected $/\\/( in expr term, got 0x%02x at offset 0x%x", b, r.pos)
	}
}

// readExprToken — OCaml get_expr_token (disassembler.ml L431).
// Called after the leading "$" has been consumed.
//   - 0xff → immediate int32 (4 bytes follow)
//   - 0xc8 → "store"
//   - any other byte (≠ 0xff, ≠ 0xc8) followed by '[' → variable reference
func (r *Reader) readExprToken() (string, error) {
	b, err := r.Next()
	if err != nil {
		return "", err
	}
	switch b {
	case 0xff:
		v, err := r.ReadInt32()
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%d", v), nil
	case 0xc8:
		return "store", nil
	default:
		// Variable: var_type byte then '[' expr ']'
		return r.readVarBody(b)
	}
}

// readVarBody reads the body of a variable reference: '[' expr ']'.
// The first byte (varType) has already been consumed by the caller.
//
// OCaml reference: variable_name (disassembler.ml ~L1100, full table).
// Both integer AND string variables are decoded here. The OCaml table
// distinguishes integer-prefixed (intA, intZ, ...) and string-prefixed
// (strK, strL, strM, strS) variables by varType byte.
func (r *Reader) readVarBody(varType byte) (string, error) {
	// Read array index expression enclosed in [ ]
	if err := r.Expect('[', "readVarBody"); err != nil {
		// Continue best-effort
		_ = err
	}

	idx, err := r.GetExpression()
	if err != nil {
		return "", err
	}

	if err := r.Expect(']', "readVarBody"); err != nil {
		// Continue best-effort
		_ = err
	}

	return varPrefix(varType) + "[" + idx + "]", nil
}

// varPrefix maps a variable-type byte to its kepago prefix.
//
// Faithful translation of OCaml variable_name decode table (disassembler.ml
// L1102-L1124 in Jérémy's fork).  Note: 0x0a = strK, 0x0c = strM, 0x12 = strS
// are STRING variables; everything else is integer.  Bit-width suffixes:
//   - bare      → 32-bit
//   - "b"       → 8-bit  (banks 0x1a-0x33)
//   - "2b"      → 16-bit (banks 0x34-0x4d)
//   - "4b"      → 32-bit (banks 0x4e-0x67)
//   - "8b"      → 64-bit (banks 0x68-0x81)
func varPrefix(t byte) string {
	switch t {
	// String variables (Config.svar_prefix in OCaml = "str")
	case 0x0a:
		return "strK"
	case 0x0c:
		return "strM"
	case 0x12:
		return "strS"

	// 32-bit integer banks (Config.ivar_prefix = "int")
	case 0x00:
		return "intA"
	case 0x01:
		return "intB"
	case 0x02:
		return "intC"
	case 0x03:
		return "intD"
	case 0x04:
		return "intE"
	case 0x05:
		return "intF"
	case 0x06:
		return "intG"
	case 0x0b:
		return "intL"
	case 0x19:
		return "intZ"

	// 8-bit integer banks
	case 0x1a:
		return "intAb"
	case 0x1b:
		return "intBb"
	case 0x1c:
		return "intCb"
	case 0x1d:
		return "intDb"
	case 0x1e:
		return "intEb"
	case 0x1f:
		return "intFb"
	case 0x20:
		return "intGb"
	case 0x33:
		return "intZb"

	// 16-bit integer banks
	case 0x34:
		return "intA2b"
	case 0x35:
		return "intB2b"
	case 0x36:
		return "intC2b"
	case 0x37:
		return "intD2b"
	case 0x38:
		return "intE2b"
	case 0x39:
		return "intF2b"
	case 0x3a:
		return "intG2b"
	case 0x4d:
		return "intZ2b"

	// 32-bit (split-bank) integer banks
	case 0x4e:
		return "intA4b"
	case 0x4f:
		return "intB4b"
	case 0x50:
		return "intC4b"
	case 0x51:
		return "intD4b"
	case 0x52:
		return "intE4b"
	case 0x53:
		return "intF4b"
	case 0x54:
		return "intG4b"
	case 0x67:
		return "intZ4b"

	// 64-bit integer banks
	case 0x68:
		return "intA8b"
	case 0x69:
		return "intB8b"
	case 0x6a:
		return "intC8b"
	case 0x6b:
		return "intD8b"
	case 0x6c:
		return "intE8b"
	case 0x6d:
		return "intF8b"
	case 0x6e:
		return "intG8b"
	case 0x81:
		return "intZ8b"

	default:
		return fmt.Sprintf("VAR%02x", t)
	}
}

// GetData reads a "data" element — either a string or an expression.
//
// OCaml reference: get_data (disassembler.ml L1383, equivalent in fork).
// Dispatch by next byte:
//   - ','           → consumed (debug separator) and recurse
//   - '\n' + 2/4 b  → consumed (debug line marker) and recurse
//   - [A-Z 0-9 ? _ "] | sjs1 | "###PRINT(" → roll back, parse string
//   - 'a' xx        → __special[xx](args)  (rare, mostly unused)
//   - anything else → roll back, parse expression
func (r *Reader) GetData() (string, error) {
	return r.GetDataSep(false)
}

// GetDataSep is the underlying get_data with the sep_str flag from OCaml.
// sep_str=true means "this string can be moved to the resource file".
func (r *Reader) GetDataSep(sepStr bool) (string, error) {
	for {
		b, err := r.Peek()
		if err != nil {
			return "", err
		}

		switch {
		case b == ',':
			r.Next()
			continue

		case b == '\n':
			// '\n' followed by debug-line int (size depends on engine mode)
			r.Next()
			if r.mode == ModeAvg2000 {
				_, _ = r.ReadInt32()
			} else {
				_, _ = r.ReadUint16()
			}
			continue

		case b == 'a':
			// 0x61 NN can appear in two contexts:
			//
			//  - General `__special[NN](args)` form: a 'a' index byte
			//    followed by a paren block whose contents are read as
			//    repeated get_data calls. OCaml: disassembler.ml L1377.
			//
			//  - Inline tagged form: 0x61 NN immediately followed by
			//    one or more parameter values (no parens). This appears
			//    in `special(0:#{intC}, 1:#{strC})+` prototypes used by
			//    farcall_with, gosub_with, GOSUBP, etc.
			//    See OCaml disassembler.ml L2274-2304 (read_soft_function /
			//    read_complex_param). The tag selects which sub-prototype
			//    is in play; the parameters that follow are emitted bare.
			//
			// Without proto information here we can't tell how many
			// parameters belong to the tag — for the inline form we read
			// a single expression after `0x61 NN` and render it as
			// `special<NN>(expr)`. The outer readFuncArgsWithProto loop
			// keeps reading more entries until it hits ')'. That mirrors
			// the layout in the bytecode (each entry self-terminates at
			// its own expression boundary).
			r.Next()
			idx, err := r.Next()
			if err != nil {
				return "", err
			}
			// Peek next byte to decide form.
			nb, perr := r.Peek()
			if perr != nil {
				return fmt.Sprintf("special<%d>", idx), nil
			}
			if nb == '(' {
				// Parenthesised form.
				r.Next() // consume '('
				var sb strings.Builder
				fmt.Fprintf(&sb, "__special[%d](", idx)
				first := true
				for !r.AtEnd() {
					bb, err := r.Peek()
					if err != nil {
						break
					}
					if bb == ')' {
						r.Next()
						break
					}
					if !first {
						sb.WriteString(", ")
					}
					inner, err := r.GetData()
					if err != nil {
						return "", err
					}
					sb.WriteString(inner)
					first = false
				}
				sb.WriteByte(')')
				return sb.String(), nil
			}
			// Inline form: read a single expression / string argument.
			// The outer arg loop continues to read further entries.
			inner, err := r.GetDataSep(sepStr)
			if err != nil {
				return fmt.Sprintf("special<%d>", idx), nil
			}
			return fmt.Sprintf("special<%d>(%s)", idx, inner), nil

		case b == '"' || isAsciiStringStart(b) || isShiftJISLead(b):
			// Roll back nothing — readStringUnquot starts at this byte.
			return r.readStringUnquot(sepStr)

		default:
			// Expression
			return r.GetExpression()
		}
	}
}

// isAsciiStringStart returns true if the byte starts a bare unquoted
// string in the OCaml grammar — that is one of [A-Z 0-9 ? _].
// Lowercase letters are NOT included on purpose.
func isAsciiStringStart(b byte) bool {
	return (b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '?' || b == '_'
}

// readStringUnquot — OCaml get_string (disassembler.ml L1313).
//
// Strings in RealLive bytecode use a peculiar two-mode encoding:
//
//   - "unquot" mode (default outside double-quotes):
//       collects [A-Z 0-9 ? _ sjs-double-byte] runs as raw text;
//       handles "###PRINT(expr)" → \i{expr} or \s{expr};
//       a '"' switches to quot mode;
//       any other byte ends the string (rolled back for caller).
//
//   - "quot" mode (after seeing a '"'):
//       \" → '"' literal in output;
//       \  → "\\" (escape backslash);
//       '  → "'" or "\'" depending on sep_str;
//       a closing '"' returns to unquot mode;
//       sjs1 lead-byte + any → 2-byte SJIS char;
//       anything else → raw byte.
//
// The final string is wrapped in single quotes when sep_str is false
// (inline argument mode). When sep_str is true the caller intends to
// move the string to a resource file and handles the quoting itself.
func (r *Reader) readStringUnquot(sepStr bool) (string, error) {
	var b strings.Builder

unquotLoop:
	for {
		c, err := r.Peek()
		if err != nil {
			break
		}

		switch {
		case c == '"':
			// Enter quot mode
			r.Next()
			if !r.readStringQuot(&b, sepStr) {
				break unquotLoop
			}
			continue

		case c == '#':
			// Maybe "###PRINT("
			if r.matchLiteral("###PRINT(") {
				expr, err := r.GetExpression()
				if err != nil {
					return b.String(), err
				}
				_ = r.Expect(')', "readStringUnquot/print")
				kind := 'i'
				if len(expr) > 0 && expr[0] == 's' {
					kind = 's'
				}
				fmt.Fprintf(&b, "\\%c{%s}", kind, expr)
				continue
			}
			// '#' alone is a command boundary — let caller handle.
			break unquotLoop

		case isAsciiStringStart(c):
			r.Next()
			b.WriteByte(c)

		case isShiftJISLead(c):
			// 2-byte SJIS character
			r.Next()
			c2, err := r.Next()
			if err != nil {
				b.WriteByte(c)
				break unquotLoop
			}
			b.WriteByte(c)
			b.WriteByte(c2)

		default:
			break unquotLoop
		}
	}

	if sepStr {
		// OCaml force_textout / get_string with sep_str=true:
		// push to the resource list and return a #res<NNNN> token.
		// (disassembler.ml L1486 force_textout, L1361 get_string body)
		s := b.String()
		if r.result != nil {
			idx := len(r.result.ResStrs)
			r.result.ResStrs = append(r.result.ResStrs, s)
			return fmt.Sprintf("#res<%04d>", idx), nil
		}
		// No collection target attached — fall through to inline.
		return "'" + s + "'", nil
	}

	s := b.String()

	// OCaml separate-all rule (disassembler.ml L1361):
	//   if separate_strings && separate_all && first byte is sjs1
	//   then push the string to the resource list and return "#res<NNNN>".
	if r.opts != nil && r.opts.SeparateStrings && r.opts.SeparateAll &&
		len(s) > 0 && isShiftJISLead(s[0]) && r.result != nil {
		idx := len(r.result.ResStrs)
		r.result.ResStrs = append(r.result.ResStrs, s)
		return fmt.Sprintf("#res<%04d>", idx), nil
	}

	// Inline arg: wrap in single quotes like OCaml get_string L1368.
	return "'" + s + "'", nil
}

// readStringQuot — OCaml's quot inner lexer (disassembler.ml L1318).
// Reads bytes until a closing '"' or EOF. Returns true to continue in
// unquot mode after the close quote, false on EOF.
func (r *Reader) readStringQuot(b *strings.Builder, sepStr bool) bool {
	for {
		c, err := r.Next()
		if err != nil {
			return false
		}

		switch c {
		case '\\':
			// Escape: \" → ", everything else → \\
			peek, err := r.Peek()
			if err == nil && peek == '"' {
				r.Next()
				b.WriteByte('"')
			} else {
				b.WriteString("\\\\")
			}

		case '\'':
			if sepStr {
				b.WriteByte('\'')
			} else {
				b.WriteString("\\'")
			}

		case '"':
			// End of quot mode → back to unquot.
			return true

		default:
			if isShiftJISLead(c) {
				c2, err := r.Next()
				if err != nil {
					b.WriteByte(c)
					return false
				}
				b.WriteByte(c)
				b.WriteByte(c2)
			} else {
				b.WriteByte(c)
			}
		}
	}
}

// matchLiteral checks if the next bytes match the given literal string.
// Consumes the bytes if matched; otherwise leaves position unchanged.
func (r *Reader) matchLiteral(s string) bool {
	if r.pos+len(s) > r.limit {
		return false
	}
	for i := 0; i < len(s); i++ {
		if r.data[r.pos+i] != s[i] {
			return false
		}
	}
	r.pos += len(s)
	return true
}

// --- Main disassembly loop ---

// DisassemblyResult holds the output of disassembly.
type DisassemblyResult struct {
	Commands  []Command
	ResStrs   []string     // Resource strings
	Pointers  map[int]bool // Set of pointer targets (for labels)
	Mode      EngineMode
	Version   Version
	Header    bytecode.FileHeader
	Error     string
	SeenMap   *SeenMap
}

// Disassemble performs bytecode disassembly on the given data.
func Disassemble(arr *binarray.Buffer, opts Options) (*DisassemblyResult, error) {
	// Read header
	hdr, err := bytecode.ReadFullHeader(arr, true)
	if err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	// Determine mode
	mode := opts.ForcedTarget
	if mode == ModeNone {
		if hdr.HeaderVersion == 1 {
			mode = ModeAvg2000
		} else {
			mode = ModeRealLive
		}
	}

	// Determine version
	var version Version
	// TODO: read from metadata or command line

	if mode == ModeAvg2000 {
		version = Version{1, 0, 0, 0}
	} else {
		version = Version{1, 2, 7, 0}
	}

	// Calculate bounds
	startAddr := hdr.DataOffset
	if opts.StartAddress > hdr.DataOffset && opts.StartAddress < arr.Len() {
		startAddr = opts.StartAddress
	}

	endAddr := arr.Len()
	if opts.EndAddress > 0 && opts.EndAddress < arr.Len() {
		if opts.EndAddress > startAddr {
			endAddr = opts.EndAddress
		}
	}

	reader := NewReader(arr.Data, startAddr, endAddr, mode)

	result := &DisassemblyResult{
		Mode:     mode,
		Version:  version,
		Header:   hdr,
		Pointers: make(map[int]bool),
		SeenMap:  NewSeenMap(),
	}

	// Hook the reader so string helpers can route Japanese strings into
	// the resource list when separate_all is on (OCaml L1361-L1366).
	reader.SetContext(result, &opts)

	// Main disassembly loop
	for !reader.AtEnd() {
		cmdOffset := reader.RelPos()

		err := readCommand(reader, &hdr, result, opts)
		if err != nil {
			result.Error = fmt.Sprintf("disassembly error at offset 0x%06x: %v", cmdOffset+startAddr, err)
			if opts.Verbose > 0 {
				fmt.Printf("Warning: %s\n", result.Error)
			}
			break
		}
	}

	return result, nil
}

// readCommand reads one command from the bytecode stream.
func readCommand(r *Reader, hdr *bytecode.FileHeader, result *DisassemblyResult, opts Options) error {
	offset := r.RelPos()

	b, err := r.Next()
	if err != nil {
		return err
	}

	switch {
	case b == 0x00:
		// halt
		cmd := Command{Offset: offset, IsJmp: true}
		cmd.Kepago = []CommandElem{ElemString{Value: "halt"}}
		result.Commands = append(result.Commands, cmd)

	case b == '#':
		// Function call: '#' type module func(16) argc(16) overload
		opType, err := r.Next()
		if err != nil {
			return err
		}
		opModule, err := r.Next()
		if err != nil {
			return err
		}
		funcNum, err := r.GetInt16()
		if err != nil {
			return err
		}
		argc, err := r.GetInt16()
		if err != nil {
			return err
		}
		overload, err := r.Next()
		if err != nil {
			return err
		}

		op := Opcode{
			Type:     int(opType),
			Module:   int(opModule),
			Function: funcNum,
			Overload: int(overload),
		}

		return readFunction(r, result, offset, op, argc, opts)

	case b == '$':
		// Assignment
		return readAssignment(r, result, offset)

	case b == '\n':
		// Debug line number
		lineNum, err := r.GetIntForMode()
		if err != nil {
			return err
		}
		cmd := Command{
			Offset: offset,
			Hidden: !opts.ReadDebugSymbols,
			CType:  "dbline",
			LineNo: lineNum,
		}
		cmd.Kepago = []CommandElem{ElemString{Value: fmt.Sprintf("#line %d", lineNum)}}
		result.Commands = append(result.Commands, cmd)

	case b == ',':
		// Debug separator
		cmd := Command{
			Offset: offset,
			Hidden: !opts.ReadDebugSymbols,
			CType:  "debug",
		}
		cmd.Kepago = []CommandElem{ElemString{Value: ","}}
		result.Commands = append(result.Commands, cmd)

	case b == '@' || b == '!':
		// Kidoku flag marker / entrypoint
		if b == '!' {
			opts.UsesExclKidoku = true
		}
		idx, err := r.GetIntForMode()
		if err != nil {
			return err
		}

		cmd := Command{Offset: offset}

		// Check if this is an entrypoint
		kidokuVal := int32(0)
		if idx >= 0 && idx < len(hdr.KidokuLnums) {
			kidokuVal = hdr.KidokuLnums[idx]
		}

		entryIdx := kidokuVal - 1_000_000
		if entryIdx >= 0 {
			cmd.Unhide = true
			cmd.CType = "entrypoint"
			cmd.Kepago = []CommandElem{ElemString{
				Value: fmt.Sprintf("#entrypoint %03d", entryIdx),
			}}
			result.SeenMap.EntryPoints = append(result.SeenMap.EntryPoints, offset)
		} else {
			cmd.Hidden = !opts.ReadDebugSymbols
			cmd.CType = "kidoku"
			cmd.Kepago = []CommandElem{ElemString{
				Value: fmt.Sprintf("{- kidoku %03d -}", idx),
			}}
		}
		result.Commands = append(result.Commands, cmd)

	default:
		// Text output - rollback and read text
		r.Rollback(1)
		return readTextout(r, result, offset, opts)
	}

	return nil
}

// readFunction handles a function call opcode.
//
// OCaml reference: read_function (disassembler.ml L2507 in fork).
// Most opcodes are handled via the KFN registry's per-function parameter
// signatures (see read_soft_function). This Go port hard-codes the most
// common control-flow opcodes (goto, gosub, conditionals, ret) which all
// have parameter shapes that don't match the generic argc-based reader.
func readFunction(r *Reader, result *DisassemblyResult, offset int, op Opcode, argc int, opts Options) error {
	cmd := Command{Offset: offset, Opcode: op.String()}

	// Special-case control-flow opcodes BEFORE generic arg parsing — they
	// have an out-of-band pointer (4-byte int) inside the parens that argc
	// doesn't account for. Letting readFuncArgs near them desyncs the
	// reader for the rest of the file.
	//
	// Module/function numbers come from reallive.kfn:
	//   module 001 = Jmp (flow control)
	//     fn  0  goto         <0:Jmp:00000, 0>
	//     fn  1  goto_if      <0:Jmp:00001, 0> (<'condition')
	//     fn  2  goto_unless  <0:Jmp:00002, 0> (<'condition')
	//     fn  3  goto_on        ('value') { label+ }
	//     fn  4  goto_case      ('value') { case+ }
	//     fn  5  gosub        <0:Jmp:00005, 0>
	//     fn  6  gosub_if     <0:Jmp:00006, 0>
	//     fn  7  gosub_unless <0:Jmp:00007, 0>
	//     fn  8  gosub_on
	//     fn  9  gosub_case
	//     fn 10  ret          <0:Jmp:00010, 0>
	//     fn 11  jump         <0:Jmp:00011, 1>
	//     fn 12  farcall      <0:Jmp:00012, 1>
	switch {
	case op.Module == 1 && (op.Function == 0 || op.Function == 5):
		// goto / gosub: 4-byte pointer follows directly.
		return readGotoLike(r, result, &cmd, op, opts)

	case op.Module == 1 && (op.Function == 1 || op.Function == 2 ||
		op.Function == 6 || op.Function == 7):
		// goto_if / goto_unless / gosub_if / gosub_unless:
		// '(' expr ')' then 4-byte pointer.
		return readCondGotoLike(r, result, &cmd, op, opts)

	case op.Module == 1 && (op.Function == 3 || op.Function == 8):
		// goto_on / gosub_on: '(' expr ')' '{' ptr×argc '}'
		return readGotoOnLike(r, result, &cmd, op, argc, opts)

	case op.Module == 1 && (op.Function == 4 || op.Function == 9):
		// goto_case / gosub_case: '(' expr ')' '{' ('('case')'|'()') ptr ×argc '}'
		return readGotoCaseLike(r, result, &cmd, op, argc, opts)

	case op.Module == 1 && op.Function == 10:
		// ret: zero args, no parens.
		cmd.Kepago = []CommandElem{ElemString{Value: "ret"}}
		cmd.IsJmp = true
		result.Commands = append(result.Commands, cmd)
		return nil

	case op.Module == 10 && (op.Function == 0 || op.Function == 2) && op.Overload == 0:
		// Strcpy / Strcat short form. OCaml disassembler.ml L2537-L2548:
		//   overload 0:
		//     <10:0,0> (a, b) → "a = b"  (strcpy short form)
		//     <10:2,0> (a, b) → "a += b" (strcat short form)
		//   overload 1: full form "strcpy(a, b, n)" / "strcat(a, b, n)"
		return readStrAssign(r, result, &cmd, op, opts)
	}

	// Generic path: KFN-or-fallback.
	// Look up the prototype so we know which parameters are typed as
	// ResStr and need sep_str=true (resource-routed) when read.
	var proto []ParamType
	if opts.FuncReg != nil {
		if def, ok := opts.FuncReg.LookupOpcode(op); ok {
			if len(def.Prototypes) > 0 {
				proto = def.Prototypes[0]
			}
		}
	}
	// Hand the resource-collection target to the reader so it can
	// allocate `#res<NNNN>` indices when sep_str routing kicks in.
	r.result = result
	r.opts = &opts

	args, err := readFuncArgsWithProto(r, argc, proto)
	if err != nil {
		// Best-effort: emit with name if known.
		funcName := opcodeDisplay(op, opts.FuncReg)
		if opts.FuncReg != nil {
			if def, ok := opts.FuncReg.LookupOpcode(op); ok {
				funcName = def.Name
			}
		}
		cmd.Kepago = []CommandElem{ElemString{Value: fmt.Sprintf("%s(?)", funcName)}}
		result.Commands = append(result.Commands, cmd)
		return nil
	}

	funcName := ""
	var hasPushStore bool
	if opts.FuncReg != nil {
		if def, ok := opts.FuncReg.LookupOpcode(op); ok {
			funcName = def.Name
			hasPushStore = def.HasFlag(FlagPushStore)
		}
	}
	var sb strings.Builder
	if funcName != "" {
		sb.WriteString(funcName)
	} else {
		sb.WriteString(opcodeDisplay(op, opts.FuncReg))
	}
	if len(args) > 0 {
		// OCaml emits "fn (arg1, arg2)" with a space before '(' for
		// readability. Match that style.
		sb.WriteString(" (")
		for i, arg := range args {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(arg)
		}
		sb.WriteByte(')')
	}

	// Build the command. When the function has PushStore, prepend an
	// empty ElemStore marker so a follow-up "dst = store" assignment
	// can fold into this command (matching OCaml STORE token).
	if hasPushStore {
		cmd.Kepago = []CommandElem{ElemStore{Value: ""}, ElemString{Value: sb.String()}}
	} else {
		cmd.Kepago = []CommandElem{ElemString{Value: sb.String()}}
	}
	result.Commands = append(result.Commands, cmd)
	return nil
}

// readStrAssign handles the Strcpy/Strcat short form (overload 0).
//
// OCaml reference: disassembler.ml L2537-L2548. With overload=0 the
// function call has exactly 2 args (dst, src) and is rendered as a
// kepago-style assignment for readability:
//
//	<10:Str:0,0>(strS[i], 'foo')  → strS[i] = 'foo'
//	<10:Str:2,0>(strS[i], 'bar')  → strS[i] += 'bar'
//
// Overload 1 is the 3-arg form `strcpy(a, b, n)` / `strcat(a, b, n)`,
// handled by the generic KFN path.
func readStrAssign(r *Reader, result *DisassemblyResult, cmd *Command, op Opcode, opts Options) error {
	if err := r.Expect('(', "readStrAssign/open"); err != nil {
		return err
	}
	a, err := r.GetData()
	if err != nil {
		return err
	}
	b, err := r.GetData()
	if err != nil {
		return err
	}
	if err := r.Expect(')', "readStrAssign/close"); err != nil {
		return err
	}

	op2 := "="
	if op.Function == 2 {
		op2 = "+="
	}
	cmd.Kepago = []CommandElem{ElemString{
		Value: fmt.Sprintf("%s %s %s", a, op2, b),
	}}
	result.Commands = append(result.Commands, *cmd)
	return nil
}
// `op<type:Module:NNNNN, overload>` form, using the symbolic module name
// when the registry knows it ("Sys", "Jmp", "Bgm", …) and the numeric
// 3-digit form ("004") otherwise.
//
// OCaml reference: string_of_opcode (used in disassembler.ml L2157, etc.).
func opcodeDisplay(op Opcode, reg *FuncRegistry) string {
	mod := fmt.Sprintf("%03d", op.Module)
	if reg != nil {
		mod = reg.ModuleName(op.Module)
	}
	return fmt.Sprintf("op<%d:%s:%05d, %d>", op.Type, mod, op.Function, op.Overload)
}

// readGotoOnLike reads goto_on / gosub_on: '(' expr ')' '{' int32×argc '}'.
//
// OCaml reference: read_goto_on (disassembler.ml L1815).
// Each case is just a 4-byte pointer; argc tells how many. The ordering
// indexes by the value of the expression.
func readGotoOnLike(r *Reader, result *DisassemblyResult, cmd *Command, op Opcode, argc int, opts Options) error {
	name := "goto_on"
	cmd.IsJmp = false
	if op.Function == 8 {
		name = "gosub_on"
	}

	if err := r.Expect('(', "readGotoOnLike/expr-open"); err != nil {
		return err
	}
	expr, err := r.GetExpression()
	if err != nil {
		return err
	}
	if err := r.Expect(')', "readGotoOnLike/expr-close"); err != nil {
		return err
	}
	if err := r.Expect('{', "readGotoOnLike/brace-open"); err != nil {
		return err
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s (%s) {", name, expr)
	for i := 0; i < argc; i++ {
		target, err := r.ReadInt32()
		if err != nil {
			return err
		}
		if result.Pointers != nil {
			result.Pointers[int(target)] = true
		}
		if i > 0 {
			sb.WriteString("; ")
		} else {
			sb.WriteByte(' ')
		}
		fmt.Fprintf(&sb, "@@PTR=%d@@", target)
	}
	sb.WriteString(" }")

	if err := r.Expect('}', "readGotoOnLike/brace-close"); err != nil {
		return err
	}

	cmd.Kepago = []CommandElem{ElemString{Value: sb.String()}}
	result.Commands = append(result.Commands, *cmd)
	return nil
}

// readGotoCaseLike reads goto_case / gosub_case:
//   '(' expr ')' '{' [ '(' case-expr ')' | '()' ] int32 ×argc '}'
//
// OCaml reference: read_goto_case (disassembler.ml L1796).
// Each case is either a value-bearing case `(case-expr) ptr` or a default
// case `() ptr`. The expression is the value being switched on.
func readGotoCaseLike(r *Reader, result *DisassemblyResult, cmd *Command, op Opcode, argc int, opts Options) error {
	name := "goto_case"
	cmd.IsJmp = false
	if op.Function == 9 {
		name = "gosub_case"
	}

	if err := r.Expect('(', "readGotoCaseLike/expr-open"); err != nil {
		return err
	}
	expr, err := r.GetExpression()
	if err != nil {
		return err
	}
	if err := r.Expect(')', "readGotoCaseLike/expr-close"); err != nil {
		return err
	}
	if err := r.Expect('{', "readGotoCaseLike/brace-open"); err != nil {
		return err
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s (%s) {", name, expr)

	for i := 0; i < argc; i++ {
		// '()' default case OR '(' case-expr ')'.
		b0, b1, ok := r.peek2()
		if !ok {
			return fmt.Errorf("EOF in goto_case body")
		}
		var caseLabel string
		if b0 == '(' && b1 == ')' {
			r.pos += 2
			caseLabel = "_"
		} else if b0 == '(' {
			r.Next()
			caseExpr, err := r.GetExpression()
			if err != nil {
				return err
			}
			if err := r.Expect(')', "readGotoCaseLike/case-close"); err != nil {
				return err
			}
			caseLabel = caseExpr
		} else {
			return fmt.Errorf("expected '(' in goto_case body, got 0x%02x at offset 0x%x", b0, r.pos)
		}

		target, err := r.ReadInt32()
		if err != nil {
			return err
		}
		if result.Pointers != nil {
			result.Pointers[int(target)] = true
		}
		if i > 0 {
			sb.WriteString("; ")
		} else {
			sb.WriteByte(' ')
		}
		fmt.Fprintf(&sb, "%s: @@PTR=%d@@", caseLabel, target)
	}
	sb.WriteString(" }")

	if err := r.Expect('}', "readGotoCaseLike/brace-close"); err != nil {
		return err
	}

	cmd.Kepago = []CommandElem{ElemString{Value: sb.String()}}
	result.Commands = append(result.Commands, *cmd)
	return nil
}

// readGotoLike reads the bytecode shape for an unconditional jump:
//   header already consumed; a 4-byte LE int32 pointer follows directly
//   (no parens, no argc — the pointer is added by the IsGoto flag in the
//   KFN).
//
// OCaml reference: read_soft_function L2356:
//
//	if List.mem IsGoto fndef.fn_flags
//	  then [pointer (get_int lexbuf)]
func readGotoLike(r *Reader, result *DisassemblyResult, cmd *Command, op Opcode, opts Options) error {
	name := "goto"
	cmd.IsJmp = true
	if op.Function == 5 {
		name = "gosub"
		cmd.IsJmp = false
	}
	target, err := r.ReadInt32()
	if err != nil {
		return err
	}
	if result.Pointers != nil {
		result.Pointers[int(target)] = true
	}
	cmd.Kepago = []CommandElem{ElemString{Value: fmt.Sprintf("%s @@PTR=%d@@", name, target)}}
	result.Commands = append(result.Commands, *cmd)
	return nil
}

// readCondGotoLike reads a conditional goto/gosub.
// Format: '(' condition-expr ')' int32-pointer.
//
// OCaml reference: when fn_flags contains IsCond/IsNeg, the soft function
// reader emits "goto_if (cond)" or "goto_unless (cond)" depending on IsNeg
// and then appends the pointer. The condition expression is wrapped in
// parens in the bytecode and ALSO in the source output. As a
// kepago-readability touch, OCaml prints "!expr" instead of "expr == 0".
func readCondGotoLike(r *Reader, result *DisassemblyResult, cmd *Command, op Opcode, opts Options) error {
	var name string
	switch op.Function {
	case 1:
		name = "goto_if"
	case 2:
		name = "goto_unless"
	case 6:
		name = "gosub_if"
	case 7:
		name = "gosub_unless"
	}

	if err := r.Expect('(', "readCondGotoLike/cond-open"); err != nil {
		return err
	}
	cond, err := r.GetExpression()
	if err != nil {
		return err
	}
	if err := r.Expect(')', "readCondGotoLike/cond-close"); err != nil {
		return err
	}
	target, err := r.ReadInt32()
	if err != nil {
		return err
	}
	if result.Pointers != nil {
		result.Pointers[int(target)] = true
	}
	cmd.Kepago = []CommandElem{ElemString{Value: fmt.Sprintf("%s (%s) @@PTR=%d@@", name, prettifyCond(cond), target)}}
	result.Commands = append(result.Commands, *cmd)
	return nil
}

// prettifyCond rewrites "expr == 0" to "!expr" and "expr != 0" to "expr",
// matching OCaml kprl output style for conditional gotos.
func prettifyCond(s string) string {
	const eqZero = " == 0"
	const neZero = " != 0"
	if strings.HasSuffix(s, eqZero) {
		return "!" + strings.TrimSuffix(s, eqZero)
	}
	if strings.HasSuffix(s, neZero) {
		return strings.TrimSuffix(s, neZero)
	}
	return s
}

// readFuncArgs reads function arguments enclosed in matched parens.
//
// OCaml reference: read_unknown_function (disassembler.ml L2058) and the
// generic loop inside read_soft_function. Both read arguments until they
// see the closing ')', regardless of argc. The argc count is only used as
// a soft sanity check (a warning is emitted when it doesn't match).
//
// Critically, this function tracks paren *depth*: some opcodes (notably
// those with `special(...)` parameters in the KFN, e.g. gosub_with, select)
// embed inner paren groups whose contents are not standard expressions.
// We consume balanced parens while reading the args so the reader stays
// in sync with the bytecode for the next command.
func readFuncArgs(r *Reader, argc int) ([]string, error) {
	return readFuncArgsWithProto(r, argc, nil)
}

// readFuncArgsWithProto is the proto-aware variant. When `proto` is
// non-nil, each parameter at index `i` is read with `sepStr=true` if
// `proto[i] == ParamResStr`, matching OCaml read_soft_function's
// `sep_str:(separate_strings && ptype = ResStr)` (disassembler.ml
// L2314). This routes resource-typed string arguments to the .utf
// resource file regardless of SeparateAll or the leading byte.
func readFuncArgsWithProto(r *Reader, argc int, proto []ParamType) ([]string, error) {
	var args []string

	b, err := r.Peek()
	if err != nil {
		return nil, err
	}
	// No args block at all: return empty.
	if b != '(' {
		return nil, nil
	}
	r.Next() // consume opening '('
	depth := 1

	for !r.AtEnd() {
		b, err := r.Peek()
		if err != nil {
			break
		}

		switch b {
		case ')':
			r.Next()
			depth--
			if depth == 0 {
				if argc > 0 && len(args) != argc {
					// Soft warning only — readers tolerate mismatch.
				}
				return args, nil
			}
			// Closing an inner paren — append it to the last arg if any,
			// or skip silently. Here we just continue.

		case '(':
			// Nested paren group — consume balanced. The contents are read
			// as raw bytes captured into the current arg.
			start := r.Pos()
			r.Next()
			depth++
			// Capture bytes until the matching close at this depth.
			localDepth := 1
			for !r.AtEnd() && localDepth > 0 {
				bb, _ := r.Next()
				switch bb {
				case '(':
					localDepth++
				case ')':
					localDepth--
				}
			}
			depth--
			// Append captured raw bytes as a hex/text snippet for visibility.
			end := r.Pos()
			args = append(args, fmt.Sprintf("/* nested:%d bytes */", end-start))

		case ',':
			// Argument separator — silently consumed in OCaml unquot.
			r.Next()

		default:
			// If we have a prototype, use sep_str=true for ResStr params.
			sepStr := false
			if proto != nil && len(args) < len(proto) && proto[len(args)] == ParamResStr {
				sepStr = true
			}
			arg, err := r.GetDataSep(sepStr)
			if err != nil {
				// Best-effort: stop reading args on error and let the
				// outer loop continue from the current position.
				return args, nil
			}
			args = append(args, arg)
		}
	}

	return args, nil
}

// readAssignment reads a variable assignment ('$' prefix).
//
// OCaml reference: get_assignment (disassembler.ml L1294).
// Format: '$' [varType] '[' expr ']'   ← consumed via readExprToken
//         '\' [0x14-0x1e]              ← assignment operator (2 bytes)
//         <expression>                 ← right-hand side
//
// The op-byte mapping is OCaml's `op_string.(byte - 0x14) ^ "="`:
//   0x14 → "+=",  0x15 → "-=",  0x16 → "*=",  0x17 → "/=",  0x18 → "%=",
//   0x19 → "&=",  0x1a → "|=",  0x1b → "^=",  0x1c → "<<=", 0x1d → ">>=",
//   0x1e → "="    (op_string[10] is empty, so just "=")
//
// IMPORTANT: there is NO trailing 0x5c after the expression. The previous
// implementation consumed an extra byte at the end of every assignment,
// which desynchronized the entire reader and caused the textout chaos.
func readAssignment(r *Reader, result *DisassemblyResult, offset int) error {
	cmd := Command{Offset: offset}

	// '$' is already consumed by readCommand. Now read get_expr_token directly:
	// the next byte is the variable type, then '[' expr ']'.
	dest, err := r.readExprToken()
	if err != nil {
		return err
	}

	// Read 2-byte operator: 0x5c followed by 0x14..0x1e
	opByte, ok := r.matchBackslashOp(0x14, 0x1e)
	if !ok {
		// Try to recover: consume a byte and emit a synthetic op
		b, _ := r.Peek()
		return fmt.Errorf("expected '\\' [0x14-0x1e] in assignment, got 0x%02x at offset 0x%x", b, r.pos)
	}

	op := opStringTable[opByte-0x14] + "="

	// Read source expression (no trailing terminator).
	src, err := r.GetExpression()
	if err != nil {
		src = "???"
	}

	// Store-fold: if the right-hand side is the literal `store`, walk
	// back through previous commands looking for the most recent one
	// whose first element is an ElemStore marker (planted by readFunction
	// when the function has the PushStore flag). If found, replace the
	// marker with "dst op= " and DON'T add a new command.
	//
	// OCaml reference: get_assignment unstored block (disassembler.ml
	// L1300-L1311).
	if src == "store" {
		for i := len(result.Commands) - 1; i >= 0; i-- {
			prev := &result.Commands[i]
			if prev.Hidden {
				continue
			}
			if len(prev.Kepago) == 0 {
				break
			}
			if _, isStore := prev.Kepago[0].(ElemStore); !isStore {
				break
			}
			// Replace ElemStore with the lhs prefix.
			prev.Kepago[0] = ElemString{Value: fmt.Sprintf("%s %s ", dest, op)}
			return nil
		}
		// Fall through: no fold target found; emit normally.
	}

	cmd.Kepago = []CommandElem{ElemString{
		Value: fmt.Sprintf("%s %s %s", dest, op, src),
	}}
	result.Commands = append(result.Commands, cmd)
	return nil
}

// readTextout reads a textout command (raw displayed text).
//
// OCaml reference: read_textout (disassembler.ml L1709 in fork).
// Terminators (consumed at outer call, NOT in the lexer):
//   - 0x00 (halt)
//   - '#'  (function call)
//   - '$'  (assignment)
//   - '\n' (debug line)
//   - '@'  (kidoku flag)
//
// CRITICAL: ',' is NOT a terminator inside textout — it's silently consumed
// in unquot mode. '!' is NOT a terminator either. The previous Go version
// terminated on both, which truncated lines.
//
// The text is collected as raw SJIS bytes and emitted as a resource string;
// the writer takes care of converting to UTF-8 for output.
func readTextout(r *Reader, result *DisassemblyResult, offset int, opts Options) error {
	cmd := Command{Offset: offset, CType: "textout"}

	// SeenEnd shortcut. Some bytecode files end with a fixed SJIS
	// sentinel "ＳｅｅｎＥｎｄ" followed by 0xff padding (typically 32
	// bytes). OCaml disassembler.ml L1786 detects this exact pattern
	// in read_textout and emits a single `eof` marker. We match it at
	// the start of every textout: if found, consume the prefix + all
	// trailing 0xff bytes and emit `eof`.
	if r.matchSeenEnd() {
		cmd.IsJmp = true
		cmd.Kepago = []CommandElem{ElemString{Value: "eof"}}
		result.Commands = append(result.Commands, cmd)
		return nil
	}

	var b strings.Builder
	inQuot := false

textoutLoop:
	for !r.AtEnd() {
		c, err := r.Peek()
		if err != nil {
			break
		}

		if !inQuot {
			// unquot mode: terminators end the textout.
			switch c {
			case 0x00, '#', '$', '\n', '@':
				break textoutLoop
			case '"':
				r.Next()
				inQuot = true
				continue
			case ',':
				// Silently consumed in OCaml unquot.
				r.Next()
				continue
			case '\\':
				r.Next()
				b.WriteString("\\\\")
				continue
			case '\'':
				r.Next()
				if opts.SeparateStrings {
					b.WriteByte('\'')
				} else {
					b.WriteString("\\'")
				}
				continue
			}

			// 2-char "<" or "//" or "{-" — only the special prefix form
			// triggers OCaml's escape; otherwise drop through to the
			// generic SJIS / catchall handler.
			if c == '<' {
				r.Next()
				if opts.SeparateStrings {
					b.WriteString("\\<")
				} else {
					b.WriteByte('<')
				}
				continue
			}
			if c == '/' && r.peekByteAt(1) == '/' {
				r.pos += 2
				if opts.SeparateStrings {
					b.WriteString("\\//")
				} else {
					b.WriteString("//")
				}
				continue
			}
			if c == '{' && r.peekByteAt(1) == '-' {
				r.pos += 2
				if opts.SeparateStrings {
					b.WriteString("{\\-")
				} else {
					b.WriteString("{-")
				}
				continue
			}

			// Lenticulars: 0x81 0x79 → \{ , 0x81 0x7a → }
			if c == 0x81 {
				next := r.peekByteAt(1)
				if next == 0x79 {
					r.pos += 2
					b.WriteString("\\{")
					continue
				}
				if next == 0x7a {
					r.pos += 2
					b.WriteByte('}')
					continue
				}
			}

			// Name markers: 0x81 0x93 → \l (Local Name), 0x81 0x96 → \m (Global Name)
			//   followed by 0x82 [0x60-0x79] (=A-Z, fullwidth)
			//   optionally another 0x82 [0x60-0x79] (=A-Z second char)
			//   optionally a final 0x82 [0x4f-0x58] (= digit 0-9 → indexed)
			//
			// Encoded -> Display:
			//   81 93 82 61                  → \l{A}
			//   81 96 82 60                  → \m{A}
			//   81 96 82 60 82 61            → \m{AA}
			//   81 96 82 60 82 4f            → \m{A, 0}
			//   81 96 82 60 82 61 82 4f      → \m{AA, 0}
			//
			// Reference: kprl/disassembler.ml L1534-1555.
			if c == 0x81 && (r.peekByteAt(1) == 0x93 || r.peekByteAt(1) == 0x96) &&
				r.peekByteAt(2) == 0x82 && r.peekByteAt(3) >= 0x60 && r.peekByteAt(3) <= 0x79 {
				lm := byte('l')
				if r.peekByteAt(1) == 0x96 {
					lm = 'm'
				}
				c1 := r.peekByteAt(3) - 0x1f // 0x60→A=0x41 ... yes: 0x60-0x1f=0x41
				// Optional second letter
				offset := 4
				var c2 byte
				hasC2 := false
				if r.peekByteAt(offset) == 0x82 && r.peekByteAt(offset+1) >= 0x60 && r.peekByteAt(offset+1) <= 0x79 {
					c2 = r.peekByteAt(offset+1) - 0x1f
					hasC2 = true
					offset += 2
				}
				// Optional index (digit suffix 0x82 0x4f-0x58)
				hasIdx := false
				var idx int
				if r.peekByteAt(offset) == 0x82 && r.peekByteAt(offset+1) >= 0x4f && r.peekByteAt(offset+1) <= 0x58 {
					idx = int(r.peekByteAt(offset+1)) - 0x4f
					hasIdx = true
					offset += 2
				}
				r.pos += offset
				if hasIdx {
					if hasC2 {
						fmt.Fprintf(&b, "\\%c{%c%c, %d}", lm, c1, c2, idx)
					} else {
						fmt.Fprintf(&b, "\\%c{%c, %d}", lm, c1, idx)
					}
				} else {
					if hasC2 {
						fmt.Fprintf(&b, "\\%c{%c%c}", lm, c1, c2)
					} else {
						fmt.Fprintf(&b, "\\%c{%c}", lm, c1)
					}
				}
				continue
			}

			// Regular SJIS pair
			if isShiftJISLead(c) {
				r.Next()
				c2, err := r.Next()
				if err != nil {
					b.WriteByte(c)
					break textoutLoop
				}
				b.WriteByte(c)
				b.WriteByte(c2)
				continue
			}

			// Plain ASCII / printable
			r.Next()
			if c >= 0x20 && c < 0x80 {
				b.WriteByte(c)
			} else if opts.ControlCodes {
				fmt.Fprintf(&b, "\\x{%02x}", c)
			} else {
				b.WriteByte(c)
			}

		} else {
			// quot mode: only EOF or '"' exit.
			r.Next()
			switch c {
			case '"':
				inQuot = false
			case '\\':
				peek, err := r.Peek()
				if err == nil && peek == '"' {
					r.Next()
					b.WriteByte('"')
				} else {
					b.WriteString("\\\\")
				}
			case '\'':
				if opts.SeparateStrings {
					b.WriteByte('\'')
				} else {
					b.WriteString("\\'")
				}
			default:
				if isShiftJISLead(c) {
					c2, err := r.Next()
					if err != nil {
						b.WriteByte(c)
						break textoutLoop
					}
					b.WriteByte(c)
					b.WriteByte(c2)
				} else {
					b.WriteByte(c)
				}
			}
		}
	}

	if b.Len() == 0 {
		return nil
	}

	textStr := b.String()

	// SeenEnd marker detection. Some bytecode files end with a fixed
	// SJIS sentinel "ＳｅｅｎＥｎｄ" (14 bytes: 82 72 82 85 82 85 82 8e
	// 82 64 82 8e 82 84) followed by 32 bytes of 0xff padding. OCaml
	// disassembler.ml L1786 special-cases this exact pattern in
	// read_textout and emits a single `eof` marker instead of the
	// garbage text. We do the same here.
	if isSeenEndMarker(textStr) {
		cmd.IsJmp = true
		cmd.Kepago = []CommandElem{ElemString{Value: "eof"}}
		result.Commands = append(result.Commands, cmd)
		return nil
	}

	// OCaml force_textout: always emit as resource when SeparateStrings.
	// However, OCaml's add_textout_fails (disassembler.ml L1389) absorbs
	// some textouts into the previous command instead of creating a new
	// resource — notably stub textouts produced by isolated control bytes
	// (e.g. 0x1e) that follow certain opcodes. We don't yet replicate
	// add_textout_fails fully, but we can avoid the most visible
	// regression: textouts that contain only escape sequences for
	// control bytes (`\x{NN}` chains) carry no readable content.
	if isOnlyControlEscapes(textStr) {
		// Drop entirely; matches the most common OCaml outcome here.
		return nil
	}

	if opts.SeparateStrings {
		resIdx := len(result.ResStrs)
		result.ResStrs = append(result.ResStrs, textStr)
		cmd.ResIdx = resIdx
		// OCaml format: "#res<NNNN>" replaces the inline string in source.
		cmd.Kepago = []CommandElem{ElemString{
			Value: fmt.Sprintf("#res<%04d>", resIdx),
		}}
	} else {
		cmd.ResIdx = -1
		cmd.Kepago = []CommandElem{
			ElemString{Value: "'"},
			ElemText{Value: textStr},
			ElemString{Value: "'"},
		}
	}

	result.Commands = append(result.Commands, cmd)
	return nil
}

// seenEndPrefix is the SJIS encoding of "ＳｅｅｎＥｎｄ" (14 bytes).
// This sentinel marks the logical end of a RealLive bytecode scenario;
// what follows is 0xff padding to align to a block boundary.
var seenEndPrefix = []byte{
	0x82, 0x72, // Ｓ
	0x82, 0x85, // ｅ
	0x82, 0x85, // ｅ
	0x82, 0x8e, // ｎ
	0x82, 0x64, // Ｅ
	0x82, 0x8e, // ｎ
	0x82, 0x84, // ｄ
}

// isSeenEndMarker reports whether the given textout buffer is the
// isOnlyControlEscapes reports whether s consists entirely of `\x{NN}`
// escape sequences and surrounding non-content (whitespace, escaped
// backslashes). These come from isolated control bytes (< 0x20) that
// the textout reader emitted in the buffer when ControlCodes is
// enabled. OCaml's add_textout_fails generally absorbs such stubs into
// the previous command rather than creating a fresh resource entry.
//
// We look for the regex-equivalent ^([\\\s]*\\x\{[0-9a-fA-F]+\})+[\\\s]*$
// — i.e. only escape sequences with optional decorative \\ or whitespace.
func isOnlyControlEscapes(s string) bool {
	if len(s) == 0 {
		return false
	}
	hasEscape := false
	i := 0
	for i < len(s) {
		c := s[i]
		// Skip whitespace and lone backslashes (e.g. "\\" or "\\\\" decoration).
		if c == ' ' || c == '\t' || c == '\\' {
			i++
			continue
		}
		// Need exactly the form \x{HEX} starting here, with the leading
		// backslash potentially already consumed above.
		// Look back: was the previous char a backslash?
		if i > 0 && s[i-1] == '\\' && c == 'x' && i+1 < len(s) && s[i+1] == '{' {
			j := i + 2
			for j < len(s) && s[j] != '}' {
				ch := s[j]
				isHex := (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
				if !isHex {
					return false
				}
				j++
			}
			if j >= len(s) {
				return false
			}
			i = j + 1
			hasEscape = true
			continue
		}
		// Anything else: real content.
		return false
	}
	return hasEscape
}

// "SeenEnd" sentinel: the SJIS string "ＳｅｅｎＥｎｄ" followed only by
// 0xff bytes (any number — typically 32, but the count can vary by
// scenario size).
func isSeenEndMarker(s string) bool {
	if len(s) < len(seenEndPrefix) {
		return false
	}
	for i, b := range seenEndPrefix {
		if s[i] != b {
			return false
		}
	}
	for i := len(seenEndPrefix); i < len(s); i++ {
		if s[i] != 0xff {
			return false
		}
	}
	return true
}

// peekByteAt returns the byte at offset n from the current position, or 0
// if past the limit. Does NOT advance. Used for multi-byte literal lookahead.
func (r *Reader) peekByteAt(n int) byte {
	if r.pos+n >= r.limit {
		return 0
	}
	return r.data[r.pos+n]
}

// matchSeenEnd checks if the next bytes are the 14-byte SJIS prefix for
// "ＳｅｅｎＥｎｄ". If so, consumes the prefix AND all immediately
// following 0xff padding bytes, then returns true. Otherwise returns
// false without advancing.
//
// OCaml reference: disassembler.ml L1786, which uses ulex regexp matching
// against the literal bytes "\x82\x72\x82\x85...\xff\xff\xff..." (32 0xff
// bytes). Our version is more lenient about the count of 0xff trailers
// because the padding length depends on scenario size alignment.
func (r *Reader) matchSeenEnd() bool {
	if r.pos+len(seenEndPrefix) > r.limit {
		return false
	}
	for i, b := range seenEndPrefix {
		if r.data[r.pos+i] != b {
			return false
		}
	}
	// Prefix matched; consume it and any trailing 0xff padding.
	r.pos += len(seenEndPrefix)
	for r.pos < r.limit && r.data[r.pos] == 0xff {
		r.pos++
	}
	return true
}

// isShiftJISLead returns true if the byte is a ShiftJIS lead byte.
func isShiftJISLead(b byte) bool {
	return (b >= 0x81 && b <= 0x9f) || (b >= 0xe0 && b <= 0xef) || (b >= 0xf0 && b <= 0xfc)
}
