// Package config provides configuration and path resolution for RLdev tools.
// Transposed from OCaml's config.ml.
package config

import (
	"os"
	"path/filepath"
)

const (
	// Version is the RLdev-Go version string
	Version = "2.0.26"

	// DefaultEncoding is the default character encoding
	DefaultEncoding = "CP932"

	// IVarPrefix is the prefix for integer variables in scripts
	IVarPrefix = "int"

	// SVarPrefix is the prefix for string variables in scripts
	SVarPrefix = "str"
)

// prefixCache stores the resolved library prefix path
var prefixCache string

// Prefix returns the RLdev library directory.
// It searches in order:
//  1. $RLDEV environment variable
//  2. Executable directory + lib/
//  3. Common installation paths
func Prefix() string {
	if prefixCache != "" {
		return prefixCache
	}

	rldev := os.Getenv("RLDEV")
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "."
	}

	execDir := filepath.Dir(os.Args[0])

	searchPaths := []string{
		filepath.Join(rldev, "lib"),
		rldev,
		filepath.Join(execDir, "lib"),
		execDir,
		home,
		filepath.Join(home, "rldev"),
		filepath.Join(home, ".rldev"),
		filepath.Join(home, "rldev", "lib"),
		filepath.Join(home, ".rldev", "lib"),
	}

	for _, p := range searchPaths {
		if p == "" {
			continue
		}
		kfn := filepath.Join(p, "reallive.kfn")
		if _, err := os.Stat(kfn); err == nil {
			prefixCache = p
			return p
		}
	}

	// Fallback: use executable directory
	prefixCache = execDir
	return execDir
}

// LibFile returns the full path to a library file.
// If fname is already absolute, returns it as-is.
func LibFile(fname string) string {
	if filepath.IsAbs(fname) {
		return fname
	}
	return filepath.Join(Prefix(), fname)
}
