package texttransforms

import (
	"testing"

	"github.com/yoremi/rldev-go/pkg/text"
)

// ============================================================
// Mode parsing
// ============================================================

func TestParseModeNone(t *testing.T) {
	for _, name := range []string{"", "NONE", "JP", "CP932", "SJIS", "Shift-JIS"} {
		m, err := ParseMode(name)
		if err != nil { t.Errorf("%q: %v", name, err) }
		if m != EncNone { t.Errorf("%q: got %d", name, m) }
	}
}

func TestParseModeChinese(t *testing.T) {
	for _, name := range []string{"CHINESE", "ZH", "CP936", "GBK"} {
		m, err := ParseMode(name)
		if err != nil { t.Errorf("%q: %v", name, err) }
		if m != EncChinese { t.Errorf("%q: got %d", name, m) }
	}
}

func TestParseModeWestern(t *testing.T) {
	for _, name := range []string{"WESTERN", "EN", "CP1252"} {
		m, err := ParseMode(name)
		if err != nil { t.Errorf("%q: %v", name, err) }
		if m != EncWestern { t.Errorf("%q: got %d", name, m) }
	}
}

func TestParseModeKorean(t *testing.T) {
	for _, name := range []string{"KOREAN", "KR", "CP949", "HANGUL"} {
		m, err := ParseMode(name)
		if err != nil { t.Errorf("%q: %v", name, err) }
		if m != EncKorean { t.Errorf("%q: got %d", name, m) }
	}
}

func TestParseModeUnknown(t *testing.T) {
	_, err := ParseMode("KLINGON")
	if err == nil { t.Error("expected error for unknown mode") }
}

// ============================================================
// SetEncoding / GetMode
// ============================================================

func TestSetEncoding(t *testing.T) {
	err := SetEncoding("WESTERN")
	if err != nil { t.Fatal(err) }
	if GetMode() != EncWestern { t.Error("should be Western") }
	SetMode(EncNone) // reset
}

func TestSetEncodingBad(t *testing.T) {
	err := SetEncoding("UNKNOWN_ENC")
	if err == nil { t.Error("expected error") }
}

// ============================================================
// Describe
// ============================================================

func TestDescribe(t *testing.T) {
	SetMode(EncNone)
	if Describe() != "no output transformation" { t.Error("none") }
	SetMode(EncChinese)
	if Describe() == "no output transformation" { t.Error("chinese") }
	SetMode(EncWestern)
	if Describe() == "no output transformation" { t.Error("western") }
	SetMode(EncKorean)
	if Describe() == "no output transformation" { t.Error("korean") }
	SetMode(EncNone) // reset
}

// ============================================================
// ToBytecode — SJIS mode (EncNone)
// ============================================================

func TestToBytecodeASCII(t *testing.T) {
	SetMode(EncNone)
	b, err := ToBytecode(text.Text{'H', 'e', 'l', 'l', 'o'})
	if err != nil { t.Fatal(err) }
	if string(b) != "Hello" { t.Errorf("got %q", string(b)) }
}

func TestToBytecodeEmpty(t *testing.T) {
	SetMode(EncNone)
	b, err := ToBytecode(text.Text{})
	if err != nil { t.Fatal(err) }
	if len(b) != 0 { t.Error("empty should produce empty") }
}

// ============================================================
// Western (CP1252) encode/decode
// ============================================================

func TestWesternASCII(t *testing.T) {
	b, err := encodeWestern(text.Text{'A', 'B', 'C'})
	if err != nil { t.Fatal(err) }
	if string(b) != "ABC" { t.Errorf("got %q", string(b)) }
}

func TestWesternHighChars(t *testing.T) {
	// 0xC0 (À) → encoded as 0xC0 - 0x1F = 0xA1
	b, err := encodeWestern(text.Text{0xC0})
	if err != nil { t.Fatal(err) }
	if len(b) != 1 || b[0] != 0xA1 { t.Errorf("À: got %x", b) }
}

func TestWesternSpecialChars(t *testing.T) {
	// 0x20AC (€) → CP1252 0x80 → encoded as 0x89 0x80
	b, err := encodeWestern(text.Text{0x20AC})
	if err != nil { t.Fatal(err) }
	if len(b) != 2 || b[0] != 0x89 || b[1] != 0x80 {
		t.Errorf("€: got %x", b)
	}
}

func TestWesternRoundtrip(t *testing.T) {
	orig := text.Text{'H', 'e', 'l', 'l', 'o'}
	b, err := encodeWestern(orig)
	if err != nil { t.Fatal(err) }
	decoded, err := decodeWestern(b)
	if err != nil { t.Fatal(err) }
	if text.ToUTF8(decoded) != text.ToUTF8(orig) {
		t.Errorf("roundtrip: %q → %q", text.ToUTF8(orig), text.ToUTF8(decoded))
	}
}

func TestWesternForceEncode(t *testing.T) {
	ForceEncode = true
	defer func() { ForceEncode = false }()
	// Unmappable char → space
	b, err := encodeWestern(text.Text{0x4E16}) // 世 (not in CP1252)
	if err != nil { t.Fatal(err) }
	if len(b) != 1 || b[0] != 0x20 { t.Errorf("forced: got %x", b) }
}

func TestWesternUnmappableError(t *testing.T) {
	ForceEncode = false
	_, err := encodeWestern(text.Text{0x4E16}) // 世
	if err == nil { t.Error("should error for unmappable char without ForceEncode") }
}

// ============================================================
// Bad character tracking
// ============================================================

func TestBadCharTracking(t *testing.T) {
	ResetBadChars()
	msg := Complain(0x1234, "test")
	if msg == "" { t.Error("should produce message") }
	if BadCharCount() != 1 { t.Errorf("count: %d", BadCharCount()) }

	// Second complaint for same char
	Complain(0x1234, "test")
	if BadCharCount() != 1 { t.Error("same char should not increase distinct count") }

	// New char
	Complain(0x5678, "test")
	if BadCharCount() != 2 { t.Errorf("count: %d", BadCharCount()) }

	ResetBadChars()
	if BadCharCount() != 0 { t.Error("should be 0 after reset") }
}

// ============================================================
// ReadBytecode
// ============================================================

func TestReadBytecodeASCII(t *testing.T) {
	SetMode(EncNone)
	txt, err := ReadBytecode([]byte("Hello"))
	if err != nil { t.Fatal(err) }
	if text.ToUTF8(txt) != "Hello" { t.Errorf("got %q", text.ToUTF8(txt)) }
}

func TestReadBytecodeWestern(t *testing.T) {
	SetMode(EncWestern)
	defer SetMode(EncNone)
	// Encode then decode
	orig := text.Text{'A', 'B'}
	b, _ := encodeWestern(orig)
	txt, err := ReadBytecode(b)
	if err != nil { t.Fatal(err) }
	if text.ToUTF8(txt) != "AB" { t.Errorf("got %q", text.ToUTF8(txt)) }
}

// ============================================================
// ToBytecode dispatch
// ============================================================

func TestToBytecodeDispatchWestern(t *testing.T) {
	SetMode(EncWestern)
	defer SetMode(EncNone)
	b, err := ToBytecode(text.Text{'X'})
	if err != nil { t.Fatal(err) }
	if len(b) != 1 || b[0] != 'X' { t.Errorf("got %x", b) }
}
