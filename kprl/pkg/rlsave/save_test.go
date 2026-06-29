package rlsave

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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

func TestParseRawReadSave(t *testing.T) {
	raw := makeRawSave("CLANNAD", ContainerRaw, 64)
	put32(raw, 0x98+4, 3)

	save, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if save.Kind != KindRead {
		t.Fatalf("kind = %s, want %s", save.Kind, KindRead)
	}
	if save.Container != ContainerRaw {
		t.Fatalf("container = %s, want %s", save.Container, ContainerRaw)
	}
	if got, want := le32(save.Body, 4), uint32(3); got != want {
		t.Fatalf("body dword[1] = %d, want %d", got, want)
	}
	if got, want := mustReadProgress(t, save, 1), uint32(3); got != want {
		t.Fatalf("seen[1] = %d, want %d", got, want)
	}

	rewritten, err := save.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := le32(rewritten, 0), uint32(0x98); got != want {
		t.Fatalf("first dword = %d, want %d", got, want)
	}
}

func TestParseRawSizedSystemSave(t *testing.T) {
	raw := makeRawSave("AVG_SYSTEM_SAVE", ContainerRawSized, 96)

	save, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if save.Kind != KindSystem {
		t.Fatalf("kind = %s, want %s", save.Kind, KindSystem)
	}
	if save.Container != ContainerRawSized {
		t.Fatalf("container = %s, want %s", save.Container, ContainerRawSized)
	}

	rewritten, err := save.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := le32(rewritten, 0), uint32(len(rewritten)); got != want {
		t.Fatalf("first dword = %d, want file size %d", got, want)
	}
}

func TestExportImportRawReadDWordEdit(t *testing.T) {
	raw := makeRawSave("CLANNAD", ContainerRaw, 64)
	put32(raw, 0x98+4, 3)

	save, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := ExportText(&buf, save, ExportOptions{Lossless: true}); err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(buf.String(), "seen[1] = 3", "seen[1] = 0", 1)

	rebuilt, err := ImportText(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := le32(rebuilt.Body, 4), uint32(0); got != want {
		t.Fatalf("body dword[1] = %d, want %d", got, want)
	}
}

func TestDiffGlobalInts(t *testing.T) {
	before, err := Parse(makeGlobalSave(t, "AVG_GLOBAL_SAVE", map[int]int32{30: 1, 31: 1}))
	if err != nil {
		t.Fatal(err)
	}
	after, err := Parse(makeGlobalSave(t, "AVG_GLOBAL_SAVE", map[int]int32{30: 0, 31: 1}))
	if err != nil {
		t.Fatal(err)
	}

	diff, err := DiffSaves(before, after)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(diff.Changes), 1; got != want {
		t.Fatalf("changes = %d, want %d: %#v", got, want, diff.Changes)
	}
	change := diff.Changes[0]
	if change.Name != "intG[30]" || change.Old != 1 || change.New != 0 {
		t.Fatalf("unexpected change: %#v", change)
	}
}

func TestDiffRawReadProgress(t *testing.T) {
	rawBefore := makeRawSave("CLANNAD", ContainerRaw, 64)
	put32(rawBefore, 0x98+4, 3)
	rawAfter := makeRawSave("CLANNAD", ContainerRaw, 64)
	put32(rawAfter, 0x98+4, 4)

	before, err := Parse(rawBefore)
	if err != nil {
		t.Fatal(err)
	}
	after, err := Parse(rawAfter)
	if err != nil {
		t.Fatal(err)
	}
	diff, err := DiffSaves(before, after)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(diff.Changes), 1; got != want {
		t.Fatalf("changes = %d, want %d: %#v", got, want, diff.Changes)
	}
	change := diff.Changes[0]
	if change.Name != "seen[1]" || change.Old != 3 || change.New != 4 {
		t.Fatalf("unexpected change: %#v", change)
	}
}

func TestDiagnoseSaveReportsReadProgress(t *testing.T) {
	raw := makeRawSave("CLANNAD", ContainerRaw, 64)
	put32(raw, 0x98+4, 3)
	save, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}

	findings := DiagnoseSave(save)
	found := false
	for _, finding := range findings {
		if finding.Severity == SeverityInfo && strings.Contains(finding.Message, "highest script seen[1]=3") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("read progress finding missing: %#v", findings)
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

func makeRawSave(label string, container Container, bodySize int) []byte {
	headerLen := 0x98
	raw := make([]byte, headerLen+bodySize)
	switch container {
	case ContainerRawSized:
		put32(raw, 0, uint32(len(raw)))
	default:
		put32(raw, 0, uint32(headerLen))
	}
	put32(raw, 4, 10002)
	copy(raw[0x18:], []byte(label))
	if label != "" {
		raw[0x18+len(label)] = 0
	}
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

func mustReadProgress(t *testing.T, save *Save, seen int) uint32 {
	t.Helper()
	value, err := save.ReadProgress(seen)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
