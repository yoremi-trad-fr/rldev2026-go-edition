package lexer

import "testing"

func TestStrLexBasicText(t *testing.T) {
	sl := NewStrLexer("hello world", StrDouble, "test", 1)
	tokens := sl.TokenizeAll()
	// Should get: "hello" STText, " " STSpace, "world" STText
	if len(tokens) < 3 {
		t.Fatalf("got %d tokens, want >= 3", len(tokens))
	}
	if tokens[0].Type != STText || tokens[0].Text != "hello" {
		t.Errorf("token 0: got %v %q, want STText 'hello'", tokens[0].Type, tokens[0].Text)
	}
	if tokens[1].Type != STSpace {
		t.Errorf("token 1: got %v, want STSpace", tokens[1].Type)
	}
	if tokens[2].Type != STText || tokens[2].Text != "world" {
		t.Errorf("token 2: got %v %q, want STText 'world'", tokens[2].Type, tokens[2].Text)
	}
}

func TestStrLexTerminators(t *testing.T) {
	// Single-quote terminated
	sl := NewStrLexer("abc'rest", StrSingle, "test", 1)
	tokens := sl.TokenizeAll()
	if len(tokens) != 1 {
		t.Fatalf("single-quote: got %d tokens, want 1, tokens=%v", len(tokens), tokens)
	}
	if tokens[0].Text != "abc" {
		t.Errorf("single-quote: got %q, want 'abc'", tokens[0].Text)
	}

	// Double-quote terminated
	sl = NewStrLexer(`abc"rest`, StrDouble, "test", 1)
	tokens = sl.TokenizeAll()
	if len(tokens) != 1 {
		t.Fatalf("double-quote: got %d tokens, want 1", len(tokens))
	}

	// ResStr terminated by <
	sl = NewStrLexer("abc<rest", StrResStr, "test", 1)
	tokens = sl.TokenizeAll()
	if len(tokens) != 1 {
		t.Fatalf("resstr: got %d tokens, want 1", len(tokens))
	}
}

func TestStrLexEscapes(t *testing.T) {
	// Escaped quote \"
	sl := NewStrLexer(`ab\"cd`, StrSingle, "test", 1)
	tokens := sl.TokenizeAll()
	found := false
	for _, tok := range tokens {
		if tok.Type == STDQuote {
			found = true
		}
	}
	if !found {
		t.Error("expected DQuote token for \\\"")
	}

	// Escaped character \kX
	sl = NewStrLexer(`\kZ`, StrDouble, "test", 1)
	tokens = sl.TokenizeAll()
	if len(tokens) != 1 {
		t.Fatalf("\\kZ: got %d tokens, want 1", len(tokens))
	}
	if tokens[0].Type != STText || tokens[0].Text != "Z" {
		t.Errorf("\\kZ: got %v %q, want STText 'Z'", tokens[0].Type, tokens[0].Text)
	}
}

func TestStrLexControlCodes(t *testing.T) {
	tests := []struct {
		input string
		ident string
	}{
		{`\n{0}`, "n"},
		{`\r{0}`, "r"},
		{`\p{0}`, "p"},
		{`\c{255}`, "c"},
		{`\size{24}`, "size"},
		{`\wait{500}`, "wait"},
		{`\i{42}`, "i"},
		{`\s{strS[0]}`, "s"},
	}
	for _, tt := range tests {
		sl := NewStrLexer(tt.input, StrDouble, "test", 1)
		tokens := sl.TokenizeAll()
		if len(tokens) == 0 {
			t.Errorf("%s: got 0 tokens", tt.input)
			continue
		}
		if tokens[0].Type != STCode {
			t.Errorf("%s: got type %v, want STCode", tt.input, tokens[0].Type)
			continue
		}
		if tokens[0].Ident != tt.ident {
			t.Errorf("%s: got ident %q, want %q", tt.input, tokens[0].Ident, tt.ident)
		}
	}
}

func TestStrLexNameCodes(t *testing.T) {
	// \l{0} — local name
	sl := NewStrLexer(`\l{0}`, StrDouble, "test", 1)
	tokens := sl.TokenizeAll()
	if len(tokens) != 1 {
		t.Fatalf("\\l{0}: got %d tokens, want 1", len(tokens))
	}
	if tokens[0].Type != STName {
		t.Errorf("\\l{0}: got type %v, want STName", tokens[0].Type)
	}
	if !tokens[0].IsLocal {
		t.Error("\\l{0}: expected IsLocal=true")
	}
	if tokens[0].Params != "0" {
		t.Errorf("\\l{0}: got params %q, want '0'", tokens[0].Params)
	}

	// \m{1} — global name
	sl = NewStrLexer(`\m{1}`, StrDouble, "test", 1)
	tokens = sl.TokenizeAll()
	if len(tokens) != 1 {
		t.Fatalf("\\m{1}: got %d tokens, want 1", len(tokens))
	}
	if tokens[0].IsLocal {
		t.Error("\\m{1}: expected IsLocal=false")
	}
}

func TestStrLexSpeaker(t *testing.T) {
	// \{ — speaker block
	sl := NewStrLexer(`\{some text}`, StrDouble, "test", 1)
	tokens := sl.TokenizeAll()
	if len(tokens) == 0 {
		t.Fatal("got 0 tokens")
	}
	if tokens[0].Type != STSpeaker {
		t.Errorf("got type %v, want STSpeaker", tokens[0].Type)
	}
}

func TestStrLexRuby(t *testing.T) {
	sl := NewStrLexer(`\ruby{漢字}={かんじ}`, StrDouble, "test", 1)
	tokens := sl.TokenizeAll()
	if len(tokens) == 0 {
		t.Fatal("ruby: got 0 tokens")
	}
	if tokens[0].Type != STGloss {
		t.Errorf("ruby: got type %v, want STGloss", tokens[0].Type)
	}
	if tokens[0].Ident != "ruby" {
		t.Errorf("ruby: got ident %q, want 'ruby'", tokens[0].Ident)
	}
	if tokens[0].Params != "漢字" {
		t.Errorf("ruby: got params %q, want '漢字'", tokens[0].Params)
	}
	if tokens[0].Gloss != "かんじ" {
		t.Errorf("ruby: got gloss %q, want 'かんじ'", tokens[0].Gloss)
	}
}

func TestStrLexGloss(t *testing.T) {
	sl := NewStrLexer(`\g{term}=<key123>`, StrDouble, "test", 1)
	tokens := sl.TokenizeAll()
	if len(tokens) == 0 {
		t.Fatal("gloss: got 0 tokens")
	}
	if tokens[0].Type != STGloss || tokens[0].Ident != "g" {
		t.Errorf("gloss: got type=%v ident=%q", tokens[0].Type, tokens[0].Ident)
	}
	if tokens[0].Params != "term" {
		t.Errorf("gloss: params got %q, want 'term'", tokens[0].Params)
	}
	if tokens[0].GlossID != "key123" {
		t.Errorf("gloss: glossID got %q, want 'key123'", tokens[0].GlossID)
	}
}

func TestStrLexSpecialChars(t *testing.T) {
	tests := []struct {
		input string
		want  StrTokenType
	}{
		{"【", STLLentic},  // U+3010 LEFT BLACK LENTICULAR BRACKET
		{"】", STRLentic},  // U+3011 RIGHT BLACK LENTICULAR BRACKET
		{"＊", STAsterisk},
		{"％", STPercent},
		{"-", STHyphen},
	}
	for _, tt := range tests {
		sl := NewStrLexer(tt.input, StrDouble, "test", 1)
		tokens := sl.TokenizeAll()
		if len(tokens) != 1 {
			t.Errorf("%q: got %d tokens, want 1", tt.input, len(tokens))
			continue
		}
		if tokens[0].Type != tt.want {
			t.Errorf("%q: got %v, want %v", tt.input, tokens[0].Type, tt.want)
		}
	}
}

func TestStrLexSpaceCounting(t *testing.T) {
	// Regular spaces
	sl := NewStrLexer("   ", StrDouble, "test", 1)
	tokens := sl.TokenizeAll()
	if len(tokens) != 1 || tokens[0].Type != STSpace || tokens[0].Count != 3 {
		t.Errorf("3 spaces: got %v count=%d, want STSpace count=3", tokens[0].Type, tokens[0].Count)
	}

	// Tab counts as 2
	sl = NewStrLexer("\t", StrDouble, "test", 1)
	tokens = sl.TokenizeAll()
	if len(tokens) != 1 || tokens[0].Count != 2 {
		t.Errorf("tab: got count=%d, want 2", tokens[0].Count)
	}
}

func TestStrLexLineContinuation(t *testing.T) {
	// \ at end of line continues the string
	sl := NewStrLexer("abc\\\ndef", StrDouble, "test", 1)
	tokens := sl.TokenizeAll()
	// Should get abc, then def (no space between)
	texts := ""
	for _, tok := range tokens {
		if tok.Type == STText {
			texts += tok.Text
		}
	}
	if texts != "abcdef" {
		t.Errorf("line continuation: got %q, want 'abcdef'", texts)
	}
}

func TestStrLexResourceRef(t *testing.T) {
	sl := NewStrLexer(`\res{mykey}`, StrDouble, "test", 1)
	tokens := sl.TokenizeAll()
	if len(tokens) != 1 {
		t.Fatalf("\\res: got %d tokens, want 1", len(tokens))
	}
	if tokens[0].Type != STResRef || tokens[0].Params != "mykey" {
		t.Errorf("\\res: got type=%v params=%q", tokens[0].Type, tokens[0].Params)
	}
}

func TestStrLexAdd(t *testing.T) {
	sl := NewStrLexer(`\a{key1}`, StrDouble, "test", 1)
	tokens := sl.TokenizeAll()
	if len(tokens) != 1 || tokens[0].Type != STAdd || tokens[0].Params != "key1" {
		t.Errorf("\\a{key}: got type=%v params=%q", tokens[0].Type, tokens[0].Params)
	}

	// Anonymous \a
	sl = NewStrLexer(`\a rest`, StrDouble, "test", 1)
	tokens = sl.TokenizeAll()
	if tokens[0].Type != STAdd {
		t.Errorf("\\a: got type=%v, want STAdd", tokens[0].Type)
	}
}

func TestStrLexDBCS(t *testing.T) {
	// Japanese text should have DBCS flag
	sl := NewStrLexer("こんにちは", StrDouble, "test", 1)
	tokens := sl.TokenizeAll()
	if len(tokens) == 0 {
		t.Fatal("DBCS: got 0 tokens")
	}
	if !tokens[0].DBCS {
		t.Error("DBCS: expected DBCS=true for Japanese text")
	}
}

func TestStrLexNestedBraces(t *testing.T) {
	sl := NewStrLexer(`\f{a{b}c}`, StrDouble, "test", 1)
	tokens := sl.TokenizeAll()
	if len(tokens) != 1 {
		t.Fatalf("nested braces: got %d tokens, want 1", len(tokens))
	}
	if tokens[0].Type != STRewrite || tokens[0].Params != "a{b}c" {
		t.Errorf("nested braces: got type=%v params=%q, want STRewrite 'a{b}c'", tokens[0].Type, tokens[0].Params)
	}
}

func TestStrLexLineTracking(t *testing.T) {
	sl := NewStrLexer("line1\nline2\nline3", StrResStr, "test", 1)
	sl.TokenizeAll()
	if sl.Line() != 3 {
		t.Errorf("line tracking: got %d, want 3", sl.Line())
	}
}
