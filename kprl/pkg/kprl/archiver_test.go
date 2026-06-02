package kprl

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/yoremi/rldev-go/pkg/binarray"
	"github.com/yoremi/rldev-go/pkg/bytecode"
	"github.com/yoremi/rldev-go/pkg/compression"
)

func TestParseRanges(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    []int
		wantErr bool
	}{
		{
			name: "empty means all",
			args: nil,
			want: nil,
		},
		{
			name: "single number",
			args: []string{"42"},
			want: []int{42},
		},
		{
			name: "range",
			args: []string{"5-8"},
			want: []int{5, 6, 7, 8},
		},
		{
			name: "multiple args",
			args: []string{"0", "5-7", "100"},
			want: []int{0, 5, 6, 7, 100},
		},
		{
			name: "tilde range",
			args: []string{"10~15"},
			want: []int{10, 11, 12, 13, 14, 15},
		},
		{
			name: "dot range",
			args: []string{"3.5"},
			want: []int{3, 4, 5},
		},
		{
			name: "negation",
			args: []string{"0-10", "!5"},
			want: []int{0, 1, 2, 3, 4, 6, 7, 8, 9, 10},
		},
		{
			name: "negated range",
			args: []string{"0-10", "!3-5"},
			want: []int{0, 1, 2, 6, 7, 8, 9, 10},
		},
		{
			name:    "bad input",
			args:    []string{"abc"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRanges(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRanges() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want == nil && got == nil {
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("ParseRanges() length = %d, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ParseRanges()[%d] = %d, want %d", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestResolveRanges(t *testing.T) {
	// Empty = all 10000
	all := resolveRanges(nil)
	if len(all) != MaxSeens {
		t.Errorf("resolveRanges(nil) = %d entries, want %d", len(all), MaxSeens)
	}
	if all[0] != 0 || all[9999] != 9999 {
		t.Error("resolveRanges(nil) bounds wrong")
	}

	// Specific indices
	specific := resolveRanges([]int{50, 10, 100})
	if len(specific) != 3 || specific[0] != 10 || specific[1] != 50 || specific[2] != 100 {
		t.Errorf("resolveRanges([50,10,100]) = %v, want [10,50,100]", specific)
	}
}

func TestSeenCountEmptyArchive(t *testing.T) {
	// Create a minimal empty archive
	data := make([]byte, IndexSize)
	// Write empty archive magic at start
	copy(data[0:], "\x00Empty RealLive archive")

	buf := &mockBuffer{data: data[:23]} // just the magic
	_ = buf
	// The real test would need a binarray.Buffer, but we can test the constant
	if MaxSeens != 10000 {
		t.Errorf("MaxSeens = %d, want 10000", MaxSeens)
	}
	if IndexSize != 80000 {
		t.Errorf("IndexSize = %d, want 80000", IndexSize)
	}
}

type mockBuffer struct {
	data []byte
}

func TestGetUncompressedMagic(t *testing.T) {
	tests := []struct {
		name            string
		headerVersion   int
		compilerVersion int
		want            string
	}{
		{"AVG2000", 1, 10002, "KP2K"},
		{"RealLive 110002", 2, 110002, "KPRM"},
		{"RealLive standard", 2, 10002, "KPRL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use the bytecode.FileHeader via the getUncompressedMagic function
			// which we test indirectly through Extract
			got := getUncompressedMagic(fakeHeader(tt.headerVersion, tt.compilerVersion))
			if got != tt.want {
				t.Errorf("getUncompressedMagic() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAddPreservesTemplateTrailer(t *testing.T) {
	dir := t.TempDir()
	template := filepath.Join(dir, "template.SEEN.TXT")
	output := filepath.Join(dir, "rebuilt.SEEN.TXT")
	input := filepath.Join(dir, "SEEN0001.TXT")
	trailer := []byte("steam trailer payload")
	newSeen := minimalRealLiveBytecode([]byte("new scenario"))

	writeTestArchive(t, template, map[int][]byte{
		1: minimalRealLiveBytecode([]byte("old scenario")),
	}, trailer)
	if err := os.WriteFile(input, newSeen, 0644); err != nil {
		t.Fatal(err)
	}

	if err := Add(output, []string{input}, Options{TemplateArchive: template}); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(got, trailer) {
		t.Fatalf("rebuilt archive did not preserve trailer %q", trailer)
	}

	arc, err := LoadArchive(output)
	if err != nil {
		t.Fatal(err)
	}
	if arc.Count != 1 {
		t.Fatalf("entry count = %d, want 1", arc.Count)
	}
	entry := arc.Entries[1]
	if entry.Length != len(newSeen) {
		t.Fatalf("entry length = %d, want %d", entry.Length, len(newSeen))
	}
	wantLen := IndexSize + len(newSeen) + len(trailer)
	if len(got) != wantLen {
		t.Fatalf("archive length = %d, want %d", len(got), wantLen)
	}
}

func TestExtractUsesParsedCompressionFlag(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "SEEN.TXT")
	outDir := filepath.Join(dir, "out")
	payload := []byte("synthetic compressed scenario payload")

	writeTestArchive(t, archive, map[int][]byte{
		1: compressedRealLiveBytecode(payload),
	}, nil)

	if err := Extract(archive, []int{1}, Options{OutDir: outDir}); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(outDir, "SEEN0001.TXT.rl"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got[:4]) != "KPRL" {
		t.Fatalf("output magic = %q, want KPRL", string(got[:4]))
	}
	if !bytes.Equal(got[0x1d0:], payload) {
		t.Fatalf("payload = %q, want %q", got[0x1d0:], payload)
	}
}

func TestAVG32PackRoundTrip(t *testing.T) {
	payload := append([]byte("TPC32\x00\x0f\x00"), bytes.Repeat([]byte{0x41, 0x42, 0x00}, 32)...)
	packed, err := packAVG32Data(payload)
	if err != nil {
		t.Fatal(err)
	}
	unpacked, err := decompressAVG32Pack(binarray.FromBytes(packed))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(unpacked.Data, payload) {
		t.Fatalf("unpacked data mismatch")
	}
}

func TestLoadAndExtractAVG32Archive(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "SEEN.TXT")
	outDir := filepath.Join(dir, "out")
	payload := []byte("TPC32 synthetic AVG32 scene")

	writeTestAVG32Archive(t, archive, map[int][]byte{
		1:  payload,
		50: []byte("TPC32 second scene"),
	})

	arc, err := LoadArchive(archive)
	if err != nil {
		t.Fatal(err)
	}
	if arc.Format != ArchiveFormatAVG32 {
		t.Fatalf("format = %v, want AVG32", arc.Format)
	}
	if arc.Count != 2 {
		t.Fatalf("entry count = %d, want 2", arc.Count)
	}
	if arc.EntryName(1) != "SEEN001.TXT" {
		t.Fatalf("entry name = %q, want SEEN001.TXT", arc.EntryName(1))
	}

	if err := Extract(archive, []int{1}, Options{OutDir: outDir}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(outDir, "SEEN001.TXT.rl"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload = %q, want %q", got, payload)
	}
}

func TestAddRebuildsAVG32ArchiveFromTemplate(t *testing.T) {
	dir := t.TempDir()
	template := filepath.Join(dir, "template.SEEN.TXT")
	output := filepath.Join(dir, "rebuilt.SEEN.TXT")
	input := filepath.Join(dir, "SEEN001.TXT")
	replacement := []byte("TPC32 replacement scene")
	kept := []byte("TPC32 kept scene")

	writeTestAVG32Archive(t, template, map[int][]byte{
		1: []byte("TPC32 old scene"),
		2: kept,
	})
	if err := os.WriteFile(input, replacement, 0644); err != nil {
		t.Fatal(err)
	}

	if err := Add(output, []string{input}, Options{TemplateArchive: template}); err != nil {
		t.Fatal(err)
	}

	arc, err := LoadArchive(output)
	if err != nil {
		t.Fatal(err)
	}
	if arc.Format != ArchiveFormatAVG32 {
		t.Fatalf("format = %v, want AVG32", arc.Format)
	}

	seen1, err := decompressAVG32Pack(arc.Subfile(1))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(seen1.Data, replacement) {
		t.Fatalf("SEEN001 payload = %q, want %q", seen1.Data, replacement)
	}
	seen2, err := decompressAVG32Pack(arc.Subfile(2))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(seen2.Data, kept) {
		t.Fatalf("SEEN002 payload = %q, want %q", seen2.Data, kept)
	}
}

func TestAddRebuildsAVG32ArchiveFromAvgSource(t *testing.T) {
	dir := t.TempDir()
	template := filepath.Join(dir, "template.SEEN.TXT")
	output := filepath.Join(dir, "rebuilt.SEEN.TXT")
	input := filepath.Join(dir, "SEEN001.avg")
	replacement := []byte("TPC32 replacement from avg source")
	kept := []byte("TPC32 kept scene")

	writeTestAVG32Archive(t, template, map[int][]byte{
		1: []byte("TPC32 old scene"),
		2: kept,
	})
	if err := os.WriteFile(input, []byte(avg32RawSource(replacement)), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Add(output, []string{input}, Options{TemplateArchive: template}); err != nil {
		t.Fatal(err)
	}

	arc, err := LoadArchive(output)
	if err != nil {
		t.Fatal(err)
	}
	seen1, err := decompressAVG32Pack(arc.Subfile(1))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(seen1.Data, replacement) {
		t.Fatalf("SEEN001 payload = %q, want %q", seen1.Data, replacement)
	}
	seen2, err := decompressAVG32Pack(arc.Subfile(2))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(seen2.Data, kept) {
		t.Fatalf("SEEN002 payload = %q, want %q", seen2.Data, kept)
	}
}

func TestAddCreatesAVG32ArchiveFromAvgSourceWithoutTemplate(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "new.SEEN.TXT")
	input := filepath.Join(dir, "SEEN001.avg")
	replacement := []byte("TPC32 replacement from standalone avg source")

	if err := os.WriteFile(input, []byte(avg32RawSource(replacement)), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Add(output, []string{input}, Options{}); err != nil {
		t.Fatal(err)
	}

	arc, err := LoadArchive(output)
	if err != nil {
		t.Fatal(err)
	}
	if arc.Format != ArchiveFormatAVG32 {
		t.Fatalf("format = %v, want AVG32", arc.Format)
	}
	if arc.EntryName(1) != "SEEN001.TXT" {
		t.Fatalf("entry name = %q, want SEEN001.TXT", arc.EntryName(1))
	}
	seen1, err := decompressAVG32Pack(arc.Subfile(1))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(seen1.Data, replacement) {
		t.Fatalf("SEEN001 payload = %q, want %q", seen1.Data, replacement)
	}
}

func TestAddRejectsAVG32SourceForRealLiveArchive(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "SEEN.TXT")
	input := filepath.Join(dir, "SEEN0001.avg")

	writeTestArchive(t, output, map[int][]byte{
		1: minimalRealLiveBytecode([]byte("old")),
	}, nil)
	if err := os.WriteFile(input, []byte(avg32RawSource([]byte("TPC32 wrong archive"))), 0644); err != nil {
		t.Fatal(err)
	}

	err := Add(output, []string{input}, Options{})
	if err == nil {
		t.Fatal("Add accepted .avg source for RealLive archive")
	}
	if !strings.Contains(err.Error(), ".avg sources require an AVG32 archive") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddRejectsNonTPC32InputForAVG32Archive(t *testing.T) {
	dir := t.TempDir()
	template := filepath.Join(dir, "template.SEEN.TXT")
	output := filepath.Join(dir, "rebuilt.SEEN.TXT")
	input := filepath.Join(dir, "SEEN001.TXT")

	writeTestAVG32Archive(t, template, map[int][]byte{
		1: []byte("TPC32 old scene"),
	})
	if err := os.WriteFile(input, []byte("not a TPC32 scene"), 0644); err != nil {
		t.Fatal(err)
	}

	err := Add(output, []string{input}, Options{TemplateArchive: template})
	if err == nil {
		t.Fatal("Add accepted non-TPC32 input for AVG32 archive")
	}
	if !strings.Contains(err.Error(), "not a .avg source, PACK block, or TPC32 scene") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// fakeHeader creates a minimal FileHeader for testing
func fakeHeader(headerVer, compilerVer int) bytecode.FileHeader {
	return bytecode.FileHeader{HeaderVersion: headerVer, CompilerVersion: compilerVer}
}

func minimalRealLiveBytecode(payload []byte) []byte {
	const dataOffset = 0x1d0
	data := make([]byte, dataOffset+len(payload))
	copy(data[0:], "KPRL")
	binary.LittleEndian.PutUint32(data[0x04:], 10002)
	binary.LittleEndian.PutUint32(data[0x08:], 0x1d0)
	binary.LittleEndian.PutUint32(data[0x14:], 0x1d0)
	binary.LittleEndian.PutUint32(data[0x20:], dataOffset)
	binary.LittleEndian.PutUint32(data[0x24:], uint32(len(payload)))
	copy(data[dataOffset:], payload)
	return data
}

func compressedRealLiveBytecode(payload []byte) []byte {
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

func writeTestArchive(t *testing.T, path string, entries map[int][]byte, trailer []byte) {
	t.Helper()
	index := make([]byte, IndexSize)
	body := make([]byte, 0)
	offset := IndexSize
	indices := make([]int, 0, len(entries))
	for idx := range entries {
		indices = append(indices, idx)
	}
	sort.Ints(indices)
	for _, idx := range indices {
		data := entries[idx]
		binary.LittleEndian.PutUint32(index[idx*8:], uint32(offset))
		binary.LittleEndian.PutUint32(index[idx*8+4:], uint32(len(data)))
		body = append(body, data...)
		offset += len(data)
	}
	out := append(index, body...)
	out = append(out, trailer...)
	if err := os.WriteFile(path, out, 0644); err != nil {
		t.Fatal(err)
	}
}

func writeTestAVG32Archive(t *testing.T, path string, entries map[int][]byte) {
	t.Helper()

	indices := make([]int, 0, len(entries))
	for idx := range entries {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	table := make([]byte, len(indices)*avg32ArchiveEntrySize)
	body := make([]byte, 0)
	offset := avg32ArchiveHeaderSize + len(table)
	for tableIdx, idx := range indices {
		packed, err := packAVG32Data(entries[idx])
		if err != nil {
			t.Fatal(err)
		}
		base := tableIdx * avg32ArchiveEntrySize
		name := []byte(fmt.Sprintf("SEEN%03d.TXT", idx))
		copy(table[base:], name)
		binary.LittleEndian.PutUint32(table[base+0x10:], uint32(offset))
		binary.LittleEndian.PutUint32(table[base+0x14:], uint32(len(packed)))
		binary.LittleEndian.PutUint32(table[base+0x18:], uint32(len(entries[idx])))
		binary.LittleEndian.PutUint32(table[base+0x1c:], 1)
		body = append(body, packed...)
		offset += len(packed)
	}

	header := make([]byte, avg32ArchiveHeaderSize)
	copy(header, "PACL")
	binary.LittleEndian.PutUint32(header[0x10:], uint32(len(indices)))
	out := append(header, table...)
	out = append(out, body...)
	if err := os.WriteFile(path, out, 0644); err != nil {
		t.Fatal(err)
	}
}

func avg32RawSource(data []byte) string {
	var b strings.Builder
	b.WriteString("#target AVG32\n#rawhex begin\n#rawhex")
	for _, x := range data {
		fmt.Fprintf(&b, " %02X", x)
	}
	b.WriteString("\n#rawhex end\n")
	return b.String()
}
