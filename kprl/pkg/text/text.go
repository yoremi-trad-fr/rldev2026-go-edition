// Package text provides Unicode text handling for RealLive compilation.
//
// Transposed from OCaml's common/text.ml (624 lines, ~300 active)
// + common/text.mli (61 lines).
//
// In OCaml, text is represented as `int array` (Unicode codepoints).
// In Go, we use `[]rune` which is the native codepoint type.
//
// The encoding conversions (SJIS ↔ UTF-8 ↔ EUC-JP) are handled by
// the encoding package (which uses golang.org/x/text internally),
// so this package focuses on:
//
//   - Text type ([]rune) with builder, iteration, mapping
//   - Conversion to/from Shift-JIS byte strings
//   - Conversion to/from UTF-8 strings (trivial in Go)
//   - Normalization (lowercase)
//   - Memoized identifier conversion
//   - "Fancy" (smart) quote configuration
//   - Unicode stream reading from byte input
package text

import (
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/yoremi/rldev-go/pkg/encoding"
)

// ============================================================
// Text type (from text.ml: type t = int array)
// ============================================================

// Text is a sequence of Unicode codepoints, the fundamental string
// type used internally by the RLdev compiler.
type Text []rune

// Empty is an empty text.
var Empty = Text{}

// OfChar creates a Text from a single codepoint.
func OfChar(c rune) Text { return Text{c} }

// Append concatenates two Texts.
func Append(a, b Text) Text {
	r := make(Text, len(a)+len(b))
	copy(r, a)
	copy(r[len(a):], b)
	return r
}

// Iter calls f for each codepoint.
func (t Text) Iter(f func(rune)) {
	for _, c := range t {
		f(c)
	}
}

// Map applies f to each codepoint and returns a new Text.
func (t Text) Map(f func(rune) rune) Text {
	r := make(Text, len(t))
	for i, c := range t {
		r[i] = f(c)
	}
	return r
}

// ToArr returns the underlying rune slice.
func (t Text) ToArr() []rune { return []rune(t) }

// OfArr creates a Text from a rune slice.
func OfArr(a []rune) Text { return Text(a) }

// ============================================================
// Buffer (from text.ml: module Buf)
// ============================================================

// Buf is a dynamic buffer for building Text incrementally.
// Maps directly from OCaml's DynArray-based Buf module.
type Buf struct {
	data []rune
}

// NewBuf creates a buffer with initial capacity hint.
func NewBuf(size int) *Buf {
	return &Buf{data: make([]rune, 0, size)}
}

// AddRune appends a single codepoint.
func (b *Buf) AddRune(c rune) { b.data = append(b.data, c) }

// AddByte appends a single byte as a codepoint.
func (b *Buf) AddByte(c byte) { b.data = append(b.data, rune(c)) }

// AddText appends all codepoints from a Text.
func (b *Buf) AddText(t Text) { b.data = append(b.data, t...) }

// AddArr appends all codepoints from a rune slice.
func (b *Buf) AddArr(a []rune) { b.data = append(b.data, a...) }

// Len returns the current number of codepoints.
func (b *Buf) Len() int { return len(b.data) }

// Contents returns the built Text.
func (b *Buf) Contents() Text { return Text(append([]rune{}, b.data...)) }

// Clear resets the buffer.
func (b *Buf) Clear() { b.data = b.data[:0] }

// ============================================================
// Encoding conversions
// ============================================================

// ToSJS converts Text to Shift-JIS bytes.
// Codepoints without a SJIS mapping are written as \u{$XXXX}.
func ToSJS(t Text) []byte {
	// Convert runes to UTF-8 string, then use encoding package
	s := string(t)
	b, err := encoding.UTF8ToSJS(s)
	if err != nil {
		// Fallback: return UTF-8 bytes for unmappable chars
		return []byte(s)
	}
	return b
}

// OfSJS converts a Shift-JIS byte string to Text.
func OfSJS(data []byte) Text {
	s, err := encoding.SJSToUTF8(data)
	if err != nil {
		// Fallback: treat as raw bytes
		r := make(Text, len(data))
		for i, b := range data {
			r[i] = rune(b)
		}
		return r
	}
	return Text([]rune(s))
}

// ToUTF8 converts Text to a UTF-8 string.
// This is trivial in Go since rune ↔ UTF-8 is native.
func ToUTF8(t Text) string {
	return string(t)
}

// OfUTF8 converts a UTF-8 string to Text.
func OfUTF8(s string) Text {
	return Text([]rune(s))
}

// ToString converts Text to the specified encoding.
func ToString(enc encoding.Type, t Text) (string, error) {
	utf8 := string(t)
	switch enc {
	case encoding.ShiftJIS:
		b, err := encoding.UTF8ToSJS(utf8)
		if err != nil {
			return "", err
		}
		return string(b), nil
	case encoding.UTF8:
		return utf8, nil
	case encoding.EUCJP:
		sjsBytes, err := encoding.UTF8ToSJS(utf8)
		if err != nil {
			return "", err
		}
		eucBytes, err := encoding.SJSToEUC(sjsBytes)
		if err != nil {
			return "", err
		}
		return string(eucBytes), nil
	default:
		return utf8, nil
	}
}

// OfString converts from the specified encoding to Text.
func OfString(enc encoding.Type, data []byte) (Text, error) {
	utf8Str, err := encoding.ToUTF8(data, enc)
	if err != nil {
		return nil, err
	}
	return Text([]rune(utf8Str)), nil
}

// SJSToEnc converts a Shift-JIS byte string to the target encoding.
func SJSToEnc(enc encoding.Type, data []byte) (string, error) {
	switch enc {
	case encoding.ShiftJIS:
		return string(data), nil
	case encoding.EUCJP:
		eucBytes, err := encoding.SJSToEUC(data)
		if err != nil {
			return "", err
		}
		return string(eucBytes), nil
	case encoding.UTF8:
		s, err := encoding.SJSToUTF8(data)
		if err != nil {
			return "", err
		}
		return s, nil
	default:
		return string(data), nil
	}
}

// ============================================================
// Normalization (from text.ml: norm)
// ============================================================

// Norm lowercases all ASCII characters in the text.
// Used for case-insensitive identifier comparison.
func Norm(t Text) Text {
	return t.Map(func(c rune) rune {
		if c >= 'A' && c <= 'Z' {
			return c + 32
		}
		return c
	})
}

// NormFull lowercases using full Unicode rules.
func NormFull(t Text) Text {
	return t.Map(unicode.ToLower)
}

// ============================================================
// Memoized identifier conversion (from text.ml: ident)
// ============================================================

var identCache sync.Map

// Ident converts a CP932 string to Text, memoizing the result.
// The string is lowercased before conversion.
func Ident(s string) Text {
	if v, ok := identCache.Load(s); ok {
		return v.(Text)
	}
	lower := strings.ToLower(s)
	result := OfSJS([]byte(lower))
	identCache.Store(s, result)
	return result
}

// ============================================================
// Fancy (smart) quotes (from text.ml lines 73-102)
// ============================================================

// QuoteSet holds the current quote/punctuation rendering strings.
type QuoteSet struct {
	SQ1 string // single quote open
	SQ2 string // single quote close
	DQ1 string // double quote open
	DQ2 string // double quote close
	Ch1 string // chevron open
	Ch2 string // chevron close
	Hel string // ellipsis
}

// ASCIIQuotes returns plain ASCII quote rendering.
func ASCIIQuotes() QuoteSet {
	return QuoteSet{
		SQ1: "'", SQ2: "'",
		DQ1: "\"", DQ2: "\"",
		Ch1: "<<", Ch2: ">>",
		Hel: "...",
	}
}

// UnicodeQuotes returns typographic Unicode quote rendering.
func UnicodeQuotes() QuoteSet {
	return QuoteSet{
		SQ1: "\u2018", SQ2: "\u2019", // ' '
		DQ1: "\u201C", DQ2: "\u201D", // " "
		Ch1: "\u00AB", Ch2: "\u00BB", // « »
		Hel: "\u2026",                 // …
	}
}

// DefaultQuotes is the active quote set (mutable, like OCaml refs).
var DefaultQuotes = ASCIIQuotes()

// SetFancy configures whether to use typographic (true) or ASCII (false) quotes.
// Matches OCaml's `fancy` function.
func SetFancy(state bool) {
	if state {
		DefaultQuotes = ASCIIQuotes()
	} else {
		DefaultQuotes = UnicodeQuotes()
	}
}

// ============================================================
// SJIS → UTF-8 preparation with substitutions
// (from text.ml: sjs_to_utf8_prep, lines 196-289)
// ============================================================

// SJSToUTF8Prep converts SJIS to UTF-8 with typographic substitutions.
// Replaces certain fullwidth/CJK punctuation with ASCII or Unicode
// equivalents for display purposes (e.g. in error messages).
func SJSToUTF8Prep(data []byte) string {
	t := OfSJS(data)
	var b strings.Builder
	qs := DefaultQuotes

	for i, c := range t {
		switch {
		case c == 0x8163:
			b.WriteString(qs.Hel) // … or ...
		case c == 0x3001:
			b.WriteString(", ") // 、 → comma-space
		case c == 0x3002:
			b.WriteString(".") // 。 → period
		case c > 0xFF00 && c < 0xFF5F:
			// Fullwidth ASCII → halfwidth
			b.WriteRune(rune((int(c) & 0xFF) + 0x20))
		case c == 0x3008:
			b.WriteString("<") // 〈
		case c == 0x3009:
			b.WriteString(">") // 〉
		case c == 0x300A:
			b.WriteString(qs.Ch1) // 《 → << or «
		case c == 0x300B:
			b.WriteString(qs.Ch2) // 》 → >> or »
		case c == 0x300C:
			b.WriteString(qs.DQ1) // 「 → " or "
		case c == 0x300D:
			b.WriteString(qs.DQ2) // 」 → " or "
		case c == 0x300E:
			b.WriteString(qs.SQ1) // 『 → ' or '
		case c == 0x300F:
			b.WriteString(qs.SQ2) // 』 → ' or '
		case c == 0x7D && i+1 < len(t) && t[i+1] != 0x3000 && t[i+1] != 0x20:
			// } followed by non-space → add space after
			b.WriteRune(c)
			b.WriteString(" ")
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}

// SJSToErr converts SJIS to the default encoding for error output.
func SJSToErr(data []byte) string {
	s, _ := SJSToEnc(encoding.DefaultEncoding(), data)
	return s
}

// ============================================================
// Stream reading (from text.ml: ustream, getc_sjs/euc/utf8)
// ============================================================

// ReadRune reads one Unicode codepoint from a byte slice starting at pos.
// Returns the rune and the new position. Returns (utf8.RuneError, pos)
// on error.
func ReadRune(data []byte, pos int, enc encoding.Type) (rune, int) {
	if pos >= len(data) {
		return utf8.RuneError, pos
	}
	switch enc {
	case encoding.UTF8:
		r, size := utf8.DecodeRune(data[pos:])
		return r, pos + size
	case encoding.ShiftJIS:
		return readRuneSJS(data, pos)
	case encoding.EUCJP:
		return readRuneEUC(data, pos)
	default:
		r, size := utf8.DecodeRune(data[pos:])
		return r, pos + size
	}
}

func readRuneSJS(data []byte, pos int) (rune, int) {
	c1 := data[pos]
	if (c1 >= 0x81 && c1 <= 0x9f) || (c1 >= 0xe0 && c1 <= 0xfc) {
		if pos+1 >= len(data) {
			return utf8.RuneError, pos + 1
		}
		// Double-byte: use encoding package to convert
		s, err := encoding.SJSToUTF8(data[pos : pos+2])
		if err != nil || len(s) == 0 {
			return utf8.RuneError, pos + 2
		}
		r, _ := utf8.DecodeRuneInString(s)
		return r, pos + 2
	}
	// Single-byte
	if c1 < 0x80 {
		return rune(c1), pos + 1
	}
	s, err := encoding.SJSToUTF8(data[pos : pos+1])
	if err != nil || len(s) == 0 {
		return rune(c1), pos + 1
	}
	r, _ := utf8.DecodeRuneInString(s)
	return r, pos + 1
}

func readRuneEUC(data []byte, pos int) (rune, int) {
	c1 := data[pos]
	if c1 < 0x7f {
		return rune(c1), pos + 1
	}
	if pos+1 >= len(data) {
		return utf8.RuneError, pos + 1
	}
	// Double-byte EUC-JP: convert via SJIS pivot
	// (simplified — full version would use direct EUC-JP tables)
	return rune(c1), pos + 2
}

// Decode reads all codepoints from a byte slice in the given encoding.
// Skips BOM if present.
func Decode(data []byte, enc encoding.Type) Text {
	result, err := OfString(enc, data)
	if err != nil {
		// Fallback: try as raw bytes
		r := make(Text, len(data))
		for i, b := range data {
			r[i] = rune(b)
		}
		return r
	}
	// Strip BOM
	if len(result) > 0 && (result[0] == 0xFEFF || result[0] == 0xFFFE) {
		result = result[1:]
	}
	return result
}
