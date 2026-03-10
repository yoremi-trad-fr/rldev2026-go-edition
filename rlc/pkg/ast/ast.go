// Package ast defines the Abstract Syntax Tree for the Kepago language.
// Transposed from OCaml's rlc/keAst.ml (~736 lines).
//
// Kepago is the scripting language used by the RealLive visual novel engine.
// The AST represents parsed .org source files with:
//   - Expressions: integer/string literals, variables, operators, function calls
//   - Statements: control flow, assignments, declarations, directives
//   - Rich string tokens: embedded formatting codes in string literals
//
// Go interfaces replace OCaml's polymorphic variants:
//   Expr     — expression nodes
//   Stmt     — statement nodes
//   Param    — function call parameters
//   StrToken — rich string tokens (defined in token package)
package ast

import "fmt"

// ============================================================
// Source location
// ============================================================

// Loc is a source position (file + line number).
type Loc struct {
	File string
	Line int
}

// Nowhere is the zero location for generated/synthetic nodes.
var Nowhere = Loc{}

func (l Loc) String() string {
	if l.File == "" {
		return fmt.Sprintf("line %d", l.Line)
	}
	return fmt.Sprintf("%s:%d", l.File, l.Line)
}

// ============================================================
// Operator types
// ============================================================

// ArithOp is a binary arithmetic/bitwise operator.
type ArithOp int

const (
	OpAdd ArithOp = iota // +
	OpSub                // -
	OpMul                // *
	OpDiv                // /
	OpMod                // %
	OpAnd                // &
	OpOr                 // |
	OpXor                // ^
	OpShl                // <<
	OpShr                // >>
)

var arithSymbol = [...]string{"+", "-", "*", "/", "%", "&", "|", "^", "<<", ">>"}

func (op ArithOp) String() string { return arithSymbol[op] }

// UnaryOp is a unary operator.
type UnaryOp int

const (
	UnarySub UnaryOp = iota // -
	UnaryNot                // !
	UnaryInv                // ~
)

var unarySymbol = [...]string{"-", "!", "~"}

func (op UnaryOp) String() string { return unarySymbol[op] }

// AssignOp is a compound assignment operator.
type AssignOp int

const (
	AssignSet AssignOp = iota // =
	AssignAdd                 // +=
	AssignSub                 // -=
	AssignMul                 // *=
	AssignDiv                 // /=
	AssignMod                 // %=
	AssignAnd                 // &=
	AssignOr                  // |=
	AssignXor                 // ^=
	AssignShl                 // <<=
	AssignShr                 // >>=
)

var assignSymbol = [...]string{"=", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>="}

func (op AssignOp) String() string { return assignSymbol[op] }

// CmpOp is a comparison/boolean operator.
type CmpOp int

const (
	CmpEqu CmpOp = iota // ==
	CmpNeq               // !=
	CmpLtn               // <
	CmpLte               // <=
	CmpGtn               // >
	CmpGte               // >=
)

var cmpSymbol = [...]string{"==", "!=", "<", "<=", ">", ">="}

func (op CmpOp) String() string { return cmpSymbol[op] }

// ChainOp is a short-circuit logical operator.
type ChainOp int

const (
	ChainAnd ChainOp = iota // &&
	ChainOr                  // ||
)

var chainSymbol = [...]string{"&&", "||"}

func (op ChainOp) String() string { return chainSymbol[op] }

// ============================================================
// Expressions — Expr interface
// ============================================================

// Expr is any expression node in the AST.
type Expr interface {
	exprNode()
	ExprLoc() Loc
}

// --- Atoms ---

// IntLit is an integer literal.
type IntLit struct {
	Loc Loc
	Val int32
}

// StrLit is a string literal with rich tokens.
type StrLit struct {
	Loc    Loc
	Tokens []StrToken // rich string tokens
}

// ResRef is a resource string reference: #res<key>.
type ResRef struct {
	Loc Loc
	Key string
}

// StoreRef is a reference to the special "store" register.
type StoreRef struct {
	Loc Loc
}

// --- Variables ---

// IntVar is an integer variable access: intA[expr], intB[expr], etc.
type IntVar struct {
	Loc     Loc
	Bank    int  // register bank (0x00=A, 0x01=B, ... 0x0b=L, etc.)
	Index   Expr // array index expression
}

// StrVar is a string variable access: strS[expr], strK[expr], strM[expr].
type StrVar struct {
	Loc   Loc
	Bank  int  // register bank (0x0a=K, 0x0c=M, 0x12=S)
	Index Expr // array index expression
}

// Deref is a named array dereference: ident[expr].
type Deref struct {
	Loc   Loc
	Ident string
	Index Expr
}

// VarOrFunc is an unresolved identifier (variable or zero-arg function).
type VarOrFunc struct {
	Loc   Loc
	Ident string
}

// --- Operators ---

// BinOp is a binary arithmetic expression: lhs op rhs.
type BinOp struct {
	Loc Loc
	LHS Expr
	Op  ArithOp
	RHS Expr
}

// CmpExpr is a comparison expression: lhs op rhs.
type CmpExpr struct {
	Loc Loc
	LHS Expr
	Op  CmpOp
	RHS Expr
}

// ChainExpr is a short-circuit logical expression: lhs && rhs or lhs || rhs.
type ChainExpr struct {
	Loc Loc
	LHS Expr
	Op  ChainOp
	RHS Expr
}

// UnaryExpr is a unary expression: -expr, !expr, ~expr.
type UnaryExpr struct {
	Loc Loc
	Op  UnaryOp
	Val Expr
}

// ParenExpr is a parenthesized expression: (expr).
type ParenExpr struct {
	Loc  Loc
	Expr Expr
}

// --- Function calls ---

// FuncCall is a function call expression: ident(params) or goto @label.
type FuncCall struct {
	Loc    Loc
	Ident  string
	Params []Param
	Label  *Label // optional goto target
}

// SelFuncCall is a select function call expression: select(options).
type SelFuncCall struct {
	Loc    Loc
	Ident  string
	Opcode int
	Window Expr // optional window expression
	Params []SelParam
}

// ExprSeq is a compiler-internal expression sequence (for debugging).
type ExprSeq struct {
	Loc   Loc
	Name  string
	Binds []ExprBind
	Stmts []Stmt
}

// ExprBind is a name=expr binding in an ExprSeq.
type ExprBind struct {
	Name string
	Expr Expr
}

// Expr interface markers
func (IntLit) exprNode()      {}
func (StrLit) exprNode()      {}
func (ResRef) exprNode()      {}
func (StoreRef) exprNode()    {}
func (IntVar) exprNode()      {}
func (StrVar) exprNode()      {}
func (Deref) exprNode()       {}
func (VarOrFunc) exprNode()   {}
func (BinOp) exprNode()       {}
func (CmpExpr) exprNode()     {}
func (ChainExpr) exprNode()   {}
func (UnaryExpr) exprNode()   {}
func (ParenExpr) exprNode()   {}
func (FuncCall) exprNode()    {}
func (SelFuncCall) exprNode() {}
func (ExprSeq) exprNode()     {}

func (e IntLit) ExprLoc() Loc      { return e.Loc }
func (e StrLit) ExprLoc() Loc      { return e.Loc }
func (e ResRef) ExprLoc() Loc      { return e.Loc }
func (e StoreRef) ExprLoc() Loc    { return e.Loc }
func (e IntVar) ExprLoc() Loc      { return e.Loc }
func (e StrVar) ExprLoc() Loc      { return e.Loc }
func (e Deref) ExprLoc() Loc       { return e.Loc }
func (e VarOrFunc) ExprLoc() Loc   { return e.Loc }
func (e BinOp) ExprLoc() Loc       { return e.Loc }
func (e CmpExpr) ExprLoc() Loc     { return e.Loc }
func (e ChainExpr) ExprLoc() Loc   { return e.Loc }
func (e UnaryExpr) ExprLoc() Loc   { return e.Loc }
func (e ParenExpr) ExprLoc() Loc   { return e.Loc }
func (e FuncCall) ExprLoc() Loc    { return e.Loc }
func (e SelFuncCall) ExprLoc() Loc { return e.Loc }
func (e ExprSeq) ExprLoc() Loc     { return e.Loc }

// ============================================================
// Rich string tokens inside StrLit
// ============================================================

// StrToken is one token inside a rich string literal.
// Defined here to avoid circular dependencies with the token package.
type StrToken interface {
	strToken()
}

type TextToken struct {
	Loc  Loc
	DBCS bool
	Text string
}

type SpaceToken struct {
	Loc   Loc
	Count int
}

type DQuoteToken struct{ Loc Loc }
type RCurToken struct{ Loc Loc }
type LLenticToken struct{ Loc Loc }
type RLenticToken struct{ Loc Loc }
type AsteriskToken struct{ Loc Loc }
type PercentToken struct{ Loc Loc }
type HyphenToken struct{ Loc Loc }
type SpeakerToken struct{ Loc Loc }
type DeleteToken struct{ Loc Loc }

type NameToken struct {
	Loc    Loc
	Global bool // false=\l (local), true=\m (global)
	Index  Expr
	CharID Expr // optional character index
}

type GlossToken struct {
	Loc      Loc
	IsRuby   bool      // false=\g, true=\ruby
	Base     []StrToken // base text tokens
	GlossKey string     // resource key (if =<key>)
	Gloss    []StrToken // inline gloss tokens (if ={...})
}

type CodeToken struct {
	Loc    Loc
	Ident  string
	OptArg Expr    // optional :arg
	Params []Param // {params}
}

type AddToken struct {
	Loc Loc
	Key string
}

type ResRefToken struct {
	Loc Loc
	Key string
}

type RewriteToken struct {
	Loc Loc
	Key int
}

func (TextToken) strToken()    {}
func (SpaceToken) strToken()   {}
func (DQuoteToken) strToken()  {}
func (RCurToken) strToken()    {}
func (LLenticToken) strToken() {}
func (RLenticToken) strToken() {}
func (AsteriskToken) strToken(){}
func (PercentToken) strToken() {}
func (HyphenToken) strToken()  {}
func (SpeakerToken) strToken() {}
func (DeleteToken) strToken()  {}
func (NameToken) strToken()    {}
func (GlossToken) strToken()   {}
func (CodeToken) strToken()    {}
func (AddToken) strToken()     {}
func (ResRefToken) strToken()  {}
func (RewriteToken) strToken() {}

// ============================================================
// Parameters
// ============================================================

// Param is a function call parameter.
type Param interface {
	paramNode()
}

// SimpleParam is a single expression parameter.
type SimpleParam struct {
	Loc  Loc
	Expr Expr
}

// ComplexParam is a tuple/braced parameter: {expr, expr, ...}.
type ComplexParam struct {
	Loc   Loc
	Exprs []Expr
}

// SpecialParam is a tagged special parameter.
type SpecialParam struct {
	Loc   Loc
	Tag   int
	Exprs []Expr
}

func (SimpleParam) paramNode()  {}
func (ComplexParam) paramNode() {}
func (SpecialParam) paramNode() {}

// --- Select parameters ---

// SelParam is a select menu option parameter.
type SelParam interface {
	selParamNode()
}

// AlwaysSelParam is an unconditional select option.
type AlwaysSelParam struct {
	Loc  Loc
	Expr Expr
}

// CondSelParam is a conditional select option with conditions.
type CondSelParam struct {
	Loc   Loc
	Conds []SelCond
	Expr  Expr
}

func (AlwaysSelParam) selParamNode() {}
func (CondSelParam) selParamNode()   {}

// SelCond is one condition in a conditional select parameter.
type SelCond struct {
	Loc   Loc
	Ident string
	Arg   Expr // optional argument
	Cond  Expr // optional if-condition
	IsFlag bool // true if just a flag (no arg/cond)
}

// ============================================================
// Labels
// ============================================================

// Label is a goto/gosub target: @identifier.
type Label struct {
	Loc   Loc
	Ident string
}

// ============================================================
// Statements — Stmt interface
// ============================================================

// Stmt is any statement node.
type Stmt interface {
	stmtNode()
	StmtLoc() Loc
}

// --- Simple statements ---

type HaltStmt struct{ Loc Loc }
type BreakStmt struct{ Loc Loc }
type ContinueStmt struct{ Loc Loc }

type LabelStmt struct {
	Loc   Loc
	Label Label
}

type ReturnStmt struct {
	Loc      Loc
	Explicit bool // true if "return expr", false if implicit text output
	Expr     Expr
}

type AssignStmt struct {
	Loc  Loc
	Dest Expr     // must be assignable (Store, IVar, SVar, Deref, VarOrFunc)
	Op   AssignOp
	Expr Expr
}

// --- Function call statements ---

type FuncCallStmt struct {
	Loc    Loc
	Dest   Expr    // optional assignment target (nil = ignored)
	Ident  string
	Params []Param
	Label  *Label  // optional goto target
}

type SelectStmt struct {
	Loc    Loc
	Dest   Expr     // assignment target (store or variable)
	Ident  string   // select variant name
	Opcode int      // select opcode
	Window Expr     // optional window expression
	Params []SelParam
}

type GotoOnStmt struct {
	Loc    Loc
	Ident  string // "goto_on" or "gosub_on"
	Expr   Expr
	Labels []Label
}

type GotoCaseStmt struct {
	Loc   Loc
	Ident string // "goto_case" or "gosub_case"
	Expr  Expr
	Cases []GotoCaseArm
}

type GotoCaseArm struct {
	IsDefault bool
	Expr      Expr  // nil for default
	Label     Label
}

type UnknownOpStmt struct {
	Loc       Loc
	OpIdent   string
	OpType    int
	OpModule  int
	OpCode    int
	Overload  int
	Params    []Param
}

type VarOrFuncStmt struct {
	Loc   Loc
	Ident string
}

type RawCodeStmt struct {
	Loc  Loc
	Elts []RawElt
}

type RawElt struct {
	Kind string // "bytes", "int", "ident"
	Str  string
	Int  int32
}

// --- Control flow ---

type IfStmt struct {
	Loc  Loc
	Cond Expr
	Then Stmt
	Else Stmt // nil if no else
}

type WhileStmt struct {
	Loc  Loc
	Cond Expr
	Body Stmt
}

type RepeatStmt struct {
	Loc  Loc
	Body []Stmt
	Cond Expr // till condition
}

type ForStmt struct {
	Loc  Loc
	Init []Stmt
	Cond Expr
	Step []Stmt
	Body Stmt
}

type CaseStmt struct {
	Loc     Loc
	Expr    Expr
	Arms    []CaseArm
	Default []Stmt // nil if no "other" clause
}

type CaseArm struct {
	Cond Expr
	Body []Stmt
}

type BlockStmt struct {
	Loc   Loc
	Stmts []Stmt
}

type SeqStmt struct {
	Stmts []Stmt
}

type HidingStmt struct {
	Loc   Loc
	Ident string
	Body  Stmt
}

// --- Declarations ---

type DeclStmt struct {
	Loc   Loc
	Type  DeclType
	Dirs  []DeclDir
	Vars  []VarDecl
}

type DeclType struct {
	IsStr    bool
	BitWidth int // 1, 2, 4, 8, 32 (only for int)
}

type DeclDir int

const (
	DirZero  DeclDir = iota // zero-initialize
	DirBlock                // block allocation
	DirExt                  // external
	DirLabel                // labelled
)

type VarDecl struct {
	Loc       Loc
	Ident     string
	ArraySize Expr      // nil=scalar, non-nil=array with optional size
	AutoArray bool      // [] without size
	Init      Expr      // scalar initializer
	ArrayInit []Expr    // array initializer {v1, v2, ...}
	AddrFrom  Expr      // -> from.to mapping
	AddrTo    Expr
}

// --- Directives ---

type DefineStmt struct {
	Loc    Loc
	Ident  string
	Scoped bool // true for #sdefine
	Value  Expr
}

type DConstStmt struct {
	Loc   Loc
	Ident string
	Kind  DConstKind // const, bind, ebind
	Value Expr
}

type DConstKind int

const (
	KindConst DConstKind = iota
	KindBind
	KindEBind
)

type DUndefStmt struct {
	Loc    Loc
	Idents []string
}

type DSetStmt struct {
	Loc      Loc
	Ident    string
	ReadOnly bool // false for #set (mutable), true for #redef
	Value    Expr
}

type DTargetStmt struct {
	Loc    Loc
	Target string
}

type DVersionStmt struct {
	Loc        Loc
	A, B, C, D Expr
}

type DirectiveStmt struct {
	Loc   Loc
	Name  string             // "file", "resource", "entrypoint", etc.
	Kind  DirectiveArgKind   // Int, Str, None
	Value Expr
}

type DirectiveArgKind int

const (
	DirArgNone DirectiveArgKind = iota
	DirArgInt
	DirArgStr
)

type LoadFileStmt struct {
	Loc  Loc
	Path Expr
}

type DInlineStmt struct {
	Loc    Loc
	Ident  string
	Scoped bool
	Params []InlineParam
	Body   Stmt
}

type InlineParam struct {
	Loc      Loc
	Ident    string
	Optional bool
	Default  Expr // nil if no default
}

type DForStmt struct {
	Loc   Loc
	Ident string
	From  Expr
	To    Expr
	Body  Stmt
}

type DIfStmt struct {
	Loc  Loc
	Cond Expr
	Body []Stmt
	Cont DIfCont // DElseStmt, DEndifStmt, or nested DIfStmt
}

type DIfCont interface {
	difCont()
}

type DElseStmt struct {
	Loc  Loc
	Body []Stmt
}

type DEndifStmt struct {
	Loc Loc
}

func (DIfStmt) difCont()    {}
func (DElseStmt) difCont()  {}
func (DEndifStmt) difCont() {}

// --- Stmt interface markers ---

func (HaltStmt) stmtNode()      {}
func (BreakStmt) stmtNode()     {}
func (ContinueStmt) stmtNode()  {}
func (LabelStmt) stmtNode()     {}
func (ReturnStmt) stmtNode()    {}
func (AssignStmt) stmtNode()    {}
func (FuncCallStmt) stmtNode()  {}
func (SelectStmt) stmtNode()    {}
func (GotoOnStmt) stmtNode()    {}
func (GotoCaseStmt) stmtNode()  {}
func (UnknownOpStmt) stmtNode() {}
func (VarOrFuncStmt) stmtNode() {}
func (RawCodeStmt) stmtNode()   {}
func (IfStmt) stmtNode()        {}
func (WhileStmt) stmtNode()     {}
func (RepeatStmt) stmtNode()    {}
func (ForStmt) stmtNode()       {}
func (CaseStmt) stmtNode()      {}
func (BlockStmt) stmtNode()     {}
func (SeqStmt) stmtNode()       {}
func (HidingStmt) stmtNode()    {}
func (DeclStmt) stmtNode()      {}
func (DefineStmt) stmtNode()    {}
func (DConstStmt) stmtNode()    {}
func (DUndefStmt) stmtNode()    {}
func (DSetStmt) stmtNode()      {}
func (DTargetStmt) stmtNode()   {}
func (DVersionStmt) stmtNode()  {}
func (DirectiveStmt) stmtNode() {}
func (LoadFileStmt) stmtNode()  {}
func (DInlineStmt) stmtNode()   {}
func (DForStmt) stmtNode()      {}
func (DIfStmt) stmtNode()       {}

func (s HaltStmt) StmtLoc() Loc      { return s.Loc }
func (s BreakStmt) StmtLoc() Loc     { return s.Loc }
func (s ContinueStmt) StmtLoc() Loc  { return s.Loc }
func (s LabelStmt) StmtLoc() Loc     { return s.Loc }
func (s ReturnStmt) StmtLoc() Loc    { return s.Loc }
func (s AssignStmt) StmtLoc() Loc    { return s.Loc }
func (s FuncCallStmt) StmtLoc() Loc  { return s.Loc }
func (s SelectStmt) StmtLoc() Loc    { return s.Loc }
func (s GotoOnStmt) StmtLoc() Loc    { return s.Loc }
func (s GotoCaseStmt) StmtLoc() Loc  { return s.Loc }
func (s UnknownOpStmt) StmtLoc() Loc { return s.Loc }
func (s VarOrFuncStmt) StmtLoc() Loc { return s.Loc }
func (s RawCodeStmt) StmtLoc() Loc   { return s.Loc }
func (s IfStmt) StmtLoc() Loc        { return s.Loc }
func (s WhileStmt) StmtLoc() Loc     { return s.Loc }
func (s RepeatStmt) StmtLoc() Loc    { return s.Loc }
func (s ForStmt) StmtLoc() Loc       { return s.Loc }
func (s CaseStmt) StmtLoc() Loc      { return s.Loc }
func (s BlockStmt) StmtLoc() Loc     { return s.Loc }
func (SeqStmt) StmtLoc() Loc         { return Nowhere }
func (HidingStmt) StmtLoc() Loc      { return Nowhere }
func (s DeclStmt) StmtLoc() Loc      { return s.Loc }
func (s DefineStmt) StmtLoc() Loc    { return s.Loc }
func (s DConstStmt) StmtLoc() Loc    { return s.Loc }
func (s DUndefStmt) StmtLoc() Loc    { return s.Loc }
func (s DSetStmt) StmtLoc() Loc      { return s.Loc }
func (s DTargetStmt) StmtLoc() Loc   { return s.Loc }
func (s DVersionStmt) StmtLoc() Loc  { return s.Loc }
func (s DirectiveStmt) StmtLoc() Loc { return s.Loc }
func (s LoadFileStmt) StmtLoc() Loc  { return s.Loc }
func (s DInlineStmt) StmtLoc() Loc   { return s.Loc }
func (s DForStmt) StmtLoc() Loc      { return s.Loc }
func (s DIfStmt) StmtLoc() Loc       { return s.Loc }

// ============================================================
// Source file (top-level)
// ============================================================

// SourceFile is a parsed .org source file.
type SourceFile struct {
	Name  string
	Stmts []Stmt
}

// ============================================================
// Utilities (from keAst.ml utility functions)
// ============================================================

// VariableName returns the Kepago name for a register bank index.
// Matches OCaml's variable_name function.
func VariableName(bank int) string {
	switch bank {
	case 0x00: return "intA";   case 0x01: return "intB"
	case 0x02: return "intC";   case 0x03: return "intD"
	case 0x04: return "intE";   case 0x05: return "intF"
	case 0x06: return "intG";   case 0x19: return "intZ"
	case 0x0a: return "strK";   case 0x0b: return "intL"
	case 0x0c: return "strM";   case 0x12: return "strS"
	case 0x1a: return "intAb";  case 0x1b: return "intBb"
	case 0x1c: return "intCb";  case 0x1d: return "intDb"
	case 0x1e: return "intEb";  case 0x1f: return "intFb"
	case 0x20: return "intGb";  case 0x33: return "intZb"
	case 0x34: return "intA2b"; case 0x35: return "intB2b"
	case 0x36: return "intC2b"; case 0x37: return "intD2b"
	case 0x38: return "intE2b"; case 0x39: return "intF2b"
	case 0x3a: return "intG2b"; case 0x4d: return "intZ2b"
	case 0x4e: return "intA4b"; case 0x4f: return "intB4b"
	case 0x50: return "intC4b"; case 0x51: return "intD4b"
	case 0x52: return "intE4b"; case 0x53: return "intF4b"
	case 0x54: return "intG4b"; case 0x67: return "intZ4b"
	case 0x68: return "intA8b"; case 0x69: return "intB8b"
	case 0x6a: return "intC8b"; case 0x6b: return "intD8b"
	case 0x6c: return "intE8b"; case 0x6d: return "intF8b"
	case 0x6e: return "intG8b"; case 0x81: return "intZ8b"
	}
	return fmt.Sprintf("var_%02x", bank)
}

// IsConst returns true if the expression is a compile-time constant.
func IsConst(e Expr) bool {
	switch x := e.(type) {
	case IntLit, StrLit:
		return true
	case ParenExpr:
		return IsConst(x.Expr)
	}
	return false
}

// IsStore returns true if the expression is a store register reference.
func IsStore(e Expr) bool {
	_, ok := e.(StoreRef)
	return ok
}

// ExprType classifies an expression as integer, string, literal, or invalid.
type ExprType int

const (
	TypeInt     ExprType = iota // integer-valued
	TypeStr                     // string variable
	TypeLiteral                 // string literal
	TypeInvalid                 // unresolved
)

// TypeOf returns the type of a normalised expression.
func TypeOf(e Expr) ExprType {
	switch x := e.(type) {
	case IntLit, StoreRef, IntVar, CmpExpr, ChainExpr, UnaryExpr:
		return TypeInt
	case StrLit:
		return TypeLiteral
	case StrVar:
		return TypeStr
	case BinOp:
		return TypeOf(x.LHS)
	case ParenExpr:
		return TypeOf(x.Expr)
	}
	return TypeInvalid
}

// DeclDirString returns the string representation of a declaration directive.
func DeclDirString(d DeclDir) string {
	switch d {
	case DirZero:  return "zero"
	case DirBlock: return "block"
	case DirExt:   return "ext"
	case DirLabel: return "labelled"
	}
	return "?"
}
