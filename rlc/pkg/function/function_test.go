package function

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/yoremi/rldev-go/rlc/pkg/ast"
	"github.com/yoremi/rldev-go/rlc/pkg/kfn"
)

// ============================================================
// Parameter serialization tests
// ============================================================

func TestSerializeEmpty(t *testing.T) {
	s := SerializeParams(nil)
	if s != "" { t.Errorf("got %q", s) }
}

func TestSerializeString(t *testing.T) {
	s := SerializeParams([]AsmParam{{Kind: AsmString, Code: "$\x0b[\xff\x05\x00\x00\x00]"}})
	if s != "$\x0b[\xff\x05\x00\x00\x00]" { t.Errorf("got %q", s) }
}

func TestSerializeInteger(t *testing.T) {
	s := SerializeParams([]AsmParam{{Kind: AsmInteger, Code: "$\xff\x2a\x00\x00\x00"}})
	if s != "$\xff\x2a\x00\x00\x00" { t.Errorf("got %q", s) }
}

func TestSerializeIntegerWithComma(t *testing.T) {
	// Integer starting with \ after another integer → needs comma
	p := []AsmParam{
		{Kind: AsmInteger, Code: "$\xff\x01\x00\x00\x00"},
		{Kind: AsmInteger, Code: "\\$\xff\x02\x00\x00\x00"},
	}
	s := SerializeParams(p)
	if !strings.Contains(s, ",") {
		t.Error("expected comma before unary operator")
	}
}

func TestSerializeIntegerNoCommaAfterList(t *testing.T) {
	// Integer starting with \ after a List → no comma
	p := []AsmParam{
		{Kind: AsmList, Items: []AsmParam{{Kind: AsmInteger, Code: "x"}}},
		{Kind: AsmInteger, Code: "\\y"},
	}
	s := SerializeParams(p)
	if strings.Contains(s, ",(") || strings.HasSuffix(s, ",") {
		t.Error("should not have comma after list before unary")
	}
}

func TestSerializeList(t *testing.T) {
	p := []AsmParam{{Kind: AsmList, Items: []AsmParam{
		{Kind: AsmInteger, Code: "a"},
		{Kind: AsmInteger, Code: "b"},
	}}}
	s := SerializeParams(p)
	if s != "(ab)" { t.Errorf("got %q", s) }
}

func TestSerializeLiteral(t *testing.T) {
	s := SerializeParams([]AsmParam{{Kind: AsmLiteral, Code: "hello"}})
	if s != "hello" { t.Errorf("got %q", s) }
}

func TestSerializeLiteralEmpty(t *testing.T) {
	s := SerializeParams([]AsmParam{{Kind: AsmLiteral, Code: ""}})
	if s != "\"\"" { t.Errorf("got %q", s) }
}

func TestSerializeLiteralComma(t *testing.T) {
	// Two consecutive literals need a comma between them
	p := []AsmParam{
		{Kind: AsmLiteral, Code: "foo"},
		{Kind: AsmLiteral, Code: "bar"},
	}
	s := SerializeParams(p)
	if s != "foo,bar" { t.Errorf("got %q", s) }
}

func TestSerializeSpecialSmallID(t *testing.T) {
	p := []AsmParam{{Kind: AsmSpecial, SpecID: 5, Items: []AsmParam{
		{Kind: AsmInteger, Code: "x"},
	}}}
	s := SerializeParams(p)
	// Should be: a\x05(x)
	if len(s) < 2 || s[0] != 'a' || s[1] != 5 {
		t.Errorf("small special: got %q", s)
	}
	if !strings.Contains(s, "(x)") {
		t.Errorf("expected parens: got %q", s)
	}
}

func TestSerializeSpecialLargeID(t *testing.T) {
	// ID = (2 << 8) | 10 = 522 → b0=10, b1=(2-1)=1
	id := (2 << 8) | 10
	p := []AsmParam{{Kind: AsmSpecial, SpecID: id, Items: []AsmParam{
		{Kind: AsmInteger, Code: "y"},
	}}}
	s := SerializeParams(p)
	// Should start with a<10>a<1>
	if len(s) < 4 || s[0] != 'a' || s[2] != 'a' {
		t.Errorf("large special: got bytes %v", []byte(s)[:min(len(s), 6)])
	}
}

func TestSerializeSpecialNoParens(t *testing.T) {
	p := []AsmParam{{Kind: AsmSpecial, SpecID: 3, NoParens: true, Items: []AsmParam{
		{Kind: AsmInteger, Code: "x"},
	}}}
	s := SerializeParams(p)
	// Should NOT have parens wrapping children
	if strings.Contains(s, "(") {
		t.Errorf("noparens special should not have parens: got %q", s)
	}
}

// ============================================================
// Prototype length tests
// ============================================================

func TestGetPrototypeLengthsUndefined(t *testing.T) {
	fd := &kfn.FuncDef{Prototypes: []kfn.Prototype{
		{Defined: false},
	}}
	lens := GetPrototypeLengths(fd)
	if lens[0].Min != -1 || lens[0].Max != -1 {
		t.Errorf("undefined: got %+v", lens[0])
	}
}

func TestGetPrototypeLengthsEmpty(t *testing.T) {
	fd := &kfn.FuncDef{Prototypes: []kfn.Prototype{
		{Defined: true, Params: nil},
	}}
	lens := GetPrototypeLengths(fd)
	if lens[0].Min != 0 || lens[0].Max != 0 {
		t.Errorf("empty: got %+v", lens[0])
	}
}

func TestGetPrototypeLengthsFixed(t *testing.T) {
	fd := &kfn.FuncDef{Prototypes: []kfn.Prototype{
		{Defined: true, Params: []kfn.Parameter{
			{Type: kfn.PIntC},
			{Type: kfn.PIntC},
		}},
	}}
	lens := GetPrototypeLengths(fd)
	if lens[0].Min != 2 || lens[0].Max != 2 {
		t.Errorf("fixed(2): got %+v", lens[0])
	}
}

func TestGetPrototypeLengthsOptional(t *testing.T) {
	fd := &kfn.FuncDef{Prototypes: []kfn.Prototype{
		{Defined: true, Params: []kfn.Parameter{
			{Type: kfn.PIntC},
			{Type: kfn.PIntC, Flags: []kfn.ParamFlag{kfn.FOptional}},
		}},
	}}
	lens := GetPrototypeLengths(fd)
	if lens[0].Min != 1 || lens[0].Max != 2 {
		t.Errorf("opt: got %+v", lens[0])
	}
}

func TestGetPrototypeLengthsArgc(t *testing.T) {
	fd := &kfn.FuncDef{Prototypes: []kfn.Prototype{
		{Defined: true, Params: []kfn.Parameter{
			{Type: kfn.PIntC},
			{Type: kfn.PIntC, Flags: []kfn.ParamFlag{kfn.FArgc}},
		}},
	}}
	lens := GetPrototypeLengths(fd)
	if lens[0].Min != -1 {
		t.Errorf("argc: got %+v", lens[0])
	}
}

// ============================================================
// Overload selection tests
// ============================================================

func TestChooseOverloadSingle(t *testing.T) {
	fd := &kfn.FuncDef{Ident: "test", Prototypes: []kfn.Prototype{
		{Defined: true, Params: []kfn.Parameter{{Type: kfn.PIntC}}},
	}}
	idx, err := ChooseOverloadByCount(fd, 1)
	if err != nil { t.Fatal(err) }
	if idx != 0 { t.Errorf("got %d", idx) }
}

func TestChooseOverloadMultiple(t *testing.T) {
	fd := &kfn.FuncDef{Ident: "jump", Prototypes: []kfn.Prototype{
		{Defined: true, Params: []kfn.Parameter{{Type: kfn.PIntC}}},
		{Defined: true, Params: []kfn.Parameter{{Type: kfn.PIntC}, {Type: kfn.PIntC}}},
	}}
	idx, err := ChooseOverloadByCount(fd, 1)
	if err != nil { t.Fatal(err) }
	if idx != 0 { t.Errorf("argc=1: got %d, want 0", idx) }

	idx, err = ChooseOverloadByCount(fd, 2)
	if err != nil { t.Fatal(err) }
	if idx != 1 { t.Errorf("argc=2: got %d, want 1", idx) }
}

func TestChooseOverloadFallbackArb(t *testing.T) {
	fd := &kfn.FuncDef{Ident: "test", Prototypes: []kfn.Prototype{
		{Defined: true, Params: []kfn.Parameter{{Type: kfn.PIntC}}},
		{Defined: false}, // arbitrary
	}}
	idx, err := ChooseOverloadByCount(fd, 99)
	if err != nil { t.Fatal(err) }
	if idx != 1 { t.Errorf("got %d, want 1 (arbitrary)", idx) }
}

func TestChooseOverloadErrorNoArb(t *testing.T) {
	// With multiple prototypes and no arbitrary, mismatched argc → error
	fd := &kfn.FuncDef{Ident: "test", Prototypes: []kfn.Prototype{
		{Defined: true, Params: []kfn.Parameter{{Type: kfn.PIntC}}},
		{Defined: true, Params: []kfn.Parameter{{Type: kfn.PIntC}, {Type: kfn.PIntC}}},
	}}
	_, err := ChooseOverloadByCount(fd, 99)
	if err == nil { t.Error("expected error") }
}

func TestChooseOverloadByParams(t *testing.T) {
	protos := []kfn.Prototype{
		{Defined: true, Params: []kfn.Parameter{{Type: kfn.PIntC}}},
		{Defined: true, Params: []kfn.Parameter{{Type: kfn.PIntC}, {Type: kfn.PIntC}}},
	}
	idx, err := ChooseOverloadByParams(protos, []ast.Param{
		ast.SimpleParam{Expr: ast.IntLit{Val: 1}},
		ast.SimpleParam{Expr: ast.IntLit{Val: 2}},
	})
	if err != nil { t.Fatal(err) }
	if idx != 1 { t.Errorf("got %d, want 1", idx) }
}

// ============================================================
// Assembly tests
// ============================================================

func TestAssembleBasic(t *testing.T) {
	fd := &kfn.FuncDef{
		Ident: "goto", OpType: 0, OpModule: 1, OpCode: 0,
		Prototypes: []kfn.Prototype{{Defined: true}},
	}
	result, err := Assemble(fd, nil, 0, "")
	if err != nil { t.Fatal(err) }
	if len(result.Code) < 8 { t.Fatalf("code too short: %d", len(result.Code)) }
	if result.Code[0] != '#' { t.Error("missing opcode prefix") }
	if result.Append != nil { t.Error("unexpected append") }
}

func TestAssembleWithParams(t *testing.T) {
	fd := &kfn.FuncDef{
		Ident: "test", OpType: 0, OpModule: 1, OpCode: 5,
		Prototypes: []kfn.Prototype{{Defined: true, Params: []kfn.Parameter{{Type: kfn.PIntC}}}},
	}
	params := []AsmParam{{Kind: AsmInteger, Code: "$\xff\x2a\x00\x00\x00"}}
	result, err := Assemble(fd, params, 0, "")
	if err != nil { t.Fatal(err) }
	// Should contain parens around params
	s := string(result.Code)
	if !strings.Contains(s, "(") || !strings.Contains(s, ")") {
		t.Error("expected parens around params")
	}
}

func TestAssembleOpcodeEncoding(t *testing.T) {
	fd := &kfn.FuncDef{
		Ident: "test", OpType: 0, OpModule: 3, OpCode: 42,
		Prototypes: []kfn.Prototype{{Defined: true}},
	}
	result, _ := Assemble(fd, nil, 0, "")
	// Check opcode bytes: # type module code_lo code_hi argc_lo argc_hi overload
	if result.Code[0] != '#' { t.Error("prefix") }
	if result.Code[1] != 0 { t.Errorf("type: %d", result.Code[1]) }
	if result.Code[2] != 3 { t.Errorf("module: %d", result.Code[2]) }
	code := binary.LittleEndian.Uint16(result.Code[3:5])
	if code != 42 { t.Errorf("code: %d", code) }
}

func TestAssemblePushStoreReturn(t *testing.T) {
	fd := &kfn.FuncDef{
		Ident: "intout", OpType: 0, OpModule: 3, OpCode: 1,
		Flags: []kfn.FuncFlag{kfn.FlagPushStore},
		Prototypes: []kfn.Prototype{{Defined: true, Params: []kfn.Parameter{{Type: kfn.PIntC}}}},
	}
	params := []AsmParam{{Kind: AsmInteger, Code: "x"}}
	result, err := Assemble(fd, params, 0, "$\x0b[\xff\x00\x00\x00\x00]")
	if err != nil { t.Fatal(err) }
	// PushStore with non-store returnVal → should have append code
	if result.Append == nil {
		t.Error("expected append for PushStore with non-store return")
	}
}

func TestAssembleReturnInject(t *testing.T) {
	fd := &kfn.FuncDef{
		Ident: "test", OpType: 0, OpModule: 1, OpCode: 0,
		Prototypes: []kfn.Prototype{{Defined: true, Params: []kfn.Parameter{
			{Type: kfn.PIntC},
			{Type: kfn.PIntV, Flags: []kfn.ParamFlag{kfn.FReturn}},
		}}},
	}
	params := []AsmParam{{Kind: AsmInteger, Code: "a"}}
	result, err := Assemble(fd, params, 0, "rv")
	if err != nil { t.Fatal(err) }
	// returnVal should be injected at position 1
	s := string(result.Code)
	if !strings.Contains(s, "rv") {
		t.Error("return value not injected")
	}
}

func TestAssembleStr(t *testing.T) {
	fd := &kfn.FuncDef{
		Ident: "test", OpType: 0, OpModule: 1, OpCode: 0,
		Prototypes: []kfn.Prototype{{Defined: true}},
	}
	s, err := AssembleStr(fd, nil, 0, "")
	if err != nil { t.Fatal(err) }
	if s[0] != '#' { t.Error("prefix") }
}

// ============================================================
// Type checking tests
// ============================================================

func TestTypeName(t *testing.T) {
	tests := []struct{ pt kfn.ParamType; want string }{
		{kfn.PAny, "any type"},
		{kfn.PInt, "integer variable"},
		{kfn.PIntC, "integer"},
		{kfn.PStr, "string variable"},
		{kfn.PStrC, "string"},
	}
	for _, tt := range tests {
		if got := TypeName(tt.pt); got != tt.want {
			t.Errorf("TypeName(%v) = %q, want %q", tt.pt, got, tt.want)
		}
	}
}

func TestClassifyExpr(t *testing.T) {
	if ClassifyExpr(ast.IntLit{Val: 1}) != ETInt { t.Error("IntLit") }
	if ClassifyExpr(ast.StoreRef{}) != ETInt { t.Error("StoreRef") }
	if ClassifyExpr(ast.IntVar{Bank: 0, Index: ast.IntLit{}}) != ETInt { t.Error("IntVar") }
	if ClassifyExpr(ast.StrVar{Bank: 0, Index: ast.IntLit{}}) != ETStr { t.Error("StrVar") }
	if ClassifyExpr(ast.StrLit{}) != ETLiteral { t.Error("StrLit") }
	if ClassifyExpr(ast.CmpExpr{}) != ETInt { t.Error("CmpExpr") }
	if ClassifyExpr(ast.ParenExpr{Expr: ast.IntLit{}}) != ETInt { t.Error("Parens(Int)") }
}

func TestCheckParamType(t *testing.T) {
	if CheckParamType(kfn.PAny, ETInt) != "" { t.Error("any/int") }
	if CheckParamType(kfn.PAny, ETStr) != "" { t.Error("any/str") }
	if CheckParamType(kfn.PIntC, ETInt) != "" { t.Error("intC/int") }
	if CheckParamType(kfn.PIntC, ETStr) == "" { t.Error("intC/str should fail") }
	if CheckParamType(kfn.PStrC, ETStr) != "" { t.Error("strC/str") }
	if CheckParamType(kfn.PStrC, ETLiteral) != "" { t.Error("strC/literal") }
	if CheckParamType(kfn.PStrC, ETInt) == "" { t.Error("strC/int should fail") }
	if CheckParamType(kfn.PStr, ETStr) != "" { t.Error("str/str") }
	if CheckParamType(kfn.PStr, ETInt) == "" { t.Error("str/int should fail") }
}

// ============================================================
// Function lookup tests
// ============================================================

func TestLookupFuncDefSingle(t *testing.T) {
	reg := kfn.NewRegistry()
	reg.Register(&kfn.FuncDef{Ident: "goto", OpType: 0, OpModule: 1, OpCode: 0})
	fd, err := LookupFuncDef(reg, "goto", nil, false)
	if err != nil { t.Fatal(err) }
	if fd.Ident != "goto" { t.Errorf("got %q", fd.Ident) }
}

func TestLookupFuncDefUndefined(t *testing.T) {
	reg := kfn.NewRegistry()
	_, err := LookupFuncDef(reg, "nonexist", nil, false)
	if err == nil { t.Error("expected error") }
}

func TestLookupFuncDefCtrlCode(t *testing.T) {
	reg := kfn.NewRegistry()
	reg.Register(&kfn.FuncDef{Ident: "strout", CCStr: "strout", OpModule: 3, OpCode: 0})
	fd, err := LookupFuncDef(reg, "strout", nil, true)
	if err != nil { t.Fatal(err) }
	if fd.Ident != "strout" { t.Errorf("got %q", fd.Ident) }
}

func TestLookupFuncDefDisambiguate(t *testing.T) {
	reg := kfn.NewRegistry()
	reg.Register(&kfn.FuncDef{Ident: "grpMulti", OpCode: 1,
		Prototypes: []kfn.Prototype{{Defined: true, Params: []kfn.Parameter{{Type: kfn.PIntC}}}}})
	reg.Register(&kfn.FuncDef{Ident: "grpMulti", OpCode: 2,
		Prototypes: []kfn.Prototype{{Defined: true, Params: []kfn.Parameter{{Type: kfn.PStrC}}}}})

	// Int param → first definition
	fd, err := LookupFuncDef(reg, "grpMulti", []ast.Param{
		ast.SimpleParam{Expr: ast.IntLit{Val: 1}},
	}, false)
	if err != nil { t.Fatal(err) }
	if fd.OpCode != 1 { t.Errorf("int param: got opcode %d, want 1", fd.OpCode) }

	// String param → second definition
	fd, err = LookupFuncDef(reg, "grpMulti", []ast.Param{
		ast.SimpleParam{Expr: ast.StrLit{}},
	}, false)
	if err != nil { t.Fatal(err) }
	if fd.OpCode != 2 { t.Errorf("str param: got opcode %d, want 2", fd.OpCode) }
}

// ============================================================
// BuildParamDefs tests
// ============================================================

func TestBuildParamDefsExact(t *testing.T) {
	proto := []kfn.Parameter{
		{Type: kfn.PIntC},
		{Type: kfn.PStrC},
	}
	defs := BuildParamDefs(proto, 2)
	if len(defs) != 2 { t.Fatalf("len: %d", len(defs)) }
	if defs[0].Type != kfn.PIntC { t.Error("def 0") }
	if defs[1].Type != kfn.PStrC { t.Error("def 1") }
}

func TestBuildParamDefsFilterReturn(t *testing.T) {
	proto := []kfn.Parameter{
		{Type: kfn.PIntC},
		{Type: kfn.PIntV, Flags: []kfn.ParamFlag{kfn.FReturn}},
		{Type: kfn.PStrC},
	}
	// 2 actual params → Return param should be filtered out
	defs := BuildParamDefs(proto, 2)
	if len(defs) != 2 { t.Fatalf("len: %d", len(defs)) }
	if defs[0].Type != kfn.PIntC { t.Error("def 0") }
	if defs[1].Type != kfn.PStrC { t.Error("def 1") }
}

func TestBuildParamDefsArgcExtend(t *testing.T) {
	proto := []kfn.Parameter{
		{Type: kfn.PIntC},
		{Type: kfn.PIntC, Flags: []kfn.ParamFlag{kfn.FArgc}},
	}
	// 5 actual params → Argc param fills remaining slots
	defs := BuildParamDefs(proto, 5)
	if len(defs) != 5 { t.Fatalf("len: %d", len(defs)) }
	for i := 1; i < 5; i++ {
		if defs[i].Type != kfn.PIntC { t.Errorf("def %d: got %v", i, defs[i].Type) }
	}
}

func TestAnalyzeOverloads(t *testing.T) {
	protos := []kfn.Prototype{
		{Defined: true, Params: []kfn.Parameter{
			{Type: kfn.PIntC},
			{Type: kfn.PIntC, Flags: []kfn.ParamFlag{kfn.FOptional}},
		}},
		{Defined: false},
	}
	infos := AnalyzeOverloads(protos)
	if infos[0].Total != 2 || infos[0].Optional != 1 {
		t.Errorf("info 0: %+v", infos[0])
	}
	if infos[1].Defined {
		t.Error("info 1 should be undefined")
	}
}

func min(a, b int) int {
	if a < b { return a }
	return b
}
