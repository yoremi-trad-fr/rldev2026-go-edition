package compilerframe

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/yoremi/rldev-go/pkg/texttransforms"
	"github.com/yoremi/rldev-go/rlc/pkg/ast"
	"github.com/yoremi/rldev-go/rlc/pkg/kfn"
	"github.com/yoremi/rldev-go/rlc/pkg/memory"
	rlbabel "github.com/yoremi/rldev-go/rlc/pkg/rlBabel"
)

const (
	rlBabelFlagLoaded = "__RLBABEL_KH__"
	rlBabelFlagDyn    = "__DynamicLineation__"

	rlBabelDisplayLabel = "__rlb_go_textoutdisplay__"

	rlBabelDLLInitialise    = 0
	rlBabelDLLTextStart     = 10
	rlBabelDLLTextAppend    = 11
	rlBabelDLLTextGetChar   = 12
	rlBabelDLLTextNewScreen = 13

	rlBabelGetcError       = 0
	rlBabelGetcEndOfString = 1
	rlBabelGetcPrintChar   = 2
	rlBabelGetcNewLine     = 3
	rlBabelGetcNewScreen   = 4
	rlBabelGetcSetIndent   = 5
	rlBabelGetcClearIndent = 6
)

type rlBabelRuntimeVars struct {
	buffer  ast.Expr
	empty   ast.Expr
	temp    ast.Expr
	itoaBuf ast.Expr
	dll     int32
	oldType bool
}

func (c *Compiler) compileKnownLoadFile(loc ast.Loc, path string) bool {
	name := strings.TrimSpace(path)
	name = strings.TrimSuffix(name, ".kh")
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if !strings.EqualFold(name, "rlBabel") {
		return false
	}
	c.loadRLBabelModule(loc)
	return true
}

func (c *Compiler) loadRLBabelModule(loc ast.Loc) {
	if c.rlBabelLoaded {
		return
	}
	c.rlBabelLoaded = true
	c.defineOrSetInt(rlBabelFlagLoaded, 1)
	c.defineOrSetInt(rlBabelFlagDyn, 1)
	dll := c.resolveRLBabelDLLIndex()
	c.defineOrSetInt("rlBabelDLL", dll)
	if c.rlBabelIsOldType() {
		c.defineOrSetString("rlBabel", "rlBabelF")
	} else {
		c.defineOrSetString("rlBabel", "rlBabel")
	}
	c.warning(loc, "#load 'rlBabel': module Go experimental charge (texte VWF, sans gloss interactifs)")
}

func (c *Compiler) defineOrSetInt(name string, value int32) {
	sym := memory.Symbol{Kind: memory.KindInteger, IntVal: value}
	if c.Mem.Defined(name) {
		_ = c.Mem.Mutate(name, sym)
		return
	}
	c.Mem.Define(name, sym)
}

func (c *Compiler) defineOrSetString(name, value string) {
	sym := memory.Symbol{Kind: memory.KindString, StrVal: value}
	if c.Mem.Defined(name) {
		_ = c.Mem.Mutate(name, sym)
		return
	}
	c.Mem.Define(name, sym)
}

func (c *Compiler) resolveRLBabelDLLIndex() int32 {
	if sym, ok := c.Mem.Get("rlBabelDLL"); ok && sym.Kind == memory.KindInteger {
		return sym.IntVal
	}
	if c.Ini != nil {
		for i := 0; i < 16; i++ {
			values := c.Ini.Find(fmt.Sprintf("DLL.%03d", i))
			if len(values) == 1 && strings.EqualFold(values[0].Str, "rlBabel") {
				return int32(i)
			}
		}
	}
	return 0
}

func (c *Compiler) rlBabelIsOldType() bool {
	v, ok := knownRegistryVersion(c.Reg)
	return ok && versionLessThan(v, kfn.Version{1, 2, 5, 0})
}

func versionLessThan(v, max kfn.Version) bool {
	for i := 0; i < 4; i++ {
		if v[i] < max[i] {
			return true
		}
		if v[i] > max[i] {
			return false
		}
	}
	return false
}

func (c *Compiler) compileRLBabelText(ret ast.ReturnStmt) {
	if ret.Expr == nil {
		return
	}
	c.loadRLBabelModule(ret.Loc)
	c.rlBabelUsed = true
	c.ensureRLBabelVars(ret.Loc)
	c.ensureRLBabelInitialised(ret.Loc)

	switch x := ret.Expr.(type) {
	case ast.StrLit:
		c.emitRLBabelKidoku(ret.Loc)
		tc := &rlBabelTextCompiler{c: c, loc: ret.Loc}
		for _, tok := range x.Tokens {
			tc.compileToken(tok)
		}
		tc.flush(true)
		c.noLineForNextPause = true
	case ast.ResRef:
		c.emitRLBabelKidoku(ret.Loc)
		tc := &rlBabelTextCompiler{c: c, loc: ret.Loc}
		tc.compileToken(ast.ResRefToken{Loc: x.Loc, Key: x.Key})
		tc.flush(true)
		c.noLineForNextPause = true
	case ast.StrVar:
		c.emitRLBabelKidoku(ret.Loc)
		c.rlBabelStartString(ret.Loc, x)
		c.rlBabelDisplay(ret.Loc)
		c.noLineForNextPause = true
	default:
		c.error(ret.Loc, "rlBabel text output expects a string literal, string variable, or #res reference")
	}
}

func (c *Compiler) emitRLBabelKidoku(loc ast.Loc) {
	if c.Out.SuppressAutoKidoku {
		return
	}
	c.addImplicitKidoku(loc)
	c.callFunc(loc, "strout", c.ensureRLBabelVars(loc).empty)
}

func (c *Compiler) ensureRLBabelVars(loc ast.Loc) *rlBabelRuntimeVars {
	if c.rlBabelVars != nil {
		return c.rlBabelVars
	}
	buffer := c.ensureRLBabelVar(loc, "__rlb_buffer", true)
	empty := c.ensureRLBabelVar(loc, "__rlb_empty", true)
	temp := c.ensureRLBabelVar(loc, "__rlb_temp0", false)
	itoaBuf := c.ensureRLBabelVar(loc, "__rlb_itoa", true)
	c.rlBabelVars = &rlBabelRuntimeVars{
		buffer:  buffer,
		empty:   empty,
		temp:    temp,
		itoaBuf: itoaBuf,
		dll:     c.resolveRLBabelDLLIndex(),
		oldType: c.rlBabelIsOldType(),
	}
	return c.rlBabelVars
}

func (c *Compiler) ensureRLBabelVar(loc ast.Loc, name string, isStr bool) ast.Expr {
	if expr, err := c.Mem.GetAsExpr(name, loc); err == nil {
		return expr
	}
	vt := memory.VarType{IsStr: isStr, BitWidth: 32}
	v, err := c.Mem.AllocVar(name, vt, 0, nil)
	if err != nil {
		c.error(loc, err.Error())
		return ast.IntLit{Loc: loc, Val: 0}
	}
	if isStr {
		return ast.StrVar{Loc: loc, Bank: v.TypedSpace, Index: ast.IntLit{Loc: loc, Val: v.Index}}
	}
	return ast.IntVar{Loc: loc, Bank: v.TypedSpace, Index: ast.IntLit{Loc: loc, Val: v.Index}}
}

func (c *Compiler) ensureRLBabelInitialised(loc ast.Loc) {
	if c.rlBabelInitDone {
		return
	}
	c.rlBabelInitDone = true
	vars := c.ensureRLBabelVars(loc)
	if vars.oldType {
		c.callFunc(loc, "LoadDLL", ast.IntLit{Loc: loc, Val: 0}, rlBabelStringExpr(loc, "rlBabelF"))
	}
	c.callRLBabelDLL(loc, nil, rlBabelDLLInitialise, ast.IntLit{Loc: loc, Val: vars.dll})
}

func (c *Compiler) rlBabelStartString(loc ast.Loc, value ast.Expr) {
	c.callRLBabelDLL(loc, nil, rlBabelDLLTextStart, c.rlBabelStringAddr(loc, value))
}

func (c *Compiler) rlBabelAppendString(loc ast.Loc, value ast.Expr) {
	c.callRLBabelDLL(loc, nil, rlBabelDLLTextAppend, c.rlBabelStringAddr(loc, value))
}

func (c *Compiler) rlBabelDisplay(loc ast.Loc) {
	c.ParseElt(ast.FuncCallStmt{Loc: loc, Ident: "gosub", Label: &ast.Label{Loc: loc, Ident: rlBabelDisplayLabel}})
}

func (c *Compiler) rlBabelStringAddr(loc ast.Loc, value ast.Expr) ast.Expr {
	switch value.(type) {
	case ast.StrVar:
		return c.addrExpr(loc, value)
	default:
		vars := c.ensureRLBabelVars(loc)
		c.ParseElt(ast.AssignStmt{Loc: loc, Dest: vars.buffer, Op: ast.AssignSet, Expr: value})
		return c.addrExpr(loc, vars.buffer)
	}
}

func (c *Compiler) addrExpr(loc ast.Loc, value ast.Expr) ast.Expr {
	expr, err := c.Intrin.EvalAsExpr("__addr", loc, []ast.Param{ast.SimpleParam{Loc: loc, Expr: value}})
	if err != nil {
		c.error(loc, err.Error())
		return ast.IntLit{Loc: loc, Val: 0}
	}
	return expr
}

func (c *Compiler) callRLBabelDLL(loc ast.Loc, dest ast.Expr, fn int32, args ...ast.Expr) {
	vars := c.ensureRLBabelVars(loc)
	exprs := []ast.Expr{
		ast.IntLit{Loc: loc, Val: vars.dll},
		ast.IntLit{Loc: loc, Val: fn},
	}
	exprs = append(exprs, args...)
	c.callFuncDest(loc, dest, "CallDLL", exprs...)
}

func (c *Compiler) callFunc(loc ast.Loc, ident string, exprs ...ast.Expr) {
	c.callFuncDest(loc, nil, ident, exprs...)
}

func (c *Compiler) callFuncDest(loc ast.Loc, dest ast.Expr, ident string, exprs ...ast.Expr) {
	params := make([]ast.Param, len(exprs))
	for i, e := range exprs {
		params[i] = ast.SimpleParam{Loc: loc, Expr: e}
	}
	c.ParseElt(ast.FuncCallStmt{Loc: loc, Dest: dest, Ident: ident, Params: params})
}

func rlBabelStringExpr(loc ast.Loc, value string) ast.Expr {
	if value == "" {
		return ast.StrLit{Loc: loc}
	}
	return ast.StrLit{Loc: loc, Tokens: []ast.StrToken{ast.TextToken{Loc: loc, Text: value}}}
}

func (c *Compiler) emitRLBabelRuntime(loc ast.Loc) {
	if c.rlBabelRuntimeDone {
		return
	}
	c.rlBabelRuntimeDone = true
	vars := c.ensureRLBabelVars(loc)

	end := "__rlb_go_display_end__"
	loop := "__rlb_go_display_loop__"
	newLine := "__rlb_go_display_newline__"
	newScreen := "__rlb_go_display_newscreen__"
	setIndent := "__rlb_go_display_setindent__"
	clearIndent := "__rlb_go_display_clearindent__"
	printChar := "__rlb_go_display_print__"

	c.ParseElt(ast.HaltStmt{Loc: loc})
	c.emitLabel(loc, ast.Label{Loc: loc, Ident: rlBabelDisplayLabel})
	c.callFunc(loc, "DisableAutoSavepoints")
	c.emitLabel(loc, ast.Label{Loc: loc, Ident: loop})
	c.callRLBabelDLL(loc, vars.temp, rlBabelDLLTextGetChar, c.addrExpr(loc, vars.buffer), c.addrExpr(loc, vars.temp))
	c.ParseNormElt(ast.GotoCaseStmt{
		Loc:   loc,
		Ident: "goto_case",
		Expr:  vars.temp,
		Cases: []ast.GotoCaseArm{
			{IsDefault: true, Label: ast.Label{Loc: loc, Ident: end}},
			{Expr: ast.IntLit{Loc: loc, Val: rlBabelGetcError}, Label: ast.Label{Loc: loc, Ident: end}},
			{Expr: ast.IntLit{Loc: loc, Val: rlBabelGetcEndOfString}, Label: ast.Label{Loc: loc, Ident: end}},
			{Expr: ast.IntLit{Loc: loc, Val: rlBabelGetcPrintChar}, Label: ast.Label{Loc: loc, Ident: printChar}},
			{Expr: ast.IntLit{Loc: loc, Val: rlBabelGetcNewLine}, Label: ast.Label{Loc: loc, Ident: newLine}},
			{Expr: ast.IntLit{Loc: loc, Val: rlBabelGetcNewScreen}, Label: ast.Label{Loc: loc, Ident: newScreen}},
			{Expr: ast.IntLit{Loc: loc, Val: rlBabelGetcSetIndent}, Label: ast.Label{Loc: loc, Ident: setIndent}},
			{Expr: ast.IntLit{Loc: loc, Val: rlBabelGetcClearIndent}, Label: ast.Label{Loc: loc, Ident: clearIndent}},
		},
	})

	c.emitLabel(loc, ast.Label{Loc: loc, Ident: printChar})
	c.callFunc(loc, "strout", vars.buffer)
	c.callFunc(loc, "TextPosX", vars.temp)
	c.ParseElt(ast.FuncCallStmt{Loc: loc, Ident: "goto", Label: &ast.Label{Loc: loc, Ident: loop}})

	c.emitLabel(loc, ast.Label{Loc: loc, Ident: newLine})
	c.callFunc(loc, "br")
	c.ParseElt(ast.FuncCallStmt{Loc: loc, Ident: "goto", Label: &ast.Label{Loc: loc, Ident: loop}})

	c.emitLabel(loc, ast.Label{Loc: loc, Ident: newScreen})
	c.callFunc(loc, "page")
	c.callFunc(loc, "TextPos", ast.IntLit{Loc: loc, Val: 0}, ast.IntLit{Loc: loc, Val: 0})
	c.callRLBabelDLL(loc, nil, rlBabelDLLTextNewScreen, c.addrExpr(loc, vars.buffer))
	c.ParseElt(ast.FuncCallStmt{Loc: loc, Ident: "goto", Label: &ast.Label{Loc: loc, Ident: loop}})

	c.emitLabel(loc, ast.Label{Loc: loc, Ident: setIndent})
	c.callFunc(loc, "SetIndent")
	c.ParseElt(ast.FuncCallStmt{Loc: loc, Ident: "goto", Label: &ast.Label{Loc: loc, Ident: loop}})

	c.emitLabel(loc, ast.Label{Loc: loc, Ident: clearIndent})
	c.callFunc(loc, "ClearIndent")
	c.ParseElt(ast.FuncCallStmt{Loc: loc, Ident: "goto", Label: &ast.Label{Loc: loc, Ident: loop}})

	c.emitLabel(loc, ast.Label{Loc: loc, Ident: end})
	c.callFunc(loc, "EnableAutoSavepoints")
	c.callFunc(loc, "ret")
}

type rlBabelTextCompiler struct {
	c              *Compiler
	loc            ast.Loc
	buf            strings.Builder
	appending      bool
	ignoreOneSpace bool
}

func (tc *rlBabelTextCompiler) flush(display bool) {
	chunk := tc.buf.String()
	tc.buf.Reset()
	if display {
		if tc.appending {
			if chunk != "" {
				tc.c.rlBabelAppendString(tc.loc, rlBabelStringExpr(tc.loc, chunk))
			}
			tc.appending = false
			tc.c.rlBabelDisplay(tc.loc)
			return
		}
		tc.c.rlBabelStartString(tc.loc, rlBabelStringExpr(tc.loc, chunk))
		tc.c.rlBabelDisplay(tc.loc)
		return
	}
	if tc.appending {
		if chunk != "" {
			tc.c.rlBabelAppendString(tc.loc, rlBabelStringExpr(tc.loc, chunk))
		}
		return
	}
	tc.c.rlBabelStartString(tc.loc, rlBabelStringExpr(tc.loc, chunk))
	tc.appending = true
}

func (tc *rlBabelTextCompiler) compileToken(tok ast.StrToken) {
	if tc.ignoreOneSpace {
		if _, ok := tok.(ast.SpaceToken); !ok {
			tc.ignoreOneSpace = false
		}
	}

	switch t := tok.(type) {
	case ast.TextToken:
		tc.buf.WriteString(t.Text)
	case ast.SpaceToken:
		count := t.Count
		if count > 0 && tc.ignoreOneSpace {
			tc.ignoreOneSpace = false
			count--
		}
		tc.buf.WriteString(strings.Repeat(" ", count))
	case ast.DQuoteToken:
		tc.buf.WriteByte(rlbabel.TokenQuote)
	case ast.SpeakerToken:
		tc.buf.WriteByte(rlbabel.TokenNameLeft)
	case ast.RCurToken:
		tc.buf.WriteByte(rlbabel.TokenNameRight)
		tc.ignoreOneSpace = true
	case ast.LLenticToken:
		tc.buf.WriteString("【")
	case ast.RLenticToken:
		tc.buf.WriteString("】")
	case ast.AsteriskToken:
		tc.writeFullwidthRune(0xff0a)
		tc.flush(true)
	case ast.PercentToken:
		tc.writeFullwidthRune(0xff05)
		tc.flush(true)
	case ast.HyphenToken:
		tc.buf.WriteByte('-')
	case ast.NameToken:
		tc.compileName(t)
	case ast.CodeToken:
		tc.compileCode(t)
	case ast.GlossToken:
		if t.IsRuby {
			tc.c.warning(t.Loc, "not implemented: \\ruby{} in Go rlBabel text")
		} else {
			tc.c.warning(t.Loc, "Go rlBabel: glosses are not interactive yet; compiling base text only")
		}
		for _, bt := range t.Base {
			tc.compileToken(bt)
		}
	case ast.ResRefToken:
		if tc.c.Out.ResolveRes != nil {
			if raw, ok := tc.c.Out.ResolveRes(t.Key); ok {
				tc.compileResourceText(raw)
			}
		}
	case ast.DeleteToken:
	case ast.AddToken:
		tc.c.warning(t.Loc, "\\a{} additional strings are not implemented in Go rlBabel text")
	case ast.RewriteToken:
		tc.c.warning(t.Loc, "\\f{} rewrite is not implemented in Go rlBabel text")
	default:
		tc.c.warning(tc.loc, fmt.Sprintf("unknown rlBabel text token type: %T", tok))
	}
}

func (tc *rlBabelTextCompiler) compileName(t ast.NameToken) {
	idx, ok := tc.c.Norm.NormalizeAndGetConst(t.Index)
	if !ok {
		tc.c.error(t.Loc, "name index must be constant in rlBabel-formatted text")
		return
	}
	if t.Global {
		tc.writeFullwidthRune(0xff0a)
	} else {
		tc.writeFullwidthRune(0xff05)
	}
	for _, r := range strconv.Itoa(int(idx)) {
		tc.writeFullwidthDigit(int(r - '0'))
	}
}

func (tc *rlBabelTextCompiler) compileCode(t ast.CodeToken) {
	switch t.Ident {
	case "n":
		if t.OptArg != nil || len(t.Params) != 0 {
			tc.c.error(t.Loc, "\\n cannot take parameters in rlBabel text")
			return
		}
		tc.buf.WriteByte(rlbabel.TokenBreak)
	case "r":
		if t.OptArg != nil || len(t.Params) != 0 {
			tc.c.error(t.Loc, "\\r cannot take parameters in rlBabel text")
			return
		}
		tc.buf.WriteByte(rlbabel.TokenClearIndent)
		tc.buf.WriteByte(rlbabel.TokenBreak)
	case "b":
		tc.buf.WriteByte(rlbabel.TokenEmphasis)
	case "u":
		tc.buf.WriteByte(rlbabel.TokenRegular)
	case "s":
		tc.compileStringVar(t)
	case "i":
		tc.compileIntVar(t)
	case "e", "em":
		tc.compileEmoji(t)
	default:
		tc.flush(true)
		tc.c.ParseElt(ast.FuncCallStmt{Loc: t.Loc, Ident: t.Ident, Params: t.Params})
	}
}

func (tc *rlBabelTextCompiler) compileStringVar(t ast.CodeToken) {
	if len(t.Params) != 1 || t.OptArg != nil {
		tc.c.error(t.Loc, "\\s{} must have one string-variable parameter")
		return
	}
	sp, ok := t.Params[0].(ast.SimpleParam)
	if !ok {
		tc.c.error(t.Loc, "\\s{} must have one string-variable parameter")
		return
	}
	expr := tc.c.resolveRLBabelExpr(sp.Expr, t.Loc)
	if _, ok := expr.(ast.StrVar); !ok {
		tc.c.error(t.Loc, "\\s{} parameter must be a string variable")
		return
	}
	tc.flush(false)
	tc.c.rlBabelAppendString(t.Loc, expr)
}

func (tc *rlBabelTextCompiler) compileIntVar(t ast.CodeToken) {
	if len(t.Params) != 1 {
		tc.c.error(t.Loc, "\\i{} must have one integer parameter")
		return
	}
	sp, ok := t.Params[0].(ast.SimpleParam)
	if !ok {
		tc.c.error(t.Loc, "\\i{} must have one integer parameter")
		return
	}
	width := int32(0)
	if t.OptArg != nil {
		if v, ok := tc.c.Norm.NormalizeAndGetConst(t.OptArg); ok {
			width = v
		}
	}
	if v, ok := tc.c.Norm.NormalizeAndGetConst(sp.Expr); ok {
		if width > 0 {
			tc.buf.WriteString(fmt.Sprintf("%0*d", width, v))
		} else {
			tc.buf.WriteString(fmt.Sprintf("%d", v))
		}
		return
	}
	tc.flush(false)
	vars := tc.c.ensureRLBabelVars(t.Loc)
	params := []ast.Expr{sp.Expr}
	if width > 0 {
		params = append(params, ast.IntLit{Loc: t.Loc, Val: width})
	}
	tc.c.callFuncDest(t.Loc, vars.itoaBuf, "itoa", params...)
	tc.c.rlBabelAppendString(t.Loc, vars.itoaBuf)
}

func (tc *rlBabelTextCompiler) compileEmoji(t ast.CodeToken) {
	result, err := rlbabel.ProcessEmoji(t.Ident, t.Params)
	if err != nil {
		tc.c.error(t.Loc, err.Error())
		return
	}
	if result.HasSize {
		tc.flush(true)
		tc.c.callFunc(t.Loc, "FontSize", result.SizeExpr)
	}
	tc.buf.WriteByte(result.EmojiMarker)
	if result.IsConst {
		tc.buf.WriteString(result.IndexText)
	} else if result.IndexExpr != nil {
		tc.flush(false)
		vars := tc.c.ensureRLBabelVars(t.Loc)
		tc.c.callFuncDest(t.Loc, vars.itoaBuf, "itoa", result.IndexExpr, ast.IntLit{Loc: t.Loc, Val: 2})
		tc.c.rlBabelAppendString(t.Loc, vars.itoaBuf)
	}
	if result.HasSize {
		tc.flush(true)
		tc.c.callFunc(t.Loc, "FontSize")
	}
}

func (tc *rlBabelTextCompiler) compileResourceText(raw string) {
	r := []rune(raw)
	for i := 0; i < len(r); {
		if r[i] == '\\' && i+1 < len(r) {
			next := r[i+1]
			if next == '{' {
				tc.compileToken(ast.SpeakerToken{Loc: tc.loc})
				i += 2
				continue
			}
			if (next == 'l' || next == 'm') && i+2 < len(r) && r[i+2] == '{' {
				global, letters, hasCharID, charID, consumed, ok := scanRLBabelResourceNameMarker(r, i)
				if ok {
					tc.compileResourceNameMarker(global, letters, hasCharID, charID)
					i += consumed
					continue
				}
			}
			if src, consumed, ok := scanResourceControl(r, i); ok {
				if stmt, ok := parseResourceControl(src, tc.loc); ok {
					if stmt.Ident == "\\n" {
						tc.compileToken(ast.CodeToken{Loc: tc.loc, Ident: "n"})
					} else if stmt.Ident == "\\r" {
						tc.compileToken(ast.CodeToken{Loc: tc.loc, Ident: "r"})
					} else {
						tc.flush(true)
						tc.c.ParseElt(stmt)
					}
					i += consumed
					continue
				}
			}
			tc.buf.WriteRune(next)
			i += 2
			continue
		}
		switch r[i] {
		case '"':
			tc.compileToken(ast.DQuoteToken{Loc: tc.loc})
		case '}':
			tc.compileToken(ast.RCurToken{Loc: tc.loc})
		case 0x3010:
			tc.compileToken(ast.LLenticToken{Loc: tc.loc})
		case 0x3011:
			tc.compileToken(ast.RLenticToken{Loc: tc.loc})
		case 0xff0a:
			tc.compileToken(ast.AsteriskToken{Loc: tc.loc})
		case 0xff05:
			tc.compileToken(ast.PercentToken{Loc: tc.loc})
		default:
			tc.buf.WriteRune(r[i])
		}
		i++
	}
}

func (tc *rlBabelTextCompiler) compileResourceNameMarker(global bool, letters []byte, hasCharID bool, charID int) {
	if global {
		tc.writeFullwidthRune(0xff0a)
	} else {
		tc.writeFullwidthRune(0xff05)
	}
	for _, letter := range letters {
		tc.writeFullwidthUpper(letter)
	}
	if hasCharID {
		tc.writeFullwidthDigit(charID)
	}
}

func (tc *rlBabelTextCompiler) writeFullwidthUpper(letter byte) {
	if letter < 'A' || letter > 'Z' {
		return
	}
	tc.writeFullwidthRune(0xff21 + rune(letter-'A'))
}

func (tc *rlBabelTextCompiler) writeFullwidthDigit(digit int) {
	if digit < 0 || digit > 9 {
		return
	}
	tc.writeFullwidthRune(0xff10 + rune(digit))
}

func (tc *rlBabelTextCompiler) writeFullwidthRune(r rune) {
	if texttransforms.GetMode() == texttransforms.EncWestern {
		tc.buf.WriteRune(0x10000 + r)
		return
	}
	tc.buf.WriteRune(r)
}

func scanRLBabelResourceNameMarker(r []rune, start int) (global bool, letters []byte, hasCharID bool, charID int, consumed int, ok bool) {
	if start+3 >= len(r) || r[start] != '\\' || (r[start+1] != 'l' && r[start+1] != 'm') || r[start+2] != '{' {
		return false, nil, false, 0, 0, false
	}
	global = r[start+1] == 'm'
	pos := start + 3
	for pos < len(r) && r[pos] >= 'A' && r[pos] <= 'Z' && len(letters) < 2 {
		letters = append(letters, byte(r[pos]))
		pos++
	}
	if len(letters) == 0 {
		return false, nil, false, 0, 0, false
	}
	if pos < len(r) && r[pos] == ',' {
		pos++
		for pos < len(r) && r[pos] == ' ' {
			pos++
		}
		if pos >= len(r) || r[pos] < '0' || r[pos] > '9' {
			return false, nil, false, 0, 0, false
		}
		hasCharID = true
		charID = int(r[pos] - '0')
		pos++
	}
	if pos >= len(r) || r[pos] != '}' {
		return false, nil, false, 0, 0, false
	}
	return global, letters, hasCharID, charID, pos - start + 1, true
}

func (c *Compiler) resolveRLBabelExpr(e ast.Expr, loc ast.Loc) ast.Expr {
	switch x := e.(type) {
	case ast.VarOrFunc:
		if expr, err := c.Mem.GetAsExpr(x.Ident, loc); err == nil {
			return expr
		}
	case ast.Deref:
		if expr, err := c.Mem.GetDerefAsExpr(x.Ident, x.Index, loc); err == nil {
			return expr
		}
	}
	return e
}
