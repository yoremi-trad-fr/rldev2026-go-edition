package rlcmp

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/yoremi/rldev-go/pkg/binarray"
	"github.com/yoremi/rldev-go/pkg/compression"
	"github.com/yoremi/rldev-go/pkg/gamedef"
)

func TestDecompressSkipsXORWhenPlainPayloadLooksValid(t *testing.T) {
	plain := plausiblePayloadForTest()
	data := compressedRealLiveForTest(plain)

	out, err := Decompress(binarray.FromBytes(data), gamedef.KeyCFV, true)
	if err != nil {
		t.Fatal(err)
	}

	if got := payloadForTest(out); !bytes.Equal(got, plain) {
		t.Fatalf("plain archive payload was modified by CFV key")
	}
}

func TestDecompressAppliesXORWhenKeyedPayloadLooksValid(t *testing.T) {
	plain := plausiblePayloadForTest()
	encrypted := append([]byte(nil), plain...)
	compression.ApplyXORKeys(binarray.FromBytes(encrypted), gamedef.KeyCFV)
	data := compressedRealLiveForTest(encrypted)

	out, err := Decompress(binarray.FromBytes(data), gamedef.KeyCFV, true)
	if err != nil {
		t.Fatal(err)
	}

	if got := payloadForTest(out); !bytes.Equal(got, plain) {
		t.Fatalf("encrypted archive payload was not decoded with CFV key")
	}
}

func plausiblePayloadForTest() []byte {
	var p []byte
	for line := 1; len(p) < 700; line++ {
		p = append(p, 0x0a, byte(line), byte(line>>8))
		p = append(p, '#', 0, 1, 4, 0, 1, 0, 0)
		p = append(p, '(', '$', 0xff, byte(line), 0, 0, 0, ')')
		p = append(p, '@', byte(line), 0, 0, 0)
		p = append(p, '$', 0x05, '[', '$', 0xff, 0x64, 0, 0, 0, ']')
		p = append(p, '\\', 0x28, '$', 0xff, 0xff, 0xff, 0xff)
	}
	return p
}

func compressedRealLiveForTest(payload []byte) []byte {
	const dataOffset = 0x1d0
	compressed := compression.Compress(payload)
	compressedSize := len(compressed) + 8
	data := make([]byte, dataOffset+compressedSize)
	copy(data[0:], "KPRL")
	binary.LittleEndian.PutUint32(data[0x04:], 10002)
	binary.LittleEndian.PutUint32(data[0x08:], dataOffset)
	binary.LittleEndian.PutUint32(data[0x14:], dataOffset)
	binary.LittleEndian.PutUint32(data[0x20:], dataOffset)
	binary.LittleEndian.PutUint32(data[0x24:], uint32(len(payload)))
	binary.LittleEndian.PutUint32(data[0x28:], uint32(compressedSize))
	binary.LittleEndian.PutUint32(data[dataOffset:], uint32(compressedSize))
	binary.LittleEndian.PutUint32(data[dataOffset+4:], uint32(len(payload)))
	copy(data[dataOffset+8:], compressed)
	compression.ApplyMask(binarray.FromBytes(data), dataOffset)
	return data
}

func payloadForTest(buf *binarray.Buffer) []byte {
	off := int(binary.LittleEndian.Uint32(buf.Data[0x20:]))
	return buf.Data[off:]
}
