// kprl is a RealLive archiver and disassembler.
// Transposed from OCaml rldev2026's kprl tool.
//
// Usage:
//
//	kprl [options] <action> <archive|files> [ranges]
//
// Actions:
//
//	-a, --add           Add files to archive
//	-k, --delete        Remove files from archive
//	-l, --list          List archive contents
//	-b, --break         Extract files (compressed)
//	-x, --extract       Extract and decompress files
//	-d, --disassemble   Disassemble bytecode (default)
//	-c, --compress      Compress standalone files
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yoremi/rldev-go/pkg/binarray"
	"github.com/yoremi/rldev-go/pkg/bytecode"
	"github.com/yoremi/rldev-go/pkg/disasm"
	"github.com/yoremi/rldev-go/pkg/gamedef"
	"github.com/yoremi/rldev-go/pkg/kprl"
	"github.com/yoremi/rldev-go/pkg/rlcmp"
)

const version = "2.0.26-go"

// Action selectors
var (
	actionAdd         = flag.Bool("a", false, "add files to archive")
	actionDelete      = flag.Bool("k", false, "remove files from archive")
	actionList        = flag.Bool("l", false, "list archive contents")
	actionInfo        = flag.Bool("i", false, "display file info")
	actionBreak       = flag.Bool("b", false, "extract files (still compressed)")
	actionExtract     = flag.Bool("x", false, "extract and decompress files")
	actionDisassemble = flag.Bool("d", false, "disassemble bytecode")
	actionCompress    = flag.Bool("c", false, "compress standalone files")
)

// General options
var (
	verbose = flag.Int("v", 0, "verbosity level (0-2)")
	outdir  = flag.String("o", "", "output directory")
	gameID  = flag.String("G", "", "game ID (LB, LBEX, CFV, FIVE, SNOW)")
)

// Disassembly options
var (
	encoding       = flag.String("e", "CP932", "output text encoding")
	bom            = flag.Bool("bom", false, "include UTF-8 BOM")
	singleFile     = flag.Bool("s", false, "don't separate text into resource file")
	separateAll    = flag.Bool("S", false, "put all text in resource file")
	suppressUnref  = flag.Bool("u", false, "suppress unreferenced code")
	annotate       = flag.Bool("n", false, "annotate with offsets")
	noCodes        = flag.Bool("r", false, "don't generate control codes")
	debugInfo      = flag.Bool("g", false, "read debug information")
	target         = flag.String("t", "", "target: RealLive, AVG2000, Kinetic")
	srcExt         = flag.String("ext", "org", "source file extension")
	showOpcodes    = flag.Bool("opcodes", false, "show opcode annotations")
	hexDump        = flag.Bool("hexdump", false, "generate hex dump")
	rawStrings     = flag.Bool("raw-strings", false, "no special markup in strings")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "kprl %s - RealLive archiver and disassembler\n\n", version)
		fmt.Fprintf(os.Stderr, "Usage: kprl [options] <files or archive> [ranges]\n\n")
		fmt.Fprintf(os.Stderr, "Actions (pick one):\n")
		fmt.Fprintf(os.Stderr, "  -a    add files to archive\n")
		fmt.Fprintf(os.Stderr, "  -k    remove files from archive\n")
		fmt.Fprintf(os.Stderr, "  -l    list archive contents\n")
		fmt.Fprintf(os.Stderr, "  -i    display file info\n")
		fmt.Fprintf(os.Stderr, "  -b    extract files (still compressed)\n")
		fmt.Fprintf(os.Stderr, "  -x    extract and decompress files\n")
		fmt.Fprintf(os.Stderr, "  -d    disassemble bytecode (default)\n")
		fmt.Fprintf(os.Stderr, "  -c    compress standalone files\n")
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
	}

	flag.Parse()
	args := flag.Args()

	if len(args) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	// Resolve game keys
	opts := kprl.Options{
		Verbose: *verbose,
		OutDir:  *outdir,
		GameID:  *gameID,
	}

	if *gameID != "" {
		if keys, ok := gamedef.KnownGames[strings.ToUpper(*gameID)]; ok {
			opts.Keys = keys
		}
	}

	// Default output dir
	if opts.OutDir == "" {
		opts.OutDir = "."
	}

	// Determine and run action
	var err error

	switch {
	case *actionAdd:
		if len(args) < 2 {
			fatal("add requires: <archive> <files...>")
		}
		err = kprl.Add(args[0], args[1:], opts)

	case *actionDelete:
		if len(args) < 2 {
			fatal("delete requires: <archive> <ranges...>")
		}
		ranges, parseErr := kprl.ParseRanges(args[1:])
		if parseErr != nil {
			fatal("bad range: %v", parseErr)
		}
		err = kprl.Remove(args[0], ranges, opts)

	case *actionList:
		err = doList(args, opts)

	case *actionInfo:
		err = doInfo(args, opts)

	case *actionBreak:
		err = doBreak(args, opts)

	case *actionExtract:
		err = doExtract(args, opts)

	case *actionCompress:
		err = kprl.Pack(args, opts)

	case *actionDisassemble:
		err = doDisassemble(args, opts)

	default:
		// Default action: disassemble
		if *verbose > 0 {
			fmt.Fprintln(os.Stderr, "No action specified, performing disassembly by default...")
		}
		err = doDisassemble(args, opts)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// --- Action implementations ---

func doList(args []string, opts kprl.Options) error {
	fname := args[0]
	var ranges []int
	if len(args) > 1 {
		var err error
		ranges, err = kprl.ParseRanges(args[1:])
		if err != nil {
			return err
		}
	}
	return kprl.List(fname, ranges, opts)
}

func doInfo(args []string, opts kprl.Options) error {
	fname := args[0]
	// For info, show archive structure details
	arc, err := kprl.LoadArchive(fname)
	if err != nil {
		return err
	}

	fmt.Printf("Archive: %s (%d entries)\n\n", filepath.Base(fname), arc.Count)
	fmt.Printf("%-16s %8s %10s %10s %7s\n", "File", "Index", "Offset", "Length", "Ratio")
	fmt.Println(strings.Repeat("-", 55))

	for i := 0; i < kprl.MaxSeens; i++ {
		entry := arc.Entries[i]
		if entry.Length == 0 {
			continue
		}

		sub := kprl.GetSubfile(arc.Data, i)
		if sub == nil {
			continue
		}

		name := fmt.Sprintf("SEEN%04d.TXT", i)
		hdr, err := bytecode.ReadFullHeader(sub, true)
		if err != nil {
			fmt.Printf("%-16s %8d %10d %10d  [error]\n", name, i, entry.Offset, entry.Length)
			continue
		}

		unc := hdr.UncompressedSize + hdr.DataOffset
		if hdr.IsCompressed {
			cmp := hdr.CompressedSize + hdr.DataOffset
			ratio := float64(cmp) / float64(unc) * 100.0
			fmt.Printf("%-16s %8d %10d %10d %6.1f%%\n", name, i, entry.Offset, entry.Length, ratio)
		} else {
			fmt.Printf("%-16s %8d %10d %10d\n", name, i, entry.Offset, entry.Length)
		}
	}

	return nil
}

func doBreak(args []string, opts kprl.Options) error {
	fname := args[0]
	var ranges []int
	if len(args) > 1 {
		var err error
		ranges, err = kprl.ParseRanges(args[1:])
		if err != nil {
			return err
		}
	}
	return kprl.Break(fname, ranges, opts)
}

func doExtract(args []string, opts kprl.Options) error {
	fname := args[0]
	var ranges []int
	if len(args) > 1 {
		var err error
		ranges, err = kprl.ParseRanges(args[1:])
		if err != nil {
			return err
		}
	}
	return kprl.Extract(fname, ranges, opts)
}

func doDisassemble(args []string, opts kprl.Options) error {
	disOpts := disasm.Options{
		SeparateStrings:  !*singleFile,
		SeparateAll:      *separateAll,
		ReadDebugSymbols: *debugInfo,
		Annotate:         *annotate,
		ControlCodes:     !*noCodes,
		SuppressUncalled: *suppressUnref,
		ShowOpcodes:      *showOpcodes,
		HexDump:          *hexDump,
		RawStrings:       *rawStrings,
		SrcExt:           *srcExt,
		Encoding:         *encoding,
		BOM:              *bom,
		Verbose:          *verbose,
	}

	if *target != "" {
		switch strings.ToLower(*target) {
		case "reallive", "2":
			disOpts.ForcedTarget = disasm.ModeRealLive
		case "avg2000", "avg2k", "1":
			disOpts.ForcedTarget = disasm.ModeAvg2000
		case "kinetic", "3":
			disOpts.ForcedTarget = disasm.ModeKinetic
		default:
			return fmt.Errorf("unknown target: %s", *target)
		}
	}

	writer := disasm.NewWriter(opts.OutDir, disOpts)

	// Check if first file is an archive
	firstFile := args[0]
	if kprl.IsArchive(firstFile) {
		return disassembleArchive(firstFile, args[1:], opts, disOpts, writer)
	}

	// Process individual files
	for _, fname := range args {
		if err := disassembleFile(fname, opts, disOpts, writer); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
	}
	return nil
}

func disassembleArchive(arcName string, rangeArgs []string, opts kprl.Options, disOpts disasm.Options, writer *disasm.Writer) error {
	arc, err := kprl.LoadArchive(arcName)
	if err != nil {
		return err
	}

	var ranges []int
	if len(rangeArgs) > 0 {
		ranges, err = kprl.ParseRanges(rangeArgs)
		if err != nil {
			return err
		}
	}

	// If no ranges specified, process all
	if ranges == nil {
		for i := 0; i < kprl.MaxSeens; i++ {
			if arc.Entries[i].Length > 0 {
				ranges = append(ranges, i)
			}
		}
	}

	for _, i := range ranges {
		sub := kprl.GetSubfile(arc.Data, i)
		if sub == nil {
			continue
		}

		seenName := fmt.Sprintf("SEEN%04d.TXT", i)
		if *verbose > 0 {
			fmt.Printf("Disassembling %s\n", seenName)
		}

		// Decompress if needed
		data := binarray.Copy(sub)
		if data.Len() >= 4 && !bytecode.UncompressedHeader(data.Read(0, 4)) {
			decompressed, err := rlcmp.Decompress(data, opts.Keys, true)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to decompress %s: %v\n", seenName, err)
				continue
			}
			data = decompressed
		}

		result, err := disasm.Disassemble(data, disOpts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to disassemble %s: %v\n", seenName, err)
			continue
		}

		if result.Error != "" && *verbose > 0 {
			fmt.Fprintf(os.Stderr, "Warning: %s: %s\n", seenName, result.Error)
		}

		if err := writer.WriteSource(seenName, result); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write %s: %v\n", seenName, err)
		}
	}

	return nil
}

func disassembleFile(fname string, opts kprl.Options, disOpts disasm.Options, writer *disasm.Writer) error {
	arr, err := binarray.ReadFile(fname)
	if err != nil {
		return fmt.Errorf("cannot read '%s': %w", fname, err)
	}

	// Decompress if needed
	if arr.Len() >= 4 && !bytecode.UncompressedHeader(arr.Read(0, 4)) {
		decompressed, err := rlcmp.Decompress(arr, opts.Keys, true)
		if err != nil {
			return fmt.Errorf("failed to decompress '%s': %w", fname, err)
		}
		arr = decompressed
	}

	result, err := disasm.Disassemble(arr, disOpts)
	if err != nil {
		return fmt.Errorf("failed to disassemble '%s': %w", fname, err)
	}

	baseName := filepath.Base(fname)
	return writer.WriteSource(baseName, result)
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}
