// Package encoding provides character encoding conversions for RealLive text.
// Transposed from OCaml's encoding.ml, cp932.ml, cp936.ml, cp949.ml, and their
// massive codepage lookup tables (cp932_in.ml, cp936_in.ml, cp949_in.ml).
//
// In Go, all this is replaced by golang.org/x/text/encoding which provides
// production-quality implementations of these codepages. This eliminates
// ~4000 lines of OCaml code (the auto-generated codepage tables).
package encoding

import (
	"fmt"
	"strings"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// Type represents a character encoding used by RealLive games.
type Type int

const (
	ShiftJIS  Type = iota // Japanese (CP932) - most games
	EUCJP                 // Japanese (EUC-JP) - rare
	UTF8                  // UTF-8 - newer games
	GBK                   // Simplified Chinese (CP936)
	EUC_KR                // Korean (CP949)
	Other                 // Unknown/unsupported
)

func (t Type) String() string {
	switch t {
	case ShiftJIS:
		return "Shift_JIS"
	case EUCJP:
		return "EUC-JP"
	case UTF8:
		return "UTF-8"
	case GBK:
		return "GBK"
	case EUC_KR:
		return "EUC-KR"
	default:
		return "Other"
	}
}

// Parse returns the encoding type from a string name.
// Equivalent to OCaml's enc_type / encoding_of_string.
func Parse(name string) Type {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "SHIFTJIS", "SHIFT_JIS", "SHIFT-JIS", "SJS", "SJIS", "CP932":
		return ShiftJIS
	case "EUC-JP", "EUC", "EUC_JP":
		return EUCJP
	case "UTF8", "UTF-8":
		return UTF8
	case "GBK", "CP936", "GB2312":
		return GBK
	case "EUC-KR", "EUC_KR", "CP949":
		return EUC_KR
	default:
		return Other
	}
}

// ToUTF8 converts a byte string from the given encoding to UTF-8.
func ToUTF8(data []byte, enc Type) (string, error) {
	var decoder *transform.Reader
	reader := strings.NewReader(string(data))

	switch enc {
	case ShiftJIS:
		decoder = transform.NewReader(reader, japanese.ShiftJIS.NewDecoder())
	case EUCJP:
		decoder = transform.NewReader(reader, japanese.EUCJP.NewDecoder())
	case UTF8:
		return string(data), nil
	case GBK:
		decoder = transform.NewReader(reader, simplifiedchinese.GBK.NewDecoder())
	case EUC_KR:
		decoder = transform.NewReader(reader, korean.EUCKR.NewDecoder())
	default:
		return "", fmt.Errorf("unsupported encoding: %v", enc)
	}

	result := make([]byte, 0, len(data)*3)
	buf := make([]byte, 4096)
	for {
		n, err := decoder.Read(buf)
		if n > 0 {
			result = append(result, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	return string(result), nil
}

// FromUTF8 converts a UTF-8 string to the given encoding.
func FromUTF8(text string, enc Type) ([]byte, error) {
	return FromUTF8String(text, enc)
}

// FromUTF8String converts a UTF-8 string to the given encoding using transform.String.
func FromUTF8String(text string, enc Type) ([]byte, error) {
	var encoder transform.Transformer
	switch enc {
	case ShiftJIS:
		encoder = japanese.ShiftJIS.NewEncoder()
	case EUCJP:
		encoder = japanese.EUCJP.NewEncoder()
	case UTF8:
		return []byte(text), nil
	case GBK:
		encoder = simplifiedchinese.GBK.NewEncoder()
	case EUC_KR:
		encoder = korean.EUCKR.NewEncoder()
	default:
		return nil, fmt.Errorf("unsupported encoding: %v", enc)
	}

	result, _, err := transform.String(encoder, text)
	if err != nil {
		return nil, fmt.Errorf("encoding conversion failed: %w", err)
	}
	return []byte(result), nil
}

// SJSToUTF8 is a convenience function for the most common conversion.
func SJSToUTF8(data []byte) (string, error) {
	return ToUTF8(data, ShiftJIS)
}

// UTF8ToSJS is a convenience function for the most common conversion.
func UTF8ToSJS(text string) ([]byte, error) {
	return FromUTF8String(text, ShiftJIS)
}

// IsLeadByte returns true if the byte is a lead byte in the given encoding.
// Used for safely iterating through multibyte strings.
func IsLeadByte(b byte, enc Type) bool {
	switch enc {
	case ShiftJIS:
		return (b >= 0x81 && b <= 0x9F) || (b >= 0xE0 && b <= 0xFC)
	case EUCJP:
		return b >= 0xA1 && b <= 0xFE
	case GBK:
		return b >= 0x81 && b <= 0xFE
	case EUC_KR:
		return b >= 0x81 && b <= 0xFE
	default:
		return false
	}
}

// SJSToEUC converts a Shift_JIS byte string to EUC-JP.
// Equivalent to OCaml's sjs_to_euc. Kept for compatibility; prefer ToUTF8.
func SJSToEUC(data []byte) ([]byte, error) {
	// Go through UTF-8 as intermediary
	utf8Str, err := ToUTF8(data, ShiftJIS)
	if err != nil {
		return nil, err
	}
	return FromUTF8String(utf8Str, EUCJP)
}

// DefaultEncoding returns the default encoding (ShiftJIS for Japanese games).
func DefaultEncoding() Type {
	return ShiftJIS
}

// --- UTF-8 utilities (replaces OCaml's Text module partially) ---

// DecodeUTF8 is a no-op retained for interface compatibility. Go strings are UTF-8.
func DecodeUTF8(s string) string {
	return s
}

// We don't need the complex OCaml Text.t type because Go natively handles UTF-8.
// The entire text.ml module (623 lines + 537 lines textTransforms.ml) is largely
// unnecessary in Go since:
//   - Go strings are UTF-8 natively
//   - golang.org/x/text handles all encoding conversions
//   - Unicode normalization: golang.org/x/text/unicode/norm

// Note: for the compiler (rlc), we'll need some of the text transform logic later,
// but for kprl (the first tool to port), basic encoding is sufficient.

// Ensure unicode package is importable (for go.sum generation)
var _ = unicode.UTF8
