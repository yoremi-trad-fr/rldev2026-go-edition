package disasm

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/yoremi/rldev-go/pkg/encoding"
	"github.com/yoremi/rldev-go/pkg/text"
	"github.com/yoremi/rldev-go/pkg/texttransforms"
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

// crlfWriter wraps an io.Writer and translates each unescaped LF (0x0a)
// into CRLF (0x0d 0x0a). Existing CRLF sequences are passed through. We
// emit CRLF in disassembled output to match OCaml kprl's historical
// format, which Windows-side tools (and some text editors) prefer.
type crlfWriter struct {
	w        io.Writer
	prevWasC bool // last byte written was 0x0d
}

func (c *crlfWriter) Write(p []byte) (int, error) {
	written := 0
	buf := make([]byte, 0, len(p)+len(p)/8)
	for _, b := range p {
		if b == '\n' && !c.prevWasC {
			buf = append(buf, '\r', '\n')
			c.prevWasC = false
		} else {
			buf = append(buf, b)
			c.prevWasC = (b == '\r')
		}
		written++
	}
	if _, err := c.w.Write(buf); err != nil {
		return 0, err
	}
	return written, nil
}

// convertText converts raw Shift-JIS bytes to the target encoding,
// after first translating RealLive name markers (\l{...} / \m{...}).
func (w *Writer) convertText(sjisText string, transform texttransforms.EncMode) string {
	// Decode RealLive name markers (0x81 0x93 / 0x96 + 0x82 NN)
	// before any encoding conversion. Otherwise SJIS-to-UTF8 sees the
	// marker bytes as fullwidth characters (e.g. 0x81 0x96 → ＊).
	sjisText = decodeNameMarkers(sjisText)
	sjisText = escapeUnsafeResourceBytes(sjisText)

	if isBytePreservingEncoding(w.opts.Encoding) {
		return sjisText
	}
	utf8Str, err := decodeMixedBytecodeText(sjisText, transform)
	if err != nil {
		return sjisText
	}
	return utf8Str
}

// convertHeaderText converts stored header strings without interpreting
// textout-only RealLive name markers. Character names in the SEEN header are
// literal display strings, so bytes like 81 96 82 61 must remain ＊Ｂ instead
// of becoming \m{B}.
func (w *Writer) convertHeaderText(sjisText string) string {
	if isBytePreservingEncoding(w.opts.Encoding) {
		return sjisText
	}
	utf8Str, err := encoding.SJSToUTF8([]byte(sjisText))
	if err != nil {
		return sjisText
	}
	return utf8Str
}

func isBytePreservingEncoding(enc string) bool {
	switch strings.ToUpper(enc) {
	case "", "CP932", "SHIFT-JIS", "SJIS", "SHIFT_JIS", "SHIFTJIS":
		return true
	default:
		return false
	}
}

func escapeUnsafeResourceBytes(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	changed := false
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case isPlainResourceByte(c):
			b.WriteByte(c)
			i++
		case isShiftJISLead(c):
			if i+1 < len(s) && isShiftJISTrail(s[i+1]) {
				b.WriteByte(c)
				b.WriteByte(s[i+1])
				i += 2
			} else {
				fmt.Fprintf(&b, "\\x{%02x}", c)
				changed = true
				i++
			}
		case c >= 0xa1 && c <= 0xdf:
			// Half-width katakana are valid single-byte Shift-JIS.
			b.WriteByte(c)
			i++
		default:
			fmt.Fprintf(&b, "\\x{%02x}", c)
			changed = true
			i++
		}
	}
	if !changed {
		return s
	}
	return b.String()
}

func isPlainResourceByte(b byte) bool {
	return b >= 0x20 && b < 0x7f
}

func isShiftJISTrail(b byte) bool {
	return (b >= 0x40 && b <= 0x7e) || (b >= 0x80 && b <= 0xfc)
}

func decodeBytecodeText(data []byte, transform texttransforms.EncMode) (string, error) {
	if transform == texttransforms.EncNone {
		return encoding.SJSToUTF8(data)
	}

	previous := texttransforms.GetMode()
	texttransforms.SetMode(transform)
	defer texttransforms.SetMode(previous)

	decoded, err := texttransforms.ReadBytecode(data)
	if err != nil {
		return "", err
	}
	return text.ToUTF8(decoded), nil
}

func decodeMixedBytecodeText(s string, transform texttransforms.EncMode) (string, error) {
	var out strings.Builder
	for len(s) > 0 {
		start := strings.Index(s, `\{`)
		if start < 0 {
			decoded, err := decodeBytecodeText([]byte(s), transform)
			if err != nil {
				return "", err
			}
			out.WriteString(decoded)
			break
		}
		if start > 0 {
			decoded, err := decodeBytecodeText([]byte(s[:start]), transform)
			if err != nil {
				return "", err
			}
			out.WriteString(decoded)
		}
		out.WriteString(`\{`)
		s = s[start+2:]
		end := findSpeakerCloseByte(s)
		if end < 0 {
			decoded, err := decodeBytecodeText([]byte(s), texttransforms.EncNone)
			if err != nil {
				return "", err
			}
			out.WriteString(decoded)
			break
		}
		name, err := decodeSpeakerNameText([]byte(s[:end]), transform)
		if err != nil {
			return "", err
		}
		out.WriteString(name)
		out.WriteByte('}')
		s = s[end+1:]
	}
	return out.String(), nil
}

func decodeSpeakerNameText(data []byte, transform texttransforms.EncMode) (string, error) {
	native, nativeErr := decodeBytecodeText(data, texttransforms.EncNone)
	if transform == texttransforms.EncNone {
		return native, nativeErr
	}

	transformed, transformedErr := decodeBytecodeText(data, transform)
	if transformedErr != nil {
		if nativeErr == nil {
			return native, nil
		}
		return "", transformedErr
	}
	if nativeErr != nil {
		return transformed, nil
	}
	if preferNativeSpeakerName(native, transformed) {
		return native, nil
	}
	return transformed, nil
}

func preferNativeSpeakerName(native, transformed string) bool {
	if native == transformed {
		return true
	}
	// WESTERN reserves 0x89 as an accent prefix. A native CP932 name that
	// happens to contain a 0x89 lead byte can otherwise turn into Latin
	// accent noise, so keep the native decode when it clearly produced
	// Japanese text and the transformed decode did not.
	return containsJapaneseScript(native) && !containsJapaneseScript(transformed)
}

func containsJapaneseScript(s string) bool {
	for _, r := range s {
		if (r >= 0x3040 && r <= 0x309f) || // Hiragana
			(r >= 0x30a0 && r <= 0x30ff) || // Katakana
			(r >= 0x3400 && r <= 0x9fff) { // CJK
			return true
		}
	}
	return false
}

func findSpeakerCloseByte(s string) int {
	for i := 0; i < len(s); {
		c := s[i]
		if c == '\\' && i+3 < len(s) && s[i+1] == 'x' && s[i+2] == '{' {
			if end := strings.IndexByte(s[i+3:], '}'); end >= 0 {
				i += 3 + end + 1
				continue
			}
		}
		if c == '}' {
			return i
		}
		if isShiftJISLead(c) && i+1 < len(s) && isShiftJISTrail(s[i+1]) {
			i += 2
			continue
		}
		i++
	}
	return -1
}

// decodeNameMarkers replaces each RealLive name-marker byte sequence in s
// by its kepago text form (\l{...} or \m{...}). Reference: kprl OCaml
// disassembler.ml L1534-1555.
//
//	81 93 82 60..79                    → \l{A..Z}
//	81 96 82 60..79                    → \m{A..Z}
//	81 96 82 60..79 82 60..79          → \m{AA..ZZ}
//	81 96 82 60..79 82 4f..58          → \m{A, 0..9}
//	81 96 82 60..79 82 60..79 82 4f..58 → \m{AA, 0..9}
//
// Other bytes are passed through verbatim.
func decodeNameMarkers(s string) string {
	if !strings.Contains(s, "\x81\x93") && !strings.Contains(s, "\x81\x96") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		// Need at least 4 bytes for a marker.
		if i+3 < len(s) && s[i] == 0x81 && (s[i+1] == 0x93 || s[i+1] == 0x96) &&
			s[i+2] == 0x82 && s[i+3] >= 0x60 && s[i+3] <= 0x79 {
			lm := byte('l')
			if s[i+1] == 0x96 {
				lm = 'm'
			}
			c1 := s[i+3] - 0x1f
			j := i + 4
			var c2 byte
			hasC2 := false
			if j+1 < len(s) && s[j] == 0x82 && s[j+1] >= 0x60 && s[j+1] <= 0x79 {
				c2 = s[j+1] - 0x1f
				hasC2 = true
				j += 2
			}
			hasIdx := false
			var idx int
			if j+1 < len(s) && s[j] == 0x82 && s[j+1] >= 0x4f && s[j+1] <= 0x58 {
				idx = int(s[j+1]) - 0x4f
				hasIdx = true
				j += 2
			}
			if hasIdx {
				if hasC2 {
					fmt.Fprintf(&b, "\\%c{%c%c, %d}", lm, c1, c2, idx)
				} else {
					fmt.Fprintf(&b, "\\%c{%c, %d}", lm, c1, idx)
				}
			} else {
				if hasC2 {
					fmt.Fprintf(&b, "\\%c{%c%c}", lm, c1, c2)
				} else {
					fmt.Fprintf(&b, "\\%c{%c}", lm, c1)
				}
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
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
	separateRes := w.opts.SeparateStrings && len(result.ResStrs) > 0
	if separateRes {
		resFile, err = os.Create(resName)
		if err != nil {
			return fmt.Errorf("cannot create resource file: %w", err)
		}
		defer resFile.Close()
	} else {
		resFile = srcFile
	}

	// Wrap output streams to translate LF → CRLF, matching OCaml kprl.
	var srcOut io.Writer = &crlfWriter{w: srcFile}
	var resOut io.Writer
	if separateRes {
		resOut = &crlfWriter{w: resFile}
	} else {
		resOut = srcOut
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
	fmt.Fprintf(srcOut, "{-# cp %s #- Disassembled with rldev-go -}\n\n#file '%s'\n",
		encNorm, baseName)

	if resFile != srcFile {
		fmt.Fprintf(resOut, "// Resources for %s\n\n", baseName)
		fmt.Fprintf(srcOut, "#resource '%s'\n", filepath.Base(resName))
	}
	fmt.Fprintln(srcOut)

	// Write target directive
	switch result.Mode {
	case ModeAvg2000:
		fmt.Fprintln(srcOut, "#target AVG2000")
	case ModeKinetic:
		fmt.Fprintln(srcOut, "#target Kinetic")
	}
	if result.Header.Int0x2C != 0 {
		fmt.Fprintf(srcOut, "#val_0x2c %d\n", result.Header.Int0x2C)
	}

	// Write dramatis personae
	for _, name := range result.Header.DramatisPersonae {
		fmt.Fprintf(resOut, "#character '%s'\n", w.convertHeaderText(name))
	}
	if resFile != srcFile && len(result.Header.DramatisPersonae) > 0 {
		fmt.Fprintln(resOut)
	}

	// Write commands. OCaml emits commands flush-left (no leading
	// indentation); labels are on their own lines indented by two spaces.
	skipping := false
	for _, cmd := range result.Commands {
		// Print label if this offset is a pointer target.
		// OCaml indents labels with two spaces (matches the "@1" style
		// in kepago source files).
		if idx, ok := labels[cmd.Offset]; ok {
			fmt.Fprintf(srcOut, "\n  @%d\n", idx)
			skipping = false
		}

		// Print command if visible
		if cmd.Unhide && skipping {
			skipping = false
		}
		if !skipping && !cmd.Hidden {
			line := formatCommand(cmd, labels, w.opts, result)
			if line != "" {
				fmt.Fprint(srcOut, line+"\n")
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
			converted := w.convertText(s, result.TextTransform)
			converted = escapeResourceLineText(converted)
			fmt.Fprintf(resOut, "<%04d> %s\n", i, converted)
		}
	}

	return nil
}

func escapeResourceLineText(s string) string {
	if s == "" {
		return s
	}
	if strings.HasPrefix(s, "//") {
		return `\` + s
	}
	switch []rune(s)[0] {
	case ' ', '\t', '<':
		return `\` + s
	}
	return s
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
			// ElemString may carry raw SJIS bytes inside ASCII-quoted
			// literals (e.g. strcmp(strS[0], 'XXX')) or other inline
			// arguments. When writing UTF-8 output we must transcode
			// these the same way ElemText is converted below, otherwise
			// stray SJIS bytes leak into the .org file and break the
			// downstream lexer.
			s := resolvePointers(v.Value, labels)
			if !isBytePreservingEncoding(opts.Encoding) {
				if utf8Str, err := decodeBytecodeText([]byte(s), result.TextTransform); err == nil {
					s = utf8Str
				}
			}
			sb.WriteString(s)
		case ElemStore:
			sb.WriteString(v.Value)
		case ElemPointer:
			if idx, ok := labels[v.Offset]; ok {
				sb.WriteString(fmt.Sprintf("@%d", idx))
			} else {
				sb.WriteString(fmt.Sprintf("@unknown_%d", v.Offset))
			}
		case ElemText:
			// Convert bytecode text if output encoding is UTF-8.
			if !isBytePreservingEncoding(opts.Encoding) {
				if utf8Str, err := decodeBytecodeText([]byte(v.Value), result.TextTransform); err == nil {
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
