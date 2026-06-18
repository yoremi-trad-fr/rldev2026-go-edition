// Package orgtext exports and imports editable dialogue text from Kepago
// .org/.ke sources.
package orgtext

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yoremi/rldev-go/pkg/encoding"
)

type Result struct {
	SourcePath string
	OutputPath string
	Entries    int
	Resources  int
	Literals   int
	Wrote      bool
}

type sourceLine struct {
	Text string
	NL   string
}

type stringLiteral struct {
	Start int
	End   int
	Text  string
	Index int
}

type resRef struct {
	Key  string
	Line int
}

type textEntry struct {
	Key         string
	Text        string
	Kind        string
	Line        int
	Literal     int
	PairLine    int
	PairLiteral int
	Missing     bool
}

var (
	cpHeaderRE          = regexp.MustCompile(`(?i)\{-#\s*cp\s+([A-Za-z0-9_-]+)\s*#-`)
	resourceDirectiveRE = regexp.MustCompile(`(?i)^\s*#resource\s+['"]([^'"]+)['"]`)
	resRefRE            = regexp.MustCompile(`(?i)#res\s*<\s*([^>\s]+)\s*>`)
	metaRE              = regexp.MustCompile(`^//\s*@rlc-org-text-v1\s+(.+)$`)
	lineKeyRE           = regexp.MustCompile(`^L(\d+)_(\d+)$`)
	symbolicRE          = regexp.MustCompile(`^[A-Za-z0-9_./:+-]+$`)
)

func ExportFile(srcPath, outDir, encName string) (Result, error) {
	src, sourceEnc, err := readSource(srcPath, encName)
	if err != nil {
		return Result{}, err
	}
	lines := splitSourceLines(src)
	resources, _ := loadSourceResources(srcPath, lines, sourceEnc)
	entries := collectEntries(lines, resources)
	if len(entries) == 0 {
		return Result{SourcePath: srcPath}, nil
	}

	if outDir == "" {
		outDir = filepath.Dir(srcPath)
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return Result{}, err
	}
	outPath := filepath.Join(outDir, strings.TrimSuffix(filepath.Base(srcPath), filepath.Ext(srcPath))+".utf")
	if err := writeExportFile(outPath, filepath.Base(srcPath), entries); err != nil {
		return Result{}, err
	}

	result := Result{SourcePath: srcPath, OutputPath: outPath, Entries: len(entries), Wrote: true}
	for _, entry := range entries {
		if entry.Kind == "res" {
			result.Resources++
		} else {
			result.Literals++
		}
	}
	return result, nil
}

func ImportFile(srcPath, utfPath, outDir, encName string) (Result, error) {
	src, sourceEnc, err := readSource(srcPath, encName)
	if err != nil {
		return Result{}, err
	}
	lines := splitSourceLines(src)
	translated, err := readTranslationFile(utfPath)
	if err != nil {
		return Result{}, err
	}
	if len(translated) == 0 {
		return Result{SourcePath: srcPath}, nil
	}

	resourceKeys := collectResourceKeys(lines)
	resourceName := firstResourceName(lines)
	var resourceEntries []textEntry
	changedLines := map[int]bool{}
	for _, entry := range translated {
		if entry.Kind == "" {
			entry.Kind = inferEntryKind(entry.Key, resourceKeys)
		}
		switch entry.Kind {
		case "res":
			if resourceKeys[entry.Key] {
				resourceEntries = append(resourceEntries, entry)
			}
			if entry.PairLine > 0 {
				if err := replaceLineLiteral(lines, entry.PairLine, entry.PairLiteral, entry.Text); err != nil {
					return Result{}, err
				}
				changedLines[entry.PairLine] = true
			}
		case "literal":
			line, lit := entry.Line, entry.Literal
			if line == 0 {
				line, lit = parseLineKey(entry.Key)
			}
			if line == 0 {
				continue
			}
			if err := replaceLineLiteral(lines, line, lit, entry.Text); err != nil {
				return Result{}, err
			}
			changedLines[line] = true
		}
	}

	if outDir == "" {
		outDir = filepath.Dir(srcPath)
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return Result{}, err
	}
	if len(resourceEntries) > 0 && resourceName == "" {
		resourceName = strings.TrimSuffix(filepath.Base(srcPath), filepath.Ext(srcPath)) + ".utf"
		lines = insertResourceDirective(lines, resourceName)
	}

	srcOut := filepath.Join(outDir, filepath.Base(srcPath))
	if err := writeSource(srcOut, joinSourceLines(lines), sourceEnc); err != nil {
		return Result{}, err
	}

	result := Result{SourcePath: srcPath, OutputPath: srcOut, Entries: len(translated), Wrote: true, Literals: len(changedLines)}
	if len(resourceEntries) > 0 {
		resOut := filepath.Join(outDir, filepath.Base(resourceName))
		if err := writeResourceFile(resOut, filepath.Base(srcPath), resourceEntries); err != nil {
			return Result{}, err
		}
		result.Resources = len(resourceEntries)
	}
	return result, nil
}

func collectEntries(lines []sourceLine, resources map[string]string) []textEntry {
	resourceOrder := []string{}
	resourceSeen := map[string]bool{}
	resourceLines := map[string]int{}
	paired := map[string]textEntry{}
	pairedLiteralLines := map[string]bool{}
	literalEntries := []textEntry{}
	var lastStandalone *resRef

	for lineNo, line := range lines {
		trimmed := strings.TrimSpace(line.Text)
		refs := findResourceRefs(line.Text)
		for _, key := range refs {
			if !resourceSeen[key] {
				resourceSeen[key] = true
				resourceOrder = append(resourceOrder, key)
				resourceLines[key] = lineNo + 1
			}
		}
		if len(refs) == 1 && trimmed == fmt.Sprintf("#res<%s>", refs[0]) {
			lastStandalone = &resRef{Key: refs[0], Line: lineNo + 1}
		}

		if strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(strings.ToLower(trimmed), "#res") {
			continue
		}
		literals := findStringLiterals(line.Text)
		for _, lit := range literals {
			if !looksLikeDialogue(lit.Text) {
				continue
			}
			if lastStandalone != nil && lineNo+1-lastStandalone.Line <= 3 {
				entry := textEntry{
					Key:         lastStandalone.Key,
					Text:        lit.Text,
					Kind:        "res",
					Line:        lastStandalone.Line,
					PairLine:    lineNo + 1,
					PairLiteral: lit.Index,
				}
				paired[lastStandalone.Key] = entry
				pairedLiteralLines[literalIdentity(lineNo+1, lit.Index)] = true
				lastStandalone = nil
				continue
			}
			key := fmt.Sprintf("L%04d_%d", lineNo+1, lit.Index)
			literalEntries = append(literalEntries, textEntry{
				Key:     key,
				Text:    lit.Text,
				Kind:    "literal",
				Line:    lineNo + 1,
				Literal: lit.Index,
			})
		}
	}

	entries := make([]textEntry, 0, len(resourceOrder)+len(literalEntries))
	for _, key := range resourceOrder {
		if entry, ok := paired[key]; ok {
			if text, ok := resources[key]; ok && text != "" {
				entry.Text = text
			}
			entries = append(entries, entry)
			continue
		}
		text, ok := resources[key]
		if !ok {
			continue
		}
		entries = append(entries, textEntry{
			Key:  key,
			Text: text,
			Kind: "res",
			Line: resourceLines[key],
		})
	}
	for _, entry := range literalEntries {
		if !pairedLiteralLines[literalIdentity(entry.Line, entry.Literal)] {
			entries = append(entries, entry)
		}
	}
	return entries
}

func literalIdentity(line, lit int) string {
	return fmt.Sprintf("%d:%d", line, lit)
}

func readSource(path, fallbackEnc string) (string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	enc := SourceEncodingFromHeader(data, fallbackEnc)
	text, err := decodeBytes(data, enc)
	if err != nil {
		return "", "", fmt.Errorf("decode %s as %s: %w", path, enc, err)
	}
	return text, enc, nil
}

func writeSource(path, text, encName string) error {
	data, err := encodeString(text, encName)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func SourceEncodingFromHeader(data []byte, fallback string) string {
	if len(data) >= 3 && bytes.Equal(data[:3], []byte{0xef, 0xbb, 0xbf}) {
		data = data[3:]
	}
	head := string(data)
	if len(head) > 256 {
		head = head[:256]
	}
	if m := cpHeaderRE.FindStringSubmatch(head); m != nil {
		return normalizeEncoding(m[1])
	}
	if strings.TrimSpace(fallback) == "" {
		return "UTF-8"
	}
	return normalizeEncoding(fallback)
}

func normalizeEncoding(encName string) string {
	switch strings.ToUpper(strings.ReplaceAll(encName, "_", "-")) {
	case "", "UTF8", "UTF-8":
		return "UTF-8"
	case "CP932", "SHIFT-JIS", "SHIFTJIS", "SJIS":
		return "CP932"
	default:
		return encName
	}
}

func decodeBytes(data []byte, encName string) (string, error) {
	switch normalizeEncoding(encName) {
	case "UTF-8":
		return strings.TrimPrefix(string(data), "\ufeff"), nil
	case "CP932":
		return encoding.SJSToUTF8(data)
	default:
		enc := encoding.Parse(encName)
		return encoding.ToUTF8(data, enc)
	}
}

func encodeString(text, encName string) ([]byte, error) {
	switch normalizeEncoding(encName) {
	case "UTF-8":
		return []byte(text), nil
	case "CP932":
		return encoding.UTF8ToSJS(text)
	default:
		enc := encoding.Parse(encName)
		return encoding.FromUTF8String(text, enc)
	}
}

func splitSourceLines(src string) []sourceLine {
	parts := strings.SplitAfter(src, "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	lines := make([]sourceLine, 0, len(parts))
	for _, part := range parts {
		line := sourceLine{Text: part}
		if strings.HasSuffix(line.Text, "\n") {
			line.NL = "\n"
			line.Text = strings.TrimSuffix(line.Text, "\n")
			if strings.HasSuffix(line.Text, "\r") {
				line.Text = strings.TrimSuffix(line.Text, "\r")
				line.NL = "\r\n"
			}
		}
		lines = append(lines, line)
	}
	return lines
}

func joinSourceLines(lines []sourceLine) string {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(line.Text)
		b.WriteString(line.NL)
	}
	return b.String()
}

func findStringLiterals(line string) []stringLiteral {
	var out []stringLiteral
	for i := 0; i < len(line); i++ {
		if line[i] != '\'' {
			continue
		}
		start := i
		i++
		var text strings.Builder
		for i < len(line) {
			if line[i] == '\\' && i+1 < len(line) {
				text.WriteByte(line[i])
				i++
				text.WriteByte(line[i])
				i++
				continue
			}
			if line[i] == '\'' {
				out = append(out, stringLiteral{Start: start, End: i + 1, Text: text.String(), Index: len(out)})
				break
			}
			r, size := utf8.DecodeRuneInString(line[i:])
			text.WriteRune(r)
			i += size
		}
	}
	return out
}

func looksLikeDialogue(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || symbolicRE.MatchString(s) {
		return false
	}
	hasASCIIWord := false
	hasSeparator := false
	for _, r := range s {
		if isJapaneseOrCJK(r) || (r > 127 && unicode.IsLetter(r)) {
			return true
		}
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			hasASCIIWord = true
		}
		if unicode.IsSpace(r) || strings.ContainsRune(".,!?;:…。、，！？「」『』()[]", r) {
			hasSeparator = true
		}
	}
	return hasASCIIWord && hasSeparator && len([]rune(s)) >= 4
}

func isJapaneseOrCJK(r rune) bool {
	return (r >= 0x3040 && r <= 0x30ff) ||
		(r >= 0x3400 && r <= 0x9fff) ||
		(r >= 0xf900 && r <= 0xfaff) ||
		(r >= 0xff00 && r <= 0xffef)
}

func findResourceRefs(line string) []string {
	matches := resRefRE.FindAllStringSubmatch(line, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out
}

func collectResourceKeys(lines []sourceLine) map[string]bool {
	out := map[string]bool{}
	for _, line := range lines {
		for _, key := range findResourceRefs(line.Text) {
			out[key] = true
		}
	}
	return out
}

func loadSourceResources(srcPath string, lines []sourceLine, sourceEnc string) (map[string]string, error) {
	resources := map[string]string{}
	for _, name := range resourceNames(lines) {
		path := name
		if !filepath.IsAbs(path) {
			path = filepath.Join(filepath.Dir(srcPath), name)
		}
		loaded, err := loadResourceFile(path, sourceEnc)
		if err != nil {
			continue
		}
		for key, value := range loaded {
			resources[key] = value
		}
	}
	return resources, nil
}

func resourceNames(lines []sourceLine) []string {
	var out []string
	for _, line := range lines {
		if m := resourceDirectiveRE.FindStringSubmatch(line.Text); m != nil {
			out = append(out, m[1])
		}
	}
	return out
}

func firstResourceName(lines []sourceLine) string {
	names := resourceNames(lines)
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func loadResourceFile(path, encName string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text, err := decodeBytes(data, encName)
	if err != nil {
		return nil, err
	}
	resources := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if key, body, ok := parseResourceEntryLine(line); ok {
			resources[key] = body
		}
	}
	return resources, scanner.Err()
}

func parseResourceEntryLine(line string) (key, body string, ok bool) {
	if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "//") {
		return "", "", false
	}
	if !strings.HasPrefix(line, "<") {
		return "", "", false
	}
	end := strings.IndexByte(line, '>')
	if end <= 0 {
		return "", "", false
	}
	body = line[end+1:]
	if strings.HasPrefix(body, " ") || strings.HasPrefix(body, "\t") {
		body = body[1:]
	}
	return line[1:end], body, true
}

func writeExportFile(path, sourceName string, entries []textEntry) error {
	var b strings.Builder
	fmt.Fprintf(&b, "// RLdev ORG text export for %s\r\n", sourceName)
	b.WriteString("// Edit the text after each <key>, then import this .utf with the matching .org/.ke.\r\n\r\n")
	for _, entry := range entries {
		fmt.Fprintf(&b, "// @rlc-org-text-v1 key=%s kind=%s line=%d lit=%d", entry.Key, entry.Kind, entry.Line, entry.Literal)
		if entry.PairLine > 0 {
			fmt.Fprintf(&b, " pair_line=%d pair_lit=%d", entry.PairLine, entry.PairLiteral)
		}
		if entry.Missing {
			b.WriteString(" missing=1")
		}
		b.WriteString("\r\n")
		fmt.Fprintf(&b, "<%s> %s\r\n", entry.Key, escapeResourceLineText(entry.Text))
	}
	return os.WriteFile(path, []byte(b.String()), 0644)
}

func writeResourceFile(path, sourceName string, entries []textEntry) error {
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	var b strings.Builder
	fmt.Fprintf(&b, "// Resources for %s\r\n\r\n", sourceName)
	for _, entry := range entries {
		fmt.Fprintf(&b, "<%s> %s\r\n", entry.Key, escapeResourceLineText(entry.Text))
	}
	return os.WriteFile(path, []byte(b.String()), 0644)
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

func readTranslationFile(path string) ([]textEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := strings.TrimPrefix(string(data), "\ufeff")
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var entries []textEntry
	var pending *textEntry
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if meta := parseMetaLine(line); meta != nil {
			pending = meta
			continue
		}
		key, body, ok := parseResourceEntryLine(line)
		if !ok {
			continue
		}
		entry := textEntry{Key: key, Text: body}
		if pending != nil && pending.Key == key {
			entry.Kind = pending.Kind
			entry.Line = pending.Line
			entry.Literal = pending.Literal
			entry.PairLine = pending.PairLine
			entry.PairLiteral = pending.PairLiteral
			entry.Missing = pending.Missing
		}
		entries = append(entries, entry)
		pending = nil
	}
	return entries, scanner.Err()
}

func parseMetaLine(line string) *textEntry {
	m := metaRE.FindStringSubmatch(line)
	if m == nil {
		return nil
	}
	fields := strings.Fields(m[1])
	entry := &textEntry{}
	for _, field := range fields {
		k, v, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		switch k {
		case "key":
			entry.Key = v
		case "kind":
			entry.Kind = v
		case "line":
			entry.Line, _ = strconv.Atoi(v)
		case "lit":
			entry.Literal, _ = strconv.Atoi(v)
		case "pair_line":
			entry.PairLine, _ = strconv.Atoi(v)
		case "pair_lit":
			entry.PairLiteral, _ = strconv.Atoi(v)
		case "missing":
			entry.Missing = v == "1" || strings.EqualFold(v, "true")
		}
	}
	if entry.Key == "" {
		return nil
	}
	return entry
}

func inferEntryKind(key string, resourceKeys map[string]bool) string {
	if resourceKeys[key] {
		return "res"
	}
	if line, _ := parseLineKey(key); line > 0 {
		return "literal"
	}
	return "res"
}

func parseLineKey(key string) (line, lit int) {
	m := lineKeyRE.FindStringSubmatch(key)
	if m == nil {
		return 0, 0
	}
	line, _ = strconv.Atoi(m[1])
	lit, _ = strconv.Atoi(m[2])
	return line, lit
}

func replaceLineLiteral(lines []sourceLine, lineNo, litIndex int, text string) error {
	if lineNo < 1 || lineNo > len(lines) {
		return fmt.Errorf("line %d out of range", lineNo)
	}
	line := lines[lineNo-1].Text
	literals := findStringLiterals(line)
	if litIndex < 0 || litIndex >= len(literals) {
		return fmt.Errorf("line %d literal %d not found", lineNo, litIndex)
	}
	lit := literals[litIndex]
	lines[lineNo-1].Text = line[:lit.Start] + quoteKepagoString(text) + line[lit.End:]
	return nil
}

func quoteKepagoString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `\'`) + "'"
}

func insertResourceDirective(lines []sourceLine, name string) []sourceLine {
	insert := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line.Text)
		if strings.HasPrefix(strings.ToLower(trimmed), "#file") {
			insert = i + 1
			break
		}
	}
	nl := "\r\n"
	if len(lines) > 0 && lines[0].NL != "" {
		nl = lines[0].NL
	}
	newLine := sourceLine{Text: fmt.Sprintf("#resource '%s'", filepath.Base(name)), NL: nl}
	out := append([]sourceLine{}, lines[:insert]...)
	out = append(out, newLine)
	out = append(out, lines[insert:]...)
	return out
}
