// Package token defines token types for the Kepago language compiler (rlc).
// Transposed from OCaml's rlc/strTokens.ml (236 lines) + keAstParser.mly
// token declarations + keAst.ml strtoken types.
//
// Two separate token systems coexist:
//
//  1. Parser tokens (Type): used by the lexer/parser pipeline to tokenize
//     and parse .org source files into AST.
//
//  2. String tokens (StrTokenKind): used inside string literals to represent
//     rich text with embedded control codes, name variables, furigana, etc.
//     These appear inside `Str expressions in the AST.
package token

import (
	"fmt"
	"strings"
)

// ============================================================
// Part 1: Parser token types (from keAstParser.mly)
// ============================================================

// Type is a parser token type.
type Type int

const (
	// Special
	EOF Type = iota
	INTEGER // int32 literal
	STRING  // string literal (rich tokens)
	LABEL   // @label
	IDENT   // identifier

	// Punctuation
	LPAR  // (
	RPAR  // )
	LSQU  // [
	RSQU  // ]
	LCUR  // {
	RCUR  // }
	COLON // :
	SEMI  // ;
	COMMA // ,
	POINT // .
	ARROW // ->

	// Compound assignment operators
	SADD // +=
	SSUB // -=
	SMUL // *=
	SDIV // /=
	SMOD // %=
	SAND // &=
	SOR  // |=
	SXOR // ^=
	SSHL // <<=
	SSHR // >>=
	SET  // =

	// Arithmetic / logic operators
	ADD   // +
	SUB   // -
	MUL   // *
	DIV   // /
	MOD   // %
	AND   // &
	OR    // |
	XOR   // ^
	SHL   // <<
	SHR   // >>
	EQU   // ==
	NEQ   // !=
	LTE   // <=
	LTN   // <
	GTE   // >=
	GTN   // >
	LAND  // &&
	LOR   // ||
	NOT   // !
	TILDE // ~

	// Statement keywords
	DEOF     // eof
	DHALT    // halt
	RETURN   // return
	OP       // op
	USCORE   // _
	IF       // if
	ELSE     // else
	WHILE    // while
	REPEAT   // repeat
	TILL     // till
	FOR      // for
	CASE     // case
	OF       // of
	OTHER    // other
	ECASE    // ecase
	BREAK    // break
	CONTINUE // continue
	RAW      // raw
	ENDRAW   // endraw

	// Type keywords
	INT // int (also bit, bit2, bit4, byte → width in IntVal)
	STR // str

	// Directive keywords
	DDEFINE   // #define, #sdefine, #const, #bind, #ebind, #redef
	DUNDEF    // #undef
	DSET      // #set
	DTARGET   // #target
	DVERSION  // #version
	DLOAD     // #load
	DIF       // #if
	DIFDEF    // #ifdef (bool: true=ifdef, false=ifndef)
	DELSE     // #else
	DELSEIF   // #elseif
	DENDIF    // #endif
	DFOR      // #for
	DINLINE   // #inline, #sinline
	DHIDING   // #hiding
	DWITHEXPR // #file, #resource, #entrypoint, #character, etc.
	DRES      // #res<key>

	// Variable registers
	VAR  // intA[...] through intZ[...], intL[...] etc. (IntVal = bank)
	SVAR // strS[...], strK[...], strM[...] (IntVal = bank)
	REG  // store (IntVal = 0xc8)

	// Goto/select function tokens
	GOTO    // goto, gosub (with label target)
	GO_LIST // goto_on, gosub_on (dispatch by index)
	GO_CASE // goto_case, gosub_case (switch dispatch)
	SELECT  // select, select_w, select_s, etc. (IntVal = opcode)

	// Special builtins
	SPECIAL // $s, $pause
)

// Token is one lexical token with associated data.
type Token struct {
	Type   Type
	IntVal int32  // for INTEGER, INT (bit width), VAR/SVAR/REG (bank), SELECT (opcode)
	StrVal string // for IDENT, LABEL, STRING text, GOTO name, GO_LIST/GO_CASE name, DWITHEXPR name
	Line   int    // source line number
	File   string // source file name
}

var typeNames = map[Type]string{
	EOF: "EOF", INTEGER: "INTEGER", STRING: "STRING", LABEL: "LABEL", IDENT: "IDENT",
	LPAR: "(", RPAR: ")", LSQU: "[", RSQU: "]", LCUR: "{", RCUR: "}",
	COLON: ":", SEMI: ";", COMMA: ",", POINT: ".", ARROW: "->",
	SADD: "+=", SSUB: "-=", SMUL: "*=", SDIV: "/=", SMOD: "%=",
	SAND: "&=", SOR: "|=", SXOR: "^=", SSHL: "<<=", SSHR: ">>=", SET: "=",
	ADD: "+", SUB: "-", MUL: "*", DIV: "/", MOD: "%",
	AND: "&", OR: "|", XOR: "^", SHL: "<<", SHR: ">>",
	EQU: "==", NEQ: "!=", LTE: "<=", LTN: "<", GTE: ">=", GTN: ">",
	LAND: "&&", LOR: "||", NOT: "!", TILDE: "~",
	DEOF: "eof", DHALT: "halt", RETURN: "return", OP: "op", USCORE: "_",
	IF: "if", ELSE: "else", WHILE: "while", REPEAT: "repeat", TILL: "till",
	FOR: "for", CASE: "case", OF: "of", OTHER: "other", ECASE: "ecase",
	BREAK: "break", CONTINUE: "continue", RAW: "raw", ENDRAW: "endraw",
	INT: "int", STR: "str",
	DDEFINE: "#define", DUNDEF: "#undef", DSET: "#set",
	DTARGET: "#target", DVERSION: "#version", DLOAD: "#load",
	DIF: "#if", DIFDEF: "#ifdef", DELSE: "#else", DELSEIF: "#elseif", DENDIF: "#endif",
	DFOR: "#for", DINLINE: "#inline", DHIDING: "#hiding", DWITHEXPR: "#directive",
	DRES: "#res",
	VAR: "VAR", SVAR: "SVAR", REG: "REG",
	GOTO: "GOTO", GO_LIST: "GO_LIST", GO_CASE: "GO_CASE", SELECT: "SELECT",
	SPECIAL: "SPECIAL",
}

func (t Type) String() string {
	if s, ok := typeNames[t]; ok {
		return s
	}
	return fmt.Sprintf("Token(%d)", int(t))
}

// IsAssignOp returns true for compound assignment operators (+=, -=, etc.).
func (t Type) IsAssignOp() bool {
	return t >= SADD && t <= SET
}

// DefineKind distinguishes #define variants carried by DDEFINE tokens.
type DefineKind int

const (
	Define       DefineKind = iota // #define
	DefineScoped                   // #sdefine
	Redefine                       // #redef
	Const                          // #const
	Bind                           // #bind
	EBind                          // #ebind
)

// ============================================================
// Part 2: String token types (from keAst.ml strtoken)
// ============================================================

// StrTokenKind identifies the type of a rich string token.
type StrTokenKind int

const (
	StrEOS      StrTokenKind = iota // end of string
	StrText                         // plain text (SBCS or DBCS)
	StrDQuote                       // double-quote character
	StrRCur                         // } closing brace
	StrLLentic                      // 【 U+3010
	StrRLentic                      // 】 U+3011
	StrAsterisk                     // ＊ U+FF0A
	StrPercent                      // ％ U+FF05
	StrHyphen                       // -
	StrSpace                        // whitespace (Count = number of spaces)
	StrSpeaker                      // \{...} or \name{...} — name/speaker block
	StrName                         // \l{idx} or \m{idx} — name variable
	StrGloss                        // \ruby{base}={gloss} or \g{term}=<key>
	StrCode                         // \ident:opt{params} — control code
	StrAdd                          // \a{key} — add resource string
	StrDelete                       // \d — delete
	StrResRef                       // \res{key} — resource reference
	StrRewrite                      // \f{code} — inline code rewrite
)

// TextEnc distinguishes single-byte vs double-byte text.
type TextEnc int

const (
	SBCS TextEnc = iota // single-byte character set (ASCII/Latin)
	DBCS                // double-byte character set (CJK)
)

// GlossKind distinguishes gloss from ruby.
type GlossKind int

const (
	Gloss GlossKind = iota // \g{term}=<key> glossary
	Ruby                   // \ruby{base}={gloss} furigana
)

// NameScope distinguishes local from global name variables.
type NameScope int

const (
	NameLocal  NameScope = iota // \l{idx}
	NameGlobal                  // \m{idx}
)

// StrToken is one token inside a rich string literal.
type StrToken struct {
	Kind    StrTokenKind
	Enc     TextEnc   // for StrText
	Text    string    // text content (StrText, StrCode ident)
	Count   int       // space count (StrSpace), rewrite key (StrRewrite)
	Scope   NameScope // for StrName
	GlossK  GlossKind // for StrGloss
	Params  string    // raw parameter string inside {}
	GlossID string    // resource key for gloss (StrGloss)
	Line    int
	File    string
}

// ============================================================
// Part 3: String token utilities (from strTokens.ml)
// ============================================================

// MakeName builds a Shift_JIS-encoded name reference string.
// Matches OCaml: make_name s i
// Local names use "\x81\x93" prefix, global use "\x81\x96".
// The index i maps to Shift_JIS fullwidth letters:
//   i < 26  → single letter: \x82 + (i + 0x60)
//   i >= 26 → two letters:   \x82 + (i/26 + 0x5f), \x82 + (i%26 + 0x60)
func MakeName(scope NameScope, index int) string {
	var prefix string
	if scope == NameLocal {
		prefix = "\x81\x93"
	} else {
		prefix = "\x81\x96"
	}
	if index < 26 {
		return fmt.Sprintf("%s\x82%c", prefix, rune(index+0x60))
	}
	return fmt.Sprintf("%s\x82%c\x82%c", prefix, rune(index/26+0x5f), rune(index%26+0x60))
}

// IsOutputCode returns true for control codes that produce text output (\i, \s).
func IsOutputCode(ident string) bool {
	return ident == "i" || ident == "s"
}

// IsObjectCode returns true for control codes that produce formatting objects.
func IsObjectCode(ident string) bool {
	switch ident {
	case "r", "n", "c", "size", "pos", "posx", "posy":
		return true
	}
	return false
}

// ObjectCodeString converts a formatting control code to its bytecode string.
// For example: \r → "#D", \size{24} → "#S24##", \c{255} → "#C255##"
// Returns an error string if the code/params combination is invalid.
func ObjectCodeString(ident string, params []int32) (string, error) {
	switch ident {
	case "r", "n":
		if len(params) != 0 {
			return "", fmt.Errorf("\\%s takes no parameters", ident)
		}
		return "#D", nil
	case "size":
		if len(params) == 0 {
			return "#S##", nil
		}
		if len(params) == 1 {
			return fmt.Sprintf("#S%d##", params[0]), nil
		}
		return "", fmt.Errorf("too many parameters to \\size")
	case "c":
		if len(params) == 0 {
			return "#C##", nil
		}
		if len(params) == 1 {
			return fmt.Sprintf("#C%d##", params[0]), nil
		}
		return "", fmt.Errorf("too many parameters to \\c")
	case "posx":
		if len(params) == 1 {
			return fmt.Sprintf("#X%d##", params[0]), nil
		}
		return "", fmt.Errorf("\\posx requires exactly 1 parameter")
	case "posy":
		if len(params) == 1 {
			return fmt.Sprintf("#Y%d##", params[0]), nil
		}
		return "", fmt.Errorf("\\posy requires exactly 1 parameter")
	case "pos":
		if len(params) == 1 {
			return fmt.Sprintf("#X%d##", params[0]), nil
		}
		if len(params) == 2 {
			return fmt.Sprintf("#X%d#Y%d##", params[0], params[1]), nil
		}
		return "", fmt.Errorf("\\pos requires 1 or 2 parameters")
	}
	return "", fmt.Errorf("unknown object code \\%s", ident)
}

// TokensToString converts a slice of StrTokens to a plain string.
// This is the Go equivalent of OCaml's StrTokens.to_string.
// It handles text, spaces, special characters, name references,
// and constant-foldable control codes.
//
// The quote parameter controls whether the output is quoted for
// use in bytecode (adds surrounding quotes if needed).
func TokensToString(tokens []StrToken, quote bool) string {
	var sb strings.Builder
	needsQuotes := false

	for _, tok := range tokens {
		switch tok.Kind {
		case StrEOS:
			// skip

		case StrDQuote:
			if quote {
				// Should not happen in quoted context
				sb.WriteByte('"')
			} else {
				sb.WriteByte('"')
			}

		case StrRCur:
			needsQuotes = true
			sb.WriteByte('}')

		case StrLLentic:
			sb.WriteString("\x81\x79") // Shift_JIS for 【

		case StrRLentic:
			sb.WriteString("\x81\x7a") // Shift_JIS for 】

		case StrAsterisk:
			sb.WriteString("\x81\x96") // Shift_JIS for ＊

		case StrPercent:
			sb.WriteString("\x81\x93") // Shift_JIS for ％

		case StrHyphen:
			needsQuotes = true
			sb.WriteByte('-')

		case StrSpace:
			needsQuotes = true
			for i := 0; i < tok.Count; i++ {
				sb.WriteByte(' ')
			}

		case StrText:
			sb.WriteString(tok.Text)

		case StrName:
			// Name variable reference → encoded as Shift_JIS name
			// The index would need to be resolved from the Params field
			// For static conversion, we encode directly
			sb.WriteString(MakeName(tok.Scope, tok.Count))

		case StrSpeaker, StrGloss, StrAdd, StrDelete, StrResRef, StrRewrite:
			// These are invalid in constant string context
			// In a full compiler, this would be an error

		case StrCode:
			if IsObjectCode(tok.Text) {
				// Object codes produce formatting markers
				s, err := ObjectCodeString(tok.Text, nil)
				if err == nil {
					sb.WriteString(s)
				}
			}
			// Output codes (\i, \s) would need constant evaluation
			needsQuotes = true
		}
	}

	if quote && needsQuotes {
		return fmt.Sprintf("\"%s\"", sb.String())
	}
	return sb.String()
}
