package lexer

import (
	"testing"

	"github.com/yoremi/rldev-go/rlc/pkg/token"
)

func TestLexEmpty(t *testing.T) {
	l := New("", "test")
	tok := l.Next()
	if tok.Type != token.EOF {
		t.Errorf("empty: got %s, want EOF", tok.Type)
	}
}

func TestLexOperators(t *testing.T) {
	l := New("+ - * / % & | ^ << >> == != <= >= < > && || ! ~ -> = += -= *= /= %= &= |= ^= <<= >>=", "test")
	expected := []token.Type{
		token.ADD, token.SUB, token.MUL, token.DIV, token.MOD,
		token.AND, token.OR, token.XOR, token.SHL, token.SHR,
		token.EQU, token.NEQ, token.LTE, token.GTE, token.LTN, token.GTN,
		token.LAND, token.LOR, token.NOT, token.TILDE,
		token.ARROW, token.SET,
		token.SADD, token.SSUB, token.SMUL, token.SDIV, token.SMOD,
		token.SAND, token.SOR, token.SXOR, token.SSHL, token.SSHR,
	}
	for i, exp := range expected {
		tok := l.Next()
		if tok.Type != exp {
			t.Errorf("op[%d]: got %s, want %s", i, tok.Type, exp)
		}
	}
}

func TestLexPunctuation(t *testing.T) {
	l := New("( ) [ ] { } : ; , .", "test")
	expected := []token.Type{
		token.LPAR, token.RPAR, token.LSQU, token.RSQU,
		token.LCUR, token.RCUR, token.COLON, token.SEMI,
		token.COMMA, token.POINT,
	}
	for i, exp := range expected {
		tok := l.Next()
		if tok.Type != exp {
			t.Errorf("punct[%d]: got %s, want %s", i, tok.Type, exp)
		}
	}
}

func TestLexDecimalNumbers(t *testing.T) {
	tests := []struct{ src string; val int32 }{
		{"0", 0}, {"42", 42}, {"1_000", 1000}, {"255", 255},
	}
	for _, tt := range tests {
		l := New(tt.src, "test")
		tok := l.Next()
		if tok.Type != token.INTEGER || tok.IntVal != tt.val {
			t.Errorf("%s: got type=%s val=%d, want INTEGER %d", tt.src, tok.Type, tok.IntVal, tt.val)
		}
	}
}

func TestLexHexNumbers(t *testing.T) {
	tests := []struct{ src string; val int32 }{
		{"$ff", 255}, {"$FF", 255}, {"$1a", 26}, {"$0", 0},
		{"0xff", 255}, {"0xFF", 255},
	}
	for _, tt := range tests {
		l := New(tt.src, "test")
		tok := l.Next()
		if tok.Type != token.INTEGER || tok.IntVal != tt.val {
			t.Errorf("%s: got type=%s val=%d, want INTEGER %d", tt.src, tok.Type, tok.IntVal, tt.val)
		}
	}
}

func TestLexBinaryOctal(t *testing.T) {
	// $#binary
	l := New("$#1010", "test")
	tok := l.Next()
	if tok.Type != token.INTEGER || tok.IntVal != 10 {
		t.Errorf("$#1010: got %d, want 10", tok.IntVal)
	}
	// $%octal
	l = New("$%77", "test")
	tok = l.Next()
	if tok.Type != token.INTEGER || tok.IntVal != 63 {
		t.Errorf("$%%77: got %d, want 63", tok.IntVal)
	}
}

func TestLexKeywords(t *testing.T) {
	tests := []struct{ src string; typ token.Type }{
		{"if", token.IF}, {"else", token.ELSE}, {"while", token.WHILE},
		{"for", token.FOR}, {"case", token.CASE}, {"of", token.OF},
		{"other", token.OTHER}, {"ecase", token.ECASE},
		{"break", token.BREAK}, {"continue", token.CONTINUE},
		{"return", token.RETURN}, {"halt", token.DHALT}, {"eof", token.DEOF},
		{"repeat", token.REPEAT}, {"till", token.TILL},
		{"raw", token.RAW}, {"endraw", token.ENDRAW},
		{"op", token.OP}, {"_", token.USCORE},
	}
	for _, tt := range tests {
		l := New(tt.src, "test")
		tok := l.Next()
		if tok.Type != tt.typ {
			t.Errorf("%s: got %s, want %s", tt.src, tok.Type, tt.typ)
		}
	}
}

func TestLexTypeKeywords(t *testing.T) {
	tests := []struct{ src string; width int32 }{
		{"int", 32}, {"str", 0}, {"bit", 1}, {"bit2", 2}, {"bit4", 4}, {"byte", 8},
	}
	for _, tt := range tests {
		l := New(tt.src, "test")
		tok := l.Next()
		if tt.src == "str" {
			if tok.Type != token.STR {
				t.Errorf("%s: got %s, want STR", tt.src, tok.Type)
			}
		} else {
			if tok.Type != token.INT || tok.IntVal != tt.width {
				t.Errorf("%s: got type=%s val=%d, want INT %d", tt.src, tok.Type, tok.IntVal, tt.width)
			}
		}
	}
}

func TestLexDirectives(t *testing.T) {
	tests := []struct{ src string; typ token.Type }{
		{"#define", token.DDEFINE}, {"#sdefine", token.DDEFINE},
		{"#undef", token.DUNDEF}, {"#set", token.DSET},
		{"#const", token.DDEFINE}, {"#bind", token.DDEFINE},
		{"#target", token.DTARGET}, {"#version", token.DVERSION},
		{"#load", token.DLOAD},
		{"#if", token.DIF}, {"#ifdef", token.DIFDEF}, {"#ifndef", token.DIFDEF},
		{"#else", token.DELSE}, {"#elseif", token.DELSEIF}, {"#endif", token.DENDIF},
		{"#for", token.DFOR}, {"#inline", token.DINLINE}, {"#hiding", token.DHIDING},
	}
	for _, tt := range tests {
		l := New(tt.src, "test")
		tok := l.Next()
		if tok.Type != tt.typ {
			t.Errorf("%s: got %s, want %s", tt.src, tok.Type, tt.typ)
		}
	}
}

func TestLexDWithExpr(t *testing.T) {
	directives := []string{"#file", "#resource", "#entrypoint", "#character", "#kidoku_type", "#print", "#error", "#warn"}
	for _, d := range directives {
		l := New(d, "test")
		tok := l.Next()
		if tok.Type != token.DWITHEXPR {
			t.Errorf("%s: got %s, want DWITHEXPR", d, tok.Type)
		}
		if tok.StrVal != d[1:] {
			t.Errorf("%s: strval got %q, want %q", d, tok.StrVal, d[1:])
		}
	}
}

func TestLexVariables(t *testing.T) {
	tests := []struct{ src string; typ token.Type; bank int32 }{
		{"intA", token.VAR, 0x00}, {"intB", token.VAR, 0x01},
		{"intZ", token.VAR, 0x19}, {"intL", token.VAR, 0x0b},
		{"strK", token.SVAR, 0x0a}, {"strM", token.SVAR, 0x0c}, {"strS", token.SVAR, 0x12},
		{"store", token.REG, 0xc8},
		{"intAb", token.VAR, 0x1a}, {"intZ8b", token.VAR, 0x81},
	}
	for _, tt := range tests {
		l := New(tt.src, "test")
		tok := l.Next()
		if tok.Type != tt.typ || tok.IntVal != tt.bank {
			t.Errorf("%s: got type=%s bank=0x%02x, want %s 0x%02x", tt.src, tok.Type, tok.IntVal, tt.typ, tt.bank)
		}
	}
}

func TestLexGotoSelect(t *testing.T) {
	tests := []struct{ src string; typ token.Type; val int32 }{
		{"goto_on", token.GO_LIST, 0},
		{"gosub_on", token.GO_LIST, 0},
		{"goto_case", token.GO_CASE, 0},
		{"gosub_case", token.GO_CASE, 0},
		{"select", token.SELECT, 1},
		{"select_w", token.SELECT, 0},
		{"select_s", token.SELECT, 3},
		{"select_btncancel", token.SELECT, 12},
	}
	for _, tt := range tests {
		l := New(tt.src, "test")
		tok := l.Next()
		if tok.Type != tt.typ {
			t.Errorf("%s: got type=%s, want %s", tt.src, tok.Type, tt.typ)
		}
		if tok.IntVal != tt.val {
			t.Errorf("%s: got val=%d, want %d", tt.src, tok.IntVal, tt.val)
		}
	}
}

func TestLexStrings(t *testing.T) {
	l := New(`'hello' "world"`, "test")
	tok1 := l.Next()
	if tok1.Type != token.STRING || tok1.StrVal != "hello" {
		t.Errorf("string1: got %s %q, want STRING 'hello'", tok1.Type, tok1.StrVal)
	}
	tok2 := l.Next()
	if tok2.Type != token.STRING || tok2.StrVal != "world" {
		t.Errorf("string2: got %s %q, want STRING 'world'", tok2.Type, tok2.StrVal)
	}
}

func TestLexLabels(t *testing.T) {
	l := New("@start @loop_end", "test")
	tok1 := l.Next()
	if tok1.Type != token.LABEL || tok1.StrVal != "start" {
		t.Errorf("label1: got %s %q", tok1.Type, tok1.StrVal)
	}
	tok2 := l.Next()
	if tok2.Type != token.LABEL || tok2.StrVal != "loop_end" {
		t.Errorf("label2: got %s %q", tok2.Type, tok2.StrVal)
	}
}

func TestLexComments(t *testing.T) {
	// Line comment
	l := New("42 // comment\n43", "test")
	tok1 := l.Next()
	if tok1.IntVal != 42 { t.Errorf("before comment: got %d", tok1.IntVal) }
	tok2 := l.Next()
	if tok2.IntVal != 43 { t.Errorf("after comment: got %d", tok2.IntVal) }

	// Block comment
	l = New("10 {- block comment -} 20", "test")
	tok1 = l.Next()
	if tok1.IntVal != 10 { t.Errorf("before block: got %d", tok1.IntVal) }
	tok2 = l.Next()
	if tok2.IntVal != 20 { t.Errorf("after block: got %d", tok2.IntVal) }
}

func TestLexLineTracking(t *testing.T) {
	l := New("a\nb\nc", "test")
	tok1 := l.Next()
	if tok1.Line != 1 { t.Errorf("line 1: got %d", tok1.Line) }
	tok2 := l.Next()
	if tok2.Line != 2 { t.Errorf("line 2: got %d", tok2.Line) }
	tok3 := l.Next()
	if tok3.Line != 3 { t.Errorf("line 3: got %d", tok3.Line) }
}

func TestLexMagicConstants(t *testing.T) {
	l := New("__file__", "myfile.org")
	tok := l.Next()
	if tok.Type != token.STRING || tok.StrVal != "myfile.org" {
		t.Errorf("__file__: got %s %q", tok.Type, tok.StrVal)
	}

	l = New("__line__", "test")
	tok = l.Next()
	if tok.Type != token.INTEGER || tok.IntVal != 1 {
		t.Errorf("__line__: got %s %d", tok.Type, tok.IntVal)
	}
}

func TestLexIdentifier(t *testing.T) {
	l := New("foo bar_baz myFunc123", "test")
	names := []string{"foo", "bar_baz", "myFunc123"}
	for i, want := range names {
		tok := l.Next()
		if tok.Type != token.IDENT || tok.StrVal != want {
			t.Errorf("ident[%d]: got %s %q, want IDENT %q", i, tok.Type, tok.StrVal, want)
		}
	}
}

func TestLexMixedProgram(t *testing.T) {
	src := `#define MAX = 100
intA[0] = MAX
if intA[0] == 100
  goto @done
@done
halt`
	l := New(src, "test.org")
	// Count tokens (should be reasonable)
	count := 0
	for {
		tok := l.Next()
		if tok.Type == token.EOF { break }
		count++
	}
	if count < 15 {
		t.Errorf("mixed program: got only %d tokens, expected >= 15", count)
	}
}

func TestLexPeekBackup(t *testing.T) {
	l := New("1 2 3", "test")
	tok1 := l.Peek()
	if tok1.IntVal != 1 { t.Errorf("peek: got %d", tok1.IntVal) }
	tok1 = l.Next()
	if tok1.IntVal != 1 { t.Errorf("next after peek: got %d", tok1.IntVal) }
	tok2 := l.Next()
	if tok2.IntVal != 2 { t.Errorf("next 2: got %d", tok2.IntVal) }
	l.Backup()
	tok2b := l.Next()
	if tok2b.IntVal != 2 { t.Errorf("backup+next: got %d", tok2b.IntVal) }
}
