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
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
	Verbose int  // -v (can be repeated)
	Quiet   bool // -q

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
	if opts.Verbose > 0 {
		fmt.Fprintf(os.Stderr, "Compiling: %s\n", srcPath)
	}

	// 1. Load GAMEEXE.INI if available
	iniTable, err := loadGameexe(opts, srcPath)
	if err != nil {
		return fmt.Errorf("loading GAMEEXE: %w", err)
	}
	if opts.Verbose > 1 {
		fmt.Fprintf(os.Stderr, "  GAMEEXE entries: %d\n", iniTable.Count())
	}

	// 2. Load KFN file if available
	kfnReg, err := loadKfn(opts)
	if err != nil {
		return fmt.Errorf("loading KFN: %w", err)
	}
	if opts.Verbose > 1 && kfnReg != nil {
		fmt.Fprintf(os.Stderr, "  KFN functions: %d\n", len(kfnReg.Functions))
	}

	// 3. Read source file
	srcBytes, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("reading source: %w", err)
	}

	// 4. Lex + parse
	lx := lexer.New(string(srcBytes), srcPath)
	if opts.Verbose > 1 {
		fmt.Fprintf(os.Stderr, "  Lexer created for %s\n", srcPath)
	}

	p := parser.New(lx)
	program := p.ParseProgram()
	if opts.Verbose > 1 {
		fmt.Fprintf(os.Stderr, "  Statements: %d\n", len(program.Stmts))
	}

	// 5. Compile via compilerframe
	compiler := compilerframe.New(kfnReg, iniTable)
	compiler.Verbose = opts.Verbose
	compiler.Compile(program.Stmts)

	// Report diagnostics
	for _, w := range compiler.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
	if compiler.HasErrors() {
		for _, e := range compiler.Errors {
			fmt.Fprintf(os.Stderr, "error: %v\n", e)
		}
		return fmt.Errorf("%d compilation errors", len(compiler.Errors))
	}

	// 6. Generate bytecode
	if opts.Verbose > 0 {
		fmt.Fprintf(os.Stderr, "  Compiled %d statements, IR length: %d\n",
			len(program.Stmts), compiler.Out.Length())
	}

	genOpts := codegen.DefaultOptions()
	bytecode, err := compiler.Out.Generate(genOpts)
	if err != nil {
		return fmt.Errorf("bytecode generation: %w", err)
	}

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

	if opts.Verbose > 0 {
		fmt.Fprintf(os.Stderr, "  Output: %s (%d bytes)\n", outName, len(bytecode))
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
		if opts.Verbose > 0 {
			fmt.Fprintln(os.Stderr, "warning: unable to locate gameexe.ini, using defaults")
		}
		return ini.NewTable(), nil
	}

	if opts.Verbose > 0 {
		fmt.Fprintf(os.Stderr, "Reading INI: %s\n", path)
	}
	return ini.ParseFile(path)
}

// loadKfn attempts to load the KFN function definition file.
func loadKfn(opts *Options) (*kfn.Registry, error) {
	if opts.KfnFile == "" {
		return kfn.NewRegistry(), nil
	}
	if _, err := os.Stat(opts.KfnFile); err != nil {
		// KFN not found → empty registry (acceptable during porting)
		if opts.Verbose > 0 {
			fmt.Fprintf(os.Stderr, "warning: %s not found, using empty registry\n", opts.KfnFile)
		}
		return kfn.NewRegistry(), nil
	}
	if opts.Verbose > 0 {
		fmt.Fprintf(os.Stderr, "Reading KFN: %s\n", opts.KfnFile)
	}
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
