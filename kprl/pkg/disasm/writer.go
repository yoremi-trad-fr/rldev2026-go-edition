package disasm

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/yoremi/rldev-go/pkg/encoding"
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

// convertText converts raw Shift-JIS bytes to the target encoding.
func (w *Writer) convertText(sjisText string) string {
	enc := strings.ToUpper(w.opts.Encoding)
	if enc == "" || enc == "CP932" || enc == "SHIFT-JIS" || enc == "SJIS" || enc == "SHIFT_JIS" || enc == "SHIFTJIS" {
		return sjisText
	}
	utf8Str, err := encoding.SJSToUTF8([]byte(sjisText))
	if err != nil {
		return sjisText
	}
	return utf8Str
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

	// Write source header. OCaml uses lowercase no-dash like "cp utf8".
	enc := w.opts.Encoding
	if enc == "" {
		enc = "cp932"
	}
	encNorm := strings.ToLower(strings.ReplaceAll(enc, "-", ""))
	if encNorm == "shiftjis" || encNorm == "shift_jis" || encNorm == "sjis" {
		encNorm = "cp932"
	}
	fmt.Fprintf(srcFile, "{-# cp %s #- Disassembled with rldev-go -}\n\n#file '%s'\n",
		encNorm, baseName)

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
		fmt.Fprintf(resFile, "#character '%s'\n", w.convertText(name))
	}
	if resFile != srcFile && len(result.Header.DramatisPersonae) > 0 {
		fmt.Fprintln(resFile)
	}

	// Write commands. OCaml emits commands flush-left (no leading
	// indentation); labels are on their own lines indented by two spaces.
	skipping := false
	for _, cmd := range result.Commands {
		// Print label if this offset is a pointer target.
		// OCaml indents labels with two spaces (matches the "@1" style
		// in kepago source files).
		if idx, ok := labels[cmd.Offset]; ok {
			fmt.Fprintf(srcFile, "\n  @%d\n", idx)
			skipping = false
		}

		// Print command if visible
		if cmd.Unhide && skipping {
			skipping = false
		}
		if !skipping && !cmd.Hidden {
			line := formatCommand(cmd, labels, w.opts, result)
			if line != "" {
				fmt.Fprint(srcFile, line+"\n")
			}
		}
		if w.opts.SuppressUncalled && cmd.IsJmp {
			skipping = true
		}
	}

	// Write resource strings.
	// OCaml format: each resource on its own line, prefixed with <NNNN>.
	if w.opts.SeparateStrings {
		for i, s := range result.ResStrs {
			converted := w.convertText(s)
			fmt.Fprintf(resFile, "<%04d> %s\n", i, converted)
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
//
// In addition to the simple element kinds, this resolver post-processes
// pointer references inside ElemString text:
//   - "@@PTR=NNNN@@"     → "@<labelN>" (sequential label number)
//   - "@@RES=NNNN@@"     → "#res<NNNN>" (resource reference)
//
// These tokens are emitted by the reader where it detects pointer or
// resource targets but doesn't yet know the final mapping.
func formatCommand(cmd Command, labels map[int]int, opts Options, result *DisassemblyResult) string {
	var sb strings.Builder

	for _, elem := range cmd.Kepago {
		switch v := elem.(type) {
		case ElemString:
			// Skip #line directives if requested
			if !opts.ReadDebugSymbols && strings.HasPrefix(v.Value, "#line ") {
				continue
			}
			sb.WriteString(resolvePointers(v.Value, labels))
		case ElemStore:
			sb.WriteString(v.Value)
		case ElemPointer:
			if idx, ok := labels[v.Offset]; ok {
				sb.WriteString(fmt.Sprintf("@%d", idx))
			} else {
				sb.WriteString(fmt.Sprintf("@unknown_%d", v.Offset))
			}
		case ElemText:
			// Convert SJIS text if output encoding is UTF-8
			enc := strings.ToUpper(opts.Encoding)
			if enc != "" && enc != "CP932" && enc != "SHIFT-JIS" && enc != "SJIS" {
				if utf8Str, err := encoding.SJSToUTF8([]byte(v.Value)); err == nil {
					sb.WriteString(utf8Str)
				} else {
					sb.WriteString(v.Value)
				}
			} else {
				sb.WriteString(v.Value)
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

// resolvePointers replaces inline pointer markers "@@PTR=NNNN@@" with the
// corresponding label number from the labels map. Markers that don't
// resolve fall back to the literal pointer offset.
//
// The reader emits these markers because at construction time we don't
// know the label assignment (which depends on all pointers being seen).
// The writer fixes them up here after buildLabelMap has run.
func resolvePointers(s string, labels map[int]int) string {
	const ptrPrefix = "@@PTR="
	const ptrSuffix = "@@"
	if !strings.Contains(s, ptrPrefix) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		j := strings.Index(s[i:], ptrPrefix)
		if j < 0 {
			b.WriteString(s[i:])
			break
		}
		b.WriteString(s[i : i+j])
		i += j + len(ptrPrefix)
		k := strings.Index(s[i:], ptrSuffix)
		if k < 0 {
			b.WriteString(ptrPrefix)
			break
		}
		num, err := strconv.Atoi(s[i : i+k])
		if err != nil {
			b.WriteString(ptrPrefix)
			b.WriteString(s[i : i+k])
			b.WriteString(ptrSuffix)
		} else if idx, ok := labels[num]; ok {
			fmt.Fprintf(&b, "@%d", idx)
		} else {
			fmt.Fprintf(&b, "@unknown_%d", num)
		}
		i += k + len(ptrSuffix)
	}
	return b.String()
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
