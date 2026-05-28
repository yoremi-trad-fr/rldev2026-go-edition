//go:build !windows

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "reallive-debug-launcher is only available on Windows")
	os.Exit(1)
}
