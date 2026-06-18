package orgtext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportAndImportResourceWithPairedLiteral(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "SEEN0001.org")
	src := "{-# cp utf8 #-}\r\n\r\n#file 'SEEN0001.TXT'\r\n#resource 'SEEN0001.utf'\r\n\r\n#res<0000>\r\nstrS[1011] = 'こんにちは'\r\n"
	if err := os.WriteFile(srcPath, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(dir, "out")
	res, err := ExportFile(srcPath, outDir, "UTF-8")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Wrote || res.Entries != 1 || res.Resources != 1 {
		t.Fatalf("export stats = %+v", res)
	}
	utfPath := filepath.Join(outDir, "SEEN0001.utf")
	data, err := os.ReadFile(utfPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "<0000> こんにちは") || !strings.Contains(text, "pair_line=7") {
		t.Fatalf("unexpected export:\n%s", text)
	}

	translated := strings.ReplaceAll(text, "こんにちは", "Bonjour")
	if err := os.WriteFile(utfPath, []byte(translated), 0644); err != nil {
		t.Fatal(err)
	}
	importDir := filepath.Join(dir, "imported")
	res, err = ImportFile(srcPath, utfPath, importDir, "UTF-8")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Wrote || res.Resources != 1 || res.Literals != 1 {
		t.Fatalf("import stats = %+v", res)
	}
	patched, err := os.ReadFile(filepath.Join(importDir, "SEEN0001.org"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(patched), "strS[1011] = 'Bonjour'") {
		t.Fatalf("literal was not patched:\n%s", patched)
	}
	resFile, err := os.ReadFile(filepath.Join(importDir, "SEEN0001.utf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(resFile), "<0000> Bonjour") {
		t.Fatalf("resource was not patched:\n%s", resFile)
	}
}

func TestExportAndImportDirectLiteral(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "SEEN7820.org")
	src := "{-# cp utf8 #-}\n#file 'SEEN7820.TXT'\nstrS[1600 + intF[652]] = '朋也の攻撃力が上がった！'\nstrS[0] = 'NONE'\n"
	if err := os.WriteFile(srcPath, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(dir, "out")
	res, err := ExportFile(srcPath, outDir, "UTF-8")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Wrote || res.Entries != 1 || res.Literals != 1 {
		t.Fatalf("export stats = %+v", res)
	}
	utfPath := filepath.Join(outDir, "SEEN7820.utf")
	data, err := os.ReadFile(utfPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "<L0003_0> 朋也の攻撃力が上がった！") {
		t.Fatalf("unexpected export:\n%s", text)
	}

	translated := strings.ReplaceAll(text, "朋也の攻撃力が上がった！", "La force de Tomoya augmente !")
	if err := os.WriteFile(utfPath, []byte(translated), 0644); err != nil {
		t.Fatal(err)
	}
	importDir := filepath.Join(dir, "imported")
	if _, err := ImportFile(srcPath, utfPath, importDir, "UTF-8"); err != nil {
		t.Fatal(err)
	}
	patched, err := os.ReadFile(filepath.Join(importDir, "SEEN7820.org"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(patched), "strS[1600 + intF[652]] = 'La force de Tomoya augmente !'") {
		t.Fatalf("literal was not patched:\n%s", patched)
	}
	if strings.Contains(string(patched), "strS[0] = 'La force") {
		t.Fatalf("non-dialogue string was patched:\n%s", patched)
	}
}

func TestExportSkipsSourceWithoutText(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "EMPTY.org")
	src := "{-# cp utf8 #-}\n#file 'EMPTY.TXT'\nstrS[0] = 'NONE'\nintF[0] = 1\n"
	if err := os.WriteFile(srcPath, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	res, err := ExportFile(srcPath, filepath.Join(dir, "out"), "UTF-8")
	if err != nil {
		t.Fatal(err)
	}
	if res.Wrote || res.Entries != 0 {
		t.Fatalf("expected no output, got %+v", res)
	}
	if _, err := os.Stat(filepath.Join(dir, "out", "EMPTY.utf")); !os.IsNotExist(err) {
		t.Fatalf("unexpected utf file: %v", err)
	}
}

func TestExportSkipsMissingResourceRefsWithoutText(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "SEEN1002.org")
	src := "{-# cp utf8 #-}\n#file 'SEEN1002.TXT'\n#resource 'SEEN1002.utf'\n#res<0000>\n#res<0001>\n"
	if err := os.WriteFile(srcPath, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	res, err := ExportFile(srcPath, filepath.Join(dir, "out"), "UTF-8")
	if err != nil {
		t.Fatal(err)
	}
	if res.Wrote || res.Entries != 0 {
		t.Fatalf("expected no output, got %+v", res)
	}
}
