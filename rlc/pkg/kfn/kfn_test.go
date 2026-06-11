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
	if !fn.HasFlag(FlagIsSkip) {
		t.Error("goto should have IsSkip")
	}
	if !fn.HasFlag(FlagIsGoto) {
		t.Error("goto should have IsGoto")
	}
}

func TestParseSecondIdentifierAlias(t *testing.T) {
	src := `
module 000 = Sys
fun __gc1 GetCursorPos <1:Sys:00133, 0> (int 'x', int 'y', int 'button1', int 'button2')
fun __shkud ShakeScreen <1:012:01100, 0> (='DOWNUP', 'amount')
`
	reg, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Lookup("__gc1"); !ok {
		t.Fatal("__gc1 not registered")
	}
	if _, ok := reg.Lookup("GetCursorPos"); !ok {
		t.Fatal("public GetCursorPos alias not registered")
	}
	if fn, ok := reg.Lookup("ShakeScreen"); !ok {
		t.Fatal("fake-parameter public alias not registered")
	} else if len(fn.Prototypes) != 1 || len(fn.Prototypes[0].FakeParams) != 1 {
		t.Fatalf("fake alias metadata missing: %#v", fn.Prototypes)
	}
}

func TestParseSpecialDefinitions(t *testing.T) {
	src := `
module 000 = Sys
fun index_series (store) <1:Sys:00800, 0> (<'index', <'offset', <'init', special(0:{'val'}, 1:{'start', 'end', 'endval'}, 2:{'start', 'end', 'endval', 'mode'})+)
fun gosub_with (store goto) <0:Sys:00016, 0> (special(0:#{intC}, 1:#{strC})+)
`
	reg, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	idx, ok := reg.Lookup("index_series")
	if !ok {
		t.Fatal("index_series not registered")
	}
	if got := idx.Prototypes[0].Params[3].Specials; len(got) != 3 {
		t.Fatalf("index_series specials = %#v, want 3 cases", got)
	} else if got[2].ID != 2 || len(got[2].Params) != 4 || got[2].Params[3].Tag != "mode" {
		t.Fatalf("bad index_series tag 2 special: %#v", got[2])
	}
	gosub, ok := reg.Lookup("gosub_with")
	if !ok {
		t.Fatal("gosub_with not registered")
	}
	got := gosub.Prototypes[0].Params[0].Specials
	if len(got) != 2 || !got[0].HasFlag(SFNoParens) || got[0].Params[0].Type != PIntC {
		t.Fatalf("inline gosub specials not parsed: %#v", got)
	}
}

func TestParseFuncFlags(t *testing.T) {
	reg := parseTestKFN(t)
	fn, ok := reg.Lookup("goto_if")
	if !ok {
		t.Fatal("goto_if not found")
	}
	if !fn.HasFlag(FlagIsCond) {
		t.Error("goto_if should have IsCond")
	}
	if !fn.HasFlag(FlagIsGoto) {
		t.Error("goto_if should have IsGoto")
	}
}

func TestParsePrototypes(t *testing.T) {
	reg := parseTestKFN(t)
	// jump has 2 overloads: ('scenario') and ('scenario', 'entrypoint')
	fns := reg.Functions["jump"]
	if len(fns) == 0 {
		t.Fatal("jump not found")
	}
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

func TestParsePrototypePlaceholdersKeepOverloadIndex(t *testing.T) {
	src := `
module 071 = OFC
fun objOfFileGan <1:OFC:01003, 2> ? ?
                                     ('buf', strC 'filename', strC 'ganname', 'visible', 'x', 'y')
`
	reg, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	fn, ok := reg.Lookup("objOfFileGan")
	if !ok {
		t.Fatal("objOfFileGan not found")
	}
	if len(fn.Prototypes) != 3 {
		t.Fatalf("prototypes = %d, want 3", len(fn.Prototypes))
	}
	if fn.Prototypes[0].Defined || fn.Prototypes[1].Defined {
		t.Fatalf("placeholder prototypes should stay undefined: %#v", fn.Prototypes[:2])
	}
	if !fn.Prototypes[2].Defined || len(fn.Prototypes[2].Params) != 6 {
		t.Fatalf("overload 2 prototype = %#v, want defined 6-arg prototype", fn.Prototypes[2])
	}
	if got := fn.Prototypes[2].Params[1].Type; got != PStrC {
		t.Fatalf("filename param type = %s, want %s", got, PStrC)
	}
	if got := fn.Prototypes[2].Params[2].Type; got != PStrC {
		t.Fatalf("ganname param type = %s, want %s", got, PStrC)
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
	if len(fn.Prototypes) == 0 || !fn.Prototypes[0].Defined {
		t.Fatal("goto_if proto")
	}
	param := fn.Prototypes[0].Params[0]
	// Should have Uncount (<) and Tagged ('condition')
	hasUncount := false
	hasTagged := false
	for _, fl := range param.Flags {
		if fl == FUncount {
			hasUncount = true
		}
		if fl == FTagged {
			hasTagged = true
		}
	}
	if !hasUncount {
		t.Error("expected FUncount flag")
	}
	if !hasTagged {
		t.Error("expected FTagged flag")
	}
	if param.Tag != "condition" {
		t.Errorf("tag: got %q, want 'condition'", param.Tag)
	}
}

func TestParseStoreFlag(t *testing.T) {
	reg := parseTestKFN(t)
	fn, _ := reg.Lookup("intout")
	if !fn.HasFlag(FlagPushStore) {
		t.Error("intout should have PushStore")
	}
}

func TestParseControlCode(t *testing.T) {
	reg := parseTestKFN(t)
	// strout has {} → control code with its own name
	fn, ok := reg.LookupCtrlCode("strout")
	if !ok {
		t.Fatal("strout ctrl code not found")
	}
	if fn.CCStr != "strout" {
		t.Errorf("ccstr: got %q", fn.CCStr)
	}
}

func TestParseVerBlock(t *testing.T) {
	reg := parseTestKFN(t)
	// strout should be in RealLive ver block
	fns := reg.Functions["strout"]
	if len(fns) == 0 {
		t.Fatal("strout not found")
	}
	if len(fns[0].Targets) == 0 {
		t.Fatal("strout should have target constraints")
	}

	// kgoto should be in Kinetic ver block
	fns = reg.Functions["kgoto"]
	if len(fns) == 0 {
		t.Fatal("kgoto not found")
	}
	if len(fns[0].Targets) == 0 {
		t.Fatal("kgoto should have target constraints")
	}
}

func TestVersionConstraintsAreConjunctive(t *testing.T) {
	reg, err := Parse(strings.NewReader(`
module 001 = Jmp

ver >= 1.2, < 1.2.5
  fun CallDLL <2:Jmp:00001, 0> (='0', intC)
end

ver >= 1.2.5
  fun CallDLL <2:Jmp:00002, 0> ('index', 'par1', 'par2', 'par3', 'par4', 'par5')
end
`))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	reg.Target = TargetRealLive
	reg.Version = Version{1, 6, 2, 3}
	fn, ok := reg.Lookup("CallDLL")
	if !ok {
		t.Fatal("CallDLL not found")
	}
	if fn.OpCode != 2 {
		t.Fatalf("RealLive 1.6 should select the >=1.2.5 CallDLL; opcode=%d", fn.OpCode)
	}
	if len(fn.Prototypes) != 1 || len(fn.Prototypes[0].Params) != 6 {
		t.Fatalf("CallDLL params = %#v, want the 6-parameter modern prototype", fn.Prototypes)
	}

	reg.Version = Version{1, 2, 4, 9}
	fn, ok = reg.Lookup("CallDLL")
	if !ok {
		t.Fatal("CallDLL not found")
	}
	if fn.OpCode != 1 {
		t.Fatalf("RealLive 1.2.4 should select the legacy CallDLL; opcode=%d", fn.OpCode)
	}
}

func TestClassConstraintsAreDisjunctive(t *testing.T) {
	reg, err := Parse(strings.NewReader(`
module 001 = Jmp

ver Avg2000, RealLive
  fun dual <0:Jmp:00003, 0> ()
end
`))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	reg.Target = TargetRealLive
	fn, ok := reg.Lookup("dual")
	if !ok {
		t.Fatal("dual should be valid for RealLive")
	}
	if !reg.ValidForTarget(fn) {
		t.Fatal("dual should be valid for RealLive")
	}

	reg.Target = TargetKinetic
	fn, ok = reg.Lookup("dual")
	if !ok {
		t.Fatal("dual not found")
	}
	if reg.ValidForTarget(fn) {
		t.Fatal("dual should not be valid for Kinetic")
	}
}

func TestParseEndKeyword(t *testing.T) {
	reg := parseTestKFN(t)
	// "end" is a special case — keyword used as function name
	fn, ok := reg.Lookup("end")
	if !ok {
		t.Fatal("end function not found")
	}
	if fn.OpCode != 14 {
		t.Errorf("end opcode: got %d, want 14", fn.OpCode)
	}
}

func TestGotoFuncs(t *testing.T) {
	reg := parseTestKFN(t)
	// Functions with IsGoto flag should be in GotoFuncs list
	found := false
	for _, name := range reg.GotoFuncs {
		if name == "goto" {
			found = true
			break
		}
	}
	if !found {
		t.Error("'goto' should be in GotoFuncs")
	}
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
	tests := []struct {
		in   string
		want Target
	}{
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
	if _, ok := reg.Modules["Jmp"]; !ok {
		t.Error("missing Jmp module")
	}
	if _, ok := reg.Modules["Msg"]; !ok {
		t.Error("missing Msg module")
	}
	if _, ok := reg.Modules["Str"]; !ok {
		t.Error("missing Str module")
	}
	// Should have goto
	fn, ok := reg.Lookup("goto")
	if !ok {
		t.Error("goto not found in real file")
	}
	if fn.OpModule != 1 || fn.OpCode != 0 {
		t.Errorf("goto: got %d:%d, want 1:0", fn.OpModule, fn.OpCode)
	}
	t.Logf("Parsed %d functions, %d modules, %d goto funcs",
		count, len(reg.Modules), len(reg.GotoFuncs))
}
