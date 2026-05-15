package texttransforms

import (
	"testing"

	"github.com/yoremi/rldev-go/pkg/text"
)

// TestBadRunesTracksUnmappable feeds CP932-incompatible runes through
// ToBytecode and verifies BadRunes() reports each distinct offender
// exactly once. Without this hook, codegen had no way to surface the
// silent space substitution that the encoder performs in ForceEncode
// mode — the prime cause of "compiles fine but won't boot".
func TestBadRunesTracksUnmappable(t *testing.T) {
	old := ForceEncode
	ForceEncode = true
	defer func() { ForceEncode = old }()

	// Hangul syllables — not in JIS X 0208, so CP932 can't represent
	// them. Two distinct runes, repeated, plus one ASCII letter.
	ResetBadChars()
	input := "A\uACE0\uB098\uACE0" // A 고 나 고
	_, err := ToBytecode(text.Text([]rune(input)))
	if err != nil {
		t.Fatalf("ForceEncode should not error, got: %v", err)
	}

	bad := BadRunes()
	if len(bad) != 2 {
		t.Fatalf("expected 2 distinct bad runes, got %d: %#v", len(bad), bad)
	}
	if bad[0] != 0xACE0 || bad[1] != 0xB098 {
		t.Errorf("wrong runes / wrong order: got U+%04X, U+%04X", bad[0], bad[1])
	}

	// A fresh Reset clears the slice.
	ResetBadChars()
	if c := BadCharCount(); c != 0 {
		t.Errorf("ResetBadChars left %d entries", c)
	}
}

// TestBadRunesEmptyOnClean verifies pure-ASCII / pure-JIS input
// leaves the tracker empty.
func TestBadRunesEmptyOnClean(t *testing.T) {
	ResetBadChars()
	_, err := ToBytecode(text.Text([]rune("Hello, ありがとう")))
	if err != nil {
		t.Fatalf("clean text errored: %v", err)
	}
	if c := BadCharCount(); c != 0 {
		t.Errorf("clean text produced %d bad runes", c)
	}
}
