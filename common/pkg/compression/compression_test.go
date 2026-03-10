package compression

import (
	"bytes"
	"testing"

	"github.com/yoremi/rldev-go/pkg/binarray"
	"github.com/yoremi/rldev-go/pkg/gamedef"
)

func TestApplyMaskRoundTrip(t *testing.T) {
	// XOR mask is its own inverse: applying it twice should restore original
	original := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99}
	for i := 0; i < 300; i++ {
		original = append(original, byte(i%256))
	}

	buf := binarray.FromBytes(make([]byte, len(original)))
	copy(buf.Data, original)

	ApplyMask(buf, 0)
	// Should be different now
	if bytes.Equal(buf.Data, original) {
		t.Fatal("ApplyMask should modify data")
	}

	// Apply again to restore
	ApplyMask(buf, 0)
	if !bytes.Equal(buf.Data, original) {
		t.Fatal("Double ApplyMask should restore original data")
	}
}

func TestApplyMaskWithOffset(t *testing.T) {
	data := make([]byte, 300)
	for i := range data {
		data[i] = byte(i % 256)
	}

	buf := binarray.FromBytes(make([]byte, len(data)))
	copy(buf.Data, data)

	origin := 100
	ApplyMask(buf, origin)

	// Bytes before origin should be unchanged
	for i := 0; i < origin; i++ {
		if buf.Data[i] != data[i] {
			t.Errorf("Byte at %d was modified (before origin): got %02x want %02x", i, buf.Data[i], data[i])
			break
		}
	}

	// Bytes at/after origin should be changed
	changed := false
	for i := origin; i < len(data); i++ {
		if buf.Data[i] != data[i] {
			changed = true
			break
		}
	}
	if !changed {
		t.Error("Bytes after origin should be modified")
	}
}

func TestXORKeysRoundTrip(t *testing.T) {
	original := make([]byte, 1024)
	for i := range original {
		original[i] = byte(i % 256)
	}

	buf := binarray.FromBytes(make([]byte, len(original)))
	copy(buf.Data, original)

	keys := gamedef.KeyLB

	ApplyXORKeys(buf, keys)
	if bytes.Equal(buf.Data, original) {
		t.Fatal("XOR keys should modify data")
	}

	// Apply again (XOR is its own inverse)
	ApplyXORKeys(buf, keys)
	if !bytes.Equal(buf.Data, original) {
		t.Fatal("Double XOR should restore original data")
	}
}

func TestDecompressCompress(t *testing.T) {
	// Create some test data with repetitive patterns (good for LZ77)
	original := make([]byte, 512)
	for i := range original {
		original[i] = byte((i * 7) % 256) // Pseudo-random but deterministic
	}
	// Add some repetition for compression
	copy(original[128:256], original[0:128])
	copy(original[384:512], original[256:384])

	// Compress
	compressed := Compress(original)
	if len(compressed) == 0 {
		t.Fatal("Compress returned empty result")
	}

	t.Logf("Original: %d bytes, Compressed: %d bytes (%.1f%%)",
		len(original), len(compressed), float64(len(compressed))/float64(len(original))*100)

	// Build the expected format for decompression (8-byte header + compressed data)
	withHeader := make([]byte, 8+len(compressed))
	withHeader[0] = byte(len(compressed) & 0xff)
	withHeader[1] = byte((len(compressed) >> 8) & 0xff)
	withHeader[2] = byte((len(compressed) >> 16) & 0xff)
	withHeader[3] = byte((len(compressed) >> 24) & 0xff)
	withHeader[4] = byte(len(original) & 0xff)
	withHeader[5] = byte((len(original) >> 8) & 0xff)
	withHeader[6] = byte((len(original) >> 16) & 0xff)
	withHeader[7] = byte((len(original) >> 24) & 0xff)
	copy(withHeader[8:], compressed)

	// Decompress
	decompressed := make([]byte, len(original))
	err := Decompress(withHeader, decompressed)
	if err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}

	if !bytes.Equal(decompressed, original) {
		// Find first difference
		for i := range original {
			if i >= len(decompressed) {
				t.Errorf("Decompressed too short at byte %d", i)
				break
			}
			if decompressed[i] != original[i] {
				t.Errorf("Mismatch at byte %d: got %02x want %02x", i, decompressed[i], original[i])
				break
			}
		}
		t.Fatal("Round-trip compression/decompression failed")
	}
}

func TestCompressReductionOnRepetitiveData(t *testing.T) {
	// Highly repetitive data should compress well
	data := bytes.Repeat([]byte("Hello World! "), 100)
	compressed := Compress(data)
	ratio := float64(len(compressed)) / float64(len(data))
	t.Logf("Repetitive: %d -> %d bytes (%.1f%%)", len(data), len(compressed), ratio*100)
	if ratio > 0.5 {
		t.Errorf("Expected better compression ratio for repetitive data, got %.1f%%", ratio*100)
	}
}

func TestKnownGameKeys(t *testing.T) {
	// Verify all known game keys exist and have valid data
	for name, keys := range gamedef.KnownGames {
		if len(keys) == 0 {
			t.Errorf("Game %s has no keys", name)
		}
		for i, key := range keys {
			if key.Length <= 0 {
				t.Errorf("Game %s key %d has invalid length %d", name, i, key.Length)
			}
			// Check key data isn't all zeros
			allZero := true
			for _, b := range key.Data {
				if b != 0 {
					allZero = false
					break
				}
			}
			if allZero {
				t.Errorf("Game %s key %d is all zeros", name, i)
			}
		}
	}
}
