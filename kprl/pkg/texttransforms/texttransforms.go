// Package texttransforms converts Unicode text to RealLive bytecode encoding.
//
// Transposed from OCaml's common/textTransforms.ml (538 lines, ~250 active).
//
// RealLive internally uses Shift-JIS (CP932) as its bytecode encoding.
// For non-Japanese games, text must be mapped into the CP932 codespace
// using one of three transformations:
//
//   - None:    standard Shift-JIS (Japanese games)
//   - Chinese: GBK (CP936) → remapped into CP932 codespace
//   - Western: CP1252 (Latin-1) → packed into CP932 single/double byte
//   - Korean:  CP949 (KS X 1001) → remapped into CP932 codespace
//
// The encode functions take Unicode codepoints and produce byte strings
// that fit within CP932's byte ranges, using custom remapping tables
// so RealLive's renderer can display the characters correctly with
// patched font files.
//
// Main entry points:
//   - SetEncoding(name) — configure the active transformation
//   - ToBytecode(text)  — convert Unicode text to RealLive bytecode
//   - ReadBytecode(data) — convert bytecode back to Unicode text
package texttransforms

import (
	"fmt"
	"strings"
	"sync"

	"github.com/yoremi/rldev-go/pkg/encoding"
	"github.com/yoremi/rldev-go/pkg/text"
)

// ============================================================
// Output encoding mode
// ============================================================

// EncMode identifies the active output transformation.
type EncMode int

const (
	EncNone    EncMode = iota // Standard Shift-JIS (Japanese)
	EncChinese                // GBK → CP932 codespace
	EncWestern                // CP1252 → CP932 codespace
	EncKorean                 // CP949 → CP932 codespace
)

// ForceEncode controls whether unmappable characters are replaced with
// space (true) or cause an error (false).
var ForceEncode bool

// outenc is the current active encoding mode.
var outenc = EncNone
var outencMu sync.Mutex

// SetMode sets the active output encoding mode.
func SetMode(mode EncMode) {
	outencMu.Lock()
	outenc = mode
	outencMu.Unlock()
}

// GetMode returns the active output encoding mode.
func GetMode() EncMode {
	outencMu.Lock()
	defer outencMu.Unlock()
	return outenc
}

// Describe returns a human-readable description of the current transformation.
func Describe() string {
	switch outenc {
	case EncNone:
		return "no output transformation"
	case EncChinese:
		return "the `Chinese' transformation of GB2312"
	case EncWestern:
		return "the `Western' transformation of CP1252"
	case EncKorean:
		return "the `Korean' transformation of KS X 1001"
	}
	return "unknown"
}

// ============================================================
// Encoding name parsing (from textTransforms.ml enc_of_string)
// ============================================================

// ParseMode converts an encoding name string to an EncMode.
func ParseMode(name string) (EncMode, error) {
	switch strings.ToUpper(name) {
	case "", "NONE", "JAPANESE", "JP", "CP932", "SHIFT_JIS", "SJIS", "SHIFT-JIS", "SHIFTJIS":
		return EncNone, nil
	case "CHINESE", "ZH", "CN", "CP936", "GB2312", "GBK":
		return EncChinese, nil
	case "WESTERN", "ENGLISH", "EN", "CP1252":
		return EncWestern, nil
	case "KOREAN", "KO", "KR", "CP949", "KSC", "KSC5601", "KSX1001", "HANGUL":
		return EncKorean, nil
	}
	return EncNone, fmt.Errorf("unknown output transformation '%s'", name)
}

// SetEncoding configures the output encoding from a name string.
func SetEncoding(name string) error {
	mode, err := ParseMode(name)
	if err != nil {
		return err
	}
	SetMode(mode)
	return nil
}

// ============================================================
// Bad character tracking
// ============================================================

var badChars sync.Map // rune → count
var complained bool

// Complain generates a warning message for an unmappable character.
func Complain(ch rune, kind string) string {
	// Increment counter
	countI, _ := badChars.LoadOrStore(ch, 0)
	badChars.Store(ch, countI.(int)+1)

	if complained {
		return fmt.Sprintf("%s: U+%04X", kind, ch)
	}
	complained = true
	return fmt.Sprintf("cannot represent U+%04X in RealLive bytecode [%s] with %s",
		ch, kind, Describe())
}

// BadCharCount returns the number of distinct bad characters encountered.
func BadCharCount() int {
	count := 0
	badChars.Range(func(_, _ interface{}) bool { count++; return true })
	return count
}

// ResetBadChars clears the bad character tracking state.
func ResetBadChars() {
	badChars = sync.Map{}
	complained = false
}

// ============================================================
// Main entry points
// ============================================================

// ToBytecode converts Unicode text to RealLive bytecode bytes.
// Uses the currently active encoding transformation.
//
// This is the primary entry point for text compilation — called by
// textout and rlbabel compilation when emitting text strings.
func ToBytecode(t text.Text) ([]byte, error) {
	mode := GetMode()
	switch mode {
	case EncNone:
		return toSJSBytecode(t)
	case EncChinese:
		return encodeChinese(t)
	case EncWestern:
		return encodeWestern(t)
	case EncKorean:
		return encodeKorean(t)
	}
	return nil, fmt.Errorf("unknown encoding mode %d", mode)
}

// ReadBytecode converts RealLive bytecode bytes back to Unicode text.
// Uses the currently active encoding transformation.
func ReadBytecode(data []byte) (text.Text, error) {
	mode := GetMode()
	switch mode {
	case EncNone:
		return text.OfSJS(data), nil
	case EncChinese:
		return decodeChinese(data)
	case EncWestern:
		return decodeWestern(data)
	case EncKorean:
		return decodeKorean(data)
	}
	return nil, fmt.Errorf("unknown encoding mode %d", mode)
}

// ============================================================
// Standard SJIS encoding (from textTransforms.ml to_sjs_bytecode)
// ============================================================

// toSJSBytecode converts Unicode text to Shift-JIS bytecode.
// Uses the encoding package for the conversion; raises BadChar
// for unmappable codepoints unless ForceEncode is set.
func toSJSBytecode(t text.Text) ([]byte, error) {
	utf8 := text.ToUTF8(t)
	b, err := encoding.UTF8ToSJS(utf8)
	if err != nil {
		if ForceEncode {
			// Replace unmappable chars with spaces
			var result []byte
			for _, r := range t {
				if r <= 0x7f {
					result = append(result, byte(r))
				} else {
					s := string([]rune{r})
					sb, err := encoding.UTF8ToSJS(s)
					if err != nil {
						result = append(result, ' ')
					} else {
						result = append(result, sb...)
					}
				}
			}
			return result, nil
		}
		return nil, err
	}
	return b, nil
}

// ============================================================
// Western (CP1252) encoding (from textTransforms.ml encode_cp1252)
// ============================================================

// cp1252ToUnicode maps CP1252 special bytes (0x80-0x9F) to Unicode.
var cp1252ToUnicode = map[byte]rune{
	0x80: 0x20AC, 0x82: 0x201A, 0x83: 0x0192, 0x84: 0x201E,
	0x85: 0x2026, 0x86: 0x2020, 0x87: 0x2021, 0x88: 0x02C6,
	0x89: 0x2030, 0x8A: 0x0160, 0x8B: 0x2039, 0x8C: 0x0152,
	0x8E: 0x017D, 0x91: 0x2018, 0x92: 0x2019, 0x93: 0x201C,
	0x94: 0x201D, 0x95: 0x2022, 0x96: 0x2013, 0x97: 0x2014,
	0x98: 0x02DC, 0x99: 0x2122, 0x9A: 0x0161, 0x9B: 0x203A,
	0x9C: 0x0153, 0x9E: 0x017E, 0x9F: 0x0178,
}

// unicodeToCP1252 is the reverse mapping.
var unicodeToCP1252 map[rune]byte

func init() {
	unicodeToCP1252 = make(map[rune]byte, len(cp1252ToUnicode))
	for b, r := range cp1252ToUnicode {
		unicodeToCP1252[r] = b
	}
	unicodeToCP1252[0xFF5E] = 0x7E // fullwidth tilde → ~
}

// encodeWestern encodes Unicode text using the CP1252 transformation.
// Characters are packed into the CP932 codespace as follows:
//   - 0x00-0x7F: single byte (direct)
//   - 0x80-0xBF: double byte 0x89 + char
//   - 0xC0-0xFE: single byte (char - 0x1F)
//   - 0xFF:       double byte 0x89 0xC0
func encodeWestern(t text.Text) ([]byte, error) {
	var b []byte
	for _, ch := range t {
		// Name variables / exfont: pass through as SJIS double-byte
		if ch >= 0x1FF00 && ch < 0x20000 {
			remapped := ch - 0x10000
			sjsBytes, err := encoding.UTF8ToSJS(string([]rune{rune(remapped)}))
			if err == nil {
				b = append(b, sjsBytes...)
				continue
			}
		}

		// Map Unicode → CP1252 byte
		wc := ch
		if mapped, ok := unicodeToCP1252[ch]; ok {
			wc = rune(mapped)
		}

		if wc < 0 || wc > 0xFF {
			if ForceEncode {
				wc = 0x20
			} else {
				return nil, fmt.Errorf("cannot encode U+%04X in CP1252 Western mode", ch)
			}
		}

		c := byte(wc)
		switch {
		case c <= 0x7F:
			b = append(b, c)
		case c >= 0x80 && c <= 0xBF:
			b = append(b, 0x89, c)
		case c >= 0xC0 && c <= 0xFE:
			b = append(b, c-0x1F)
		case c == 0xFF:
			b = append(b, 0x89, 0xC0)
		}
	}
	return b, nil
}

// decodeWestern decodes CP1252-in-CP932 bytecode back to Unicode text.
func decodeWestern(data []byte) (text.Text, error) {
	buf := text.NewBuf(len(data))
	i := 0
	for i < len(data) {
		c := data[i]
		switch {
		case c <= 0x7F:
			buf.AddRune(rune(c))
			i++
		case c == 0x81 || c == 0x82:
			// SJIS double-byte passthrough
			if i+1 >= len(data) {
				return nil, fmt.Errorf("decode_cp1252: truncated SJIS at %d", i)
			}
			sjsData := data[i : i+2]
			t := text.OfSJS(sjsData)
			buf.AddText(t)
			i += 2
		case c == 0x89:
			// Encoded CP1252 character follows
			i++
			if i >= len(data) {
				return nil, fmt.Errorf("decode_cp1252: truncated at %d", i)
			}
			ec := data[i]
			if ec == 0xC0 {
				buf.AddRune(0xFF)
			} else if r, ok := cp1252ToUnicode[ec]; ok {
				buf.AddRune(r)
			} else {
				buf.AddRune(rune(ec))
			}
			i++
		case c >= 0xA1 && c <= 0xDF:
			// Remapped high byte: c + 0x1F
			buf.AddRune(rune(c) + 0x1F)
			i++
		default:
			return nil, fmt.Errorf("decode_cp1252: unexpected byte 0x%02X at %d", c, i)
		}
	}
	return buf.Contents(), nil
}

// ============================================================
// Chinese (GBK/CP936) encoding (from textTransforms.ml encode_kfc)
// ============================================================

// encodeChinese encodes Unicode text using the GBK transformation.
// GBK double-byte characters are remapped into the CP932 codespace.
func encodeChinese(t text.Text) ([]byte, error) {
	var b []byte
	for _, ch := range t {
		if ch <= 0x7F {
			b = append(b, byte(ch))
			continue
		}

		// Convert to GBK via encoding package
		gbk, err := encoding.FromUTF8(string([]rune{ch}), encoding.GBK)
		if err != nil || len(gbk) < 2 {
			if ForceEncode {
				b = append(b, ' ')
				continue
			}
			return nil, fmt.Errorf("cannot encode U+%04X in Chinese/GBK mode", ch)
		}

		gbCode := int(gbk[0])<<8 | int(gbk[1])

		// Special cases: CP932 equivalents (OCaml L55-57)
		switch gbCode {
		case 0xA1B8:
			b = append(b, 0x81, 0x75)
		case 0xA1BA:
			b = append(b, 0x81, 0x77)
		case 0xA3A8:
			b = append(b, 0x81, 0x69)
		default:
			// General remapping: GBK → CP932 codespace
			c1 := (gbCode>>8)&0xFF - 0xA1
			c2 := gbCode&0xFF - 0xA1
			if c1 < 0 || c2 < 0 {
				if ForceEncode {
					b = append(b, ' ')
					continue
				}
				return nil, fmt.Errorf("cannot remap GBK %04X", gbCode)
			}
			nc1 := c1*2 + (c2 % 2) + 0x40
			nc2 := c2/2 + 0x81
			if nc1 >= 0x7F {
				nc1++
			}
			if nc2 > 0x9F {
				nc2 += 0x40
			}
			b = append(b, byte(nc2), byte(nc1))
		}
	}
	return b, nil
}

// decodeChinese decodes GBK-in-CP932 bytecode back to Unicode text.
func decodeChinese(data []byte) (text.Text, error) {
	buf := text.NewBuf(len(data))
	i := 0
	for i < len(data) {
		c := data[i]
		if c <= 0x7F {
			buf.AddRune(rune(c))
			i++
			continue
		}
		if i+1 >= len(data) {
			return nil, fmt.Errorf("decode_kfc: truncated at %d", i)
		}
		a1, a2 := int(c), int(data[i+1])
		combined := (a1 << 8) | a2

		// Reverse special cases
		var gc1, gc2 int
		switch combined {
		case 0x8175:
			gc1, gc2 = 0xA1-0xA1, 0xB8-0xA1
		case 0x8177:
			gc1, gc2 = 0xA1-0xA1, 0xBA-0xA1
		case 0x8169:
			gc1, gc2 = 0xA3-0xA1, 0xA8-0xA1
		default:
			// General reverse mapping
			nc2 := a1
			nc1 := a2
			if nc2 > 0xDF {
				nc2 -= 0x40
			}
			c2r := (nc2 - 0x81) * 2
			if nc1 >= 0x80 {
				nc1--
			}
			c1r := nc1 - 0x40
			gc1 = c1r / 2
			gc2 = c2r + (c1r % 2)
		}

		// Convert back through GBK
		gbCode := ((gc1 + 0xA1) << 8) | (gc2 + 0xA1)
		gbBytes := []byte{byte(gbCode >> 8), byte(gbCode & 0xFF)}
		utf8Str, err := encoding.ToUTF8(gbBytes, encoding.GBK)
		if err != nil {
			return nil, fmt.Errorf("decode_kfc: cannot decode %04X", combined)
		}
		for _, r := range utf8Str {
			buf.AddRune(r)
		}
		i += 2
	}
	return buf.Contents(), nil
}

// ============================================================
// Korean (CP949) encoding (from textTransforms.ml encode_hangul)
// ============================================================

// encodeKorean encodes Unicode text using the CP949/KS X 1001 transformation.
// Korean characters are remapped into the CP932 codespace using a complex
// multi-range scheme (see textTransforms.ml lines 178-240).
func encodeKorean(t text.Text) ([]byte, error) {
	var b []byte
	for _, ch := range t {
		if ch <= 0x7F {
			b = append(b, byte(ch))
			continue
		}

		// Convert to CP949 via encoding package
		kr, err := encoding.FromUTF8(string([]rune{ch}), encoding.EUC_KR)
		if err != nil || len(kr) < 2 {
			if ForceEncode {
				b = append(b, ' ')
				continue
			}
			return nil, fmt.Errorf("cannot encode U+%04X in Korean mode", ch)
		}

		haCode := int(kr[0])<<8 | int(kr[1])

		// Special cases (OCaml L212-214)
		switch haCode {
		case 0xA1B8:
			b = append(b, 0x81, 0x75)
		case 0xA1BA:
			b = append(b, 0x81, 0x77)
		case 0xA3A8:
			b = append(b, 0x81, 0x69)
		default:
			// General remapping into CP932 codespace
			c1 := haCode >> 8 & 0xFF
			c2 := haCode & 0xFF

			var nc1, nc2 int
			if c1 <= 0x9F {
				// 8141-9FFE range → remap onto 8101-9685
				cc1 := c1 - 0x81
				cc2 := c2
				if cc2 < 0x5B {
					cc2 -= 0x41
				} else if cc2 < 0x7B {
					cc2 -= 0x47
				} else {
					cc2 -= 0x4D
				}
				idx := cc1*177 + cc2
				nc1 = 0x81 + idx/255
				nc2 = 0x01 + idx%255
			} else if c1 >= 0xA1 && c2 >= 0xA1 {
				// A1A1-C8FE range → remap onto 9701-E6BE
				idx := (c1-0xA1)*94 + (c2 - 0xA1)
				nc1 = 0x97 + idx/255
				if nc1 > 0x9F {
					nc1 += 0x41
				}
				nc2 = 0x01 + idx%255
			} else {
				// Remainder → simplified fallback
				if ForceEncode {
					b = append(b, ' ')
					continue
				}
				return nil, fmt.Errorf("cannot remap Korean %04X", haCode)
			}
			b = append(b, byte(nc1), byte(nc2))
		}
	}
	return b, nil
}

// decodeKorean decodes Korean-in-CP932 bytecode back to Unicode text.
func decodeKorean(data []byte) (text.Text, error) {
	buf := text.NewBuf(len(data))
	i := 0
	for i < len(data) {
		c := data[i]
		if c <= 0x7F {
			buf.AddRune(rune(c))
			i++
			continue
		}
		if i+1 >= len(data) {
			return nil, fmt.Errorf("decode_hangul: truncated at %d", i)
		}
		a1, a2 := int(c), int(data[i+1])
		combined := (a1 << 8) | a2

		// Reverse special cases
		var kc1, kc2 int
		switch combined {
		case 0x8175:
			kc1, kc2 = 0xA1, 0xB8
		case 0x8177:
			kc1, kc2 = 0xA1, 0xBA
		case 0x8169:
			kc1, kc2 = 0xA3, 0xA8
		case 0xEA40:
			kc1, kc2 = 0x81, 0xC1
		case 0xEA41:
			kc1, kc2 = 0x81, 0xC3
		case 0xEA42:
			kc1, kc2 = 0x81, 0xB5
		default:
			if a1 <= 0x96 {
				idx := (a1-0x81)*255 + a2 - 1
				cc2 := idx % 177
				kc1 = 0x81 + idx/177
				if cc2 < 0x1A {
					kc2 = cc2 + 0x41
				} else if cc2 < 0x34 {
					kc2 = cc2 + 0x47
				} else {
					kc2 = cc2 + 0x4D
				}
			} else if a1 <= 0xE6 {
				base := a1
				if base <= 0x9F {
					base -= 0x97
				} else {
					base -= 0xD8
				}
				idx := base*255 + a2 - 1
				kc1 = 0xA1 + idx/94
				kc2 = 0xA1 + idx%94
			} else {
				return nil, fmt.Errorf("decode_hangul: cannot decode %02X%02X", a1, a2)
			}
		}

		// Convert back through CP949
		krBytes := []byte{byte(kc1), byte(kc2)}
		utf8Str, err := encoding.ToUTF8(krBytes, encoding.EUC_KR)
		if err != nil {
			return nil, fmt.Errorf("decode_hangul: cannot decode %02X%02X → %02X%02X", a1, a2, kc1, kc2)
		}
		for _, r := range utf8Str {
			buf.AddRune(r)
		}
		i += 2
	}
	return buf.Contents(), nil
}
