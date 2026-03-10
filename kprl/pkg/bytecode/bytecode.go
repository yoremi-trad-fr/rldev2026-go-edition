// Package bytecode handles RealLive bytecode file headers.
// Transposed from OCaml's bytecode.ml.
//
// RealLive bytecode files (SEEN.xxx) have a structured header containing
// version info, entry points, compression metadata, character names, etc.
package bytecode

import (
	"fmt"

	"github.com/yoremi/rldev-go/pkg/binarray"
)

// HeaderVersion identifies the bytecode format generation.
const (
	HeaderV1 = 1 // AVG2000 format
	HeaderV2 = 2 // RealLive format (most common)
)

// Known header magic strings
var (
	magicKP2K = "KP2K" // AVG2000 compressed
	magicRD2K = "RD2K" // AVG2000 with RLdev version
	magicKPRL = "KPRL" // RealLive, compiler 10002
	magicRDRL = "RDRL" // RealLive with RLdev version
	magicKPRM = "KPRM" // RealLive, compiler 110002 (Little Busters!)
	magicRDRM = "RDRM" // RealLive with RLdev version, 110002

	// Binary magic values
	magicD001 = "\xd0\x01\x00\x00" // 0x1d0 LE
	magicCC01 = "\xcc\x01\x00\x00" // 0x1cc LE
	magicB801 = "\xb8\x01\x00\x00" // 0x1b8 LE
)

// FileHeader holds the parsed header of a RealLive bytecode file.
// Transposed from OCaml's file_header_t record.
type FileHeader struct {
	HeaderVersion    int
	CompilerVersion  int
	DataOffset       int
	UncompressedSize int
	CompressedSize   int  // -1 if not compressed
	IsCompressed     bool // true if CompressedSize is valid
	Int0x2C          int
	EntryPoints      [100]int32
	KidokuLnums      []int32
	DramatisPersonae []string // Character names
	Archived         bool     // Whether this was in a SEEN.TXT archive
}

// EmptyHeader returns a default empty header.
func EmptyHeader() FileHeader {
	return FileHeader{
		CompressedSize: -1,
	}
}

// IsBytecode checks if the data at the given index looks like a valid
// RealLive bytecode file header.
// Equivalent to OCaml's is_bytecode.
func IsBytecode(arr *binarray.Buffer, idx int) bool {
	if idx+8 > arr.Len() {
		return false
	}
	id := arr.Read(idx, 4)

	// Check for text identifiers
	switch id {
	case magicRDRL, magicRD2K, magicRDRM:
		return true
	case magicKPRL, magicKP2K, magicKPRM, magicD001, magicCC01, magicB801:
		// Check compiler version
		ver := int(arr.GetInt(idx + 4))
		return ver == 10002 || ver == 110002 || ver == 1110002
	}
	return false
}

// UncompressedHeader returns true if this header magic indicates no compression.
func UncompressedHeader(magic string) bool {
	switch magic {
	case magicKPRL, magicKP2K, magicKPRM, magicRDRL, magicRD2K, magicRDRM:
		return true
	}
	return false
}

// ReadFileHeader reads the basic file header from a bytecode file.
// Equivalent to OCaml's read_file_header.
func ReadFileHeader(arr *binarray.Buffer, archived bool) (FileHeader, error) {
	if !IsBytecode(arr, 0) {
		return FileHeader{}, fmt.Errorf("not a bytecode file")
	}

	hdr := EmptyHeader()
	hdr.Archived = archived

	// Determine compiler version
	magic := arr.Read(0, 4)
	if magic[:2] == "RD" {
		// RLdev-produced file: version is implicit from magic
		if magic[2:4] == "RM" {
			hdr.CompilerVersion = 110002
		} else {
			hdr.CompilerVersion = 10002
		}
	} else {
		hdr.CompilerVersion = int(arr.GetInt(4))
	}

	switch magic {
	case magicKP2K, magicRD2K, magicCC01:
		// AVG2000 / V1 format
		hdr.HeaderVersion = HeaderV1
		hdr.DataOffset = 0x1cc + int(arr.GetInt(0x20))*4
		hdr.UncompressedSize = int(arr.GetInt(0x24))
		hdr.Int0x2C = int(arr.GetInt(0x28))

	case magicKPRL, magicRDRL, magicKPRM, magicRDRM, magicD001:
		// RealLive / V2 format
		hdr.HeaderVersion = HeaderV2
		hdr.DataOffset = int(arr.GetInt(0x20))
		hdr.UncompressedSize = int(arr.GetInt(0x24))
		compSize := int(arr.GetInt(0x28))
		if compSize > 0 {
			hdr.CompressedSize = compSize
			hdr.IsCompressed = true
		}
		hdr.Int0x2C = int(arr.GetInt(0x2c))

	default:
		return FileHeader{}, fmt.Errorf("unsupported header format: %q", magic)
	}

	return hdr, nil
}

// ReadFullHeader reads the complete header including entry points, kidoku
// line numbers, and dramatis personae.
// Equivalent to OCaml's read_full_header.
func ReadFullHeader(arr *binarray.Buffer, archived bool) (FileHeader, error) {
	hdr, err := ReadFileHeader(arr, archived)
	if err != nil {
		return hdr, err
	}

	switch hdr.HeaderVersion {
	case HeaderV1:
		// Entry points at 0x30
		for i := 0; i < 100; i++ {
			hdr.EntryPoints[i] = arr.GetInt(0x30 + i*4)
		}
		// Kidoku line numbers at 0x1cc
		count := int(arr.GetInt(0x20))
		hdr.KidokuLnums = make([]int32, count)
		for i := 0; i < count; i++ {
			hdr.KidokuLnums[i] = arr.GetInt32(0x1cc + i*4)
		}

	case HeaderV2:
		// Entry points at 0x34
		for i := 0; i < 100; i++ {
			hdr.EntryPoints[i] = arr.GetInt(0x34 + i*4)
		}
		// Kidoku line numbers
		t1Offset := int(arr.GetInt(0x08))
		count := int(arr.GetInt(0x0c))
		hdr.KidokuLnums = make([]int32, count)
		for i := 0; i < count; i++ {
			hdr.KidokuLnums[i] = arr.GetInt32(t1Offset + i*4)
		}
		// Dramatis personae (character names)
		dpOffset := int(arr.GetInt(0x14))
		dpCount := int(arr.GetInt(0x18))
		hdr.DramatisPersonae = make([]string, dpCount)
		offset := dpOffset
		for i := 0; i < dpCount; i++ {
			nameLen := int(arr.GetInt(offset))
			hdr.DramatisPersonae[i] = arr.ReadSz(offset+4, nameLen)
			offset += 4 + nameLen
		}
	}

	return hdr, nil
}
