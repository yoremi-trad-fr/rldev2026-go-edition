package binarray

import (
	"testing"
)

func TestIntReadWrite(t *testing.T) {
	buf := New(16)

	// Test 32-bit LE
	buf.PutInt(0, 0x12345678)
	if got := buf.GetInt(0); got != 0x12345678 {
		t.Errorf("GetInt = %08x, want %08x", got, 0x12345678)
	}
	// Verify LE byte order
	if buf.Data[0] != 0x78 || buf.Data[1] != 0x56 || buf.Data[2] != 0x34 || buf.Data[3] != 0x12 {
		t.Errorf("Not little-endian: %02x %02x %02x %02x", buf.Data[0], buf.Data[1], buf.Data[2], buf.Data[3])
	}

	// Test 16-bit LE
	buf.PutI16(8, 0xABCD)
	if got := buf.GetI16(8); got != 0xABCD {
		t.Errorf("GetI16 = %04x, want %04x", got, 0xABCD)
	}

	// Test negative int32
	buf.PutInt(4, -1)
	if got := buf.GetInt(4); got != -1 {
		t.Errorf("GetInt(-1) = %d, want -1", got)
	}
}

func TestStringReadWrite(t *testing.T) {
	buf := New(32)

	buf.Write(0, "KPRL")
	if got := buf.Read(0, 4); got != "KPRL" {
		t.Errorf("Read = %q, want %q", got, "KPRL")
	}

	buf.WriteSz(8, 10, "Hello")
	if got := buf.ReadSz(8, 10); got != "Hello" {
		t.Errorf("ReadSz = %q, want %q", got, "Hello")
	}
	// Check zero padding
	if buf.Data[13] != 0 || buf.Data[17] != 0 {
		t.Error("WriteSz should zero-pad")
	}
}

func TestReadCString(t *testing.T) {
	buf := New(16)
	buf.Write(0, "test\x00extra")
	if got := buf.ReadCString(0); got != "test" {
		t.Errorf("ReadCString = %q, want %q", got, "test")
	}
}

func TestSubAndCopy(t *testing.T) {
	buf := New(8)
	for i := 0; i < 8; i++ {
		buf.Data[i] = byte(i)
	}

	// Sub shares memory
	sub := buf.Sub(2, 4)
	sub.Data[0] = 0xFF
	if buf.Data[2] != 0xFF {
		t.Error("Sub should share memory")
	}

	// SubCopy is independent
	buf.Data[2] = 2 // restore
	sc := buf.SubCopy(2, 4)
	sc.Data[0] = 0xFF
	if buf.Data[2] != 2 {
		t.Error("SubCopy should be independent")
	}
}

func TestBlit(t *testing.T) {
	src := New(4)
	src.Write(0, "ABCD")
	dst := New(4)
	Blit(src, dst)
	if dst.Read(0, 4) != "ABCD" {
		t.Error("Blit failed")
	}
}
