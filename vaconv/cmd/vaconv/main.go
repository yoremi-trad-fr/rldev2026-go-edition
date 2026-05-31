// Command vaconv converts RealLive G00 images to/from PNG.
//
// Ported from vaconv (OCaml + C++).
//
// Usage:
//
//	vaconv file.g00              → file.png
//	vaconv -d outdir *.g00       → outdir/file.png (batch)
//	vaconv -i file.png -m file.xml -o file.g00  → PNG+XML to G00
//	vaconv -g 2 -o file.g00 file.png            → force G00 format 2
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yoremi/rldev-go/vaconv/pkg/g00"
)

const (
	appName    = "vaconv"
	appVersion = "2026 (Go port)"
)

func main() {
	verbose := flag.Bool("v", false, "verbose output")
	outdir := flag.String("d", "", "output directory")
	outfile := flag.String("o", "", "output filename")
	inputOrFmt := flag.String("i", "", "input file, or input format (png/g00) when a file argument is also supplied")
	g00Fmt := flag.String("f", "auto", "G00 format: 0, 1, 2, or auto")
	g00FmtAlias := flag.String("g", "", "G00 format: 0, 1, 2, or auto (alias for -f)")
	metafile := flag.String("m", "", "metadata XML file")
	noMeta := flag.Bool("q", false, "disable metadata")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s %s - VisualArt's bitmap format converter\n\n", appName, appVersion)
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  %s [options] <file.g00>           → convert to PNG\n", appName)
		fmt.Fprintf(os.Stderr, "  %s [options] -d DIR *.g00         → batch convert to PNG\n", appName)
		fmt.Fprintf(os.Stderr, "  %s -i file.png -m file.xml -o file.g00 → convert PNG+XML to G00\n", appName)
		fmt.Fprintf(os.Stderr, "  %s -g 2 -o file.g00 file.png      → convert PNG to G00 format 2\n\n", appName)
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
		if err := convert(f, *verbose, *outdir, *outfile, inFmt, *g00Fmt, *metafile, *noMeta); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", f, err)
			os.Exit(1)
		}
	}
}

func convert(file string, verbose bool, outdir, outfile, inFmt, g00Fmt, metafile string, noMeta bool) error {
	ext := strings.ToLower(filepath.Ext(file))
	base := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))

	// Detect direction
	isG00 := ext == ".g00"
	isPNG := ext == ".png"

	if inFmt != "" {
		switch strings.ToLower(inFmt) {
		case "g00":
			isG00 = true
			isPNG = false
		case "png":
			isPNG = true
			isG00 = false
		}
	}

	if isG00 {
		return g00ToPNG(file, base, outdir, outfile, metafile, noMeta, verbose)
	} else if isPNG {
		return pngToG00(file, base, outdir, outfile, metafile, noMeta, verbose, g00Fmt)
	}
	return fmt.Errorf("unknown file type '%s' (expected .g00 or .png)", ext)
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

func looksLikeInputFile(value string) bool {
	ext := strings.ToLower(filepath.Ext(value))
	return ext == ".png" || ext == ".g00" || fileExists(value)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func pathIsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
