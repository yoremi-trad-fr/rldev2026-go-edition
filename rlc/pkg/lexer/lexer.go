// Package lexer implements tokenizers for the Kepago language.
//
// This file: the main source lexer (keULexer.ml, 510 lines OCaml).
// Tokenizes .org source files into token.Token values for the parser.
//
// Features:
//   - Unicode-aware identifier scanning (ASCII, Hiragana, Katakana, CJK, fullwidth)
//   - Kepago number formats: decimal, $hex, $#binary, $%octal, 0xhex
//   - Line comments (//) and block comments ({- ... -})
//   - Full keyword/variable/directive table
//   - String literals (delegates to StrLexer for rich tokens)
//   - Labels (@identifier)
//   - Magic constants: __file__, __line__
//   - #line directive for line number override
package lexer

import (
	"strconv"
	"strings"

	"github.com/yoremi/rldev-go/rlc/pkg/token"
)

// Lexer tokenizes Kepago source code into a stream of token.Token values.
type Lexer struct {
	src    []rune
	pos    int
	line   int
	file   string
	tokens []token.Token
	idx    int // current read position in tokens
}

// New creates a Lexer from source text.
func New(src, file string) *Lexer {
	l := &Lexer{src: []rune(src), line: 1, file: file}
	l.scan()
	return l
}

// Next returns the next token and advances.
func (l *Lexer) Next() token.Token {
	if l.idx >= len(l.tokens) {
		return token.Token{Type: token.EOF, Line: l.line, File: l.file}
	}
	t := l.tokens[l.idx]
	l.idx++
	return t
}

// Peek returns the next token without advancing.
func (l *Lexer) Peek() token.Token {
	if l.idx >= len(l.tokens) {
		return token.Token{Type: token.EOF, Line: l.line, File: l.file}
	}
	return l.tokens[l.idx]
}

// Backup unreads one token.
func (l *Lexer) Backup() {
	if l.idx > 0 {
		l.idx--
	}
}

// --- Internal helpers ---

func (l *Lexer) ch() rune {
	if l.pos >= len(l.src) {
		return 0
	}
	return l.src[l.pos]
}

func (l *Lexer) peek1() rune {
	if l.pos+1 >= len(l.src) {
		return 0
	}
	return l.src[l.pos+1]
}

func (l *Lexer) peek2() rune {
	if l.pos+2 >= len(l.src) {
		return 0
	}
	return l.src[l.pos+2]
}

func (l *Lexer) advance() { l.pos++ }

func (l *Lexer) emit(typ token.Type) {
	l.tokens = append(l.tokens, token.Token{Type: typ, Line: l.line, File: l.file})
}

func (l *Lexer) emitInt(val int32) {
	l.tokens = append(l.tokens, token.Token{Type: token.INTEGER, IntVal: val, Line: l.line, File: l.file})
}

func (l *Lexer) emitStr(typ token.Type, s string) {
	l.tokens = append(l.tokens, token.Token{Type: typ, StrVal: s, Line: l.line, File: l.file})
}

func (l *Lexer) emitIntStr(typ token.Type, iv int32, sv string) {
	l.tokens = append(l.tokens, token.Token{Type: typ, IntVal: iv, StrVal: sv, Line: l.line, File: l.file})
}

// --- Main scan loop ---

func (l *Lexer) scan() {
	for l.pos < len(l.src) {
		l.skipWhitespace()
		if l.pos >= len(l.src) {
			break
		}

		c := l.ch()
		c1 := l.peek1()

		// --- Comments ---
		if c == '/' && c1 == '/' {
			l.skipLineComment()
			continue
		}
		if c == '{' && c1 == '-' {
			l.skipBlockComment()
			continue
		}
		if c == '/' && c1 == '*' {
			l.skipCBlockComment()
			continue
		}

		// --- Three-char operators ---
		if c == '<' && c1 == '<' && l.peek2() == '=' {
			l.pos += 3; l.emit(token.SSHL); continue
		}
		if c == '>' && c1 == '>' && l.peek2() == '=' {
			l.pos += 3; l.emit(token.SSHR); continue
		}

		// --- Two-char operators ---
		if l.pos+1 < len(l.src) {
			if l.scanTwoChar(c, c1) {
				continue
			}
		}

		// --- Single-char operators/punctuation ---
		if l.scanOneChar(c) {
			continue
		}

		// --- Numbers ---
		if c == '$' || (c >= '0' && c <= '9') {
			l.scanNumber()
			continue
		}

		// --- Strings ---
		if c == '\'' || c == '"' {
			l.scanString()
			continue
		}

		// --- Labels ---
		if c == '@' {
			l.scanLabel()
			continue
		}

		// --- Identifiers / keywords / directives ---
		if isIdentStart(c) || c == '#' {
			// Special case: an opcode literal of the form
			//   op<TYPE:MODULE:FUNCTION, OVERLOAD>
			// emitted by the disassembler when the KFN doesn't know the
			// opcode name. Consume it as a single IDENT so the colons
			// and comma inside don't break the parser.
			if c == 'o' && l.peek1() == 'p' && l.peek2() == '<' {
				if l.scanOpcodeLiteral() {
					continue
				}
			}
			// Special case: `#res<KEY>` resource reference. OCaml's
			// keULexer.ml L302 produces a single DRES token here. The
			// disassembler emits this form for `res`-typed parameters,
			// e.g. `title (#res<0067>)`.
			if c == '#' && l.matchesAhead("#res") {
				if l.scanResRef() {
					continue
				}
			}
			l.scanIdentOrKeyword()
			continue
		}

		// Skip unknown
		l.advance()
	}

	l.tokens = append(l.tokens, token.Token{Type: token.EOF, Line: l.line, File: l.file})
}

// --- Two-char operator scan ---

func (l *Lexer) scanTwoChar(c, c1 rune) bool {
	var typ token.Type
	switch {
	case c == '+' && c1 == '=': typ = token.SADD
	case c == '-' && c1 == '=': typ = token.SSUB
	case c == '*' && c1 == '=': typ = token.SMUL
	case c == '/' && c1 == '=': typ = token.SDIV
	case c == '%' && c1 == '=': typ = token.SMOD
	case c == '&' && c1 == '=': typ = token.SAND
	case c == '|' && c1 == '=': typ = token.SOR
	case c == '^' && c1 == '=': typ = token.SXOR
	case c == '-' && c1 == '>': typ = token.ARROW
	case c == '<' && c1 == '<': typ = token.SHL
	case c == '>' && c1 == '>': typ = token.SHR
	case c == '=' && c1 == '=': typ = token.EQU
	case c == '!' && c1 == '=': typ = token.NEQ
	case c == '<' && c1 == '=': typ = token.LTE
	case c == '>' && c1 == '=': typ = token.GTE
	case c == '&' && c1 == '&': typ = token.LAND
	case c == '|' && c1 == '|': typ = token.LOR
	default:
		return false
	}
	l.pos += 2
	l.emit(typ)
	return true
}

// --- Single-char operator scan ---

func (l *Lexer) scanOneChar(c rune) bool {
	var typ token.Type
	switch c {
	case '(': typ = token.LPAR
	case ')': typ = token.RPAR
	case '[': typ = token.LSQU
	case ']': typ = token.RSQU
	case '{': typ = token.LCUR
	case '}': typ = token.RCUR
	case ':': typ = token.COLON
	case ';': typ = token.SEMI
	case ',': typ = token.COMMA
	case '.': typ = token.POINT
	case '=': typ = token.SET
	case '+': typ = token.ADD
	case '-': typ = token.SUB
	case '*': typ = token.MUL
	case '/': typ = token.DIV
	case '%': typ = token.MOD
	case '&': typ = token.AND
	case '|': typ = token.OR
	case '^': typ = token.XOR
	case '<': typ = token.LTN
	case '>': typ = token.GTN
	case '!': typ = token.NOT
	case '~': typ = token.TILDE
	default:
		return false
	}
	l.pos++
	l.emit(typ)
	return true
}

// --- Numbers ---
// Kepago formats: decimal, $hex, $#binary, $%octal, 0xhex

func (l *Lexer) scanNumber() {
	c := l.ch()

	// 0xHEX (C-style, with warning in OCaml)
	if c == '0' && (l.peek1() == 'x' || l.peek1() == 'X') {
		l.pos += 2
		s := l.collectWhile(isHexDigitOrUnderscore)
		s = strings.ReplaceAll(s, "_", "")
		v, _ := strconv.ParseInt("0x"+s, 0, 32)
		l.emitInt(int32(v))
		return
	}

	// $hex, $#binary, $%octal
	if c == '$' {
		l.pos++
		if l.pos >= len(l.src) {
			// lone $ — treat as ident start
			l.pos--
			l.scanIdentOrKeyword()
			return
		}
		next := l.ch()
		if next == '#' {
			// $#binary
			l.pos++
			s := l.collectWhile(func(r rune) bool { return r == '0' || r == '1' || r == '_' })
			s = strings.ReplaceAll(s, "_", "")
			if s == "" { s = "0" }
			v, _ := strconv.ParseInt(s, 2, 32)
			l.emitInt(int32(v))
		} else if next == '%' {
			// $%octal
			l.pos++
			s := l.collectWhile(func(r rune) bool { return (r >= '0' && r <= '7') || r == '_' })
			s = strings.ReplaceAll(s, "_", "")
			if s == "" { s = "0" }
			v, _ := strconv.ParseInt(s, 8, 32)
			l.emitInt(int32(v))
		} else if isHexDigit(next) {
			// $hex
			s := l.collectWhile(isHexDigitOrUnderscore)
			s = strings.ReplaceAll(s, "_", "")
			v, _ := strconv.ParseInt(s, 16, 32)
			l.emitInt(int32(v))
		} else {
			// $ followed by non-hex → rewind, treat as ident
			l.pos--
			l.scanIdentOrKeyword()
		}
		return
	}

	// Decimal
	s := l.collectWhile(func(r rune) bool { return (r >= '0' && r <= '9') || r == '_' })
	s = strings.ReplaceAll(s, "_", "")
	v, _ := strconv.ParseInt(s, 10, 32)
	l.emitInt(int32(v))
}

// --- Strings ---

func (l *Lexer) scanString() {
	quote := l.ch()
	l.pos++ // skip opening quote
	var sb strings.Builder
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if c == quote {
			l.pos++
			break
		}
		if c == '\\' && l.pos+1 < len(l.src) {
			l.pos++
			switch l.src[l.pos] {
			case 'n':  sb.WriteByte('\n')
			case 't':  sb.WriteByte('\t')
			case '\\': sb.WriteByte('\\')
			case '\'': sb.WriteByte('\'')
			case '"':  sb.WriteByte('"')
			default:   sb.WriteRune(l.src[l.pos])
			}
			l.pos++
		} else {
			if c == '\n' { l.line++ }
			sb.WriteRune(c)
			l.pos++
		}
	}
	l.emitStr(token.STRING, sb.String())
}

// --- Labels ---

func (l *Lexer) scanLabel() {
	l.pos++ // skip @
	start := l.pos
	for l.pos < len(l.src) && isIdentCont(l.src[l.pos]) {
		l.pos++
	}
	name := string(l.src[start:l.pos])
	l.emitStr(token.LABEL, name)
}

// --- Identifiers and keywords ---

func (l *Lexer) scanIdentOrKeyword() {
	start := l.pos
	for l.pos < len(l.src) && isIdentCont(l.src[l.pos]) {
		l.pos++
	}
	word := string(l.src[start:l.pos])

	// Magic constants
	if word == "__file__" {
		l.emitStr(token.STRING, l.file)
		return
	}
	if word == "__line__" {
		l.emitInt(int32(l.line))
		return
	}

	// #line directive: adjust line number
	if word == "#line" {
		l.skipInlineSpace()
		if l.pos < len(l.src) && l.src[l.pos] >= '0' && l.src[l.pos] <= '9' {
			ns := l.collectWhile(func(r rune) bool { return r >= '0' && r <= '9' })
			n, _ := strconv.Atoi(ns)
			l.line = n
		}
		return
	}

	// Keyword lookup
	if tok, ok := keywords[word]; ok {
		l.tokens = append(l.tokens, token.Token{
			Type:   tok.Type,
			IntVal: tok.IntVal,
			StrVal: tok.StrVal,
			Line:   l.line,
			File:   l.file,
		})
		return
	}

	// Default: identifier
	l.emitStr(token.IDENT, word)
}

// --- Whitespace and comments ---

func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if c == ' ' || c == '\t' || c == '\r' || c == 0x3000 {
			l.pos++
		} else if c == '\n' {
			l.pos++
			l.line++
		} else {
			break
		}
	}
}

func (l *Lexer) skipInlineSpace() {
	for l.pos < len(l.src) && (l.src[l.pos] == ' ' || l.src[l.pos] == '\t') {
		l.pos++
	}
}

func (l *Lexer) skipLineComment() {
	l.pos += 2 // skip //
	for l.pos < len(l.src) && l.src[l.pos] != '\n' {
		l.pos++
	}
	if l.pos < len(l.src) {
		l.pos++ // skip \n
		l.line++
	}
}

// scanOpcodeLiteral consumes a literal of the form
//   op<TYPE:MODULE:FUNCTION, OVERLOAD>
// and emits it as a single IDENT token whose StrVal is the entire literal
// (e.g. "op<35:035:21072,84>"). Returns true when the literal was matched
// and consumed, false otherwise (in which case the lexer falls through
// to normal identifier scanning).
//
// This shape comes from the disassembler when the KFN registry has no
// name for an opcode. Treating it as a single token keeps the colons
// and comma from triggering parseBlock or parseParamList paths that
// would otherwise crash.
func (l *Lexer) scanOpcodeLiteral() bool {
	start := l.pos
	if !(l.pos+2 < len(l.src) && l.src[l.pos] == 'o' && l.src[l.pos+1] == 'p' && l.src[l.pos+2] == '<') {
		return false
	}
	pos := l.pos + 3
	depth := 1
	for pos < len(l.src) && depth > 0 {
		switch l.src[pos] {
		case '<':
			depth++
		case '>':
			depth--
		case '\n':
			// Don't span lines.
			return false
		}
		pos++
	}
	if depth != 0 {
		return false
	}
	name := string(l.src[start:pos])
	// Strip whitespace inside the literal so the IDENT is canonical.
	var b strings.Builder
	b.Grow(len(name))
	for i := 0; i < len(name); i++ {
		if name[i] == ' ' || name[i] == '\t' {
			continue
		}
		b.WriteByte(name[i])
	}
	l.pos = pos
	l.emitStr(token.IDENT, b.String())
	return true
}

// matchesAhead reports whether the next len(s) runes of source starting
// at the current position spell `s` exactly.
func (l *Lexer) matchesAhead(s string) bool {
	rs := []rune(s)
	if l.pos+len(rs) > len(l.src) {
		return false
	}
	for i, r := range rs {
		if l.src[l.pos+i] != r {
			return false
		}
	}
	return true
}

// scanResRef consumes a `#res<KEY>` resource reference and emits a
// single DRES token whose StrVal is the key. The OCaml reference is
// keULexer.ml L302:
//
//	"#res" [" \t\r\n"]* "<" [" \t\r\n"]*  → DRES key
//
// The key continues until a closing `>` and is taken verbatim. Returns
// true when the form is successfully consumed; on any mismatch the
// position is restored and `false` is returned so the caller can fall
// back to the regular identifier path.
func (l *Lexer) scanResRef() bool {
	start := l.pos
	if !l.matchesAhead("#res") {
		return false
	}
	pos := l.pos + 4
	// Optional whitespace between '#res' and '<'.
	for pos < len(l.src) && (l.src[pos] == ' ' || l.src[pos] == '\t') {
		pos++
	}
	if pos >= len(l.src) || l.src[pos] != '<' {
		// Not a resource reference — could be `#resource` etc.
		l.pos = start
		return false
	}
	pos++ // skip '<'
	for pos < len(l.src) && (l.src[pos] == ' ' || l.src[pos] == '\t') {
		pos++
	}
	keyStart := pos
	for pos < len(l.src) && l.src[pos] != '>' && l.src[pos] != '\n' {
		pos++
	}
	if pos >= len(l.src) || l.src[pos] != '>' {
		l.pos = start
		return false
	}
	key := strings.TrimSpace(string(l.src[keyStart:pos]))
	l.pos = pos + 1 // skip '>'
	l.emitStr(token.DRES, key)
	return true
}

func (l *Lexer) skipBlockComment() {
	l.pos += 2 // skip {-
	for l.pos < len(l.src) {
		if l.src[l.pos] == '-' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '}' {
			l.pos += 2
			return
		}
		if l.src[l.pos] == '\n' {
			l.line++
		}
		l.pos++
	}
}

// skipCBlockComment handles C-style /* ... */ block comments. The
// disassembler emits these around inline annotations such as
// "/* nested:30 bytes */" and "/* STORE = */", so we tolerate them here
// to keep the round-trip clean.
func (l *Lexer) skipCBlockComment() {
	l.pos += 2 // skip /*
	for l.pos < len(l.src) {
		if l.src[l.pos] == '*' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '/' {
			l.pos += 2
			return
		}
		if l.src[l.pos] == '\n' {
			l.line++
		}
		l.pos++
	}
}

func (l *Lexer) collectWhile(pred func(rune) bool) string {
	start := l.pos
	for l.pos < len(l.src) && pred(l.src[l.pos]) {
		l.pos++
	}
	return string(l.src[start:l.pos])
}

// --- Character classification ---

func isIdentStart(r rune) bool {
	if r >= 'A' && r <= 'Z' { return true }
	if r >= 'a' && r <= 'z' { return true }
	if r == '_' || r == '?' || r == '#' || r == '$' { return true }
	if r >= 0x3041 && r <= 0x3093 { return true } // Hiragana
	if r >= 0x30A1 && r <= 0x30F6 { return true } // Katakana
	if r >= 0x4E00 && r <= 0x9FA0 { return true } // CJK
	if r >= 0xFF01 && r <= 0xFF5E { return true } // Fullwidth ASCII
	return false
}

func isIdentCont(r rune) bool {
	return isIdentStart(r) || (r >= '0' && r <= '9')
}

func isHexDigit(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func isHexDigitOrUnderscore(r rune) bool {
	return isHexDigit(r) || r == '_'
}

// ============================================================
// Keyword table — matches OCaml keULexer.ml keyword map
// ============================================================

// kwEntry is a pre-built token for keyword lookup.
type kwEntry struct {
	Type   token.Type
	IntVal int32
	StrVal string
}

var keywords = map[string]kwEntry{
	// --- Directives ---
	"#file":        {token.DWITHEXPR, 0, "file"},
	"#resource":    {token.DWITHEXPR, 0, "resource"},
	"#base_res":    {token.DWITHEXPR, 0, "base_res"},
	"#entrypoint":  {token.DWITHEXPR, 0, "entrypoint"},
	"#character":   {token.DWITHEXPR, 0, "character"},
	"#val_0x2c":    {token.DWITHEXPR, 0, "val_0x2c"},
	"#kidoku_type": {token.DWITHEXPR, 0, "kidoku_type"},
	"#print":       {token.DWITHEXPR, 0, "print"},
	"#error":       {token.DWITHEXPR, 0, "error"},
	"#warn":        {token.DWITHEXPR, 0, "warn"},
	"#exclude":     {token.DWITHEXPR, 0, "exclude"},
	"#hiding":      {token.DHIDING, 0, ""},
	"#define":      {token.DDEFINE, 0, "define"},
	"#sdefine":     {token.DDEFINE, 0, "sdefine"},
	"#undef":       {token.DUNDEF, 0, ""},
	"#redef":       {token.DDEFINE, 0, "redefine"},
	"#const":       {token.DDEFINE, 0, "const"},
	"#bind":        {token.DDEFINE, 0, "bind"},
	"#ebind":       {token.DDEFINE, 0, "ebind"},
	"#set":         {token.DSET, 0, ""},
	"#target":      {token.DTARGET, 0, ""},
	"#version":     {token.DVERSION, 0, ""},
	"#inline":      {token.DINLINE, 0, ""},
	"#sinline":     {token.DINLINE, 0, "scoped"},
	"#load":        {token.DLOAD, 0, ""},
	"#if":          {token.DIF, 0, ""},
	"#ifdef":       {token.DIFDEF, 1, ""}, // IntVal=1 → true
	"#ifndef":      {token.DIFDEF, 0, ""}, // IntVal=0 → false
	"#else":        {token.DELSE, 0, ""},
	"#elseif":      {token.DELSEIF, 0, ""},
	"#endif":       {token.DENDIF, 0, ""},
	"#for":         {token.DFOR, 0, ""},

	// --- Statement keywords ---
	"eof":      {token.DEOF, 0, ""},
	"halt":     {token.DHALT, 0, ""},
	"op":       {token.OP, 0, ""},
	"return":   {token.RETURN, 0, ""},
	"_":        {token.USCORE, 0, ""},
	"if":       {token.IF, 0, ""},
	"else":     {token.ELSE, 0, ""},
	"while":    {token.WHILE, 0, ""},
	"repeat":   {token.REPEAT, 0, ""},
	"till":     {token.TILL, 0, ""},
	"for":      {token.FOR, 0, ""},
	"case":     {token.CASE, 0, ""},
	"of":       {token.OF, 0, ""},
	"other":    {token.OTHER, 0, ""},
	"ecase":    {token.ECASE, 0, ""},
	"break":    {token.BREAK, 0, ""},
	"continue": {token.CONTINUE, 0, ""},
	"raw":      {token.RAW, 0, ""},
	"endraw":   {token.ENDRAW, 0, ""},

	// --- Type keywords ---
	"int":  {token.INT, 32, ""},
	"str":  {token.STR, 0, ""},
	"bit":  {token.INT, 1, ""},
	"bit2": {token.INT, 2, ""},
	"bit4": {token.INT, 4, ""},
	"byte": {token.INT, 8, ""},

	// --- Special ---
	"$s":     {token.SPECIAL, 0, "s"},
	"$pause": {token.SPECIAL, 0, "pause"},

	// --- Register ---
	"store": {token.REG, 0xc8, ""},

	// --- Integer variables (intX prefix) ---
	"intA": {token.VAR, 0x00, "intA"}, "intB": {token.VAR, 0x01, "intB"},
	"intC": {token.VAR, 0x02, "intC"}, "intD": {token.VAR, 0x03, "intD"},
	"intE": {token.VAR, 0x04, "intE"}, "intF": {token.VAR, 0x05, "intF"},
	"intG": {token.VAR, 0x06, "intG"}, "intZ": {token.VAR, 0x19, "intZ"},
	"intL": {token.VAR, 0x0b, "intL"},
	// Byte-width variants
	"intAb": {token.VAR, 0x1a, ""}, "intBb": {token.VAR, 0x1b, ""},
	"intCb": {token.VAR, 0x1c, ""}, "intDb": {token.VAR, 0x1d, ""},
	"intEb": {token.VAR, 0x1e, ""}, "intFb": {token.VAR, 0x1f, ""},
	"intGb": {token.VAR, 0x20, ""}, "intZb": {token.VAR, 0x33, ""},
	"intA2b": {token.VAR, 0x34, ""}, "intB2b": {token.VAR, 0x35, ""},
	"intC2b": {token.VAR, 0x36, ""}, "intD2b": {token.VAR, 0x37, ""},
	"intE2b": {token.VAR, 0x38, ""}, "intF2b": {token.VAR, 0x39, ""},
	"intG2b": {token.VAR, 0x3a, ""}, "intZ2b": {token.VAR, 0x4d, ""},
	"intA4b": {token.VAR, 0x4e, ""}, "intB4b": {token.VAR, 0x4f, ""},
	"intC4b": {token.VAR, 0x50, ""}, "intD4b": {token.VAR, 0x51, ""},
	"intE4b": {token.VAR, 0x52, ""}, "intF4b": {token.VAR, 0x53, ""},
	"intG4b": {token.VAR, 0x54, ""}, "intZ4b": {token.VAR, 0x67, ""},
	"intA8b": {token.VAR, 0x68, ""}, "intB8b": {token.VAR, 0x69, ""},
	"intC8b": {token.VAR, 0x6a, ""}, "intD8b": {token.VAR, 0x6b, ""},
	"intE8b": {token.VAR, 0x6c, ""}, "intF8b": {token.VAR, 0x6d, ""},
	"intG8b": {token.VAR, 0x6e, ""}, "intZ8b": {token.VAR, 0x81, ""},

	// --- String variables (strX prefix) ---
	"strK": {token.SVAR, 0x0a, "strK"},
	"strM": {token.SVAR, 0x0c, "strM"},
	"strS": {token.SVAR, 0x12, "strS"},

	// --- Goto/gosub functions ---
	"goto_on":    {token.GO_LIST, 0, "goto_on"},
	"gosub_on":   {token.GO_LIST, 0, "gosub_on"},
	"goto_case":  {token.GO_CASE, 0, "goto_case"},
	"gosub_case": {token.GO_CASE, 0, "gosub_case"},

	// --- Select functions ---
	"select_w":           {token.SELECT, 0, "select_w"},
	"select":             {token.SELECT, 1, "select"},
	"select_s2":          {token.SELECT, 2, "select_s2"},
	"select_s":           {token.SELECT, 3, "select_s"},
	"select_w2":          {token.SELECT, 10, "select_w2"},
	"select_msgcancel":   {token.SELECT, 11, "select_msgcancel"},
	"select_btncancel":   {token.SELECT, 12, "select_btncancel"},
	"select_btnwkcancel": {token.SELECT, 13, "select_btnwkcancel"},
}
