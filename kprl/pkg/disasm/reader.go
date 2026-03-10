package disasm

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/yoremi/rldev-go/pkg/binarray"
	"github.com/yoremi/rldev-go/pkg/bytecode"
)

// Reader reads bytecodes sequentially from a buffer.
type Reader struct {
	data   []byte
	pos    int
	origin int // data_offset (start of code section)
	limit  int // end of data
	mode   EngineMode
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
func (r *Reader) GetExpression() (string, error) {
	return r.getExprBool()
}

func (r *Reader) getExprBool() (string, error) {
	left, err := r.getExprCond()
	if err != nil {
		return "", err
	}

	for {
		b, err := r.Peek()
		if err != nil {
			return left, nil
		}
		switch b {
		case 0x3c: // &&
			r.Next()
			right, err := r.getExprCond()
			if err != nil {
				return "", err
			}
			left = left + " && " + right
		case 0x3d: // ||
			r.Next()
			right, err := r.getExprCond()
			if err != nil {
				return "", err
			}
			left = left + " || " + right
		default:
			return left, nil
		}
	}
}

func (r *Reader) getExprCond() (string, error) {
	left, err := r.getExprArith()
	if err != nil {
		return "", err
	}

	b, err := r.Peek()
	if err != nil {
		return left, nil
	}

	var op string
	switch b {
	case 0x28:
		op = "=="
	case 0x29:
		op = "!="
	case 0x2a:
		op = "<="
	case 0x2b:
		op = "<"
	case 0x2c:
		op = ">="
	case 0x2d:
		op = ">"
	default:
		return left, nil
	}

	r.Next()
	right, err := r.getExprArith()
	if err != nil {
		return "", err
	}
	return left + " " + op + " " + right, nil
}

func (r *Reader) getExprArith() (string, error) {
	left, err := r.getExprTerm()
	if err != nil {
		return "", err
	}

	for {
		b, err := r.Peek()
		if err != nil {
			return left, nil
		}

		var op string
		switch b {
		case 0x00:
			op = "+"
		case 0x01:
			op = "-"
		case 0x02:
			op = "*"
		case 0x03:
			op = "/"
		case 0x04:
			op = "%"
		case 0x05:
			op = "&"
		case 0x06:
			op = "|"
		case 0x07:
			op = "^"
		case 0x08:
			op = "<<"
		case 0x09:
			op = ">>"
		default:
			return left, nil
		}

		r.Next()
		right, err := r.getExprTerm()
		if err != nil {
			return "", err
		}
		left = left + " " + op + " " + right
	}
}

func (r *Reader) getExprTerm() (string, error) {
	b, err := r.Next()
	if err != nil {
		return "", err
	}

	switch {
	case b == 0xff: // Immediate integer constant
		v, err := r.ReadInt32()
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%d", v), nil

	case b == 0xc8: // Store register
		return "store", nil

	case b == 0x0a: // Unary minus
		term, err := r.getExprTerm()
		if err != nil {
			return "", err
		}
		return "-" + term, nil

	case b == 0x0b: // Parenthesized expression
		expr, err := r.GetExpression()
		if err != nil {
			return "", err
		}
		if err := r.Expect(0x29, "getExprTerm/paren"); err != nil {
			// Try to continue
		}
		return "(" + expr + ")", nil

	case b >= 0x14 && b <= 0x1e: // Unary operators
		switch b {
		case 0x14:
			term, err := r.getExprTerm()
			if err != nil {
				return "", err
			}
			return "~" + term, nil
		default:
			// Other unary ops - read term
			term, err := r.getExprTerm()
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("op_%02x(%s)", b, term), nil
		}

	case b == '$': // Integer variable
		return r.readIntVar()

	default:
		r.Rollback(1)
		return "", fmt.Errorf("unexpected byte 0x%02x in expression at offset 0x%x", b, r.pos)
	}
}

// readIntVar reads an integer variable reference (intA[idx], intB[idx], etc.)
func (r *Reader) readIntVar() (string, error) {
	// Variable type byte
	varType, err := r.Next()
	if err != nil {
		return "", err
	}

	// Read array index expression enclosed in [ ]
	if err := r.Expect('[', "readIntVar"); err != nil {
		r.Rollback(1)
		// Some formats use different bracketing
	}

	idx, err := r.GetExpression()
	if err != nil {
		return "", err
	}

	if err := r.Expect(']', "readIntVar"); err != nil {
		// Try to continue
	}

	prefix := "int"
	switch {
	case varType < 6:
		letters := []string{"A", "B", "C", "D", "E", "F"}
		prefix = "int" + letters[varType]
	case varType == 6:
		prefix = "intG"
	case varType == 7:
		prefix = "intZ"
	case varType == 0x0a:
		prefix = "intL"
	default:
		prefix = fmt.Sprintf("int_%02x", varType)
	}

	return fmt.Sprintf("%s[%s]", prefix, idx), nil
}

// GetData reads a "data" element - either a string or expression.
// This is the main data reader used for function arguments.
func (r *Reader) GetData() (string, error) {
	b, err := r.Peek()
	if err != nil {
		return "", err
	}

	if b == '"' || b == 0x0a {
		// String data
		return r.readStringData()
	}

	// Expression data
	return r.GetExpression()
}

// readStringData reads a string argument (quoted string or string variable).
func (r *Reader) readStringData() (string, error) {
	b, err := r.Next()
	if err != nil {
		return "", err
	}

	switch b {
	case '"':
		// Literal string
		var sb strings.Builder
		sb.WriteByte('"')
		for {
			c, err := r.Next()
			if err != nil {
				break
			}
			if c == '"' {
				sb.WriteByte('"')
				break
			}
			sb.WriteByte(c)
		}
		return sb.String(), nil

	case 0x0a:
		// String variable reference
		return r.readStrVar()

	default:
		r.Rollback(1)
		return "", fmt.Errorf("unexpected byte 0x%02x in string data", b)
	}
}

// readStrVar reads a string variable reference (strS[idx], etc.)
func (r *Reader) readStrVar() (string, error) {
	varType, err := r.Next()
	if err != nil {
		return "", err
	}

	if err := r.Expect('[', "readStrVar"); err != nil {
		r.Rollback(1)
	}

	idx, err := r.GetExpression()
	if err != nil {
		return "", err
	}

	if err := r.Expect(']', "readStrVar"); err != nil {
		// Continue
	}

	prefix := "str"
	switch varType {
	case 0x0c:
		prefix = "strS"
	case 0x12:
		prefix = "strM"
	default:
		prefix = fmt.Sprintf("str_%02x", varType)
	}

	return fmt.Sprintf("%s[%s]", prefix, idx), nil
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
				Value: fmt.Sprintf("#entrypoint %03d // Z%02d", entryIdx, entryIdx),
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
func readFunction(r *Reader, result *DisassemblyResult, offset int, op Opcode, argc int, opts Options) error {
	cmd := Command{Offset: offset}
	opStr := op.String()

	// Read function arguments
	args, err := readFuncArgs(r, argc)
	if err != nil {
		// If we fail to parse args, emit what we can
		cmd.Kepago = []CommandElem{ElemString{Value: fmt.Sprintf("op<%s>(?)", opStr)}}
		cmd.Opcode = opStr
		result.Commands = append(result.Commands, cmd)
		return nil // Don't propagate - try to continue
	}

	// Special handling for known opcodes
	switch {
	case op.Module == 1 && (op.Function == 1 || op.Function == 3):
		// goto / gosub - has pointer target
		handleGoto(r, result, &cmd, op, args, offset)
	case op.Module == 1 && (op.Function == 5 || op.Function == 8 ||
		op.Function == 9 || op.Function == 16):
		// Conditional goto/gosub
		handleCondGoto(r, result, &cmd, op, args, offset)
	case op.Module == 5 && op.Function == 1:
		// ret
		cmd.Kepago = []CommandElem{ElemString{Value: "ret"}}
		cmd.IsJmp = true
	default:
		// Generic function
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("op<%s>", opStr))
		if len(args) > 0 {
			sb.WriteByte('(')
			for i, arg := range args {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(arg)
			}
			sb.WriteByte(')')
		}
		cmd.Kepago = []CommandElem{ElemString{Value: sb.String()}}
		cmd.Opcode = opStr
	}

	result.Commands = append(result.Commands, cmd)
	return nil
}

// readFuncArgs reads argc function arguments.
func readFuncArgs(r *Reader, argc int) ([]string, error) {
	var args []string

	// Check for opening paren
	b, err := r.Peek()
	if err != nil {
		return nil, err
	}
	if b == '(' {
		r.Next()
	}

	for i := 0; i < argc; i++ {
		arg, err := r.GetData()
		if err != nil {
			return args, err
		}
		args = append(args, arg)
	}

	// Check for closing paren
	b, err = r.Peek()
	if err == nil && b == ')' {
		r.Next()
	}

	return args, nil
}

// handleGoto processes goto/gosub instructions.
func handleGoto(r *Reader, result *DisassemblyResult, cmd *Command, op Opcode, args []string, offset int) {
	name := "goto"
	if op.Function == 3 {
		name = "gosub"
	}

	cmd.IsJmp = (op.Function == 1) // goto is unconditional jump
	cmd.Kepago = []CommandElem{ElemString{Value: name}}

	// The argument is a pointer offset
	if len(args) > 0 {
		cmd.Kepago = append(cmd.Kepago, ElemString{Value: "(" + args[0] + ")"})
	}
}

// handleCondGoto processes conditional goto/gosub.
func handleCondGoto(r *Reader, result *DisassemblyResult, cmd *Command, op Opcode, args []string, offset int) {
	name := "goto_if"
	switch op.Function {
	case 5:
		name = "goto_if"
	case 8:
		name = "goto_unless"
	case 9:
		name = "gosub_if"
	case 16:
		name = "gosub_unless"
	}

	var sb strings.Builder
	sb.WriteString(name)
	if len(args) > 0 {
		sb.WriteByte('(')
		sb.WriteString(strings.Join(args, ", "))
		sb.WriteByte(')')
	}

	cmd.Kepago = []CommandElem{ElemString{Value: sb.String()}}
}

// readAssignment reads a variable assignment ('$' prefix).
func readAssignment(r *Reader, result *DisassemblyResult, offset int) error {
	cmd := Command{Offset: offset}

	// Read destination variable
	dest, err := r.readIntVar()
	if err != nil {
		return err
	}

	// Read operator
	opByte, err := r.Next()
	if err != nil {
		return err
	}

	var op string
	switch opByte {
	case 0x14:
		op = "="
	case 0x15:
		op = "+="
	case 0x16:
		op = "-="
	case 0x17:
		op = "*="
	case 0x18:
		op = "/="
	case 0x19:
		op = "%="
	case 0x1a:
		op = "&="
	case 0x1b:
		op = "|="
	case 0x1c:
		op = "^="
	case 0x1d:
		op = "<<="
	case 0x1e:
		op = ">>="
	default:
		op = fmt.Sprintf("?=%02x=", opByte)
	}

	// Read source expression
	if err := r.Expect(0x5c, "readAssignment"); err != nil {
		// backslash separator - may be missing in some versions
	}

	src, err := r.GetExpression()
	if err != nil {
		src = "???"
	}

	// Expect terminator
	r.Expect(0x5c, "readAssignment/end")

	cmd.Kepago = []CommandElem{ElemString{
		Value: fmt.Sprintf("%s %s %s", dest, op, src),
	}}
	result.Commands = append(result.Commands, cmd)
	return nil
}

// readTextout reads a textout command (displayed text).
// This is the most important part for translation work.
func readTextout(r *Reader, result *DisassemblyResult, offset int, opts Options) error {
	cmd := Command{Offset: offset, CType: "textout"}

	var text strings.Builder

	for !r.AtEnd() {
		b, err := r.Peek()
		if err != nil {
			break
		}

		// Check for end of text markers
		if b == 0x00 || b == '#' || b == '$' || b == '\n' || b == ',' ||
			b == '@' || b == '!' {
			break
		}

		r.Next()

		switch {
		case b == 0x03:
			// Page break
			text.WriteString("\\p")
			break // End text on page break

		case b == 0x04:
			// Ruby text (furigana) start
			text.WriteString("{ruby ")
			// Read until 0x05 (ruby separator) and 0x06 (end)
			for !r.AtEnd() {
				c, err := r.Next()
				if err != nil {
					break
				}
				if c == 0x05 {
					text.WriteString("}{")
					continue
				}
				if c == 0x06 {
					text.WriteByte('}')
					break
				}
				// ShiftJIS double-byte check
				if isShiftJISLead(c) && !r.AtEnd() {
					c2, _ := r.Next()
					text.WriteByte(c)
					text.WriteByte(c2)
				} else {
					text.WriteByte(c)
				}
			}

		case b == 0x01:
			// Line break
			text.WriteString("\\n")

		case b == 0x02:
			// Pause (wait for click)
			text.WriteString("\\w")

		case isShiftJISLead(b):
			// Double-byte ShiftJIS character
			b2, err := r.Next()
			if err != nil {
				text.WriteByte(b)
				break
			}
			text.WriteByte(b)
			text.WriteByte(b2)

		case b >= 0x20 && b < 0x80:
			// ASCII
			text.WriteByte(b)

		default:
			// Control code or unknown byte
			if opts.ControlCodes {
				text.WriteString(fmt.Sprintf("\\x{%02x}", b))
			}
		}
	}

	if text.Len() == 0 {
		return nil // Don't emit empty text commands
	}

	textStr := text.String()

	// Add as resource string
	resIdx := len(result.ResStrs)
	result.ResStrs = append(result.ResStrs, textStr)
	cmd.ResIdx = resIdx

	if opts.SeparateStrings {
		cmd.Kepago = []CommandElem{ElemString{
			Value: fmt.Sprintf("<res_%04d>", resIdx),
		}}
	} else {
		cmd.Kepago = []CommandElem{ElemString{Value: "'" + textStr + "'"}}
	}

	result.Commands = append(result.Commands, cmd)
	return nil
}

// isShiftJISLead returns true if the byte is a ShiftJIS lead byte.
func isShiftJISLead(b byte) bool {
	return (b >= 0x81 && b <= 0x9f) || (b >= 0xe0 && b <= 0xef) || (b >= 0xf0 && b <= 0xfc)
}
