package avg32

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/yoremi/rldev-go/pkg/text"
	"github.com/yoremi/rldev-go/pkg/texttransforms"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

type sourceLine struct {
	no   int
	text string
}

// Assemble rebuilds a TPC32 scene from the lossless #rawhex block emitted by the
// AVG32 disassembler, then applies supported semantic edits from the readable
// listing. Unsupported instruction bodies still roundtrip from raw bytes.
func Assemble(src []byte) ([]byte, error) {
	return AssembleWithOptions(src, Options{})
}

func AssembleWithOptions(src []byte, opts Options) ([]byte, error) {
	return assembleWithResources(src, nil, opts)
}

func AssembleFile(path string) ([]byte, error) {
	return AssembleFileWithOptions(path, Options{})
}

func AssembleFileWithOptions(path string, opts Options) ([]byte, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	resources, err := loadAVG32Resources(src, filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	return assembleWithResources(src, resources, opts)
}

func assembleWithResources(src []byte, resources map[string]string, opts Options) ([]byte, error) {
	raw, err := assembleRawHex(src)
	if err != nil {
		return nil, err
	}
	return applyTextPatches(raw, src, resources, opts)
}

func assembleRawHex(src []byte) ([]byte, error) {
	scanner := bufio.NewScanner(bytes.NewReader(src))
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	inRaw := false
	sawRaw := false
	var out []byte
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())
		lower := strings.ToLower(line)
		switch {
		case lower == "#rawhex begin":
			if inRaw {
				return nil, fmt.Errorf("nested #rawhex begin at line %d", lineNo)
			}
			inRaw = true
			sawRaw = true
			continue
		case lower == "#rawhex end":
			if !inRaw {
				return nil, fmt.Errorf("#rawhex end without begin at line %d", lineNo)
			}
			inRaw = false
			continue
		case !inRaw:
			continue
		}

		fields := rawHexFields(line)
		for _, field := range fields {
			b, err := hex.DecodeString(field)
			if err != nil {
				return nil, fmt.Errorf("invalid raw hex at line %d: %w", lineNo, err)
			}
			out = append(out, b...)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if inRaw {
		return nil, fmt.Errorf("unterminated #rawhex block")
	}
	if !sawRaw {
		return nil, fmt.Errorf("missing #rawhex block")
	}
	if len(out) < 5 || string(out[:5]) != "TPC32" {
		return nil, fmt.Errorf("raw AVG32 block is not a TPC32 scene")
	}
	return out, nil
}

func applyTextPatches(raw []byte, src []byte, resources map[string]string, opts Options) ([]byte, error) {
	lines := sourceInstructionLines(src)
	if len(lines) == 0 {
		return raw, nil
	}

	result, err := Disassemble(raw, opts)
	if err != nil {
		return nil, err
	}
	if len(lines) != len(result.Instructions) {
		return nil, fmt.Errorf("AVG32 source instruction count changed: got %d, want %d", len(lines), len(result.Instructions))
	}

	instRaw := make([][]byte, len(result.Instructions))
	changed := false
	for i, inst := range result.Instructions {
		instRaw[i] = append([]byte(nil), inst.Raw...)
		patched, ok, err := patchInstructionFromSource(inst, lines[i].text, resources, opts)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lines[i].no, err)
		}
		if ok && !bytes.Equal(patched, inst.Raw) {
			instRaw[i] = patched
			changed = true
		}
	}
	if !changed {
		return raw, nil
	}

	return rebuildTPC32WithPatchedInstructions(raw, result, instRaw)
}

func sourceInstructionLines(src []byte) []sourceLine {
	scanner := bufio.NewScanner(bytes.NewReader(src))
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	inRaw := false
	var lines []sourceLine
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())
		lower := strings.ToLower(line)
		switch {
		case lower == "#rawhex begin":
			inRaw = true
			continue
		case lower == "#rawhex end":
			inRaw = false
			continue
		case inRaw || line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "@"):
			continue
		case strings.HasPrefix(line, "//"):
			continue
		}
		lines = append(lines, sourceLine{no: lineNo, text: line})
	}
	return lines
}

func patchInstructionFromSource(inst Instruction, line string, resources map[string]string, opts Options) ([]byte, bool, error) {
	if inst.Code == 0xfe || inst.Code == 0xff {
		text, targetCode, legacyID, ok, err := parseTopLevelTextLine(line, resources)
		if err != nil || !ok || legacyID {
			return nil, ok && !legacyID, err
		}
		encoded, err := encodeTopLevelText(targetCode, text, opts)
		return encoded, true, err
	}
	if inst.Code == 0x58 && (inst.Subcode == 0x01 || inst.Subcode == 0x02) {
		patched, ok, err := patchChoiceInstruction(inst.Raw, line, resources, opts)
		return patched, ok, err
	}
	if inst.Code == 0x60 && inst.Subcode == 0x04 {
		patched, ok, err := patchSingleFormattedArgInstruction(inst.Raw, line, resources, opts, 2, "set_title")
		return patched, ok, err
	}
	return nil, false, nil
}

func parseTopLevelTextLine(line string, resources map[string]string) (text string, targetCode byte, legacyID bool, ok bool, err error) {
	open := strings.IndexByte(line, '(')
	close := strings.LastIndexByte(line, ')')
	if open < 0 || close < open {
		return "", 0, false, false, nil
	}
	name := strings.ToLower(strings.TrimSpace(line[:open]))
	switch name {
	case "text_hankaku", "op_fe":
		targetCode = 0xfe
	case "text_zenkaku", "op_ff":
		targetCode = 0xff
	default:
		return "", 0, false, false, nil
	}
	args := strings.TrimSpace(line[open+1 : close])
	if strings.HasPrefix(strings.ToLower(args), "#res<") {
		text, err = resolveResourceRef(args, resources)
		return text, targetCode, false, true, err
	}
	qStart := strings.IndexByte(args, '"')
	if qStart < 0 {
		return "", targetCode, false, true, fmt.Errorf("missing quoted text")
	}
	prefix := strings.TrimSpace(strings.TrimSuffix(args[:qStart], ","))
	if strings.HasPrefix(strings.ToLower(prefix), "id:") {
		legacyID = true
	}
	quoted, _, err := readQuotedLiteral(args[qStart:])
	if err != nil {
		return "", targetCode, false, true, err
	}
	text, err = strconv.Unquote(quoted)
	if err != nil {
		return "", targetCode, false, true, err
	}
	return text, targetCode, legacyID, true, nil
}

func patchSingleFormattedArgInstruction(raw []byte, line string, resources map[string]string, opts Options, prefixLen int, names ...string) ([]byte, bool, error) {
	name, _, ok := splitCall(line)
	if !ok || !stringInSet(strings.ToLower(name), names) {
		return nil, false, nil
	}
	blocks, err := extractFormattedBlocks(line)
	if err != nil {
		return nil, true, err
	}
	if len(blocks) != 1 {
		return nil, true, fmt.Errorf("%s expects one formatted text block, got %d", name, len(blocks))
	}
	if len(raw) < prefixLen {
		return nil, true, fmt.Errorf("%s raw instruction is too short", name)
	}
	patched, err := patchFormattedTextBytes(raw[prefixLen:], blocks[0], resources, opts)
	if err != nil {
		return nil, true, err
	}
	out := append([]byte(nil), raw[:prefixLen]...)
	out = append(out, patched...)
	return out, true, nil
}

func patchChoiceInstruction(raw []byte, line string, resources map[string]string, opts Options) ([]byte, bool, error) {
	name, _, ok := splitCall(line)
	if !ok || !stringInSet(strings.ToLower(name), []string{"choice", "choice2"}) {
		return nil, false, nil
	}
	if len(raw) < 5 || raw[0] != 0x58 {
		return nil, true, fmt.Errorf("choice raw instruction is too short")
	}
	pos := 2
	valLen := valueLen(raw[pos])
	if valLen == 0 || pos+valLen >= len(raw) {
		return nil, true, fmt.Errorf("choice selector value is invalid")
	}
	pos += valLen
	flag := raw[pos]
	pos++
	if flag != 0x22 {
		return append([]byte(nil), raw...), true, nil
	}
	if pos >= len(raw) {
		return nil, true, fmt.Errorf("choice padding is missing")
	}
	pos++

	blocks, err := extractFormattedBlocks(line)
	if err != nil {
		return nil, true, err
	}

	out := append([]byte(nil), raw[:pos]...)
	for _, block := range blocks {
		end, err := formattedTextEnd(raw[pos:])
		if err != nil {
			return nil, true, err
		}
		patched, err := patchFormattedTextBytes(raw[pos:pos+end], block, resources, opts)
		if err != nil {
			return nil, true, err
		}
		out = append(out, patched...)
		pos += end
	}
	if pos >= len(raw) || raw[pos] != 0x23 {
		return nil, true, fmt.Errorf("choice text count changed or terminator missing")
	}
	out = append(out, raw[pos:]...)
	return out, true, nil
}

func readQuotedLiteral(s string) (literal string, consumed int, err error) {
	if s == "" || s[0] != '"' {
		return "", 0, fmt.Errorf("missing quoted text")
	}
	escaped := false
	for i := 1; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == '"' {
			return s[:i+1], i + 1, nil
		}
	}
	return "", 0, fmt.Errorf("unterminated quoted text")
}

func encodeTopLevelText(code byte, text string, opts Options) ([]byte, error) {
	code = avg32WesternTextTag(text, opts, code)
	encoded, err := encodeAVG32Text(text, opts, code)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, 1+len(encoded)+1)
	out = append(out, code)
	out = append(out, encoded...)
	out = append(out, 0x00)
	return out, nil
}

func patchFormattedTextBytes(raw []byte, block string, resources map[string]string, opts Options) ([]byte, error) {
	replacements, err := formattedBlockTextItems(block, resources)
	if err != nil {
		return nil, err
	}
	pos := 0
	repl := 0
	var out []byte
	for {
		if pos >= len(raw) {
			return nil, fmt.Errorf("unterminated formatted text")
		}
		tag := raw[pos]
		pos++
		if tag == 0x00 {
			if repl != len(replacements) {
				return nil, fmt.Errorf("formatted text string count changed: got %d, want %d", len(replacements), repl)
			}
			out = append(out, 0x00)
			if pos < len(raw) {
				out = append(out, raw[pos:]...)
			}
			return out, nil
		}
		switch tag {
		case 0xfe, 0xff:
			end := bytes.IndexByte(raw[pos:], 0x00)
			if end < 0 {
				return nil, fmt.Errorf("unterminated formatted text string")
			}
			if repl >= len(replacements) {
				return nil, fmt.Errorf("formatted text string count changed")
			}
			outTag := avg32WesternTextTag(replacements[repl], opts, tag)
			out = append(out, outTag)
			encoded, err := encodeAVG32Text(replacements[repl], opts, outTag)
			if err != nil {
				return nil, err
			}
			out = append(out, encoded...)
			out = append(out, 0x00)
			pos += end + 1
			repl++
		default:
			out = append(out, tag)
			next, err := skipFormattedTextPayload(raw, pos, tag)
			if err != nil {
				return nil, err
			}
			out = append(out, raw[pos:next]...)
			pos = next
		}
	}
}

func formattedTextEnd(raw []byte) (int, error) {
	pos := 0
	for {
		if pos >= len(raw) {
			return 0, fmt.Errorf("unterminated formatted text")
		}
		tag := raw[pos]
		pos++
		if tag == 0x00 {
			return pos, nil
		}
		switch tag {
		case 0xfe, 0xff:
			end := bytes.IndexByte(raw[pos:], 0x00)
			if end < 0 {
				return 0, fmt.Errorf("unterminated formatted text string")
			}
			pos += end + 1
		default:
			next, err := skipFormattedTextPayload(raw, pos, tag)
			if err != nil {
				return 0, err
			}
			pos = next
		}
	}
}

func skipFormattedTextPayload(raw []byte, pos int, tag byte) (int, error) {
	switch tag {
	case 0x10:
		if pos >= len(raw) {
			return 0, fmt.Errorf("truncated formatted command")
		}
		sub := raw[pos]
		pos++
		count := map[byte]int{0x01: 1, 0x02: 2, 0x03: 1, 0x11: 1, 0x13: 0}[sub]
		if _, ok := formattedTextNames[sub]; !ok {
			return 0, fmt.Errorf("unknown formatted text command 0x%02X", sub)
		}
		return skipVals(raw, pos, count)
	case 0x12:
		return pos, nil
	case 0x28:
		return skipConditionPayload(raw, pos)
	case 0xfd:
		return skipVal(raw, pos)
	default:
		return 0, fmt.Errorf("unknown formatted text tag 0x%02X", tag)
	}
}

func skipConditionPayload(raw []byte, pos int) (int, error) {
	depth := 0
	for {
		if pos >= len(raw) {
			return 0, fmt.Errorf("unterminated formatted text condition")
		}
		op := raw[pos]
		pos++
		switch op {
		case 0x26, 0x27:
		case 0x28:
			depth++
		case 0x29:
			depth--
			if depth <= 0 {
				return pos, nil
			}
		case 0x36, 0x37, 0x38, 0x39, 0x3a, 0x3b, 0x41, 0x42, 0x43, 0x44, 0x45,
			0x46, 0x47, 0x48, 0x49, 0x4f, 0x50, 0x51, 0x52, 0x53, 0x54, 0x55:
			var err error
			pos, err = skipVals(raw, pos, 2)
			if err != nil {
				return 0, err
			}
		case 0x58:
			if pos >= len(raw) {
				return 0, fmt.Errorf("truncated return condition")
			}
			attr := raw[pos]
			pos++
			if attr == 0x20 || attr == 0x22 {
				var err error
				pos, err = skipVal(raw, pos)
				if err != nil {
					return 0, err
				}
			} else if attr != 0x21 {
				return 0, fmt.Errorf("unknown return condition 0x%02X", attr)
			}
		default:
			return 0, fmt.Errorf("unknown condition opcode 0x%02X", op)
		}
	}
}

func skipVals(raw []byte, pos int, count int) (int, error) {
	var err error
	for i := 0; i < count; i++ {
		pos, err = skipVal(raw, pos)
		if err != nil {
			return 0, err
		}
	}
	return pos, nil
}

func skipVal(raw []byte, pos int) (int, error) {
	if pos >= len(raw) {
		return 0, fmt.Errorf("truncated value")
	}
	length := valueLen(raw[pos])
	if length == 0 {
		return 0, fmt.Errorf("invalid zero-length value marker 0x%02X", raw[pos])
	}
	if pos+length > len(raw) {
		return 0, fmt.Errorf("truncated value")
	}
	return pos + length, nil
}

func valueLen(num byte) int {
	return int((num >> 4) & 7)
}

func extractFormattedBlocks(line string) ([]string, error) {
	var blocks []string
	inQuote := false
	escaped := false
	depth := 0
	start := -1
	for i, r := range line {
		switch {
		case escaped:
			escaped = false
		case inQuote && r == '\\':
			escaped = true
		case r == '"':
			inQuote = !inQuote
		case inQuote:
		case r == '[':
			if depth == 0 {
				start = i
			}
			depth++
		case r == ']':
			if depth == 0 {
				return nil, fmt.Errorf("unmatched closing bracket")
			}
			depth--
			if depth == 0 && start >= 0 {
				blocks = append(blocks, line[start:i+1])
				start = -1
			}
		}
	}
	if inQuote {
		return nil, fmt.Errorf("unterminated quoted text")
	}
	if depth != 0 {
		return nil, fmt.Errorf("unterminated formatted text block")
	}
	return blocks, nil
}

func formattedBlockTextItems(block string, resources map[string]string) ([]string, error) {
	if !strings.HasPrefix(strings.TrimSpace(block), "[") {
		return nil, fmt.Errorf("formatted text block must start with '['")
	}
	var out []string
	for i := 0; i < len(block); {
		if strings.HasPrefix(strings.ToLower(block[i:]), "#res<") {
			end := strings.IndexByte(block[i:], '>')
			if end < 0 {
				return nil, fmt.Errorf("unterminated resource reference")
			}
			text, err := resolveResourceRef(block[i:i+end+1], resources)
			if err != nil {
				return nil, err
			}
			out = append(out, text)
			i += end + 1
			continue
		}
		if block[i] != '"' {
			i++
			continue
		}
		lit, n, err := readQuotedLiteral(block[i:])
		if err != nil {
			return nil, err
		}
		s, err := strconv.Unquote(lit)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
		i += n
	}
	return out, nil
}

func resolveResourceRef(ref string, resources map[string]string) (string, error) {
	ref = strings.TrimSpace(strings.TrimSuffix(ref, ","))
	if !strings.HasPrefix(strings.ToLower(ref), "#res<") || !strings.HasSuffix(ref, ">") {
		return "", fmt.Errorf("invalid resource reference %q", ref)
	}
	key := ref[len("#res<") : len(ref)-1]
	if resources == nil {
		return "", fmt.Errorf("resource %s referenced but no resource file was loaded", key)
	}
	text, ok := resources[key]
	if !ok {
		return "", fmt.Errorf("resource %s not found", key)
	}
	return text, nil
}

func loadAVG32Resources(src []byte, sourceDir string) (map[string]string, error) {
	paths, err := resourceDirectivePaths(src)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, nil
	}
	resources := make(map[string]string)
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			p = filepath.Join(sourceDir, p)
		}
		if err := loadAVG32ResourceFile(p, resources); err != nil {
			return nil, err
		}
	}
	return resources, nil
}

func resourceDirectivePaths(src []byte) ([]string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(src))
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	var paths []string
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(strings.ToLower(line), "#resource") {
			continue
		}
		rest := strings.TrimSpace(line[len("#resource"):])
		path, ok := unquoteDirectivePath(rest)
		if !ok {
			return nil, fmt.Errorf("line %d: malformed #resource directive", lineNo)
		}
		paths = append(paths, path)
	}
	return paths, scanner.Err()
}

func unquoteDirectivePath(rest string) (string, bool) {
	if len(rest) < 2 {
		return "", false
	}
	q := rest[0]
	if q != '\'' && q != '"' {
		return "", false
	}
	end := strings.IndexByte(rest[1:], q)
	if end < 0 {
		return "", false
	}
	return rest[1 : 1+end], true
}

func loadAVG32ResourceFile(path string, resources map[string]string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read AVG32 resource file %s: %w", path, err)
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if !strings.HasPrefix(line, "<") {
			continue
		}
		end := strings.IndexByte(line, '>')
		if end < 0 {
			return fmt.Errorf("%s line %d: malformed resource entry", path, lineNo)
		}
		key := line[1:end]
		body := ""
		if end+1 < len(line) {
			body = line[end+1:]
			if strings.HasPrefix(body, " ") {
				body = body[1:]
			}
			if isEscapedResourcePrefix(body) {
				body = body[1:]
			}
		}
		resources[key] = body
	}
	return scanner.Err()
}

func isEscapedResourcePrefix(body string) bool {
	if !strings.HasPrefix(body, `\`) {
		return false
	}
	if len(body) == 1 {
		return true
	}
	return body[1] == ' ' || body[1] == '\t' || body[1] == '<' || strings.HasPrefix(body[1:], "//")
}

func splitCall(line string) (name string, args string, ok bool) {
	open := strings.IndexByte(line, '(')
	close := strings.LastIndexByte(line, ')')
	if open < 0 || close < open {
		return "", "", false
	}
	return strings.TrimSpace(line[:open]), strings.TrimSpace(line[open+1 : close]), true
}

func stringInSet(s string, vals []string) bool {
	for _, v := range vals {
		if s == strings.ToLower(v) {
			return true
		}
	}
	return false
}

func encodeShiftJIS(text string) ([]byte, error) {
	encoded, _, err := transform.Bytes(japanese.ShiftJIS.NewEncoder(), []byte(text))
	if err != nil {
		return nil, fmt.Errorf("cannot encode text as Shift_JIS: %w", err)
	}
	return encoded, nil
}

func encodeAVG32Text(value string, opts Options, textTag byte) ([]byte, error) {
	if opts.TextTransform == texttransforms.EncNone {
		return encodeShiftJIS(value)
	}
	if opts.TextTransform == texttransforms.EncWestern {
		return encodeAVG32WesternText(value, opts, textTag)
	}

	previousMode := texttransforms.GetMode()
	previousForce := texttransforms.ForceEncode
	texttransforms.SetMode(opts.TextTransform)
	texttransforms.ForceEncode = opts.ForceTransform
	defer func() {
		texttransforms.SetMode(previousMode)
		texttransforms.ForceEncode = previousForce
	}()

	texttransforms.ResetBadChars()
	encoded, err := texttransforms.ToBytecode(text.OfUTF8(value))
	if err != nil {
		return nil, fmt.Errorf("cannot encode text with output transformation: %w", err)
	}
	return encoded, nil
}

func encodeAVG32WesternText(value string, opts Options, textTag byte) ([]byte, error) {
	previousMode := texttransforms.GetMode()
	previousForce := texttransforms.ForceEncode
	texttransforms.SetMode(texttransforms.EncWestern)
	texttransforms.ForceEncode = opts.ForceTransform
	defer func() {
		texttransforms.SetMode(previousMode)
		texttransforms.ForceEncode = previousForce
	}()

	out := make([]byte, 0, len(value))
	for _, r := range avg32WesternDisplayText(value, textTag) {
		if encoded, err := encodeShiftJIS(string(r)); err == nil {
			out = append(out, encoded...)
			continue
		}
		texttransforms.ResetBadChars()
		encoded, err := texttransforms.ToBytecode(text.OfChar(r))
		if err != nil {
			return nil, fmt.Errorf("cannot encode text with Western output transformation: %w", err)
		}
		out = append(out, encoded...)
	}
	return out, nil
}

func avg32WesternTextTag(value string, opts Options, textTag byte) byte {
	if opts.TextTransform == texttransforms.EncWestern && textTag == 0xff && avg32WesternPreferHankaku(value) {
		return 0xfe
	}
	return textTag
}

func avg32WesternPreferHankaku(value string) bool {
	hasLatin := false
	for _, r := range value {
		if isJapaneseTextRune(r) {
			return false
		}
		if r <= 0x7f || (r >= 0x00a0 && r <= 0x00ff) || r == '€' || r == 'Œ' || r == 'œ' || r == 'Š' || r == 'š' || r == 'Ÿ' {
			if r != ' ' && r != '\t' {
				hasLatin = true
			}
		}
	}
	return hasLatin
}

func isJapaneseTextRune(r rune) bool {
	return (r >= 0x3040 && r <= 0x30ff) ||
		(r >= 0x3400 && r <= 0x9fff) ||
		(r >= 0xf900 && r <= 0xfaff)
}

func avg32WesternDisplayText(value string, textTag byte) string {
	if textTag != 0xff {
		return value
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r == ' ':
			b.WriteRune('\u3000')
		case r >= '!' && r <= '~':
			b.WriteRune(rune(0xff01 + (r - '!')))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func rebuildTPC32WithPatchedInstructions(raw []byte, result *Result, instRaw [][]byte) ([]byte, error) {
	offsetMap := make(map[int]int, len(result.Instructions)+1)
	newRel := 0
	for i, inst := range result.Instructions {
		offsetMap[inst.Offset] = newRel
		newRel += len(instRaw[i])
	}
	oldTerm := codeTerminatorOffset(result)
	offsetMap[oldTerm] = newRel

	for i, inst := range result.Instructions {
		if err := patchInstructionTargets(inst, instRaw[i], offsetMap); err != nil {
			return nil, err
		}
	}

	oldTermAbs := result.Header.CodeOffset + oldTerm
	if oldTermAbs >= len(raw) {
		return nil, fmt.Errorf("raw TPC32 terminator is outside the scene")
	}

	out := append([]byte(nil), raw[:result.Header.CodeOffset]...)
	for _, data := range instRaw {
		out = append(out, data...)
	}
	out = append(out, 0x00)
	if oldTermAbs+1 < len(raw) {
		out = append(out, raw[oldTermAbs+1:]...)
	}
	patchHeaderLabels(out, result.Header, offsetMap)
	return out, nil
}

func codeTerminatorOffset(result *Result) int {
	if len(result.Instructions) == 0 {
		return 0
	}
	last := result.Instructions[len(result.Instructions)-1]
	return last.Offset + len(last.Raw)
}

func patchHeaderLabels(out []byte, header Header, offsetMap map[int]int) {
	for i, old := range header.Labels {
		if mapped, ok := offsetMap[int(old)]; ok {
			binary.LittleEndian.PutUint32(out[0x20+i*4:], uint32(mapped))
		}
	}
}

func patchInstructionTargets(inst Instruction, raw []byte, offsetMap map[int]int) error {
	if len(inst.Targets) == 0 {
		return nil
	}
	switch inst.Code {
	case 0x15:
		if len(raw) < 5 {
			return fmt.Errorf("condition at 0x%04X is too short", inst.Offset)
		}
		return patchTargetAt(raw, len(raw)-4, inst.Targets[0], offsetMap)
	case 0x1b, 0x1c:
		return patchTargetAt(raw, 1, inst.Targets[0], offsetMap)
	case 0x1d, 0x1e:
		return patchTableTargets(raw, inst, offsetMap)
	default:
		return fmt.Errorf("cannot retarget opcode 0x%02X at 0x%04X", inst.Code, inst.Offset)
	}
}

func patchTableTargets(raw []byte, inst Instruction, offsetMap map[int]int) error {
	if len(raw) < 3 {
		return fmt.Errorf("table jump at 0x%04X is too short", inst.Offset)
	}
	count := int(raw[1])
	valLen := int((raw[2] >> 4) & 7)
	if valLen == 0 {
		return fmt.Errorf("table jump at 0x%04X has invalid selector value", inst.Offset)
	}
	targetPos := 2 + valLen
	if len(raw) < targetPos+count*4 {
		return fmt.Errorf("table jump at 0x%04X is truncated", inst.Offset)
	}
	if len(inst.Targets) != count {
		return fmt.Errorf("table jump at 0x%04X target count mismatch", inst.Offset)
	}
	for i, target := range inst.Targets {
		if err := patchTargetAt(raw, targetPos+i*4, target, offsetMap); err != nil {
			return err
		}
	}
	return nil
}

func patchTargetAt(raw []byte, pos int, old int, offsetMap map[int]int) error {
	if pos < 0 || pos+4 > len(raw) {
		return fmt.Errorf("target patch position out of range")
	}
	mapped, ok := offsetMap[old]
	if !ok {
		return fmt.Errorf("cannot map target 0x%04X after text edit", old)
	}
	binary.LittleEndian.PutUint32(raw[pos:], uint32(mapped))
	return nil
}

func rawHexFields(line string) []string {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(strings.ToLower(line), "#rawhex") {
		line = strings.TrimSpace(line[len("#rawhex"):])
	}
	if idx := strings.Index(line, "//"); idx >= 0 {
		line = line[:idx]
	}
	line = strings.ReplaceAll(line, ",", " ")
	return strings.Fields(line)
}
