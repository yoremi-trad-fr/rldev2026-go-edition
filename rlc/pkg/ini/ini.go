// Package ini implements the GAMEEXE.INI parser for the Kepago compiler.
//
// Transposed from OCaml:
//   - rlc/ini.ml (97 lines)         — definition table, find/set/get helpers
//   - rlc/iniParser.mly (100 lines) — parser grammar
//   - rlc/iniLexer.mll (64 lines)   — tokenizer
//
// GAMEEXE.INI is the main configuration file for RealLive games.
// Each line starts with # followed by a key and optional = value list.
//
// Keys support dotted paths with numeric and text segments:
//   #IDENT = value
//   #IDENT.NNN = value
//   #IDENT.NNN.TEXT = value
//   #IDENT.NNN.NNN.TEXT = value
//   #IDENT.NNN.NNN.TEXT.NNN = value   (rldev2026 fix: Tomoyo After Steam)
//   #IDENT.NNN.TEXT.TEXT.TEXT = value  (rldev2026 fix: Clannad Side Stories)
//   etc.
//
// Values: integers, quoted strings, U (enabled), N (disabled), ranges (1,2,3)
package ini

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// ============================================================
// Value types (from ini.ml)
// ============================================================

// ValueKind identifies the type of an INI value.
type ValueKind int

const (
	VDefined ValueKind = iota // key exists with no value
	VEnabled                   // U (true) or N (false)
	VInteger                   // integer value
	VString                    // quoted string
	VRange                     // (int, int, ...) range
)

// Value is one element of an INI definition's value list.
type Value struct {
	Kind    ValueKind
	Bool    bool    // for VEnabled
	Int     int32   // for VInteger
	Str     string  // for VString
	Ints    []int32 // for VRange
}

// ============================================================
// Definition table (from ini.ml)
// ============================================================

// Table holds all parsed GAMEEXE.INI definitions.
type Table struct {
	defs map[string][]Value
}

// NewTable creates an empty definition table.
func NewTable() *Table {
	return &Table{defs: make(map[string][]Value)}
}

// Set defines a key with a value list.
func (t *Table) Set(key string, values []Value) {
	t.defs[strings.ToLower(key)] = values
}

// Find looks up a key. Returns nil if not found.
func (t *Table) Find(key string) []Value {
	v, ok := t.defs[strings.ToLower(key)]
	if !ok {
		return nil
	}
	return v
}

// Exists returns true if a key is defined.
func (t *Table) Exists(key string) bool {
	_, ok := t.defs[strings.ToLower(key)]
	return ok
}

// Unset removes a key.
func (t *Table) Unset(key string) {
	delete(t.defs, strings.ToLower(key))
}

// SetInt sets a key to a single integer value.
func (t *Table) SetInt(key string, value int) {
	t.Set(key, []Value{{Kind: VInteger, Int: int32(value)}})
}

// GetInt returns a key's integer value, or def if not found or wrong type.
func (t *Table) GetInt(key string, def int) int {
	v := t.Find(key)
	if len(v) == 1 && v[0].Kind == VInteger {
		return int(v[0].Int)
	}
	return def
}

// GetPair returns two integer values from a key, or def if not found.
func (t *Table) GetPair(key string, def [2]int) [2]int {
	v := t.Find(key)
	if len(v) == 2 && v[0].Kind == VInteger && v[1].Kind == VInteger {
		return [2]int{int(v[0].Int), int(v[1].Int)}
	}
	return def
}

// Keys returns all defined keys.
func (t *Table) Keys() []string {
	keys := make([]string, 0, len(t.defs))
	for k := range t.defs {
		keys = append(keys, k)
	}
	return keys
}

// Count returns the number of defined keys.
func (t *Table) Count() int { return len(t.defs) }

// ============================================================
// INI file parser (from iniLexer.mll + iniParser.mly)
// ============================================================

// ParseFile parses a GAMEEXE.INI file.
func ParseFile(path string) (*Table, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(f)
}

// Parse parses a GAMEEXE.INI from a reader.
func Parse(r io.Reader) (*Table, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	t := NewTable()
	p := &iniParser{src: string(data), table: t, line: 1}
	if err := p.parse(); err != nil {
		return nil, err
	}
	return t, nil
}

// --- INI tokenizer ---

type iniTokType int

const (
	iTEOF iniTokType = iota
	iTHash; iTEq; iTCm; iTCo; iTLp; iTRp; iTHy; iTDot
	iTINT; iTDOTINT; iTIDENT; iTDOTIDENT; iTSTRING; iTUN
	iTSHAKE; iTDSTRACK; iTCDTRACK; iTNAMAE
)

type iniTok struct {
	typ iniTokType
	num int32
	str string
	boo bool
}

type iniLexer struct {
	src  []byte
	pos  int
	line int
}

func (l *iniLexer) next() iniTok {
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		// Whitespace
		if c == ' ' || c == '\t' { l.pos++; continue }
		if c == '\r' { l.pos++; continue }
		if c == '\n' { l.pos++; l.line++; continue }
		// Comment: ; until end of line
		if c == ';' {
			for l.pos < len(l.src) && l.src[l.pos] != '\n' { l.pos++ }
			continue
		}
		// Single-char tokens
		l.pos++
		switch c {
		case '#': return iniTok{typ: iTHash}
		case '=': return iniTok{typ: iTEq}
		case ',': return iniTok{typ: iTCm}
		case ':': return iniTok{typ: iTCo}
		case '(': return iniTok{typ: iTLp}
		case ')': return iniTok{typ: iTRp}
		case '-':
			// Could be minus sign before number, or standalone hyphen
			if l.pos < len(l.src) && l.src[l.pos] >= '0' && l.src[l.pos] <= '9' {
				l.pos-- // let number parser handle it
				return l.scanNumber()
			}
			return iniTok{typ: iTHy}
		case 'U':
			if l.pos >= len(l.src) || !isIDChar(l.src[l.pos]) {
				return iniTok{typ: iTUN, boo: true}
			}
			l.pos--
			return l.scanIdent()
		case 'N':
			if l.pos >= len(l.src) || !isIDChar(l.src[l.pos]) {
				return iniTok{typ: iTUN, boo: false}
			}
			l.pos--
			return l.scanIdent()
		case '"':
			return l.scanString()
		case '.':
			// .NNN or .IDENT
			if l.pos < len(l.src) {
				nc := l.src[l.pos]
				if nc == '-' || (nc >= '0' && nc <= '9') {
					return l.scanDotNumber()
				}
				if isIDStart(nc) {
					return l.scanDotIdent()
				}
			}
			return iniTok{typ: iTDot}
		default:
			l.pos--
			if c == '-' || (c >= '0' && c <= '9') {
				return l.scanNumber()
			}
			if isIDStart(c) {
				return l.scanIdent()
			}
			// Skip unknown
			l.pos++
		}
	}
	return iniTok{typ: iTEOF}
}

func (l *iniLexer) scanNumber() iniTok {
	start := l.pos
	if l.pos < len(l.src) && l.src[l.pos] == '-' { l.pos++ }
	for l.pos < len(l.src) && l.src[l.pos] >= '0' && l.src[l.pos] <= '9' { l.pos++ }
	n, _ := strconv.ParseInt(string(l.src[start:l.pos]), 10, 32)
	return iniTok{typ: iTINT, num: int32(n)}
}

func (l *iniLexer) scanDotNumber() iniTok {
	start := l.pos
	if l.pos < len(l.src) && l.src[l.pos] == '-' { l.pos++ }
	for l.pos < len(l.src) && l.src[l.pos] >= '0' && l.src[l.pos] <= '9' { l.pos++ }
	n, _ := strconv.Atoi(string(l.src[start:l.pos]))
	return iniTok{typ: iTDOTINT, num: int32(n)}
}

func (l *iniLexer) scanIdent() iniTok {
	start := l.pos
	for l.pos < len(l.src) && isIDChar(l.src[l.pos]) { l.pos++ }
	word := string(l.src[start:l.pos])
	switch word {
	case "SHAKE", "SHAKEZOOM":
		return iniTok{typ: iTSHAKE, str: word}
	case "DSTRACK":
		return iniTok{typ: iTDSTRACK}
	case "CDTRACK":
		return iniTok{typ: iTCDTRACK}
	case "NAMAE":
		return iniTok{typ: iTNAMAE}
	}
	return iniTok{typ: iTIDENT, str: word}
}

func (l *iniLexer) scanDotIdent() iniTok {
	start := l.pos
	for l.pos < len(l.src) && isIDChar(l.src[l.pos]) { l.pos++ }
	return iniTok{typ: iTDOTIDENT, str: string(l.src[start:l.pos])}
}

func (l *iniLexer) scanString() iniTok {
	start := l.pos
	for l.pos < len(l.src) && l.src[l.pos] != '"' { l.pos++ }
	s := string(l.src[start:l.pos])
	if l.pos < len(l.src) { l.pos++ } // skip closing "
	return iniTok{typ: iTSTRING, str: s}
}

func isIDStart(c byte) bool {
	return (c >= 'A' && c <= 'Z') || c == '_' || (c >= '0' && c <= '9') || c == '[' || c == ']'
}
func isIDChar(c byte) bool { return isIDStart(c) }

// --- INI parser ---

type iniParser struct {
	src   string
	lex   iniLexer
	cur   iniTok
	table *Table
	line  int
}

func (p *iniParser) advance() iniTok {
	prev := p.cur
	p.cur = p.lex.next()
	p.line = p.lex.line
	return prev
}

func (p *iniParser) expect(t iniTokType) iniTok {
	if p.cur.typ != t {
		panic(fmt.Sprintf("GAMEEXE.INI line %d: expected token %d, got %d", p.line, t, p.cur.typ))
	}
	return p.advance()
}

func (p *iniParser) match(t iniTokType) bool {
	if p.cur.typ == t { p.advance(); return true }
	return false
}

func (p *iniParser) parse() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	p.lex = iniLexer{src: []byte(p.src), line: 1}
	p.advance()
	for p.cur.typ != iTEOF {
		if p.match(iTHash) {
			p.parseDefinition()
		} else {
			p.advance() // skip unexpected tokens
		}
	}
	return nil
}

func (p *iniParser) parseDefinition() {
	switch p.cur.typ {
	case iTSHAKE:
		p.parseShake()
	case iTDSTRACK:
		p.parseDSTrack()
	case iTCDTRACK:
		p.parseCDTrack()
	case iTNAMAE:
		p.parseNamae()
	case iTIDENT:
		p.parseKeyDef()
	default:
		// Skip to next # or EOF
		for p.cur.typ != iTHash && p.cur.typ != iTEOF { p.advance() }
	}
}

// parseKeyDef handles all the key patterns from iniParser.mly.
// This is where Jérémy's 5-level key fixes are implemented.
func (p *iniParser) parseKeyDef() {
	ident := p.advance().str // consume IDENT

	// Collect dot segments to build the full key
	var segments []segment
	for p.cur.typ == iTDOTINT || p.cur.typ == iTDOTIDENT || p.cur.typ == iTDot {
		if p.cur.typ == iTDOTINT {
			segments = append(segments, segment{isNum: true, num: int(p.cur.num)})
			p.advance()
		} else if p.cur.typ == iTDOTIDENT {
			segments = append(segments, segment{isNum: false, str: p.cur.str})
			p.advance()
		} else {
			// bare dot — might be followed by a range
			p.advance()
			break
		}
	}

	// Check for range pattern: IDENT.NNN:INT (range definition)
	if len(segments) == 1 && segments[0].isNum && p.cur.typ == iTCo {
		p.advance() // skip :
		endVal := p.expect(iTINT).num
		p.expect(iTEq)
		vals := p.parseParameters()
		for i := segments[0].num; i <= int(endVal); i++ {
			p.table.Set(fmt.Sprintf("%s.%03d", ident, i), vals)
		}
		return
	}

	// Check for range+dotident: IDENT.NNN.(range).TEXT
	if len(segments) == 1 && segments[0].isNum && p.cur.typ == iTLp {
		rangeVals := p.parseRange()
		dotText := p.expect(iTDOTIDENT).str
		p.expect(iTEq)
		vals := p.parseParameters()
		for _, rv := range rangeVals {
			p.table.Set(fmt.Sprintf("%s.%03d.%03d.%s", ident, segments[0].num, rv, dotText), vals)
		}
		return
	}

	// Standard: key = parameters
	p.expect(iTEq)
	vals := p.parseParameters()

	key := buildKey(ident, segments)
	p.table.Set(key, vals)
}

type segment struct {
	isNum bool
	num   int
	str   string
}

func buildKey(ident string, segs []segment) string {
	var b strings.Builder
	b.WriteString(ident)
	for _, s := range segs {
		if s.isNum {
			fmt.Fprintf(&b, ".%03d", s.num)
		} else {
			fmt.Fprintf(&b, ".%s", s.str)
		}
	}
	return b.String()
}

func (p *iniParser) parseShake() {
	name := p.advance().str // consume SHAKE/SHAKEZOOM
	dotNum := p.expect(iTDOTINT).num
	p.expect(iTEq)
	vals := p.parseRanges()
	p.table.Set(fmt.Sprintf("%s.%03d", name, dotNum), vals)
}

func (p *iniParser) parseDSTrack() {
	p.advance() // skip DSTRACK
	// DSTRACK = int int INT = STRING = STRING
	// Skip everything until next # or EOF
	for p.cur.typ != iTHash && p.cur.typ != iTEOF { p.advance() }
}

func (p *iniParser) parseCDTrack() {
	p.advance()
	for p.cur.typ != iTHash && p.cur.typ != iTEOF { p.advance() }
}

func (p *iniParser) parseNamae() {
	p.advance()
	for p.cur.typ != iTHash && p.cur.typ != iTEOF { p.advance() }
}

func (p *iniParser) parseParameters() []Value {
	var vals []Value
	if p.cur.typ == iTHash || p.cur.typ == iTEOF {
		return vals
	}
	v, ok := p.parseValue()
	if !ok { return vals }
	vals = append(vals, v)
	for p.cur.typ == iTCm || p.cur.typ == iTCo || p.cur.typ == iTEq {
		p.advance() // skip separator
		v, ok = p.parseValue()
		if !ok { break }
		vals = append(vals, v)
	}
	return vals
}

func (p *iniParser) parseValue() (Value, bool) {
	switch p.cur.typ {
	case iTINT:
		v := p.advance().num
		return Value{Kind: VInteger, Int: v}, true
	case iTSTRING:
		s := p.advance().str
		return Value{Kind: VString, Str: s}, true
	case iTUN:
		b := p.advance().boo
		return Value{Kind: VEnabled, Bool: b}, true
	}
	return Value{}, false
}

func (p *iniParser) parseRanges() []Value {
	var vals []Value
	for p.cur.typ == iTLp {
		ints := p.parseRange()
		vals = append(vals, Value{Kind: VRange, Ints: ints})
	}
	return vals
}

func (p *iniParser) parseRange() []int32 {
	p.expect(iTLp)
	var ints []int32
	ints = append(ints, p.expect(iTINT).num)
	for p.match(iTCm) {
		ints = append(ints, p.expect(iTINT).num)
	}
	p.expect(iTRp)
	return ints
}
