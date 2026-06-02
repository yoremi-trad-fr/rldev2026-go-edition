package kprl

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/yoremi/rldev-go/pkg/avg32"
	"github.com/yoremi/rldev-go/pkg/binarray"
)

const (
	avg32ArchiveHeaderSize = 0x20
	avg32ArchiveEntrySize  = 0x20
	avg32PackHeaderSize    = 0x10
)

var avg32SeenRe = regexp.MustCompile(`(?i)^SEEN0*([0-9]{1,4})\.TXT`)

type avg32ArchiveEntry struct {
	Index          int
	Name           string
	Offset         int
	PackedSize     int
	UnpackedSize   int
	Flags          int
	ArchiveOrdinal int
}

func avg32ArchiveCount(arr *binarray.Buffer) int {
	entries, err := parseAVG32ArchiveEntries(arr)
	if err != nil {
		return -1
	}
	return len(entries)
}

func loadAVG32Archive(data *binarray.Buffer) (*Archive, error) {
	entries, err := parseAVG32ArchiveEntries(data)
	if err != nil {
		return nil, err
	}

	arc := &Archive{
		Data:   data,
		Format: ArchiveFormatAVG32,
		Count:  len(entries),
	}
	for _, entry := range entries {
		arc.Entries[entry.Index] = SeenEntry{Offset: entry.Offset, Length: entry.PackedSize}
		arc.Names[entry.Index] = entry.Name
		arc.Order = append(arc.Order, entry.Index)
	}
	return arc, nil
}

func parseAVG32ArchiveEntries(arr *binarray.Buffer) ([]avg32ArchiveEntry, error) {
	if arr == nil || arr.Len() < avg32ArchiveHeaderSize {
		return nil, fmt.Errorf("archive too short")
	}
	if arr.Read(0, 4) != "PACL" {
		return nil, fmt.Errorf("missing PACL magic")
	}

	entryCount := int(u32(arr.Data, 0x10))
	if entryCount < 0 || entryCount > MaxSeens {
		return nil, fmt.Errorf("invalid AVG32 entry count %d", entryCount)
	}
	tableEnd := avg32ArchiveHeaderSize + entryCount*avg32ArchiveEntrySize
	if tableEnd > arr.Len() {
		return nil, fmt.Errorf("AVG32 entry table exceeds file size")
	}

	entries := make([]avg32ArchiveEntry, 0, entryCount)
	seen := make(map[int]bool, entryCount)
	for i := 0; i < entryCount; i++ {
		base := avg32ArchiveHeaderSize + i*avg32ArchiveEntrySize
		name := readFixedCString(arr.Data[base : base+0x10])
		idx, ok := avg32SeenIndex(name)
		if !ok {
			return nil, fmt.Errorf("unsupported AVG32 archive entry name %q", name)
		}
		if seen[idx] {
			return nil, fmt.Errorf("duplicate AVG32 SEEN index %d", idx)
		}
		seen[idx] = true

		offset := int(u32(arr.Data, base+0x10))
		packedSize := int(u32(arr.Data, base+0x14))
		unpackedSize := int(u32(arr.Data, base+0x18))
		flags := int(u32(arr.Data, base+0x1c))

		if offset < tableEnd || packedSize < avg32PackHeaderSize || offset+packedSize > arr.Len() {
			return nil, fmt.Errorf("%s has invalid PACK bounds", name)
		}
		if arr.Read(offset, 4) != "PACK" {
			return nil, fmt.Errorf("%s missing PACK magic", name)
		}
		if packUnpacked := int(u32(arr.Data, offset+0x08)); packUnpacked != unpackedSize {
			return nil, fmt.Errorf("%s unpacked size mismatch: table=%d pack=%d", name, unpackedSize, packUnpacked)
		}
		if packPacked := int(u32(arr.Data, offset+0x0c)); packPacked != packedSize {
			return nil, fmt.Errorf("%s packed size mismatch: table=%d pack=%d", name, packedSize, packPacked)
		}

		entries = append(entries, avg32ArchiveEntry{
			Index:          idx,
			Name:           name,
			Offset:         offset,
			PackedSize:     packedSize,
			UnpackedSize:   unpackedSize,
			Flags:          flags,
			ArchiveOrdinal: i,
		})
	}

	return entries, nil
}

func avg32SeenIndex(name string) (int, bool) {
	m := avg32SeenRe.FindStringSubmatch(strings.ToUpper(name))
	if m == nil {
		return 0, false
	}
	var idx int
	for _, ch := range m[1] {
		idx = idx*10 + int(ch-'0')
	}
	return idx, idx >= 0 && idx < MaxSeens
}

func readFixedCString(data []byte) string {
	end := len(data)
	for i, b := range data {
		if b == 0 {
			end = i
			break
		}
	}
	return string(data[:end])
}

func avg32PackSizes(pack *binarray.Buffer) (unpackedSize, packedSize int, err error) {
	if pack == nil || pack.Len() < avg32PackHeaderSize {
		return 0, 0, fmt.Errorf("PACK block too short")
	}
	if pack.Read(0, 4) != "PACK" {
		return 0, 0, fmt.Errorf("missing PACK magic")
	}
	unpackedSize = int(u32(pack.Data, 0x08))
	packedSize = int(u32(pack.Data, 0x0c))
	if packedSize != pack.Len() {
		return 0, 0, fmt.Errorf("PACK size mismatch: header=%d actual=%d", packedSize, pack.Len())
	}
	return unpackedSize, packedSize, nil
}

func decompressAVG32Pack(pack *binarray.Buffer) (*binarray.Buffer, error) {
	unpackedSize, packedSize, err := avg32PackSizes(pack)
	if err != nil {
		return nil, err
	}
	out, err := decompressAVG32Data(pack.Data[avg32PackHeaderSize:packedSize], unpackedSize)
	if err != nil {
		return nil, err
	}
	return binarray.FromBytes(out), nil
}

// DecompressAVG32SubfileForDisasm returns the uncompressed TPC32 scene data
// from an AVG32 PACK block. It is exported for the kprl command's AVG32
// disassembly path; archive mutation stays in this package.
func DecompressAVG32SubfileForDisasm(pack *binarray.Buffer) ([]byte, error) {
	out, err := decompressAVG32Pack(pack)
	if err != nil {
		return nil, err
	}
	return out.Data, nil
}

func decompressAVG32Data(input []byte, unpackedSize int) ([]byte, error) {
	out := make([]byte, unpackedSize)
	src := 0
	dst := 0

	for dst < unpackedSize {
		if src >= len(input) {
			return nil, fmt.Errorf("compressed data ended at %d/%d bytes", dst, unpackedSize)
		}
		flag := input[src]
		src++

		for bit := 0; bit < 8 && dst < unpackedSize; bit++ {
			if flag&(0x80>>bit) != 0 {
				if src >= len(input) {
					return nil, fmt.Errorf("literal exceeds PACK data")
				}
				out[dst] = input[src]
				src++
				dst++
				continue
			}

			if src+1 >= len(input) {
				return nil, fmt.Errorf("back-reference exceeds PACK data")
			}
			word := int(input[src]) | int(input[src+1])<<8
			src += 2

			length := (word & 0x0f) + 2
			distance := (word >> 4) + 1
			start := dst - distance
			if start < 0 {
				return nil, fmt.Errorf("invalid back-reference distance %d at output %d", distance, dst)
			}
			for i := 0; i < length && dst < unpackedSize; i++ {
				out[dst] = out[start+i]
				dst++
			}
		}
	}

	return out, nil
}

func packAVG32Data(data []byte) ([]byte, error) {
	compressed := compressAVG32Data(data)
	packedSize := len(compressed) + avg32PackHeaderSize
	out := make([]byte, packedSize)
	copy(out[0:], "PACK")
	binary.LittleEndian.PutUint32(out[0x04:], 0)
	binary.LittleEndian.PutUint32(out[0x08:], uint32(len(data)))
	binary.LittleEndian.PutUint32(out[0x0c:], uint32(packedSize))
	copy(out[avg32PackHeaderSize:], compressed)
	return out, nil
}

func compressAVG32Data(data []byte) []byte {
	out := make([]byte, 0, len(data)+len(data)/8+1)
	for i, b := range data {
		if i%8 == 0 {
			out = append(out, 0xff)
		}
		out = append(out, b)
	}
	return out
}

func readAndPackAVG32(fname string, opts Options) ([]byte, error) {
	arr, err := binarray.ReadFile(fname)
	if err != nil {
		return nil, fmt.Errorf("cannot read '%s': %w", fname, err)
	}
	if isAVG32SourcePath(fname) || looksLikeAVG32Source(arr.Data) {
		raw, err := avg32.AssembleFileWithOptions(fname, avg32.Options{
			TextTransform:  opts.TextTransform,
			ForceTransform: opts.ForceTransform,
		})
		if err != nil {
			return nil, fmt.Errorf("cannot assemble AVG32 source '%s': %w", fname, err)
		}
		return packAVG32Data(raw)
	}
	if arr.Len() >= avg32PackHeaderSize && arr.Read(0, 4) == "PACK" {
		if _, _, err := avg32PackSizes(arr); err != nil {
			return nil, err
		}
		return arr.Data, nil
	}
	if arr.Len() < 5 || arr.Read(0, 5) != "TPC32" {
		return nil, fmt.Errorf("AVG32 input '%s' is not a .avg source, PACK block, or TPC32 scene", fname)
	}
	return packAVG32Data(arr.Data)
}

func isAVG32SourcePath(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".avg")
}

func looksLikeAVG32Source(data []byte) bool {
	firstLine := data
	if idx := bytes.IndexAny(data, "\r\n"); idx >= 0 {
		firstLine = data[:idx]
	}
	return strings.EqualFold(strings.TrimSpace(string(firstLine)), "#target AVG32")
}

func rebuildAVG32Arc(existingArc *Archive, arcName string, sources map[int]interface{}, names map[int]string, opts Options) error {
	type outEntry struct {
		idx          int
		name         string
		packed       []byte
		unpackedSize int
		flags        int
	}

	var sortedIndices []int
	for idx := range sources {
		sortedIndices = append(sortedIndices, idx)
	}
	sort.Ints(sortedIndices)

	outEntries := make([]outEntry, 0, len(sortedIndices))
	for _, idx := range sortedIndices {
		source := sources[idx]
		var packed []byte
		flags := 1

		switch s := source.(type) {
		case SeenEntry:
			if existingArc != nil && s.Length > 0 {
				packed = existingArc.Data.Data[s.Offset : s.Offset+s.Length]
				if existingEntries, err := parseAVG32ArchiveEntries(existingArc.Data); err == nil {
					for _, entry := range existingEntries {
						if entry.Index == idx {
							flags = entry.Flags
							break
						}
					}
				}
			}
		case string:
			fileData, err := readAndPackAVG32(s, opts)
			if err != nil {
				return fmt.Errorf("%v", err)
			}
			packed = fileData
		}

		if len(packed) == 0 {
			continue
		}

		unpackedSize, packedSize, err := avg32PackSizes(binarray.FromBytes(packed))
		if err != nil {
			return fmt.Errorf("%s: %w", names[idx], err)
		}
		if packedSize != len(packed) {
			return fmt.Errorf("%s: PACK size mismatch", names[idx])
		}

		name := names[idx]
		if name == "" {
			name = fmt.Sprintf("SEEN%03d.TXT", idx)
		}
		outEntries = append(outEntries, outEntry{
			idx:          idx,
			name:         name,
			packed:       packed,
			unpackedSize: unpackedSize,
			flags:        flags,
		})
	}

	tmpName := arcName + ".tmp"
	oc, err := os.Create(tmpName)
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	defer func() {
		oc.Close()
		os.Remove(tmpName)
	}()

	unk1 := make([]byte, 0x0c)
	unk2 := make([]byte, 0x0c)
	if existingArc != nil && existingArc.Data != nil && existingArc.Data.Len() >= avg32ArchiveHeaderSize && existingArc.Data.Read(0, 4) == "PACL" {
		copy(unk1, existingArc.Data.Data[0x04:0x10])
		copy(unk2, existingArc.Data.Data[0x14:0x20])
	}

	if _, err := oc.Write([]byte("PACL")); err != nil {
		return err
	}
	if _, err := oc.Write(unk1); err != nil {
		return err
	}
	if err := binary.Write(oc, binary.LittleEndian, uint32(len(outEntries))); err != nil {
		return err
	}
	if _, err := oc.Write(unk2); err != nil {
		return err
	}

	offset := avg32ArchiveHeaderSize + len(outEntries)*avg32ArchiveEntrySize
	for _, entry := range outEntries {
		nameBytes := make([]byte, 0x10)
		copy(nameBytes, []byte(entry.name))
		if _, err := oc.Write(nameBytes); err != nil {
			return err
		}
		if err := binary.Write(oc, binary.LittleEndian, uint32(offset)); err != nil {
			return err
		}
		if err := binary.Write(oc, binary.LittleEndian, uint32(len(entry.packed))); err != nil {
			return err
		}
		if err := binary.Write(oc, binary.LittleEndian, uint32(entry.unpackedSize)); err != nil {
			return err
		}
		if err := binary.Write(oc, binary.LittleEndian, uint32(entry.flags)); err != nil {
			return err
		}
		offset += len(entry.packed)
	}

	for _, entry := range outEntries {
		if _, err := oc.Write(entry.packed); err != nil {
			return err
		}
	}

	oc.Close()

	if err := os.Remove(arcName); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot remove old archive: %w", err)
	}
	if err := os.Rename(tmpName, arcName); err != nil {
		return fmt.Errorf("cannot rename temp to archive: %w", err)
	}
	return nil
}

func seenNameFromPath(path string, format ArchiveFormat) string {
	base := filepath.Base(path)
	upper := strings.ToUpper(base)
	if idx := strings.Index(upper, ".TXT"); idx >= 0 {
		base = base[:idx+4]
	}
	if _, ok := avg32SeenIndex(base); ok {
		if format == ArchiveFormatAVG32 {
			return strings.ToUpper(base)
		}
	}
	m := regexp.MustCompile(`(?i)seen0*(\d{1,4})`).FindStringSubmatch(base)
	if m == nil {
		return base
	}
	var idx int
	for _, ch := range m[1] {
		idx = idx*10 + int(ch-'0')
	}
	if format == ArchiveFormatAVG32 {
		return fmt.Sprintf("SEEN%03d.TXT", idx)
	}
	return fmt.Sprintf("SEEN%04d.TXT", idx)
}

func u32(data []byte, off int) uint32 {
	return binary.LittleEndian.Uint32(data[off : off+4])
}
