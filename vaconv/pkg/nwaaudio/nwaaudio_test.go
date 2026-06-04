package nwaaudio

import "testing"

func TestPCMToInt16From8BitUnsigned(t *testing.T) {
	got, err := pcmToInt16([]byte{0, 128, 255}, 8)
	if err != nil {
		t.Fatal(err)
	}
	want := []int16{-32768, 0, 32512}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sample %d: got %d, want %d", i, got[i], want[i])
		}
	}
}

func TestPCMToInt16From16BitLittleEndian(t *testing.T) {
	got, err := pcmToInt16([]byte{0x00, 0x80, 0x00, 0x00, 0xff, 0x7f}, 16)
	if err != nil {
		t.Fatal(err)
	}
	want := []int16{-32768, 0, 32767}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sample %d: got %d, want %d", i, got[i], want[i])
		}
	}
}

func TestPCMToInt16RejectsOdd16BitData(t *testing.T) {
	if _, err := pcmToInt16([]byte{0x00}, 16); err == nil {
		t.Fatal("expected error")
	}
}
