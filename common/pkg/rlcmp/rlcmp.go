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

	// Step 4: Apply per-game XOR keys if this was in an archive
	if hdr.Archived && len(keys) > 0 {
		dstBuf := binarray.FromBytes(rv.Data[hdr.DataOffset:])
		compression.ApplyXORKeys(dstBuf, keys)
	}

	return rv, nil
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
