// Package parser implements a recursive descent parser for the Kepago language.
// Transposed from OCaml's rlc/keAstParser.mly (~569 lines).
//
// Grammar overview (operator precedence low → high):
//   expr1: || (LOR)
//   expr2: && (LAND)
//   expr3: == != (equality)
//   expr4: < <= > >= (comparison)
//   expr5: + - | ^ (add/bitwise)
//   expr6: * / % & (mul/bitwise)
//   expr7: << >> (shift)
//   expr8: unary (- ! ~)
//   expr9: primary (int, string, variable, funcall, parens)
//
// Statement types: halt, break, continue, label, return, assignment,
//   function call, if/else, while, repeat/till, for, case/of/ecase,
//   declarations (int/str), directives (#define/#if/#for/etc.),
//   goto_on/goto_case, select, raw/endraw, op<...>
package parser

import (
	"fmt"

	"github.com/yoremi/rldev-go/rlc/pkg/ast"
	"github.com/yoremi/rldev-go/rlc/pkg/lexer"
	"github.com/yoremi/rldev-go/rlc/pkg/token"
)

// Parser parses Kepago tokens into AST nodes.
type Parser struct {
	lex *lexer.Lexer
	cur token.Token
}

// New creates a parser from a lexer.
func New(l *lexer.Lexer) *Parser {
	p := &Parser{lex: l}
	p.advance()
	return p
}

// ParseFile is a convenience: lex + parse source into a SourceFile.
func ParseFile(src []byte, filename string) (*ast.SourceFile, error) {
	l := lexer.New(string(src), filename)
	p := New(l)
	return p.ParseProgram(), nil
}

func (p *Parser) advance() token.Token {
	prev := p.cur
	p.cur = p.lex.Next()
	return prev
}

func (p *Parser) expect(t token.Type) token.Token {
	if p.cur.Type != t {
		panic(fmt.Sprintf("%s:%d: expected %s, got %s", p.cur.File, p.cur.Line, t, p.cur.Type))
	}
	return p.advance()
}

func (p *Parser) match(t token.Type) bool {
	if p.cur.Type == t {
		p.advance()
		return true
	}
	return false
}

func (p *Parser) loc() ast.Loc {
	return ast.Loc{File: p.cur.File, Line: p.cur.Line}
}

// ============================================================
// Entry points
// ============================================================

// ParseProgram parses a full source file.
func (p *Parser) ParseProgram() *ast.SourceFile {
	stmts := p.parseStatements()
	return &ast.SourceFile{Name: p.cur.File, Stmts: stmts}
}

// ParseExpression parses a single expression (for #if evaluation etc.).
func (p *Parser) ParseExpression() ast.Expr {
	return p.parseExpr()
}

// ============================================================
// Statements
// ============================================================

func (p *Parser) parseStatements() []ast.Stmt {
	var stmts []ast.Stmt
	for p.cur.Type != token.EOF && p.cur.Type != token.DEOF {
		if p.match(token.COMMA) { continue }
		s := p.parseStatement()
		if s != nil {
			stmts = append(stmts, s)
		}
	}
	return stmts
}

func (p *Parser) parseStatementsUntil(stops ...token.Type) []ast.Stmt {
	var stmts []ast.Stmt
	for {
		for _, s := range stops {
			if p.cur.Type == s { return stmts }
		}
		if p.cur.Type == token.EOF { return stmts }
		if p.match(token.COMMA) { continue }
		s := p.parseStatement()
		if s != nil { stmts = append(stmts, s) }
	}
}

func (p *Parser) parseStatement() ast.Stmt {
	loc := p.loc()
	switch p.cur.Type {
	case token.DHALT:
		p.advance(); return ast.HaltStmt{Loc: loc}
	case token.BREAK:
		p.advance(); return ast.BreakStmt{Loc: loc}
	case token.CONTINUE:
		p.advance(); return ast.ContinueStmt{Loc: loc}
	case token.LABEL:
		name := p.cur.StrVal; p.advance()
		return ast.LabelStmt{Loc: loc, Label: ast.Label{Loc: loc, Ident: name}}
	case token.RETURN:
		p.advance()
		if isExprStart(p.cur.Type) {
			expr := p.parseExpr()
			return ast.ReturnStmt{Loc: loc, Explicit: true, Expr: expr}
		}
		return ast.ReturnStmt{Loc: loc, Explicit: true}
	case token.RAW:
		return p.parseRaw()
	case token.IF:
		return p.parseIf()
	case token.WHILE:
		return p.parseWhile()
	case token.REPEAT:
		return p.parseRepeat()
	case token.FOR:
		return p.parseFor()
	case token.CASE:
		return p.parseCase()
	case token.COLON:
		return p.parseBlock()
	case token.DIF, token.DIFDEF:
		return p.parseDIf()
	case token.DFOR:
		return p.parseDFor()
	case token.DDEFINE:
		return p.parseDefine()
	case token.DUNDEF:
		return p.parseUndef()
	case token.DSET:
		return p.parseDSet()
	case token.DWITHEXPR:
		return p.parseDWithExpr()
	case token.DTARGET:
		return p.parseDTarget()
	case token.DVERSION:
		return p.parseDVersion()
	case token.DLOAD:
		p.advance(); return ast.LoadFileStmt{Loc: loc, Path: p.parseExpr()}
	case token.DINLINE:
		return p.parseDInline()
	case token.DHIDING:
		return p.parseDHiding()
	case token.OP:
		return p.parseUnknownOp()
	case token.GO_LIST:
		return p.parseGotoOn()
	case token.GO_CASE:
		return p.parseGotoCase()
	case token.SELECT:
		return p.parseSelect()
	case token.INT, token.STR:
		return p.parseDecl()
	default:
		return p.parseExprOrAssign()
	}
}

// ============================================================
// Control flow
// ============================================================

func (p *Parser) parseIf() ast.Stmt {
	loc := p.loc(); p.expect(token.IF)
	cond := p.parseExpr()
	then := p.parseCStatement()
	var els ast.Stmt
	if p.match(token.ELSE) { els = p.parseCStatement() }
	return ast.IfStmt{Loc: loc, Cond: cond, Then: then, Else: els}
}

func (p *Parser) parseWhile() ast.Stmt {
	loc := p.loc(); p.expect(token.WHILE)
	cond := p.parseExpr()
	body := p.parseCStatement()
	return ast.WhileStmt{Loc: loc, Cond: cond, Body: body}
}

func (p *Parser) parseRepeat() ast.Stmt {
	loc := p.loc(); p.expect(token.REPEAT)
	body := p.parseStatementsUntil(token.TILL)
	p.expect(token.TILL)
	cond := p.parseExpr()
	return ast.RepeatStmt{Loc: loc, Body: body, Cond: cond}
}

func (p *Parser) parseFor() ast.Stmt {
	loc := p.loc(); p.expect(token.FOR); p.expect(token.LPAR)
	init := p.parseStatementsUntil(token.SEMI)
	p.expect(token.SEMI)
	cond := p.parseExpr()
	// forsep: ; or )(
	if p.match(token.SEMI) { /* ok */ } else { p.expect(token.RPAR); p.expect(token.LPAR) }
	step := p.parseStatementsUntil(token.RPAR)
	p.expect(token.RPAR)
	body := p.parseCStatement()
	return ast.ForStmt{Loc: loc, Init: init, Cond: cond, Step: step, Body: body}
}

func (p *Parser) parseCase() ast.Stmt {
	loc := p.loc(); p.expect(token.CASE)
	expr := p.parseExpr()
	var arms []ast.CaseArm
	var def []ast.Stmt
	for p.cur.Type != token.ECASE && p.cur.Type != token.EOF {
		if p.match(token.COMMA) { continue }
		if p.cur.Type == token.OF {
			p.advance()
			val := p.parseExpr()
			body := p.parseStatementsUntil(token.OF, token.OTHER, token.ECASE)
			arms = append(arms, ast.CaseArm{Cond: val, Body: body})
		} else if p.cur.Type == token.OTHER {
			p.advance()
			def = p.parseStatementsUntil(token.ECASE)
		} else {
			break
		}
	}
	p.match(token.ECASE)
	return ast.CaseStmt{Loc: loc, Expr: expr, Arms: arms, Default: def}
}

func (p *Parser) parseBlock() ast.Stmt {
	loc := p.loc(); p.expect(token.COLON)
	stmts := p.parseStatementsUntil(token.SEMI)
	p.expect(token.SEMI)
	return ast.BlockStmt{Loc: loc, Stmts: stmts}
}

func (p *Parser) parseRaw() ast.Stmt {
	loc := p.loc(); p.expect(token.RAW)
	var elts []ast.RawElt
	for p.cur.Type != token.ENDRAW && p.cur.Type != token.EOF {
		switch p.cur.Type {
		case token.IDENT:
			elts = append(elts, ast.RawElt{Kind: "ident", Str: p.cur.StrVal})
			p.advance()
		case token.INTEGER:
			elts = append(elts, ast.RawElt{Kind: "int", Int: p.cur.IntVal})
			p.advance()
		case token.VAR, token.SVAR, token.REG:
			elts = append(elts, ast.RawElt{Kind: "bytes", Str: fmt.Sprintf("$%c", rune(p.cur.IntVal))})
			p.advance()
		case token.COMMA:
			elts = append(elts, ast.RawElt{Kind: "bytes", Str: ","}); p.advance()
		case token.LPAR:
			elts = append(elts, ast.RawElt{Kind: "bytes", Str: "("}); p.advance()
		case token.RPAR:
			elts = append(elts, ast.RawElt{Kind: "bytes", Str: ")"}); p.advance()
		case token.LCUR:
			elts = append(elts, ast.RawElt{Kind: "bytes", Str: "{"}); p.advance()
		case token.RCUR:
			elts = append(elts, ast.RawElt{Kind: "bytes", Str: "}"}); p.advance()
		case token.LSQU:
			elts = append(elts, ast.RawElt{Kind: "bytes", Str: "["}); p.advance()
		case token.RSQU:
			elts = append(elts, ast.RawElt{Kind: "bytes", Str: "]"}); p.advance()
		default:
			p.advance()
		}
	}
	p.match(token.ENDRAW)
	return ast.RawCodeStmt{Loc: loc, Elts: elts}
}

// parseCStatement: statement or ,statement (skip leading comma)
func (p *Parser) parseCStatement() ast.Stmt {
	p.match(token.COMMA)
	return p.parseStatement()
}

// ============================================================
// Directives
// ============================================================

func (p *Parser) parseDIf() ast.Stmt {
	loc := p.loc(); p.advance() // skip #if / #ifdef / #ifndef
	cond := p.parseExpr()
	body := p.parseStatementsUntil(token.DELSE, token.DELSEIF, token.DENDIF)
	var cont ast.DIfCont
	if p.match(token.DENDIF) {
		cont = ast.DEndifStmt{Loc: p.loc()}
	} else if p.match(token.DELSE) {
		elseBody := p.parseStatementsUntil(token.DENDIF)
		p.match(token.DENDIF)
		cont = ast.DElseStmt{Loc: p.loc(), Body: elseBody}
	} else if p.cur.Type == token.DELSEIF {
		cont = p.parseDIf().(ast.DIfStmt)
	} else {
		cont = ast.DEndifStmt{Loc: p.loc()}
	}
	return ast.DIfStmt{Loc: loc, Cond: cond, Body: body, Cont: cont}
}

func (p *Parser) parseDFor() ast.Stmt {
	loc := p.loc(); p.expect(token.DFOR)
	name := p.expect(token.IDENT).StrVal
	p.expect(token.SET)
	from := p.parseExpr()
	p.expect(token.POINT); p.expect(token.POINT)
	to := p.parseExpr()
	body := p.parseCStatement()
	return ast.DForStmt{Loc: loc, Ident: name, From: from, To: to, Body: body}
}

func (p *Parser) parseDefine() ast.Stmt {
	loc := p.loc()
	kind := p.cur.StrVal // "define", "sdefine", "const", "bind", "ebind", "redefine"
	p.advance()
	if p.cur.Type != token.IDENT { return nil }
	name := p.cur.StrVal; p.advance()
	var val ast.Expr
	if p.match(token.SET) {
		val = p.parseExpr()
	} else {
		val = ast.IntLit{Loc: loc, Val: 1}
	}
	switch kind {
	case "const":
		return ast.DConstStmt{Loc: loc, Ident: name, Kind: ast.KindConst, Value: val}
	case "bind":
		return ast.DConstStmt{Loc: loc, Ident: name, Kind: ast.KindBind, Value: val}
	case "ebind":
		return ast.DConstStmt{Loc: loc, Ident: name, Kind: ast.KindEBind, Value: val}
	case "redefine":
		return ast.DSetStmt{Loc: loc, Ident: name, Value: val}
	default:
		return ast.DefineStmt{Loc: loc, Ident: name, Scoped: kind == "sdefine", Value: val}
	}
}

func (p *Parser) parseUndef() ast.Stmt {
	loc := p.loc(); p.expect(token.DUNDEF)
	var names []string
	for p.cur.Type == token.IDENT {
		names = append(names, p.cur.StrVal); p.advance()
		p.match(token.COMMA)
	}
	return ast.DUndefStmt{Loc: loc, Idents: names}
}

func (p *Parser) parseDSet() ast.Stmt {
	loc := p.loc(); p.expect(token.DSET)
	name := p.expect(token.IDENT).StrVal
	op := p.parseAssignOp()
	val := p.parseExpr()
	_ = op // simplified: always treat as set
	return ast.DSetStmt{Loc: loc, Ident: name, ReadOnly: true, Value: val}
}

func (p *Parser) parseDWithExpr() ast.Stmt {
	loc := p.loc()
	name := p.cur.StrVal; p.advance()
	val := p.parseExpr()
	return ast.DirectiveStmt{Loc: loc, Name: name, Value: val}
}

func (p *Parser) parseDTarget() ast.Stmt {
	loc := p.loc(); p.expect(token.DTARGET)
	name := p.expect(token.IDENT).StrVal
	return ast.DTargetStmt{Loc: loc, Target: name}
}

func (p *Parser) parseDVersion() ast.Stmt {
	loc := p.loc(); p.expect(token.DVERSION)
	a := p.parseExpr()
	zero := ast.IntLit{Loc: loc, Val: 0}
	b, c, d := ast.Expr(zero), ast.Expr(zero), ast.Expr(zero)
	if p.match(token.POINT) {
		b = p.parseExpr()
		if p.match(token.POINT) {
			c = p.parseExpr()
			if p.match(token.POINT) {
				d = p.parseExpr()
			}
		}
	}
	return ast.DVersionStmt{Loc: loc, A: a, B: b, C: c, D: d}
}

func (p *Parser) parseDInline() ast.Stmt {
	loc := p.loc(); p.expect(token.DINLINE)
	name := p.expect(token.IDENT).StrVal
	p.expect(token.LPAR)
	var params []ast.InlineParam
	for p.cur.Type != token.RPAR && p.cur.Type != token.EOF {
		if p.cur.Type == token.LSQU {
			p.advance()
			pname := p.expect(token.IDENT).StrVal
			p.expect(token.RSQU)
			params = append(params, ast.InlineParam{Loc: p.loc(), Ident: pname, Optional: true})
		} else if p.cur.Type == token.IDENT {
			pname := p.cur.StrVal; p.advance()
			var def ast.Expr
			if p.match(token.SET) { def = p.parseExpr() }
			params = append(params, ast.InlineParam{Loc: p.loc(), Ident: pname, Default: def})
		}
		p.match(token.COMMA)
	}
	p.expect(token.RPAR)
	body := p.parseStatement()
	return ast.DInlineStmt{Loc: loc, Ident: name, Params: params, Body: body}
}

func (p *Parser) parseDHiding() ast.Stmt {
	loc := p.loc(); p.expect(token.DHIDING)
	name := p.expect(token.IDENT).StrVal
	body := p.parseCStatement()
	return ast.HidingStmt{Loc: loc, Ident: name, Body: body}
}

// ============================================================
// Goto/select
// ============================================================

func (p *Parser) parseGotoOn() ast.Stmt {
	loc := p.loc()
	ident := p.cur.StrVal; p.advance()
	expr := p.parseExpr()
	p.expect(token.LCUR)
	var labels []ast.Label
	for p.cur.Type != token.RCUR && p.cur.Type != token.EOF {
		if p.cur.Type == token.LABEL {
			labels = append(labels, ast.Label{Loc: p.loc(), Ident: p.cur.StrVal})
			p.advance()
		}
		p.match(token.COMMA)
	}
	p.match(token.RCUR)
	return ast.GotoOnStmt{Loc: loc, Ident: ident, Expr: expr, Labels: labels}
}

func (p *Parser) parseGotoCase() ast.Stmt {
	loc := p.loc()
	ident := p.cur.StrVal; p.advance()
	expr := p.parseExpr()
	p.expect(token.LCUR)
	var cases []ast.GotoCaseArm
	for p.cur.Type != token.RCUR && p.cur.Type != token.EOF {
		if p.cur.Type == token.USCORE {
			// default: _: @label
			p.advance(); p.expect(token.COLON)
			lbl := p.expect(token.LABEL)
			cases = append(cases, ast.GotoCaseArm{IsDefault: true, Label: ast.Label{Loc: p.loc(), Ident: lbl.StrVal}})
		} else {
			val := p.parseExpr()
			p.expect(token.COLON)
			lbl := p.expect(token.LABEL)
			cases = append(cases, ast.GotoCaseArm{Expr: val, Label: ast.Label{Loc: p.loc(), Ident: lbl.StrVal}})
		}
		p.match(token.SEMI)
	}
	p.match(token.RCUR)
	return ast.GotoCaseStmt{Loc: loc, Ident: ident, Expr: expr, Cases: cases}
}

func (p *Parser) parseSelect() ast.Stmt {
	loc := p.loc()
	tok := p.advance() // consume SELECT
	var window ast.Expr
	if p.match(token.LSQU) {
		window = p.parseExpr()
		p.expect(token.RSQU)
	}
	var params []ast.SelParam
	if p.match(token.LPAR) {
		params = p.parseSelParamList()
		p.expect(token.RPAR)
	}
	return ast.SelectStmt{Loc: loc, Dest: ast.StoreRef{Loc: loc}, Ident: tok.StrVal, Opcode: int(tok.IntVal), Window: window, Params: params}
}

func (p *Parser) parseSelParamList() []ast.SelParam {
	var params []ast.SelParam
	if p.cur.Type == token.RPAR { return params }
	params = append(params, p.parseSelParam())
	for p.match(token.COMMA) {
		params = append(params, p.parseSelParam())
	}
	return params
}

func (p *Parser) parseSelParam() ast.SelParam {
	loc := p.loc()
	expr := p.parseExpr()
	if p.cur.Type != token.COLON {
		return ast.AlwaysSelParam{Loc: loc, Expr: expr}
	}
	// conditional: conds : expr
	// Not fully implemented — return as always for simplicity
	p.advance()
	val := p.parseExpr()
	return ast.AlwaysSelParam{Loc: loc, Expr: val}
}

// ============================================================
// Unknown op: op<type:module:code,overload>(params)
// ============================================================

func (p *Parser) parseUnknownOp() ast.Stmt {
	loc := p.loc(); p.expect(token.OP)
	p.expect(token.LTN)
	opType := int(p.expect(token.INTEGER).IntVal)
	p.parseSep()
	opModule := 0
	if p.cur.Type == token.INTEGER {
		opModule = int(p.cur.IntVal); p.advance()
	} else if p.cur.Type == token.IDENT {
		_ = p.cur.StrVal; p.advance() // module name lookup would go here
	}
	p.parseSep()
	opCode := int(p.expect(token.INTEGER).IntVal)
	p.parseSep()
	overload := int(p.expect(token.INTEGER).IntVal)
	p.expect(token.GTN)
	var params []ast.Param
	if p.match(token.LPAR) {
		params = p.parseParamList()
		p.expect(token.RPAR)
	}
	return ast.UnknownOpStmt{Loc: loc, OpType: opType, OpModule: opModule, OpCode: opCode, Overload: overload, Params: params}
}

func (p *Parser) parseSep() {
	switch p.cur.Type {
	case token.COLON, token.COMMA, token.POINT, token.SEMI, token.SUB, token.DIV:
		p.advance()
	}
}

// ============================================================
// Declarations: int/str varname[size] = init
// ============================================================

func (p *Parser) parseDecl() ast.Stmt {
	loc := p.loc()
	var dt ast.DeclType
	if p.cur.Type == token.STR {
		dt.IsStr = true; p.advance()
	} else {
		dt.BitWidth = int(p.cur.IntVal); p.advance()
	}
	// Optional decldirs: (zero, block, ext, labelled)
	var dirs []ast.DeclDir
	if p.match(token.LPAR) {
		for p.cur.Type != token.RPAR && p.cur.Type != token.EOF {
			if p.cur.Type == token.IDENT {
				switch p.cur.StrVal {
				case "zero": dirs = append(dirs, ast.DirZero)
				case "block": dirs = append(dirs, ast.DirBlock)
				case "ext": dirs = append(dirs, ast.DirExt)
				case "labelled", "labeled": dirs = append(dirs, ast.DirLabel)
				}
				p.advance()
			}
			p.match(token.COMMA)
		}
		p.match(token.RPAR)
	}
	var vars []ast.VarDecl
	vars = append(vars, p.parseVarDecl())
	for p.match(token.COMMA) {
		vars = append(vars, p.parseVarDecl())
	}
	return ast.DeclStmt{Loc: loc, Type: dt, Dirs: dirs, Vars: vars}
}

func (p *Parser) parseVarDecl() ast.VarDecl {
	loc := p.loc()
	name := p.expect(token.IDENT).StrVal
	vd := ast.VarDecl{Loc: loc, Ident: name}
	// arraydecl: empty | [] | [expr]
	if p.match(token.LSQU) {
		if p.match(token.RSQU) {
			vd.AutoArray = true
		} else {
			vd.ArraySize = p.parseExpr()
			p.expect(token.RSQU)
		}
	}
	// initdecl: = value or -> addr
	if p.match(token.SET) {
		if p.cur.Type == token.LCUR {
			// Array init: = {v1, v2, ...}
			p.advance()
			for p.cur.Type != token.RCUR && p.cur.Type != token.EOF {
				vd.ArrayInit = append(vd.ArrayInit, p.parseExpr())
				p.match(token.COMMA)
			}
			p.match(token.RCUR)
		} else {
			vd.Init = p.parseExpr()
		}
	}
	if p.match(token.ARROW) {
		vd.AddrFrom = p.parseExpr()
		p.expect(token.POINT)
		vd.AddrTo = p.parseExpr()
	}
	return vd
}

// ============================================================
// Expressions / assignments
// ============================================================

func (p *Parser) parseExprOrAssign() ast.Stmt {
	loc := p.loc()
	expr := p.parseExpr()

	// Check for assignment operators
	if p.cur.Type.IsAssignOp() {
		op := p.parseAssignOp()
		rhs := p.parseExpr()
		return ast.AssignStmt{Loc: loc, Dest: expr, Op: op, Expr: rhs}
	}

	// Function call as statement
	if fc, ok := expr.(ast.FuncCall); ok {
		return ast.FuncCallStmt{Loc: loc, Ident: fc.Ident, Params: fc.Params, Label: fc.Label}
	}
	// Select as statement
	if sf, ok := expr.(ast.SelFuncCall); ok {
		return ast.SelectStmt{Loc: loc, Dest: ast.StoreRef{Loc: loc}, Ident: sf.Ident, Opcode: sf.Opcode, Window: sf.Window, Params: sf.Params}
	}
	// VarOrFunc as statement
	if vf, ok := expr.(ast.VarOrFunc); ok {
		return ast.VarOrFuncStmt{Loc: loc, Ident: vf.Ident}
	}
	// Implicit return/textout
	return ast.ReturnStmt{Loc: loc, Explicit: false, Expr: expr}
}

func (p *Parser) parseAssignOp() ast.AssignOp {
	var op ast.AssignOp
	switch p.cur.Type {
	case token.SET:  op = ast.AssignSet
	case token.SADD: op = ast.AssignAdd
	case token.SSUB: op = ast.AssignSub
	case token.SMUL: op = ast.AssignMul
	case token.SDIV: op = ast.AssignDiv
	case token.SMOD: op = ast.AssignMod
	case token.SAND: op = ast.AssignAnd
	case token.SOR:  op = ast.AssignOr
	case token.SXOR: op = ast.AssignXor
	case token.SSHL: op = ast.AssignShl
	case token.SSHR: op = ast.AssignShr
	}
	p.advance()
	return op
}

// ============================================================
// Expression parsing (precedence climbing)
// ============================================================

func (p *Parser) parseExpr() ast.Expr { return p.parseExprOr() }

// continueExprFrom restarts the precedence-climbing chain at the
// primary level using the given expression as the initial LHS. This
// is needed when parseParam has already pulled a `(expr)` out of the
// token stream (to disambiguate it from a complex tuple `(a, b, c)`)
// and the surrounding context might still extend that expression with
// binary or comparison operators — e.g.
//   goto_unless ((cond1) || (cond2)) @label
// where the outer `||` must attach to the freshly built ParenExpr.
func (p *Parser) continueExprFrom(atom ast.Expr) ast.Expr {
	return p.contOr(p.contAnd(p.contEq(p.contCmp(p.contAdd(p.contMul(p.contShift(atom)))))))
}

func (p *Parser) contShift(lhs ast.Expr) ast.Expr {
	for {
		loc := p.loc()
		switch p.cur.Type {
		case token.SHL:
			p.advance()
			lhs = ast.BinOp{Loc: loc, LHS: lhs, Op: ast.OpShl, RHS: p.parseUnary()}
		case token.SHR:
			p.advance()
			lhs = ast.BinOp{Loc: loc, LHS: lhs, Op: ast.OpShr, RHS: p.parseUnary()}
		default:
			return lhs
		}
	}
}

func (p *Parser) contMul(lhs ast.Expr) ast.Expr {
	for {
		loc := p.loc()
		switch p.cur.Type {
		case token.MUL:
			p.advance()
			lhs = ast.BinOp{Loc: loc, LHS: lhs, Op: ast.OpMul, RHS: p.parseExprShift()}
		case token.DIV:
			p.advance()
			lhs = ast.BinOp{Loc: loc, LHS: lhs, Op: ast.OpDiv, RHS: p.parseExprShift()}
		case token.MOD:
			p.advance()
			lhs = ast.BinOp{Loc: loc, LHS: lhs, Op: ast.OpMod, RHS: p.parseExprShift()}
		case token.AND:
			p.advance()
			lhs = ast.BinOp{Loc: loc, LHS: lhs, Op: ast.OpAnd, RHS: p.parseExprShift()}
		default:
			return lhs
		}
	}
}

func (p *Parser) contAdd(lhs ast.Expr) ast.Expr {
	for {
		loc := p.loc()
		switch p.cur.Type {
		case token.ADD:
			p.advance()
			lhs = ast.BinOp{Loc: loc, LHS: lhs, Op: ast.OpAdd, RHS: p.parseExprMul()}
		case token.SUB:
			p.advance()
			lhs = ast.BinOp{Loc: loc, LHS: lhs, Op: ast.OpSub, RHS: p.parseExprMul()}
		case token.OR:
			p.advance()
			lhs = ast.BinOp{Loc: loc, LHS: lhs, Op: ast.OpOr, RHS: p.parseExprMul()}
		case token.XOR:
			p.advance()
			lhs = ast.BinOp{Loc: loc, LHS: lhs, Op: ast.OpXor, RHS: p.parseExprMul()}
		default:
			return lhs
		}
	}
}

func (p *Parser) contCmp(lhs ast.Expr) ast.Expr {
	for {
		loc := p.loc()
		switch p.cur.Type {
		case token.LTN:
			p.advance()
			lhs = ast.CmpExpr{Loc: loc, LHS: lhs, Op: ast.CmpLtn, RHS: p.parseExprAdd()}
		case token.LTE:
			p.advance()
			lhs = ast.CmpExpr{Loc: loc, LHS: lhs, Op: ast.CmpLte, RHS: p.parseExprAdd()}
		case token.GTN:
			p.advance()
			lhs = ast.CmpExpr{Loc: loc, LHS: lhs, Op: ast.CmpGtn, RHS: p.parseExprAdd()}
		case token.GTE:
			p.advance()
			lhs = ast.CmpExpr{Loc: loc, LHS: lhs, Op: ast.CmpGte, RHS: p.parseExprAdd()}
		default:
			return lhs
		}
	}
}

func (p *Parser) contEq(lhs ast.Expr) ast.Expr {
	for {
		loc := p.loc()
		switch p.cur.Type {
		case token.EQU:
			p.advance()
			lhs = ast.CmpExpr{Loc: loc, LHS: lhs, Op: ast.CmpEqu, RHS: p.parseExprCmp()}
		case token.NEQ:
			p.advance()
			lhs = ast.CmpExpr{Loc: loc, LHS: lhs, Op: ast.CmpNeq, RHS: p.parseExprCmp()}
		default:
			return lhs
		}
	}
}

func (p *Parser) contAnd(lhs ast.Expr) ast.Expr {
	for p.cur.Type == token.LAND {
		loc := p.loc()
		p.advance()
		rhs := p.parseExprEq()
		lhs = ast.ChainExpr{Loc: loc, LHS: lhs, Op: ast.ChainAnd, RHS: rhs}
	}
	return lhs
}

func (p *Parser) contOr(lhs ast.Expr) ast.Expr {
	for p.cur.Type == token.LOR {
		loc := p.loc()
		p.advance()
		rhs := p.parseExprAnd()
		lhs = ast.ChainExpr{Loc: loc, LHS: lhs, Op: ast.ChainOr, RHS: rhs}
	}
	return lhs
}

// expr1: || (lowest)
func (p *Parser) parseExprOr() ast.Expr {
	lhs := p.parseExprAnd()
	for p.cur.Type == token.LOR {
		loc := p.loc(); p.advance()
		rhs := p.parseExprAnd()
		lhs = ast.ChainExpr{Loc: loc, LHS: lhs, Op: ast.ChainOr, RHS: rhs}
	}
	return lhs
}

// expr2: &&
func (p *Parser) parseExprAnd() ast.Expr {
	lhs := p.parseExprEq()
	for p.cur.Type == token.LAND {
		loc := p.loc(); p.advance()
		rhs := p.parseExprEq()
		lhs = ast.ChainExpr{Loc: loc, LHS: lhs, Op: ast.ChainAnd, RHS: rhs}
	}
	return lhs
}

// expr3: == !=
func (p *Parser) parseExprEq() ast.Expr {
	lhs := p.parseExprCmp()
	for {
		loc := p.loc()
		switch p.cur.Type {
		case token.EQU: p.advance(); lhs = ast.CmpExpr{Loc: loc, LHS: lhs, Op: ast.CmpEqu, RHS: p.parseExprCmp()}
		case token.NEQ: p.advance(); lhs = ast.CmpExpr{Loc: loc, LHS: lhs, Op: ast.CmpNeq, RHS: p.parseExprCmp()}
		default: return lhs
		}
	}
}

// expr4: < <= > >=
func (p *Parser) parseExprCmp() ast.Expr {
	lhs := p.parseExprAdd()
	for {
		loc := p.loc()
		switch p.cur.Type {
		case token.LTN: p.advance(); lhs = ast.CmpExpr{Loc: loc, LHS: lhs, Op: ast.CmpLtn, RHS: p.parseExprAdd()}
		case token.LTE: p.advance(); lhs = ast.CmpExpr{Loc: loc, LHS: lhs, Op: ast.CmpLte, RHS: p.parseExprAdd()}
		case token.GTN: p.advance(); lhs = ast.CmpExpr{Loc: loc, LHS: lhs, Op: ast.CmpGtn, RHS: p.parseExprAdd()}
		case token.GTE: p.advance(); lhs = ast.CmpExpr{Loc: loc, LHS: lhs, Op: ast.CmpGte, RHS: p.parseExprAdd()}
		default: return lhs
		}
	}
}

// expr5: + - | ^
func (p *Parser) parseExprAdd() ast.Expr {
	lhs := p.parseExprMul()
	for {
		loc := p.loc()
		switch p.cur.Type {
		case token.ADD: p.advance(); lhs = ast.BinOp{Loc: loc, LHS: lhs, Op: ast.OpAdd, RHS: p.parseExprMul()}
		case token.SUB: p.advance(); lhs = ast.BinOp{Loc: loc, LHS: lhs, Op: ast.OpSub, RHS: p.parseExprMul()}
		case token.OR:  p.advance(); lhs = ast.BinOp{Loc: loc, LHS: lhs, Op: ast.OpOr,  RHS: p.parseExprMul()}
		case token.XOR: p.advance(); lhs = ast.BinOp{Loc: loc, LHS: lhs, Op: ast.OpXor, RHS: p.parseExprMul()}
		default: return lhs
		}
	}
}

// expr6: * / % &
func (p *Parser) parseExprMul() ast.Expr {
	lhs := p.parseExprShift()
	for {
		loc := p.loc()
		switch p.cur.Type {
		case token.MUL: p.advance(); lhs = ast.BinOp{Loc: loc, LHS: lhs, Op: ast.OpMul, RHS: p.parseExprShift()}
		case token.DIV: p.advance(); lhs = ast.BinOp{Loc: loc, LHS: lhs, Op: ast.OpDiv, RHS: p.parseExprShift()}
		case token.MOD: p.advance(); lhs = ast.BinOp{Loc: loc, LHS: lhs, Op: ast.OpMod, RHS: p.parseExprShift()}
		case token.AND: p.advance(); lhs = ast.BinOp{Loc: loc, LHS: lhs, Op: ast.OpAnd, RHS: p.parseExprShift()}
		default: return lhs
		}
	}
}

// expr7: << >>
func (p *Parser) parseExprShift() ast.Expr {
	lhs := p.parseUnary()
	for {
		loc := p.loc()
		switch p.cur.Type {
		case token.SHL: p.advance(); lhs = ast.BinOp{Loc: loc, LHS: lhs, Op: ast.OpShl, RHS: p.parseUnary()}
		case token.SHR: p.advance(); lhs = ast.BinOp{Loc: loc, LHS: lhs, Op: ast.OpShr, RHS: p.parseUnary()}
		default: return lhs
		}
	}
}

// expr8: unary -x !x ~x
func (p *Parser) parseUnary() ast.Expr {
	loc := p.loc()
	switch p.cur.Type {
	case token.SUB:   p.advance(); return ast.UnaryExpr{Loc: loc, Op: ast.UnarySub, Val: p.parseUnary()}
	case token.NOT:   p.advance(); return ast.UnaryExpr{Loc: loc, Op: ast.UnaryNot, Val: p.parseUnary()}
	case token.TILDE: p.advance(); return ast.UnaryExpr{Loc: loc, Op: ast.UnaryInv, Val: p.parseUnary()}
	}
	return p.parsePrimary()
}

// expr9: primary
func (p *Parser) parsePrimary() ast.Expr {
	loc := p.loc()
	switch p.cur.Type {
	case token.INTEGER:
		v := p.cur.IntVal; p.advance()
		return ast.IntLit{Loc: loc, Val: v}
	case token.STRING:
		s := p.cur.StrVal; p.advance()
		return ast.StrLit{Loc: loc, Tokens: []ast.StrToken{ast.TextToken{Loc: loc, Text: s}}}
	case token.DRES:
		key := p.cur.StrVal; p.advance()
		return ast.ResRef{Loc: loc, Key: key}
	case token.REG:
		p.advance()
		return ast.StoreRef{Loc: loc}
	case token.VAR:
		bank := int(p.cur.IntVal); p.advance()
		p.expect(token.LSQU)
		idx := p.parseExpr()
		p.expect(token.RSQU)
		return ast.IntVar{Loc: loc, Bank: bank, Index: idx}
	case token.SVAR:
		bank := int(p.cur.IntVal); p.advance()
		p.expect(token.LSQU)
		idx := p.parseExpr()
		p.expect(token.RSQU)
		return ast.StrVar{Loc: loc, Bank: bank, Index: idx}
	case token.IDENT:
		name := p.cur.StrVal; p.advance()
		if p.cur.Type == token.LPAR {
			return p.parseFuncCall(loc, name)
		}
		if p.cur.Type == token.LSQU {
			p.advance()
			idx := p.parseExpr()
			p.expect(token.RSQU)
			// `name[idx](args)` — Deref-of-array followed by a call,
			// e.g. __special[0]('CGSH25'). This shape comes out of the
			// disassembler for special parameter markers. Treat the
			// whole thing as a function call where the index becomes the
			// first parameter (decorative; codegen ignores __special).
			if p.cur.Type == token.LPAR {
				p.advance()
				rest := p.parseParamList()
				p.expect(token.RPAR)
				params := append([]ast.Param{ast.SimpleParam{Loc: loc, Expr: idx}}, rest...)
				return ast.FuncCall{Loc: loc, Ident: name, Params: params}
			}
			return ast.Deref{Loc: loc, Ident: name, Index: idx}
		}
		return ast.VarOrFunc{Loc: loc, Ident: name}
	case token.GOTO:
		name := p.cur.StrVal; p.advance()
		var params []ast.Param
		if p.cur.Type == token.LPAR {
			p.advance()
			params = p.parseParamList()
			p.expect(token.RPAR)
		}
		var label *ast.Label
		if p.cur.Type == token.LABEL {
			lbl := ast.Label{Loc: p.loc(), Ident: p.cur.StrVal}
			label = &lbl; p.advance()
		}
		return ast.FuncCall{Loc: loc, Ident: name, Params: params, Label: label}
	case token.LPAR:
		p.advance()
		expr := p.parseExpr()
		p.expect(token.RPAR)
		return ast.ParenExpr{Loc: loc, Expr: expr}
	}
	// Fallback: advance and return zero
	p.advance()
	return ast.IntLit{Loc: loc, Val: 0}
}

func (p *Parser) parseFuncCall(loc ast.Loc, name string) ast.Expr {
	p.expect(token.LPAR)
	params := p.parseParamList()
	p.expect(token.RPAR)
	// Only goto-like functions can have a trailing `@label`. For any
	// other function, a `@N` token after the closing paren is a separate
	// statement — a label definition for the next instruction.
	//
	// OCaml grammar (keAstParser.mly L324-327):
	//
	//   gotofunction:
	//     | GOTO label
	//     | GOTO LPAR param_list RPAR label
	//
	// i.e. only the GOTO token type accepts a trailing label. In our Go
	// lexer, `goto`/`gosub`/`goto_if`/`goto_unless`/`gosub_if`/
	// `gosub_unless` are emitted as plain IDENT (not a dedicated GOTO
	// token), so we check the name here instead.
	var label *ast.Label
	if p.cur.Type == token.LABEL && isGotoLike(name) {
		lbl := ast.Label{Loc: p.loc(), Ident: p.cur.StrVal}
		label = &lbl; p.advance()
	}
	return ast.FuncCall{Loc: loc, Ident: name, Params: params, Label: label}
}

// isGotoLike reports whether the given function name takes a trailing
// `@label` argument in the kepago source syntax.
func isGotoLike(name string) bool {
	switch name {
	case "goto", "gosub",
		"goto_if", "goto_unless",
		"gosub_if", "gosub_unless":
		return true
	}
	return false
}

func (p *Parser) parseParamList() []ast.Param {
	var params []ast.Param
	if p.cur.Type == token.RPAR { return params }
	// Empty-slot tolerance — see comment below. Applies to the leading
	// position too: `(, expr)` after the lexer has skipped a stripped
	// /* nested:N bytes */ commented out by the disassembler.
	if p.cur.Type == token.COMMA {
		params = append(params, ast.SimpleParam{Loc: p.loc(), Expr: ast.IntLit{Loc: p.loc(), Val: 0}})
	} else {
		params = append(params, p.parseParam())
	}
	for p.match(token.COMMA) {
		// Empty slot tolerance: when the disassembler emits a nested
		// arg block as `/* nested:N bytes */` and the lexer skips it,
		// adjacent commas leave a hole. Treat the missing expression
		// as IntLit(0) so the parser can keep going. The resulting
		// bytecode won't be semantically correct for that argument,
		// but the parse no longer panics; this matters for opcodes
		// like InitExFrames whose nested-complex parameters aren't
		// yet round-trippable through our toolchain.
		if p.cur.Type == token.RPAR || p.cur.Type == token.COMMA {
			params = append(params, ast.SimpleParam{Loc: p.loc(), Expr: ast.IntLit{Loc: p.loc(), Val: 0}})
			continue
		}
		params = append(params, p.parseParam())
	}
	return params
}

func (p *Parser) parseParam() ast.Param {
	loc := p.loc()
	if p.cur.Type == token.LCUR {
		p.advance()
		var exprs []ast.Expr
		for p.cur.Type != token.RCUR && p.cur.Type != token.EOF {
			exprs = append(exprs, p.parseExpr())
			p.match(token.COMMA)
		}
		p.match(token.RCUR)
		return ast.ComplexParam{Loc: loc, Exprs: exprs}
	}
	// Tuple-form complex parameter: `(a, b, c, ...)`.
	// The OCaml disassembler emits complex(...)+ args as parenthesized
	// tuples, e.g. `InitExFrames ((0, 0, 255, intC[0]) /* nested:30 */)`.
	// We detect them by parsing `( expr` and looking at what follows:
	//   - COMMA → it's a tuple, accumulate the rest as a ComplexParam.
	//   - RPAR  → it was just `(expr)`, wrap it in a ParenExpr and
	//             continue the precedence-climbing chain so that
	//             trailing operators (e.g. `(a) || (b)` inside a
	//             goto_unless condition) still bind correctly.
	if p.cur.Type == token.LPAR {
		openLoc := p.loc()
		p.advance() // consume `(`
		first := p.parseExpr()
		if p.cur.Type == token.COMMA {
			exprs := []ast.Expr{first}
			for p.match(token.COMMA) {
				if p.cur.Type == token.RPAR || p.cur.Type == token.COMMA {
					// Tolerate empty slots, same convention as
					// parseParamList above.
					exprs = append(exprs, ast.IntLit{Loc: p.loc(), Val: 0})
					continue
				}
				exprs = append(exprs, p.parseExpr())
			}
			p.expect(token.RPAR)
			return ast.ComplexParam{Loc: loc, Exprs: exprs}
		}
		p.expect(token.RPAR)
		paren := ast.ParenExpr{Loc: openLoc, Expr: first}
		return ast.SimpleParam{Loc: loc, Expr: p.continueExprFrom(paren)}
	}
	return ast.SimpleParam{Loc: loc, Expr: p.parseExpr()}
}

func isExprStart(t token.Type) bool {
	switch t {
	case token.INTEGER, token.STRING, token.DRES, token.IDENT,
		token.VAR, token.SVAR, token.REG, token.GOTO,
		token.LPAR, token.SUB, token.NOT, token.TILDE:
		return true
	}
	return false
}
