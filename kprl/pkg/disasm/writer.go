package disasm

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Writer outputs disassembly results to files.
type Writer struct {
	opts   Options
	outDir string
}

// NewWriter creates a new output writer.
func NewWriter(outDir string, opts Options) *Writer {
	return &Writer{
		opts:   opts,
		outDir: outDir,
	}
}

// WriteSource writes the disassembled source and resource files.
// Produces:
//   - {base}.org   (kepago source code)
//   - {base}.sjs   (resource strings, if separate)
func (w *Writer) WriteSource(baseName string, result *DisassemblyResult) error {
	if err := os.MkdirAll(w.outDir, 0755); err != nil {
		return fmt.Errorf("cannot create output directory: %w", err)
	}

	// Strip extensions to get base name
	base := baseName
	for _, ext := range []string{".uncompressed", ".rl", ".rlc", ".TXT"} {
		base = strings.TrimSuffix(base, ext)
	}

	srcExt := w.opts.SrcExt
	if srcExt == "" {
		srcExt = "org"
	}

	srcName := filepath.Join(w.outDir, base+"."+srcExt)
	resExt := encodingToExt(w.opts.Encoding)
	resName := filepath.Join(w.outDir, base+"."+resExt)

	// Build label map from pointers
	labels := buildLabelMap(result.Pointers)

	// Write source file
	srcFile, err := os.Create(srcName)
	if err != nil {
		return fmt.Errorf("cannot create source file: %w", err)
	}
	defer srcFile.Close()

	// Write resource file (or same as source if not separating)
	var resFile *os.File
	if w.opts.SeparateStrings && len(result.ResStrs) > 0 {
		resFile, err = os.Create(resName)
		if err != nil {
			return fmt.Errorf("cannot create resource file: %w", err)
		}
		defer resFile.Close()
	} else {
		resFile = srcFile
	}

	// Write BOM if requested
	if w.opts.BOM && strings.EqualFold(w.opts.Encoding, "UTF8") {
		srcFile.Write([]byte{0xef, 0xbb, 0xbf})
		if resFile != srcFile {
			resFile.Write([]byte{0xef, 0xbb, 0xbf})
		}
	}

	// Write source header
	enc := w.opts.Encoding
	if enc == "" {
		enc = "cp932"
	}
	fmt.Fprintf(srcFile, "{-# cp %s #- Disassembled with rldev-go -}\n\n#file '%s'\n",
		strings.ToLower(enc), baseName)

	if resFile != srcFile {
		fmt.Fprintf(resFile, "// Resources for %s\n\n", baseName)
		fmt.Fprintf(srcFile, "#resource '%s'\n", filepath.Base(resName))
	}
	fmt.Fprintln(srcFile)

	// Write target directive
	switch result.Mode {
	case ModeAvg2000:
		fmt.Fprintln(srcFile, "#target AVG2000")
	case ModeKinetic:
		fmt.Fprintln(srcFile, "#target Kinetic")
	}

	// Write dramatis personae
	for _, name := range result.Header.DramatisPersonae {
		fmt.Fprintf(resFile, "#character '%s'\n", name)
	}
	if resFile != srcFile && len(result.Header.DramatisPersonae) > 0 {
		fmt.Fprintln(resFile)
	}

	// Write commands
	skipping := false
	for _, cmd := range result.Commands {
		// Print label if this offset is a pointer target
		if idx, ok := labels[cmd.Offset]; ok {
			fmt.Fprintf(srcFile, "\n  @%d\n", idx)
			skipping = false
		}

		// Print command if visible
		if cmd.Unhide && skipping {
			skipping = false
		}
		if !skipping && !cmd.Hidden {
			line := formatCommand(cmd, labels, w.opts)
			if line != "" {
				fmt.Fprint(srcFile, "    "+line+"\n")
			}
		}
		if w.opts.SuppressUncalled && cmd.IsJmp {
			skipping = true
		}
	}

	// Write resource strings
	if w.opts.SeparateStrings {
		for i, s := range result.ResStrs {
			if w.opts.IDStrings {
				fmt.Fprintf(resFile, "<%04d> %s\n", i, s)
			} else {
				fmt.Fprintf(resFile, "%s\n", s)
			}
		}
	}

	return nil
}

// SourceInfo returns the count of text lines and total byte length
// of resource strings (for the info/stats mode).
func SourceInfo(result *DisassemblyResult) (lines int, bytes int) {
	for _, s := range result.ResStrs {
		if len(s) > 0 {
			lines++
			bytes += len(s)
		}
	}
	return
}

// buildLabelMap assigns sequential label numbers to pointer targets.
func buildLabelMap(pointers map[int]bool) map[int]int {
	if len(pointers) == 0 {
		return nil
	}

	// Sort pointer offsets
	sorted := make([]int, 0, len(pointers))
	for offset := range pointers {
		sorted = append(sorted, offset)
	}
	sort.Ints(sorted)

	labels := make(map[int]int, len(sorted))
	for i, offset := range sorted {
		labels[offset] = i + 1
	}
	return labels
}

// formatCommand renders a command as a string for output.
func formatCommand(cmd Command, labels map[int]int, opts Options) string {
	var sb strings.Builder

	for _, elem := range cmd.Kepago {
		switch v := elem.(type) {
		case ElemString:
			// Skip #line directives if they start with that
			if !opts.ReadDebugSymbols && strings.HasPrefix(v.Value, "#line ") {
				continue
			}
			sb.WriteString(v.Value)
		case ElemStore:
			sb.WriteString(v.Value)
		case ElemPointer:
			if idx, ok := labels[v.Offset]; ok {
				sb.WriteString(fmt.Sprintf("@%d", idx))
			} else {
				sb.WriteString(fmt.Sprintf("@unknown_%d", v.Offset))
			}
		}
	}

	text := sb.String()
	if text == "" {
		return ""
	}

	// Add annotations if requested
	if opts.Annotate {
		text = fmt.Sprintf("{-%08x-} %s", cmd.Offset, text)
	}

	if opts.ShowOpcodes && cmd.Opcode != "" {
		text += " // " + cmd.Opcode
	}

	return text
}

// encodingToExt returns the file extension for resource files.
func encodingToExt(enc string) string {
	switch strings.ToUpper(enc) {
	case "CP932", "SHIFTJIS", "SHIFT_JIS", "SJS":
		return "sjs"
	case "EUCJP", "EUC-JP", "EUC":
		return "euc"
	case "UTF8", "UTF-8":
		return "utf"
	default:
		return "res"
	}
}

// WriteHexDump writes a hex dump of the bytecode.
func (w *Writer) WriteHexDump(baseName string, data []byte, startOffset int) error {
	if err := os.MkdirAll(w.outDir, 0755); err != nil {
		return err
	}

	base := strings.TrimSuffix(baseName, filepath.Ext(baseName))
	hexName := filepath.Join(w.outDir, base+".hex")

	f, err := os.Create(hexName)
	if err != nil {
		return err
	}
	defer f.Close()

	for i := startOffset; i < len(data); i += 16 {
		// Offset
		fmt.Fprintf(f, "[%08x] ", i)

		// Hex bytes
		end := i + 16
		if end > len(data) {
			end = len(data)
		}
		for j := i; j < end; j++ {
			fmt.Fprintf(f, "%02x ", data[j])
		}
		// Padding
		for j := end; j < i+16; j++ {
			fmt.Fprint(f, "   ")
		}

		// ASCII
		fmt.Fprint(f, " |")
		for j := i; j < end; j++ {
			c := data[j]
			if c >= 0x20 && c <= 0x7e {
				f.Write([]byte{c})
			} else {
				f.Write([]byte{'.'})
			}
		}
		fmt.Fprintln(f, "|")
	}

	return nil
}
