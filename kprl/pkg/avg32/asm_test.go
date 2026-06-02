package avg32

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoremi/rldev-go/pkg/texttransforms"
)

func TestAssembleRawHexBlock(t *testing.T) {
	raw := []byte("TPC32\x00\x01\x02\xff")
	src := []byte(`#target AVG32
#rawhex begin
#rawhex 54 50 43 33 32 00 01 02
#rawhex FF
#rawhex end
`)
	got, err := Assemble(src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("assembled bytes = % X, want % X", got, raw)
	}
}

func TestRenderIncludesRawHexBlock(t *testing.T) {
	raw := []byte("TPC32\x00")
	src := Render(&Result{Raw: raw}, Options{})
	if !strings.Contains(src, "#rawhex begin") || !strings.Contains(src, "54 50 43 33 32 00") {
		t.Fatalf("rendered source lacks raw block:\n%s", src)
	}
	got, err := Assemble([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("assembled bytes = % X, want % X", got, raw)
	}
}

func TestAssemblePatchesTopLevelTextAndTargets(t *testing.T) {
	raw := minimalTPC32Scene([]byte{
		0x1c, 0x08, 0x00, 0x00, 0x00,
		0xff, 'A', 0x00,
		0x03,
	})
	result, err := Disassemble(raw, Options{})
	if err != nil {
		t.Fatal(err)
	}
	src := Render(result, Options{})
	src = strings.Replace(src, `op_FF("A")`, `op_FF("Longer")`, 1)

	got, err := Assemble([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	gotResult, err := Disassemble(got, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if gotResult.Instructions[1].Args[0] != `"Longer"` {
		t.Fatalf("patched text = %v", gotResult.Instructions[1].Args)
	}
	target := binary.LittleEndian.Uint32(got[result.Header.CodeOffset+1:])
	if target != 13 {
		t.Fatalf("jump target = %d, want 13", target)
	}
}

func TestAssembleAllowsTopLevelTextWidthSwitch(t *testing.T) {
	raw := minimalTPC32Scene([]byte{
		0xff, 'A', 0x00,
	})
	result, err := Disassemble(raw, Options{})
	if err != nil {
		t.Fatal(err)
	}
	src := Render(result, Options{})
	src = strings.Replace(src, `op_FF("A")`, `op_FE("A")`, 1)

	got, err := Assemble([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if got[result.Header.CodeOffset] != 0xfe {
		t.Fatalf("text opcode = 0x%02X, want 0xFE", got[result.Header.CodeOffset])
	}
	gotResult, err := Disassemble(got, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if gotResult.Instructions[0].Name != "op_FE" {
		t.Fatalf("instruction name = %s, want op_FE", gotResult.Instructions[0].Name)
	}
}

func TestAssemblePatchesSetTitle(t *testing.T) {
	raw := minimalTPC32Scene([]byte{
		0x60, 0x04, 0xff, 'A', 0x00, 0x00,
		0x03,
	})
	result, err := Disassemble(raw, Options{})
	if err != nil {
		t.Fatal(err)
	}
	src := Render(result, Options{})
	src = strings.Replace(src, `set_title(["A"])`, `set_title(["Long title"])`, 1)

	got, err := Assemble([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	gotResult, err := Disassemble(got, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if gotResult.Instructions[0].Args[0] != `["Long title"]` {
		t.Fatalf("patched title = %v", gotResult.Instructions[0].Args)
	}
}

func TestAssemblePatchesChoiceText(t *testing.T) {
	raw := minimalTPC32Scene([]byte{
		0x58, 0x01, 0x24, 0x06, 0x22, 0x00,
		0xff, 'O', 'n', 'e', 0x00, 0x00,
		0xff, 'T', 'w', 'o', 0x00, 0x00,
		0x23,
		0x03,
	})
	result, err := Disassemble(raw, Options{})
	if err != nil {
		t.Fatal(err)
	}
	src := Render(result, Options{})
	src = strings.Replace(src, `["One"]`, `["First option"]`, 1)
	src = strings.Replace(src, `["Two"]`, `["Second option"]`, 1)

	got, err := Assemble([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	gotResult, err := Disassemble(got, Options{})
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(gotResult.Instructions[0].Args, ", ")
	if !strings.Contains(args, `["First option"]`) || !strings.Contains(args, `["Second option"]`) {
		t.Fatalf("patched choice args = %v", gotResult.Instructions[0].Args)
	}
}

func TestRenderSeparatesAVG32Resources(t *testing.T) {
	raw := minimalTPC32Scene([]byte{
		0xff, 'A', 0x00,
		0x60, 0x04, 0xff, 'T', 0x00, 0x00,
		0x58, 0x01, 0x24, 0x06, 0x22, 0x00,
		0xff, 'O', 0x00, 0x00,
		0x23,
	})
	result, err := Disassemble(raw, Options{})
	if err != nil {
		t.Fatal(err)
	}
	src, resources := renderSource(result, Options{SeparateStrings: true}, "SEEN001.utf")
	if !strings.Contains(src, "#resource 'SEEN001.utf'") {
		t.Fatalf("source lacks resource directive:\n%s", src)
	}
	for _, want := range []string{"op_FF(#res<0000>)", "set_title([#res<0001>])", "[#res<0002>]"} {
		if !strings.Contains(src, want) {
			t.Fatalf("source lacks %q:\n%s", want, src)
		}
	}
	if !strings.Contains(renderResourceFile("SEEN001.TXT", resources), "\r\n<0000> A\r\n") {
		t.Fatalf("resource output does not use CRLF")
	}
	if len(resources) != 3 {
		t.Fatalf("resource count = %d, want 3", len(resources))
	}
}

func TestAssembleFileUsesAVG32ResourceEdits(t *testing.T) {
	raw := minimalTPC32Scene([]byte{
		0xff, 'A', 0x00,
		0x60, 0x04, 0xff, 'T', 0x00, 0x00,
	})
	result, err := Disassemble(raw, Options{})
	if err != nil {
		t.Fatal(err)
	}
	src, resources := renderSource(result, Options{SeparateStrings: true}, "SEEN001.utf")
	if len(resources) != 2 {
		t.Fatalf("resource count = %d, want 2", len(resources))
	}

	dir := t.TempDir()
	avgPath := filepath.Join(dir, "SEEN001.avg")
	if err := os.WriteFile(avgPath, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SEEN001.utf"), []byte("<0000> Dialogue modifie\n<0001> Titre modifie\n"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := AssembleFile(avgPath)
	if err != nil {
		t.Fatal(err)
	}
	gotResult, err := Disassemble(got, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if gotResult.Instructions[0].Args[0] != `"Dialogue modifie"` {
		t.Fatalf("patched dialogue = %v", gotResult.Instructions[0].Args)
	}
	if gotResult.Instructions[1].Args[0] != `["Titre modifie"]` {
		t.Fatalf("patched title = %v", gotResult.Instructions[1].Args)
	}
}

func TestAssembleFileUsesWesternTransformForAVG32Resources(t *testing.T) {
	raw := minimalTPC32Scene([]byte{
		0xff, 'A', 0x00,
		0x60, 0x04, 0xff, 'T', 0x00, 0x00,
	})
	result, err := Disassemble(raw, Options{})
	if err != nil {
		t.Fatal(err)
	}
	src, resources := renderSource(result, Options{SeparateStrings: true}, "SEEN050.utf")
	if len(resources) != 2 {
		t.Fatalf("resource count = %d, want 2", len(resources))
	}

	dir := t.TempDir()
	avgPath := filepath.Join(dir, "SEEN050.avg")
	if err := os.WriteFile(avgPath, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SEEN050.utf"), []byte("<0000> Deja vu, ca marche\n<0001> Déjà vu, ça marche\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err = AssembleFile(avgPath)
	if err == nil {
		t.Fatal("AssembleFile without transform accepted accented CP1252 text")
	}

	opts := Options{TextTransform: texttransforms.EncWestern}
	got, err := AssembleFileWithOptions(avgPath, opts)
	if err != nil {
		t.Fatal(err)
	}
	gotResult, err := Disassemble(got, opts)
	if err != nil {
		t.Fatal(err)
	}
	if gotResult.Instructions[0].Args[0] != `"Deja vu, ca marche"` {
		t.Fatalf("patched dialogue = %v", gotResult.Instructions[0].Args)
	}
	if gotResult.Instructions[1].Args[0] != `["Déjà vu, ça marche"]` {
		t.Fatalf("patched title = %v", gotResult.Instructions[1].Args)
	}
}

func minimalTPC32Scene(code []byte) []byte {
	const codeOffset = 0x59
	data := make([]byte, codeOffset, codeOffset+len(code)+1)
	copy(data, "TPC32")
	data = append(data, code...)
	data = append(data, 0x00)
	return data
}
