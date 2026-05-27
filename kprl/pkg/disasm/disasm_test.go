package disasm

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoremi/rldev-go/pkg/binarray"
	"github.com/yoremi/rldev-go/pkg/bytecode"
	"github.com/yoremi/rldev-go/pkg/encoding"
	"github.com/yoremi/rldev-go/pkg/metadata"
	"github.com/yoremi/rldev-go/pkg/texttransforms"
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
	// Test immediate integer: '$' 0xff followed by LE int32.
	// In OCaml, 0xff (immediate int) is a get_expr_token, which is reachable
	// only after a '$' prefix at the start of a term.
	data := make([]byte, 6)
	data[0] = '$'
	data[1] = 0xff
	binary.LittleEndian.PutUint32(data[2:], 42)

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

func TestReadCommandPreservesFF01ControlTextout(t *testing.T) {
	data := []byte{0xff, 0x01, 0x00, 0x00}
	r := NewReader(data, 0, len(data), ModeRealLive)
	result := &DisassemblyResult{}
	opts := DefaultOptions()
	opts.ControlCodes = true

	if err := readCommand(r, &bytecode.FileHeader{}, result, opts); err != nil {
		t.Fatalf("readCommand() error: %v", err)
	}
	if len(result.Commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(result.Commands))
	}
	got, ok := result.Commands[0].Kepago[0].(ElemString)
	if !ok || got.Value != "raw #ff #01 endraw" {
		t.Fatalf("command = %#v, want raw #ff #01 endraw", result.Commands[0].Kepago)
	}
	if r.Pos() != 2 {
		t.Fatalf("reader pos = %d, want 2", r.Pos())
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

func TestKFNHintGotoToken(t *testing.T) {
	if !hasHintToken("fun gosub_with (store goto) <0:Jmp:00016, 0> (...)", "goto") {
		t.Fatal("store goto hint should expose a goto pointer")
	}
	if hasHintToken("fun goto_on (skip gotos) <0:Jmp:00003, 0> (...)", "goto") {
		t.Fatal("gotos table hint must not be treated as a single goto pointer")
	}
}

func TestReadFunctionGenericGotoPointer(t *testing.T) {
	data := []byte{
		'(',
		'a', 0x00,
		'$', 0x0b, '[',
		'$', 0xff, 0x01, 0x00, 0x00, 0x00,
		']',
		')',
		0x78, 0x56, 0x34, 0x12,
	}
	r := NewReader(data, 0, len(data), ModeRealLive)
	result := &DisassemblyResult{Pointers: make(map[int]bool)}
	reg := NewFuncRegistry()
	reg.Register("0:001:00016,0", FuncDef{
		Name:       "gosub_with",
		Flags:      []FuncFlag{FlagPushStore, FlagIsGoto},
		Prototypes: [][]ParamType{{ParamAny}},
	})
	opts := DefaultOptions()
	opts.FuncReg = reg

	op := Opcode{Type: 0, Module: 1, Function: 16, Overload: 0}
	if err := readFunction(r, result, 0, op, 1, opts); err != nil {
		t.Fatalf("readFunction() error: %v", err)
	}
	if r.Pos() != len(data) {
		t.Fatalf("reader pos = %d, want %d", r.Pos(), len(data))
	}
	if !result.Pointers[0x12345678] {
		t.Fatalf("pointer target was not registered: %#v", result.Pointers)
	}
	if len(result.Commands) != 1 || len(result.Commands[0].Kepago) != 2 {
		t.Fatalf("command = %#v", result.Commands)
	}
	got, ok := result.Commands[0].Kepago[1].(ElemString)
	if !ok {
		t.Fatalf("second elem = %#v", result.Commands[0].Kepago[1])
	}
	want := "gosub_with (special<0>(intL[1])) @@PTR=305419896@@"
	if got.Value != want {
		t.Fatalf("command = %q, want %q", got.Value, want)
	}
}

func TestReadSelectCond(t *testing.T) {
	data := []byte{
		'(',
		'(', '$', 0xff, 1, 0, 0, 0, ')',
		'1', '$', 0xff, 155, 0, 0, 0,
		'2',
		')',
	}
	r := NewReader(data, 0, len(data), ModeRealLive)
	got, err := readSelectCond(r)
	if err != nil {
		t.Fatalf("readSelectCond: %v", err)
	}
	want := "title(155) if 1; hide: "
	if got != want {
		t.Fatalf("readSelectCond = %q, want %q", got, want)
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

func TestDisassembleReadsMetadataTextTransform(t *testing.T) {
	var m metadata.Metadata
	meta := m.ToBytes("RLdev", 1.39, [4]byte{1, 2, 7, 0}, metadata.TransformWestern)
	dataOffset := 0x1d0 + len(meta)
	data := make([]byte, dataOffset)

	copy(data[0:], []byte("KPRL"))
	binary.LittleEndian.PutUint32(data[0x04:], 10002)
	binary.LittleEndian.PutUint32(data[0x08:], 0x1d0)
	binary.LittleEndian.PutUint32(data[0x14:], 0x1d0)
	binary.LittleEndian.PutUint32(data[0x20:], uint32(dataOffset))
	copy(data[0x1d0:], meta)

	result, err := Disassemble(binarray.FromBytes(data), DefaultOptions())
	if err != nil {
		t.Fatalf("Disassemble: %v", err)
	}
	if result.TextTransform != texttransforms.EncWestern {
		t.Fatalf("TextTransform = %v, want Western", result.TextTransform)
	}
}

func TestWriterConvertTextUsesWesternTransform(t *testing.T) {
	opts := DefaultOptions()
	opts.Encoding = "UTF-8"
	w := NewWriter("", opts)

	got := w.convertText("Je d\xcateste", texttransforms.EncWestern)
	want := "Je déteste"
	if got != want {
		t.Fatalf("convertText() = %q, want %q", got, want)
	}
}

func TestWriterEmitsVal0x2CDirective(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.SeparateStrings = false
	w := NewWriter(dir, opts)

	result := &DisassemblyResult{
		Mode: ModeRealLive,
		Header: bytecode.FileHeader{
			Int0x2C: 9,
		},
	}
	if err := w.WriteSource("SEEN0414.TXT", result); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "SEEN0414.org"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "#val_0x2c 9") {
		t.Fatalf("writer did not emit #val_0x2c directive:\n%s", string(data))
	}
}

func TestReadFuncArgsComplexTupleKeepsParenExpressionOperators(t *testing.T) {
	var data []byte
	addInt := func(v int32) {
		data = append(data, '$', 0xff)
		var buf [4]byte
		binary.LittleEndian.PutUint32(buf[:], uint32(v))
		data = append(data, buf[:]...)
	}

	data = append(data, '(', '(')
	addInt(0)
	addInt(3600)
	data = append(data, '\\', 0x02)
	addInt(2)
	data = append(data, '\\', 0x00)
	addInt(1400)
	data = append(data, '(')
	addInt(3600)
	data = append(data, '\\', 0x02)
	addInt(2)
	data = append(data, '\\', 0x00)
	addInt(1400)
	data = append(data, ')', '\\', 0x02)
	addInt(2)
	data = append(data, '$', 0x02, '[')
	addInt(0)
	data = append(data, ']', ')', ')')

	r := NewReader(data, 0, len(data), ModeRealLive)
	args, err := readFuncArgsCtx(r, 1, nil, nil)
	if err != nil {
		t.Fatalf("readFuncArgsCtx() error: %v", err)
	}
	if len(args) != 1 {
		t.Fatalf("args len = %d, want 1 (%v)", len(args), args)
	}

	wantPrefix := "(0, 3600 * 2 + 1400, (3600 * 2 + 1400) * 2, intC[0]) /* nested:"
	if !strings.HasPrefix(args[0], wantPrefix) {
		t.Fatalf("arg = %q, want prefix %q", args[0], wantPrefix)
	}
	if r.Pos() != len(data) {
		t.Fatalf("reader pos = %d, want %d", r.Pos(), len(data))
	}
}

func TestEscapeResourceLineText(t *testing.T) {
	tests := map[string]string{
		"plain":      "plain",
		"   leading": `\   leading`,
		"\tleading":  "\\\tleading",
		"<tag>":      `\<tag>`,
		"//comment":  `\//comment`,
	}
	for input, want := range tests {
		if got := escapeResourceLineText(input); got != want {
			t.Fatalf("escapeResourceLineText(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestEscapeUnsafeResourceBytes(t *testing.T) {
	tests := map[string]string{
		"plain":                        "plain",
		string([]byte{0x82, 0xa0}):     string([]byte{0x82, 0xa0}),
		string([]byte{0x84, 0x02}):     `\x{84}\x{02}`,
		string([]byte{'A', 0x01, 'B'}): `A\x{01}B`,
		string([]byte{0x84, '0', 'A'}): `\x{84}0A`,
	}
	for input, want := range tests {
		if got := escapeUnsafeResourceBytes(input); got != want {
			t.Fatalf("escapeUnsafeResourceBytes(% x) = %q, want %q", []byte(input), got, want)
		}
	}
}

func TestConvertTextSpeakerNameBypassesTransform(t *testing.T) {
	name, err := encoding.UTF8ToSJS(string(rune(0x58f0)))
	if err != nil {
		t.Fatal(err)
	}
	w := NewWriter("", Options{Encoding: "UTF-8"})
	got := w.convertText(`\{`+string(name)+`}*Sigh*`, texttransforms.EncWestern)
	if got != `\{`+string(rune(0x58f0))+`}*Sigh*` {
		t.Fatalf("speaker name transform = %q", got)
	}
}

func TestConvertTextSpeakerNameWithBraceTrailByte(t *testing.T) {
	name := "美佐枝"
	sjsName, err := encoding.UTF8ToSJS(name)
	if err != nil {
		t.Fatal(err)
	}
	if len(sjsName) == 0 || sjsName[len(sjsName)-1] != '}' {
		t.Fatalf("test fixture must end with byte 0x7d, got % x", sjsName)
	}
	w := NewWriter("", Options{Encoding: "UTF-8"})
	got := w.convertText(`\{`+string(sjsName)+`}text`, texttransforms.EncWestern)
	if got != `\{`+name+`}text` {
		t.Fatalf("speaker close inside SJIS trail byte: got %q", got)
	}
}
