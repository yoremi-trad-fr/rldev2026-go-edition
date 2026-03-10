// Package metadata handles RLdev-specific metadata stored in bytecode headers.
// Transposed from OCaml's metadata.ml.
//
// Metadata format:
//   int  metadata_len
//   int  id_len
//   char[id_len+1] compiler_identifier (null-terminated)
//   int  compiler_version * 100
//   4 bytes target_version (a.b.c.d)
//   byte text_transform: 0=none, 1=Chinese, 2=Western, 3=Korean
package metadata

import (
	"github.com/yoremi/rldev-go/pkg/binarray"
)

// TextTransform represents the text encoding transformation type.
type TextTransform int

const (
	TransformNone    TextTransform = 0
	TransformChinese TextTransform = 1
	TransformWestern TextTransform = 2
	TransformKorean  TextTransform = 3
)

func (t TextTransform) String() string {
	switch t {
	case TransformChinese:
		return "Chinese"
	case TransformWestern:
		return "Western"
	case TransformKorean:
		return "Korean"
	default:
		return "None"
	}
}

// Metadata holds RLdev compiler metadata embedded in a bytecode header.
type Metadata struct {
	CompilerName    string
	CompilerVersion int
	TargetVersion   [4]byte // a, b, c, d
	TextTransform   TextTransform
}

// Empty returns a zero-value metadata.
func Empty() Metadata {
	return Metadata{}
}

// Read parses metadata from the buffer at the given offset.
// Equivalent to OCaml's Metadata.read.
func Read(arr *binarray.Buffer, idx int) Metadata {
	if idx+8 > arr.Len() {
		return Empty()
	}

	metaLen := int(arr.GetInt(idx))
	idLen := int(arr.GetInt(idx+4)) + 1

	if metaLen < idLen+17 {
		return Empty()
	}

	idx2 := idx + 8 + idLen
	m := Metadata{
		CompilerName:    arr.ReadSz(idx+8, idLen),
		CompilerVersion: int(arr.GetInt(idx2)),
	}

	m.TargetVersion[0] = arr.GetU8(idx2 + 4)
	m.TargetVersion[1] = arr.GetU8(idx2 + 5)
	m.TargetVersion[2] = arr.GetU8(idx2 + 6)
	m.TargetVersion[3] = arr.GetU8(idx2 + 7)

	switch arr.GetU8(idx2 + 8) {
	case 1:
		m.TextTransform = TransformChinese
	case 2:
		m.TextTransform = TransformWestern
	case 3:
		m.TextTransform = TransformKorean
	default:
		m.TextTransform = TransformNone
	}

	return m
}

// ToBytes serializes metadata to bytes for embedding in a header.
// Equivalent to OCaml's Metadata.to_string.
func (m *Metadata) ToBytes(ident string, version float64, targetVersion [4]byte, transform TextTransform) []byte {
	identBytes := []byte(ident)
	identLen := len(identBytes)

	// Calculate total size
	totalLen := 4 + identLen + 1 + 4 + 4 + 1
	buf := binarray.New(totalLen + 4)

	buf.PutInt(0, int32(totalLen))
	buf.PutInt(4, int32(identLen))
	buf.Write(8, ident)
	buf.PutU8(8+identLen, 0) // null terminator

	verInt := int32(version * 100)
	buf.PutInt(8+identLen+1, verInt)

	off := 8 + identLen + 1 + 4
	buf.PutU8(off, targetVersion[0])
	buf.PutU8(off+1, targetVersion[1])
	buf.PutU8(off+2, targetVersion[2])
	buf.PutU8(off+3, targetVersion[3])
	buf.PutU8(off+4, byte(transform))

	return buf.Data
}
