package datasset

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCGTableRoundTripPreservesRawData(t *testing.T) {
	raw := []byte("CG01\x00CG02\x00CG03\x00")
	doc := &Document{
		Type:   TypeCGTable,
		Count:  3,
		RawHex: hex.EncodeToString(raw),
	}

	bin, ext, err := EncodeDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	if ext != ".cgm" {
		t.Fatalf("extension = %q, want .cgm", ext)
	}

	decoded, err := decodeCGTable(bin)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Count != 3 {
		t.Fatalf("count = %d, want 3", decoded.Count)
	}
	if decoded.RawHex != doc.RawHex {
		t.Fatalf("raw_hex mismatch:\n got %s\nwant %s", decoded.RawHex, doc.RawHex)
	}
}

func TestCGTableEntriesRoundTrip(t *testing.T) {
	doc := &Document{
		Type: TypeCGTable,
		Entries: []CGTableEntry{
			{Index: 0, Name: "CG01"},
			{Index: 71, Name: "HCG11D"},
		},
	}

	bin, ext, err := EncodeDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	if ext != ".cgm" {
		t.Fatalf("extension = %q, want .cgm", ext)
	}

	decoded, err := decodeCGTable(bin)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Count != len(doc.Entries) {
		t.Fatalf("count = %d, want %d", decoded.Count, len(doc.Entries))
	}
	if len(decoded.Entries) != len(doc.Entries) {
		t.Fatalf("entries = %d, want %d", len(decoded.Entries), len(doc.Entries))
	}
	if decoded.Entries[1].Name != "HCG11D" || decoded.Entries[1].Index != 71 {
		t.Fatalf("entry[1] = %+v, want HCG11D/71", decoded.Entries[1])
	}
	rebuilt, _, err := EncodeDocument(decoded)
	if err != nil {
		t.Fatal(err)
	}
	redecoded, err := decodeCGTable(rebuilt)
	if err != nil {
		t.Fatal(err)
	}
	if redecoded.Entries[0].Name != "CG01" || redecoded.Entries[0].Index != 0 {
		t.Fatalf("entry[0] after rebuild = %+v", redecoded.Entries[0])
	}
}

func TestToneCurveRoundTripPreservesBinary(t *testing.T) {
	doc := &Document{
		Type: TypeToneCurve,
		Effects: []ToneCurveEffect{
			{
				Index: 0,
				Red:   ramp(0),
				Green: ramp(1),
				Blue:  ramp(2),
			},
			{
				Index: 1,
				Red:   ramp(255),
				Green: ramp(254),
				Blue:  ramp(253),
			},
		},
	}

	bin, ext, err := EncodeDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	if ext != ".tcc" {
		t.Fatalf("extension = %q, want .tcc", ext)
	}

	decoded, err := decodeTCC(bin)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, _, err := EncodeDocument(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bin, rebuilt) {
		t.Fatal("rebuilt TCC differs from original encoded data")
	}
}

func TestJSONFileHelpers(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "tcdata.json")
	tccPath := filepath.Join(dir, "tcdata.tcc")
	doc := &Document{
		Type: TypeToneCurve,
		Effects: []ToneCurveEffect{
			{
				Index: 0,
				Red:   ramp(0),
				Green: ramp(0),
				Blue:  ramp(0),
			},
		},
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jsonPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	ext, err := BinaryExtForJSONFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	if ext != ".tcc" {
		t.Fatalf("extension = %q, want .tcc", ext)
	}
	ext, err = WriteBinaryFromJSONFile(jsonPath, tccPath)
	if err != nil {
		t.Fatal(err)
	}
	if ext != ".tcc" {
		t.Fatalf("written extension = %q, want .tcc", ext)
	}
	if decoded, err := DecodeFile(tccPath); err != nil {
		t.Fatal(err)
	} else if decoded.EffectCount != 1 {
		t.Fatalf("effect_count = %d, want 1", decoded.EffectCount)
	}
}

func ramp(start int) []int {
	out := make([]int, 256)
	for i := range out {
		out[i] = (start + i) & 0xFF
	}
	return out
}
