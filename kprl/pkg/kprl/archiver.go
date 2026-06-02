// Package kprl implements the SEEN.TXT archive format used by the RealLive engine.
// Transposed from OCaml's kprl/archiver.ml.
//
// SEEN.TXT archive format:
//   - 10000 entry index table at offset 0 (80000 bytes)
//   - Each entry: 4 bytes offset + 4 bytes length (LE)
//   - Entry i corresponds to SEEN{i:04d}.TXT
//   - Offset 0 + length 0 = empty slot
//   - Actual file data follows the index table
package kprl

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/yoremi/rldev-go/pkg/binarray"
	"github.com/yoremi/rldev-go/pkg/bytecode"
	"github.com/yoremi/rldev-go/pkg/diag"
	"github.com/yoremi/rldev-go/pkg/gamedef"
	"github.com/yoremi/rldev-go/pkg/rlcmp"
	"github.com/yoremi/rldev-go/pkg/texttransforms"
)

const (
	// MaxSeens is the number of SEEN file slots in an archive.
	MaxSeens = 10000
	// IndexSize is the total size of the index table in bytes.
	IndexSize = MaxSeens * 8 // 80000 bytes
	// CompExt is the extension for compressed extracted files.
	CompExt = "rlc"
	// Avg32CompExt is the extension for compressed AVG32 PACK blocks.
	Avg32CompExt = "pack"
	// UncompExt is the extension for uncompressed extracted files.
	UncompExt = "rl"
)

var emptyArcMagic = "\x00Empty RealLive archive"

// ArchiveFormat identifies the outer container format.
type ArchiveFormat int

const (
	ArchiveFormatRealLive ArchiveFormat = iota
	ArchiveFormatAVG32
)

func (f ArchiveFormat) String() string {
	switch f {
	case ArchiveFormatAVG32:
		return "AVG32"
	default:
		return "RealLive"
	}
}

// SeenEntry holds the offset and length of a file within the archive.
type SeenEntry struct {
	Offset int
	Length int
}

// Archive represents a loaded SEEN.TXT archive.
type Archive struct {
	Data    *binarray.Buffer
	Format  ArchiveFormat
	Entries [MaxSeens]SeenEntry
	Names   [MaxSeens]string
	Order   []int
	Count   int // Number of non-empty entries
}

// Options controls kprl operation behavior.
type Options struct {
	Verbose         int
	OutDir          string
	GameID          string
	Keys            []gamedef.XORSubkey
	TemplateArchive string
	TextTransform   texttransforms.EncMode
	ForceTransform  bool
}

// --- Archive detection and loading ---

// getSubfileInfo returns the offset and length for entry idx in the archive.
func getSubfileInfo(arc *binarray.Buffer, idx int) SeenEntry {
	if arc.Len() <= 23 {
		return SeenEntry{} // empty archive
	}
	off := idx * 8
	return SeenEntry{
		Offset: int(arc.GetInt(off)),
		Length: int(arc.GetInt(off + 4)),
	}
}

// GetSubfile returns the data for entry idx, or nil if empty.
func GetSubfile(arc *binarray.Buffer, idx int) *binarray.Buffer {
	entry := getSubfileInfo(arc, idx)
	if entry.Length == 0 {
		return nil
	}
	return arc.Sub(entry.Offset, entry.Length)
}

// EntryName returns the archive-stored filename for idx, or the default name
// for the archive format when no explicit name exists.
func (arc *Archive) EntryName(idx int) string {
	if idx >= 0 && idx < MaxSeens && arc.Names[idx] != "" {
		return arc.Names[idx]
	}
	if arc.Format == ArchiveFormatAVG32 {
		return fmt.Sprintf("SEEN%03d.TXT", idx)
	}
	return fmt.Sprintf("SEEN%04d.TXT", idx)
}

// Subfile returns the raw archived block for idx.
func (arc *Archive) Subfile(idx int) *binarray.Buffer {
	if idx < 0 || idx >= MaxSeens {
		return nil
	}
	entry := arc.Entries[idx]
	if entry.Length == 0 {
		return nil
	}
	return arc.Data.Sub(entry.Offset, entry.Length)
}

func (arc *Archive) selectedIndices(ranges []int) []int {
	if ranges != nil {
		return resolveRanges(ranges)
	}
	if arc.Format == ArchiveFormatAVG32 && len(arc.Order) > 0 {
		out := append([]int(nil), arc.Order...)
		sort.Ints(out)
		return out
	}
	return resolveRanges(nil)
}

// SeenCount checks if the buffer looks like a SEEN.TXT archive and returns
// the number of valid entries. Returns -1 if not an archive.
// Equivalent to OCaml's seen_count.
func SeenCount(arr *binarray.Buffer) int {
	// Check for empty archive marker
	if arr.Len() >= 23 && arr.Read(0, 23) == emptyArcMagic {
		return 0
	}

	// Archive must be at least IndexSize bytes
	if arr.Len() < IndexSize {
		return -1
	}

	count := 0
	for i := 0; i < MaxSeens; i++ {
		entry := getSubfileInfo(arr, i)
		if entry.Length == 0 {
			continue
		}
		// Validate: offset must be past index, and data must fit
		if entry.Offset+entry.Length > IndexSize &&
			entry.Offset+entry.Length <= arr.Len() &&
			bytecode.IsBytecode(arr, entry.Offset) {
			count++
		} else {
			// Invalid entry found
			if count > 0 {
				return -count // partial archive
			}
			return -1
		}
	}

	return count
}

// IsArchive checks if the file at the given path is a SEEN.TXT archive.
func IsArchive(fname string) bool {
	data, err := binarray.ReadFile(fname)
	if err != nil {
		return false
	}
	if SeenCount(data) >= 0 {
		return true
	}
	return avg32ArchiveCount(data) >= 0
}

// LoadArchive loads a SEEN.TXT archive from file.
func LoadArchive(fname string) (*Archive, error) {
	data, err := binarray.ReadFile(fname)
	if err != nil {
		return nil, fmt.Errorf("cannot read archive '%s': %w", fname, err)
	}

	count := SeenCount(data)
	if count >= 0 {
		arc := &Archive{Data: data, Count: count, Format: ArchiveFormatRealLive}
		for i := 0; i < MaxSeens; i++ {
			arc.Entries[i] = getSubfileInfo(data, i)
		}
		return arc, nil
	}

	arc, err := loadAVG32Archive(data)
	if err == nil {
		return arc, nil
	}

	return nil, fmt.Errorf("%s is not a valid RealLive or AVG32 archive", filepath.Base(fname))
}

// --- Core operations ---

// List prints the contents of the archive.
// Equivalent to OCaml's Archiver.list.
func List(fname string, ranges []int, opts Options) error {
	arc, err := LoadArchive(fname)
	if err != nil {
		return err
	}

	indices := arc.selectedIndices(ranges)

	for _, i := range indices {
		entry := arc.Entries[i]
		if entry.Length == 0 {
			continue
		}

		sub := arc.Subfile(i)
		if sub == nil {
			continue
		}

		if arc.Format == ArchiveFormatAVG32 {
			unc, cmp, err := avg32PackSizes(sub)
			if err != nil {
				diag.SysWarning("%s: cannot read PACK header: %v", arc.EntryName(i), err)
				continue
			}
			ratio := float64(cmp) / float64(unc) * 100.0
			fmt.Printf("%s: %10.2f k -> %10.2f k   (%.2f%%)\n", arc.EntryName(i), float64(unc)/1024.0, float64(cmp)/1024.0, ratio)
			continue
		}

		hdr, err := bytecode.ReadFullHeader(sub, true)
		if err != nil {
			// Was printed on stdout, mixing data with the archive
			// listing. Route via diag so it lands on stderr like
			// every other diagnostic and counts towards the summary.
			diag.SysWarning("SEEN%04d.TXT: cannot read header: %v", i, err)
			continue
		}

		unc := float64(hdr.UncompressedSize+hdr.DataOffset) / 1024.0
		if hdr.IsCompressed {
			cmp := float64(hdr.CompressedSize+hdr.DataOffset) / 1024.0
			ratio := cmp / unc * 100.0
			fmt.Printf("SEEN%04d.TXT: %10.2f k -> %10.2f k   (%.2f%%)\n", i, unc, cmp, ratio)
		} else {
			fmt.Printf("SEEN%04d.TXT: %10.2f k\n", i, unc)
		}
	}
	return nil
}

// Break extracts individual (still compressed) files from the archive.
// Equivalent to OCaml's Archiver.break.
func Break(fname string, ranges []int, opts Options) error {
	arc, err := LoadArchive(fname)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(opts.OutDir, 0755); err != nil {
		return fmt.Errorf("cannot create output directory: %w", err)
	}

	indices := arc.selectedIndices(ranges)

	for _, i := range indices {
		sub := arc.Subfile(i)
		if sub == nil {
			continue
		}

		ext := CompExt
		if arc.Format == ArchiveFormatAVG32 {
			ext = Avg32CompExt
		}
		outName := fmt.Sprintf("%s.%s", arc.EntryName(i), ext)
		outPath := filepath.Join(opts.OutDir, outName)

		if opts.Verbose > 0 {
			fmt.Printf("Extracting %s to %s\n", arc.EntryName(i), outName)
		}

		if err := sub.WriteFile(outPath); err != nil {
			return fmt.Errorf("failed to write %s: %w", outPath, err)
		}
	}
	return nil
}

// Extract decompresses individual files from the archive.
// Equivalent to OCaml's Archiver.extract.
func Extract(fname string, ranges []int, opts Options) error {
	arc, err := LoadArchive(fname)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(opts.OutDir, 0755); err != nil {
		return fmt.Errorf("cannot create output directory: %w", err)
	}

	indices := arc.selectedIndices(ranges)

	for _, i := range indices {
		sub := arc.Subfile(i)
		if sub == nil {
			continue
		}

		outName := fmt.Sprintf("%s.%s", arc.EntryName(i), UncompExt)
		outPath := filepath.Join(opts.OutDir, outName)

		if arc.Format == ArchiveFormatAVG32 {
			if opts.Verbose > 0 {
				fmt.Printf("Decompressing %s to %s\n", arc.EntryName(i), outName)
			}
			decompressed, err := decompressAVG32Pack(sub)
			if err != nil {
				fmt.Printf("Warning: failed to decompress %s: %v\n", arc.EntryName(i), err)
				continue
			}
			if err := decompressed.WriteFile(outPath); err != nil {
				return fmt.Errorf("failed to write %s: %w", outPath, err)
			}
			continue
		}

		// Check if already uncompressed
		hdr, hdrErr := bytecode.ReadFileHeader(sub, true)
		if hdrErr != nil || !hdr.IsCompressed {
			if opts.Verbose > 0 {
				fmt.Printf("Ignoring SEEN%04d.TXT (not compressed)\n", i)
			}
			continue
		}

		if opts.Verbose > 0 {
			fmt.Printf("Decompressing SEEN%04d.TXT to %s\n", i, outName)
		}

		decompressed, err := rlcmp.Decompress(binarray.Copy(sub), opts.Keys, true)
		if err != nil {
			fmt.Printf("Warning: failed to decompress SEEN%04d.TXT: %v\n", i, err)
			continue
		}

		// Write uncompressed header magic
		ucMagic := getUncompressedMagic(hdr)
		decompressed.Write(0, ucMagic)

		if err := decompressed.WriteFile(outPath); err != nil {
			return fmt.Errorf("failed to write %s: %w", outPath, err)
		}
	}
	return nil
}

// Pack compresses uncompressed bytecode files.
// Equivalent to OCaml's Archiver.pack.
func Pack(files []string, opts Options) error {
	if err := os.MkdirAll(opts.OutDir, 0755); err != nil {
		return fmt.Errorf("cannot create output directory: %w", err)
	}

	for _, fname := range files {
		arr, err := binarray.ReadFile(fname)
		if err != nil {
			fmt.Printf("Warning: cannot read %s: %v\n", fname, err)
			continue
		}

		if arr.Len() < 4 || !bytecode.UncompressedHeader(arr.Read(0, 4)) {
			fmt.Printf("Skipping %s: not an uncompressed bytecode file\n", filepath.Base(fname))
			continue
		}

		// Determine output name
		base := filepath.Base(fname)
		outName := base
		if strings.HasSuffix(base, ".uncompressed") {
			outName = strings.TrimSuffix(base, ".uncompressed")
		} else if strings.HasSuffix(base, "."+UncompExt) {
			outName = strings.TrimSuffix(base, "."+UncompExt)
		}
		outPath := filepath.Join(opts.OutDir, outName)

		if opts.Verbose > 0 {
			fmt.Printf("Compressing %s to %s\n", fname, outName)
		}

		compressed, err := rlcmp.Compress(arr, opts.Keys)
		if err != nil {
			fmt.Printf("Warning: failed to compress %s: %v\n", fname, err)
			continue
		}

		if err := compressed.WriteFile(outPath); err != nil {
			return fmt.Errorf("failed to write %s: %w", outPath, err)
		}
	}
	return nil
}

// Add adds bytecode files to an archive, creating it if needed.
// Files must be named SEENxxxx.TXT where xxxx is 0000-9999.
// Equivalent to OCaml's Archiver.add.
func Add(arcName string, files []string, opts Options) error {
	if len(files) == 0 {
		return fmt.Errorf("no files to process")
	}

	// Load or create archive
	var arcData *binarray.Buffer
	var loadedArc *Archive
	var tailSource *binarray.Buffer
	targetFormat := ArchiveFormatRealLive
	formatLocked := false
	existing := make(map[int]SeenEntry)
	names := make(map[int]string)

	if fileExists(arcName) {
		arc, err := LoadArchive(arcName)
		if err != nil {
			return fmt.Errorf("cannot read archive: %w", err)
		}
		loadedArc = arc
		arcData = arc.Data
		tailSource = arc.Data
		targetFormat = arc.Format
		formatLocked = true
		if arc.Count > 0 {
			collectExistingEntries(arc, existing, names)
		}
	} else if opts.TemplateArchive != "" {
		arc, err := LoadArchive(opts.TemplateArchive)
		if err != nil {
			return fmt.Errorf("cannot read template archive: %w", err)
		}
		loadedArc = arc
		arcData = arc.Data
		tailSource = arc.Data
		targetFormat = arc.Format
		formatLocked = true
		if arc.Count > 0 {
			collectExistingEntries(arc, existing, names)
		}
	} else {
		// Create empty archive
		arcData = binarray.New(0)
		f, err := os.Create(arcName)
		if err != nil {
			return fmt.Errorf("cannot create archive: %w", err)
		}
		f.Write([]byte(emptyArcMagic))
		f.Close()
	}

	// Parse SEEN indices from filenames and prepare sources
	seenRe := regexp.MustCompile(`(?i)seen0*(\d{1,4})`)
	sources := make(map[int]interface{}) // int -> SeenEntry (keep) or string (file)

	// Start with existing entries
	for idx, entry := range existing {
		sources[idx] = entry
	}

	// Override with new files
	anyAdded := false
	sawAVG32Source := false
	for _, fname := range files {
		if !fileExists(fname) {
			fmt.Printf("Warning: file not found: %s\n", fname)
			continue
		}
		if isAVG32SourcePath(fname) {
			sawAVG32Source = true
		}
		match := seenRe.FindStringSubmatch(filepath.Base(fname))
		if match == nil {
			fmt.Printf("Warning: unable to add '%s': name must contain SEENxxxx (0000-9999)\n", fname)
			continue
		}
		idx, _ := strconv.Atoi(match[1])
		sources[idx] = fname
		names[idx] = seenNameFromPath(fname, targetFormat)
		anyAdded = true
	}

	if !anyAdded {
		return fmt.Errorf("no files to process")
	}

	if sawAVG32Source && targetFormat != ArchiveFormatAVG32 {
		if formatLocked {
			return fmt.Errorf(".avg sources require an AVG32 archive or AVG32 template")
		}
		targetFormat = ArchiveFormatAVG32
		for idx, source := range sources {
			if fname, ok := source.(string); ok {
				names[idx] = seenNameFromPath(fname, targetFormat)
			}
		}
	}

	if opts.TemplateArchive != "" && targetFormat == ArchiveFormatRealLive {
		arc, err := LoadArchive(opts.TemplateArchive)
		if err != nil {
			return fmt.Errorf("cannot read template archive: %w", err)
		}
		loadedArc = arc
		tailSource = arc.Data
	}

	if targetFormat == ArchiveFormatAVG32 {
		return rebuildAVG32Arc(loadedArc, arcName, sources, names, opts)
	}
	return rebuildArc(arcData, arcName, sources, opts, tailSource)
}

// Remove removes entries from an archive.
// Equivalent to OCaml's Archiver.remove.
func Remove(arcName string, ranges []int, opts Options) error {
	arc, err := LoadArchive(arcName)
	if err != nil {
		return err
	}

	toRemove := make(map[int]bool)
	indices := resolveRanges(ranges)
	for _, i := range indices {
		toRemove[i] = true
	}

	sources := make(map[int]interface{})
	names := make(map[int]string)
	anyRemoved := false
	anyRemain := false

	for i := 0; i < MaxSeens; i++ {
		entry := arc.Entries[i]
		if entry.Length == 0 {
			continue
		}
		if toRemove[i] {
			anyRemoved = true
		} else {
			anyRemain = true
			sources[i] = entry
			names[i] = arc.EntryName(i)
		}
	}

	if !anyRemoved {
		fmt.Println("No files to remove.")
		return nil
	}

	if !anyRemain {
		fmt.Println("Warning: all archive contents removed")
		return writeEmptyArc(arcName)
	}

	if arc.Format == ArchiveFormatAVG32 {
		return rebuildAVG32Arc(arc, arcName, sources, names, opts)
	}
	return rebuildArc(arc.Data, arcName, sources, opts, arc.Data)
}

// --- Internal helpers ---

// rebuildArc reconstructs the archive file from sources.
// sources maps SEEN index -> SeenEntry (keep from existing) or string (read from file).
func rebuildArc(arc *binarray.Buffer, arcName string, sources map[int]interface{}, opts Options, tailSource *binarray.Buffer) error {
	// Create temp file
	tmpName := arcName + ".tmp"
	oc, err := os.Create(tmpName)
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}

	defer func() {
		oc.Close()
		os.Remove(tmpName)
	}()

	// Reserve space for index table
	indexBuf := make([]byte, IndexSize)
	if _, err := oc.Write(indexBuf); err != nil {
		return fmt.Errorf("failed to write index: %w", err)
	}

	// Sort indices for deterministic output
	var sortedIndices []int
	for idx := range sources {
		sortedIndices = append(sortedIndices, idx)
	}
	sort.Ints(sortedIndices)

	// Write data and track offsets
	offsets := make(map[int]SeenEntry)
	currentOffset := IndexSize

	for _, idx := range sortedIndices {
		source := sources[idx]
		var data []byte

		switch s := source.(type) {
		case SeenEntry:
			// Keep existing data from archive
			if arc != nil && s.Length > 0 {
				data = arc.Data[s.Offset : s.Offset+s.Length]
			}
		case string:
			// Read and compress file
			fileData, err := readAndCompress(s, opts)
			if err != nil {
				fmt.Printf("Warning: %v\n", err)
				continue
			}
			data = fileData
		}

		if len(data) == 0 {
			continue
		}

		n, err := oc.Write(data)
		if err != nil {
			return fmt.Errorf("failed to write SEEN%04d: %w", idx, err)
		}

		offsets[idx] = SeenEntry{Offset: currentOffset, Length: n}
		currentOffset += n
	}

	if tail := archiveTrailingData(tailSource); len(tail) > 0 {
		if opts.Verbose > 0 {
			fmt.Printf("Preserving %d trailing byte(s) from template archive\n", len(tail))
		}
		n, err := oc.Write(tail)
		if err != nil {
			return fmt.Errorf("failed to write archive trailer: %w", err)
		}
		currentOffset += n
	}

	// Write index table
	for i := 0; i < MaxSeens; i++ {
		entry := offsets[i]
		binary.LittleEndian.PutUint32(indexBuf[i*8:], uint32(entry.Offset))
		binary.LittleEndian.PutUint32(indexBuf[i*8+4:], uint32(entry.Length))
	}

	if _, err := oc.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek: %w", err)
	}
	if _, err := oc.Write(indexBuf); err != nil {
		return fmt.Errorf("failed to write index: %w", err)
	}

	oc.Close()

	// Atomic replace
	if err := os.Remove(arcName); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot remove old archive: %w", err)
	}
	if err := os.Rename(tmpName, arcName); err != nil {
		return fmt.Errorf("cannot rename temp to archive: %w", err)
	}

	return nil
}

func collectExistingEntries(arc *Archive, existing map[int]SeenEntry, names map[int]string) {
	for i := 0; i < MaxSeens; i++ {
		entry := arc.Entries[i]
		if entry.Length > 0 {
			existing[i] = entry
			names[i] = arc.EntryName(i)
		}
	}
}

func archiveDataEnd(arc *binarray.Buffer) int {
	if arc == nil || arc.Len() < IndexSize {
		return -1
	}
	end := IndexSize
	for i := 0; i < MaxSeens; i++ {
		entry := getSubfileInfo(arc, i)
		if entry.Length <= 0 {
			continue
		}
		entryEnd := entry.Offset + entry.Length
		if entry.Offset >= IndexSize && entryEnd <= arc.Len() && entryEnd > end {
			end = entryEnd
		}
	}
	return end
}

func archiveTrailingData(arc *binarray.Buffer) []byte {
	end := archiveDataEnd(arc)
	if end < 0 || end >= arc.Len() {
		return nil
	}
	return arc.Data[end:]
}

// readAndCompress reads a bytecode file and compresses it if needed.
func readAndCompress(fname string, opts Options) ([]byte, error) {
	arr, err := binarray.ReadFile(fname)
	if err != nil {
		return nil, fmt.Errorf("cannot read '%s': %w", fname, err)
	}

	if !bytecode.IsBytecode(arr, 0) {
		return nil, fmt.Errorf("unable to add '%s': not a bytecode file", fname)
	}

	// If already compressed, use as-is
	if arr.Len() >= 4 && !bytecode.UncompressedHeader(arr.Read(0, 4)) {
		return arr.Data, nil
	}

	// Compress
	compressed, err := rlcmp.Compress(arr, opts.Keys)
	if err != nil {
		return nil, fmt.Errorf("failed to compress '%s': %w", fname, err)
	}
	return compressed.Data, nil
}

// writeEmptyArc writes an empty archive (all zero index + empty marker).
func writeEmptyArc(arcName string) error {
	f, err := os.Create(arcName)
	if err != nil {
		return err
	}
	defer f.Close()

	// Write 10000 empty entries (all zeros)
	buf := make([]byte, IndexSize)
	_, err = f.Write(buf)
	return err
}

// resolveRanges converts range specs to a sorted list of indices.
// Empty input = all 0-9999.
func resolveRanges(ranges []int) []int {
	if len(ranges) == 0 {
		result := make([]int, MaxSeens)
		for i := range result {
			result[i] = i
		}
		return result
	}
	sort.Ints(ranges)
	return ranges
}

// ParseRanges parses range strings like "50", "100-150", "0-9999" into indices.
func ParseRanges(args []string) ([]int, error) {
	if len(args) == 0 {
		return nil, nil // means "all"
	}

	var result []int
	rangeRe := regexp.MustCompile(`^(\d+)[-~.](\d+)$`)
	negRangeRe := regexp.MustCompile(`^!(\d+)[-~.](\d+)$`)
	negRe := regexp.MustCompile(`^!(\d+)$`)

	excluded := make(map[int]bool)

	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}

		if m := negRangeRe.FindStringSubmatch(arg); m != nil {
			start, _ := strconv.Atoi(m[1])
			end, _ := strconv.Atoi(m[2])
			for i := start; i <= end && i < MaxSeens; i++ {
				excluded[i] = true
			}
		} else if m := negRe.FindStringSubmatch(arg); m != nil {
			idx, _ := strconv.Atoi(m[1])
			excluded[idx] = true
		} else if m := rangeRe.FindStringSubmatch(arg); m != nil {
			start, _ := strconv.Atoi(m[1])
			end, _ := strconv.Atoi(m[2])
			for i := start; i <= end && i < MaxSeens; i++ {
				result = append(result, i)
			}
		} else if idx, err := strconv.Atoi(arg); err == nil {
			if idx >= 0 && idx < MaxSeens {
				result = append(result, idx)
			}
		} else {
			return nil, fmt.Errorf("malformed range parameter: %s", arg)
		}
	}

	// If only exclusions, start with full range
	if len(result) == 0 && len(excluded) > 0 {
		for i := 0; i < MaxSeens; i++ {
			if !excluded[i] {
				result = append(result, i)
			}
		}
	} else if len(excluded) > 0 {
		// Filter out excluded
		var filtered []int
		for _, i := range result {
			if !excluded[i] {
				filtered = append(filtered, i)
			}
		}
		result = filtered
	}

	sort.Ints(result)
	return result, nil
}

// getUncompressedMagic returns the 4-byte magic for an uncompressed file header.
func getUncompressedMagic(hdr bytecode.FileHeader) string {
	if hdr.HeaderVersion == 1 {
		return "KP2K"
	}
	if hdr.CompilerVersion == 110002 {
		return "KPRM"
	}
	return "KPRL"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
