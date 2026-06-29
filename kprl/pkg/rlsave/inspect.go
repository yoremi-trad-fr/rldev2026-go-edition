package rlsave

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

type Summary struct {
	Path            string    `json:"path,omitempty"`
	Label           string    `json:"label"`
	Kind            Kind      `json:"kind"`
	Container       Container `json:"container"`
	FileSize        int       `json:"fileSize"`
	HeaderLen       int       `json:"headerLen"`
	CompressedSize  int       `json:"compressedSize,omitempty"`
	BodySize        int       `json:"bodySize"`
	NonZeroBytes    int       `json:"nonZeroBytes"`
	NonZeroDWords   int       `json:"nonZeroDWords"`
	NonZeroByteRuns int       `json:"nonZeroByteRuns"`
}

type BodyStats struct {
	Size            int `json:"size"`
	DWords          int `json:"dwords"`
	NonZeroBytes    int `json:"nonZeroBytes"`
	NonZeroDWords   int `json:"nonZeroDWords"`
	NonZeroByteRuns int `json:"nonZeroByteRuns"`
}

type DWordEntry struct {
	Index  int    `json:"index"`
	Offset int    `json:"offset"`
	Value  uint32 `json:"value"`
}

type ByteRun struct {
	Start  int `json:"start"`
	End    int `json:"end"`
	Length int `json:"length"`
}

type StringEntry struct {
	Offset int    `json:"offset"`
	Text   string `json:"text"`
}

type ExportOptions struct {
	Lossless bool
}

var (
	intGLineRE = regexp.MustCompile(`^intG\[(\d+)\]\s*=\s*([+-]?(?:0x[0-9a-fA-F]+|\d+))`)
	seenLineRE = regexp.MustCompile(`^seen\[(\d+)\]\s*=\s*(0x[0-9a-fA-F]+|\d+)`)
	dwordRE    = regexp.MustCompile(`^dword\[(\d+)\]\s*=\s*(0x[0-9a-fA-F]+|\d+)`)
)

func (s *Save) Summary() Summary {
	stats := s.BodyStats()
	return Summary{
		Path:            s.Path,
		Label:           s.Label,
		Kind:            s.Kind,
		Container:       s.Container,
		FileSize:        len(s.raw),
		HeaderLen:       s.HeaderLen,
		CompressedSize:  s.CompressedSize,
		BodySize:        len(s.Body),
		NonZeroBytes:    stats.NonZeroBytes,
		NonZeroDWords:   stats.NonZeroDWords,
		NonZeroByteRuns: stats.NonZeroByteRuns,
	}
}

func (s *Save) BodyStats() BodyStats {
	stats := BodyStats{
		Size:   len(s.Body),
		DWords: len(s.Body) / 4,
	}
	inRun := false
	for _, b := range s.Body {
		if b != 0 {
			stats.NonZeroBytes++
			if !inRun {
				stats.NonZeroByteRuns++
				inRun = true
			}
			continue
		}
		if inRun {
			inRun = false
		}
	}
	for off := 0; off+3 < len(s.Body); off += 4 {
		if le32(s.Body, off) != 0 {
			stats.NonZeroDWords++
		}
	}
	return stats
}

func (s *Save) NonZeroDWords() []DWordEntry {
	return nonZeroDWords(s.Body)
}

func (s *Save) NonZeroByteRuns() []ByteRun {
	var runs []ByteRun
	start := -1
	for i, b := range s.Body {
		if b != 0 {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			runs = append(runs, ByteRun{Start: start, End: i - 1, Length: i - start})
			start = -1
		}
	}
	if start >= 0 {
		runs = append(runs, ByteRun{Start: start, End: len(s.Body) - 1, Length: len(s.Body) - start})
	}
	return runs
}

func (s *Save) RawSize() int {
	return len(s.raw)
}

func (s *Save) BodyDWord(index int) (uint32, error) {
	if err := s.checkBodyDWordIndex(index); err != nil {
		return 0, err
	}
	return le32(s.Body, index*4), nil
}

func (s *Save) SetBodyDWord(index int, value uint32) error {
	if err := s.checkBodyDWordIndex(index); err != nil {
		return err
	}
	put32(s.Body, index*4, value)
	return nil
}

func (s *Save) ReadProgress(seen int) (uint32, error) {
	if s.Kind != KindRead {
		return 0, fmt.Errorf("seen progress editing is supported only for read.sav")
	}
	return s.BodyDWord(seen)
}

func (s *Save) SetReadProgress(seen int, value uint32) error {
	if s.Kind != KindRead {
		return fmt.Errorf("seen progress editing is supported only for read.sav")
	}
	return s.SetBodyDWord(seen, value)
}

func ExportText(w io.Writer, save *Save, opts ExportOptions) error {
	if save == nil {
		return fmt.Errorf("nil save")
	}
	stats := save.BodyStats()

	fmt.Fprintln(w, "# rlsave text v1")
	fmt.Fprintln(w, "# Edit intG[...], seen[...] or dword[...] values, then rebuild from an export made with -lossless.")
	fmt.Fprintln(w, "# Human-editable decoded sections are above the data.*_base64 blocks.")
	fmt.Fprintln(w, "# data.*_base64 blocks are binary rebuild payloads, not encrypted dialogue text.")
	if !opts.Lossless {
		fmt.Fprintln(w, "# This export is readable only; add -lossless to embed rebuild data.")
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "[meta]")
	fmt.Fprintf(w, "kind = %s\n", save.Kind)
	fmt.Fprintf(w, "container = %s\n", save.Container)
	fmt.Fprintf(w, "label = %q\n", save.Label)
	fmt.Fprintf(w, "file_size = %d\n", len(save.raw))
	fmt.Fprintf(w, "header_len = %d\n", save.HeaderLen)
	fmt.Fprintf(w, "compressed_size = %d\n", save.CompressedSize)
	fmt.Fprintf(w, "body_size = %d\n", len(save.Body))
	fmt.Fprintln(w)

	fmt.Fprintln(w, "[body.stats]")
	fmt.Fprintf(w, "bytes = %d\n", stats.Size)
	fmt.Fprintf(w, "dwords = %d\n", stats.DWords)
	fmt.Fprintf(w, "nonzero_bytes = %d\n", stats.NonZeroBytes)
	fmt.Fprintf(w, "nonzero_dwords = %d\n", stats.NonZeroDWords)
	fmt.Fprintf(w, "nonzero_byte_runs = %d\n", stats.NonZeroByteRuns)
	fmt.Fprintln(w)

	writeHeaderDWords(w, save)
	writeStrings(w, "body.ascii_strings", save.Body, 5, 80)

	switch save.Kind {
	case KindGlobal:
		if err := writeGlobalInts(w, save); err != nil {
			return err
		}
	case KindRead:
		writeReadProgress(w, save.NonZeroDWords())
	case KindSystem:
		writeDWords(w, "system.nonzero_dwords", save.NonZeroDWords())
	default:
		writeDWords(w, "body.nonzero_dwords", save.NonZeroDWords())
	}
	writeRuns(w, save.NonZeroByteRuns())

	if opts.Lossless {
		if save.HeaderLen > len(save.raw) {
			return fmt.Errorf("invalid header length %d for file size %d", save.HeaderLen, len(save.raw))
		}
		writeBase64Block(w, "data.header_base64", save.raw[:save.HeaderLen])
		writeBase64Block(w, "data.body_base64", save.Body)
		if save.Container == ContainerCompressed {
			trailingOffset := save.HeaderLen + save.CompressedSize
			if trailingOffset < len(save.raw) {
				writeBase64Block(w, "data.trailing_base64", save.raw[trailingOffset:])
			}
		}
	}

	return nil
}

func ImportText(r io.Reader) (*Save, error) {
	content, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var section string
	meta := map[string]string{}
	blocks := map[string][]string{}
	var edits []func(*Save) error

	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		if strings.HasPrefix(section, "data.") {
			blocks[section] = append(blocks[section], line)
			continue
		}
		if section == "meta" {
			key, value, ok := splitKeyValue(line)
			if ok {
				meta[key] = value
			}
			continue
		}
		if section == "global.intG" {
			edit, ok, err := parseIntGEdit(line)
			if err != nil {
				return nil, err
			}
			if ok {
				edits = append(edits, edit)
			}
			continue
		}
		if section == "read.seen_progress" {
			edit, ok, err := parseSeenEdit(line)
			if err != nil {
				return nil, err
			}
			if ok {
				edits = append(edits, edit)
			}
			continue
		}
		if strings.HasSuffix(section, "nonzero_dwords") || section == "body.dwords" {
			edit, ok, err := parseDWordEdit(line)
			if err != nil {
				return nil, err
			}
			if ok {
				edits = append(edits, edit)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	container := Container(meta["container"])
	if container == "" {
		return nil, fmt.Errorf("missing meta container")
	}
	header, err := decodeBase64Block(blocks["data.header_base64"])
	if err != nil {
		return nil, fmt.Errorf("decode header base64: %w", err)
	}
	body, err := decodeBase64Block(blocks["data.body_base64"])
	if err != nil {
		return nil, fmt.Errorf("decode body base64: %w", err)
	}
	trailing, err := decodeBase64Block(blocks["data.trailing_base64"])
	if err != nil {
		return nil, fmt.Errorf("decode trailing base64: %w", err)
	}
	if len(header) == 0 || len(body) == 0 {
		return nil, fmt.Errorf("lossless data is missing; export again with -lossless")
	}
	if headerLenText := meta["header_len"]; headerLenText != "" {
		headerLen, err := strconv.Atoi(headerLenText)
		if err != nil {
			return nil, fmt.Errorf("bad header_len %q: %w", headerLenText, err)
		}
		if headerLen != len(header) {
			return nil, fmt.Errorf("header_len is %d but embedded header has %d bytes", headerLen, len(header))
		}
	}

	rawLen := len(header) + len(body)
	if container == ContainerCompressed {
		rawLen = len(header) + len(trailing)
	}
	raw := make([]byte, rawLen)
	copy(raw, header)
	if container == ContainerCompressed {
		copy(raw[len(header):], trailing)
	} else {
		copy(raw[len(header):], body)
	}
	save := &Save{
		Kind:             detectKind(readCString(header, 0x18, 64), container),
		Container:        container,
		Label:            readCString(header, 0x18, 64),
		HeaderLen:        len(header),
		CompressedSize:   0,
		UncompressedSize: len(body),
		Body:             body,
		raw:              raw,
	}

	for _, edit := range edits {
		if err := edit(save); err != nil {
			return nil, err
		}
	}

	out, err := save.Bytes()
	if err != nil {
		return nil, err
	}
	return Parse(out)
}

func writeHeaderDWords(w io.Writer, save *Save) {
	fmt.Fprintln(w, "[header.dwords]")
	limit := save.HeaderLen
	if limit > len(save.raw) {
		limit = len(save.raw)
	}
	for off := 0; off+3 < limit; off += 4 {
		value := le32(save.raw, off)
		fmt.Fprintf(w, "dword[%d] = 0x%08X ; offset=0x%04X decimal=%d\n", off/4, value, off, value)
	}
	fmt.Fprintln(w)
}

func writeGlobalInts(w io.Writer, save *Save) error {
	ints, err := save.NonZeroGlobalInts()
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "[global.intG]")
	for _, entry := range ints {
		fmt.Fprintf(w, "intG[%d] = %d\n", entry.Index, entry.Value)
	}
	fmt.Fprintln(w)
	return nil
}

func writeReadProgress(w io.Writer, entries []DWordEntry) {
	fmt.Fprintln(w, "[read.seen_progress]")
	fmt.Fprintln(w, "# seen[n] maps to the read/progression dword for script seenNNNN / seenNNNN.org.")
	for _, entry := range entries {
		fmt.Fprintf(w, "seen[%d] = %d ; script=seen%04d org=seen%04d.org dword[%d] offset=0x%06X hex=0x%08X\n", entry.Index, entry.Value, entry.Index, entry.Index, entry.Index, entry.Offset, entry.Value)
	}
	fmt.Fprintln(w)
}

func writeDWords(w io.Writer, section string, entries []DWordEntry) {
	fmt.Fprintf(w, "[%s]\n", section)
	for _, entry := range entries {
		fmt.Fprintf(w, "dword[%d] = 0x%08X ; offset=0x%06X decimal=%d\n", entry.Index, entry.Value, entry.Offset, entry.Value)
	}
	fmt.Fprintln(w)
}

func writeRuns(w io.Writer, runs []ByteRun) {
	fmt.Fprintln(w, "[body.nonzero_byte_runs]")
	for _, run := range runs {
		fmt.Fprintf(w, "0x%06X..0x%06X len=%d\n", run.Start, run.End, run.Length)
	}
	fmt.Fprintln(w)
}

func writeStrings(w io.Writer, section string, data []byte, minLen, limit int) {
	entries := asciiStrings(data, minLen, limit)
	if len(entries) == 0 {
		return
	}
	fmt.Fprintf(w, "[%s]\n", section)
	for _, entry := range entries {
		fmt.Fprintf(w, "0x%06X = %q\n", entry.Offset, entry.Text)
	}
	fmt.Fprintln(w)
}

func writeBase64Block(w io.Writer, section string, data []byte) {
	encoded := base64.StdEncoding.EncodeToString(data)
	fmt.Fprintf(w, "[%s]\n", section)
	for len(encoded) > 76 {
		fmt.Fprintln(w, encoded[:76])
		encoded = encoded[76:]
	}
	if encoded != "" {
		fmt.Fprintln(w, encoded)
	}
	fmt.Fprintln(w)
}

func nonZeroDWords(data []byte) []DWordEntry {
	var entries []DWordEntry
	for off := 0; off+3 < len(data); off += 4 {
		value := le32(data, off)
		if value == 0 {
			continue
		}
		entries = append(entries, DWordEntry{Index: off / 4, Offset: off, Value: value})
	}
	return entries
}

func asciiStrings(data []byte, minLen, limit int) []StringEntry {
	var entries []StringEntry
	start := -1
	for i, b := range data {
		if b >= 0x20 && b <= 0x7e {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 && i-start >= minLen {
			entries = append(entries, StringEntry{Offset: start, Text: string(data[start:i])})
			if limit > 0 && len(entries) >= limit {
				return entries
			}
		}
		start = -1
	}
	if start >= 0 && len(data)-start >= minLen {
		entries = append(entries, StringEntry{Offset: start, Text: string(data[start:])})
	}
	if limit > 0 && len(entries) > limit {
		return entries[:limit]
	}
	return entries
}

func splitKeyValue(line string) (string, string, bool) {
	line = stripInlineComment(line)
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.Trim(strings.TrimSpace(parts[1]), `"`), true
}

func stripInlineComment(line string) string {
	if idx := strings.Index(line, ";"); idx >= 0 {
		return strings.TrimSpace(line[:idx])
	}
	return strings.TrimSpace(line)
}

func parseIntGEdit(line string) (func(*Save) error, bool, error) {
	line = stripInlineComment(line)
	m := intGLineRE.FindStringSubmatch(line)
	if m == nil {
		return nil, false, nil
	}
	index, err := strconv.Atoi(m[1])
	if err != nil {
		return nil, false, err
	}
	value, err := strconv.ParseInt(m[2], 0, 32)
	if err != nil {
		return nil, false, err
	}
	return func(save *Save) error {
		return save.SetGlobalInt(index, int32(value))
	}, true, nil
}

func parseSeenEdit(line string) (func(*Save) error, bool, error) {
	line = stripInlineComment(line)
	m := seenLineRE.FindStringSubmatch(line)
	if m == nil {
		return nil, false, nil
	}
	index, err := strconv.Atoi(m[1])
	if err != nil {
		return nil, false, err
	}
	value, err := strconv.ParseUint(m[2], 0, 32)
	if err != nil {
		return nil, false, err
	}
	return func(save *Save) error {
		return save.SetReadProgress(index, uint32(value))
	}, true, nil
}

func parseDWordEdit(line string) (func(*Save) error, bool, error) {
	line = stripInlineComment(line)
	m := dwordRE.FindStringSubmatch(line)
	if m == nil {
		return nil, false, nil
	}
	index, err := strconv.Atoi(m[1])
	if err != nil {
		return nil, false, err
	}
	value, err := strconv.ParseUint(m[2], 0, 32)
	if err != nil {
		return nil, false, err
	}
	return func(save *Save) error {
		return save.SetBodyDWord(index, uint32(value))
	}, true, nil
}

func (s *Save) checkBodyDWordIndex(index int) error {
	if index < 0 {
		return fmt.Errorf("dword index %d out of range", index)
	}
	dwordCount := len(s.Body) / 4
	if index >= dwordCount {
		return fmt.Errorf("dword[%d] is outside body dword range 0..%d", index, dwordCount-1)
	}
	return nil
}

func decodeBase64Block(lines []string) ([]byte, error) {
	if len(lines) == 0 {
		return nil, nil
	}
	text := strings.Join(lines, "")
	return base64.StdEncoding.DecodeString(text)
}
