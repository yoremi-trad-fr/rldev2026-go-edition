// Command rlxml converts RealLive GAN animation files to/from XML.
//
// Transposed from OCaml's rlxml/main.ml (82L) + rlxml/app.ml (29L).
//
// Usage:
//
//	rlxml [options] <file.gan>      → converts to file.ganxml
//	rlxml [options] <file.ganxml>   → converts to file.gan
//
// Options:
//
//	-o NAME    output file or directory name
//	-v         verbose output
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yoremi/rldev-go/rlxml/pkg/gan"
)

const (
	appName    = "rlxml"
	appVersion = "2026 (Go port)"
	appDesc    = "converter between RealLive auxiliary data formats and XML"
)

var (
	verbose bool
	outdir  string
)

func main() {
	flag.BoolVar(&verbose, "v", false, "verbose output")
	flag.StringVar(&outdir, "o", "", "output file or directory name")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s - %s (%s)\n", appName, appDesc, appVersion)
		fmt.Fprintf(os.Stderr, "\nUsage: %s [options] <file.gan|file.ganxml> ...\n\nOptions:\n", appName)
		flag.PrintDefaults()
	}
	flag.Parse()

	files := flag.Args()
	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "%s: no input files\n", appName)
		flag.Usage()
		os.Exit(1)
	}

	single := len(files) == 1
	for _, f := range files {
		if err := convert(f, single); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", f, err)
			os.Exit(1)
		}
	}
}

func convert(file string, single bool) error {
	ext := strings.ToLower(filepath.Ext(file))
	base := strings.TrimSuffix(file, filepath.Ext(file))

	switch ext {
	case ".gan":
		return ganToXML(file, base, single)
	case ".ganxml":
		return xmlToGAN(file, base, single)
	default:
		return fmt.Errorf("unknown file type '%s' (expected .gan or .ganxml)", ext)
	}
}

func ganToXML(inFile, base string, single bool) error {
	if verbose {
		fmt.Fprintf(os.Stderr, "Reading GAN: %s\n", inFile)
	}

	g, err := gan.ReadFile(inFile)
	if err != nil {
		return fmt.Errorf("reading GAN: %w", err)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "  Bitmap: %s, Sets: %d\n", g.Bitmap, len(g.Sets))
		for i, s := range g.Sets {
			fmt.Fprintf(os.Stderr, "  Set %d: %d frames\n", i, len(s.Frames))
		}
	}

	xmlStr, err := gan.ToXML(g)
	if err != nil {
		return fmt.Errorf("generating XML: %w", err)
	}

	outFile := resolveOutput(base, ".ganxml", single)
	if verbose {
		fmt.Fprintf(os.Stderr, "Writing XML: %s\n", outFile)
	}
	return os.WriteFile(outFile, []byte(xmlStr), 0644)
}

func xmlToGAN(inFile, base string, single bool) error {
	if verbose {
		fmt.Fprintf(os.Stderr, "Reading XML: %s\n", inFile)
	}

	data, err := os.ReadFile(inFile)
	if err != nil {
		return err
	}

	g, err := gan.FromXML(data)
	if err != nil {
		return fmt.Errorf("parsing XML: %w", err)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "  Bitmap: %s, Sets: %d\n", g.Bitmap, len(g.Sets))
	}

	outFile := resolveOutput(base, ".gan", single)
	if verbose {
		fmt.Fprintf(os.Stderr, "Writing GAN: %s\n", outFile)
	}
	return gan.WriteFile(outFile, g)
}

func resolveOutput(base, ext string, single bool) string {
	if single && outdir != "" {
		if strings.HasSuffix(outdir, ext) {
			return outdir
		}
		return outdir + ext
	}
	if outdir != "" {
		return filepath.Join(outdir, filepath.Base(base)+ext)
	}
	return base + ext
}
