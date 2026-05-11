// Package kfn parses reallive.kfn function definition files and provides
// a registry of RealLive engine API functions.
//
// Transposed from OCaml:
//   - common/kfnTypes.ml (67 lines)  — parameter/flag types
//   - common/kfnLexer.mll (75 lines) — KFN file tokenizer
//   - common/kfnParser.mly (269 lines) — KFN file parser
//   - rlc/keTypes.ml (265 lines)     — function registry, targets, opcodes
//
// The .kfn file format defines the RealLive engine's API: which opcodes
// exist, their parameter types, flags, and version constraints. This is
// essential for the compiler to emit correct bytecode.
//
// Usage:
//
//	reg, err := kfn.ParseFile("reallive.kfn")
//	fn, ok := reg.Lookup("goto")
//	fn.OpType, fn.OpModule, fn.OpCode → opcode triple
package kfn

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// ============================================================
// Parameter and flag types (from kfnTypes.ml)
// ============================================================

// ParamType is the type of a function parameter.
type ParamType int

const (
	PAny    ParamType = iota // any type
	PInt                     // integer
	PIntC                    // integer constant
	PIntV                    // integer variable
	PStr                     // string
	PStrC                    // string constant
	PStrV                    // string variable
	PResStr                  // resource string
	PSpecial                 // special tagged parameter
	PComplex                 // complex (tuple) parameter
)

var paramTypeNames = [...]string{"any", "int", "intC", "intV", "str", "strC", "strV", "res", "special", "complex"}

func (p ParamType) String() string { return paramTypeNames[p] }

// ParamFlag modifies parameter behavior.
type ParamFlag int

const (
	FOptional   ParamFlag = iota // ?  parameter is optional
	FReturn                      // >  parameter receives return value
	FUncount                     // <  don't count this parameter
	FFake                        // =  fake parameter (filtered out)
	FTextObject                  // #  text object parameter
	FTagged                      // 'tag'  tagged parameter
	FArgc                        // +  argc parameter
)

// Parameter is one parameter in a function prototype.
type Parameter struct {
	Type  ParamType
	Flags []ParamFlag
	Tag   string // for FTagged
}

// HasFlag reports whether the parameter carries the given flag.
func (p Parameter) HasFlag(f ParamFlag) bool {
	for _, x := range p.Flags {
		if x == f {
			return true
		}
	}
	return false
}

// FuncFlag is a function-level flag.
type FuncFlag int

const (
	FlagPushStore FuncFlag = iota // function pushes to store register
	FlagIsSkip                    // skip instruction
	FlagIsJump                    // jump instruction
	FlagIsGoto                    // goto function (registered for label dispatch)
	FlagIsCond                    // conditional
	FlagIsNeg                     // negated conditional
	FlagHasCases                  // has case dispatch
	FlagHasGotos                  // has goto dispatch
	FlagIsCall                    // call instruction
	FlagIsRet                     // return instruction
	FlagIsTextout                 // text output control code
	FlagNoBraces                  // control code without braces
	FlagIsLbr                     // left-brace control code
)

// SpecialFlag modifies special parameter behavior.
type SpecialFlag int

const (
	SFNoParens SpecialFlag = iota
)

// SpecialDef defines one case of a special parameter.
type SpecialDef struct {
	ID    int
	Name  string      // for named specials
	Params []Parameter // parameters inside the special
	Flags []SpecialFlag
}

// Prototype is one overload of a function (nil = undefined for this overload).
type Prototype struct {
	Defined bool
	Params  []Parameter
}

// ============================================================
// Target/version types (from keTypes.ml)
// ============================================================

// Target identifies the target engine.
type Target int

const (
	TargetDefault  Target = iota
	TargetRealLive
	TargetAVG2000
	TargetKinetic
)

func (t Target) String() string {
	switch t {
	case TargetRealLive: return "RealLive"
	case TargetAVG2000:  return "AVG2000"
	case TargetKinetic:  return "Kinetic"
	}
	return "Default"
}

// ParseTarget converts a string to a Target.
func ParseTarget(s string) Target {
	switch strings.ToLower(s) {
	case "reallive", "2": return TargetRealLive
	case "avg2000", "1":  return TargetAVG2000
	case "kinetic", "3":  return TargetKinetic
	}
	return TargetDefault
}

// Version is a 4-component version number.
type Version [4]int

// VersionConstraint tests whether a version matches.
type VersionConstraint func(Version) bool

// TargetConstraint is either a target class name or a version comparator.
type TargetConstraint struct {
	Class   Target            // if non-default, match this target
	Compare VersionConstraint // if non-nil, match this version constraint
}

// ============================================================
// Function definition (from keTypes.ml func type)
// ============================================================

// FuncDef is one function in the RealLive API.
type FuncDef struct {
	Ident      string
	CCStr      string     // control code string (empty if not a control code)
	Flags      []FuncFlag
	OpType     int
	OpModule   int
	OpCode     int
	Prototypes []Prototype
	Targets    []TargetConstraint
	// SyntheticOverload, when non-zero, is used as the encoded overload
	// for opcode literals that bypassed the registry (e.g. "op<X:Y:Z,W>").
	// When zero, normal prototype-based overload selection applies.
	SyntheticOverload int
}

// IdentOfOpcode builds a synthetic identifier from opcode components.
func IdentOfOpcode(opType, opModule, opCode, overload int) string {
	return fmt.Sprintf("__op_%d_%d_%d_%d", opType, opModule, opCode, overload)
}

// HasFlag checks if the function has a given flag.
func (f *FuncDef) HasFlag(flag FuncFlag) bool {
	for _, fl := range f.Flags {
		if fl == flag { return true }
	}
	return false
}

// ReturnType determines the return type of a function.
// Returns "int", "str", or "none".
func (f *FuncDef) ReturnType() string {
	if f.HasFlag(FlagPushStore) { return "int" }
	for _, proto := range f.Prototypes {
		if !proto.Defined { continue }
		for _, p := range proto.Params {
			for _, fl := range p.Flags {
				if fl == FReturn {
					if p.Type == PInt || p.Type == PIntC || p.Type == PIntV { return "int" }
					if p.Type == PStr || p.Type == PStrC || p.Type == PStrV { return "str" }
				}
			}
		}
	}
	return "none"
}

// ============================================================
// Registry (from keTypes.ml function tables)
// ============================================================

// Registry holds all parsed function definitions and module mappings.
type Registry struct {
	Functions map[string][]*FuncDef // ident → list of overloaded defs
	CtrlCodes map[string][]*FuncDef // control code name → defs
	Modules   map[string]int        // module name → module number
	GotoFuncs []string              // identifiers with IsGoto flag
	Target    Target
	Version   Version
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		Functions: make(map[string][]*FuncDef),
		CtrlCodes: make(map[string][]*FuncDef),
		Modules:   make(map[string]int),
		Target:    TargetDefault,
	}
}

// Register adds a function definition to the registry.
func (r *Registry) Register(fd *FuncDef) {
	r.Functions[fd.Ident] = append(r.Functions[fd.Ident], fd)
	if fd.CCStr != "" {
		r.CtrlCodes[fd.CCStr] = append(r.CtrlCodes[fd.CCStr], fd)
	}
	if fd.HasFlag(FlagIsGoto) {
		r.GotoFuncs = append(r.GotoFuncs, fd.Ident)
	}
}

// Lookup finds a function by identifier, filtering by current target/version.
func (r *Registry) Lookup(ident string) (*FuncDef, bool) {
	fns, ok := r.Functions[ident]
	if !ok || len(fns) == 0 { return nil, false }
	for _, fn := range fns {
		if r.validForTarget(fn) { return fn, true }
	}
	return fns[0], true // fallback to first
}

// LookupCtrlCode finds a control code function.
func (r *Registry) LookupCtrlCode(name string) (*FuncDef, bool) {
	fns, ok := r.CtrlCodes[name]
	if !ok || len(fns) == 0 { return nil, false }
	for _, fn := range fns {
		if r.validForTarget(fn) { return fn, true }
	}
	return fns[0], true
}

func (r *Registry) validForTarget(fd *FuncDef) bool {
	if len(fd.Targets) == 0 { return true }
	target := r.Target
	if target == TargetDefault { target = TargetRealLive }
	// A KFN entry written as `ver Avg2000, RealLive` carries two
	// TargetConstraint entries. The semantics are OR: the function is
	// valid if ANY constraint matches the current target. (The previous
	// AND-shaped loop rejected RealLive entries that also mentioned
	// Avg2000, including bgmLoop and many others.)
	//
	// A version Compare clause is treated as a refinement of the class:
	// only checked when the class itself matches the current target.
	for _, tc := range fd.Targets {
		classOK := tc.Class == TargetDefault || tc.Class == target
		if !classOK {
			continue
		}
		if tc.Compare != nil && !tc.Compare(r.Version) {
			continue
		}
		return true
	}
	return false
}

// ValidForTarget is the exported form of validForTarget. It lets
// callers outside the kfn package (notably function.LookupFuncDef)
// filter overload candidates by the target/version selected at load
// time — critical when an opcode like bgmLoop has both a Kinetic
// `<0:Sys:2>` entry and a RealLive `<1:Bgm:0>` entry.
func (r *Registry) ValidForTarget(fd *FuncDef) bool {
	return r.validForTarget(fd)
}

// CurrentVersionString returns a display string like "RealLive 1.2.7".
func (r *Registry) CurrentVersionString() string {
	name := r.Target.String()
	v := r.Version
	if v == (Version{}) {
		if r.Target == TargetAVG2000 {
			v = Version{1, 0, 0, 0}
		} else {
			v = Version{1, 2, 7, 0}
		}
	}
	switch {
	case v[2] == 0 && v[3] == 0: return fmt.Sprintf("%s %d.%d", name, v[0], v[1])
	case v[3] == 0:              return fmt.Sprintf("%s %d.%d.%d", name, v[0], v[1], v[2])
	default:                     return fmt.Sprintf("%s %d.%d.%d.%d", name, v[0], v[1], v[2], v[3])
	}
}

// ============================================================
// KFN file parser (from kfnLexer.mll + kfnParser.mly)
// ============================================================

// ParseFile parses a reallive.kfn file and returns a populated Registry.
func ParseFile(path string) (*Registry, error) {
	f, err := os.Open(path)
	if err != nil { return nil, err }
	defer f.Close()
	return Parse(f)
}

// Parse parses a reallive.kfn from a reader.
func Parse(r io.Reader) (*Registry, error) {
	data, err := io.ReadAll(r)
	if err != nil { return nil, err }
	reg := NewRegistry()
	p := &kfnParser{
		src:  string(data),
		reg:  reg,
		mods: make(map[string]int),
		line: 1,
	}
	if err := p.parse(); err != nil {
		return nil, err
	}
	return reg, nil
}

// --- KFN tokenizer ---

type kfnTokType int

const (
	kEOF kfnTokType = iota
	kMODULE; kFUN; kVER; kEND
	kLt; kGt; kEq; kCm; kLp; kRp; kLbr; kRbr; kQu; kSt; kPl; kCo; kPt; kHa; kHy
	kINT; kINTC; kINTV; kSTR; kSTRC; kSTRV; kRES; kSPECIAL
	kINTEGER; kIDENT; kSTRING
)

type kfnTok struct {
	typ kfnTokType
	num int
	str string
}

type kfnLexer struct {
	src  []byte
	pos  int
	line int
}

func (l *kfnLexer) next() kfnTok {
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		// Skip whitespace
		if c == ' ' || c == '\t' || c == '\r' {
			l.pos++; continue
		}
		if c == '\n' {
			l.pos++; l.line++; continue
		}
		// Line comment
		if c == '/' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '/' {
			l.pos += 2
			for l.pos < len(l.src) && l.src[l.pos] != '\n' { l.pos++ }
			continue
		}
		// Single-char tokens
		l.pos++
		switch c {
		case '=': return kfnTok{typ: kEq}
		case '<': return kfnTok{typ: kLt}
		case '>': return kfnTok{typ: kGt}
		case '(': return kfnTok{typ: kLp}
		case ')': return kfnTok{typ: kRp}
		case '{': return kfnTok{typ: kLbr}
		case '}': return kfnTok{typ: kRbr}
		case '?': return kfnTok{typ: kQu}
		case ',': return kfnTok{typ: kCm}
		case '*': return kfnTok{typ: kSt}
		case '+': return kfnTok{typ: kPl}
		case ':': return kfnTok{typ: kCo}
		case '.': return kfnTok{typ: kPt}
		case '#': return kfnTok{typ: kHa}
		case '-': return kfnTok{typ: kHy}
		}
		l.pos-- // rewind for multi-char tokens
		// Number: decimal or $hex
		if c >= '0' && c <= '9' {
			start := l.pos
			for l.pos < len(l.src) && l.src[l.pos] >= '0' && l.src[l.pos] <= '9' { l.pos++ }
			n, _ := strconv.Atoi(string(l.src[start:l.pos]))
			return kfnTok{typ: kINTEGER, num: n}
		}
		if c == '$' {
			l.pos++ // skip $
			start := l.pos
			for l.pos < len(l.src) && isHexDigit(l.src[l.pos]) { l.pos++ }
			n, _ := strconv.ParseInt(string(l.src[start:l.pos]), 16, 32)
			return kfnTok{typ: kINTEGER, num: int(n)}
		}
		// Quoted string
		if c == '\'' {
			l.pos++ // skip opening '
			start := l.pos
			for l.pos < len(l.src) && l.src[l.pos] != '\'' { l.pos++ }
			s := string(l.src[start:l.pos])
			if l.pos < len(l.src) { l.pos++ } // skip closing '
			return kfnTok{typ: kSTRING, str: s}
		}
		// Identifier or keyword
		if isAlpha(c) || c == '_' {
			start := l.pos
			for l.pos < len(l.src) && isIdentChar(l.src[l.pos]) { l.pos++ }
			word := string(l.src[start:l.pos])
			switch word {
			case "module":  return kfnTok{typ: kMODULE}
			case "fun":     return kfnTok{typ: kFUN}
			case "ver":     return kfnTok{typ: kVER}
			case "end":     return kfnTok{typ: kEND, str: "end"}
			case "int":     return kfnTok{typ: kINT}
			case "intC":    return kfnTok{typ: kINTC}
			case "intV":    return kfnTok{typ: kINTV}
			case "str":     return kfnTok{typ: kSTR}
			case "strC":    return kfnTok{typ: kSTRC}
			case "strV":    return kfnTok{typ: kSTRV}
			case "res":     return kfnTok{typ: kRES}
			case "special": return kfnTok{typ: kSPECIAL}
			default:        return kfnTok{typ: kIDENT, str: word}
			}
		}
		// Unknown char — skip
		l.pos++
	}
	return kfnTok{typ: kEOF}
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
func isAlpha(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_'
}
func isIdentChar(c byte) bool {
	return isAlpha(c) || (c >= '0' && c <= '9') || c == '$' || c == '?'
}

// --- KFN parser ---

type kfnParser struct {
	src  string
	lex  kfnLexer
	cur  kfnTok
	reg  *Registry
	mods map[string]int
	line int
}

func (p *kfnParser) advance() kfnTok {
	prev := p.cur
	p.cur = p.lex.next()
	p.line = p.lex.line
	return prev
}

func (p *kfnParser) expect(t kfnTokType) kfnTok {
	if p.cur.typ != t {
		panic(fmt.Sprintf("kfn line %d: expected token %d, got %d", p.line, t, p.cur.typ))
	}
	return p.advance()
}

func (p *kfnParser) match(t kfnTokType) bool {
	if p.cur.typ == t { p.advance(); return true }
	return false
}

func (p *kfnParser) parse() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	p.lex = kfnLexer{src: []byte(p.src), line: 1}
	p.advance()
	for p.cur.typ != kEOF {
		switch p.cur.typ {
		case kMODULE:
			p.parseModule()
		case kFUN:
			fd := p.parseFunDef()
			p.processFunDef(nil, fd)
		case kVER:
			p.parseVerBlock()
		default:
			p.advance() // skip unexpected
		}
	}
	return nil
}

func (p *kfnParser) parseModule() {
	p.expect(kMODULE)
	num := p.expect(kINTEGER).num
	if p.cur.typ == kEq {
		p.advance()
		name := p.expect(kIDENT).str
		p.mods[name] = num
		p.reg.Modules[name] = num
	}
}

type rawFunDef struct {
	ident    string
	ccName   string // "" absent, "__self__" unnamed, else named
	ccFlags  []FuncFlag
	funFlags []FuncFlag
	opType   int
	opModule int
	opCode   int
	overloads int
	protos   []Prototype
}

func (p *kfnParser) parseFunDef() rawFunDef {
	p.expect(kFUN)
	// ident (may be empty, "end", single ident, or two idents)
	ident := ""
	if p.cur.typ == kIDENT {
		ident = p.cur.str; p.advance()
		if p.cur.typ == kIDENT {
			// second ident = alternate name, use first
			p.advance()
		}
	} else if p.cur.typ == kEND {
		ident = "end"; p.advance()
	}

	// ccode: {}, {name}, {*name}, {=name}, {*=name}
	ccName := ""
	var ccFlags []FuncFlag
	if p.match(kLbr) {
		if p.match(kRbr) {
			ccName = "__self__" // unnamed = use ident
		} else {
			hasStar := p.match(kSt)
			hasEq := p.match(kEq)
			if p.cur.typ == kIDENT {
				ccName = p.cur.str; p.advance()
			}
			p.expect(kRbr)
			if hasStar { ccFlags = append(ccFlags, FlagIsTextout) }
			if hasEq { ccFlags = append(ccFlags, FlagNoBraces) }
			if hasStar && hasEq { ccFlags = append(ccFlags, FlagIsLbr) }
		}
	}

	// fun_flags: (flag flag ...)
	var funFlags []FuncFlag
	if p.match(kLp) {
		for p.cur.typ == kIDENT {
			flag := parseFuncFlag(strings.ToLower(p.cur.str))
			if flag >= 0 { funFlags = append(funFlags, FuncFlag(flag)) }
			p.advance()
		}
		p.expect(kRp)
	}

	// <opType:moduleId:opCode,overloads>
	p.expect(kLt)
	opType := p.expect(kINTEGER).num
	p.expect(kCo)
	opModule := p.parseModuleID()
	p.expect(kCo)
	opCode := p.expect(kINTEGER).num
	p.expect(kCm)
	overloads := p.expect(kINTEGER).num
	p.expect(kGt)

	// prototypes
	var protos []Prototype
	for p.cur.typ == kQu || p.cur.typ == kLp {
		protos = append(protos, p.parsePrototype())
	}

	return rawFunDef{
		ident: ident, ccName: ccName, ccFlags: ccFlags, funFlags: funFlags,
		opType: opType, opModule: opModule, opCode: opCode,
		overloads: overloads, protos: protos,
	}
}

func (p *kfnParser) parseModuleID() int {
	if p.cur.typ == kINTEGER {
		return p.advance().num
	}
	if p.cur.typ == kIDENT {
		name := p.cur.str; p.advance()
		if num, ok := p.mods[name]; ok { return num }
		panic(fmt.Sprintf("kfn line %d: undeclared module %s", p.line, name))
	}
	panic(fmt.Sprintf("kfn line %d: expected module id", p.line))
}

func (p *kfnParser) parsePrototype() Prototype {
	if p.match(kQu) {
		return Prototype{Defined: false}
	}
	p.expect(kLp)
	var params []Parameter
	for p.cur.typ != kRp && p.cur.typ != kEOF {
		if p.cur.typ == kCm { p.advance(); continue } // skip trailing/extra commas
		params = append(params, p.parseParameter())
		p.match(kCm) // optional comma
	}
	p.expect(kRp)
	return Prototype{Defined: true, Params: params}
}

func (p *kfnParser) parseParameter() Parameter {
	// preparm: ?, #, <, >, =
	var flags []ParamFlag
	var tag string
	for {
		switch p.cur.typ {
		case kHa: p.advance(); flags = append(flags, FTextObject); continue
		case kQu: p.advance(); flags = append(flags, FOptional); continue
		case kLt: p.advance(); flags = append(flags, FUncount); continue
		case kGt: p.advance(); flags = append(flags, FReturn); continue
		case kEq: p.advance(); flags = append(flags, FFake); continue
		}
		break
	}

	// typedef or tagged string
	var pt ParamType
	if p.cur.typ == kSTRING {
		tag = p.cur.str; p.advance()
		pt = PIntC
		flags = append(flags, FTagged)
	} else {
		pt = p.parseTypeDef()
	}

	// postparm: +, 'tag'
	for {
		if p.match(kPl) { flags = append(flags, FArgc); continue }
		if p.cur.typ == kSTRING {
			tag = p.cur.str; p.advance()
			flags = append(flags, FTagged)
			continue
		}
		break
	}

	return Parameter{Type: pt, Flags: flags, Tag: tag}
}

func (p *kfnParser) parseTypeDef() ParamType {
	switch p.cur.typ {
	case kINT:     p.advance(); return PInt
	case kINTC:    p.advance(); return PIntC
	case kINTV:    p.advance(); return PIntV
	case kSTR:     p.advance(); return PStr
	case kSTRC:    p.advance(); return PStrC
	case kSTRV:    p.advance(); return PStrV
	case kRES:     p.advance(); return PResStr
	case kSPECIAL:
		p.advance(); p.expect(kLp)
		// skip special definition details for now
		depth := 1
		for depth > 0 && p.cur.typ != kEOF {
			if p.cur.typ == kLp { depth++ }
			if p.cur.typ == kRp { depth-- }
			if depth > 0 { p.advance() }
		}
		p.expect(kRp)
		return PSpecial
	case kLp:
		p.advance()
		// complex: (typedef, typedef, ...)
		depth := 1
		for depth > 0 && p.cur.typ != kEOF {
			if p.cur.typ == kLp { depth++ }
			if p.cur.typ == kRp { depth-- }
			if depth > 0 { p.advance() }
		}
		p.expect(kRp)
		return PComplex
	}
	// Default: skip unknown and return Any
	p.advance()
	return PAny
}

func (p *kfnParser) parseVerBlock() {
	p.expect(kVER)
	// Parse version constraints
	var constraints []TargetConstraint
	constraints = append(constraints, p.parseVersionConstraint())
	for p.match(kCm) {
		constraints = append(constraints, p.parseVersionConstraint())
	}
	// Parse fun_defs until END
	for p.cur.typ == kFUN {
		fd := p.parseFunDef()
		p.processFunDef(constraints, fd)
	}
	p.expect(kEND)
}

func (p *kfnParser) parseVersionConstraint() TargetConstraint {
	if p.cur.typ == kIDENT {
		class := ParseTarget(p.cur.str)
		p.advance()
		return TargetConstraint{Class: class}
	}
	// < or > version comparison
	if p.cur.typ == kLt || p.cur.typ == kGt {
		isLt := p.cur.typ == kLt
		p.advance()
		hasEq := p.match(kEq)
		v := p.parseVStamp()
		return TargetConstraint{Compare: func(cur Version) bool {
			if isLt && hasEq { return cur[0] <= v[0] || (cur[0] == v[0] && cur[1] <= v[1]) }
			if isLt { return cur[0] < v[0] || (cur[0] == v[0] && cur[1] < v[1]) }
			if hasEq { return cur[0] >= v[0] || (cur[0] == v[0] && cur[1] >= v[1]) }
			return cur[0] > v[0] || (cur[0] == v[0] && cur[1] > v[1])
		}}
	}
	return TargetConstraint{}
}

func (p *kfnParser) parseVStamp() Version {
	v := Version{}
	v[0] = p.expect(kINTEGER).num
	if p.match(kPt) {
		v[1] = p.expect(kINTEGER).num
		if p.match(kPt) {
			v[2] = p.expect(kINTEGER).num
			if p.match(kPt) {
				v[3] = p.expect(kINTEGER).num
			}
		}
	}
	return v
}

func (p *kfnParser) processFunDef(constraints []TargetConstraint, raw rawFunDef) {
	ident := raw.ident
	if ident == "" {
		ident = IdentOfOpcode(raw.opType, raw.opModule, raw.opCode, 0)
	}
	ccStr := ""
	switch raw.ccName {
	case "":          // absent
	case "__self__":  ccStr = ident
	default:          ccStr = raw.ccName
	}

	// Filter out Fake params
	var protos []Prototype
	for _, proto := range raw.protos {
		if !proto.Defined {
			protos = append(protos, proto)
			continue
		}
		var filtered []Parameter
		for _, param := range proto.Params {
			isFake := false
			for _, fl := range param.Flags {
				if fl == FFake { isFake = true; break }
			}
			if !isFake { filtered = append(filtered, param) }
		}
		protos = append(protos, Prototype{Defined: true, Params: filtered})
	}

	allFlags := append(raw.ccFlags, raw.funFlags...)

	fd := &FuncDef{
		Ident:      ident,
		CCStr:      ccStr,
		Flags:      allFlags,
		OpType:     raw.opType,
		OpModule:   raw.opModule,
		OpCode:     raw.opCode,
		Prototypes: protos,
		Targets:    constraints,
	}
	p.reg.Register(fd)
}

func parseFuncFlag(s string) FuncFlag {
	switch s {
	case "store": return FlagPushStore
	case "skip":  return FlagIsSkip
	case "jump":  return FlagIsJump
	case "goto":  return FlagIsGoto
	case "if":    return FlagIsCond
	case "neg":   return FlagIsNeg
	case "cases": return FlagHasCases
	case "gotos": return FlagHasGotos
	case "call":  return FlagIsCall
	case "ret":   return FlagIsRet
	}
	return -1
}
