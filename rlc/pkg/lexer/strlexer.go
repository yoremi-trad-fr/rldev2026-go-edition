// Rich string tokenizer for the Kepago language.
// Transposed from OCaml's rlc/strLexer.ml (~1019 lines).
//
// Handles string literals with embedded control codes:
//
//   Text output codes:
//     \n          newline
//     \r          carriage return
//     \p          page break
//     \"          literal double quote
//     \_          non-breaking space
//     \k<char>    keep character (escaped)
//
//   Formatting codes (with {parameters}):
//     \c{fg}      foreground colour
//     \c{fg,bg}   fore+background colour
//     \size{n}    font size
//     \size{}     reset font size
//     \wait{ms}   wait/pause
//     \i{expr}    integer interpolation
//     \s{var}     string variable interpolation
//     \e{idx}     emoji
//     \em{idx}    emoji (monochrome)
//
//   Position codes:
//     \mv{x,y}    move relative
//     \mvx{x}     move X relative
//     \mvy{y}     move Y relative
//     \pos{x,y}   set absolute position
//     \posx{x}    set X position
//     \posy{y}    set Y position
//
//   Name/speaker codes:
//     \l{idx}     local name variable
//     \m{idx}     global name variable
//     \{...}      name/speaker block (synonym for \name{...})
//     \name{...}  name/speaker block
//
//   Gloss/ruby (furigana) codes:
//     \ruby{base}={gloss}   furigana annotation
//     \g{term}=<key>        glossary term with resource key
//
//   Resource string codes:
//     \a{key}     add/append resource string
//     \a          add anonymous resource
//     \d          delete
//     \res{key}   resource reference
//     \f{code}    inline code rewrite
//
//   String modes:
//     Single (')    single-quoted string
//     Double (")    double-quoted string
//     ResStr (<)    resource string (terminated by < or EOF)
package lexer

import (
	"strings"
	"unicode/utf8"
)

// StrTerminator defines how the string is terminated.
type StrTerminator int

const (
	StrSingle StrTerminator = iota // terminated by '
	StrDouble                      // terminated by "
	StrResStr                      // terminated by < or EOF (resource strings)
)

// StrTokenType identifies the kind of string token.
type StrTokenType int

const (
	STEnd       StrTokenType = iota // end of string
	STText                          // plain text (SBCS or DBCS)
	STSpace                         // whitespace with count
	STDQuote                        // double quote character
	STLLentic                       // 〔 left lenticular bracket
	STRLentic                       // 〕 right lenticular bracket
	STAsterisk                      // ＊ fullwidth asterisk
	STPercent                       // ％ fullwidth percent
	STHyphen                        // - hyphen
	STCode                          // \code{params} control code
	STName                          // \l{idx} or \m{idx} name variable
	STSpeaker                       // \{...} or \name{...} speaker block
	STGloss                         // \ruby{base}={gloss} or \g{term}=<key>
	STRCur                          // } closing brace
	STResRef                        // \res{key} resource reference
	STAdd                           // \a{key} add resource
	STDelete                        // \d delete
	STRewrite                       // \f{code} inline rewrite
)

// StrToken is one token from a rich string.
type StrToken struct {
	Type    StrTokenType
	Text    string // text content
	DBCS    bool   // true if text is double-byte
	Count   int    // space count, or name index
	Ident   string // control code identifier (for STCode)
	IsLocal bool   // for STName: true=\l (local), false=\m (global)
	Params  string // raw parameter text inside {}
	Gloss   string // gloss/ruby text
	GlossID string // gloss resource key
	Line    int    // source line number
}

// StrLexer tokenizes a rich string.
type StrLexer struct {
	src  []rune
	pos  int
	line int
	file string
	term StrTerminator
}

// NewStrLexer creates a string tokenizer.
func NewStrLexer(src string, term StrTerminator, file string, line int) *StrLexer {
	return &StrLexer{
		src:  []rune(src),
		pos:  0,
		line: line,
		file: file,
		term: term,
	}
}

// Line returns the current line number (updated during lexing).
func (sl *StrLexer) Line() int { return sl.line }

// TokenizeAll returns all tokens in the string.
func (sl *StrLexer) TokenizeAll() []StrToken {
	var tokens []StrToken
	for {
		tok := sl.Next()
		if tok.Type == STEnd {
			break
		}
		tokens = append(tokens, tok)
	}
	return tokens
}

// Next returns the next string token.
func (sl *StrLexer) Next() StrToken {
	if sl.pos >= len(sl.src) {
		if sl.term == StrResStr {
			return StrToken{Type: STEnd, Line: sl.line}
		}
		return StrToken{Type: STEnd, Line: sl.line}
	}

	c := sl.src[sl.pos]

	// --- Terminators ---
	if c == '\'' && sl.term == StrSingle {
		sl.pos++
		return StrToken{Type: STEnd, Line: sl.line}
	}
	if c == '"' && sl.term == StrDouble {
		sl.pos++
		return StrToken{Type: STEnd, Line: sl.line}
	}
	if c == '<' && sl.term == StrResStr {
		sl.pos++
		return StrToken{Type: STEnd, Line: sl.line}
	}
	// Non-terminator quote → DQuote token
	if c == '"' {
		sl.pos++
		return StrToken{Type: STDQuote, Line: sl.line}
	}

	// --- Carriage return (skip) ---
	if c == '\r' {
		sl.pos++
		return sl.Next()
	}

	// --- Line break → Space ---
	if c == '\n' || (sl.isSpaceRun() && sl.peekNewline()) {
		sl.skipSpaces()
		sl.skipNewline()
		sl.skipSpaces()
		return StrToken{Type: STSpace, Count: 1, Line: sl.line}
	}

	// --- Block comment {- ... -} (resource strings only) ---
	if c == '{' && sl.peek(1) == '-' && sl.term == StrResStr {
		sl.skipBlockComment()
		return sl.Next()
	}

	// --- Line comment // (resource strings only) ---
	if c == '/' && sl.peek(1) == '/' && sl.term == StrResStr {
		sl.pos += 2
		for sl.pos < len(sl.src) && sl.src[sl.pos] != '\n' {
			sl.pos++
		}
		if sl.pos < len(sl.src) {
			sl.pos++ // skip \n
			sl.line++
		}
		sl.skipSpaces()
		return sl.Next()
	}

	// --- Closing brace ---
	if c == '}' {
		sl.pos++
		return StrToken{Type: STRCur, Line: sl.line}
	}

	// --- Backslash escape / control codes ---
	if c == '\\' {
		return sl.lexEscape()
	}

	// --- Special fullwidth characters ---
	if c == 0x3010 { // 〔
		sl.pos++
		return StrToken{Type: STLLentic, Line: sl.line}
	}
	if c == 0x3011 { // 〕
		sl.pos++
		return StrToken{Type: STRLentic, Line: sl.line}
	}
	if c == 0xFF0A { // ＊
		sl.pos++
		return StrToken{Type: STAsterisk, Line: sl.line}
	}
	if c == 0xFF05 { // ％
		sl.pos++
		return StrToken{Type: STPercent, Line: sl.line}
	}
	if c == '-' {
		sl.pos++
		return StrToken{Type: STHyphen, Line: sl.line}
	}

	// --- Spaces (including \_) ---
	if c == ' ' || c == '\t' || c == 0x3000 {
		return sl.lexSpaces()
	}

	// --- Regular text ---
	return sl.lexText()
}

// lexEscape handles backslash-prefixed tokens.
func (sl *StrLexer) lexEscape() StrToken {
	sl.pos++ // skip '\'
	if sl.pos >= len(sl.src) {
		return StrToken{Type: STText, Text: "\\", Line: sl.line}
	}

	c := sl.src[sl.pos]

	// Line continuation: \ followed by optional \r and \n
	if c == '\r' || c == '\n' {
		if c == '\r' {
			sl.pos++
			if sl.pos < len(sl.src) && sl.src[sl.pos] == '\n' {
				sl.pos++
			}
		} else {
			sl.pos++
		}
		sl.line++
		sl.skipSpaces()
		return sl.Next() // continuation — skip to next token
	}

	// Escaped quote
	if c == '"' {
		sl.pos++
		return StrToken{Type: STDQuote, Line: sl.line}
	}

	// Non-breaking space
	if c == '_' {
		sl.pos++
		// Count consecutive \_ or spaces
		count := 1
		for sl.pos < len(sl.src) {
			if sl.src[sl.pos] == '\\' && sl.pos+1 < len(sl.src) && sl.src[sl.pos+1] == '_' {
				sl.pos += 2
				count++
			} else if sl.src[sl.pos] == ' ' || sl.src[sl.pos] == '\t' || sl.src[sl.pos] == 0x3000 {
				if sl.src[sl.pos] == '\t' || sl.src[sl.pos] == 0x3000 {
					count += 2
				} else {
					count++
				}
				sl.pos++
			} else {
				break
			}
		}
		return StrToken{Type: STSpace, Count: count, Line: sl.line}
	}

	// \k<char> — keep/escaped character
	if c == 'k' {
		sl.pos++
		if sl.pos < len(sl.src) {
			ch := sl.src[sl.pos]
			sl.pos++
			return StrToken{Type: STText, Text: string(ch), DBCS: ch > 0x300, Line: sl.line}
		}
		return StrToken{Type: STText, Text: "", Line: sl.line}
	}

	// \{ or \name{ — speaker/name block
	if c == '{' {
		sl.pos++
		return StrToken{Type: STSpeaker, Line: sl.line}
	}

	// Alphabetic control codes
	if isASCIIAlpha(c) || c == '_' {
		return sl.lexControlCode()
	}

	// Any other escaped character
	sl.pos++
	return StrToken{Type: STText, Text: string(c), DBCS: c > 0x300, Line: sl.line}
}

// lexControlCode parses \ident{params} or \ident:optarg{params}
func (sl *StrLexer) lexControlCode() StrToken {
	// Read identifier
	start := sl.pos
	for sl.pos < len(sl.src) && (isASCIIAlpha(sl.src[sl.pos]) || sl.src[sl.pos] == '_') {
		sl.pos++
	}
	ident := string(sl.src[start:sl.pos])
	sl.skipInlineSpaces()

	// Check for specific control codes
	switch ident {
	case "name":
		// \name { — synonym for \{ speaker block
		if sl.pos < len(sl.src) && sl.src[sl.pos] == '{' {
			sl.pos++
			return StrToken{Type: STSpeaker, Line: sl.line}
		}
		return StrToken{Type: STCode, Ident: ident, Line: sl.line}

	case "l", "m":
		// \l{idx} / \m{idx} — name variables
		isLocal := ident == "l"
		params := sl.readBraceContent()
		return StrToken{Type: STName, IsLocal: isLocal, Params: params, Line: sl.line}

	case "ruby":
		// \ruby{base}={gloss} or \ruby{base}=<key>
		base := sl.readBraceContent()
		sl.skipInlineSpaces()
		gloss := ""
		glossID := ""
		if sl.pos < len(sl.src) && sl.src[sl.pos] == '=' {
			sl.pos++
			sl.skipInlineSpaces()
			if sl.pos < len(sl.src) && sl.src[sl.pos] == '<' {
				sl.pos++
				glossID = sl.readUntilChar('>')
			} else if sl.pos < len(sl.src) && sl.src[sl.pos] == '{' {
				gloss = sl.readBraceContent()
			}
		}
		return StrToken{Type: STGloss, Ident: "ruby", Params: base, Gloss: gloss, GlossID: glossID, Line: sl.line}

	case "g":
		// \g{term}=<key>
		term := sl.readBraceContent()
		sl.skipInlineSpaces()
		glossID := ""
		gloss := ""
		if sl.pos < len(sl.src) && sl.src[sl.pos] == '=' {
			sl.pos++
			sl.skipInlineSpaces()
			if sl.pos < len(sl.src) && sl.src[sl.pos] == '<' {
				sl.pos++
				glossID = sl.readUntilChar('>')
			} else if sl.pos < len(sl.src) && sl.src[sl.pos] == '{' {
				gloss = sl.readBraceContent()
			}
		}
		return StrToken{Type: STGloss, Ident: "g", Params: term, Gloss: gloss, GlossID: glossID, Line: sl.line}

	case "a":
		// \a{key} or \a (anonymous)
		if sl.pos < len(sl.src) && sl.src[sl.pos] == '{' {
			key := sl.readBraceContent()
			return StrToken{Type: STAdd, Params: key, Line: sl.line}
		}
		return StrToken{Type: STAdd, Line: sl.line}

	case "d":
		// \d or \d{}
		if sl.pos < len(sl.src) && sl.src[sl.pos] == '{' {
			sl.readBraceContent() // consume empty {}
		}
		return StrToken{Type: STDelete, Line: sl.line}

	case "res":
		// \res{key}
		key := sl.readBraceContent()
		return StrToken{Type: STResRef, Params: key, Line: sl.line}

	case "f":
		// \f{code} inline rewrite
		code := sl.readBraceContent()
		return StrToken{Type: STRewrite, Params: code, Line: sl.line}
	}

	// Generic control code: \ident:optarg{params} or \ident{params}
	optarg := ""
	if sl.pos < len(sl.src) && sl.src[sl.pos] == ':' {
		sl.pos++
		// Read optional argument until {
		optStart := sl.pos
		for sl.pos < len(sl.src) && sl.src[sl.pos] != '{' && sl.src[sl.pos] != '\n' {
			sl.pos++
		}
		optarg = string(sl.src[optStart:sl.pos])
	}

	params := ""
	if sl.pos < len(sl.src) && sl.src[sl.pos] == '{' {
		params = sl.readBraceContent()
	}

	tok := StrToken{Type: STCode, Ident: ident, Params: params, Line: sl.line}
	if optarg != "" {
		tok.Gloss = optarg // reuse Gloss field for optional argument
	}
	return tok
}

// lexSpaces reads a run of spaces/tabs/fullwidth spaces.
func (sl *StrLexer) lexSpaces() StrToken {
	count := 0
	for sl.pos < len(sl.src) {
		c := sl.src[sl.pos]
		if c == ' ' {
			count++
			sl.pos++
		} else if c == '\t' || c == 0x3000 {
			count += 2 // tabs and fullwidth spaces count as 2
			sl.pos++
		} else if c == '\\' && sl.pos+1 < len(sl.src) && sl.src[sl.pos+1] == '_' {
			count++
			sl.pos += 2
		} else {
			break
		}
	}
	return StrToken{Type: STSpace, Count: count, Line: sl.line}
}

// lexText reads a run of plain text characters.
func (sl *StrLexer) lexText() StrToken {
	start := sl.pos
	dbcs := false
	for sl.pos < len(sl.src) {
		c := sl.src[sl.pos]
		// Stop at special characters
		if c == '\\' || c == '\'' || c == '"' || c == '<' ||
			c == '{' || c == '}' || c == '-' ||
			c == ' ' || c == '\t' || c == '\r' || c == '\n' ||
			c == 0x3000 || c == 0x3010 || c == 0x3011 ||
			c == 0xFF0A || c == 0xFF05 {
			break
		}
		if c >= 0x300 {
			dbcs = true
		}
		sl.pos++
	}
	if sl.pos == start {
		// Single character fallback
		c := sl.src[sl.pos]
		sl.pos++
		return StrToken{Type: STText, Text: string(c), DBCS: c >= 0x300, Line: sl.line}
	}
	return StrToken{Type: STText, Text: string(sl.src[start:sl.pos]), DBCS: dbcs, Line: sl.line}
}

// --- Helper methods ---

func (sl *StrLexer) peek(offset int) rune {
	p := sl.pos + offset
	if p >= len(sl.src) {
		return 0
	}
	return sl.src[p]
}

func (sl *StrLexer) isSpaceRun() bool {
	c := sl.src[sl.pos]
	return c == ' ' || c == '\t' || c == 0x3000
}

func (sl *StrLexer) peekNewline() bool {
	p := sl.pos
	for p < len(sl.src) && (sl.src[p] == ' ' || sl.src[p] == '\t' || sl.src[p] == 0x3000) {
		p++
	}
	if p < len(sl.src) && sl.src[p] == '\r' {
		p++
	}
	return p < len(sl.src) && sl.src[p] == '\n'
}

func (sl *StrLexer) skipSpaces() {
	for sl.pos < len(sl.src) {
		c := sl.src[sl.pos]
		if c == ' ' || c == '\t' || c == 0x3000 {
			sl.pos++
		} else {
			break
		}
	}
}

func (sl *StrLexer) skipInlineSpaces() {
	for sl.pos < len(sl.src) {
		c := sl.src[sl.pos]
		if c == ' ' || c == '\t' || c == 0x3000 {
			sl.pos++
		} else {
			break
		}
	}
}

func (sl *StrLexer) skipNewline() {
	if sl.pos < len(sl.src) && sl.src[sl.pos] == '\r' {
		sl.pos++
	}
	if sl.pos < len(sl.src) && sl.src[sl.pos] == '\n' {
		sl.pos++
		sl.line++
	}
}

func (sl *StrLexer) skipBlockComment() {
	sl.pos += 2 // skip {-
	for sl.pos < len(sl.src) {
		if sl.src[sl.pos] == '-' && sl.pos+1 < len(sl.src) && sl.src[sl.pos+1] == '}' {
			sl.pos += 2
			return
		}
		if sl.src[sl.pos] == '\n' {
			sl.line++
		}
		sl.pos++
	}
}

// readBraceContent reads text between { and }, handling nested braces.
// Assumes the opening { is the next character (or already consumed for some codes).
func (sl *StrLexer) readBraceContent() string {
	if sl.pos >= len(sl.src) || sl.src[sl.pos] != '{' {
		return ""
	}
	sl.pos++ // skip {
	var sb strings.Builder
	depth := 1
	for sl.pos < len(sl.src) && depth > 0 {
		c := sl.src[sl.pos]
		if c == '{' {
			depth++
			if depth > 1 {
				sb.WriteRune(c)
			}
		} else if c == '}' {
			depth--
			if depth > 0 {
				sb.WriteRune(c)
			}
		} else {
			if c == '\n' {
				sl.line++
			}
			sb.WriteRune(c)
		}
		sl.pos++
	}
	return sb.String()
}

// readUntilChar reads until the given character, consuming it.
func (sl *StrLexer) readUntilChar(end rune) string {
	var sb strings.Builder
	for sl.pos < len(sl.src) && sl.src[sl.pos] != end {
		if sl.src[sl.pos] == '\n' {
			sl.line++
		}
		sb.WriteRune(sl.src[sl.pos])
		sl.pos++
	}
	if sl.pos < len(sl.src) {
		sl.pos++ // skip end char
	}
	return sb.String()
}

func isASCIIAlpha(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}

// Ensure utf8 import is used
var _ = utf8.RuneError
