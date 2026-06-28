package rlsave

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yoremi/rldev-go/pkg/compression"
)

func TestParseGlobalSaveAndEditIntG(t *testing.T) {
	raw := makeGlobalSave(t, "AVG_GLOBAL_SAVE", map[int]int32{
		0:  3,
		30: 1,
		31: 1,
	})

	save, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if save.Kind != KindGlobal {
		t.Fatalf("kind = %s, want %s", save.Kind, KindGlobal)
	}
	if got, want := mustGlobalInt(t, save, 30), int32(1); got != want {
		t.Fatalf("intG[30] = %d, want %d", got, want)
	}

	if err := save.SetGlobalInt(30, 0); err != nil {
		t.Fatal(err)
	}
	if err := save.SetGlobalInt(0, 2); err != nil {
		t.Fatal(err)
	}
	rewritten, err := save.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	roundTrip, err := Parse(rewritten)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := mustGlobalInt(t, roundTrip, 0), int32(2); got != want {
		t.Fatalf("intG[0] = %d, want %d", got, want)
	}
	if got, want := mustGlobalInt(t, roundTrip, 30), int32(0); got != want {
		t.Fatalf("intG[30] = %d, want %d", got, want)
	}
	if got, want := mustGlobalInt(t, roundTrip, 31), int32(1); got != want {
		t.Fatalf("intG[31] = %d, want %d", got, want)
	}
}

func TestRejectIntGEditForRegularSlot(t *testing.T) {
	raw := makeGlobalSave(t, "CLANNAD", map[int]int32{30: 1})
	save, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if save.Kind != KindGame {
		t.Fatalf("kind = %s, want %s", save.Kind, KindGame)
	}
	if _, err := save.GlobalInt(30); err == nil {
		t.Fatal("GlobalInt on regular slot succeeded, want error")
	}
	if err := save.SetGlobalInt(30, 0); err == nil {
		t.Fatal("SetGlobalInt on regular slot succeeded, want error")
	}
}

func TestWriteFileCreatesBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "save999.sav")
	raw := makeGlobalSave(t, "AVG_GLOBAL_SAVE", map[int]int32{30: 1})
	if err := os.WriteFile(path, raw, 0666); err != nil {
		t.Fatal(err)
	}

	save, err := ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := save.SetGlobalInt(30, 0); err != nil {
		t.Fatal(err)
	}
	result, err := save.WriteFile(path, WriteOptions{Backup: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.BackupPath == "" {
		t.Fatal("backup path is empty")
	}
	if _, err := os.Stat(result.BackupPath); err != nil {
		t.Fatalf("backup was not written: %v", err)
	}

	roundTrip, err := ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := mustGlobalInt(t, roundTrip, 30), int32(0); got != want {
		t.Fatalf("intG[30] = %d, want %d", got, want)
	}
}

func makeGlobalSave(t *testing.T, label string, ints map[int]int32) []byte {
	t.Helper()
	body := make([]byte, 51296)
	for idx, value := range ints {
		put32(body, idx*4, uint32(value))
	}
	copy(body[16000:], []byte("Okazaki\x00Tomoya\x00"))

	payload := compression.Compress(body)
	headerLen := 0xa4
	compSize := len(payload) + 8
	raw := make([]byte, headerLen+compSize)
	put32(raw, 0, uint32(headerLen))
	copy(raw[0x18:], []byte(label))
	if label != "" {
		raw[0x18+len(label)] = 0
	}
	put32(raw, headerLen-12, uint32(headerLen))
	put32(raw, headerLen-8, uint32(len(body)))
	put32(raw, headerLen-4, uint32(compSize))
	put32(raw, headerLen, uint32(compSize))
	put32(raw, headerLen+4, uint32(len(body)))
	copy(raw[headerLen+8:], payload)
	return raw
}

func mustGlobalInt(t *testing.T, save *Save, index int) int32 {
	t.Helper()
	value, err := save.GlobalInt(index)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
