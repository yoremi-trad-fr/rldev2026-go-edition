package bytecode

import (
	"encoding/binary"
	"testing"

	"github.com/yoremi/rldev-go/pkg/binarray"
)

func TestReadFileHeaderRawRealLiveVariantsAreCompressed(t *testing.T) {
	tests := []struct {
		name       string
		magic      string
		dataOffset int
	}{
		{name: "raw 0x1d0", magic: magicD001, dataOffset: 0x1d0},
		{name: "raw 0x1cc", magic: magicCC01, dataOffset: 0x1cc},
		{name: "raw 0x1b8", magic: magicB801, dataOffset: 0x1b8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const (
				uncompressedSize = 1234
				compressedSize   = 456
			)

			data := make([]byte, tt.dataOffset+compressedSize)
			copy(data, tt.magic)
			binary.LittleEndian.PutUint32(data[0x04:], 10002)
			binary.LittleEndian.PutUint32(data[0x20:], uint32(tt.dataOffset))
			binary.LittleEndian.PutUint32(data[0x24:], uncompressedSize)
			binary.LittleEndian.PutUint32(data[0x28:], compressedSize)

			hdr, err := ReadFileHeader(binarray.FromBytes(data), true)
			if err != nil {
				t.Fatalf("ReadFileHeader: %v", err)
			}
			if hdr.HeaderVersion != HeaderV2 {
				t.Fatalf("HeaderVersion = %d, want %d", hdr.HeaderVersion, HeaderV2)
			}
			if hdr.DataOffset != tt.dataOffset {
				t.Fatalf("DataOffset = %#x, want %#x", hdr.DataOffset, tt.dataOffset)
			}
			if !hdr.IsCompressed {
				t.Fatal("IsCompressed = false, want true")
			}
			if hdr.CompressedSize != compressedSize {
				t.Fatalf("CompressedSize = %d, want %d", hdr.CompressedSize, compressedSize)
			}
		})
	}
}

func TestReadFileHeaderCC01KeepsAVG2000Fallback(t *testing.T) {
	data := make([]byte, 0x200)
	copy(data, magicCC01)
	binary.LittleEndian.PutUint32(data[0x04:], 10002)
	binary.LittleEndian.PutUint32(data[0x20:], 2)
	binary.LittleEndian.PutUint32(data[0x24:], 99)
	binary.LittleEndian.PutUint32(data[0x28:], 7)

	hdr, err := ReadFileHeader(binarray.FromBytes(data), true)
	if err != nil {
		t.Fatalf("ReadFileHeader: %v", err)
	}
	if hdr.HeaderVersion != HeaderV1 {
		t.Fatalf("HeaderVersion = %d, want %d", hdr.HeaderVersion, HeaderV1)
	}
	if hdr.DataOffset != 0x1cc+8 {
		t.Fatalf("DataOffset = %#x, want %#x", hdr.DataOffset, 0x1cc+8)
	}
	if hdr.IsCompressed {
		t.Fatal("IsCompressed = true, want false")
	}
}
