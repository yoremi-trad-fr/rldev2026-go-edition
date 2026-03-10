package codegen

import (
	"encoding/binary"
	"testing"

	"github.com/yoremi/rldev-go/rlc/pkg/ast"
	"github.com/yoremi/rldev-go/rlc/pkg/kfn"
)

func TestOpCodes(t *testing.T) {
	tests := []struct{ op ast.ArithOp; want byte }{
		{ast.OpAdd, 0x00}, {ast.OpSub, 0x01}, {ast.OpMul, 0x02}, {ast.OpDiv, 0x03},
		{ast.OpMod, 0x04}, {ast.OpAnd, 0x05}, {ast.OpOr, 0x06}, {ast.OpXor, 0x07},
		{ast.OpShl, 0x08}, {ast.OpShr, 0x09},
	}
	for _, tt := range tests {
		if got := OpCode(tt.op); got != tt.want {
			t.Errorf("OpCode(%v) = 0x%02x, want 0x%02x", tt.op, got, tt.want)
		}
	}
}

func TestCmpCodes(t *testing.T) {
	tests := []struct{ op ast.CmpOp; want byte }{
		{ast.CmpEqu, 0x28}, {ast.CmpNeq, 0x29},
		{ast.CmpLte, 0x2a}, {ast.CmpLtn, 0x2b},
		{ast.CmpGte, 0x2c}, {ast.CmpGtn, 0x2d},
	}
	for _, tt := range tests {
		if got := CmpCode(tt.op); got != tt.want {
			t.Errorf("CmpCode(%v) = 0x%02x, want 0x%02x", tt.op, got, tt.want)
		}
	}
}

func TestChainCodes(t *testing.T) {
	if ChainCode(ast.ChainAnd) != 0x3c { t.Error("ChainAnd") }
	if ChainCode(ast.ChainOr) != 0x3d { t.Error("ChainOr") }
}

func TestAssignCodes(t *testing.T) {
	tests := []struct{ op ast.AssignOp; want byte }{
		{ast.AssignAdd, 0x14}, {ast.AssignSub, 0x15},
		{ast.AssignMul, 0x16}, {ast.AssignDiv, 0x17},
		{ast.AssignSet, 0x1e},
	}
	for _, tt := range tests {
		if got := AssignCode(tt.op); got != tt.want {
			t.Errorf("AssignCode(%v) = 0x%02x, want 0x%02x", tt.op, got, tt.want)
		}
	}
}

func TestEncodeInt32(t *testing.T) {
	b := EncodeInt32(42)
	if len(b) != 6 { t.Fatalf("len = %d", len(b)) }
	if b[0] != '$' || b[1] != 0xff { t.Error("prefix") }
	v := binary.LittleEndian.Uint32(b[2:])
	if v != 42 { t.Errorf("value: got %d", v) }

	b = EncodeInt32(-1)
	v = binary.LittleEndian.Uint32(b[2:])
	if int32(v) != -1 { t.Errorf("negative: got %d", int32(v)) }
}

func TestEncodeInt16(t *testing.T) {
	b := EncodeInt16(0x1234)
	if len(b) != 2 { t.Fatalf("len = %d", len(b)) }
	v := binary.LittleEndian.Uint16(b)
	if v != 0x1234 { t.Errorf("got 0x%04x", v) }
}

func TestEncodeOpcode(t *testing.T) {
	b := EncodeOpcode(0, 1, 5, 2, 0)
	if len(b) != 8 { t.Fatalf("len = %d", len(b)) }
	if b[0] != '#' { t.Error("prefix") }
	if b[1] != 0 { t.Errorf("type: got %d", b[1]) }
	if b[2] != 1 { t.Errorf("module: got %d", b[2]) }
	code := binary.LittleEndian.Uint16(b[3:5])
	if code != 5 { t.Errorf("code: got %d", code) }
	argc := binary.LittleEndian.Uint16(b[5:7])
	if argc != 2 { t.Errorf("argc: got %d", argc) }
	if b[7] != 0 { t.Errorf("overload: got %d", b[7]) }
}

func TestOutputAddCode(t *testing.T) {
	o := NewOutput()
	o.AddCode(ast.Nowhere, []byte{0x01, 0x02})
	o.AddCode(ast.Nowhere, []byte{0x03})
	if o.Length() != 2 { t.Errorf("length: got %d", o.Length()) }
}

func TestOutputLabels(t *testing.T) {
	o := NewOutput()
	err := o.AddLabel("start", ast.Nowhere)
	if err != nil { t.Fatal(err) }
	// Duplicate should fail
	err = o.AddLabel("start", ast.Nowhere)
	if err == nil { t.Error("expected duplicate label error") }
}

func TestOutputEmitExprInt(t *testing.T) {
	o := NewOutput()
	o.EmitExpr(ast.IntLit{Val: 100})
	if o.Length() != 1 { t.Fatalf("length: %d", o.Length()) }
	if len(o.IR[0].Bytes) != 6 { t.Errorf("bytes: %d", len(o.IR[0].Bytes)) }
}

func TestOutputEmitExprStore(t *testing.T) {
	o := NewOutput()
	o.EmitExpr(ast.StoreRef{})
	if o.Length() != 1 { t.Fatalf("length: %d", o.Length()) }
	if o.IR[0].Bytes[0] != '$' || o.IR[0].Bytes[1] != 0xc8 {
		t.Errorf("bytes: %v", o.IR[0].Bytes)
	}
}

func TestOutputEmitExprVar(t *testing.T) {
	o := NewOutput()
	o.EmitExpr(ast.IntVar{Bank: 0x0b, Index: ast.IntLit{Val: 5}})
	// Should produce: $ 0x0b [ <int5> ]
	if o.Length() < 3 { t.Fatalf("length: %d", o.Length()) }
	if o.IR[0].Bytes[0] != '$' || o.IR[0].Bytes[1] != 0x0b {
		t.Errorf("var prefix: %v", o.IR[0].Bytes)
	}
}

func TestOutputEmitExprBinOp(t *testing.T) {
	o := NewOutput()
	o.EmitExpr(ast.BinOp{
		LHS: ast.IntLit{Val: 3},
		Op:  ast.OpAdd,
		RHS: ast.IntLit{Val: 4},
	})
	// Should produce: <int3> \ 0x00 <int4>
	if o.Length() != 3 { t.Fatalf("length: %d", o.Length()) }
	if o.IR[1].Bytes[0] != '\\' || o.IR[1].Bytes[1] != 0x00 {
		t.Errorf("operator: %v", o.IR[1].Bytes)
	}
}

func TestOutputEmitAssignment(t *testing.T) {
	o := NewOutput()
	o.EmitAssignment(ast.Nowhere,
		ast.StoreRef{},
		ast.AssignSet,
		ast.IntLit{Val: 42},
	)
	// store \0x1e <int42>
	if o.Length() < 3 { t.Fatalf("length: %d", o.Length()) }
}

func TestOutputEmitOpcode(t *testing.T) {
	o := NewOutput()
	o.EmitOpcode(ast.Nowhere, 0, 1, 0, 0, 0)
	if o.Length() != 1 { t.Fatalf("length: %d", o.Length()) }
	if o.IR[0].Bytes[0] != '#' { t.Error("opcode prefix") }
}

func TestGenerateEmpty(t *testing.T) {
	o := NewOutput()
	o.AddEntrypoint(0)
	o.AddKidoku(ast.Loc{Line: 1}, 1)
	o.AddCode(ast.Nowhere, []byte{0x00}) // halt

	data, err := o.Generate(DefaultOptions())
	if err != nil { t.Fatal(err) }
	if len(data) < 0x1d0 { t.Errorf("file too small: %d bytes", len(data)) }

	// Check magic
	magic := binary.LittleEndian.Uint32(data[0:4])
	if magic != 0x1d0 { // compressed mode → header offset
		t.Errorf("magic: got 0x%08x, want 0x1d0", magic)
	}
	// Check compiler version
	cv := binary.LittleEndian.Uint32(data[4:8])
	if cv != 10002 { t.Errorf("compiler version: got %d", cv) }
}

func TestGenerateWithLabels(t *testing.T) {
	o := NewOutput()
	o.AddEntrypoint(0)
	o.AddKidoku(ast.Loc{Line: 1}, 1)
	o.AddLabel("start", ast.Nowhere)
	o.AddCode(ast.Nowhere, []byte{0x01, 0x02, 0x03})
	o.AddLabelRef("start", ast.Nowhere)
	o.AddCode(ast.Nowhere, []byte{0x00})

	data, err := o.Generate(DefaultOptions())
	if err != nil { t.Fatal(err) }
	if len(data) < 0x1d0 { t.Fatalf("too small: %d", len(data)) }
}

func TestGenerateUndefinedLabel(t *testing.T) {
	o := NewOutput()
	o.AddLabelRef("nonexistent", ast.Loc{File: "test", Line: 1})
	_, err := o.Generate(DefaultOptions())
	if err == nil { t.Error("expected error for undefined label") }
}

func TestGenerateAVG2000(t *testing.T) {
	o := NewOutput()
	o.AddEntrypoint(0)
	o.AddKidoku(ast.Loc{Line: 1}, 1)
	o.AddCode(ast.Nowhere, []byte{0x00})

	opts := DefaultOptions()
	opts.Target = kfn.TargetAVG2000
	opts.Compress = false
	data, err := o.Generate(opts)
	if err != nil { t.Fatal(err) }
	if len(data) < 0x1cc { t.Errorf("too small: %d", len(data)) }
	// Check AVG2000 magic
	if string(data[0:4]) != "KP2K" {
		t.Errorf("magic: got %q", string(data[0:4]))
	}
}

func TestGenerateUncompressed(t *testing.T) {
	o := NewOutput()
	o.AddEntrypoint(0)
	o.AddKidoku(ast.Loc{Line: 1}, 1)
	o.AddCode(ast.Nowhere, []byte{0x00})

	opts := DefaultOptions()
	opts.Compress = false
	data, err := o.Generate(opts)
	if err != nil { t.Fatal(err) }
	// Uncompressed → magic is "KPRL"
	if string(data[0:4]) != "KPRL" {
		t.Errorf("magic: got %q, want 'KPRL'", string(data[0:4]))
	}
}

func TestGenerateMultipleEntrypoints(t *testing.T) {
	o := NewOutput()
	o.AddEntrypoint(0)
	o.AddKidoku(ast.Loc{Line: 1}, 1)
	o.AddCode(ast.Nowhere, []byte{0x01, 0x02})
	o.AddEntrypoint(1)
	o.AddKidoku(ast.Loc{Line: 5}, 5)
	o.AddCode(ast.Nowhere, []byte{0x00})

	data, err := o.Generate(DefaultOptions())
	if err != nil { t.Fatal(err) }
	// Entrypoint 0 should be at position 0
	ep0 := binary.LittleEndian.Uint32(data[0x34:0x38])
	// Entrypoint 1 should be after the first code + kidoku
	ep1 := binary.LittleEndian.Uint32(data[0x38:0x3c])
	if ep1 <= ep0 { t.Errorf("ep1 (%d) should be > ep0 (%d)", ep1, ep0) }
}

func TestGenerateKidokuTable(t *testing.T) {
	o := NewOutput()
	o.AddEntrypoint(0)
	o.AddKidoku(ast.Loc{Line: 1}, 1)
	o.AddKidoku(ast.Loc{Line: 2}, 2)
	o.AddCode(ast.Nowhere, []byte{0x00})

	data, err := o.Generate(DefaultOptions())
	if err != nil { t.Fatal(err) }

	kidokuCount := int(binary.LittleEndian.Uint32(data[0x0c:0x10]))
	if kidokuCount != 3 { // 1 entrypoint + 2 kidoku
		t.Errorf("kidoku count: got %d, want 3", kidokuCount)
	}
}
