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
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/yoremi/rldev-go/pkg/avg32"
	"github.com/yoremi/rldev-go/pkg/binarray"
	"github.com/yoremi/rldev-go/pkg/bytecode"
	"github.com/yoremi/rldev-go/pkg/diag"
	"github.com/yoremi/rldev-go/pkg/disasm"
	"github.com/yoremi/rldev-go/pkg/gamedef"
	"github.com/yoremi/rldev-go/pkg/kprl"
	"github.com/yoremi/rldev-go/pkg/rlcmp"
	"github.com/yoremi/rldev-go/pkg/texttransforms"
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
	verbose         = flag.Int("v", 0, "verbosity level (0-2)")
	outdir          = flag.String("o", "", "output directory")
	gameID          = flag.String("G", "", "game ID (CFV, LB, LBEX, LBME, LBPE, FIVE, SNOW, KUDO, KUDA, PLHD, TMPE, ONIU, ONIUTA, PING, KOYO, SHINO, TAMA, PRIP, PRID, HINA, LUV)")
	archiveTemplate = flag.String("template", "", "template SEEN.TXT whose trailing data is preserved when rebuilding")
)

// Disassembly options
var (
	encoding        = flag.String("e", "CP932", "output text encoding")
	outputTransform = flag.String("transform-output", "", "text transform override: none, western, chinese, korean")
	forceTransform  = flag.Bool("force-transform", false, "replace unmappable characters when using a text transform")
	bom             = flag.Bool("bom", false, "include UTF-8 BOM")
	singleFile      = flag.Bool("s", false, "don't separate text into resource file")
	separateAll     = flag.Bool("S", false, "put all text in resource file")
	suppressUnref   = flag.Bool("u", false, "suppress unreferenced code")
	annotate        = flag.Bool("n", false, "annotate with offsets")
	noCodes         = flag.Bool("r", false, "don't generate control codes")
	debugInfo       = flag.Bool("g", false, "read debug information")
	target          = flag.String("t", "", "target: RealLive, AVG2000, AVG32, Kinetic")
	targetVersion   = flag.String("f", "", "interpreter version (n.n.n.n) or filename")
	kfnFile         = flag.String("kfn", "", "RealLive function definition file (default: reallive.kfn)")
	castFile        = flag.String("cast", "", "cast of characters translation file")
	decKey          = flag.String("y", "", "decoder key for compiler version 110002")
	srcExt          = flag.String("ext", "org", "source file extension")
	showOpcodes     = flag.Bool("opcodes", false, "show opcode annotations")
	hexDump         = flag.Bool("hexdump", false, "generate hex dump")
	rawStrings      = flag.Bool("raw-strings", false, "no special markup in strings")
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

	// Wire the diag reporter to kprl's verbosity flag. kprl doesn't
	// expose -q / -Wfatal yet — the disassembler's diagnostics are
	// always informational, not "fail this run", so quiet/fatal
	// don't carry the same meaning as for rlc. Verbose level >0
	// turns on Phase logging exactly like in rlc.
	diag.SetVerbose(*verbose > 0)

	// Resolve game keys
	opts := kprl.Options{
		Verbose:         *verbose,
		OutDir:          *outdir,
		GameID:          *gameID,
		TemplateArchive: *archiveTemplate,
		ForceTransform:  *forceTransform,
	}

	if *outputTransform != "" {
		mode, err := texttransforms.ParseMode(*outputTransform)
		if err != nil {
			fatal("%v", err)
		}
		opts.TextTransform = mode
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
		if isAVG32AssembleRequest(args) {
			err = doAssembleAVG32(args, opts)
		} else {
			err = kprl.Pack(args, opts)
		}

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

	fmt.Printf("Archive: %s (%s, %d entries)\n\n", filepath.Base(fname), arc.Format, arc.Count)
	fmt.Printf("%-16s %8s %10s %10s %7s\n", "File", "Index", "Offset", "Length", "Ratio")
	fmt.Println(strings.Repeat("-", 55))

	for _, i := range archiveInfoIndices(arc) {
		entry := arc.Entries[i]
		if entry.Length == 0 {
			continue
		}

		sub := arc.Subfile(i)
		if sub == nil {
			continue
		}

		name := arc.EntryName(i)
		if arc.Format == kprl.ArchiveFormatAVG32 {
			unpacked := int(sub.GetInt(0x08))
			ratio := float64(entry.Length) / float64(unpacked) * 100.0
			fmt.Printf("%-16s %8d %10d %10d %6.1f%%\n", name, i, entry.Offset, entry.Length, ratio)
			continue
		}

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

func archiveInfoIndices(arc *kprl.Archive) []int {
	if arc.Format == kprl.ArchiveFormatAVG32 && len(arc.Order) > 0 {
		return arc.Order
	}
	indices := make([]int, 0, arc.Count)
	for i := 0; i < kprl.MaxSeens; i++ {
		if arc.Entries[i].Length > 0 {
			indices = append(indices, i)
		}
	}
	return indices
}

func isAVG32AssembleRequest(args []string) bool {
	switch strings.ToLower(*target) {
	case "avg32", "avg", "tpc32":
		return true
	}
	for _, arg := range args {
		if strings.EqualFold(filepath.Ext(arg), ".avg") {
			return true
		}
	}
	return false
}

func doAssembleAVG32(args []string, opts kprl.Options) error {
	if err := os.MkdirAll(opts.OutDir, 0755); err != nil {
		return fmt.Errorf("cannot create output directory: %w", err)
	}
	for _, fname := range args {
		data, err := avg32.AssembleFileWithOptions(fname, avg32.Options{
			TextTransform:  opts.TextTransform,
			ForceTransform: opts.ForceTransform,
		})
		if err != nil {
			return fmt.Errorf("%s: %w", fname, err)
		}
		base := filepath.Base(fname)
		base = strings.TrimSuffix(base, filepath.Ext(base))
		outPath := filepath.Join(opts.OutDir, base+".TXT")
		if opts.Verbose > 0 {
			fmt.Printf("Assembling %s to %s\n", filepath.Base(fname), filepath.Base(outPath))
		}
		if err := os.WriteFile(outPath, data, 0644); err != nil {
			return fmt.Errorf("cannot write %s: %w", outPath, err)
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

	if *outputTransform != "" {
		disOpts.TextTransform = opts.TextTransform
		disOpts.TextTransformSet = true
	}

	if *target != "" {
		switch strings.ToLower(*target) {
		case "reallive", "2":
			disOpts.ForcedTarget = disasm.ModeRealLive
		case "avg2000", "avg2k", "1":
			disOpts.ForcedTarget = disasm.ModeAvg2000
		case "avg32", "avg", "tpc32":
			disOpts.ForcedTarget = disasm.ModeAVG32
		case "kinetic", "3":
			disOpts.ForcedTarget = disasm.ModeKinetic
		default:
			return fmt.Errorf("unknown target: %s", *target)
		}
	}
	firstFile := args[0]
	disOpts.Version = disasmVersionForTarget(disOpts.ForcedTarget, *targetVersion)
	if rawVersion := strings.TrimSpace(*targetVersion); rawVersion != "" {
		if _, ok := parseDisasmVersion(rawVersion); ok {
			// Already handled by disasmVersionForTarget above.
		} else if v, exePath, err := detectDisasmVersionFromPath(rawVersion); err == nil {
			disOpts.Version = v
			diag.Phase("interpreter %s version %s", filepath.Base(exePath), v.String())
		} else {
			diag.SysWarning("cannot read interpreter version from %s: %v", rawVersion, err)
		}
	} else if shouldAutoDetectDisasmVersion(disOpts.ForcedTarget) {
		if v, exePath, err := autoDetectDisasmVersion(firstFile); err == nil {
			disOpts.Version = v
			diag.Phase("detected interpreter %s version %s", filepath.Base(exePath), v.String())
		}
	}

	// Load KFN function definitions
	kfnPath := *kfnFile
	if kfnPath == "" {
		// Auto-detect: search near executable, in lib/, etc.
		kfnName := "reallive.kfn"
		if disOpts.ForcedTarget == disasm.ModeAVG32 {
			kfnName = "avg32.kfn"
		}
		candidates := findKFN(kfnName)
		if candidates != "" {
			kfnPath = candidates
		}
	}
	if kfnPath != "" {
		reg, err := disasm.LoadKFNForTarget(kfnPath, kfnTargetMode(disOpts.ForcedTarget), disOpts.Version)
		if err != nil {
			// Always reported: a KFN load failure changes every
			// opcode in the output (raw op<…> form, no overload
			// filtering). Was silenced without -v.
			diag.SysWarning("cannot load KFN %s: %v", kfnPath, err)
		} else {
			disOpts.FuncReg = reg
			diag.Phase("loaded KFN: %s (%d functions)", kfnPath, len(reg.AllNames()))
		}
	}

	writer := disasm.NewWriter(opts.OutDir, disOpts)

	// Check if first file is an archive
	explicitKFN := *kfnFile != ""
	if kprl.IsArchive(firstFile) {
		return disassembleArchive(firstFile, args[1:], opts, disOpts, writer, explicitKFN)
	}

	// Process individual files
	for _, fname := range args {
		if err := disassembleFile(fname, opts, disOpts, writer, explicitKFN); err != nil {
			diag.SysWarning("%s: %v", fname, err)
		}
	}
	return nil
}

func kfnTargetMode(mode disasm.EngineMode) disasm.EngineMode {
	if mode == disasm.ModeNone {
		return disasm.ModeRealLive
	}
	return mode
}

func disasmVersionForTarget(mode disasm.EngineMode, raw string) disasm.Version {
	if v, ok := parseDisasmVersion(raw); ok {
		return v
	}
	if mode == disasm.ModeAvg2000 || mode == disasm.ModeAVG32 {
		return disasm.Version{1, 0, 0, 0}
	}
	return disasm.Version{1, 2, 7, 0}
}

func parseDisasmVersion(raw string) (disasm.Version, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return disasm.Version{}, false
	}
	parts := strings.Split(raw, ".")
	if len(parts) < 2 || len(parts) > 4 {
		return disasm.Version{}, false
	}
	var v disasm.Version
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 0 {
			return disasm.Version{}, false
		}
		v[i] = n
	}
	return v, true
}

func shouldAutoDetectDisasmVersion(mode disasm.EngineMode) bool {
	return mode == disasm.ModeNone || mode == disasm.ModeRealLive || mode == disasm.ModeKinetic
}

func detectDisasmVersionFromPath(path string) (disasm.Version, string, error) {
	info, statErr := os.Stat(path)
	if statErr == nil {
		if info.IsDir() {
			return autoDetectDisasmVersionInDirs([]string{path, filepath.Join(path, "..")})
		}
		if strings.EqualFold(filepath.Ext(path), ".exe") {
			v, err := peVersionFromExe(path)
			return v, path, err
		}
	}
	return autoDetectDisasmVersion(path)
}

func autoDetectDisasmVersion(srcPath string) (disasm.Version, string, error) {
	return autoDetectDisasmVersionInDirs([]string{
		filepath.Dir(srcPath),
		filepath.Join(filepath.Dir(srcPath), ".."),
	})
}

func autoDetectDisasmVersionInDirs(dirs []string) (disasm.Version, string, error) {
	candidates := []string{
		"RealLive.exe", "RealLiveEn.exe", "Kinetic.exe", "kinetic.exe",
		"AVG2000.exe", "avg2000.exe", "SiglusEngine.exe", "siglusengine.exe",
		"SiglusEngine_Steam.exe", "siglusengine_steam.exe", "reallive.exe",
	}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		for _, name := range candidates {
			path := filepath.Join(dir, name)
			if _, err := os.Stat(path); err != nil {
				continue
			}
			v, err := peVersionFromExe(path)
			if err == nil && v != (disasm.Version{}) {
				return v, path, nil
			}
		}
	}
	return disasm.Version{}, "", fmt.Errorf("no interpreter executable found")
}

func peVersionFromExe(path string) (disasm.Version, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return disasm.Version{}, err
	}
	if len(data) < 0x40 || data[0] != 'M' || data[1] != 'Z' {
		return disasm.Version{}, fmt.Errorf("not a PE file")
	}
	peOff := int(uint32(data[0x3c]) | uint32(data[0x3d])<<8 | uint32(data[0x3e])<<16 | uint32(data[0x3f])<<24)
	if peOff+4 > len(data) || string(data[peOff:peOff+4]) != "PE\x00\x00" {
		return disasm.Version{}, fmt.Errorf("PE header signature not found")
	}

	coffOff := peOff + 4
	if coffOff+20 > len(data) {
		return disasm.Version{}, fmt.Errorf("truncated COFF header")
	}
	nsec := int(uint16(data[coffOff+2]) | uint16(data[coffOff+3])<<8)
	optSize := int(uint16(data[coffOff+16]) | uint16(data[coffOff+17])<<8)
	secStart := coffOff + 20 + optSize

	rsrcOff := 0
	rsrcSize := 0
	for i := 0; i < nsec; i++ {
		s := secStart + i*40
		if s+40 > len(data) {
			break
		}
		name := strings.TrimRight(string(data[s:s+8]), "\x00")
		if name != ".rsrc" {
			continue
		}
		rsrcSize = int(uint32(data[s+16]) | uint32(data[s+17])<<8 | uint32(data[s+18])<<16 | uint32(data[s+19])<<24)
		rsrcOff = int(uint32(data[s+20]) | uint32(data[s+21])<<8 | uint32(data[s+22])<<16 | uint32(data[s+23])<<24)
		break
	}
	if rsrcOff == 0 {
		return disasm.Version{}, fmt.Errorf(".rsrc section not found")
	}
	end := rsrcOff + rsrcSize
	if end > len(data) {
		end = len(data)
	}

	idx := findVSFixedFileInfo(data, rsrcOff, end)
	if idx < 0 {
		idx = findVSFixedFileInfo(data, 0, len(data))
	}
	if idx < 0 {
		if v, err := peVersionFromStringFileInfo(data); err == nil {
			return v, nil
		}
		return disasm.Version{}, fmt.Errorf("VS_FIXEDFILEINFO not found")
	}
	if idx+16 > len(data) {
		return disasm.Version{}, fmt.Errorf("truncated VS_FIXEDFILEINFO")
	}
	fvms := uint32(data[idx+8]) | uint32(data[idx+9])<<8 | uint32(data[idx+10])<<16 | uint32(data[idx+11])<<24
	fvls := uint32(data[idx+12]) | uint32(data[idx+13])<<8 | uint32(data[idx+14])<<16 | uint32(data[idx+15])<<24
	return disasm.Version{
		int(fvms >> 16),
		int(fvms & 0xffff),
		int(fvls >> 16),
		int(fvls & 0xffff),
	}, nil
}

func findVSFixedFileInfo(data []byte, start, end int) int {
	if start < 0 {
		start = 0
	}
	if end > len(data) {
		end = len(data)
	}
	if start >= end {
		return -1
	}
	sig := []byte{0xbd, 0x04, 0xef, 0xfe}
	for i := start; i+16 < end; i++ {
		if data[i] == sig[0] && data[i+1] == sig[1] && data[i+2] == sig[2] && data[i+3] == sig[3] {
			return i
		}
	}
	return -1
}

func peVersionFromStringFileInfo(data []byte) (disasm.Version, error) {
	key := utf16LEBytes("FileVersion")
	for i := 0; i+len(key) < len(data); i++ {
		if !equalBytes(data[i:i+len(key)], key) {
			continue
		}
		searchEnd := i + len(key) + 256
		if searchEnd > len(data) {
			searchEnd = len(data)
		}
		for j := i + len(key); j+2 <= searchEnd; j += 2 {
			s := readUTF16LEString(data[j:searchEnd], 64)
			if v, ok := parseFileVersionString(s); ok {
				return v, nil
			}
		}
	}
	return disasm.Version{}, fmt.Errorf("FileVersion string not found")
}

func utf16LEBytes(s string) []byte {
	words := utf16.Encode([]rune(s))
	out := make([]byte, 0, len(words)*2)
	for _, w := range words {
		out = append(out, byte(w), byte(w>>8))
	}
	return out
}

func readUTF16LEString(data []byte, maxRunes int) string {
	words := make([]uint16, 0, maxRunes)
	for i := 0; i+1 < len(data) && len(words) < maxRunes; i += 2 {
		w := uint16(data[i]) | uint16(data[i+1])<<8
		if w == 0 {
			break
		}
		words = append(words, w)
	}
	return string(utf16.Decode(words))
}

func parseFileVersionString(s string) (disasm.Version, bool) {
	if !strings.ContainsAny(s, ".,") {
		return disasm.Version{}, false
	}
	parts := make([]int, 0, 4)
	current := -1
	flush := func() bool {
		if current < 0 {
			return true
		}
		if current > 9999 {
			return false
		}
		parts = append(parts, current)
		current = -1
		return len(parts) <= 4
	}
	for _, r := range s {
		if r >= '0' && r <= '9' {
			if current < 0 {
				current = 0
			}
			current = current*10 + int(r-'0')
			continue
		}
		if !flush() {
			return disasm.Version{}, false
		}
	}
	if !flush() || len(parts) < 2 || len(parts) > 4 || parts[0] > 20 {
		return disasm.Version{}, false
	}
	var v disasm.Version
	for i, p := range parts {
		v[i] = p
	}
	return v, true
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func disassembleArchive(arcName string, rangeArgs []string, opts kprl.Options, disOpts disasm.Options, writer *disasm.Writer, explicitKFN bool) error {
	arc, err := kprl.LoadArchive(arcName)
	if err != nil {
		return err
	}
	if arc.Format == kprl.ArchiveFormatAVG32 {
		if err := ensureAVG32KFN(&disOpts, explicitKFN); err != nil {
			diag.SysWarning("cannot load AVG32 KFN: %v", err)
		}
		return disassembleAVG32Archive(arc, rangeArgs, opts, disOpts)
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
		sub := arc.Subfile(i)
		if sub == nil {
			continue
		}

		seenName := fmt.Sprintf("SEEN%04d.TXT", i)
		diag.Phase("disassembling %s", seenName)
		// Propagate the SEEN name into the disassembler so the
		// reader's diagnostics (arg-count mismatches, stream
		// desyncs) can name the offending file in their message.
		disOpts.SourceFile = seenName

		// Decompress if needed
		data := binarray.Copy(sub)
		if data.Len() >= 4 && !bytecode.UncompressedHeader(data.Read(0, 4)) {
			decompressed, err := rlcmp.Decompress(data, opts.Keys, true)
			if err != nil {
				diag.SysWarning("%s: failed to decompress: %v", seenName, err)
				continue
			}
			data = decompressed
		}

		result, err := disasm.Disassemble(data, disOpts)
		if err != nil {
			diag.SysWarning("%s: failed to disassemble: %v", seenName, err)
			continue
		}

		if result.Error != "" {
			// result.Error is set by the disassembly main loop on
			// stream desync. The diag.SysWarning was already emitted
			// from inside Disassemble with the precise offset, so
			// here we only need to bubble up a per-file marker if
			// the verbose user wanted a follow-up trace.
			diag.Phase("%s: %s", seenName, result.Error)
		}

		if err := writer.WriteSource(seenName, result); err != nil {
			diag.SysWarning("%s: failed to write: %v", seenName, err)
		}
	}

	return nil
}

func disassembleAVG32Archive(arc *kprl.Archive, rangeArgs []string, opts kprl.Options, disOpts disasm.Options) error {
	var ranges []int
	var err error
	if len(rangeArgs) > 0 {
		ranges, err = kprl.ParseRanges(rangeArgs)
		if err != nil {
			return err
		}
	} else {
		ranges = arc.Order
	}

	avgOpts := avg32.Options{
		OutDir:          opts.OutDir,
		SrcExt:          disOpts.SrcExt,
		FuncReg:         disOpts.FuncReg,
		Annotate:        disOpts.Annotate,
		SeparateStrings: disOpts.SeparateStrings,
		TextTransform:   disOpts.TextTransform,
		ForceTransform:  opts.ForceTransform,
	}

	for _, i := range ranges {
		sub := arc.Subfile(i)
		if sub == nil {
			continue
		}
		seenName := arc.EntryName(i)
		diag.Phase("disassembling %s (AVG32)", seenName)
		decompressed, err := kprl.DecompressAVG32SubfileForDisasm(sub)
		if err != nil {
			diag.SysWarning("%s: failed to decompress AVG32 PACK: %v", seenName, err)
			continue
		}
		result, err := avg32.Disassemble(decompressed, avgOpts)
		if err != nil {
			diag.SysWarning("%s: failed to disassemble AVG32 scene: %v", seenName, err)
			continue
		}
		if err := avg32.WriteSource(seenName, result, avgOpts); err != nil {
			diag.SysWarning("%s: failed to write AVG32 source: %v", seenName, err)
		}
	}
	return nil
}

func disassembleFile(fname string, opts kprl.Options, disOpts disasm.Options, writer *disasm.Writer, explicitKFN bool) error {
	arr, err := binarray.ReadFile(fname)
	if err != nil {
		return fmt.Errorf("cannot read '%s': %w", fname, err)
	}

	if disOpts.ForcedTarget == disasm.ModeAVG32 || (arr.Len() >= 5 && arr.Read(0, 5) == "TPC32") {
		if err := ensureAVG32KFN(&disOpts, explicitKFN); err != nil {
			diag.SysWarning("cannot load AVG32 KFN: %v", err)
		}
		avgOpts := avg32.Options{
			OutDir:          opts.OutDir,
			SrcExt:          disOpts.SrcExt,
			FuncReg:         disOpts.FuncReg,
			Annotate:        disOpts.Annotate,
			SeparateStrings: disOpts.SeparateStrings,
			TextTransform:   disOpts.TextTransform,
			ForceTransform:  opts.ForceTransform,
		}
		result, err := avg32.Disassemble(arr.Data, avgOpts)
		if err != nil {
			return fmt.Errorf("failed to disassemble AVG32 scene '%s': %w", fname, err)
		}
		return avg32.WriteSource(filepath.Base(fname), result, avgOpts)
	}

	// Decompress if needed (for standalone files, also accept raw RealLive headers)
	if arr.Len() >= 4 && !bytecode.UncompressedHeader(arr.Read(0, 4)) && !bytecode.IsRawRealLive(arr.Read(0, 4)) {
		decompressed, err := rlcmp.Decompress(arr, opts.Keys, true)
		if err != nil {
			return fmt.Errorf("failed to decompress '%s': %w", fname, err)
		}
		arr = decompressed
	}

	// Propagate the file name into the disassembler so the reader's
	// offset-based diagnostics can identify the source. Mutate a
	// local copy so the caller's struct stays clean across files.
	disOpts.SourceFile = filepath.Base(fname)

	result, err := disasm.Disassemble(arr, disOpts)
	if err != nil {
		return fmt.Errorf("failed to disassemble '%s': %w", fname, err)
	}

	baseName := filepath.Base(fname)
	return writer.WriteSource(baseName, result)
}

func ensureAVG32KFN(disOpts *disasm.Options, explicit bool) error {
	if explicit {
		return nil
	}
	kfnPath := findKFN("avg32.kfn")
	if kfnPath == "" {
		return fmt.Errorf("avg32.kfn not found")
	}
	reg, err := disasm.LoadKFN(kfnPath)
	if err != nil {
		return err
	}
	disOpts.FuncReg = reg
	diag.Phase("loaded KFN: %s (%d functions)", kfnPath, len(reg.AllNames()))
	return nil
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

// findKFN searches for a KFN file in common locations.
func findKFN(name string) string {
	execPath, _ := os.Executable()
	execDir := filepath.Dir(execPath)
	home, _ := os.UserHomeDir()
	rldev := os.Getenv("RLDEV")
	wd, _ := os.Getwd()

	candidates := []string{
		filepath.Join(execDir, name),
		filepath.Join(execDir, "lib", name),
	}
	if rldev != "" {
		candidates = append([]string{
			filepath.Join(rldev, "lib", name),
			filepath.Join(rldev, name),
		}, candidates...)
	}
	if wd != "" {
		candidates = append([]string{
			filepath.Join(wd, name),
			filepath.Join(wd, "KFN", name),
			filepath.Join(wd, "..", "KFN", name),
		}, candidates...)
	}
	if home != "" {
		candidates = append(candidates,
			filepath.Join(home, "rldev", "lib", name),
			filepath.Join(home, ".rldev", "lib", name),
		)
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}
