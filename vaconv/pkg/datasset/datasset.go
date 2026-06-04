// Package datasset converts selected RealLive DAT-side binary formats to JSON.
package datasset

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

const (
	TypeCGTable   = "reallive-cgtable-cgm"
	TypeToneCurve = "reallive-tonecurve-tcc"

	cgTableHeaderSize = 32
	tccDataOffset     = 0xFE8
	tccChannelSize    = 256
	tccEffectStride   = 0x340
	tccReservedSize   = 0x40
)

// Document is the JSON envelope used for DAT conversions.
type Document struct {
	Type string `json:"type"`

	// CGTABLE fields.
	Count            int            `json:"count,omitempty"`
	HeaderHex        string         `json:"header_hex,omitempty"`
	CompressedSize   int            `json:"compressed_size,omitempty"`
	DecompressedSize int            `json:"decompressed_size,omitempty"`
	RawHex           string         `json:"raw_hex,omitempty"`
	Entries          []CGTableEntry `json:"entries,omitempty"`

	// TCC fields.
	EffectCount int               `json:"effect_count,omitempty"`
	PrefixHex   string            `json:"prefix_hex,omitempty"`
	Effects     []ToneCurveEffect `json:"effects,omitempty"`
	TailHex     string            `json:"tail_hex,omitempty"`
}

// CGTableEntry stores one standard mode.cgm record.
type CGTableEntry struct {
	Index   int    `json:"index"`
	Name    string `json:"name,omitempty"`
	NameHex string `json:"name_hex,omitempty"`
}

// ToneCurveEffect stores one RGB LUT. Each channel must contain 256 values.
type ToneCurveEffect struct {
	Index       int    `json:"index"`
	Red         []int  `json:"red"`
	Green       []int  `json:"green"`
	Blue        []int  `json:"blue"`
	ReservedHex string `json:"reserved_hex,omitempty"`
}

// DecodeFile reads a supported DAT file and returns its editable JSON document.
func DecodeFile(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	switch {
	case isCGTable(data):
		return decodeCGTable(data)
	case isTCC(data):
		return decodeTCC(data)
	default:
		return nil, fmt.Errorf("unsupported DAT file")
	}
}

// WriteJSONFile exports a supported DAT file to JSON.
func WriteJSONFile(inputPath, outputPath string) error {
	doc, err := DecodeFile(inputPath)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(outputPath, data, 0644)
}

// WriteBinaryFromJSONFile rebuilds a DAT file from a JSON document.
func WriteBinaryFromJSONFile(inputPath, outputPath string) (string, error) {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return "", err
	}
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return "", err
	}
	out, ext, err := EncodeDocument(&doc)
	if err != nil {
		return "", err
	}
	return ext, os.WriteFile(outputPath, out, 0644)
}

// BinaryExtForJSONFile returns the binary extension implied by a DAT JSON file.
func BinaryExtForJSONFile(inputPath string) (string, error) {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return "", err
	}
	var doc struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return "", err
	}
	switch doc.Type {
	case TypeCGTable:
		return ".cgm", nil
	case TypeToneCurve:
		return ".tcc", nil
	default:
		return "", fmt.Errorf("unknown DAT JSON type %q", doc.Type)
	}
}

// EncodeDocument converts a JSON document back to binary.
func EncodeDocument(doc *Document) ([]byte, string, error) {
	switch doc.Type {
	case TypeCGTable:
		data, err := encodeCGTable(doc)
		return data, ".cgm", err
	case TypeToneCurve:
		data, err := encodeTCC(doc)
		return data, ".tcc", err
	default:
		return nil, "", fmt.Errorf("unknown DAT JSON type %q", doc.Type)
	}
}

func isCGTable(data []byte) bool {
	return len(data) >= cgTableHeaderSize && bytes.Equal(data[:8], []byte("CGTABLE\x00"))
}

func isTCC(data []byte) bool {
	return len(data) >= 8 && binary.LittleEndian.Uint32(data[:4]) == 1000
}

func decodeCGTable(data []byte) (*Document, error) {
	if len(data) < cgTableHeaderSize+8 {
		return nil, fmt.Errorf("CGTABLE file is too small")
	}
	header := append([]byte(nil), data[:cgTableHeaderSize]...)
	encrypted := append([]byte(nil), data[cgTableHeaderSize:]...)
	xorCGTablePayload(encrypted)
	raw, err := unpackCGTablePayload(encrypted)
	if err != nil {
		return nil, err
	}
	doc := &Document{
		Type:             TypeCGTable,
		Count:            int(binary.LittleEndian.Uint32(header[16:20])),
		HeaderHex:        hex.EncodeToString(header),
		CompressedSize:   len(encrypted),
		DecompressedSize: len(raw),
	}
	entries, err := parseCGTableEntries(raw, doc.Count)
	if err == nil {
		doc.Entries = entries
	} else {
		doc.RawHex = hex.EncodeToString(raw)
	}
	return doc, nil
}

func encodeCGTable(doc *Document) ([]byte, error) {
	raw, err := encodeCGTableRaw(doc)
	if err != nil {
		return nil, err
	}
	header, err := decodeHexField("header_hex", doc.HeaderHex)
	if err != nil || len(header) == 0 {
		header = make([]byte, cgTableHeaderSize)
		copy(header, []byte("CGTABLE\x00"))
	} else if len(header) != cgTableHeaderSize {
		return nil, fmt.Errorf("header_hex must decode to %d bytes", cgTableHeaderSize)
	}
	if len(doc.Entries) > 0 {
		binary.LittleEndian.PutUint32(header[16:20], uint32(len(doc.Entries)))
	} else if doc.Count > 0 {
		binary.LittleEndian.PutUint32(header[16:20], uint32(doc.Count))
	}
	payload := packCGTableLiteral(raw)
	xorCGTablePayload(payload)
	out := make([]byte, 0, len(header)+len(payload))
	out = append(out, header...)
	out = append(out, payload...)
	return out, nil
}

func encodeCGTableRaw(doc *Document) ([]byte, error) {
	if len(doc.Entries) > 0 {
		return encodeCGTableEntries(doc.Entries)
	}
	return decodeHexField("raw_hex", doc.RawHex)
}

func parseCGTableEntries(raw []byte, count int) ([]CGTableEntry, error) {
	if count <= 0 || len(raw) != count*36 {
		return nil, fmt.Errorf("CGTABLE is not a standard 36-byte entry table")
	}
	entries := make([]CGTableEntry, 0, count)
	for i := 0; i < count; i++ {
		offset := i * 36
		nameField := raw[offset : offset+32]
		nameLen := bytes.IndexByte(nameField, 0)
		if nameLen < 0 {
			nameLen = len(nameField)
		}
		nameBytes := nameField[:nameLen]
		entry := CGTableEntry{
			Index: int(binary.LittleEndian.Uint32(raw[offset+32 : offset+36])),
		}
		if utf8.Valid(nameBytes) {
			entry.Name = string(nameBytes)
		} else {
			entry.NameHex = hex.EncodeToString(nameBytes)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func encodeCGTableEntries(entries []CGTableEntry) ([]byte, error) {
	raw := make([]byte, len(entries)*36)
	for i, entry := range entries {
		offset := i * 36
		nameBytes, err := cgTableEntryNameBytes(entry)
		if err != nil {
			return nil, fmt.Errorf("entries[%d]: %w", i, err)
		}
		if len(nameBytes) > 32 {
			return nil, fmt.Errorf("entries[%d] name is %d bytes, maximum is 32", i, len(nameBytes))
		}
		copy(raw[offset:offset+32], nameBytes)
		if entry.Index < 0 {
			return nil, fmt.Errorf("entries[%d] index must be >= 0", i)
		}
		binary.LittleEndian.PutUint32(raw[offset+32:offset+36], uint32(entry.Index))
	}
	return raw, nil
}

func cgTableEntryNameBytes(entry CGTableEntry) ([]byte, error) {
	if strings.TrimSpace(entry.NameHex) != "" {
		return decodeHexField("name_hex", entry.NameHex)
	}
	return []byte(entry.Name), nil
}

func unpackCGTablePayload(src []byte) ([]byte, error) {
	if len(src) < 8 {
		return nil, fmt.Errorf("CGTABLE payload is too small")
	}
	rawLen := int(binary.LittleEndian.Uint32(src[0:4]))
	outLen := int(binary.LittleEndian.Uint32(src[4:8]))
	if rawLen <= 8 || rawLen > len(src) {
		return nil, fmt.Errorf("invalid CGTABLE compressed size %d", rawLen)
	}
	if outLen < 0 {
		return nil, fmt.Errorf("invalid CGTABLE decompressed size %d", outLen)
	}
	dst := make([]byte, outLen)
	si := 8
	di := 0
	mask := byte(0)
	flag := byte(0)
	for di < outLen && si < rawLen {
		if mask == 0 {
			flag = src[si]
			si++
			mask = 1
		}
		if flag&mask != 0 {
			if si >= rawLen {
				return nil, fmt.Errorf("truncated CGTABLE literal")
			}
			dst[di] = src[si]
			di++
			si++
		} else {
			if si+1 >= rawLen {
				return nil, fmt.Errorf("truncated CGTABLE back-reference")
			}
			code := int(src[si]) | int(src[si+1])<<8
			si += 2
			count := (code & 0x0F) + 2
			distance := code >> 4
			if distance <= 0 || distance > di {
				return nil, fmt.Errorf("invalid CGTABLE back-reference distance %d at output offset %d", distance, di)
			}
			ref := di - distance
			for i := 0; i < count && di < outLen; i++ {
				dst[di] = dst[ref]
				di++
				ref++
			}
		}
		mask <<= 1
	}
	if di != outLen {
		return nil, fmt.Errorf("CGTABLE payload ended after %d of %d bytes", di, outLen)
	}
	return dst, nil
}

func packCGTableLiteral(raw []byte) []byte {
	payload := make([]byte, 0, len(raw)+len(raw)/8+16)
	for pos := 0; pos < len(raw); {
		group := len(raw) - pos
		if group > 8 {
			group = 8
		}
		flag := byte((1 << group) - 1)
		payload = append(payload, flag)
		payload = append(payload, raw[pos:pos+group]...)
		pos += group
	}
	out := make([]byte, 8, 8+len(payload))
	binary.LittleEndian.PutUint32(out[0:4], uint32(8+len(payload)))
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(raw)))
	out = append(out, payload...)
	return out
}

func xorCGTablePayload(data []byte) {
	for i := range data {
		data[i] ^= cgTableKey[i%len(cgTableKey)]
	}
}

func decodeTCC(data []byte) (*Document, error) {
	count := int(binary.LittleEndian.Uint32(data[4:8]))
	if len(data) < tccDataOffset {
		return nil, fmt.Errorf("TCC file is too small")
	}
	minLen := tccDataOffset + count*3*tccChannelSize
	if count > 0 {
		minLen += (count - 1) * tccReservedSize
	}
	if len(data) < minLen {
		return nil, fmt.Errorf("TCC file is truncated: need at least %d bytes, got %d", minLen, len(data))
	}
	doc := &Document{
		Type:        TypeToneCurve,
		EffectCount: count,
		PrefixHex:   hex.EncodeToString(data[:tccDataOffset]),
		Effects:     make([]ToneCurveEffect, 0, count),
	}
	offset := tccDataOffset
	for i := 0; i < count; i++ {
		effect := ToneCurveEffect{
			Index: i,
			Red:   bytesToInts(data[offset : offset+tccChannelSize]),
		}
		offset += tccChannelSize
		effect.Green = bytesToInts(data[offset : offset+tccChannelSize])
		offset += tccChannelSize
		effect.Blue = bytesToInts(data[offset : offset+tccChannelSize])
		offset += tccChannelSize
		if offset+tccReservedSize <= len(data) && i < count-1 {
			effect.ReservedHex = hex.EncodeToString(data[offset : offset+tccReservedSize])
			offset += tccReservedSize
		}
		doc.Effects = append(doc.Effects, effect)
	}
	if offset < len(data) {
		doc.TailHex = hex.EncodeToString(data[offset:])
	}
	return doc, nil
}

func encodeTCC(doc *Document) ([]byte, error) {
	prefix, err := decodeHexField("prefix_hex", doc.PrefixHex)
	if err != nil {
		return nil, err
	}
	if len(prefix) == 0 {
		prefix = make([]byte, tccDataOffset)
	} else if len(prefix) != tccDataOffset {
		return nil, fmt.Errorf("prefix_hex must decode to %d bytes", tccDataOffset)
	} else {
		prefix = append([]byte(nil), prefix...)
	}
	binary.LittleEndian.PutUint32(prefix[0:4], 1000)
	binary.LittleEndian.PutUint32(prefix[4:8], uint32(len(doc.Effects)))
	out := append([]byte(nil), prefix...)
	for i, effect := range doc.Effects {
		if effect.Index != i {
			effect.Index = i
		}
		red, err := intsToBytes("red", effect.Red)
		if err != nil {
			return nil, err
		}
		green, err := intsToBytes("green", effect.Green)
		if err != nil {
			return nil, err
		}
		blue, err := intsToBytes("blue", effect.Blue)
		if err != nil {
			return nil, err
		}
		out = append(out, red...)
		out = append(out, green...)
		out = append(out, blue...)
		reserved, err := decodeHexField("reserved_hex", effect.ReservedHex)
		if err != nil {
			return nil, err
		}
		if len(reserved) > 0 {
			out = append(out, reserved...)
		} else if i < len(doc.Effects)-1 {
			out = append(out, make([]byte, tccReservedSize)...)
		}
	}
	tail, err := decodeHexField("tail_hex", doc.TailHex)
	if err != nil {
		return nil, err
	}
	out = append(out, tail...)
	return out, nil
}

func bytesToInts(data []byte) []int {
	out := make([]int, len(data))
	for i, b := range data {
		out[i] = int(b)
	}
	return out
}

func intsToBytes(name string, values []int) ([]byte, error) {
	if len(values) != tccChannelSize {
		return nil, fmt.Errorf("%s channel must contain %d values", name, tccChannelSize)
	}
	out := make([]byte, len(values))
	for i, v := range values {
		if v < 0 || v > 255 {
			return nil, fmt.Errorf("%s[%d] = %d, expected 0..255", name, i, v)
		}
		out[i] = byte(v)
	}
	return out, nil
}

func decodeHexField(name, value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	data, err := hex.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return data, nil
}

var cgTableKey = []byte{
	0x8b, 0xe5, 0x5d, 0xc3, 0xa1, 0xe0, 0x30, 0x44, 0x00, 0x85, 0xc0, 0x74, 0x09, 0x5f, 0x5e, 0x33,
	0xc0, 0x5b, 0x8b, 0xe5, 0x5d, 0xc3, 0x8b, 0x45, 0x0c, 0x85, 0xc0, 0x75, 0x14, 0x8b, 0x55, 0xec,
	0x83, 0xc2, 0x20, 0x52, 0x6a, 0x00, 0xe8, 0xf5, 0x28, 0x01, 0x00, 0x83, 0xc4, 0x08, 0x89, 0x45,
	0x0c, 0x8b, 0x45, 0xe4, 0x6a, 0x00, 0x6a, 0x00, 0x50, 0x53, 0xff, 0x15, 0x34, 0xb1, 0x43, 0x00,
	0x8b, 0x45, 0x10, 0x85, 0xc0, 0x74, 0x05, 0x8b, 0x4d, 0xec, 0x89, 0x08, 0x8a, 0x45, 0xf0, 0x84,
	0xc0, 0x75, 0x78, 0xa1, 0xe0, 0x30, 0x44, 0x00, 0x8b, 0x7d, 0xe8, 0x8b, 0x75, 0x0c, 0x85, 0xc0,
	0x75, 0x44, 0x8b, 0x1d, 0xd0, 0xb0, 0x43, 0x00, 0x85, 0xff, 0x76, 0x37, 0x81, 0xff, 0x00, 0x00,
	0x04, 0x00, 0x6a, 0x00, 0x76, 0x43, 0x8b, 0x45, 0xf8, 0x8d, 0x55, 0xfc, 0x52, 0x68, 0x00, 0x00,
	0x04, 0x00, 0x56, 0x50, 0xff, 0x15, 0x2c, 0xb1, 0x43, 0x00, 0x6a, 0x05, 0xff, 0xd3, 0xa1, 0xe0,
	0x30, 0x44, 0x00, 0x81, 0xef, 0x00, 0x00, 0x04, 0x00, 0x81, 0xc6, 0x00, 0x00, 0x04, 0x00, 0x85,
	0xc0, 0x74, 0xc5, 0x8b, 0x5d, 0xf8, 0x53, 0xe8, 0xf4, 0xfb, 0xff, 0xff, 0x8b, 0x45, 0x0c, 0x83,
	0xc4, 0x04, 0x5f, 0x5e, 0x5b, 0x8b, 0xe5, 0x5d, 0xc3, 0x8b, 0x55, 0xf8, 0x8d, 0x4d, 0xfc, 0x51,
	0x57, 0x56, 0x52, 0xff, 0x15, 0x2c, 0xb1, 0x43, 0x00, 0xeb, 0xd8, 0x8b, 0x45, 0xe8, 0x83, 0xc0,
	0x20, 0x50, 0x6a, 0x00, 0xe8, 0x47, 0x28, 0x01, 0x00, 0x8b, 0x7d, 0xe8, 0x89, 0x45, 0xf4, 0x8b,
	0xf0, 0xa1, 0xe0, 0x30, 0x44, 0x00, 0x83, 0xc4, 0x08, 0x85, 0xc0, 0x75, 0x56, 0x8b, 0x1d, 0xd0,
	0xb0, 0x43, 0x00, 0x85, 0xff, 0x76, 0x49, 0x81, 0xff, 0x00, 0x00, 0x04, 0x00, 0x6a, 0x00, 0x76,
}
