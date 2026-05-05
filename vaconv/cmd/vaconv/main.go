// Command vaconv converts RealLive G00 images to/from PNG.
//
// Ported from vaconv (OCaml + C++).
//
// Usage:
//
//	vaconv file.g00              → file.png
//	vaconv -d outdir *.g00       → outdir/file.png (batch)
//	vaconv -i png file.png -o file.g00  → PNG to G00
//	vaconv -f 0 file.png -o file.g00    → force format 0
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
	inFmt := flag.String("i", "", "input format (auto-detect by default)")
	g00Fmt := flag.String("f", "auto", "G00 format: 0, 1, 2, or auto")
	// metafile := flag.String("m", "", "metadata XML file")
	// noMeta := flag.Bool("q", false, "disable metadata")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s %s - VisualArt's bitmap format converter\n\n", appName, appVersion)
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  %s [options] <file.g00>           → convert to PNG\n", appName)
		fmt.Fprintf(os.Stderr, "  %s [options] -d DIR *.g00         → batch convert to PNG\n", appName)
		fmt.Fprintf(os.Stderr, "  %s -i png file.png -o file.g00    → convert PNG to G00\n\n", appName)
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	files := flag.Args()
	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "%s: no input files\n", appName)
		flag.Usage()
		os.Exit(1)
	}

	for _, f := range files {
		if err := convert(f, *verbose, *outdir, *outfile, *inFmt, *g00Fmt); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", f, err)
			os.Exit(1)
		}
	}
}

func convert(file string, verbose bool, outdir, outfile, inFmt, g00Fmt string) error {
	ext := strings.ToLower(filepath.Ext(file))
	base := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))

	// Detect direction
	isG00 := ext == ".g00"
	isPNG := ext == ".png"

	if inFmt != "" {
		switch strings.ToLower(inFmt) {
		case "g00": isG00 = true; isPNG = false
		case "png": isPNG = true; isG00 = false
		}
	}

	if isG00 {
		return g00ToPNG(file, base, outdir, outfile, verbose)
	} else if isPNG {
		return pngToG00(file, base, outdir, outfile, verbose, g00Fmt)
	}
	return fmt.Errorf("unknown file type '%s' (expected .g00 or .png)", ext)
}

func g00ToPNG(inFile, base, outdir, outfile string, verbose bool) error {
	if verbose { fmt.Fprintf(os.Stderr, "Reading G00: %s\n", inFile) }

	img, err := g00.ReadFile(inFile)
	if err != nil { return err }

	if verbose {
		fmt.Fprintf(os.Stderr, "  Format: %d, Size: %dx%d\n", img.Format, img.Width, img.Height)
		if len(img.Regions) > 0 {
			fmt.Fprintf(os.Stderr, "  Regions: %d\n", len(img.Regions))
		}
	}

	out := resolveOutput(base, ".png", outdir, outfile)
	if verbose { fmt.Fprintf(os.Stderr, "Writing PNG: %s\n", out) }
	return g00.ToPNGFile(img, out)
}

func pngToG00(inFile, base, outdir, outfile string, verbose bool, g00Fmt string) error {
	if verbose { fmt.Fprintf(os.Stderr, "Reading PNG: %s\n", inFile) }

	img, err := g00.FromPNGFile(inFile)
	if err != nil { return err }

	// Set format
	switch g00Fmt {
	case "0": img.Format = 0
	case "1": img.Format = 1
	case "2": img.Format = 2
	default: img.Format = 0 // auto = format 0 for now
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "  Size: %dx%d, Output format: %d\n", img.Width, img.Height, img.Format)
	}

	out := resolveOutput(base, ".g00", outdir, outfile)
	if verbose { fmt.Fprintf(os.Stderr, "Writing G00: %s\n", out) }
	return g00.WriteFile(out, img)
}

func resolveOutput(base, ext, outdir, outfile string) string {
	if outfile != "" { return outfile }
	name := base + ext
	if outdir != "" { return filepath.Join(outdir, name) }
	return name
}
