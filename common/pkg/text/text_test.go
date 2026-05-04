package text

import (
	"testing"

	"github.com/yoremi/rldev-go/pkg/encoding"
)

// ============================================================
// Text type
// ============================================================

func TestEmpty(t *testing.T) {
	if len(Empty) != 0 { t.Error("empty should have length 0") }
}

func TestOfChar(t *testing.T) {
	x := OfChar('A')
	if len(x) != 1 || x[0] != 'A' { t.Error("OfChar") }
}

func TestAppend(t *testing.T) {
	a := Text{'H', 'i'}
	b := Text{'!'}
	c := Append(a, b)
	if len(c) != 3 { t.Errorf("len: %d", len(c)) }
	if string(c) != "Hi!" { t.Errorf("got %q", string(c)) }
}

func TestIter(t *testing.T) {
	x := Text{'a', 'b', 'c'}
	var result []rune
	x.Iter(func(r rune) { result = append(result, r) })
	if len(result) != 3 || result[0] != 'a' { t.Error("Iter") }
}

func TestMap(t *testing.T) {
	x := Text{'a', 'b', 'c'}
	y := x.Map(func(r rune) rune { return r - 32 }) // uppercase
	if string(y) != "ABC" { t.Errorf("Map: %q", string(y)) }
}

func TestToArrOfArr(t *testing.T) {
	orig := []rune{'H', 'e', 'l', 'l', 'o'}
	x := OfArr(orig)
	if len(x) != 5 { t.Error("OfArr len") }
	arr := x.ToArr()
	if len(arr) != 5 { t.Error("ToArr len") }
}

// ============================================================
// Buffer
// ============================================================

func TestBuf(t *testing.T) {
	b := NewBuf(10)
	b.AddRune('H')
	b.AddRune('i')
	b.AddByte('!')
	if b.Len() != 3 { t.Errorf("len: %d", b.Len()) }
	result := b.Contents()
	if string(result) != "Hi!" { t.Errorf("contents: %q", string(result)) }
}

func TestBufAddText(t *testing.T) {
	b := NewBuf(10)
	b.AddText(Text{'A', 'B'})
	b.AddText(Text{'C'})
	if b.Len() != 3 { t.Errorf("len: %d", b.Len()) }
}

func TestBufClear(t *testing.T) {
	b := NewBuf(10)
	b.AddRune('X')
	b.Clear()
	if b.Len() != 0 { t.Error("should be empty after clear") }
}

// ============================================================
// UTF-8 conversions (trivial in Go)
// ============================================================

func TestToUTF8(t *testing.T) {
	x := Text{'H', 'e', 'l', 'l', 'o', ' ', 0x4E16, 0x754C} // Hello 世界
	s := ToUTF8(x)
	if s != "Hello 世界" { t.Errorf("got %q", s) }
}

func TestOfUTF8(t *testing.T) {
	x := OfUTF8("Hello 世界")
	if len(x) != 8 { t.Errorf("len: %d", len(x)) }
	if x[6] != 0x4E16 { t.Errorf("x[6]: %x", x[6]) }
}

func TestRoundtripUTF8(t *testing.T) {
	orig := "こんにちは世界"
	result := ToUTF8(OfUTF8(orig))
	if result != orig { t.Errorf("roundtrip: %q", result) }
}

// ============================================================
// SJIS conversions
// ============================================================

func TestOfSJSAscii(t *testing.T) {
	x := OfSJS([]byte("Hello"))
	if string(x) != "Hello" { t.Errorf("ASCII: %q", string(x)) }
}

func TestToSJSAscii(t *testing.T) {
	x := Text{'H', 'i'}
	b := ToSJS(x)
	if string(b) != "Hi" { t.Errorf("got %q", string(b)) }
}

// ============================================================
// Normalization
// ============================================================

func TestNorm(t *testing.T) {
	x := Text{'H', 'E', 'L', 'L', 'O'}
	y := Norm(x)
	if string(y) != "hello" { t.Errorf("Norm: %q", string(y)) }
}

func TestNormMixed(t *testing.T) {
	x := Text{'H', 'i', '1', '!'}
	y := Norm(x)
	if string(y) != "hi1!" { t.Errorf("Norm mixed: %q", string(y)) }
}

func TestNormFull(t *testing.T) {
	x := OfUTF8("CAFÉ")
	y := NormFull(x)
	if string(y) != "café" { t.Errorf("NormFull: %q", string(y)) }
}

// ============================================================
// Ident (memoized)
// ============================================================

func TestIdent(t *testing.T) {
	a := Ident("HELLO")
	b := Ident("HELLO")
	// Should return same content (and be memoized)
	if string(a) != string(b) { t.Error("memoized ident should match") }
	if string(a) != "hello" { t.Errorf("ident: %q", string(a)) }
}

// ============================================================
// Quotes
// ============================================================

func TestASCIIQuotes(t *testing.T) {
	q := ASCIIQuotes()
	if q.SQ1 != "'" { t.Error("SQ1") }
	if q.DQ1 != "\"" { t.Error("DQ1") }
	if q.Ch1 != "<<" { t.Error("Ch1") }
	if q.Hel != "..." { t.Error("Hel") }
}

func TestUnicodeQuotes(t *testing.T) {
	q := UnicodeQuotes()
	if q.SQ1 != "\u2018" { t.Error("SQ1") }
	if q.DQ1 != "\u201C" { t.Error("DQ1") }
	if q.Ch1 != "\u00AB" { t.Error("Ch1") }
}

func TestSetFancy(t *testing.T) {
	SetFancy(true)
	if DefaultQuotes.Hel != "..." { t.Error("ASCII mode") }
	SetFancy(false)
	if DefaultQuotes.Hel != "\u2026" { t.Error("Unicode mode") }
	SetFancy(true) // reset
}

// ============================================================
// SJSToUTF8Prep substitutions
// ============================================================

func TestSJSToUTF8PrepAscii(t *testing.T) {
	s := SJSToUTF8Prep([]byte("Hello"))
	if s != "Hello" { t.Errorf("ASCII passthrough: %q", s) }
}

// ============================================================
// ToString / OfString
// ============================================================

func TestToStringUTF8(t *testing.T) {
	x := OfUTF8("test")
	s, err := ToString(encoding.UTF8, x)
	if err != nil { t.Fatal(err) }
	if s != "test" { t.Errorf("got %q", s) }
}

// ============================================================
// Decode
// ============================================================

func TestDecodeUTF8(t *testing.T) {
	x := Decode([]byte("Hello"), encoding.UTF8)
	if string(x) != "Hello" { t.Errorf("got %q", string(x)) }
}

func TestDecodeBOM(t *testing.T) {
	// UTF-8 BOM + content
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte("Hi")...)
	x := Decode(data, encoding.UTF8)
	if string(x) != "Hi" { t.Errorf("BOM not stripped: %q", string(x)) }
}

// ============================================================
// ReadRune
// ============================================================

func TestReadRuneUTF8(t *testing.T) {
	data := []byte("Aé")
	r, pos := ReadRune(data, 0, encoding.UTF8)
	if r != 'A' || pos != 1 { t.Errorf("first: %c pos=%d", r, pos) }
	r, pos = ReadRune(data, pos, encoding.UTF8)
	if r != 'é' { t.Errorf("second: %c (0x%x)", r, r) }
}

func TestReadRuneEOF(t *testing.T) {
	r, _ := ReadRune([]byte{}, 0, encoding.UTF8)
	if r != 0xFFFD { t.Error("should return RuneError at EOF") }
}
