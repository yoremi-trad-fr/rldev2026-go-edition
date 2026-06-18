// Command vaconv converts RealLive G00 images, NWA audio, and selected DAT assets.
//
// Ported from vaconv (OCaml + C++).
//
// Usage:
//
//	vaconv file.g00              → file.png
//	vaconv -d outdir *.g00       → outdir/file.png (batch)
//	vaconv -i file.png -m file.xml -o file.g00  → PNG+XML to G00
//	vaconv -g 2 -o file.g00 file.png            → force G00 format 2
//	vaconv file.nwa              → file.mp3
//	vaconv mode.cgm              → mode.json
//	vaconv tcdata.tcc            → tcdata.json
//	vaconv mode.json             → mode.cgm or mode.tcc depending on JSON type
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yoremi/rldev-go/vaconv/pkg/datasset"
	"github.com/yoremi/rldev-go/vaconv/pkg/g00"
	"github.com/yoremi/rldev-go/vaconv/pkg/nwaaudio"
)

const (
	appName    = "vaconv"
	appVersion = "2026 (Go port)"
)

func main() {
	verbose := flag.Bool("v", false, "verbose output")
	outdir := flag.String("d", "", "output directory")
	outfile := flag.String("o", "", "output filename")
	inputOrFmt := flag.String("i", "", "input file, or input format (png/g00/nwa/cgm/tcc/json) when a file argument is also supplied")
	g00Fmt := flag.String("f", "auto", "G00 format: 0, 1, 2, or auto")
	g00FmtAlias := flag.String("g", "", "G00 format: 0, 1, 2, or auto (alias for -f)")
	audioFmt := flag.String("audio", "auto", "NWA audio output format: mp3, wav, or auto")
	metafile := flag.String("m", "", "metadata XML file")
	noMeta := flag.Bool("q", false, "disable metadata")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s %s - VisualArt's bitmap format converter\n\n", appName, appVersion)
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  %s [options] <file.g00>           → convert to PNG\n", appName)
		fmt.Fprintf(os.Stderr, "  %s [options] -d DIR *.g00         → batch convert to PNG\n", appName)
		fmt.Fprintf(os.Stderr, "  %s -i file.png -m file.xml -o file.g00 → convert PNG+XML to G00\n", appName)
		fmt.Fprintf(os.Stderr, "  %s -g 2 -o file.g00 file.png      → convert PNG to G00 format 2\n\n", appName)
		fmt.Fprintf(os.Stderr, "  %s [options] <file.nwa>           → convert to MP3 or WAV\n\n", appName)
		fmt.Fprintf(os.Stderr, "  %s [options] <mode.cgm|tcdata.tcc> → export DAT asset to JSON\n", appName)
		fmt.Fprintf(os.Stderr, "  %s [options] <file.json>          → rebuild CGM/TCC from JSON\n\n", appName)
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	files := flag.Args()
	inFmt := *inputOrFmt
	if *g00FmtAlias != "" {
		*g00Fmt = *g00FmtAlias
	}
	if inFmt != "" && (looksLikeInputFile(inFmt) || len(files) == 0) {
		files = append([]string{inFmt}, files...)
		inFmt = ""
	}
	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "%s: no input files\n", appName)
		flag.Usage()
		os.Exit(1)
	}
	var err error
	files, err = expandInputFiles(files, inFmt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", appName, err)
		os.Exit(1)
	}
	if len(files) > 1 && *outfile != "" {
		fmt.Fprintf(os.Stderr, "%s: -o can only be used with one input file\n", appName)
		os.Exit(1)
	}
	if *noMeta && *metafile != "" {
		fmt.Fprintf(os.Stderr, "%s: -m and -q cannot be used together\n", appName)
		os.Exit(1)
	}
	if len(files) > 1 && *metafile != "" && !pathIsDir(*metafile) {
		fmt.Fprintf(os.Stderr, "%s: -m must be a directory when converting multiple files\n", appName)
		os.Exit(1)
	}

	for _, f := range files {
		if err := convert(f, *verbose, *outdir, *outfile, inFmt, *g00Fmt, *audioFmt, *metafile, *noMeta); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", f, err)
			os.Exit(1)
		}
	}
}

func convert(file string, verbose bool, outdir, outfile, inFmt, g00Fmt, audioFmt, metafile string, noMeta bool) error {
	ext := strings.ToLower(filepath.Ext(file))
	base := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))

	// Detect direction
	isG00 := ext == ".g00"
	isPNG := ext == ".png"
	isNWA := ext == ".nwa"
	isCGM := ext == ".cgm"
	isTCC := ext == ".tcc"
	isDATJSON := ext == ".json"

	if inFmt != "" {
		switch strings.ToLower(inFmt) {
		case "g00":
			isG00 = true
			isPNG = false
			isNWA = false
			isCGM = false
			isTCC = false
			isDATJSON = false
		case "png":
			isPNG = true
			isG00 = false
			isNWA = false
			isCGM = false
			isTCC = false
			isDATJSON = false
		case "nwa":
			isNWA = true
			isG00 = false
			isPNG = false
			isCGM = false
			isTCC = false
			isDATJSON = false
		case "cgm", "cgtable":
			isCGM = true
			isG00 = false
			isPNG = false
			isNWA = false
			isTCC = false
			isDATJSON = false
		case "tcc", "tonecurve":
			isTCC = true
			isG00 = false
			isPNG = false
			isNWA = false
			isCGM = false
			isDATJSON = false
		case "json", "dat-json":
			isDATJSON = true
			isG00 = false
			isPNG = false
			isNWA = false
			isCGM = false
			isTCC = false
		}
	}

	if isG00 {
		return g00ToPNG(file, base, outdir, outfile, metafile, noMeta, verbose)
	} else if isPNG {
		return pngToG00(file, base, outdir, outfile, metafile, noMeta, verbose, g00Fmt)
	} else if isNWA {
		return nwaToAudio(file, base, outdir, outfile, audioFmt, verbose)
	} else if isCGM || isTCC {
		return datToJSON(file, base, outdir, outfile, verbose)
	} else if isDATJSON {
		return datJSONToBinary(file, base, outdir, outfile, verbose)
	}
	return fmt.Errorf("unknown file type '%s' (expected .g00, .png, .nwa, .cgm, .tcc, or .json)", ext)
}

func g00ToPNG(inFile, base, outdir, outfile, metafile string, noMeta bool, verbose bool) error {
	if verbose {
		fmt.Fprintf(os.Stderr, "Reading G00: %s\n", inFile)
	}

	img, err := g00.ReadFile(inFile)
	if err != nil {
		return err
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "  Format: %d, Size: %dx%d\n", img.Format, img.Width, img.Height)
		if len(img.Regions) > 0 {
			fmt.Fprintf(os.Stderr, "  Regions: %d\n", len(img.Regions))
		}
	}

	out := resolveOutput(base, ".png", outdir, outfile)
	if err := ensureOutputDir(out); err != nil {
		return err
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "Writing PNG: %s\n", out)
	}
	if err := g00.ToPNGFile(img, out); err != nil {
		return err
	}
	if !noMeta && img.Format == 2 {
		metaOut := resolveMetadataOutput(base, outdir, metafile)
		if err := ensureOutputDir(metaOut); err != nil {
			return err
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "Writing XML metadata: %s\n", metaOut)
		}
		if err := g00.WriteMetadataFile(metaOut, img); err != nil {
			return err
		}
	}
	return nil
}

func pngToG00(inFile, base, outdir, outfile, metafile string, noMeta bool, verbose bool, g00Fmt string) error {
	if verbose {
		fmt.Fprintf(os.Stderr, "Reading PNG: %s\n", inFile)
	}

	img, err := g00.FromPNGFile(inFile)
	if err != nil {
		return err
	}

	if !noMeta {
		metaIn := resolveMetadataInput(base, inFile, metafile)
		if metaIn != "" {
			format, regions, err := g00.ReadMetadataFile(metaIn)
			if err != nil {
				return err
			}
			if format != 0 {
				img.Format = format
			}
			if len(regions) > 0 {
				img.Regions = regions
				if img.Format == 0 {
					img.Format = 2
				}
			}
			if verbose {
				fmt.Fprintf(os.Stderr, "  XML metadata: %s (%d region(s))\n", metaIn, len(regions))
			}
		}
	}

	switch strings.ToLower(g00Fmt) {
	case "", "auto":
		// Keep format from metadata, otherwise default to format 0 for compatibility.
	case "0":
		img.Format = 0
	case "1":
		img.Format = 1
	case "2":
		img.Format = 2
	default:
		return fmt.Errorf("unknown G00 format %q", g00Fmt)
	}
	if img.Format == 2 && len(img.Regions) == 0 {
		img.Regions = []g00.Region{{X1: 0, Y1: 0, X2: img.Width - 1, Y2: img.Height - 1}}
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "  Size: %dx%d, Output format: %d\n", img.Width, img.Height, img.Format)
	}

	out := resolveOutput(base, ".g00", outdir, outfile)
	if err := ensureOutputDir(out); err != nil {
		return err
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "Writing G00: %s\n", out)
	}
	return g00.WriteFile(out, img)
}

func nwaToAudio(inFile, base, outdir, outfile, audioFmt string, verbose bool) error {
	format, err := resolveAudioFormat(audioFmt, outfile)
	if err != nil {
		return err
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Reading NWA: %s\n", inFile)
	}

	out := resolveOutput(base, "."+format, outdir, outfile)
	if err := ensureOutputDir(out); err != nil {
		return err
	}

	info, err := nwaaudio.ConvertFile(inFile, out, format)
	if err != nil {
		return err
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "  Format: %d Hz, %d channel(s), %d-bit\n", info.Frequency, info.Channels, info.BitsPerSample)
		fmt.Fprintf(os.Stderr, "Writing %s: %s\n", strings.ToUpper(format), out)
	}
	return nil
}

func datToJSON(inFile, base, outdir, outfile string, verbose bool) error {
	if verbose {
		fmt.Fprintf(os.Stderr, "Reading DAT asset: %s\n", inFile)
	}

	out := resolveOutput(base, ".json", outdir, outfile)
	if err := ensureOutputDir(out); err != nil {
		return err
	}
	if err := datasset.WriteJSONFile(inFile, out); err != nil {
		return err
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "Writing JSON: %s\n", out)
	}
	return nil
}

func datJSONToBinary(inFile, base, outdir, outfile string, verbose bool) error {
	ext, err := datasset.BinaryExtForJSONFile(inFile)
	if err != nil {
		return err
	}
	base = trimBaseSuffix(base, ext)
	out := resolveOutput(base, ext, outdir, outfile)
	if err := ensureOutputDir(out); err != nil {
		return err
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Reading DAT JSON: %s\n", inFile)
	}
	writtenExt, err := datasset.WriteBinaryFromJSONFile(inFile, out)
	if err != nil {
		return err
	}
	if verbose {
		label := strings.TrimPrefix(strings.ToUpper(writtenExt), ".")
		fmt.Fprintf(os.Stderr, "Writing %s: %s\n", label, out)
	}
	return nil
}

func trimBaseSuffix(base, ext string) string {
	if strings.HasSuffix(strings.ToLower(base), strings.ToLower(ext)) {
		return base[:len(base)-len(ext)]
	}
	return base
}

func resolveAudioFormat(audioFmt, outfile string) (string, error) {
	format := strings.ToLower(strings.TrimSpace(audioFmt))
	if format == "" || format == "auto" {
		switch strings.ToLower(filepath.Ext(outfile)) {
		case ".wav":
			format = "wav"
		case ".mp3":
			format = "mp3"
		default:
			format = "mp3"
		}
	}
	switch format {
	case "mp3", "wav":
		return format, nil
	default:
		return "", fmt.Errorf("unknown NWA audio format %q (expected mp3, wav, or auto)", audioFmt)
	}
}

func resolveOutput(base, ext, outdir, outfile string) string {
	if outfile != "" {
		return outfile
	}
	name := base + ext
	if outdir != "" {
		return filepath.Join(outdir, name)
	}
	return name
}

func resolveMetadataOutput(base, outdir, metafile string) string {
	if metafile != "" {
		if pathIsDir(metafile) {
			return filepath.Join(metafile, base+".xml")
		}
		return metafile
	}
	if outdir != "" {
		return filepath.Join(outdir, base+".xml")
	}
	return base + ".xml"
}

func resolveMetadataInput(base, inFile, metafile string) string {
	if metafile != "" {
		if pathIsDir(metafile) {
			p := filepath.Join(metafile, base+".xml")
			if fileExists(p) {
				return p
			}
			return ""
		}
		return metafile
	}
	p := filepath.Join(filepath.Dir(inFile), base+".xml")
	if fileExists(p) {
		return p
	}
	return ""
}

func ensureOutputDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0755)
}

func expandInputFiles(files []string, inFmt string) ([]string, error) {
	expanded := make([]string, 0, len(files))
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil || !info.IsDir() {
			expanded = append(expanded, file)
			continue
		}

		before := len(expanded)
		entries, err := os.ReadDir(file)
		if err != nil {
			return nil, fmt.Errorf("read input directory %s: %w", file, err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if isInputExt(filepath.Ext(entry.Name()), inFmt) {
				expanded = append(expanded, filepath.Join(file, entry.Name()))
			}
		}
		if len(expanded) == before {
			return nil, fmt.Errorf("no supported input files found in %s (expected %s)", file, expectedInputExts(inFmt))
		}
	}
	return expanded, nil
}

func isInputExt(ext, inFmt string) bool {
	ext = strings.ToLower(ext)
	for _, candidate := range inputExtsForFormat(inFmt) {
		if ext == candidate {
			return true
		}
	}
	return false
}

func expectedInputExts(inFmt string) string {
	return strings.Join(inputExtsForFormat(inFmt), ", ")
}

func inputExtsForFormat(inFmt string) []string {
	switch strings.ToLower(strings.TrimSpace(inFmt)) {
	case "g00":
		return []string{".g00"}
	case "png":
		return []string{".png"}
	case "nwa":
		return []string{".nwa"}
	case "cgm", "cgtable":
		return []string{".cgm"}
	case "tcc", "tonecurve":
		return []string{".tcc"}
	case "json", "dat-json":
		return []string{".json"}
	case "dat", "datasset":
		return []string{".cgm", ".tcc"}
	default:
		return []string{".g00", ".png", ".nwa", ".cgm", ".tcc", ".json"}
	}
}

func looksLikeInputFile(value string) bool {
	ext := strings.ToLower(filepath.Ext(value))
	return ext == ".png" || ext == ".g00" || ext == ".nwa" || ext == ".cgm" || ext == ".tcc" || ext == ".json" || fileExists(value)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func pathIsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
