package ini

import (
	"fmt"
	"strings"
	"testing"
)

func parseStr(s string) *Table {
	t, err := Parse(strings.NewReader(s))
	if err != nil {
		panic(err)
	}
	return t
}

// ============================================================
// Table operations
// ============================================================

func TestTableSetGet(t *testing.T) {
	tbl := NewTable()
	tbl.Set("KEY", []Value{{Kind: VInteger, Int: 42}})
	v := tbl.Find("KEY")
	if len(v) != 1 || v[0].Int != 42 { t.Errorf("got %v", v) }
}

func TestTableCaseInsensitive(t *testing.T) {
	tbl := NewTable()
	tbl.Set("MyKey", []Value{{Kind: VInteger, Int: 1}})
	if !tbl.Exists("mykey") { t.Error("case insensitive lookup failed") }
	if !tbl.Exists("MYKEY") { t.Error("case insensitive lookup failed") }
}

func TestTableUnset(t *testing.T) {
	tbl := NewTable()
	tbl.Set("X", []Value{{Kind: VInteger, Int: 1}})
	tbl.Unset("X")
	if tbl.Exists("X") { t.Error("should be unset") }
}

func TestTableSetInt(t *testing.T) {
	tbl := NewTable()
	tbl.SetInt("SIZE", 640)
	if tbl.GetInt("SIZE", 0) != 640 { t.Error("SetInt/GetInt") }
}

func TestTableGetIntDefault(t *testing.T) {
	tbl := NewTable()
	if tbl.GetInt("NOPE", 99) != 99 { t.Error("default") }
}

func TestTableGetPair(t *testing.T) {
	tbl := NewTable()
	tbl.Set("RES", []Value{{Kind: VInteger, Int: 800}, {Kind: VInteger, Int: 600}})
	p := tbl.GetPair("RES", [2]int{0, 0})
	if p[0] != 800 || p[1] != 600 { t.Errorf("got %v", p) }
}

func TestTableGetPairDefault(t *testing.T) {
	tbl := NewTable()
	p := tbl.GetPair("NOPE", [2]int{1, 2})
	if p[0] != 1 || p[1] != 2 { t.Error("default") }
}

func TestTableCount(t *testing.T) {
	tbl := NewTable()
	tbl.Set("A", nil)
	tbl.Set("B", nil)
	if tbl.Count() != 2 { t.Errorf("count: %d", tbl.Count()) }
}

// ============================================================
// Basic parsing
// ============================================================

func TestParseSimple(t *testing.T) {
	tbl := parseStr("#WINDOW_ATTR = 128, 128, 190")
	v := tbl.Find("WINDOW_ATTR")
	if v == nil { t.Fatal("WINDOW_ATTR not found") }
	if len(v) != 3 { t.Fatalf("values: %d", len(v)) }
	if v[0].Int != 128 || v[2].Int != 190 { t.Errorf("values: %v", v) }
}

func TestParseString(t *testing.T) {
	tbl := parseStr(`#REGNAME = "VisualArt's" , "KEY"`)
	v := tbl.Find("REGNAME")
	if len(v) != 2 { t.Fatalf("values: %d", len(v)) }
	if v[0].Str != "VisualArt's" { t.Errorf("got %q", v[0].Str) }
}

func TestParseEnabled(t *testing.T) {
	tbl := parseStr("#SCREENMODE = U")
	v := tbl.Find("SCREENMODE")
	if len(v) != 1 || !v[0].Bool { t.Error("expected U=true") }
}

func TestParseDisabled(t *testing.T) {
	tbl := parseStr("#SCREENMODE = N")
	v := tbl.Find("SCREENMODE")
	if len(v) != 1 || v[0].Bool { t.Error("expected N=false") }
}

func TestParseNoValue(t *testing.T) {
	tbl := parseStr("#EMPTY_KEY =\n#NEXT = 1")
	// EMPTY_KEY should exist with empty value list
	if !tbl.Exists("EMPTY_KEY") { t.Error("EMPTY_KEY should exist") }
	v := tbl.Find("EMPTY_KEY")
	if len(v) != 0 { t.Errorf("expected empty values, got %d", len(v)) }
}

func TestParseNegative(t *testing.T) {
	tbl := parseStr("#POS = -100, -200")
	v := tbl.Find("POS")
	if len(v) != 2 { t.Fatalf("values: %d", len(v)) }
	if v[0].Int != -100 { t.Errorf("v[0]: %d", v[0].Int) }
	if v[1].Int != -200 { t.Errorf("v[1]: %d", v[1].Int) }
}

func TestParseMultiple(t *testing.T) {
	tbl := parseStr("#A = 1\n#B = 2\n#C = 3")
	if tbl.Count() != 3 { t.Errorf("count: %d", tbl.Count()) }
	if tbl.GetInt("A", 0) != 1 { t.Error("A") }
	if tbl.GetInt("B", 0) != 2 { t.Error("B") }
	if tbl.GetInt("C", 0) != 3 { t.Error("C") }
}

func TestParseComment(t *testing.T) {
	tbl := parseStr("; this is a comment\n#KEY = 42 ; inline comment")
	if tbl.GetInt("KEY", 0) != 42 { t.Error("comment handling") }
}

// ============================================================
// Dotted key patterns
// ============================================================

func TestParseDotInt(t *testing.T) {
	// #IDENT.NNN = value
	tbl := parseStr("#FONT.000 = 26")
	if tbl.GetInt("FONT.000", 0) != 26 { t.Error("FONT.000") }
}

func TestParseDotIdent(t *testing.T) {
	// #IDENT.TEXT = value
	tbl := parseStr("#WINDOW.SIZE = 640")
	if tbl.GetInt("WINDOW.SIZE", 0) != 640 { t.Error("WINDOW.SIZE") }
}

func TestParseDotIntDotInt(t *testing.T) {
	// #IDENT.NNN.NNN = value
	tbl := parseStr("#SEL.000.001 = 128")
	if tbl.GetInt("SEL.000.001", 0) != 128 { t.Error("SEL.000.001") }
}

func TestParseDotIntDotIdent(t *testing.T) {
	// #IDENT.NNN.TEXT = value
	tbl := parseStr("#WAKU.000.NAME = \"frame\"")
	v := tbl.Find("WAKU.000.NAME")
	if v == nil || v[0].Str != "frame" { t.Error("WAKU.000.NAME") }
}

func TestParseDotIntDotIntDotIdent(t *testing.T) {
	// #IDENT.NNN.NNN.TEXT = value
	tbl := parseStr("#WAKU.020.000.FILENAME = \"s_ped\"")
	v := tbl.Find("WAKU.020.000.FILENAME")
	if v == nil || v[0].Str != "s_ped" { t.Error("WAKU.020.000.FILENAME") }
}

func TestParseDotIntDotIdentDotIdent(t *testing.T) {
	// #IDENT.NNN.TEXT.TEXT = value
	tbl := parseStr("#ITEM.000.BASE.FILENAME = \"btn\"")
	v := tbl.Find("ITEM.000.BASE.FILENAME")
	if v == nil || v[0].Str != "btn" { t.Error("ITEM.000.BASE.FILENAME") }
}

func TestParseDotIntDotIdentDotIdentDotIdent(t *testing.T) {
	// #IDENT.NNN.TEXT.TEXT.TEXT = value  (rldev2026 fix: Clannad Side Stories)
	tbl := parseStr("#FULLSCREEN_MSGBK.000.ITEM.SLIDER_BASE.FILENAME = \"slider\"")
	v := tbl.Find("FULLSCREEN_MSGBK.000.ITEM.SLIDER_BASE.FILENAME")
	if v == nil || v[0].Str != "slider" { t.Error("5-level key (Clannad)") }
}

// rldev2026.2 fix: IDENT.NNN.NNN.TEXT.NNN pattern (Tomoyo After Steam)
func TestParseDotIntDotIntDotIdentDotInt(t *testing.T) {
	tbl := parseStr("#WAKU.020.000.EXBTN_000_BTN.000 = \"s_ped_mw00c\"")
	v := tbl.Find("WAKU.020.000.EXBTN_000_BTN.000")
	if v == nil { t.Fatal("key not found") }
	if v[0].Str != "s_ped_mw00c" { t.Errorf("got %q", v[0].Str) }
}

func TestParseDotIdentDotIntDotIdent(t *testing.T) {
	// #IDENT.TEXT.NNN.TEXT = value
	tbl := parseStr("#SECTION.ITEM.003.NAME = \"test\"")
	v := tbl.Find("SECTION.ITEM.003.NAME")
	if v == nil || v[0].Str != "test" { t.Error("SECTION.ITEM.003.NAME") }
}

func TestParseDotIntDotIdentDotInt(t *testing.T) {
	// #IDENT.NNN.TEXT.NNN = value
	tbl := parseStr("#DATA.010.ENTRY.005 = 42")
	if tbl.GetInt("DATA.010.ENTRY.005", 0) != 42 { t.Error("DATA.010.ENTRY.005") }
}

// ============================================================
// Range definitions
// ============================================================

func TestParseRange(t *testing.T) {
	// #IDENT.NNN:INT = values (range assignment)
	tbl := parseStr("#SEL.000:003 = 128, 0")
	for i := 0; i <= 3; i++ {
		key := fmt.Sprintf("SEL.%03d", i)
		v := tbl.Find(key)
		if v == nil { t.Errorf("%s not found", key); continue }
		if len(v) != 2 || v[0].Int != 128 { t.Errorf("%s: %v", key, v) }
	}
}

func TestParseShake(t *testing.T) {
	tbl := parseStr("#SHAKE.001 = (100, 200, 300)")
	v := tbl.Find("SHAKE.001")
	if v == nil { t.Fatal("SHAKE.001 not found") }
	if len(v) != 1 || v[0].Kind != VRange { t.Fatalf("expected range: %v", v) }
	if len(v[0].Ints) != 3 { t.Errorf("range ints: %d", len(v[0].Ints)) }
	if v[0].Ints[0] != 100 || v[0].Ints[2] != 300 { t.Errorf("values: %v", v[0].Ints) }
}

// ============================================================
// Colon-separated values (treated like commas)
// ============================================================

func TestParseColonSeparated(t *testing.T) {
	tbl := parseStr("#FONT_SIZE = 20:30:40")
	v := tbl.Find("FONT_SIZE")
	if len(v) != 3 { t.Fatalf("values: %d", len(v)) }
}

// ============================================================
// Real-world GAMEEXE.INI fragments
// ============================================================

func TestParseRealWorld(t *testing.T) {
	src := `
; Clannad Side Stories sample
#CAPTION = "CLANNAD Side Stories"
#SCREENSIZE_MOD = 0
#WINDOW_ATTR = 128, 128, 190, -1, -1, -1
#FONT_SIZE = 26
#INIT_FRAME_MOD = 1
#WAKU.000.NAME = "waku"
#WAKU.000.FILENAME = "waku_bg0"
#WAKU.000.000.FILENAME = "waku_btn0"
#WAKU.000.000.EXBTN_000_BTN.000 = "waku_ex0"
#SEL.000 = 0, 0, 639, 479
`
	tbl := parseStr(src)
	if tbl.GetInt("SCREENSIZE_MOD", -1) != 0 { t.Error("SCREENSIZE_MOD") }
	if tbl.GetInt("FONT_SIZE", 0) != 26 { t.Error("FONT_SIZE") }
	v := tbl.Find("CAPTION")
	if v == nil || v[0].Str != "CLANNAD Side Stories" { t.Error("CAPTION") }

	// 5-level key
	v = tbl.Find("WAKU.000.000.EXBTN_000_BTN.000")
	if v == nil || v[0].Str != "waku_ex0" { t.Error("5-level key") }
}

func TestParseTomoyo(t *testing.T) {
	// Tomoyo After Steam format that triggered rldev2026.2 fix
	src := `
#WAKU.020.000.EXBTN_000_BTN.000 = "s_ped_mw00c"
#WAKU.020.000.EXBTN_000_BTN.001 = "s_ped_mw00c2"
#WAKU.020.001.EXBTN_000_BTN.000 = "s_ped_mw01a"
`
	tbl := parseStr(src)
	keys := []string{
		"WAKU.020.000.EXBTN_000_BTN.000",
		"WAKU.020.000.EXBTN_000_BTN.001",
		"WAKU.020.001.EXBTN_000_BTN.000",
	}
	expected := []string{"s_ped_mw00c", "s_ped_mw00c2", "s_ped_mw01a"}
	for i, key := range keys {
		v := tbl.Find(key)
		if v == nil { t.Errorf("%s not found", key); continue }
		if v[0].Str != expected[i] { t.Errorf("%s: got %q, want %q", key, v[0].Str, expected[i]) }
	}
}

// ============================================================
// Edge cases
// ============================================================

func TestParseEmpty(t *testing.T) {
	tbl := parseStr("")
	if tbl.Count() != 0 { t.Errorf("count: %d", tbl.Count()) }
}

func TestParseCommentOnly(t *testing.T) {
	tbl := parseStr("; just a comment\n; another one")
	if tbl.Count() != 0 { t.Errorf("count: %d", tbl.Count()) }
}

func TestParseFile(t *testing.T) {
	// Try parsing a real file if available
	_, err := ParseFile("/nonexistent/gameexe.ini")
	if err == nil { t.Error("expected error for nonexistent file") }
}

func TestValueKinds(t *testing.T) {
	tbl := parseStr(`#A = 42
#B = "hello"
#C = U
#D = N`)
	a := tbl.Find("A")
	if a[0].Kind != VInteger { t.Error("A kind") }
	b := tbl.Find("B")
	if b[0].Kind != VString { t.Error("B kind") }
	c := tbl.Find("C")
	if c[0].Kind != VEnabled || !c[0].Bool { t.Error("C kind") }
	d := tbl.Find("D")
	if d[0].Kind != VEnabled || d[0].Bool { t.Error("D kind") }
}
