// Package binarray provides binary buffer operations for reading and writing
// RealLive bytecode files. It replaces the OCaml Bigarray wrapper from the
// original RLdev codebase with idiomatic Go []byte operations.
//
// All integer operations use little-endian byte order, matching the RealLive
// engine's native format (x86 Windows).
package binarray

import (
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"os"
)

// Buffer wraps a byte slice with convenient read/write methods for
// little-endian integers and strings, mirroring OCaml's Binarray.t.
type Buffer struct {
	Data []byte
}

// New creates a zero-filled buffer of the given size.
func New(size int) *Buffer {
	return &Buffer{Data: make([]byte, size)}
}

// FromBytes wraps an existing byte slice (no copy).
func FromBytes(data []byte) *Buffer {
	return &Buffer{Data: data}
}

// Copy returns a deep copy of the buffer.
func Copy(b *Buffer) *Buffer {
	dst := make([]byte, len(b.Data))
	copy(dst, b.Data)
	return &Buffer{Data: dst}
}

// Len returns the buffer length (equivalent to OCaml's Binarray.dim).
func (b *Buffer) Len() int {
	return len(b.Data)
}

// Sub returns a sub-slice view (shared memory, like OCaml's Binarray.sub).
func (b *Buffer) Sub(offset, length int) *Buffer {
	return &Buffer{Data: b.Data[offset : offset+length]}
}

// SubCopy returns a copy of a sub-slice (independent memory).
func (b *Buffer) SubCopy(offset, length int) *Buffer {
	dst := make([]byte, length)
	copy(dst, b.Data[offset:offset+length])
	return &Buffer{Data: dst}
}

// Fill sets all bytes to the given value.
func (b *Buffer) Fill(val byte) {
	for i := range b.Data {
		b.Data[i] = val
	}
}

// Blit copies src into dst (like OCaml's Binarray.blit).
func Blit(src, dst *Buffer) {
	copy(dst.Data, src.Data)
}

// BlitRange copies length bytes from src[srcOff:] to dst[dstOff:].
func BlitRange(src *Buffer, srcOff int, dst *Buffer, dstOff int, length int) {
	copy(dst.Data[dstOff:dstOff+length], src.Data[srcOff:srcOff+length])
}

// --- Integer read/write (little-endian) ---

// GetU8 reads a single unsigned byte.
func (b *Buffer) GetU8(idx int) byte {
	return b.Data[idx]
}

// PutU8 writes a single byte.
func (b *Buffer) PutU8(idx int, val byte) {
	b.Data[idx] = val
}

// GetI16 reads a 16-bit unsigned integer (little-endian).
// Named to match OCaml's get_i16 which returns unsigned 16-bit.
func (b *Buffer) GetI16(idx int) uint16 {
	return binary.LittleEndian.Uint16(b.Data[idx:])
}

// PutI16 writes a 16-bit integer (little-endian).
func (b *Buffer) PutI16(idx int, val uint16) {
	binary.LittleEndian.PutUint16(b.Data[idx:], val)
}

// GetInt reads a 32-bit signed integer (little-endian).
// Named to match OCaml's get_int.
func (b *Buffer) GetInt(idx int) int32 {
	return int32(binary.LittleEndian.Uint32(b.Data[idx:]))
}

// GetUint reads a 32-bit unsigned integer (little-endian).
func (b *Buffer) GetUint(idx int) uint32 {
	return binary.LittleEndian.Uint32(b.Data[idx:])
}

// PutInt writes a 32-bit integer (little-endian).
func (b *Buffer) PutInt(idx int, val int32) {
	binary.LittleEndian.PutUint32(b.Data[idx:], uint32(val))
}

// PutUint writes a 32-bit unsigned integer (little-endian).
func (b *Buffer) PutUint(idx int, val uint32) {
	binary.LittleEndian.PutUint32(b.Data[idx:], val)
}

// GetInt32 reads a 32-bit value as int32 (alias for GetInt, matches OCaml name).
func (b *Buffer) GetInt32(idx int) int32 {
	return b.GetInt(idx)
}

// PutInt32 writes an int32 value (alias for PutInt, matches OCaml name).
func (b *Buffer) PutInt32(idx int, val int32) {
	b.PutInt(idx, val)
}

// --- String read/write ---

// Read returns a string of len bytes starting at idx.
// Equivalent to OCaml's Binarray.read.
func (b *Buffer) Read(idx, length int) string {
	return string(b.Data[idx : idx+length])
}

// ReadSz reads a null-terminated string from a fixed-length field.
// Equivalent to OCaml's Binarray.read_sz.
func (b *Buffer) ReadSz(idx, length int) string {
	data := b.Data[idx : idx+length]
	for i, c := range data {
		if c == 0 {
			return string(data[:i])
		}
	}
	return string(data)
}

// ReadCString reads a null-terminated string starting at idx, no length limit.
// Equivalent to OCaml's unsafe_read_sz. USE WITH CAUTION.
func (b *Buffer) ReadCString(idx int) string {
	end := idx
	for end < len(b.Data) && b.Data[end] != 0 {
		end++
	}
	return string(b.Data[idx:end])
}

// Write copies string data into the buffer at idx.
// Equivalent to OCaml's Binarray.write.
func (b *Buffer) Write(idx int, s string) {
	copy(b.Data[idx:], []byte(s))
}

// WriteBytes copies byte slice data into the buffer at idx.
func (b *Buffer) WriteBytes(idx int, data []byte) {
	copy(b.Data[idx:], data)
}

// WriteSz writes a string into a fixed-length field, padding with zeroes.
// Equivalent to OCaml's Binarray.write_sz.
func (b *Buffer) WriteSz(idx, length int, s string) {
	src := []byte(s)
	if len(src) > length {
		src = src[:length]
	}
	copy(b.Data[idx:idx+length], src)
	// Zero-pad remainder
	for i := len(src); i < length; i++ {
		b.Data[idx+i] = 0
	}
}

// --- File I/O ---

// ReadFile reads an entire file into a new Buffer.
// Equivalent to OCaml's Binarray.read_input.
func ReadFile(fname string) (*Buffer, error) {
	data, err := os.ReadFile(fname)
	if err != nil {
		return nil, fmt.Errorf("file '%s' not found: %w", fname, err)
	}
	return &Buffer{Data: data}, nil
}

// WriteFile writes the buffer contents to a file.
// Equivalent to OCaml's Binarray.write_file.
func (b *Buffer) WriteFile(fname string) error {
	return os.WriteFile(fname, b.Data, 0755)
}

// --- Hashing ---

// Digest returns the MD5 digest of the buffer contents.
// Equivalent to OCaml's Binarray.digest.
func (b *Buffer) Digest() [16]byte {
	return md5.Sum(b.Data)
}
