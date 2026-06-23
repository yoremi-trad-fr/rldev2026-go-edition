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

func TestParseKFNContinuationPrototypes(t *testing.T) {
	src := strings.NewReader(`
module 010 = Str
fun zentohan <1:Str:00011, 1> (>str 'buf')
                                  (strC 'src', >str 'dst')
`)
	reg, err := ParseKFN(src)
	if err != nil {
		t.Fatalf("ParseKFN() error: %v", err)
	}
	def, ok := reg.LookupOpcode(Opcode{Type: 1, Module: 10, Function: 11, Overload: 1})
	if !ok {
		t.Fatal("zentohan opcode not registered")
	}
	if len(def.Prototypes) != 2 {
		t.Fatalf("prototype count = %d, want 2", len(def.Prototypes))
	}
	if got := def.Prototypes[1]; len(got) != 2 || got[0] != ParamStrC || got[1] != ParamStr {
		t.Fatalf("prototype[1] = %#v, want [ParamStrC ParamStr]", got)
	}
	if len(def.ParamFlags[1]) != 2 || len(def.ParamFlags[1][1]) != 1 || def.ParamFlags[1][1][0] != ParamReturn {
		t.Fatalf("prototype[1] return flags = %#v, want return on second parameter", def.ParamFlags[1])
	}
}

func TestParseKFNVersionBlocksChooseRealLiveGanName(t *testing.T) {
	src := strings.NewReader(`
module 071 = Obj
ver < 1.1
  fun objOfFileAnm <1:Obj:01003, 0> (intC, intC, strC, strC)
end
ver >= 1.1
  fun objOfFileGan <1:Obj:01003, 4> ('buf', strC 'filename', strC 'ganname')
                                      ('buf', strC 'filename', strC 'ganname', 'visible')
end
`)
	reg, err := ParseKFNForTarget(src, ModeRealLive, Version{1, 2, 3, 5})
	if err != nil {
		t.Fatalf("ParseKFNForTarget() error: %v", err)
	}
	def, ok := reg.LookupOpcodeForArgc(Opcode{Type: 1, Module: 71, Function: 1003, Overload: 0}, 4)
	if !ok {
		t.Fatal("opcode not found")
	}
	if def.Name != "objOfFileGan" {
		t.Fatalf("opcode name = %q, want objOfFileGan", def.Name)
	}
	if _, ok := reg.Lookup("1:071:01003,0"); ok {
		t.Fatal("inactive pre-1.1 objOfFileAnm entry was registered")
	}
}

func TestRealLive167KeepsWithCallOpcodes(t *testing.T) {
	kfnPath := filepath.Join("..", "..", "..", "KFN", "reallive.kfn")
	reg, err := LoadKFNForTarget(kfnPath, ModeRealLive, Version{1, 6, 7, 3})
	if err != nil {
		t.Fatal(err)
	}
	def, ok := reg.LookupOpcodeForArgc(Opcode{Type: 0, Module: 1, Function: 16, Overload: 0}, 1)
	if !ok {
		t.Fatal("gosub_with opcode was not registered for RealLive 1.6.7.3")
	}
	if def.Name != "gosub_with" {
		t.Fatalf("opcode name = %q, want gosub_with", def.Name)
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

func TestReadStringQuotedArgumentStopsBeforeAdjacentQuote(t *testing.T) {
	data := []byte("\"TITLE\"\"BODY\")")
	r := NewReader(data, 0, len(data), ModeRealLive)

	first, err := r.GetData()
	if err != nil {
		t.Fatalf("first GetData() error: %v", err)
	}
	if first != "'TITLE'" {
		t.Fatalf("first GetData() = %q, want 'TITLE'", first)
	}

	second, err := r.GetData()
	if err != nil {
		t.Fatalf("second GetData() error: %v", err)
	}
	if second != "'BODY'" {
		t.Fatalf("second GetData() = %q, want 'BODY'", second)
	}
	if b, err := r.Peek(); err != nil || b != ')' {
		t.Fatalf("next byte = 0x%02x, %v; want ')'", b, err)
	}
}

func TestGetDataParsesHyphenPrefixedSelectString(t *testing.T) {
	input := "-\" Test Name -\"}"
	r := NewReader([]byte(input), 0, len(input), ModeRealLive)

	got, err := r.GetData()
	if err != nil {
		t.Fatalf("GetData() error: %v", err)
	}
	if got != "'- Test Name -'" {
		t.Fatalf("GetData() = %q, want %q", got, "'- Test Name -'")
	}
	if b, err := r.Peek(); err != nil || b != '}' {
		t.Fatalf("next byte = 0x%02x, %v; want '}'", b, err)
	}
}

func TestReadStringQuotedArgumentKeepsInternalQuotes(t *testing.T) {
	tests := map[string]string{
		"\"Say \"Hello.\"\"\n": "'Say \"Hello.\"'",
		"\"Always add \"and a toilet seat cover\" to the end\"\n": "'Always add \"and a toilet seat cover\" to the end'",
	}

	for input, want := range tests {
		r := NewReader([]byte(input), 0, len(input), ModeRealLive)
		got, err := r.GetData()
		if err != nil {
			t.Fatalf("GetData(%q) error: %v", input, err)
		}
		if got != want {
			t.Fatalf("GetData(%q) = %q, want %q", input, got, want)
		}
		if b, err := r.Peek(); err != nil || b != '\n' {
			t.Fatalf("next byte = 0x%02x, %v; want newline", b, err)
		}
	}
}

func TestReadFuncArgsQuotedStringBeforeExpressionArg(t *testing.T) {
	var data []byte
	data = append(data, '(')
	data = append(data, []byte("\"DUMMY\"")...)
	data = append(data, '$', 0xff)
	var imm [4]byte
	binary.LittleEndian.PutUint32(imm[:], 157)
	data = append(data, imm[:]...)
	data = append(data, []byte("\"TOMOYO_ME_ALL_A\"")...)
	data = append(data, ')')

	r := NewReader(data, 0, len(data), ModeRealLive)
	args, err := readFuncArgsCtx(r, 1, []ParamType{ParamStrC, ParamIntC, ParamStrC}, nil)
	if err != nil {
		t.Fatalf("readFuncArgsCtx() error: %v", err)
	}
	want := []string{"'DUMMY'", "157", "'TOMOYO_ME_ALL_A'"}
	if len(args) != len(want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q (all args %#v)", i, args[i], want[i], args)
		}
	}
	if r.Pos() != len(data) {
		t.Fatalf("reader pos = %d, want %d", r.Pos(), len(data))
	}
}

func TestReadFuncArgsSplitsAdjacentPrototypeStrings(t *testing.T) {
	var data []byte
	addInt := func(v int32) {
		data = append(data, '$', 0xff)
		var imm [4]byte
		binary.LittleEndian.PutUint32(imm[:], uint32(v))
		data = append(data, imm[:]...)
	}

	data = append(data, '(')
	addInt(151)
	data = append(data, ',')
	data = append(data, []byte("NYEF_6001_01\"NYEF_KD01\"")...)
	addInt(1)
	data = append(data, ',')
	addInt(400)
	data = append(data, ',')
	addInt(150)
	data = append(data, ')')

	r := NewReader(data, 0, len(data), ModeRealLive)
	proto := []ParamType{ParamAny, ParamStrC, ParamStrC, ParamAny, ParamAny, ParamAny}
	args, err := readFuncArgsCtx(r, 6, proto, nil)
	if err != nil {
		t.Fatalf("readFuncArgsCtx() error: %v", err)
	}
	want := []string{"151", "'NYEF_6001_01'", "'NYEF_KD01'", "1", "400", "150"}
	if len(args) != len(want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q (all args %#v)", i, args[i], want[i], args)
		}
	}
}

func TestReadCommandPrintMarkerAtCommandBoundary(t *testing.T) {
	for _, marker := range [][]byte{
		[]byte("###PRINT("),
		{'#', '#', '#', 'P', 'R', 0x01, 0x00, 'T', '('},
	} {
		var data []byte
		addInt := func(v int32) {
			data = append(data, '$', 0xff)
			var buf [4]byte
			binary.LittleEndian.PutUint32(buf[:], uint32(v))
			data = append(data, buf[:]...)
		}
		data = append(data, marker...)
		data = append(data, '$', 0x12, '[')
		addInt(1020)
		data = append(data, ']', ')', 0x00)

		r := NewReader(data, 0, len(data), ModeRealLive)
		result := &DisassemblyResult{SeenMap: NewSeenMap()}
		opts := DefaultOptions()
		if err := readCommand(r, &bytecode.FileHeader{}, result, opts); err != nil {
			t.Fatalf("readCommand(% x) error: %v", marker, err)
		}
		if len(result.ResStrs) != 1 {
			t.Fatalf("resources = %#v, want one", result.ResStrs)
		}
		if got, want := result.ResStrs[0], `\s{strS[1020]}`; got != want {
			t.Fatalf("resource = %q, want %q", got, want)
		}
		if strings.Contains(result.Commands[0].Text(), "op<35") {
			t.Fatalf("print marker was emitted as opcode: %q", result.Commands[0].Text())
		}
	}
}

func TestReadFuncArgsSplitsUnaryMinusArgsAfterGreedyExpression(t *testing.T) {
	var data []byte
	addInt := func(v int32) {
		data = append(data, '$', 0xff)
		var buf [4]byte
		binary.LittleEndian.PutUint32(buf[:], uint32(v))
		data = append(data, buf[:]...)
	}

	data = append(data, '(')
	addInt(2)
	addInt(992)
	data = append(data, '\\', 0x01)
	addInt(5048)
	data = append(data, '\\', 0x01)
	addInt(320)
	addInt(86000)
	data = append(data, ')')

	r := NewReader(data, 0, len(data), ModeRealLive)
	args, err := readFuncArgsCtx(r, 4, nil, nil)
	if err != nil {
		t.Fatalf("readFuncArgsCtx() error: %v", err)
	}
	want := []string{"2", "992", "-5048 - 320", "86000"}
	if strings.Join(args, "|") != strings.Join(want, "|") {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestReadFuncArgsSplitsMultipleUnaryMinusArgs(t *testing.T) {
	var data []byte
	addInt := func(v int32) {
		data = append(data, '$', 0xff)
		var buf [4]byte
		binary.LittleEndian.PutUint32(buf[:], uint32(v))
		data = append(data, buf[:]...)
	}

	data = append(data, '(')
	addInt(84)
	for _, v := range []int32{24, 12, 24, 12} {
		data = append(data, '\\', 0x01)
		addInt(v)
	}
	data = append(data, ')')

	r := NewReader(data, 0, len(data), ModeRealLive)
	args, err := readFuncArgsCtx(r, 3, nil, nil)
	if err != nil {
		t.Fatalf("readFuncArgsCtx() error: %v", err)
	}
	want := []string{"84", "-24 - 12", "-24 - 12"}
	if strings.Join(args, "|") != strings.Join(want, "|") {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestReadFuncArgsStructuralTupleAfterScalarArg(t *testing.T) {
	var data []byte
	addInt := func(v int32) {
		data = append(data, '$', 0xff)
		var buf [4]byte
		binary.LittleEndian.PutUint32(buf[:], uint32(v))
		data = append(data, buf[:]...)
	}
	addLine := func(line uint16) {
		data = append(data, '\n')
		var buf [2]byte
		binary.LittleEndian.PutUint16(buf[:], line)
		data = append(data, buf[:]...)
	}

	data = append(data, '(')
	addInt(0)
	addLine(260)
	data = append(data, '(')
	data = append(data, []byte("BT_SE_A00A")...)
	addInt(100)
	addInt(100)
	data = append(data, ')')
	addLine(261)
	data = append(data, '(')
	data = append(data, []byte("BT_SE_A00A")...)
	addInt(100)
	addInt(100)
	data = append(data, ')')
	data = append(data, ')')

	r := NewReader(data, 0, len(data), ModeRealLive)
	args, err := readFuncArgsCtx(r, 1, nil, nil)
	if err != nil {
		t.Fatalf("readFuncArgsCtx() error: %v", err)
	}
	if len(args) != 3 {
		t.Fatalf("args len = %d, want 3 (%v)", len(args), args)
	}
	if args[0] != "0" {
		t.Fatalf("args[0] = %q, want 0", args[0])
	}
	wantPrefix := "('BT_SE_A00A', 100, 100) /* nested:"
	for i := 1; i < len(args); i++ {
		if !strings.HasPrefix(args[i], wantPrefix) {
			t.Fatalf("args[%d] = %q, want prefix %q", i, args[i], wantPrefix)
		}
	}
	if r.Pos() != len(data) {
		t.Fatalf("reader pos = %d, want %d", r.Pos(), len(data))
	}
}

func TestParseParamProtosKeepsOverloads(t *testing.T) {
	protos, flags := parseParamProtos("fun ResetTimer <1:Sys:00110, 1> ('counter') ()")
	if len(protos) != 2 {
		t.Fatalf("prototype count = %d, want 2", len(protos))
	}
	if len(protos[0]) != 1 || len(flags[0]) != 1 {
		t.Fatalf("first prototype = %#v / %#v, want one arg", protos[0], flags[0])
	}
	if len(protos[1]) != 0 || len(flags[1]) != 0 {
		t.Fatalf("second prototype = %#v / %#v, want empty", protos[1], flags[1])
	}
}

func TestMergeSkipsVisibleDebugLines(t *testing.T) {
	result := &DisassemblyResult{
		ResStrs: []string{`\{Sunohara}`},
		Commands: []Command{
			{CType: "textout", Kepago: []CommandElem{ElemString{Value: "#res<0000>"}}},
			{CType: "dbline", Kepago: []CommandElem{ElemString{Value: "#line 113"}}},
		},
	}

	if addTextoutFails(result, `\size{40}`) {
		t.Fatal("addTextoutFails returned true, want merge into previous resource")
	}
	if got, want := result.ResStrs[0], `\{Sunohara}\size{40}`; got != want {
		t.Fatalf("merged resource = %q, want %q", got, want)
	}
}

func TestMergeStopsAtHiddenKidoku(t *testing.T) {
	result := &DisassemblyResult{
		ResStrs: []string{"first"},
		Commands: []Command{
			{CType: "textout", Kepago: []CommandElem{ElemString{Value: "#res<0000>"}}},
			{Hidden: true, CType: "kidoku", Kepago: []CommandElem{ElemString{Value: "{- kidoku 001 -}"}}},
		},
	}

	if !addTextoutFails(result, "second") {
		t.Fatal("addTextoutFails returned false, want hidden kidoku to block merge")
	}
	if got, want := result.ResStrs[0], "first"; got != want {
		t.Fatalf("resource after blocked merge = %q, want %q", got, want)
	}
}

func TestReadCommandShowsKidokuWithoutDebugLines(t *testing.T) {
	data := []byte{'@', 0x00, 0x00}
	r := NewReader(data, 0, len(data), ModeRealLive)
	hdr := &bytecode.FileHeader{KidokuLnums: []int32{42}}
	result := &DisassemblyResult{}

	if err := readCommand(r, hdr, result, Options{}); err != nil {
		t.Fatalf("readCommand error: %v", err)
	}
	if len(result.Commands) != 1 {
		t.Fatalf("command count = %d, want 1", len(result.Commands))
	}
	cmd := result.Commands[0]
	if cmd.Hidden {
		t.Fatal("regular kidoku marker was hidden without -g")
	}
	if got, want := cmd.Text(), "{- kidoku 000 line 42 -}"; got != want {
		t.Fatalf("kidoku text = %q, want %q", got, want)
	}
}

func TestReadCommandHidesCompactLineWithoutDebugSymbols(t *testing.T) {
	data := []byte{'\n', 0x81, 0x00}
	r := NewReader(data, 0, len(data), ModeRealLive)
	result := &DisassemblyResult{}

	if err := readCommand(r, &bytecode.FileHeader{}, result, Options{}); err != nil {
		t.Fatalf("readCommand error: %v", err)
	}
	if len(result.Commands) != 1 {
		t.Fatalf("command count = %d, want 1", len(result.Commands))
	}
	cmd := result.Commands[0]
	if !cmd.Hidden {
		t.Fatal("compact line marker should be hidden without -g")
	}
	if got, want := cmd.Text(), "{- line 129 -}"; got != want {
		t.Fatalf("line text = %q, want %q", got, want)
	}
}

func TestReadCommandShowsCompactLineWithDebugSymbols(t *testing.T) {
	data := []byte{'\n', 0x81, 0x00}
	r := NewReader(data, 0, len(data), ModeRealLive)
	result := &DisassemblyResult{}

	if err := readCommand(r, &bytecode.FileHeader{}, result, Options{ReadDebugSymbols: true}); err != nil {
		t.Fatalf("readCommand error: %v", err)
	}
	if len(result.Commands) != 1 {
		t.Fatalf("command count = %d, want 1", len(result.Commands))
	}
	cmd := result.Commands[0]
	if cmd.Hidden {
		t.Fatal("compact line marker was hidden with -g")
	}
	if got, want := cmd.Text(), "#line 129"; got != want {
		t.Fatalf("line text = %q, want %q", got, want)
	}
}

func TestReadSelectKeepsQuotedContinuations(t *testing.T) {
	data := []byte(`{"ONE"A,"TWO"B,"THREE"C}`)
	r := NewReader(data, 0, len(data), ModeRealLive)
	result := &DisassemblyResult{}
	cmd := Command{}
	opts := Options{SeparateStrings: true}

	if err := readSelect(r, result, &cmd, Opcode{Function: 1}, 3, opts); err != nil {
		t.Fatalf("readSelect error: %v", err)
	}
	if got, want := result.ResStrs, []string{"ONEA", "TWOB", "THREEC"}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("resources = %#v, want %#v", got, want)
	}
	if len(result.Commands) != 1 {
		t.Fatalf("command count = %d, want 1", len(result.Commands))
	}
	got := result.Commands[0].Kepago[0].(ElemString).Value
	if want := "select(#res<0000>, #res<0001>, #res<0002>)"; got != want {
		t.Fatalf("select command = %q, want %q", got, want)
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

func TestFuncRegistryLookupOpcodeForArgcPrefersMatchingArity(t *testing.T) {
	reg := NewFuncRegistry()
	reg.Register("1:072:01003,2", FuncDef{
		Name: "objBgOfFileAnm",
		Prototypes: [][]ParamType{
			{ParamAny, ParamStrC},
			{ParamAny, ParamStrC, ParamAny, ParamAny, ParamAny},
			{ParamAny, ParamStrC, ParamStrC, ParamAny, ParamAny, ParamAny},
		},
	})
	reg.Register("1:072:01003,4", FuncDef{
		Name: "objBgOfFileGan",
		Prototypes: [][]ParamType{
			{ParamAny, ParamStrC, ParamStrC},
			{ParamAny, ParamStrC, ParamStrC, ParamAny},
			{ParamAny, ParamStrC, ParamStrC, ParamAny, ParamAny, ParamAny},
		},
	})

	op := Opcode{Type: 1, Module: 72, Function: 1003, Overload: 2}
	def, ok := reg.LookupOpcodeForArgc(op, 3)
	if !ok {
		t.Fatal("3-arg opcode not found")
	}
	if def.Name != "objBgOfFileGan" {
		t.Fatalf("3-arg opcode = %q, want objBgOfFileGan", def.Name)
	}

	def, ok = reg.LookupOpcodeForArgc(op, 6)
	if !ok {
		t.Fatal("6-arg opcode not found")
	}
	if def.Name != "objBgOfFileAnm" {
		t.Fatalf("6-arg opcode = %q, want objBgOfFileAnm", def.Name)
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

func TestKFNDisplayAliasSkipsFakeParams(t *testing.T) {
	src := strings.NewReader(`
module 000 = Sys
fun __gc1 GetCursorPos <1:Sys:00133, 0> (int 'x', int 'y', int 'button1', int 'button2')
fun __shkud ShakeScreen <1:012:01100, 0> (='DOWNUP', 'amount')
`)
	reg, err := ParseKFN(src)
	if err != nil {
		t.Fatalf("ParseKFN() error: %v", err)
	}

	def, ok := reg.Lookup("1:000:00133,0")
	if !ok {
		t.Fatal("GetCursorPos opcode not registered")
	}
	if def.Name != "GetCursorPos" {
		t.Fatalf("display name = %q, want GetCursorPos", def.Name)
	}

	def, ok = reg.Lookup("1:012:01100,0")
	if !ok {
		t.Fatal("ShakeScreen opcode not registered")
	}
	if def.Name != "__shkud" {
		t.Fatalf("fake-param alias should not be used, got %q", def.Name)
	}
}

func TestLittleBustersExShk00010UsesNamedZeroArgFunction(t *testing.T) {
	kfnPath := filepath.Join("..", "..", "..", "KFN", "reallive.kfn")
	reg, err := LoadKFNForTarget(kfnPath, ModeRealLive, Version{1, 5, 2, 4})
	if err != nil {
		t.Fatalf("LoadKFNForTarget() error: %v", err)
	}

	op := Opcode{Type: 1, Module: 13, Function: 10, Overload: 0}
	def, ok := reg.LookupOpcodeForArgc(op, 0)
	if !ok {
		t.Fatal("Shk:00010 zero-arg opcode not found")
	}
	if def.Name != "__shk_00010" {
		t.Fatalf("Shk:00010 name = %q, want __shk_00010", def.Name)
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

func TestReadFunctionReturnParamRendersAssignment(t *testing.T) {
	var data []byte
	addInt := func(v int32) {
		data = append(data, '$', 0xff)
		var buf [4]byte
		binary.LittleEndian.PutUint32(buf[:], uint32(v))
		data = append(data, buf[:]...)
	}
	addStrS := func(idx int32) {
		data = append(data, '$', 0x12, '[')
		addInt(idx)
		data = append(data, ']')
	}
	data = append(data, '(')
	addStrS(1)
	addStrS(1004)
	addInt(4)
	data = append(data, ')')

	r := NewReader(data, 0, len(data), ModeRealLive)
	result := &DisassemblyResult{}
	reg := NewFuncRegistry()
	reg.Register("1:010:00005,1", FuncDef{
		Name:       "strsub",
		Prototypes: [][]ParamType{{ParamStr, ParamStrC, ParamIntC}},
		ParamFlags: [][][]ParamFlag{{{ParamReturn}, nil, nil}},
	})
	opts := DefaultOptions()
	opts.FuncReg = reg

	op := Opcode{Type: 1, Module: 10, Function: 5, Overload: 0}
	if err := readFunction(r, result, 0, op, 3, opts); err != nil {
		t.Fatalf("readFunction() error: %v", err)
	}
	if len(result.Commands) != 1 {
		t.Fatalf("commands = %#v", result.Commands)
	}
	if got, want := result.Commands[0].Text(), "strS[1] = strsub(strS[1004], 4)"; got != want {
		t.Fatalf("command = %q, want %q", got, want)
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

func TestWriterConvertHeaderTextUsesWesternTransform(t *testing.T) {
	w := NewWriter("", Options{Encoding: "UTF-8"})

	got := w.convertHeaderText("\xaal\xc9ve", texttransforms.EncWestern)
	if got != "Élève" {
		t.Fatalf("convertHeaderText() = %q, want %q", got, "Élève")
	}
}

func TestWriterConvertHeaderTextKeepsNativeNameWithWesternPrefixLead(t *testing.T) {
	name := "河"
	sjsName, err := encoding.UTF8ToSJS(name)
	if err != nil {
		t.Fatal(err)
	}
	if len(sjsName) == 0 || sjsName[0] != 0x89 {
		t.Fatalf("test fixture must start with byte 0x89, got % x", sjsName)
	}

	w := NewWriter("", Options{Encoding: "UTF-8"})
	got := w.convertHeaderText(string(sjsName), texttransforms.EncWestern)
	if got != name {
		t.Fatalf("convertHeaderText() = %q, want %q", got, name)
	}
}

func TestReadStrAssignAcceptsWesternSingleByteText(t *testing.T) {
	data := []byte{
		'(',
		'$', 0x12, '[', '$', 0xff, 0x0e, 0x06, 0x00, 0x00, ']',
		',',
		0x8e, 0xfc, 0x88, 0xcd, 0x82, 0x52, 0xb8, 0x82, 0x52,
		0x82, 0xc9, 0x83, 0x5f, 0x83, 0x81, 0x81, 0x5b, 0x83, 0x57,
		')',
	}

	r := NewReader(data, 0, len(data), ModeRealLive)
	result := &DisassemblyResult{TextTransform: texttransforms.EncWestern}
	opts := DefaultOptions()
	opts.Encoding = "UTF-8"
	r.SetContext(result, &opts)

	op := Opcode{Type: 1, Module: 10, Function: 0, Overload: 0}
	if err := readFunction(r, result, 0, op, 2, opts); err != nil {
		t.Fatalf("readFunction() error: %v", err)
	}
	if r.Pos() != len(data) {
		t.Fatalf("reader stopped at 0x%x, want 0x%x", r.Pos(), len(data))
	}
	if len(result.Commands) != 1 {
		t.Fatalf("commands = %#v", result.Commands)
	}

	got := formatCommand(result.Commands[0], nil, opts, result)
	want := "strS[1550] = '周囲３×３にダメージ'"
	if got != want {
		t.Fatalf("command = %q, want %q", got, want)
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

func TestConvertTextSpeakerNameUsesWesternTransform(t *testing.T) {
	w := NewWriter("", Options{Encoding: "UTF-8"})
	got := w.convertText("\\{Homme \xc3g\xca}Salut", texttransforms.EncWestern)
	if got != `\{Homme âgé}Salut` {
		t.Fatalf("speaker name transform = %q", got)
	}
}

func TestConvertTextNativeSpeakerNameWithWesternPrefixLead(t *testing.T) {
	name := "河"
	sjsName, err := encoding.UTF8ToSJS(name)
	if err != nil {
		t.Fatal(err)
	}
	if len(sjsName) == 0 || sjsName[0] != 0x89 {
		t.Fatalf("test fixture must start with byte 0x89, got % x", sjsName)
	}
	w := NewWriter("", Options{Encoding: "UTF-8"})
	got := w.convertText(`\{`+string(sjsName)+`}text`, texttransforms.EncWestern)
	if got != `\{`+name+`}text` {
		t.Fatalf("native speaker name transform = %q", got)
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
