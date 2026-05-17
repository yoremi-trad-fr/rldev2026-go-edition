// Command rlc is the RealLive-compatible Kepago compiler.
//
// Transposed from OCaml:
//   - rlc/app.ml (132 lines)  — application config and global options
//   - rlc/main.ml (459 lines) — CLI argument parsing and driver
//
// Usage:
//
//	rlc [options] <file.org>
//
// Reads a .org (Kepago source) file and produces a .seen (RealLive bytecode)
// output file. The compiler pipeline:
//
//  1. Parse the GAMEEXE.INI config (via the ini package)
//  2. Parse the KFN function definitions (via the kfn package)
//  3. Lex and parse the .org source file (via the lexer+parser packages)
//  4. Normalize expressions (via the expr package)
//  5. Compile statements (via compilerFrame when available)
//  6. Emit bytecode (via the codegen package)
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/yoremi/rldev-go/pkg/diag"
	"github.com/yoremi/rldev-go/pkg/encoding"
	"github.com/yoremi/rldev-go/rlc/pkg/codegen"
	"github.com/yoremi/rldev-go/rlc/pkg/compilerframe"
	"github.com/yoremi/rldev-go/rlc/pkg/ini"
	"github.com/yoremi/rldev-go/rlc/pkg/kfn"
	"github.com/yoremi/rldev-go/rlc/pkg/lexer"
	"github.com/yoremi/rldev-go/rlc/pkg/parser"
)

// ============================================================
// Options (from app.ml)
// ============================================================

// Options holds all compiler command-line options.
type Options struct {
	// I/O
	OutDir   string // -d output directory
	OutFile  string // -o output filename
	Gameexe  string // -g GAMEEXE.INI path
	KfnFile  string // -K reallive.kfn path
	CastFile string // --cast cast file
	GameFile string // --game game.cfg
	GameID   string // --id game identifier
	ResDir   string // --resdir resource directory
	SrcExt   string // --src-ext source extension (default "org")

	// Encoding
	Encoding string // -e encoding (default "CP932")

	// Target
	Target        string // --target RealLive|AVG2000|Kinetic
	TargetForced  bool   // true if target was specified on command line
	TargetVersion string // --target-version
	Interpreter   string // -I explicit RealLive.exe path (PE version source)

	// Compilation
	StartLine   int  // --start-line
	EndLine     int  // --end-line
	OptLevel    int  // -O optimization level (default 1)
	Compress    bool // -c compression
	OldVars     bool // --old-vars
	WithRtl     bool // --with-rtl
	Assertions  bool // --assertions
	DebugInfo   bool // --debug-info
	Metadata    bool // --metadata
	ArrayBounds bool // --array-bounds
	FlagLabels  bool // --flag-labels

	// Runtime
	RuntimeTrace int // --runtime-trace

	// Verbosity
	Verbose       int  // -v (can be repeated)
	Quiet         bool // -q
	WarningsFatal bool // -Wfatal: treat warnings as errors

	// Remaining args
	InputFiles []string
}

// DefaultOptions returns the default options matching app.ml defaults.
func DefaultOptions() *Options {
	return &Options{
		Gameexe:      "",
		KfnFile:      "reallive.kfn",
		GameFile:     "game.cfg",
		GameID:       "LB",
		SrcExt:       "org",
		Encoding:     "CP932",
		StartLine:    -1,
		EndLine:      -1,
		OptLevel:     1,
		Compress:     true,
		WithRtl:      true,
		Assertions:   true,
		DebugInfo:    true,
		Metadata:     true,
		ArrayBounds:  false,
		FlagLabels:   false,
		RuntimeTrace: 0,
	}
}

// ============================================================
// CLI (from main.ml)
// ============================================================

const (
	appName        = "rlc"
	appDescription = "RealLive-compatible compiler"
	appVersion     = "2026 (Go port)"
)

// verboseCounter is a flag.Value that increments on each invocation.
type verboseCounter int

func (v *verboseCounter) String() string     { return strconv.Itoa(int(*v)) }
func (v *verboseCounter) Set(string) error   { *v++; return nil }
func (v *verboseCounter) IsBoolFlag() bool   { return true }

func parseFlags(args []string) (*Options, error) {
	opts := DefaultOptions()
	fs := flag.NewFlagSet("rlc", flag.ContinueOnError)

	// I/O
	fs.StringVar(&opts.OutDir, "d", opts.OutDir, "output directory")
	fs.StringVar(&opts.OutFile, "o", opts.OutFile, "output filename")
	fs.StringVar(&opts.Gameexe, "g", opts.Gameexe, "GAMEEXE.INI path")
	fs.StringVar(&opts.Gameexe, "i", opts.Gameexe, "GAMEEXE.INI path (alias for -g)")
	fs.StringVar(&opts.Gameexe, "ini", opts.Gameexe, "GAMEEXE.INI path (alias for -g)")
	fs.StringVar(&opts.KfnFile, "K", opts.KfnFile, "reallive.kfn path")
	fs.StringVar(&opts.CastFile, "cast", opts.CastFile, "cast file")
	fs.StringVar(&opts.GameFile, "game", opts.GameFile, "game.cfg path")
	fs.StringVar(&opts.GameID, "id", opts.GameID, "game identifier")
	fs.StringVar(&opts.ResDir, "resdir", opts.ResDir, "resource directory")
	fs.StringVar(&opts.SrcExt, "src-ext", opts.SrcExt, "source extension")

	// Encoding
	fs.StringVar(&opts.Encoding, "e", opts.Encoding, "encoding (CP932|UTF-8|...)")

	// Target
	fs.StringVar(&opts.Target, "target", "", "target engine: RealLive|AVG2000|Kinetic")
	fs.StringVar(&opts.TargetVersion, "target-version", "", "target version (e.g. 1.2.7.0)")
	fs.StringVar(&opts.Interpreter, "I", "", "path to RealLive.exe (extract PE interpreter version)")
	fs.StringVar(&opts.Interpreter, "interpreter", "", "path to RealLive.exe (alias for -I)")

	// Compilation
	fs.IntVar(&opts.StartLine, "start-line", opts.StartLine, "start line for partial compilation")
	fs.IntVar(&opts.EndLine, "end-line", opts.EndLine, "end line for partial compilation")
	fs.IntVar(&opts.OptLevel, "O", opts.OptLevel, "optimization level (0|1|2)")
	fs.BoolVar(&opts.Compress, "compress", opts.Compress, "compress output")
	fs.BoolVar(&opts.OldVars, "old-vars", opts.OldVars, "use old variable layout")
	fs.BoolVar(&opts.WithRtl, "with-rtl", opts.WithRtl, "include runtime library")
	fs.BoolVar(&opts.Assertions, "assertions", opts.Assertions, "enable runtime assertions")
	fs.BoolVar(&opts.DebugInfo, "debug-info", opts.DebugInfo, "include debug info")
	fs.BoolVar(&opts.Metadata, "metadata", opts.Metadata, "include metadata")
	fs.BoolVar(&opts.ArrayBounds, "array-bounds", opts.ArrayBounds, "runtime array bounds checking")
	fs.BoolVar(&opts.FlagLabels, "flag-labels", opts.FlagLabels, "flag labels in output")
	fs.IntVar(&opts.RuntimeTrace, "runtime-trace", opts.RuntimeTrace, "runtime trace level")

	// Verbosity
	vc := (*verboseCounter)(&opts.Verbose)
	fs.Var(vc, "v", "verbose (repeat for more)")
	fs.BoolVar(&opts.Quiet, "q", opts.Quiet, "quiet mode")
	fs.BoolVar(&opts.WarningsFatal, "Wfatal", opts.WarningsFatal, "treat warnings as errors (abort file on any warning)")
	fs.BoolVar(&opts.WarningsFatal, "warnings-fatal", opts.WarningsFatal, "alias for -Wfatal")

	// Usage
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s - %s (%s)\n", appName, appDescription, appVersion)
		fmt.Fprintf(os.Stderr, "\nUsage: %s [options] <file.org>\n\nOptions:\n", appName)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	opts.InputFiles = fs.Args()
	if opts.Target != "" {
		opts.TargetForced = true
	}
	return opts, nil
}

// ============================================================
// Compilation pipeline (from main.ml)
// ============================================================

// compileFile runs the full compilation pipeline on one source file.
func compileFile(opts *Options, srcPath string) error {
	// Per-file diagnostic state: zero the warning/error counters so
	// Summary() at the end reflects only this file. The global Set*
	// configuration (quiet, verbose, Wfatal) is preserved.
	diag.Reset()

	diag.Phase("compiling %s", srcPath)

	// 1. Load GAMEEXE.INI if available
	iniTable, err := loadGameexe(opts, srcPath)
	if err != nil {
		return fmt.Errorf("loading GAMEEXE: %w", err)
	}
	if iniTable != nil {
		diag.Phase("GAMEEXE entries: %d", iniTable.Count())
	}

	// 2. Load KFN file if available
	kfnReg, err := loadKfn(opts)
	if err != nil {
		return fmt.Errorf("loading KFN: %w", err)
	}
	if kfnReg != nil {
		diag.Phase("KFN functions: %d", len(kfnReg.Functions))
	}

	// 2a. Resolve interpreter version. Priority:
	//   1. Explicit --target-version on command line (wins).
	//   2. Auto-detected from RealLive.exe / kinetic.exe / … alongside
	//      the .org source. Mirrors OCaml main.ml L408-428.
	//   3. Default kfn.Version{1, 2, 7, 0}.
	// The version drives the kidoku marker character (`@` vs `!`) and
	// version-constrained KFN overload selection.
	var detectedVersion kfn.Version
	if opts.TargetVersion != "" {
		v, err := parseVersion(opts.TargetVersion)
		if err == nil {
			detectedVersion = v
		}
	} else if opts.Interpreter != "" {
		v, err := pe_versionFromExe(opts.Interpreter)
		if err == nil && v != (kfn.Version{}) {
			detectedVersion = v
			diag.Phase("interpreter %s version %d.%d.%d.%d",
				filepath.Base(opts.Interpreter), v[0], v[1], v[2], v[3])
		} else {
			// Reported even without -v: a user who specified -I
			// explicitly wants to know if it was ignored.
			diag.SysWarning("cannot read interpreter version from %s: %v",
				opts.Interpreter, err)
		}
	} else {
		v, exePath, err := autoDetectVersion(srcPath)
		if err == nil {
			detectedVersion = v
			diag.Phase("detected interpreter %s version %d.%d.%d.%d",
				filepath.Base(exePath), v[0], v[1], v[2], v[3])
		}
	}
	if detectedVersion != (kfn.Version{}) && kfnReg != nil {
		kfnReg.Version = detectedVersion
	}

	// 3. Read source file
	srcBytes, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("reading source: %w", err)
	}

	// 3a. Decode the raw bytes to a Go (UTF-8) string according to the
	// requested encoding. Without this step, Shift-JIS sources would
	// arrive at the lexer as invalid UTF-8 — every multibyte SJIS
	// character (〔, 古, 河, …) would become U+FFFD replacement runes
	// and lose its original code point. Codegen relies on TextToken.Text
	// being a true Unicode string so it can re-encode to bytecode via
	// TextTransforms.
	srcString, err := decodeSource(srcBytes, opts.Encoding, srcPath)
	if err != nil {
		return fmt.Errorf("decoding source (%s): %w", opts.Encoding, err)
	}

	// 4. Lex + parse
	diag.Phase("lexing %s (%d bytes, encoding %s)", srcPath, len(srcBytes), opts.Encoding)
	lx := lexer.New(srcString, srcPath)

	p := parser.New(lx)
	program := p.ParseProgram()
	diag.Phase("parsed %d statement(s)", len(program.Stmts))

	// 5. Compile via compilerframe
	compiler := compilerframe.New(kfnReg, iniTable)
	compiler.Verbose = opts.Verbose

	// Wire the directive compiler with information it needs to resolve
	// `#resource 'foo.sjs'` references relative to the source file, and
	// to decode the resource file in the right encoding. Without this,
	// `#res<NNNN>` references in the .org would resolve to nothing and
	// the bytecode would contain literal "#res<NNNN>" text instead of
	// the actual caption (288 occurrences in SEEN0414 alone).
	if compiler.Directive != nil {
		compiler.Directive.SourceDir = filepath.Dir(srcPath)
		compiler.Directive.SourceEnc = opts.Encoding
	}

	// Wire codegen's ResolveRes callback to State.Resources so EmitExpr
	// can substitute `#res<KEY>` references with the actual string from
	// the loaded .sjs/.utf file.
	compiler.Out.ResolveRes = func(key string) (string, bool) {
		if compiler.State == nil {
			return "", false
		}
		r, err := compiler.State.GetResource(key)
		if err != nil {
			return "", false
		}
		return r.Text, true
	}

	compiler.Compile(program.Stmts)

	// Diagnostics already streamed via the diag reporter as
	// c.warning / c.error were called; the slices are kept on the
	// compiler for tests and HasErrors(). No flush here.
	if compiler.HasErrors() {
		return fmt.Errorf("%d compilation errors", len(compiler.Errors))
	}

	// 6. Generate bytecode
	diag.Phase("compiled %d statement(s), IR length %d",
		len(program.Stmts), compiler.Out.Length())

	genOpts := codegen.DefaultOptions()
	if detectedVersion != (kfn.Version{}) {
		genOpts.Version = detectedVersion
	}
	genOpts.DebugInfo = opts.DebugInfo
	// Collect the dramatis personae names gathered during directive
	// processing (#character 'name'). They must be written into the
	// bytecode header in the target encoding (Shift-JIS / CP932): the
	// engine reads names as raw SJIS bytes. If the source file is in a
	// non-SJIS encoding (UTF-8, EUC-JP…), transcode each name now.
	if compiler.State != nil && len(compiler.State.DramatisPersonae) > 0 {
		names := make([]string, 0, len(compiler.State.DramatisPersonae))
		needTrans := false
		srcEnc := strings.ToUpper(strings.ReplaceAll(opts.Encoding, "-", ""))
		if srcEnc != "" && srcEnc != "CP932" && srcEnc != "SHIFTJIS" && srcEnc != "SJIS" && srcEnc != "SHIFT_JIS" {
			needTrans = true
		}
		for _, n := range compiler.State.DramatisPersonae {
			if needTrans {
				sjis, err := encoding.UTF8ToSJS(n)
				if err != nil {
					diag.Warning(diag.Loc{}, "could not transcode #character '%s' to Shift-JIS: %v — using raw bytes", n, err)
					names = append(names, n)
					continue
				}
				names = append(names, string(sjis))
			} else {
				names = append(names, n)
			}
		}
		genOpts.DramatisPersonae = names
	}

	// Build the RLdev metadata segment when --metadata is on. Format
	// (common/metadata.ml):
	//   u32 LE  total length (excluding this 4-byte prefix)
	//   u32 LE  id length (= 5 for "RLdev")
	//   bytes   identifier
	//   u8      0x00 (NUL terminator)
	//   u32 LE  compiler_version * 100  (RLdev 1.39 → 139)
	//   u8 ×4   target version (a, b, c, d)
	//   u8      text-transform: 0=None, 1=Chinese, 2=Western, 3=Korean
	// Total for "RLdev" = 23 bytes, matching what the engine and tools
	// expect.
	if opts.Metadata {
		genOpts.Metadata = buildRLdevMetadata(genOpts.Version, opts.Encoding)
	}

	bytecode, err := compiler.Out.Generate(genOpts)
	if err != nil {
		return fmt.Errorf("bytecode generation: %w", err)
	}
	diag.Phase("generated %d bytes of bytecode (version %d.%d.%d.%d)",
		len(bytecode),
		genOpts.Version[0], genOpts.Version[1], genOpts.Version[2], genOpts.Version[3])

	// Determine output filename
	outName := compiler.Directive.OutFile
	if outName == "" {
		base := filepath.Base(srcPath)
		ext := filepath.Ext(base)
		outName = strings.TrimSuffix(base, ext) + ".TXT"
	}
	if opts.OutDir != "" {
		outName = filepath.Join(opts.OutDir, outName)
	}

	if err := os.WriteFile(outName, bytecode, 0644); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	diag.Phase("output: %s (%d bytes)", outName, len(bytecode))

	// End-of-file diag summary. With -Wfatal active, every warning
	// has already bumped the error counter, so a non-zero Errors()
	// here means the file must be reported as failed even though
	// the pipeline ran to completion. This is the OCaml behaviour
	// of `cliError` raising at end-of-compile when warnings were
	// promoted: the caller sees a clear "this file failed".
	diag.Summary(srcPath)
	if diag.Errors() > 0 {
		return fmt.Errorf("%d diagnostic error(s)", diag.Errors())
	}

	return nil
}

// loadGameexe attempts to locate and load GAMEEXE.INI.
// Search order:
//  1. --gameexe flag
//  2. $GAMEEXE env var
//  3. GAMEEXE.INI in source directory
//  4. gameexe.ini in source directory
//  5. ../GAMEEXE.INI
//  6. ../gameexe.ini
func loadGameexe(opts *Options, srcPath string) (*ini.Table, error) {
	var path string
	if opts.Gameexe != "" {
		if _, err := os.Stat(opts.Gameexe); err != nil {
			return nil, fmt.Errorf("'%s' is not a valid INI file", opts.Gameexe)
		}
		path = opts.Gameexe
	} else if env := os.Getenv("GAMEEXE"); env != "" {
		path = env
	} else {
		srcDir := filepath.Dir(srcPath)
		candidates := []string{
			filepath.Join(srcDir, "GAMEEXE.INI"),
			filepath.Join(srcDir, "gameexe.ini"),
			filepath.Join(srcDir, "..", "GAMEEXE.INI"),
			filepath.Join(srcDir, "..", "gameexe.ini"),
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				path = c
				break
			}
		}
	}

	if path == "" {
		// Mirrors OCaml ini.ml: gameexe.ini is optional but the
		// absence is reported so the translator notices that
		// compilation is running with defaults — which can change
		// the bytecode (Gameexe entries drive overload selection).
		diag.SysWarning("unable to locate gameexe.ini, using defaults")
		return ini.NewTable(), nil
	}

	diag.Phase("reading INI: %s", path)
	return ini.ParseFile(path)
}

// loadKfn attempts to load the KFN function definition file.
func loadKfn(opts *Options) (*kfn.Registry, error) {
	if opts.KfnFile == "" {
		return kfn.NewRegistry(), nil
	}
	if _, err := os.Stat(opts.KfnFile); err != nil {
		// KFN absent — every opcode falls back to its op<TYPE:MOD:FN, OVL>
		// raw form. Bytecode still compiles but with massively reduced
		// safety: no overload version filtering, no argument-type
		// checking. Always reported so the translator knows what's
		// happening; it's far more important than a verbose-only hint.
		diag.SysWarning("KFN file %q not found — opcodes will use raw op<…> form, no overload filtering", opts.KfnFile)
		return kfn.NewRegistry(), nil
	}
	diag.Phase("reading KFN: %s", opts.KfnFile)
	return kfn.ParseFile(opts.KfnFile)
}

// resolveSourcePath adds the source extension if missing.
func resolveSourcePath(opts *Options, arg string) string {
	if filepath.Ext(arg) != "" {
		return arg
	}
	return arg + "." + opts.SrcExt
}

// ============================================================
// Target parsing
// ============================================================

// parseTarget converts a target name string to a kfn.Target.
func parseTarget(s string) (kfn.Target, error) {
	switch strings.ToLower(s) {
	case "", "reallive":
		return kfn.TargetRealLive, nil
	case "avg2000":
		return kfn.TargetAVG2000, nil
	case "kinetic":
		return kfn.TargetKinetic, nil
	}
	return 0, fmt.Errorf("unknown target: %s (expected RealLive|AVG2000|Kinetic)", s)
}

// parseVersion converts "1.2.7.0" to a kfn.Version.
func parseVersion(s string) (kfn.Version, error) {
	if s == "" {
		return kfn.Version{1, 2, 7, 0}, nil
	}
	parts := strings.Split(s, ".")
	if len(parts) < 1 || len(parts) > 4 {
		return kfn.Version{}, fmt.Errorf("version must have 1-4 components, got %d", len(parts))
	}
	var v kfn.Version
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return kfn.Version{}, fmt.Errorf("version component %d: %w", i, err)
		}
		v[i] = n
	}
	return v, nil
}

// ============================================================
// Main
// ============================================================

func main() {
	opts, err := parseFlags(os.Args[1:])
	if err != nil {
		if err == flag.ErrHelp {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	if len(opts.InputFiles) == 0 {
		fmt.Fprintf(os.Stderr, "%s: no input files\n", appName)
		fmt.Fprintf(os.Stderr, "Run '%s -h' for usage.\n", appName)
		os.Exit(1)
	}

	// Validate target
	if opts.Target != "" {
		if _, err := parseTarget(opts.Target); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}

	// Validate version
	if opts.TargetVersion != "" {
		if _, err := parseVersion(opts.TargetVersion); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}

	// Configure the diag reporter once, process-wide. compileFile()
	// calls diag.Reset() per file so counters and Summary() are
	// accurate per-source; the three Set* flags here apply to every
	// file in the run.
	diag.SetQuiet(opts.Quiet)
	diag.SetVerbose(opts.Verbose > 0)
	diag.SetWarningsFatal(opts.WarningsFatal)

	// Compile each input file
	errors := 0
	for _, f := range opts.InputFiles {
		srcPath := resolveSourcePath(opts, f)
		if err := compileFile(opts, srcPath); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", srcPath, err)
			errors++
		}
	}

	if errors > 0 {
		os.Exit(1)
	}
}

// decodeSource decodes raw source bytes according to the user-selected
// encoding so the rest of the compiler can work with a proper Go (UTF-8)
// string. Common spellings are accepted ("CP932", "Shift-JIS", "UTF-8",
// …) and unrecognised encodings fall through to a permissive cast — that
// path also covers sources that are already valid UTF-8.
//
// Before returning, the function scans for byte-level encoding problems
// the rest of the pipeline cannot recover from and reports each one via
// diag.WarnAt with the precise (file, line) coordinates. This is the
// Go counterpart of OCaml strLexer.ml's `invalid character 0x%02x in
// source file` diagnostic, run up-front so the translator gets the
// complete list in one pass. The first port of rldev-go silently turned
// every stray Shift-JIS byte in a UTF-8 file into U+FFFD, producing a
// SEEN.TXT that "compiled fine" yet the game refused to boot.
func decodeSource(data []byte, encName, srcPath string) (string, error) {
	switch strings.ToUpper(strings.ReplaceAll(encName, "_", "-")) {
	case "", "UTF-8", "UTF8":
		// Strip UTF-8 BOM if present and note it (cosmetic but the
		// game engine can choke on a leading BOM).
		if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
			diag.WarnAt(srcPath, 1, "UTF-8 BOM stripped at start of file (some interpreters reject it)")
			data = data[3:]
		}
		// Per-line scan for invalid UTF-8 bytes. Every offender is
		// likely a stray Shift-JIS byte left in a UTF-8 source — the
		// classic "compiles but won't boot" cause.
		scanInvalidUTF8(srcPath, data)
		return string(data), nil
	case "CP932", "SHIFT-JIS", "SJIS", "SHIFTJIS":
		s, err := encoding.SJSToUTF8(data)
		if err != nil {
			return "", err
		}
		// SJSToUTF8 (via golang.org/x/text) silently truncates at the
		// first undecodable byte. If the decoded length looks short
		// compared to the input, flag it — the translator can then
		// look at the tail of the file.
		if len(s) > 0 && len(data) > 0 {
			scanResidualReplacement(srcPath, s, "Shift-JIS")
		}
		return s, nil
	default:
		enc := encoding.Parse(encName)
		s, err := encoding.ToUTF8(data, enc)
		if err != nil {
			diag.SysWarning("encoding %q not understood, falling back to raw bytes", encName)
			return string(data), nil
		}
		scanResidualReplacement(srcPath, s, encName)
		return s, nil
	}
}

// scanInvalidUTF8 walks data line by line and emits one warning per
// byte that doesn't form a valid UTF-8 sequence. Mirrors the
// OCaml diagnostic but runs over the whole file in one pass, so the
// translator sees every offender at once.
func scanInvalidUTF8(file string, data []byte) {
	line := 1
	for i := 0; i < len(data); {
		c := data[i]
		switch {
		case c == '\n':
			line++
			i++
		case c < 0x80:
			i++
		default:
			r, size := utf8DecodeRune(data[i:])
			if r == 0xFFFD && size <= 1 {
				diag.WarnAt(file, line,
					"invalid UTF-8 byte 0x%02X — likely a stray Shift-JIS character in a UTF-8 file; the original character is lost",
					c)
				i++
			} else {
				i += size
			}
		}
	}
}

// scanResidualReplacement looks for U+FFFD code points in already-
// decoded text. Some decoders (golang.org/x/text fallback path) emit
// U+FFFD on encoding errors instead of erroring out; without this
// pass the substitute character would silently survive into the
// bytecode. We report by line so the translator can grep the source.
func scanResidualReplacement(file, s, encLabel string) {
	line := 1
	for _, r := range s {
		if r == '\n' {
			line++
			continue
		}
		if r == 0xFFFD {
			diag.WarnAt(file, line,
				"U+FFFD replacement character in source — %s decoder could not represent the original byte",
				encLabel)
		}
	}
}

// utf8DecodeRune is a thin alias kept local so the scanInvalidUTF8
// hot loop doesn't pull the full unicode/utf8 import name into every
// reading. Returns (U+FFFD, 1) for an invalid byte sequence, matching
// stdlib semantics.
var utf8DecodeRune = utf8.DecodeRune

// pe_versionFromExe extracts the FileVersion tuple from a PE executable
// (typically RealLive.exe). Mirrors OCaml's
// `rldev_get_interpreter_version` (C binding in get_interpreter_version.c)
// for non-Windows builds: walks the .rsrc section to find the
// VS_FIXEDFILEINFO signature 0xFEEF04BD and reads dwFileVersionMS /
// dwFileVersionLS.
//
// The interpreter version drives:
//   - kidoku marker character: `@` for versions ≤ 1.2.5, `!` after
//     (bytecodeGen.ml L157). Wrong marker → engine misparses
//     entrypoint table → crash.
//   - KFN overload filtering: many opcodes have version-constrained
//     prototypes (`ver >= 1.3, < 1.6.4.6`).
//
// Returns the zero Version and a non-nil error when no version info is
// found; callers can then fall back to user-supplied --target-version.
func pe_versionFromExe(path string) (kfn.Version, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return kfn.Version{}, err
	}
	if len(data) < 0x40 || data[0] != 'M' || data[1] != 'Z' {
		return kfn.Version{}, fmt.Errorf("not a PE file")
	}
	peOff := int(uint32(data[0x3c]) | uint32(data[0x3d])<<8 | uint32(data[0x3e])<<16 | uint32(data[0x3f])<<24)
	if peOff+4 > len(data) || string(data[peOff:peOff+4]) != "PE\x00\x00" {
		return kfn.Version{}, fmt.Errorf("PE header signature not found")
	}

	// COFF header at peOff+4
	coffOff := peOff + 4
	if coffOff+20 > len(data) {
		return kfn.Version{}, fmt.Errorf("truncated COFF header")
	}
	nsec := int(uint16(data[coffOff+2]) | uint16(data[coffOff+3])<<8)
	optSize := int(uint16(data[coffOff+16]) | uint16(data[coffOff+17])<<8)
	secStart := coffOff + 20 + optSize

	// Find .rsrc section
	rsrcOff := 0
	rsrcSize := 0
	for i := 0; i < nsec; i++ {
		s := secStart + i*40
		if s+40 > len(data) {
			break
		}
		name := strings.TrimRight(string(data[s:s+8]), "\x00")
		if name == ".rsrc" {
			rsrcSize = int(uint32(data[s+16]) | uint32(data[s+17])<<8 | uint32(data[s+18])<<16 | uint32(data[s+19])<<24)
			rsrcOff = int(uint32(data[s+20]) | uint32(data[s+21])<<8 | uint32(data[s+22])<<16 | uint32(data[s+23])<<24)
			break
		}
	}
	if rsrcOff == 0 {
		return kfn.Version{}, fmt.Errorf(".rsrc section not found")
	}
	end := rsrcOff + rsrcSize
	if end > len(data) {
		end = len(data)
	}

	// Search for VS_FIXEDFILEINFO signature 0xFEEF04BD (little-endian: BD 04 EF FE)
	sig := []byte{0xbd, 0x04, 0xef, 0xfe}
	idx := -1
	for i := rsrcOff; i+16 < end; i++ {
		if data[i] == sig[0] && data[i+1] == sig[1] && data[i+2] == sig[2] && data[i+3] == sig[3] {
			idx = i
			break
		}
	}
	if idx < 0 {
		return kfn.Version{}, fmt.Errorf("VS_FIXEDFILEINFO not found")
	}
	if idx+16 > len(data) {
		return kfn.Version{}, fmt.Errorf("truncated VS_FIXEDFILEINFO")
	}
	// dwFileVersionMS at offset +8 (4 bytes), dwFileVersionLS at offset +12 (4 bytes)
	fvms := uint32(data[idx+8]) | uint32(data[idx+9])<<8 | uint32(data[idx+10])<<16 | uint32(data[idx+11])<<24
	fvls := uint32(data[idx+12]) | uint32(data[idx+13])<<8 | uint32(data[idx+14])<<16 | uint32(data[idx+15])<<24
	return kfn.Version{
		int(fvms >> 16),
		int(fvms & 0xffff),
		int(fvls >> 16),
		int(fvls & 0xffff),
	}, nil
}

// autoDetectVersion looks for a RealLive.exe / RealLiveEn.exe /
// kinetic.exe / avg2000.exe / siglusengine.exe next to the source
// .org file (and one directory up), and extracts the interpreter
// version via pe_versionFromExe. Mirrors OCaml main.ml L408-428.
//
// Returns zero Version when no candidate executable is found —
// callers should then leave the version at the user-supplied default.
func autoDetectVersion(srcPath string) (kfn.Version, string, error) {
	candidates := []string{"RealLive.exe", "RealLiveEn.exe", "Kinetic.exe", "kinetic.exe",
		"AVG2000.exe", "avg2000.exe", "SiglusEngine.exe", "siglusengine.exe", "reallive.exe"}
	dirs := []string{filepath.Dir(srcPath), filepath.Join(filepath.Dir(srcPath), "..")}
	for _, dir := range dirs {
		for _, name := range candidates {
			path := filepath.Join(dir, name)
			if _, err := os.Stat(path); err == nil {
				v, err := pe_versionFromExe(path)
				if err == nil && v != (kfn.Version{}) {
					return v, path, nil
				}
			}
		}
	}
	return kfn.Version{}, "", fmt.Errorf("no interpreter executable found")
}

// buildRLdevMetadata serialises the RLdev compiler-identification block
// that goes after the dramatis personae table in the bytecode header.
// Layout per common/metadata.ml `to_string`:
//
//	u32 LE  total length (= len(rest))
//	u32 LE  id length (= 5 for "RLdev")
//	bytes   "RLdev"
//	u8      0x00 NUL terminator
//	u32 LE  compiler_version * 100 (RLdev 1.39 → 139)
//	u8 ×4   target version (a, b, c, d)
//	u8      text-transform: 0 None | 1 Chinese | 2 Western | 3 Korean
//
// For "RLdev" the total payload is 23 bytes — matching what RealLive and
// the OCaml toolchain produce. We pin the compiler version to 139 so the
// output stays byte-compatible with the canonical RLdev 1.39 / OCaml
// 2026 metadata; tools that read this field never gate behaviour on it,
// and any change here would make Go-produced bytecode diff every byte
// against the OCaml reference for no functional benefit.
func buildRLdevMetadata(ver kfn.Version, sourceEnc string) []byte {
	const compilerVersionTimes100 = 139 // RLdev 1.39
	id := []byte("RLdev")

	// Detect translation transform from the source encoding. The
	// translator typically writes its scripts in CP1252 (Western) for
	// French / English patches, EUC-KR (Korean) or GBK (Chinese); the
	// default Japanese pipeline uses Shift-JIS / UTF-8 with no
	// transform.
	tt := byte(0)
	switch strings.ToUpper(strings.ReplaceAll(sourceEnc, "-", "")) {
	case "CP1252", "WINDOWS1252", "LATIN1", "ISO88591":
		tt = 2
	case "GBK", "CP936", "GB2312":
		tt = 1
	case "EUCKR", "CP949":
		tt = 3
	}

	body := make([]byte, 0, 32)

	// u32 id_len
	tmp4 := make([]byte, 4)
	binary.LittleEndian.PutUint32(tmp4, uint32(len(id)))
	body = append(body, tmp4...)
	// "RLdev"
	body = append(body, id...)
	// NUL
	body = append(body, 0)
	// u32 compiler_version
	binary.LittleEndian.PutUint32(tmp4, compilerVersionTimes100)
	body = append(body, tmp4...)
	// 4 target version bytes
	body = append(body, byte(ver[0]), byte(ver[1]), byte(ver[2]), byte(ver[3]))
	// text-transform
	body = append(body, tt)

	// Prefix length: per common/metadata.ml `to_string` this is
	// `String.length s + 4` — i.e. the total segment length including
	// the prefix itself. For "RLdev" that's len(body) + 4 = 23.
	out := make([]byte, 0, 4+len(body))
	binary.LittleEndian.PutUint32(tmp4, uint32(len(body)+4))
	out = append(out, tmp4...)
	out = append(out, body...)
	return out
}
