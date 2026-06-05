// Package rlcmp provides high-level RealLive bytecode file decompression
// and compression. It combines header parsing, LZ77, XOR masking, and
// per-game encryption into single decompress/compress operations.
// Transposed from the active (non-commented) code in OCaml's rlcmp.ml.
package rlcmp

import (
	"fmt"

	"github.com/yoremi/rldev-go/pkg/binarray"
	"github.com/yoremi/rldev-go/pkg/bytecode"
	"github.com/yoremi/rldev-go/pkg/compression"
	"github.com/yoremi/rldev-go/pkg/gamedef"
)

// Decompress returns the decompressed bytecode file data.
// The input buffer is modified (XOR mask applied in place).
// Equivalent to OCaml's Rlcmp.decompress.
//
// Process:
//  1. Read file header
//  2. Apply static XOR mask to data region
//  3. If compressed, decompress with LZ77
//  4. If archived, apply per-game XOR keys
func Decompress(arr *binarray.Buffer, keys []gamedef.XORSubkey, archived bool) (*binarray.Buffer, error) {
	hdr, err := bytecode.ReadFileHeader(arr, archived)
	if err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	// Step 1: Apply static XOR mask
	compression.ApplyMask(arr, hdr.DataOffset)

	// Step 2: If not compressed, return as-is
	if !hdr.IsCompressed {
		return arr, nil
	}

	// Step 3: LZ77 decompress
	rv := binarray.New(hdr.DataOffset + hdr.UncompressedSize)
	// Copy header
	copy(rv.Data[:hdr.DataOffset], arr.Data[:hdr.DataOffset])

	// Decompress data
	compData := arr.Data[hdr.DataOffset : hdr.DataOffset+hdr.CompressedSize]
	dstData := rv.Data[hdr.DataOffset : hdr.DataOffset+hdr.UncompressedSize]
	if err := compression.Decompress(compData, dstData); err != nil {
		return nil, fmt.Errorf("decompression failed: %w", err)
	}

	// Step 4: Apply per-game XOR keys if this was in an archive.
	if hdr.Archived && len(keys) > 0 {
		applyArchiveXORKeys(rv, hdr, keys)
	}

	return rv, nil
}

func applyArchiveXORKeys(rv *binarray.Buffer, hdr bytecode.FileHeader, keys []gamedef.XORSubkey) {
	payload := rv.Data[hdr.DataOffset:]
	keyed := append([]byte(nil), payload...)
	compression.ApplyXORKeys(binarray.FromBytes(keyed), keys)
	if preferPlainXORCandidate(payload, keyed, keys) {
		return
	}
	copy(payload, keyed)
}

func preferPlainXORCandidate(plain, keyed []byte, keys []gamedef.XORSubkey) bool {
	plainScore, keyedScore, covered := scoreXORKeyRanges(plain, keyed, keys)
	if covered < 32 {
		return false
	}
	margin := covered / 8
	if margin < 32 {
		margin = 32
	}
	return plainScore > keyedScore+margin
}

func scoreXORKeyRanges(plain, keyed []byte, keys []gamedef.XORSubkey) (int, int, int) {
	plainScore := 0
	keyedScore := 0
	covered := 0
	for _, sk := range keys {
		start := sk.Offset
		if start < 0 || start >= len(plain) || start >= len(keyed) {
			continue
		}
		end := start + sk.Length
		if end > len(plain) {
			end = len(plain)
		}
		if end > len(keyed) {
			end = len(keyed)
		}
		if end <= start {
			continue
		}
		plainScore += scoreRealLivePayload(plain[start:end])
		keyedScore += scoreRealLivePayload(keyed[start:end])
		covered += end - start
	}
	return plainScore, keyedScore, covered
}

func scoreRealLivePayload(data []byte) int {
	score := 0
	for i := 0; i < len(data); i++ {
		b := data[i]
		switch {
		case b == 0x0a && i+2 < len(data):
			line := int(data[i+1]) | int(data[i+2])<<8
			if line > 0 && line < 10000 {
				score += 6
				i += 2
				continue
			}
		case b == '#' && i+7 < len(data):
			opType := data[i+1]
			argc := int(data[i+5]) | int(data[i+6])<<8
			if opType <= 2 && argc < 64 {
				score += 10
				continue
			}
		case b == '$' && i+1 < len(data):
			if data[i+1] == 0xff {
				score += 3
				continue
			}
			if i+2 < len(data) && data[i+2] == '[' {
				score += 4
				continue
			}
		case b == '\\' && i+1 < len(data):
			op := data[i+1]
			if op <= 0x3d || (op >= 0x14 && op <= 0x1e) {
				score += 3
				continue
			}
		case b == '(' || b == ')' || b == '[' || b == ']' || b == '@' || b == '{' || b == '}' || b == '"':
			score++
			continue
		case b < 0x20 && b != 0 && b != 1 && b != 2:
			score--
		}
	}
	return score
}

// Compress returns the compressed bytecode file data.
// The input buffer is not modified (a copy is made).
// Equivalent to OCaml's Rlcmp.compress.
//
// Process:
//  1. Read file header
//  2. If not compressible, just apply mask and return
//  3. Apply per-game XOR keys (reverse of decompress)
//  4. LZ77 compress the data
//  5. Build output with updated header
//  6. Apply static XOR mask
func Compress(arr *binarray.Buffer, keys []gamedef.XORSubkey) (*binarray.Buffer, error) {
	hdr, err := bytecode.ReadFileHeader(arr, false)
	if err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	dataOffset := hdr.DataOffset

	if !hdr.IsCompressed {
		// Not compressible: just copy, update version, apply mask
		rv := binarray.Copy(arr)
		rv.PutInt(4, int32(hdr.CompilerVersion))
		compression.ApplyMask(rv, dataOffset)
		return rv, nil
	}

	// Prepare data for compression
	uncompressedSize := arr.Len() - dataOffset

	// Create buffer with extra space for compression overhead
	bufSize := uncompressedSize*9/8 + 9
	buffer := binarray.New(bufSize + 8) // +8 for size header

	// Copy uncompressed data (with 8-byte prepend for sizes)
	copy(buffer.Data[0:8], arr.Data[dataOffset-8:dataOffset])
	copy(buffer.Data[8:8+uncompressedSize], arr.Data[dataOffset:])

	// Apply per-game XOR keys before compression
	if len(keys) > 0 {
		dataBuf := binarray.FromBytes(buffer.Data[8 : 8+uncompressedSize])
		compression.ApplyXORKeys(dataBuf, keys)
	}

	// Compress
	compressed := compression.Compress(buffer.Data[8 : 8+uncompressedSize])
	compressedSize := len(compressed) + 8 // include size header

	// Build output
	rv := binarray.New(dataOffset + compressedSize)
	copy(rv.Data[:dataOffset], arr.Data[:dataOffset])

	// Write size header
	rv.PutInt(dataOffset, int32(compressedSize))
	rv.PutInt(dataOffset+4, int32(uncompressedSize))
	copy(rv.Data[dataOffset+8:], compressed)

	// Update file header
	rv.PutInt(0, 0x1d0)
	rv.PutInt(4, int32(hdr.CompilerVersion))
	rv.PutInt(0x28, int32(compressedSize))

	// Apply static XOR mask
	compression.ApplyMask(rv, dataOffset)

	return rv, nil
}

// Inflate decompresses raw LZ77 data and applies XOR keys.
// Lower-level than Decompress - operates on raw data without header parsing.
func Inflate(src, dst *binarray.Buffer, keys []gamedef.XORSubkey) error {
	if err := compression.Decompress(src.Data, dst.Data); err != nil {
		return err
	}
	if len(keys) > 0 {
		compression.ApplyXORKeys(dst, keys)
	}
	return nil
}

// Deflate compresses raw data and applies XOR keys.
// Returns the new end offset of compressed data within buf.
func Deflate(buf *binarray.Buffer, offset, length int, keys []gamedef.XORSubkey) int {
	// Apply XOR keys before compression
	dataBuf := binarray.FromBytes(buf.Data[offset : offset+length])
	if len(keys) > 0 {
		compression.ApplyXORKeys(dataBuf, keys)
	}

	compressed := compression.Compress(buf.Data[offset : offset+length])
	copy(buf.Data[offset:], compressed)
	return len(compressed) + offset
}
