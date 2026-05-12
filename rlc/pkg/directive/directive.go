// Package directive implements compiler directive compilation for Kepago.
//
// Transposed from OCaml's rlc/directive.ml (110 lines).
//
// Compiler directives are #-prefixed statements that control compilation:
//
//   #define sym = expr      — define a macro
//   #const sym = expr       — define a constant (evaluated at compile time)
//   #inline sym(params) body — define an inline expansion
//   #undef sym              — remove a symbol
//   #set sym = expr         — mutate an existing symbol
//   #target RealLive        — set target engine
//   #version 1.2.7.0        — set target version
//   #warn "msg"             — emit a warning
//   #error "msg"            — emit an error
//   #print "msg"            — print a message
//   #resource "file"        — load resource strings
//   #entrypoint N           — register an entrypoint
//   #file "name"            — set output filename
//   #character "name"       — add to dramatis personae
//   #kidoku_type N           — set kidoku marker type
//   #val_0x2c N              — set header field
package directive

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yoremi/rldev-go/pkg/encoding"
	"github.com/yoremi/rldev-go/rlc/pkg/ast"
	"github.com/yoremi/rldev-go/rlc/pkg/codegen"
	"github.com/yoremi/rldev-go/rlc/pkg/expr"
	"github.com/yoremi/rldev-go/rlc/pkg/ini"
	"github.com/yoremi/rldev-go/rlc/pkg/kfn"
	"github.com/yoremi/rldev-go/rlc/pkg/memory"
	"github.com/yoremi/rldev-go/rlc/pkg/meta"
)

// Compiler holds the dependencies needed for directive compilation.
type Compiler struct {
	Mem     *memory.Memory
	Norm    *expr.Normalizer
	Output  *codegen.Output
	Ini     *ini.Table
	State   *meta.State
	Target  *kfn.Target
	Version *kfn.Version

	// Configuration
	TargetForced bool   // true if target was set on command line
	OutFile      string // output filename (set by #file)
	SourceDir    string // directory of the .org file (used to resolve #resource)
	SourceEnc    string // encoding name of the .org source ("CP932", "UTF-8", …)

	// Errors/warnings collected during compilation
	Errors   []error
	Warnings []string
}

func (c *Compiler) error(loc ast.Loc, msg string) {
	c.Errors = append(c.Errors, fmt.Errorf("%s: %s", loc, msg))
}

func (c *Compiler) warning(loc ast.Loc, msg string) {
	c.Warnings = append(c.Warnings, fmt.Sprintf("%s: %s", loc, msg))
}

// ============================================================
// Directive compilation (from directive.ml compile)
// ============================================================

// Compile processes a compiler directive statement.
func (c *Compiler) Compile(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case ast.DirectiveStmt:
		c.compileGeneric(s.Loc, s.Name, s.Value)

	case ast.DTargetStmt:
		if c.TargetForced {
			c.warning(s.Loc, "target specified on command-line: ignoring #target directive")
		} else {
			t := kfn.ParseTarget(s.Target)
			if c.Target != nil {
				*c.Target = t
			}
		}

	case ast.DefineStmt:
		c.Mem.Define(s.Ident, memory.Symbol{
			Kind: memory.KindMacro,
			Expr: s.Value,
		})

	case ast.DConstStmt:
		val, ok := c.Norm.NormalizeAndGetConst(s.Value)
		if ok {
			c.Mem.Define(s.Ident, memory.Symbol{
				Kind:   memory.KindInteger,
				IntVal: val,
			})
		} else {
			// Try as string
			str, err := c.Norm.NormalizeAndGetStr(s.Value)
			if err == nil {
				c.Mem.Define(s.Ident, memory.Symbol{
					Kind:   memory.KindString,
					StrVal: str,
				})
			} else {
				c.error(s.Loc, fmt.Sprintf("const value for '%s' must be a compile-time constant", s.Ident))
			}
		}

	case ast.DInlineStmt:
		c.Mem.Define(s.Ident, memory.Symbol{
			Kind:         memory.KindInline,
			InlineParams: s.Params,
			InlineBody:   s.Body,
		})

	case ast.DUndefStmt:
		for _, name := range s.Idents {
			if err := c.Mem.Undefine(name); err != nil {
				c.error(s.Loc, fmt.Sprintf("cannot undefine '%s': %v", name, err))
			}
		}

	case ast.DSetStmt:
		c.compileSet(s)

	case ast.DVersionStmt:
		if c.Version != nil {
			a, _ := c.Norm.NormalizeAndGetInt(s.A)
			b, _ := c.Norm.NormalizeAndGetInt(s.B)
			cv, _ := c.Norm.NormalizeAndGetInt(s.C)
			d, _ := c.Norm.NormalizeAndGetInt(s.D)
			*c.Version = kfn.Version{int(a), int(b), int(cv), int(d)}
		}
	}
}

// compileSet handles #set — mutating an existing symbol.
func (c *Compiler) compileSet(s ast.DSetStmt) {
	sym, ok := c.Mem.Get(s.Ident)
	if !ok {
		c.error(s.Loc, fmt.Sprintf("cannot mutate '%s': not defined", s.Ident))
		return
	}
	switch sym.Kind {
	case memory.KindMacro, memory.KindInteger, memory.KindString:
		if s.ReadOnly {
			// #set with read-only flag → evaluate to constant
			val, ok := c.Norm.NormalizeAndGetConst(s.Value)
			if ok {
				c.Mem.Mutate(s.Ident, memory.Symbol{Kind: memory.KindInteger, IntVal: val})
			} else {
				c.Mem.Mutate(s.Ident, memory.Symbol{Kind: memory.KindMacro, Expr: s.Value})
			}
		} else {
			c.Mem.Mutate(s.Ident, memory.Symbol{Kind: memory.KindMacro, Expr: s.Value})
		}
	default:
		c.error(s.Loc, fmt.Sprintf("cannot mutate '%s': not a constant", s.Ident))
	}
}

// ============================================================
// Generic directives (from directive.ml generic)
// ============================================================

func (c *Compiler) compileGeneric(loc ast.Loc, name string, value ast.Expr) {
	switch name {
	case "warn":
		s := c.exprToString(value)
		c.warning(loc, s)

	case "error":
		s := c.exprToString(value)
		c.error(loc, s)

	case "print":
		s := c.exprToString(value)
		c.Warnings = append(c.Warnings, fmt.Sprintf("%s line %d: %s", loc.File, loc.Line, s))

	case "resource":
		// Load resource strings from the named file (typically a .sjs
		// or .utf next to the .org). The file is parsed by
		// loadResourceFile and the strings are inserted into
		// c.State.Resources where the codegen will look them up to
		// resolve `#res<KEY>` references.
		//
		// Without this, every `#res<NNNN>` was emitted as the literal
		// text "#res<NNNN>" into the bytecode (288 occurrences in
		// SEEN0414 alone), producing invalid bytecode the engine
		// rejected. Mirrors OCaml strLexer.ml load_resfile.
		if c.State != nil {
			fname := c.exprToString(value)
			if fname != "" {
				path := fname
				if c.SourceDir != "" && !filepath.IsAbs(path) {
					path = filepath.Join(c.SourceDir, fname)
				}
				if err := c.loadResourceFile(loc, path); err != nil {
					c.warning(loc, fmt.Sprintf("resource %q: %v", fname, err))
				}
			}
		}

	case "base_res":
		// Base resource loading

	case "val_0x2c":
		if c.State != nil {
			v, err := c.Norm.NormalizeAndGetInt(value)
			if err == nil {
				c.State.Val0x2C = int(v)
			}
		}

	case "character":
		if c.State != nil {
			s := c.exprToString(value)
			c.State.AddCharacter(s)
		}

	case "entrypoint":
		v, err := c.Norm.NormalizeAndGetInt(value)
		if err == nil {
			idx := int(v)
			if idx < 0 || idx >= 100 {
				c.error(loc, fmt.Sprintf("invalid entrypoint #Z%02d: valid values are 0..99", idx))
			} else if c.Output != nil {
				c.Output.AddEntrypoint(idx)
			}
		}

	case "kidoku_type":
		if c.Ini != nil {
			v, err := c.Norm.NormalizeAndGetInt(value)
			if err == nil {
				c.Ini.SetInt("KIDOKU_TYPE", int(v))
			}
		}

	case "file":
		if c.OutFile == "" {
			c.OutFile = c.exprToString(value)
		}
	}
}

// exprToString tries to extract a string from an expression.
func (c *Compiler) exprToString(e ast.Expr) string {
	// Try as string literal
	if slit, ok := e.(ast.StrLit); ok && len(slit.Tokens) > 0 {
		if tt, ok := slit.Tokens[0].(ast.TextToken); ok {
			return tt.Text
		}
	}
	// Try as integer
	if v, ok := c.Norm.NormalizeAndGetConst(e); ok {
		return fmt.Sprintf("%d", v)
	}
	return ""
}

// resEntryRE matches a `<KEY> TEXT` line in a resource file.
// Whitespace between '>' and TEXT is part of TEXT only if it includes a
// non-space character; we strip a single optional leading space.
var resEntryRE = regexp.MustCompile(`^<([^>]*)>\s?(.*)$`)

// loadResourceFile parses a .sjs / .utf resource file and inserts each
// `<KEY> TEXT` entry into c.State.Resources so that codegen can
// substitute `#res<KEY>` references with the corresponding text.
//
// The file format (mirror of OCaml strLexer.ml load_resfile):
//
//	// comment
//	#character 'name'        ← absorbed silently (dramatis hint only)
//	#resource 'other.sjs'    ← chained resource file (recurses)
//	<KEY> the text content   ← one entry per line
//
// Bytes are decoded per c.SourceEnc so a Shift-JIS .sjs and a UTF-8 .utf
// both produce a Go (UTF-8) string in the resource map. Codegen
// re-encodes to bytecode via TextTransforms when emitting.
//
// Resource keys are case-sensitive and stored verbatim; entries with
// duplicate keys overwrite the previous one with a warning.
func (c *Compiler) loadResourceFile(loc ast.Loc, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Allow long lines (up to 1 MB) — some scripts have very long captions.
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := scanner.Bytes()

		// Decode line per source encoding into UTF-8 Go string.
		text, err := decodeBytes(raw, c.SourceEnc)
		if err != nil {
			continue
		}
		text = strings.TrimRight(text, "\r")

		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		if strings.HasPrefix(trimmed, "#character") || strings.HasPrefix(trimmed, "#resource") {
			// We don't follow chained #resource references in resource
			// files yet — the typical layout is one .sjs/.utf per .org.
			continue
		}

		m := resEntryRE.FindStringSubmatch(text)
		if m == nil {
			continue
		}
		key := m[1]
		body := m[2]
		if c.State != nil {
			c.State.SetResource(key, body, ast.Loc{File: path, Line: lineNo})
		}
	}
	return scanner.Err()
}

// decodeBytes decodes raw bytes into a Go (UTF-8) string according to
// the given encoding name. Accepts the same spellings as the rest of
// the toolchain; unknown encodings fall back to a permissive cast.
func decodeBytes(data []byte, encName string) (string, error) {
	switch strings.ToUpper(strings.ReplaceAll(encName, "_", "-")) {
	case "", "UTF-8", "UTF8":
		return string(data), nil
	case "CP932", "SHIFT-JIS", "SJIS", "SHIFTJIS":
		return encoding.SJSToUTF8(data)
	default:
		enc := encoding.Parse(encName)
		s, err := encoding.ToUTF8(data, enc)
		if err != nil {
			return string(data), nil
		}
		return s, nil
	}
}
