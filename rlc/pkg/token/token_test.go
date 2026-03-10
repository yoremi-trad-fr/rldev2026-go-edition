package token

import (
	"testing"
)

func TestTypeString(t *testing.T) {
	tests := []struct {
		typ  Type
		want string
	}{
		{EOF, "EOF"},
		{INTEGER, "INTEGER"},
		{LPAR, "("},
		{RPAR, ")"},
		{SADD, "+="},
		{EQU, "=="},
		{LAND, "&&"},
		{IF, "if"},
		{WHILE, "while"},
		{DDEFINE, "#define"},
		{SELECT, "SELECT"},
	}
	for _, tt := range tests {
		got := tt.typ.String()
		if got != tt.want {
			t.Errorf("Type(%d).String() = %q, want %q", int(tt.typ), got, tt.want)
		}
	}
}

func TestIsAssignOp(t *testing.T) {
	assigns := []Type{SADD, SSUB, SMUL, SDIV, SMOD, SAND, SOR, SXOR, SSHL, SSHR, SET}
	for _, a := range assigns {
		if !a.IsAssignOp() {
			t.Errorf("%s.IsAssignOp() = false, want true", a)
		}
	}
	nonAssigns := []Type{ADD, SUB, EQU, IF, IDENT}
	for _, na := range nonAssigns {
		if na.IsAssignOp() {
			t.Errorf("%s.IsAssignOp() = true, want false", na)
		}
	}
}

func TestMakeName(t *testing.T) {
	tests := []struct {
		scope NameScope
		index int
		want  string
	}{
		// Local name, index 0 → prefix \x81\x93 + \x82\x60
		{NameLocal, 0, "\x81\x93\x82\x60"},
		// Local name, index 25 → prefix \x81\x93 + \x82\x79
		{NameLocal, 25, "\x81\x93\x82\x79"},
		// Global name, index 0 → prefix \x81\x96 + \x82\x60
		{NameGlobal, 0, "\x81\x96\x82\x60"},
		// Index 26 → two letters: \x82\x60 \x82\x60
		{NameLocal, 26, "\x81\x93\x82\x60\x82\x60"},
	}
	for _, tt := range tests {
		got := MakeName(tt.scope, tt.index)
		if got != tt.want {
			t.Errorf("MakeName(%d, %d) = %q, want %q", tt.scope, tt.index, got, tt.want)
		}
	}
}

func TestIsOutputCode(t *testing.T) {
	if !IsOutputCode("i") {
		t.Error("IsOutputCode('i') should be true")
	}
	if !IsOutputCode("s") {
		t.Error("IsOutputCode('s') should be true")
	}
	if IsOutputCode("r") {
		t.Error("IsOutputCode('r') should be false")
	}
}

func TestIsObjectCode(t *testing.T) {
	objs := []string{"r", "n", "c", "size", "pos", "posx", "posy"}
	for _, o := range objs {
		if !IsObjectCode(o) {
			t.Errorf("IsObjectCode(%q) = false, want true", o)
		}
	}
	if IsObjectCode("i") {
		t.Error("IsObjectCode('i') should be false")
	}
	if IsObjectCode("wait") {
		t.Error("IsObjectCode('wait') should be false")
	}
}

func TestObjectCodeString(t *testing.T) {
	tests := []struct {
		ident  string
		params []int32
		want   string
		err    bool
	}{
		{"r", nil, "#D", false},
		{"n", nil, "#D", false},
		{"size", nil, "#S##", false},
		{"size", []int32{24}, "#S24##", false},
		{"c", nil, "#C##", false},
		{"c", []int32{255}, "#C255##", false},
		{"posx", []int32{100}, "#X100##", false},
		{"posy", []int32{50}, "#Y50##", false},
		{"pos", []int32{10, 20}, "#X10#Y20##", false},
		{"pos", []int32{10}, "#X10##", false},
		// Error cases
		{"r", []int32{1}, "", true},
		{"size", []int32{1, 2}, "", true},
		{"unknown", nil, "", true},
	}
	for _, tt := range tests {
		got, err := ObjectCodeString(tt.ident, tt.params)
		if (err != nil) != tt.err {
			t.Errorf("ObjectCodeString(%q, %v): err=%v, wantErr=%v", tt.ident, tt.params, err, tt.err)
			continue
		}
		if got != tt.want {
			t.Errorf("ObjectCodeString(%q, %v) = %q, want %q", tt.ident, tt.params, got, tt.want)
		}
	}
}

func TestTokensToString(t *testing.T) {
	// Simple text
	tokens := []StrToken{
		{Kind: StrText, Text: "hello"},
		{Kind: StrSpace, Count: 1},
		{Kind: StrText, Text: "world"},
	}
	got := TokensToString(tokens, false)
	if got != "hello world" {
		t.Errorf("simple text: got %q, want 'hello world'", got)
	}

	// With quote
	got = TokensToString(tokens, true)
	if got != "\"hello world\"" {
		t.Errorf("quoted text: got %q, want '\"hello world\"'", got)
	}

	// Special characters
	tokens = []StrToken{
		{Kind: StrHyphen},
		{Kind: StrText, Text: "abc"},
	}
	got = TokensToString(tokens, false)
	if got != "-abc" {
		t.Errorf("hyphen: got %q, want '-abc'", got)
	}

	// No quotes needed when no spaces/special
	tokens = []StrToken{
		{Kind: StrText, Text: "plain"},
	}
	got = TokensToString(tokens, true)
	if got != "plain" {
		t.Errorf("no-quote: got %q, want 'plain'", got)
	}
}

func TestTokensToStringEmpty(t *testing.T) {
	got := TokensToString(nil, false)
	if got != "" {
		t.Errorf("empty: got %q, want ''", got)
	}
}

func TestTokensToStringDQuote(t *testing.T) {
	tokens := []StrToken{
		{Kind: StrText, Text: "say"},
		{Kind: StrSpace, Count: 1},
		{Kind: StrDQuote},
		{Kind: StrText, Text: "hi"},
		{Kind: StrDQuote},
	}
	got := TokensToString(tokens, false)
	if got != "say \"hi\"" {
		t.Errorf("dquote: got %q, want 'say \"hi\"'", got)
	}
}
