package kfn

import (
	"strings"
	"testing"
)

const sampleKFN = `
module 001 = Jmp
module 003 = Msg

fun goto (skip goto) <0:Jmp:00000, 0> ()
fun goto_if (if goto) <0:Jmp:00001, 0> (<'condition')
fun goto_unless (if neg goto) <0:Jmp:00002, 0> (<'condition')
fun gosub (goto) <0:Jmp:00005, 0> ()
fun ret (skip ret) <0:Jmp:00010, 0> ()
fun jump (skip jump) <0:Jmp:00011, 1> ('scenario')
                                       ('scenario', 'entrypoint')
fun end <0:Jmp:00014, 0> ()

ver RealLive
  fun strout {} <0:Msg:00000, 0> (str)
  fun intout (store) <0:Msg:00001, 0> (int)
end

ver Kinetic
  fun kgoto (skip goto) <0:005:00001, 0> ()
end
`

func parseTestKFN(t *testing.T) *Registry {
	t.Helper()
	reg, err := Parse(strings.NewReader(sampleKFN))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	return reg
}

func TestParseModules(t *testing.T) {
	reg := parseTestKFN(t)
	if v, ok := reg.Modules["Jmp"]; !ok || v != 1 {
		t.Errorf("Jmp module: got %d, %v", v, ok)
	}
	if v, ok := reg.Modules["Msg"]; !ok || v != 3 {
		t.Errorf("Msg module: got %d, %v", v, ok)
	}
}

func TestParseFunctions(t *testing.T) {
	reg := parseTestKFN(t)
	// goto should exist
	fn, ok := reg.Lookup("goto")
	if !ok {
		t.Fatal("goto not found")
	}
	if fn.OpType != 0 || fn.OpModule != 1 || fn.OpCode != 0 {
		t.Errorf("goto opcode: got %d:%d:%d, want 0:1:0", fn.OpType, fn.OpModule, fn.OpCode)
	}
	// Check flags
	if !fn.HasFlag(FlagIsSkip) { t.Error("goto should have IsSkip") }
	if !fn.HasFlag(FlagIsGoto) { t.Error("goto should have IsGoto") }
}

func TestParseFuncFlags(t *testing.T) {
	reg := parseTestKFN(t)
	fn, ok := reg.Lookup("goto_if")
	if !ok { t.Fatal("goto_if not found") }
	if !fn.HasFlag(FlagIsCond) { t.Error("goto_if should have IsCond") }
	if !fn.HasFlag(FlagIsGoto) { t.Error("goto_if should have IsGoto") }
}

func TestParsePrototypes(t *testing.T) {
	reg := parseTestKFN(t)
	// jump has 2 overloads: ('scenario') and ('scenario', 'entrypoint')
	fns := reg.Functions["jump"]
	if len(fns) == 0 { t.Fatal("jump not found") }
	fn := fns[0]
	if len(fn.Prototypes) != 2 {
		t.Fatalf("jump prototypes: got %d, want 2", len(fn.Prototypes))
	}
	if !fn.Prototypes[0].Defined {
		t.Error("jump proto[0] should be defined")
	}
	if len(fn.Prototypes[0].Params) != 1 {
		t.Errorf("jump proto[0] params: got %d, want 1", len(fn.Prototypes[0].Params))
	}
	if len(fn.Prototypes[1].Params) != 2 {
		t.Errorf("jump proto[1] params: got %d, want 2", len(fn.Prototypes[1].Params))
	}
}

func TestParseEmptyPrototype(t *testing.T) {
	reg := parseTestKFN(t)
	fn, _ := reg.Lookup("goto")
	if len(fn.Prototypes) != 1 {
		t.Fatalf("goto prototypes: got %d, want 1", len(fn.Prototypes))
	}
	if !fn.Prototypes[0].Defined {
		t.Error("goto proto should be defined")
	}
	if len(fn.Prototypes[0].Params) != 0 {
		t.Errorf("goto proto params: got %d, want 0", len(fn.Prototypes[0].Params))
	}
}

func TestParseParameterFlags(t *testing.T) {
	reg := parseTestKFN(t)
	fn, _ := reg.Lookup("goto_if")
	if len(fn.Prototypes) == 0 || !fn.Prototypes[0].Defined { t.Fatal("goto_if proto") }
	param := fn.Prototypes[0].Params[0]
	// Should have Uncount (<) and Tagged ('condition')
	hasUncount := false
	hasTagged := false
	for _, fl := range param.Flags {
		if fl == FUncount { hasUncount = true }
		if fl == FTagged { hasTagged = true }
	}
	if !hasUncount { t.Error("expected FUncount flag") }
	if !hasTagged { t.Error("expected FTagged flag") }
	if param.Tag != "condition" { t.Errorf("tag: got %q, want 'condition'", param.Tag) }
}

func TestParseStoreFlag(t *testing.T) {
	reg := parseTestKFN(t)
	fn, _ := reg.Lookup("intout")
	if !fn.HasFlag(FlagPushStore) { t.Error("intout should have PushStore") }
}

func TestParseControlCode(t *testing.T) {
	reg := parseTestKFN(t)
	// strout has {} → control code with its own name
	fn, ok := reg.LookupCtrlCode("strout")
	if !ok { t.Fatal("strout ctrl code not found") }
	if fn.CCStr != "strout" { t.Errorf("ccstr: got %q", fn.CCStr) }
}

func TestParseVerBlock(t *testing.T) {
	reg := parseTestKFN(t)
	// strout should be in RealLive ver block
	fns := reg.Functions["strout"]
	if len(fns) == 0 { t.Fatal("strout not found") }
	if len(fns[0].Targets) == 0 { t.Fatal("strout should have target constraints") }

	// kgoto should be in Kinetic ver block
	fns = reg.Functions["kgoto"]
	if len(fns) == 0 { t.Fatal("kgoto not found") }
	if len(fns[0].Targets) == 0 { t.Fatal("kgoto should have target constraints") }
}

func TestParseEndKeyword(t *testing.T) {
	reg := parseTestKFN(t)
	// "end" is a special case — keyword used as function name
	fn, ok := reg.Lookup("end")
	if !ok { t.Fatal("end function not found") }
	if fn.OpCode != 14 { t.Errorf("end opcode: got %d, want 14", fn.OpCode) }
}

func TestGotoFuncs(t *testing.T) {
	reg := parseTestKFN(t)
	// Functions with IsGoto flag should be in GotoFuncs list
	found := false
	for _, name := range reg.GotoFuncs {
		if name == "goto" { found = true; break }
	}
	if !found { t.Error("'goto' should be in GotoFuncs") }
}

func TestIdentOfOpcode(t *testing.T) {
	s := IdentOfOpcode(0, 1, 5, 0)
	if s != "__op_0_1_5_0" {
		t.Errorf("got %q, want '__op_0_1_5_0'", s)
	}
}

func TestReturnType(t *testing.T) {
	reg := parseTestKFN(t)
	fn, _ := reg.Lookup("intout")
	if fn.ReturnType() != "int" {
		t.Errorf("intout return: got %q, want 'int'", fn.ReturnType())
	}
	fn, _ = reg.Lookup("goto")
	if fn.ReturnType() != "none" {
		t.Errorf("goto return: got %q, want 'none'", fn.ReturnType())
	}
}

func TestParseTarget(t *testing.T) {
	tests := []struct{ in string; want Target }{
		{"reallive", TargetRealLive},
		{"RealLive", TargetRealLive},
		{"avg2000", TargetAVG2000},
		{"kinetic", TargetKinetic},
		{"2", TargetRealLive},
		{"1", TargetAVG2000},
		{"3", TargetKinetic},
		{"unknown", TargetDefault},
	}
	for _, tt := range tests {
		got := ParseTarget(tt.in)
		if got != tt.want {
			t.Errorf("ParseTarget(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestVersionString(t *testing.T) {
	reg := NewRegistry()
	reg.Target = TargetRealLive
	s := reg.CurrentVersionString()
	if s != "RealLive 1.2.7" {
		t.Errorf("got %q, want 'RealLive 1.2.7'", s)
	}
	reg.Target = TargetAVG2000
	s = reg.CurrentVersionString()
	if s != "AVG2000 1.0" {
		t.Errorf("got %q, want 'AVG2000 1.0'", s)
	}
}

func TestParseRealFile(t *testing.T) {
	// Try parsing the actual reallive.kfn if available
	reg, err := ParseFile("/home/claude/rldev/lib/reallive.kfn")
	if err != nil {
		t.Skipf("reallive.kfn not available: %v", err)
	}
	// Should have many functions
	count := 0
	for _, fns := range reg.Functions {
		count += len(fns)
	}
	if count < 100 {
		t.Errorf("expected 100+ functions, got %d", count)
	}
	// Should have common modules
	if _, ok := reg.Modules["Jmp"]; !ok { t.Error("missing Jmp module") }
	if _, ok := reg.Modules["Msg"]; !ok { t.Error("missing Msg module") }
	if _, ok := reg.Modules["Str"]; !ok { t.Error("missing Str module") }
	// Should have goto
	fn, ok := reg.Lookup("goto")
	if !ok { t.Error("goto not found in real file") }
	if fn.OpModule != 1 || fn.OpCode != 0 {
		t.Errorf("goto: got %d:%d, want 1:0", fn.OpModule, fn.OpCode)
	}
	t.Logf("Parsed %d functions, %d modules, %d goto funcs",
		count, len(reg.Modules), len(reg.GotoFuncs))
}
