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

func TestAssemblePatchesTopLevelTextWithAIR2000ID(t *testing.T) {
	raw := minimalTPC32Scene([]byte{
		0xff, 0x07, 0x00, 0x00, 0x00, 'A', 0x00,
	})
	result, err := Disassemble(raw, Options{})
	if err != nil {
		t.Fatal(err)
	}
	src, resources := renderSource(result, Options{SeparateStrings: true}, "SEEN170.utf")
	if !strings.Contains(src, `op_FF(id:7, #res<0000>)`) {
		t.Fatalf("source does not externalize id text:\n%s", src)
	}
	if len(resources) != 1 || resources[0].Text != "A" {
		t.Fatalf("resources = %#v", resources)
	}

	dir := t.TempDir()
	avgPath := filepath.Join(dir, "SEEN170.avg")
	if err := os.WriteFile(avgPath, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SEEN170.utf"), []byte("<0000> Longer\n"), 0644); err != nil {
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
	if strings.Join(gotResult.Instructions[0].Args, ", ") != `id:7, "Longer"` {
		t.Fatalf("patched id text args = %v", gotResult.Instructions[0].Args)
	}
	if got[result.Header.CodeOffset+1] != 0x07 || got[result.Header.CodeOffset+2] != 0x00 {
		t.Fatalf("text id was not preserved: % X", got[result.Header.CodeOffset:result.Header.CodeOffset+6])
	}
}

func TestAssemblePatchesTopLevelTextWithLargeAIR2000ID(t *testing.T) {
	raw := minimalTPC32SceneWithLabelCount(300, []byte{
		0xff, 0x00, 0x01, 0x00, 0x00, 'A', 0x00,
	})
	result, err := Disassemble(raw, Options{})
	if err != nil {
		t.Fatal(err)
	}
	src, resources := renderSource(result, Options{SeparateStrings: true}, "SEEN700.utf")
	if !strings.Contains(src, `op_FF(id:256, #res<0000>)`) {
		t.Fatalf("source does not externalize large id text:\n%s", src)
	}
	if len(resources) != 1 || resources[0].Text != "A" {
		t.Fatalf("resources = %#v", resources)
	}

	dir := t.TempDir()
	avgPath := filepath.Join(dir, "SEEN700.avg")
	if err := os.WriteFile(avgPath, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SEEN700.utf"), []byte("<0000> Longer\n"), 0644); err != nil {
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
	if strings.Join(gotResult.Instructions[0].Args, ", ") != `id:256, "Longer"` {
		t.Fatalf("patched large id text args = %v", gotResult.Instructions[0].Args)
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

func TestDisassembleAIR2000OpaqueAVG32Commands(t *testing.T) {
	raw := minimalTPC32Scene([]byte{
		0x5b, 0x01, 0x22, 0x08, 0x10, 0x10, 0x00,
		0x5f, 0x20, 0x04, 0xa5, 0x06, 0x10, 0x26, 0x06, 0x27, 0x06, 0x28, 0x06, 0x29, 0x06,
		0x10, 0x22, 0x08,
		0x10, 0x29,
		0x03,
	})
	result, err := Disassemble(raw, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Instructions) != 5 {
		t.Fatalf("instruction count = %d, want 5", len(result.Instructions))
	}
	if result.Instructions[0].Name != "op_5b_01" || result.Instructions[0].Args[0] != "raw:22 08 10 10" {
		t.Fatalf("op_5b_01 = %s %v", result.Instructions[0].Name, result.Instructions[0].Args)
	}
	if result.Instructions[1].Name != "op_5F_20" || result.Instructions[1].Subcode != 0x20 || len(result.Instructions[1].Args) != 7 {
		t.Fatalf("op_5F_20 = %s sub=%d args=%v", result.Instructions[1].Name, result.Instructions[1].Subcode, result.Instructions[1].Args)
	}
	if result.Instructions[2].Name != "draw_raw_0x22" || result.Instructions[2].Args[0] != "raw:0x08" {
		t.Fatalf("draw_raw_0x22 = %s %v", result.Instructions[2].Name, result.Instructions[2].Args)
	}
	if result.Instructions[3].Name != "draw_raw_0x29" || len(result.Instructions[3].Args) != 0 {
		t.Fatalf("draw_raw_0x29 = %s %v", result.Instructions[3].Name, result.Instructions[3].Args)
	}
}

func TestAssemblePatchesFormattedTextWithAIR2000RawCommand(t *testing.T) {
	raw := minimalTPC32Scene([]byte{
		0x60, 0x04, 0x10, 0x22, 0x08, 0xff, 'A', 0x00, 0x00,
	})
	result, err := Disassemble(raw, Options{})
	if err != nil {
		t.Fatal(err)
	}
	src := Render(result, Options{})
	src = strings.Replace(src, `"A"`, `"Long"`, 1)

	got, err := Assemble([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	gotResult, err := Disassemble(got, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if gotResult.Instructions[0].Args[0] != `[cmd(raw:0x08) "Long"]` {
		t.Fatalf("patched formatted text = %v", gotResult.Instructions[0].Args)
	}
	wantPrefix := []byte{0x60, 0x04, 0x10, 0x22, 0x08, 0xff}
	if !bytes.Contains(got, wantPrefix) {
		t.Fatalf("assembled bytes lost AIR raw command: % X", got)
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
	return minimalTPC32SceneWithLabelCount(0, code)
}

func minimalTPC32SceneWithLabelCount(labelCount int, code []byte) []byte {
	const codeOffset = 0x59
	offset := codeOffset + labelCount*4
	data := make([]byte, offset, offset+len(code)+1)
	copy(data, "TPC32")
	binary.LittleEndian.PutUint32(data[5+0x13:], uint32(labelCount))
	data = append(data, code...)
	data = append(data, 0x00)
	return data
}
