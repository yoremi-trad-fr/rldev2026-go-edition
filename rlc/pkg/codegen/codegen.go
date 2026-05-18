// Package codegen generates RealLive bytecode from compiler IR.
//
// Transposed from OCaml:
//   - rlc/codegen.ml (144 lines)       — expression encoding, opcode encoding, Output IR buffer
//   - rlc/bytecodeGen.ml (267 lines)   — binary file generation for RealLive/AVG2000
//
// The codegen pipeline has two stages:
//   1. IR construction: the compiler emits IR elements (Code, Label, LabelRef,
//      Entrypoint, Kidoku, Lineref) into an Output buffer.
//   2. Binary generation: Generate() traverses the IR to compute label positions,
//      build the bytecode buffer, optionally compress, and produce the final
//      .TXT (SEEN) file in RealLive or AVG2000 format.
//
// Bytecode encoding (RealLive expressions):
//   - Integers: $\xff followed by 4 LE bytes
//   - Variables: $<bank>[<expr>]  (bank = register id byte)
//   - Store register: $\xc8
//   - Operators: \<opbyte>  between operands
//   - Opcodes: #<type><module><code_lo><code_hi><argc_lo><argc_hi><overload>
package codegen

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/yoremi/rldev-go/pkg/diag"
	"github.com/yoremi/rldev-go/pkg/text"
	"github.com/yoremi/rldev-go/pkg/texttransforms"
	"github.com/yoremi/rldev-go/rlc/pkg/ast"
	"github.com/yoremi/rldev-go/rlc/pkg/kfn"
)

// ============================================================
// Expression encoding (from codegen.ml)
// ============================================================

// Bytecodes for arithmetic operators in expressions.
func OpCode(op ast.ArithOp) byte {
	switch op {
	case ast.OpAdd: return 0x00
	case ast.OpSub: return 0x01
	case ast.OpMul: return 0x02
	case ast.OpDiv: return 0x03
	case ast.OpMod: return 0x04
	case ast.OpAnd: return 0x05
	case ast.OpOr:  return 0x06
	case ast.OpXor: return 0x07
	case ast.OpShl: return 0x08
	case ast.OpShr: return 0x09
	}
	return 0x00
}

// Bytecodes for comparison operators.
func CmpCode(op ast.CmpOp) byte {
	switch op {
	case ast.CmpEqu: return 0x28
	case ast.CmpNeq: return 0x29
	case ast.CmpLte: return 0x2a
	case ast.CmpLtn: return 0x2b
	case ast.CmpGte: return 0x2c
	case ast.CmpGtn: return 0x2d
	}
	return 0x28
}

// Bytecodes for short-circuit logical operators.
func ChainCode(op ast.ChainOp) byte {
	switch op {
	case ast.ChainAnd: return 0x3c
	case ast.ChainOr:  return 0x3d
	}
	return 0x3c
}

// Bytecodes for assignment operators.
func AssignCode(op ast.AssignOp) byte {
	switch op {
	case ast.AssignAdd: return 0x14
	case ast.AssignSub: return 0x15
	case ast.AssignMul: return 0x16
	case ast.AssignDiv: return 0x17
	case ast.AssignMod: return 0x18
	case ast.AssignAnd: return 0x19
	case ast.AssignOr:  return 0x1a
	case ast.AssignXor: return 0x1b
	case ast.AssignShl: return 0x1c
	case ast.AssignShr: return 0x1d
	case ast.AssignSet: return 0x1e
	}
	return 0x1e
}

// EncodeInt32 encodes an integer literal as $\xff + 4 LE bytes.
func EncodeInt32(v int32) []byte {
	buf := make([]byte, 6)
	buf[0] = '$'
	buf[1] = 0xff
	binary.LittleEndian.PutUint32(buf[2:], uint32(v))
	return buf
}

// EncodeInt16 encodes a 16-bit value as 2 LE bytes.
func EncodeInt16(v int) []byte {
	buf := make([]byte, 2)
	binary.LittleEndian.PutUint16(buf, uint16(v))
	return buf
}

// EncodeOpcode encodes an opcode header: #<type><module><code16><argc16><overload>
func EncodeOpcode(opType, opModule, opCode, argc, overload int) []byte {
	buf := make([]byte, 8)
	buf[0] = '#'
	buf[1] = byte(opType)
	buf[2] = byte(opModule)
	binary.LittleEndian.PutUint16(buf[3:5], uint16(opCode))
	binary.LittleEndian.PutUint16(buf[5:7], uint16(argc))
	buf[7] = byte(overload)
	return buf
}

// ============================================================
// Intermediate Representation (IR)
// ============================================================

// IRType identifies the kind of IR element.
type IRType int

const (
	IRCode       IRType = iota // raw bytecode bytes
	IRLabel                    // label definition (zero-width)
	IRLabelRef                 // 4-byte label reference (resolved during Generate)
	IREntrypoint               // entrypoint marker + kidoku
	IRKidoku                   // kidoku (read-flag) marker
	IRLineref                  // line number reference (for debug info)
)

// IR is one element of the intermediate representation.
type IR struct {
	Type  IRType
	Bytes []byte // for IRCode
	Label string // for IRLabel, IRLabelRef
	Index int    // for IREntrypoint, IRKidoku, IRLineref
	Loc   ast.Loc
}

// Output accumulates IR elements during compilation.
type Output struct {
	IR      []IR
	labels  map[string]bool // tracks defined label names (for duplicate detection)
	lastLine int

	// ResolveRes, when non-nil, is invoked to resolve a `#res<KEY>`
	// reference to its expanded string. The compiler frame wires this
	// to State.Resources after parsing the source. Returning ok=false
	// causes EmitExpr to emit a textual fallback that the engine will
	// reject but won't desync the reader.
	ResolveRes func(key string) (string, bool)
}

// NewOutput creates a fresh IR output buffer.
func NewOutput() *Output {
	return &Output{
		labels: make(map[string]bool),
	}
}

// AddCode appends raw bytecode.
func (o *Output) AddCode(loc ast.Loc, code []byte) {
	o.maybeLine(loc)
	o.IR = append(o.IR, IR{Type: IRCode, Bytes: code, Loc: loc})
}

// AddCodeStr appends raw bytecode from a string.
func (o *Output) AddCodeStr(loc ast.Loc, s string) {
	o.AddCode(loc, []byte(s))
}

// AddLabel defines a label at the current position.
func (o *Output) AddLabel(name string, loc ast.Loc) error {
	if o.labels[name] {
		return fmt.Errorf("%s: @%s already defined; label identifiers must be unique", loc, name)
	}
	o.IR = append(o.IR, IR{Type: IRLabel, Label: name, Loc: loc})
	o.labels[name] = true
	return nil
}

// AddLabelRef emits a 4-byte forward reference to a label.
func (o *Output) AddLabelRef(name string, loc ast.Loc) {
	o.IR = append(o.IR, IR{Type: IRLabelRef, Label: name, Loc: loc})
}

// AddEntrypoint emits an entrypoint marker.
func (o *Output) AddEntrypoint(index int) {
	o.IR = append(o.IR, IR{Type: IREntrypoint, Index: index})
}

// AddKidoku emits a kidoku (read-flag) marker.
func (o *Output) AddKidoku(loc ast.Loc, line int) {
	o.maybeLine(loc)
	o.IR = append(o.IR, IR{Type: IRKidoku, Index: line, Loc: loc})
}

// Length returns the number of IR elements.
func (o *Output) Length() int { return len(o.IR) }

// maybeLine emits a line-number reference (IRLineref) when entering a
// new source line, matching OCaml's add_line/maybe_line (codegen.ml
// L98-115). Visual Art's compiler emits these `\x0a <line:u16>` markers
// throughout the bytecode; reproducing them keeps our bytecode close to
// byte-for-byte parity with the reference compiler.
func (o *Output) maybeLine(loc ast.Loc) {
	if loc == ast.Nowhere || loc.Line == o.lastLine {
		return
	}
	o.IR = append(o.IR, IR{Type: IRLineref, Index: loc.Line, Loc: loc})
	o.lastLine = loc.Line
}

// --- Expression emission helpers ---

// EmitExpr encodes an expression into bytecode and appends it to the output.
// encodeText wraps texttransforms.ToBytecode so unmappable code
// points become visible diagnostics. The first Go port treated the
// underlying encoder's silent space substitution as success, so a
// translator's character that didn't exist in CP932 was lost without
// warning — the kind of silent corruption that produces a SEEN.TXT
// the engine refuses to boot. This wrapper:
//
//  1. Resets bad-rune tracking (per call, per Loc).
//  2. Runs the encoder.
//  3. Emits one diag.Warning per distinct offending rune with the
//     OCaml wording "cannot represent U+%04X in RealLive bytecode".
//
// The encoder's bytes are returned unchanged so the bytecode stream
// stays balanced; with ForceEncode (the default in compile mode)
// the substituted spaces are kept, with the warnings making the
// loss explicit.
func (o *Output) encodeText(loc ast.Loc, s string) ([]byte, error) {
	texttransforms.ResetBadChars()
	b, err := texttransforms.ToBytecode(text.Text([]rune(s)))
	for _, r := range texttransforms.BadRunes() {
		diag.Warning(diag.Loc{File: loc.File, Line: loc.Line},
			"cannot represent U+%04X %q in RealLive bytecode", r, string(r))
	}
	return b, err
}

func (o *Output) EmitExpr(e ast.Expr) {
	switch x := e.(type) {
	case ast.IntLit:
		o.AddCode(x.Loc, EncodeInt32(x.Val))
	case ast.StoreRef:
		o.AddCode(x.Loc, []byte{'$', 0xc8})
	case ast.IntVar:
		o.AddCode(x.Loc, []byte{'$', byte(x.Bank), '['})
		o.EmitExpr(x.Index)
		o.AddCode(x.Loc, []byte{']'})
	case ast.StrVar:
		o.AddCode(x.Loc, []byte{'$', byte(x.Bank), '['})
		o.EmitExpr(x.Index)
		o.AddCode(x.Loc, []byte{']'})
	case ast.BinOp:
		o.EmitExpr(x.LHS)
		o.AddCode(x.Loc, []byte{'\\', OpCode(x.Op)})
		o.EmitExpr(x.RHS)
	case ast.CmpExpr:
		o.EmitExpr(x.LHS)
		o.AddCode(x.Loc, []byte{'\\', CmpCode(x.Op)})
		o.EmitExpr(x.RHS)
	case ast.ChainExpr:
		o.EmitExpr(x.LHS)
		o.AddCode(x.Loc, []byte{'\\', ChainCode(x.Op)})
		o.EmitExpr(x.RHS)
	case ast.UnaryExpr:
		if x.Op == ast.UnarySub {
			o.AddCode(x.Loc, []byte{'\\', OpCode(ast.OpSub)})
			o.EmitExpr(x.Val)
		}
		// Other unary ops should have been transformed to binary by expr normalization
	case ast.ParenExpr:
		o.AddCode(x.Loc, []byte{'('})
		o.EmitExpr(x.Expr)
		o.AddCode(x.Loc, []byte{')'})
	case ast.StrLit:
		// String literals are inlined in the bytecode as raw encoded
		// bytes — there's no length prefix or terminator; the closing
		// ')' of the surrounding parameter list serves as the delimiter.
		// Reference: OCaml strTokens.ml to_string + TextTransforms.to_bytecode.
		// Without this case, every `SetLocalName(0, '〔Nom〕')`,
		// `strcpy(strS[…], 'foo')`, and other string parameter was
		// silently dropped, leaving the bytecode 30-40 % too short.
		bytes, err := o.encodeStrLit(x)
		if err == nil {
			o.AddCode(x.Loc, bytes)
		} else {
			// Best-effort: emit an empty quoted pair so the param list
			// stays balanced. Bytecode will still be wrong but the
			// reader won't desync past this opcode.
			o.AddCode(x.Loc, []byte{'"', '"'})
		}
	case ast.ResRef:
		// #res<KEY> is a deferred reference to a string defined in the
		// .sjs / .utf companion file. The resource text contains rich
		// markers — \{Name}, \m{B}, \l{A}, 【 】, ＊, ％, \e{N}, etc. —
		// that the RealLive engine expects to see encoded as specific
		// SJIS byte sequences (lenticulars 0x81 0x79 / 0x81 0x7a, etc.)
		// surrounded by quote-mode transitions (0x22). Naively
		// concatenating the .utf text and transcoding to SJIS produces
		// literal `\{` ASCII bytes (0x5c 0x7b) that the engine doesn't
		// recognise, causing it to crash on launch (0xC0000005). We
		// therefore tokenise the resource text and run the same quote
		// state machine OCaml's textout.ml `compile_stub` uses
		// (textout.ml L334-347).
		if o.ResolveRes != nil {
			if t, ok := o.ResolveRes(x.Key); ok {
				b, err := o.encodeResourceText(x.Loc, t)
				if err == nil {
					o.AddCode(x.Loc, b)
					break
				}
			}
		}
		o.AddCode(x.Loc, []byte{'"', '"'})
	}
}

// encodeResourceText tokenises a resolved resource string and emits the
// SJIS bytecode the engine expects, matching OCaml strTokens.ml `to_string`
// (the function used when a #res<> resolves into an argument-list slot).
//
// Critical difference vs compile_stub (textout.ml): to_string does NOT
// emit a leading quote, and it does NOT wrap the whole sequence in
// quotes. It only inserts `"` around \{Speaker} / } RCur transitions,
// because those need to switch the engine's quote state. Plain text and
// lenticulars are emitted as raw SJIS bytes — no surrounding quotes.
//
// Examples (Clannad bytecode):
//
//   `title (#res<0000>)`  where res = "渚・後日"
//     →  ( SJIS-bytes )      no quotes
//
//   `SetLocalName(0, #res<0001>)` where res = "\{美佐枝}「お…」"
//     →  ( "" 81 79 " SJIS " 81 7a " 81 75 SJIS 81 76 " )
//
// The previous implementation called setQuotes(true) at the start,
// producing `( "SJIS" )` for the simple case — the engine sees an
// unexpected `"` byte and reads garbage past the closing `)`, leading to
// gradual desync and crashes / black screen.
//
// Marker → byte table (textout.ml L334-347 / strTokens.ml L157-171):
//
//	\{ (Speaker)   → 0x81 0x79  (set_quotes false)
//	}  (RCur)      → 0x81 0x7a  (set_quotes false)
//	【 (LLentic)    → 0x81 0x79  (plain SJIS char in to_string)
//	】 (RLentic)    → 0x81 0x7a
//	＊ (Asterisk)   → 0x81 0x96
//	％ (Percent)    → 0x81 0x93
//	\l{X} (Name)   → 0x81 0x93 0x82 (X+0x1f)        (local)
//	\m{X} (Name)   → 0x81 0x96 0x82 (X+0x1f)        (global)
//	-  (Hyphen)    → '-'
//	"  (DQuote)    → '"' raw
//	text           → plain SJIS
func (o *Output) encodeResourceText(loc ast.Loc, text string) ([]byte, error) {
	tokens, err := lexResourceText(text)
	if err != nil {
		return nil, err
	}

	var buf []byte
	quoted := false
	setQuotes := func(q bool) {
		if quoted != q {
			quoted = q
			buf = append(buf, '"')
		}
	}

	for _, tk := range tokens {
		switch tk.kind {
		case rtText:
			b, err := o.encodeText(loc, tk.text)
			if err != nil {
				return nil, err
			}
			buf = append(buf, b...)
		case rtSpace:
			for i := 0; i < tk.count; i++ {
				buf = append(buf, ' ')
			}
		case rtDQuote:
			buf = append(buf, '"')
		case rtSpeaker:
			// \{Name} opens a speaker block. OCaml emits a 0x22
			// transition byte before the lenticular bytes so the engine
			// switches out of any text-quoting state it was in.
			setQuotes(false)
			buf = append(buf, 0x81, 0x79)
		case rtRCur:
			// } closes the speaker block. Same quote-state transition.
			setQuotes(false)
			buf = append(buf, 0x81, 0x7a)
		case rtLLentic:
			buf = append(buf, 0x81, 0x79)
		case rtRLentic:
			buf = append(buf, 0x81, 0x7a)
		case rtAsterisk:
			buf = append(buf, 0x81, 0x96)
		case rtPercent:
			buf = append(buf, 0x81, 0x93)
		case rtHyphen:
			buf = append(buf, '-')
		case rtName:
			setQuotes(false)
			if tk.isLocal {
				buf = append(buf, 0x81, 0x93)
			} else {
				buf = append(buf, 0x81, 0x96)
			}
			for _, c := range tk.letters {
				buf = append(buf, 0x82, byte(c-'A')+0x60)
			}
			if tk.hasIndex {
				buf = append(buf, 0x82, byte(tk.index)+0x4f)
			}
		}
	}

	// Close any pending quoted run — if we ended inside quote mode,
	// emit the matching closing 0x22.
	if quoted {
		buf = append(buf, '"')
	}
	return buf, nil
}

// rtKind / rtToken are a minimal token representation used by
// encodeResourceText. We can't reuse ast.StrToken here because nothing
// in the production pipeline currently materialises SpeakerToken /
// NameToken / lenticular tokens — see compileTextStub in compilerframe
// which is set up for them but never sees them. Going through a small
// internal token type avoids a dependency on the upstream string
// lexer's full token model.
type rtKind int

const (
	rtText rtKind = iota
	rtSpace
	rtDQuote
	rtSpeaker
	rtRCur
	rtLLentic
	rtRLentic
	rtAsterisk
	rtPercent
	rtHyphen
	rtName
)

type rtToken struct {
	kind     rtKind
	text     string // for rtText
	count    int    // for rtSpace
	isLocal  bool   // for rtName: true = \l, false = \m
	letters  string // for rtName: 1-2 uppercase letters A..Z
	hasIndex bool   // for rtName
	index    int    // for rtName: 0..9
}

// lexResourceText breaks a resource string into tokens consumable by
// encodeResourceText. The grammar mirrors what the disassembler emits
// into .utf files (textout.ml `unquot` rules):
//
//	\{                          → Speaker
//	\l{X}, \l{XX}, \l{X, N}…    → Name (local)
//	\m{X}, \m{XX}, \m{X, N}…    → Name (global)
//	【                          → LLentic   (UTF-8: e3 80 90)
//	】                          → RLentic   (UTF-8: e3 80 91)
//	＊                          → Asterisk  (UTF-8: ef bc 8a)
//	％                          → Percent   (UTF-8: ef bc 85)
//	}                           → RCur
//	-                           → Hyphen    (kept as plain char; OCaml only
//	                              re-emits as raw '-' so we let it fall
//	                              through to TextToken — recompile sees a
//	                              regular dash, identical bytecode)
//	"                           → DQuote
//	otherwise                   → Text (chunk until next marker)
func lexResourceText(s string) ([]rtToken, error) {
	var out []rtToken
	var buf []rune
	flush := func() {
		if len(buf) == 0 {
			return
		}
		out = append(out, rtToken{kind: rtText, text: string(buf)})
		buf = buf[:0]
	}
	r := []rune(s)
	i := 0
	for i < len(r) {
		c := r[i]

		// Backslash escapes: \{ \l \m
		if c == '\\' && i+1 < len(r) {
			next := r[i+1]
			switch next {
			case '{':
				flush()
				out = append(out, rtToken{kind: rtSpeaker})
				i += 2
				continue
			case 'l', 'm':
				// Expect \l{X} or \l{XX} or \l{X, N} (and same for \m)
				if i+2 < len(r) && r[i+2] == '{' {
					tok, consumed, ok := parseNameToken(r, i)
					if ok {
						flush()
						out = append(out, tok)
						i += consumed
						continue
					}
				}
			}
			// Unknown backslash escape: emit literal '\' as text. The
			// disassembler shouldn't produce these, but tolerate
			// gracefully.
			buf = append(buf, c)
			i++
			continue
		}

		switch c {
		case '"':
			flush()
			out = append(out, rtToken{kind: rtDQuote})
			i++
			continue
		case '}':
			flush()
			out = append(out, rtToken{kind: rtRCur})
			i++
			continue
		case 0x3010: // 【
			flush()
			out = append(out, rtToken{kind: rtLLentic})
			i++
			continue
		case 0x3011: // 】
			flush()
			out = append(out, rtToken{kind: rtRLentic})
			i++
			continue
		case 0xff0a: // ＊ fullwidth asterisk
			flush()
			out = append(out, rtToken{kind: rtAsterisk})
			i++
			continue
		case 0xff05: // ％ fullwidth percent
			flush()
			out = append(out, rtToken{kind: rtPercent})
			i++
			continue
		}

		buf = append(buf, c)
		i++
	}
	flush()
	return out, nil
}

// parseNameToken parses `\l{X}`, `\l{XX}`, `\l{X, N}`, `\l{XX, N}` and
// the same for `\m`. Starts at the backslash. Returns the token, the
// number of runes consumed and true on success.
func parseNameToken(r []rune, start int) (rtToken, int, bool) {
	if start+2 >= len(r) {
		return rtToken{}, 0, false
	}
	if r[start] != '\\' {
		return rtToken{}, 0, false
	}
	isLocal := r[start+1] == 'l'
	if r[start+2] != '{' {
		return rtToken{}, 0, false
	}
	i := start + 3
	// First letter — required, uppercase A..Z.
	if i >= len(r) || r[i] < 'A' || r[i] > 'Z' {
		return rtToken{}, 0, false
	}
	letters := []rune{r[i]}
	i++
	// Optional second letter.
	if i < len(r) && r[i] >= 'A' && r[i] <= 'Z' {
		letters = append(letters, r[i])
		i++
	}
	// Optional index `, N` (single digit 0..9).
	hasIndex := false
	index := 0
	if i < len(r) && r[i] == ',' {
		i++
		// Skip spaces.
		for i < len(r) && r[i] == ' ' {
			i++
		}
		if i >= len(r) || r[i] < '0' || r[i] > '9' {
			return rtToken{}, 0, false
		}
		index = int(r[i] - '0')
		hasIndex = true
		i++
	}
	if i >= len(r) || r[i] != '}' {
		return rtToken{}, 0, false
	}
	i++ // consume '}'
	return rtToken{
		kind:     rtName,
		isLocal:  isLocal,
		letters:  string(letters),
		hasIndex: hasIndex,
		index:    index,
	}, i - start, true
}

// encodeStrLit serialises a string literal to bytecode bytes.
//
// The encoding mirrors OCaml strTokens.ml to_string with quote=true:
// plain text is encoded via the active TextTransforms pipeline (which
// resolves to Shift-JIS by default), and a handful of presentation
// tokens (lenticulars, asterisk, percent, hyphen, right brace, double
// quote) have fixed SJIS code points. ResRefToken is resolved via
// Output.ResolveRes when set so that `#res<…>` references inside
// string literals are expanded to their backing text. Rich tokens that
// can't legally appear in a string parameter to an opcode (gloss,
// code, name) cause the function to fail so the caller can emit a
// safe fallback.
func (o *Output) encodeStrLit(s ast.StrLit) ([]byte, error) {
	var buf []byte
	for _, tok := range s.Tokens {
		switch t := tok.(type) {
		case ast.TextToken:
			b, err := o.encodeText(s.Loc, t.Text)
			if err != nil {
				return nil, err
			}
			buf = append(buf, b...)
		case ast.SpaceToken:
			for i := 0; i < t.Count; i++ {
				buf = append(buf, ' ')
			}
		case ast.LLenticToken:
			buf = append(buf, 0x81, 0x79)
		case ast.RLenticToken:
			buf = append(buf, 0x81, 0x7a)
		case ast.AsteriskToken:
			buf = append(buf, 0x81, 0x96)
		case ast.PercentToken:
			buf = append(buf, 0x81, 0x93)
		case ast.HyphenToken:
			buf = append(buf, '-')
		case ast.RCurToken:
			buf = append(buf, '}')
		case ast.DQuoteToken:
			buf = append(buf, '"')
		case ast.ResRefToken:
			// Resolve and inline the resource text with full marker
			// re-encoding — see encodeResourceText above for why.
			// Falling back to plain encodeText would emit `\{` as
			// 0x5c 0x7b and break name markers in the engine.
			if o.ResolveRes != nil {
				if r, ok := o.ResolveRes(t.Key); ok {
					b, err := o.encodeResourceText(s.Loc, r)
					if err == nil {
						buf = append(buf, b...)
						continue
					}
				}
			}
			// Unresolved: emit nothing rather than the textual "#res<…>"
			// which would inject bogus opcodes into the bytecode.
		default:
			return nil, fmt.Errorf("unsupported string token %T", tok)
		}
	}
	return buf, nil
}

// EmitAssignment encodes an assignment and appends it.
func (o *Output) EmitAssignment(loc ast.Loc, dest ast.Expr, op ast.AssignOp, expr ast.Expr) {
	o.EmitExpr(dest)
	o.AddCode(loc, []byte{'\\', AssignCode(op)})
	o.EmitExpr(expr)
}

// EmitOpcode encodes and appends an opcode header.
func (o *Output) EmitOpcode(loc ast.Loc, opType, opModule, opCode, argc, overload int) {
	o.AddCode(loc, EncodeOpcode(opType, opModule, opCode, argc, overload))
}

// ============================================================
// Binary file generation (from bytecodeGen.ml)
// ============================================================

// GenerateOptions controls output file generation.
type GenerateOptions struct {
	Target          kfn.Target
	CompilerVersion int    // e.g., 10002
	Compress        bool
	DebugInfo       bool
	Metadata        []byte // optional metadata bytes
	Version         kfn.Version
	KidokuType      int    // 0=auto, 1=@, 2=!

	// DramatisPersonae is the list of #character names collected during
	// directive processing. When DebugInfo is true and Target is
	// RealLive, the header dramatis table is emitted with these names.
	// Names must already be in the target bytecode encoding (Shift-JIS
	// / CP932) — the caller is responsible for any transcoding from
	// the source file encoding (UTF-8 etc).
	DramatisPersonae []string
}

// DefaultOptions returns sensible defaults.
func DefaultOptions() GenerateOptions {
	return GenerateOptions{
		Target:          kfn.TargetRealLive,
		CompilerVersion: 10002,
		Compress:        true,
		DebugInfo:       false,
		KidokuType:      0,
	}
}

// targetSpec holds format-specific parameters.
type targetSpec struct {
	kidokuLen int  // bytes per kidoku entry (2=RL, 4=AVG)
	linenoLen int  // bytes per line reference
	useLZ77   bool // compress with LZ77 (RealLive only)
}

func specForTarget(t kfn.Target) targetSpec {
	if t == kfn.TargetAVG2000 {
		return targetSpec{kidokuLen: 4, linenoLen: 4, useLZ77: false}
	}
	return targetSpec{kidokuLen: 2, linenoLen: 2, useLZ77: true}
}

// Generate traverses the IR and produces the final binary output.
// Returns the complete file bytes.
func (o *Output) Generate(opts GenerateOptions) ([]byte, error) {
	spec := specForTarget(opts.Target)

	// --- Phase 1: Deduplicate entrypoints (last definition wins) ---
	entrySlots := make(map[int]int) // entrypoint index → IR index
	for i, ir := range o.IR {
		if ir.Type == IREntrypoint {
			if prev, ok := entrySlots[ir.Index]; ok {
				o.IR[prev] = IR{Type: IRCode, Bytes: nil} // nullify previous
			}
			entrySlots[ir.Index] = i
		}
	}

	// --- Phase 2: Compute label positions and bytecode length ---
	labelPos := make(map[string]int)
	entrypoints := make([]int, 100)
	var kidokuTable []int
	bytecodeLen := 0

	for _, ir := range o.IR {
		switch ir.Type {
		case IRCode:
			bytecodeLen += len(ir.Bytes)
		case IRLabelRef:
			bytecodeLen += 4
		case IRLabel:
			labelPos[ir.Label] = bytecodeLen
		case IRKidoku:
			kidokuTable = append(kidokuTable, ir.Index)
			bytecodeLen += 1 + spec.kidokuLen
		case IREntrypoint:
			entrypoints[ir.Index] = bytecodeLen
			kidokuTable = append(kidokuTable, ir.Index+1_000_000)
			bytecodeLen += 1 + spec.kidokuLen
		case IRLineref:
			bytecodeLen += 1 + spec.linenoLen
		}
	}

	// --- Phase 3: Build bytecode buffer ---
	// Buffer starts at offset 8 to leave room for compressed header
	bufSize := bytecodeLen + 16
	buf := make([]byte, bufSize)
	kidokuIdx := 0
	pos := 8 // start after compression header space

	// Determine kidoku marker character
	kidokuChar := byte('@')
	kt := opts.KidokuType
	if kt == 0 {
		v := opts.Version
		if v[0] > 1 || (v[0] == 1 && v[1] > 2) || (v[0] == 1 && v[1] == 2 && v[2] > 5) {
			kt = 2
		} else {
			kt = 1
		}
	}
	if kt == 2 {
		kidokuChar = '!'
	}

	for _, ir := range o.IR {
		switch ir.Type {
		case IRCode:
			copy(buf[pos:], ir.Bytes)
			pos += len(ir.Bytes)
		case IRLabelRef:
			target, ok := labelPos[ir.Label]
			if !ok {
				return nil, fmt.Errorf("%s: reference to undefined label @%s", ir.Loc, ir.Label)
			}
			binary.LittleEndian.PutUint32(buf[pos:], uint32(target))
			pos += 4
		case IRLabel:
			// zero-width
		case IRKidoku:
			buf[pos] = kidokuChar
			pos++
			if spec.kidokuLen == 2 {
				binary.LittleEndian.PutUint16(buf[pos:], uint16(kidokuIdx))
				pos += 2
			} else {
				binary.LittleEndian.PutUint32(buf[pos:], uint32(kidokuIdx))
				pos += 4
			}
			kidokuIdx++
		case IREntrypoint:
			buf[pos] = kidokuChar
			pos++
			if spec.kidokuLen == 2 {
				binary.LittleEndian.PutUint16(buf[pos:], uint16(kidokuIdx))
				pos += 2
			} else {
				binary.LittleEndian.PutUint32(buf[pos:], uint32(kidokuIdx))
				pos += 4
			}
			kidokuIdx++
		case IRLineref:
			buf[pos] = 0x0a
			pos++
			if spec.linenoLen == 2 {
				binary.LittleEndian.PutUint16(buf[pos:], uint16(ir.Index))
				pos += 2
			} else {
				binary.LittleEndian.PutUint32(buf[pos:], uint32(ir.Index))
				pos += 4
			}
		}
	}

	bytecode := buf[8 : 8+bytecodeLen]
	compressedLen := bytecodeLen

	// --- Phase 4: Compress if required ---
	// (Compression is handled externally — we store uncompressed for now.
	//  The caller can use the compression package to compress bytecode.)

	// --- Phase 5: Build output file ---
	if opts.Target == kfn.TargetAVG2000 {
		return buildAVG2000(bytecode, bytecodeLen, entrypoints, kidokuTable, opts)
	}
	return buildRealLive(bytecode, bytecodeLen, compressedLen, entrypoints, kidokuTable, opts)
}

// buildRealLive creates a RealLive format .TXT (SEEN) file.
// Header: 0x1d0 bytes, then kidoku table, then optional metadata, then bytecode.
func buildRealLive(bytecode []byte, bytecodeLen, compressedLen int, entrypoints []int, kidokuTable []int, opts GenerateOptions) ([]byte, error) {
	metadataLen := len(opts.Metadata)
	kidokuBytes := len(kidokuTable) * 4

	// Build the dramatis personae table. Format per OCaml bytecodeGen.ml
	// L27-31: for each name, emit
	//   u32 LE (length+1)   bytes (name)   0x00 (NUL terminator)
	// The table is only present when debug info is on; otherwise an
	// empty table is written and the count/size fields at 0x18/0x1c
	// stay at zero.
	//
	// Name bytes are expected to be in the target bytecode encoding
	// (Shift-JIS / CP932) — see GenerateOptions.DramatisPersonae.
	var dramatisTable []byte
	dramatisCount := 0
	if opts.DebugInfo && opts.Target == kfn.TargetRealLive && len(opts.DramatisPersonae) > 0 {
		var buf bytes.Buffer
		for _, name := range opts.DramatisPersonae {
			nb := []byte(name)
			lenField := make([]byte, 4)
			binary.LittleEndian.PutUint32(lenField, uint32(len(nb)+1))
			buf.Write(lenField)
			buf.Write(nb)
			buf.WriteByte(0)
		}
		dramatisTable = buf.Bytes()
		dramatisCount = len(opts.DramatisPersonae)
	}
	dramatisSize := len(dramatisTable)

	dramOff := 0x1d0 + kidokuBytes
	bcOff := dramOff + dramatisSize + metadataLen
	fileLen := bcOff + compressedLen

	file := make([]byte, fileLen)

	// Magic / header offset.
	// IMPORTANT: codegen does NOT compress the bytecode (see Phase 4
	// below) — the buffer that follows the header is the raw bytecode.
	// We therefore must use the "KPRL" textual magic so the archiver
	// (kprl -a) recognises this file as uncompressed and compresses it
	// before inserting it into the SEEN.TXT archive.
	//
	// Emitting the 0x1d0 numeric magic here would lie about the file
	// being compressed: the archiver would store it as-is, producing
	// a bloated and broken SEEN.TXT that the engine refuses to load.
	copy(file[0x00:], []byte("KPRL"))
	putInt32(file, 0x04, opts.CompilerVersion)
	putInt32(file, 0x08, 0x1d0)         // kidoku table offset
	putInt32(file, 0x0c, len(kidokuTable)) // kidoku count
	putInt32(file, 0x10, kidokuBytes)     // kidoku table size
	putInt32(file, 0x14, dramOff)         // dramatis offset
	putInt32(file, 0x18, dramatisCount)   // dramatis count (0 if !debug_info)
	putInt32(file, 0x1c, dramatisSize)    // dramatis table size in bytes
	putInt32(file, 0x20, bcOff)           // bytecode offset
	putInt32(file, 0x24, bytecodeLen)     // bytecode length
	putInt32(file, 0x28, compressedLen)   // compressed length
	// val_0x2c (#Z-1) defaults to 0; 0x30 (#Z-2) = val_0x2c + 3.
	// OCaml bytecodeGen.ml L54-55. Although the engine itself doesn't
	// check these fields, OCaml output sets them and certain tools may.
	putInt32(file, 0x2c, 0)
	putInt32(file, 0x30, 3)

	// Entrypoints at 0x34 (up to 100 × 4 bytes)
	for i := 0; i < len(entrypoints) && i < 100; i++ {
		putInt32(file, 0x34+i*4, entrypoints[i])
	}

	// Kidoku table
	for i, v := range kidokuTable {
		putInt32(file, 0x1d0+i*4, v)
	}

	// Dramatis table (if any) then metadata
	if dramatisSize > 0 {
		copy(file[dramOff:], dramatisTable)
	}
	if metadataLen > 0 {
		copy(file[dramOff+dramatisSize:], opts.Metadata)
	}

	// Bytecode
	copy(file[bcOff:], bytecode[:compressedLen])

	return file, nil
}

// buildAVG2000 creates an AVG2000 format .TXT file.
// Header: 0x1cc bytes, then kidoku table, then bytecode.
func buildAVG2000(bytecode []byte, bytecodeLen int, entrypoints []int, kidokuTable []int, opts GenerateOptions) ([]byte, error) {
	kidokuBytes := len(kidokuTable) * 4
	bcOff := 0x1cc + kidokuBytes
	fileLen := bcOff + bytecodeLen

	file := make([]byte, fileLen)

	if opts.Compress {
		putInt32(file, 0x00, 0x1cc)
	} else {
		copy(file[0x00:], []byte("KP2K"))
	}
	putInt32(file, 0x04, 10002) // always 10002 for AVG2000

	// Timestamp at 0x08-0x15 (optional — zeros are acceptable)

	putInt32(file, 0x20, len(kidokuTable))
	putInt32(file, 0x24, bytecodeLen)

	// Entrypoints at 0x30
	for i := 0; i < len(entrypoints) && i < 100; i++ {
		putInt32(file, 0x30+i*4, entrypoints[i])
	}

	// Kidoku table
	for i, v := range kidokuTable {
		putInt32(file, 0x1cc+i*4, v)
	}

	// Bytecode
	copy(file[bcOff:], bytecode)

	return file, nil
}

func putInt32(buf []byte, offset, value int) {
	binary.LittleEndian.PutUint32(buf[offset:], uint32(value))
}
