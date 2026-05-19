package disasm

import (
	"fmt"
	"strings"
)

// addTextoutFails tries to fold the given content `s` into the previous
// (non-hidden) command's resource string, matching the OCaml routine
// `add_textout_fails` in kprl/disassembler.ml (L1389-L1485).
//
// Returns true when the caller should still emit a new command (merge
// failed); returns false when the content was absorbed into the
// previous command (no new command needed).
//
// The merge is called from two sites:
//
//   - readTextout, after building the textout string: collapses
//     consecutive Msg() opcodes into one resource ("バタンッ！" + Msg("\shake{4}")
//     stays as a single resource entry when the second piece is folded
//     in by the shake call below).
//
//   - readFunction, for opcodes whose KFN entry has a ccode form
//     (FontSize → \size, shake → \shake, …): the rendered "\xxx{args}"
//     is appended to the running resource so screen-effect / styling
//     commands stay inline inside dialogue strings.
//
// This implementation covers the latest-resource case (OCaml case 3),
// which handles every Clannad / AIR / Kanon / Tomoyo After SEEN we've
// looked at. The two FontSize back-merge variants (cases 1-2 in
// OCaml — merging a textout backwards into a preceding standalone
// FontSize) are stubbed with a TODO: they only trigger when FontSize
// emits with no preceding textout, which the Key games do not exercise.
func addTextoutFails(result *DisassemblyResult, s string) bool {
	if len(result.Commands) == 0 {
		return true
	}
	// Walk backwards past hidden commands (#line directives, etc.).
	i := len(result.Commands) - 1
	for i >= 0 && result.Commands[i].Hidden {
		i--
	}
	if i < 0 {
		return true
	}
	prev := &result.Commands[i]

	// Refuse to merge if the previous command has multiple kepago
	// elements (pointer, store marker, several string fragments) —
	// matches the OCaml `| [] | [P _] | [STORE _] | _::_::_ -> true`
	// guard. We also refuse if the single element isn't a plain
	// string (pointers/store/text elements).
	if len(prev.Kepago) != 1 {
		return true
	}
	last, ok := prev.Kepago[0].(ElemString)
	if !ok {
		return true
	}

	// Case 3 (latest-resource append). OCaml uses
	//     last = sprintf "#res<%04d>" !rescount
	// where !rescount is the index of the most recent resource string.
	// In Go terms: len(ResStrs)-1 is the latest index, and `last.Value`
	// must equal "#res<NNNN>" for that index.
	if len(result.ResStrs) == 0 {
		// TODO: FontSize back-merge cases. The two OCaml branches
		//   | [S last] when ...opcode = "op<0:Msg:00101,..."
		//   | [S "FontSize"]
		// rewrite the previous command's kepago in-place AND allocate
		// a fresh resource for the merged "\size{N}<s>" content. This
		// shifts all subsequent resource indices, so we defer it until
		// a game that needs it (none of the Key titles seen so far).
		return true
	}
	latestIdx := len(result.ResStrs) - 1
	expected := fmt.Sprintf("#res<%04d>", latestIdx)
	if last.Value != expected {
		return true
	}
	// Merge: extend the latest resource string.
	result.ResStrs[latestIdx] += s
	return false
}

// formatCcodeForm renders a function call as its kepago control-code
// form, matching OCaml `ccode_form` (disassembler.ml L2340-L2349):
//
//	if no args        → "\<ccode>" + ("{}" unless NoBraces flag)
//	with args         → "\<ccode>{arg1, arg2, …}"
//
// NoBraces with IsLbr emits a trailing newline (used for `\r` / `\n`
// line breaks in dynamic textout); we keep them simple here since the
// Key titles don't ship rlBabel dynamic textout. The line break form
// can be extended later if needed.
func formatCcodeForm(def FuncDef, args []string) string {
	if len(args) == 0 {
		var sb strings.Builder
		sb.WriteByte('\\')
		sb.WriteString(def.Ccode)
		if !def.HasFlag(FlagNoBraces) {
			sb.WriteString("{}")
		}
		return sb.String()
	}
	return fmt.Sprintf("\\%s{%s}", def.Ccode, strings.Join(args, ", "))
}
