// Package codegen generates RealLive bytecode from compiler IR.
//
// Transposed from OCaml:
//   - rlc/codegen.ml (144 lines)       — expression encoding, opcode encoding, Output IR buffer
//   - rlc/bytecodeGen.ml (267 lines)   — binary file generation for RealLive/AVG2000
//
// The codegen pipeline has two stages:
//   1. IR construction: the compiler emits IR elements (Code, Label, LabelRef,
//      Entrypoint, Kidoku, Lineref) into an Output buffer.
//   2. Binary generation: Generate() traverses the IR to compute label positions,
//      build the bytecode buffer, optionally compress, and produce the final
//      .TXT (SEEN) file in RealLive or AVG2000 format.
//
// Bytecode encoding (RealLive expressions):
//   - Integers: $\xff followed by 4 LE bytes
//   - Variables: $<bank>[<expr>]  (bank = register id byte)
//   - Store register: $\xc8
//   - Operators: \<opbyte>  between operands
//   - Opcodes: #<type><module><code_lo><code_hi><argc_lo><argc_hi><overload>
package codegen

import (
	"encoding/binary"
	"fmt"

	"github.com/yoremi/rldev-go/rlc/pkg/ast"
	"github.com/yoremi/rldev-go/rlc/pkg/kfn"
)

// ============================================================
// Expression encoding (from codegen.ml)
// ============================================================

// Bytecodes for arithmetic operators in expressions.
func OpCode(op ast.ArithOp) byte {
	switch op {
	case ast.OpAdd: return 0x00
	case ast.OpSub: return 0x01
	case ast.OpMul: return 0x02
	case ast.OpDiv: return 0x03
	case ast.OpMod: return 0x04
	case ast.OpAnd: return 0x05
	case ast.OpOr:  return 0x06
	case ast.OpXor: return 0x07
	case ast.OpShl: return 0x08
	case ast.OpShr: return 0x09
	}
	return 0x00
}

// Bytecodes for comparison operators.
func CmpCode(op ast.CmpOp) byte {
	switch op {
	case ast.CmpEqu: return 0x28
	case ast.CmpNeq: return 0x29
	case ast.CmpLte: return 0x2a
	case ast.CmpLtn: return 0x2b
	case ast.CmpGte: return 0x2c
	case ast.CmpGtn: return 0x2d
	}
	return 0x28
}

// Bytecodes for short-circuit logical operators.
func ChainCode(op ast.ChainOp) byte {
	switch op {
	case ast.ChainAnd: return 0x3c
	case ast.ChainOr:  return 0x3d
	}
	return 0x3c
}

// Bytecodes for assignment operators.
func AssignCode(op ast.AssignOp) byte {
	switch op {
	case ast.AssignAdd: return 0x14
	case ast.AssignSub: return 0x15
	case ast.AssignMul: return 0x16
	case ast.AssignDiv: return 0x17
	case ast.AssignMod: return 0x18
	case ast.AssignAnd: return 0x19
	case ast.AssignOr:  return 0x1a
	case ast.AssignXor: return 0x1b
	case ast.AssignShl: return 0x1c
	case ast.AssignShr: return 0x1d
	case ast.AssignSet: return 0x1e
	}
	return 0x1e
}

// EncodeInt32 encodes an integer literal as $\xff + 4 LE bytes.
func EncodeInt32(v int32) []byte {
	buf := make([]byte, 6)
	buf[0] = '$'
	buf[1] = 0xff
	binary.LittleEndian.PutUint32(buf[2:], uint32(v))
	return buf
}

// EncodeInt16 encodes a 16-bit value as 2 LE bytes.
func EncodeInt16(v int) []byte {
	buf := make([]byte, 2)
	binary.LittleEndian.PutUint16(buf, uint16(v))
	return buf
}

// EncodeOpcode encodes an opcode header: #<type><module><code16><argc16><overload>
func EncodeOpcode(opType, opModule, opCode, argc, overload int) []byte {
	buf := make([]byte, 8)
	buf[0] = '#'
	buf[1] = byte(opType)
	buf[2] = byte(opModule)
	binary.LittleEndian.PutUint16(buf[3:5], uint16(opCode))
	binary.LittleEndian.PutUint16(buf[5:7], uint16(argc))
	buf[7] = byte(overload)
	return buf
}

// ============================================================
// Intermediate Representation (IR)
// ============================================================

// IRType identifies the kind of IR element.
type IRType int

const (
	IRCode       IRType = iota // raw bytecode bytes
	IRLabel                    // label definition (zero-width)
	IRLabelRef                 // 4-byte label reference (resolved during Generate)
	IREntrypoint               // entrypoint marker + kidoku
	IRKidoku                   // kidoku (read-flag) marker
	IRLineref                  // line number reference (for debug info)
)

// IR is one element of the intermediate representation.
type IR struct {
	Type  IRType
	Bytes []byte // for IRCode
	Label string // for IRLabel, IRLabelRef
	Index int    // for IREntrypoint, IRKidoku, IRLineref
	Loc   ast.Loc
}

// Output accumulates IR elements during compilation.
type Output struct {
	IR      []IR
	labels  map[string]bool // tracks defined label names (for duplicate detection)
	lastLine int
}

// NewOutput creates a fresh IR output buffer.
func NewOutput() *Output {
	return &Output{
		labels: make(map[string]bool),
	}
}

// AddCode appends raw bytecode.
func (o *Output) AddCode(loc ast.Loc, code []byte) {
	o.maybeLine(loc)
	o.IR = append(o.IR, IR{Type: IRCode, Bytes: code, Loc: loc})
}

// AddCodeStr appends raw bytecode from a string.
func (o *Output) AddCodeStr(loc ast.Loc, s string) {
	o.AddCode(loc, []byte(s))
}

// AddLabel defines a label at the current position.
func (o *Output) AddLabel(name string, loc ast.Loc) error {
	if o.labels[name] {
		return fmt.Errorf("%s: @%s already defined; label identifiers must be unique", loc, name)
	}
	o.IR = append(o.IR, IR{Type: IRLabel, Label: name, Loc: loc})
	o.labels[name] = true
	return nil
}

// AddLabelRef emits a 4-byte forward reference to a label.
func (o *Output) AddLabelRef(name string, loc ast.Loc) {
	o.IR = append(o.IR, IR{Type: IRLabelRef, Label: name, Loc: loc})
}

// AddEntrypoint emits an entrypoint marker.
func (o *Output) AddEntrypoint(index int) {
	o.IR = append(o.IR, IR{Type: IREntrypoint, Index: index})
}

// AddKidoku emits a kidoku (read-flag) marker.
func (o *Output) AddKidoku(loc ast.Loc, line int) {
	o.maybeLine(loc)
	o.IR = append(o.IR, IR{Type: IRKidoku, Index: line, Loc: loc})
}

// Length returns the number of IR elements.
func (o *Output) Length() int { return len(o.IR) }

func (o *Output) maybeLine(loc ast.Loc) {
	if loc != ast.Nowhere && loc.Line != o.lastLine {
		o.lastLine = loc.Line
	}
}

// --- Expression emission helpers ---

// EmitExpr encodes an expression into bytecode and appends it to the output.
func (o *Output) EmitExpr(e ast.Expr) {
	switch x := e.(type) {
	case ast.IntLit:
		o.AddCode(x.Loc, EncodeInt32(x.Val))
	case ast.StoreRef:
		o.AddCode(x.Loc, []byte{'$', 0xc8})
	case ast.IntVar:
		o.AddCode(x.Loc, []byte{'$', byte(x.Bank), '['})
		o.EmitExpr(x.Index)
		o.AddCode(x.Loc, []byte{']'})
	case ast.StrVar:
		o.AddCode(x.Loc, []byte{'$', byte(x.Bank), '['})
		o.EmitExpr(x.Index)
		o.AddCode(x.Loc, []byte{']'})
	case ast.BinOp:
		o.EmitExpr(x.LHS)
		o.AddCode(x.Loc, []byte{'\\', OpCode(x.Op)})
		o.EmitExpr(x.RHS)
	case ast.CmpExpr:
		o.EmitExpr(x.LHS)
		o.AddCode(x.Loc, []byte{'\\', CmpCode(x.Op)})
		o.EmitExpr(x.RHS)
	case ast.ChainExpr:
		o.EmitExpr(x.LHS)
		o.AddCode(x.Loc, []byte{'\\', ChainCode(x.Op)})
		o.EmitExpr(x.RHS)
	case ast.UnaryExpr:
		if x.Op == ast.UnarySub {
			o.AddCode(x.Loc, []byte{'\\', OpCode(ast.OpSub)})
			o.EmitExpr(x.Val)
		}
		// Other unary ops should have been transformed to binary by expr normalization
	case ast.ParenExpr:
		o.AddCode(x.Loc, []byte{'('})
		o.EmitExpr(x.Expr)
		o.AddCode(x.Loc, []byte{')'})
	}
}

// EmitAssignment encodes an assignment and appends it.
func (o *Output) EmitAssignment(loc ast.Loc, dest ast.Expr, op ast.AssignOp, expr ast.Expr) {
	o.EmitExpr(dest)
	o.AddCode(loc, []byte{'\\', AssignCode(op)})
	o.EmitExpr(expr)
}

// EmitOpcode encodes and appends an opcode header.
func (o *Output) EmitOpcode(loc ast.Loc, opType, opModule, opCode, argc, overload int) {
	o.AddCode(loc, EncodeOpcode(opType, opModule, opCode, argc, overload))
}

// ============================================================
// Binary file generation (from bytecodeGen.ml)
// ============================================================

// GenerateOptions controls output file generation.
type GenerateOptions struct {
	Target          kfn.Target
	CompilerVersion int    // e.g., 10002
	Compress        bool
	DebugInfo       bool
	Metadata        []byte // optional metadata bytes
	Version         kfn.Version
	KidokuType      int    // 0=auto, 1=@, 2=!
}

// DefaultOptions returns sensible defaults.
func DefaultOptions() GenerateOptions {
	return GenerateOptions{
		Target:          kfn.TargetRealLive,
		CompilerVersion: 10002,
		Compress:        true,
		DebugInfo:       false,
		KidokuType:      0,
	}
}

// targetSpec holds format-specific parameters.
type targetSpec struct {
	kidokuLen int  // bytes per kidoku entry (2=RL, 4=AVG)
	linenoLen int  // bytes per line reference
	useLZ77   bool // compress with LZ77 (RealLive only)
}

func specForTarget(t kfn.Target) targetSpec {
	if t == kfn.TargetAVG2000 {
		return targetSpec{kidokuLen: 4, linenoLen: 4, useLZ77: false}
	}
	return targetSpec{kidokuLen: 2, linenoLen: 2, useLZ77: true}
}

// Generate traverses the IR and produces the final binary output.
// Returns the complete file bytes.
func (o *Output) Generate(opts GenerateOptions) ([]byte, error) {
	spec := specForTarget(opts.Target)

	// --- Phase 1: Deduplicate entrypoints (last definition wins) ---
	entrySlots := make(map[int]int) // entrypoint index → IR index
	for i, ir := range o.IR {
		if ir.Type == IREntrypoint {
			if prev, ok := entrySlots[ir.Index]; ok {
				o.IR[prev] = IR{Type: IRCode, Bytes: nil} // nullify previous
			}
			entrySlots[ir.Index] = i
		}
	}

	// --- Phase 2: Compute label positions and bytecode length ---
	labelPos := make(map[string]int)
	entrypoints := make([]int, 100)
	var kidokuTable []int
	bytecodeLen := 0

	for _, ir := range o.IR {
		switch ir.Type {
		case IRCode:
			bytecodeLen += len(ir.Bytes)
		case IRLabelRef:
			bytecodeLen += 4
		case IRLabel:
			labelPos[ir.Label] = bytecodeLen
		case IRKidoku:
			kidokuTable = append(kidokuTable, ir.Index)
			bytecodeLen += 1 + spec.kidokuLen
		case IREntrypoint:
			entrypoints[ir.Index] = bytecodeLen
			kidokuTable = append(kidokuTable, ir.Index+1_000_000)
			bytecodeLen += 1 + spec.kidokuLen
		case IRLineref:
			bytecodeLen += 1 + spec.linenoLen
		}
	}

	// --- Phase 3: Build bytecode buffer ---
	// Buffer starts at offset 8 to leave room for compressed header
	bufSize := bytecodeLen + 16
	buf := make([]byte, bufSize)
	kidokuIdx := 0
	pos := 8 // start after compression header space

	// Determine kidoku marker character
	kidokuChar := byte('@')
	kt := opts.KidokuType
	if kt == 0 {
		v := opts.Version
		if v[0] > 1 || (v[0] == 1 && v[1] > 2) || (v[0] == 1 && v[1] == 2 && v[2] > 5) {
			kt = 2
		} else {
			kt = 1
		}
	}
	if kt == 2 {
		kidokuChar = '!'
	}

	for _, ir := range o.IR {
		switch ir.Type {
		case IRCode:
			copy(buf[pos:], ir.Bytes)
			pos += len(ir.Bytes)
		case IRLabelRef:
			target, ok := labelPos[ir.Label]
			if !ok {
				return nil, fmt.Errorf("%s: reference to undefined label @%s", ir.Loc, ir.Label)
			}
			binary.LittleEndian.PutUint32(buf[pos:], uint32(target))
			pos += 4
		case IRLabel:
			// zero-width
		case IRKidoku:
			buf[pos] = '@'
			pos++
			if spec.kidokuLen == 2 {
				binary.LittleEndian.PutUint16(buf[pos:], uint16(kidokuIdx))
				pos += 2
			} else {
				binary.LittleEndian.PutUint32(buf[pos:], uint32(kidokuIdx))
				pos += 4
			}
			kidokuIdx++
		case IREntrypoint:
			buf[pos] = kidokuChar
			pos++
			if spec.kidokuLen == 2 {
				binary.LittleEndian.PutUint16(buf[pos:], uint16(kidokuIdx))
				pos += 2
			} else {
				binary.LittleEndian.PutUint32(buf[pos:], uint32(kidokuIdx))
				pos += 4
			}
			kidokuIdx++
		case IRLineref:
			buf[pos] = 0x0a
			pos++
			if spec.linenoLen == 2 {
				binary.LittleEndian.PutUint16(buf[pos:], uint16(ir.Index))
				pos += 2
			} else {
				binary.LittleEndian.PutUint32(buf[pos:], uint32(ir.Index))
				pos += 4
			}
		}
	}

	bytecode := buf[8 : 8+bytecodeLen]
	compressedLen := bytecodeLen

	// --- Phase 4: Compress if required ---
	// (Compression is handled externally — we store uncompressed for now.
	//  The caller can use the compression package to compress bytecode.)

	// --- Phase 5: Build output file ---
	if opts.Target == kfn.TargetAVG2000 {
		return buildAVG2000(bytecode, bytecodeLen, entrypoints, kidokuTable, opts)
	}
	return buildRealLive(bytecode, bytecodeLen, compressedLen, entrypoints, kidokuTable, opts)
}

// buildRealLive creates a RealLive format .TXT (SEEN) file.
// Header: 0x1d0 bytes, then kidoku table, then optional metadata, then bytecode.
func buildRealLive(bytecode []byte, bytecodeLen, compressedLen int, entrypoints []int, kidokuTable []int, opts GenerateOptions) ([]byte, error) {
	metadataLen := len(opts.Metadata)
	kidokuBytes := len(kidokuTable) * 4
	dramOff := 0x1d0 + kidokuBytes
	bcOff := dramOff + metadataLen
	fileLen := bcOff + compressedLen

	file := make([]byte, fileLen)

	// Magic / header offset
	if opts.Compress {
		putInt32(file, 0x00, 0x1d0)
	} else {
		copy(file[0x00:], []byte("KPRL"))
	}
	putInt32(file, 0x04, opts.CompilerVersion)
	putInt32(file, 0x08, 0x1d0)                // kidoku table offset
	putInt32(file, 0x0c, len(kidokuTable))       // kidoku count
	putInt32(file, 0x10, kidokuBytes)            // kidoku table size
	putInt32(file, 0x14, dramOff)                // dramatis offset
	putInt32(file, 0x18, 0)                      // dramatis count
	putInt32(file, 0x1c, 0)                      // dramatis size
	putInt32(file, 0x20, bcOff)                  // bytecode offset
	putInt32(file, 0x24, bytecodeLen)            // bytecode length
	putInt32(file, 0x28, compressedLen)          // compressed length

	// Entrypoints at 0x34 (up to 100 × 4 bytes)
	for i := 0; i < len(entrypoints) && i < 100; i++ {
		putInt32(file, 0x34+i*4, entrypoints[i])
	}

	// Kidoku table
	for i, v := range kidokuTable {
		putInt32(file, 0x1d0+i*4, v)
	}

	// Metadata
	if metadataLen > 0 {
		copy(file[dramOff:], opts.Metadata)
	}

	// Bytecode
	copy(file[bcOff:], bytecode[:compressedLen])

	return file, nil
}

// buildAVG2000 creates an AVG2000 format .TXT file.
// Header: 0x1cc bytes, then kidoku table, then bytecode.
func buildAVG2000(bytecode []byte, bytecodeLen int, entrypoints []int, kidokuTable []int, opts GenerateOptions) ([]byte, error) {
	kidokuBytes := len(kidokuTable) * 4
	bcOff := 0x1cc + kidokuBytes
	fileLen := bcOff + bytecodeLen

	file := make([]byte, fileLen)

	if opts.Compress {
		putInt32(file, 0x00, 0x1cc)
	} else {
		copy(file[0x00:], []byte("KP2K"))
	}
	putInt32(file, 0x04, 10002) // always 10002 for AVG2000

	// Timestamp at 0x08-0x15 (optional — zeros are acceptable)

	putInt32(file, 0x20, len(kidokuTable))
	putInt32(file, 0x24, bytecodeLen)

	// Entrypoints at 0x30
	for i := 0; i < len(entrypoints) && i < 100; i++ {
		putInt32(file, 0x30+i*4, entrypoints[i])
	}

	// Kidoku table
	for i, v := range kidokuTable {
		putInt32(file, 0x1cc+i*4, v)
	}

	// Bytecode
	copy(file[bcOff:], bytecode)

	return file, nil
}

func putInt32(buf []byte, offset, value int) {
	binary.LittleEndian.PutUint32(buf[offset:], uint32(value))
}
