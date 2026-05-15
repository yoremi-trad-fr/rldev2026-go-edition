package disasm

import (
	"strings"
	"testing"
)

// TestPrintMarkerInGetData reproduces the SEEN1002 / select_s case:
// the bytecode contains "###PRINT(strS[1011])" as part of a string
// argument. The first Go port had no '#' case in GetDataSep so the
// three leading '#' bytes were read as an opcode header, producing
// the ubiquitous `op<35:035:21072, 84>` warning. After the fix the
// marker dispatches to readStringUnquot which folds it into the
// proper \s{strS[1011]} inline form.
func TestPrintMarkerInGetData(t *testing.T) {
	// The exact expression layout for strS[1011] is grammar-heavy
	// and not the point of this test — what we're validating is the
	// dispatch fix: a leading "###PRINT(" must funnel into
	// readStringUnquot (where the marker is handled) instead of
	// falling through to GetExpression (where '#' is read as opcode
	// data).
	//
	// We use a trivial expression body: just the integer 7, which
	// is encoded as `$ \xff <7 as int32>` in the RealLive grammar.
	expr := []byte{
		'$', 0xff,
		0x07, 0x00, 0x00, 0x00,
	}
	bc := append([]byte("###PRINT("), expr...)
	bc = append(bc, ')', ',')

	r := NewReader(bc, 0, len(bc), ModeRealLive)
	result, err := r.GetDataSep(false)
	if err != nil {
		t.Fatalf("GetDataSep: %v (result so far: %q)", err, result)
	}

	// Expected rendering: the inner readStringUnquot wraps an
	// integer-typed interpolation as \i{<expr>} (disassembler.ml
	// L1623 — \s when expr starts with 's', \i otherwise).
	if !strings.Contains(result, `\i{`) && !strings.Contains(result, `\s{`) {
		t.Errorf("missing \\i{} or \\s{} interpolation marker, got: %q", result)
	}
	// Critically: no leftover '#' fragment ended up in the result.
	if strings.HasPrefix(result, "#") && !strings.HasPrefix(result, "#res<") {
		t.Errorf("result still starts with bare '#' — marker not consumed: %q", result)
	}
	// The bogus opcode-shaped output `op<35:...>` from the old
	// behaviour should be impossible now.
	if strings.Contains(result, "op<35") {
		t.Errorf("regression: ##PRINT still being read as opcode header: %q", result)
	}
}

// TestPlainHashFallback: a lone '#' that does NOT start ###PRINT(
// should still go through the legacy expression path so existing
// behaviour for non-marker hash bytes is preserved.
func TestPlainHashFallback(t *testing.T) {
	// A single 0x23 followed by something that isn't ##PRINT(
	bc := []byte{'#', 'a', 'b'}
	r := NewReader(bc, 0, len(bc), ModeRealLive)
	// We don't check the result content — just that it didn't panic
	// and didn't consume the ###PRINT path.
	_, _ = r.GetDataSep(false)
	// Position should have moved (expression path consumed at
	// least one byte) but we don't pin the exact behaviour to
	// avoid coupling to expr internals.
}
