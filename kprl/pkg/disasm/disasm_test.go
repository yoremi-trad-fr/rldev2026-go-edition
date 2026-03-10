package disasm

import (
	"encoding/binary"
	"testing"
)

func TestReaderBasics(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	r := NewReader(data, 0, len(data), ModeRealLive)

	// Next
	b, err := r.Next()
	if err != nil || b != 0x01 {
		t.Errorf("Next() = %02x, err=%v", b, err)
	}

	// Peek
	b, err = r.Peek()
	if err != nil || b != 0x02 {
		t.Errorf("Peek() = %02x, want 0x02", b)
	}
	if r.Pos() != 1 {
		t.Errorf("Peek should not advance pos")
	}

	// Rollback
	r.Rollback(1)
	b, _ = r.Next()
	if b != 0x01 {
		t.Errorf("After Rollback, Next() = %02x, want 0x01", b)
	}
}

func TestReaderInt(t *testing.T) {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint32(data[0:], 0x12345678)
	binary.LittleEndian.PutUint16(data[4:], 0xABCD)

	r := NewReader(data, 0, len(data), ModeRealLive)

	v32, err := r.ReadInt32()
	if err != nil || v32 != 0x12345678 {
		t.Errorf("ReadInt32() = %x, err=%v", v32, err)
	}

	v16, err := r.ReadInt16()
	if err != nil || v16 != -0x5433 { // 0xABCD as signed int16
		t.Errorf("ReadInt16() = %x, err=%v", v16, err)
	}
}

func TestReaderExpression(t *testing.T) {
	// Test immediate integer: 0xff followed by LE int32
	data := make([]byte, 5)
	data[0] = 0xff
	binary.LittleEndian.PutUint32(data[1:], 42)

	r := NewReader(data, 0, len(data), ModeRealLive)
	expr, err := r.GetExpression()
	if err != nil {
		t.Fatalf("GetExpression() error: %v", err)
	}
	if expr != "42" {
		t.Errorf("GetExpression() = %q, want %q", expr, "42")
	}
}

func TestReaderExpectSuccess(t *testing.T) {
	data := []byte{'(', ')'}
	r := NewReader(data, 0, 2, ModeRealLive)
	if err := r.Expect('(', "test"); err != nil {
		t.Errorf("Expect('(') should succeed: %v", err)
	}
}

func TestReaderExpectFail(t *testing.T) {
	data := []byte{'X'}
	r := NewReader(data, 0, 1, ModeRealLive)
	err := r.Expect('(', "test")
	if err == nil {
		t.Error("Expect('(') should fail on 'X'")
	}
}

func TestReaderAtEnd(t *testing.T) {
	data := []byte{0x01}
	r := NewReader(data, 0, 1, ModeRealLive)
	if r.AtEnd() {
		t.Error("should not be at end initially")
	}
	r.Next()
	if !r.AtEnd() {
		t.Error("should be at end after reading 1 byte")
	}
}

func TestIsShiftJISLead(t *testing.T) {
	// Valid lead bytes
	for _, b := range []byte{0x81, 0x9f, 0xe0, 0xef, 0xf0, 0xfc} {
		if !isShiftJISLead(b) {
			t.Errorf("isShiftJISLead(0x%02x) = false, want true", b)
		}
	}
	// Invalid lead bytes
	for _, b := range []byte{0x20, 0x41, 0x7f, 0x80, 0xa0, 0xfd, 0xff} {
		if isShiftJISLead(b) {
			t.Errorf("isShiftJISLead(0x%02x) = true, want false", b)
		}
	}
}

func TestEngineModeString(t *testing.T) {
	if ModeRealLive.String() != "RealLive" {
		t.Errorf("ModeRealLive.String() = %q", ModeRealLive.String())
	}
	if ModeAvg2000.String() != "AVG2000" {
		t.Errorf("ModeAvg2000.String() = %q", ModeAvg2000.String())
	}
}

func TestVersionString(t *testing.T) {
	tests := []struct {
		v    Version
		want string
	}{
		{Version{1, 2, 0, 0}, "1.2"},
		{Version{1, 2, 7, 0}, "1.2.7"},
		{Version{1, 2, 7, 1}, "1.2.7.1"},
	}
	for _, tt := range tests {
		if got := tt.v.String(); got != tt.want {
			t.Errorf("Version%v.String() = %q, want %q", tt.v, got, tt.want)
		}
	}
}

func TestOpcodeString(t *testing.T) {
	op := Opcode{Type: 0, Module: 1, Function: 3, Overload: 0}
	got := op.String()
	if got != "0:001:00003,0" {
		t.Errorf("Opcode.String() = %q, want %q", got, "0:001:00003,0")
	}
}

func TestCommandText(t *testing.T) {
	cmd := Command{
		Kepago: []CommandElem{
			ElemString{Value: "goto("},
			ElemPointer{Offset: 42},
			ElemString{Value: ")"},
		},
	}
	got := cmd.Text()
	if got != "goto(@ptr_42)" {
		t.Errorf("Command.Text() = %q", got)
	}
}

func TestFuncRegistryLookup(t *testing.T) {
	reg := NewFuncRegistry()
	reg.Register("0:001:00003,0", FuncDef{
		Name:  "gosub",
		Flags: []FuncFlag{FlagIsJump, FlagIsCall},
	})

	def, ok := reg.Lookup("0:001:00003,0")
	if !ok {
		t.Fatal("expected to find gosub")
	}
	if def.Name != "gosub" {
		t.Errorf("got name %q", def.Name)
	}
	if !def.HasFlag(FlagIsJump) {
		t.Error("expected FlagIsJump")
	}
	if def.HasFlag(FlagIsGoto) {
		t.Error("did not expect FlagIsGoto")
	}
}

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()
	if !opts.SeparateStrings {
		t.Error("SeparateStrings should default to true")
	}
	if !opts.ControlCodes {
		t.Error("ControlCodes should default to true")
	}
	if opts.SrcExt != "org" {
		t.Errorf("SrcExt = %q, want %q", opts.SrcExt, "org")
	}
}
