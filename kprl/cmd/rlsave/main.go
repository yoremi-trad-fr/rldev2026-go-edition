// rlsave inspects and edits RealLive save files.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/yoremi/rldev-go/pkg/rlsave"
)

const version = "0.1.0"

var (
	flagAll      = flag.Bool("all", false, "include zero values when dumping intG")
	flagJSON     = flag.Bool("json", false, "write dump output as JSON")
	flagBackup   = flag.Bool("backup", true, "create a timestamped .bak file before writing")
	flagNoBackup = flag.Bool("no-backup", false, "disable automatic backup")
	intGRefRE    = regexp.MustCompile(`(?i)^intG\[(\d+)\]$`)
)

func main() {
	flag.Usage = usage
	flagArgs, positional := splitArgs(os.Args[1:])
	if err := flag.CommandLine.Parse(flagArgs); err != nil {
		os.Exit(2)
	}

	if len(positional) == 0 {
		usage()
		os.Exit(1)
	}

	cmd := positional[0]
	args := positional[1:]

	var err error
	switch cmd {
	case "info":
		err = cmdInfo(args)
	case "get":
		err = cmdGet(args)
	case "set":
		err = cmdSet(args)
	case "dump":
		err = cmdDump(args)
	default:
		err = fmt.Errorf("unknown command %q", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "rlsave:", err)
		os.Exit(1)
	}
}

func splitArgs(args []string) ([]string, []string) {
	var flagArgs []string
	var positional []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			continue
		}
		positional = append(positional, arg)
	}
	return flagArgs, positional
}

func usage() {
	fmt.Fprintf(os.Stderr, "rlsave %s - RealLive save inspector/editor\n\n", version)
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  rlsave info <save.sav>")
	fmt.Fprintln(os.Stderr, "  rlsave get <save999.sav> intG[30] [intG[0]...]")
	fmt.Fprintln(os.Stderr, "  rlsave set <save999.sav> intG[30]=0 [intG[0]=2...]")
	fmt.Fprintln(os.Stderr, "  rlsave dump [-all] [-json] <save999.sav>")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Notes:")
	fmt.Fprintln(os.Stderr, "  intG editing currently targets AVG_GLOBAL_SAVE/save999.sav.")
	fmt.Fprintln(os.Stderr, "  Regular slot saves can be decoded with info; their variable layout is not edited yet.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Options:")
	flag.PrintDefaults()
}

func cmdInfo(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("info requires <save.sav>")
	}
	save, err := rlsave.ReadFile(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("file: %s\n", args[0])
	fmt.Printf("label: %s\n", save.Label)
	fmt.Printf("kind: %s\n", save.Kind)
	fmt.Printf("header: %d bytes\n", save.HeaderLen)
	fmt.Printf("compressed body: %d bytes\n", save.CompressedSize)
	fmt.Printf("uncompressed body: %d bytes\n", save.UncompressedSize)
	if save.Kind == rlsave.KindGlobal {
		ints, err := save.NonZeroGlobalInts()
		if err == nil {
			fmt.Printf("non-zero intG values: %d\n", len(ints))
		}
	}
	return nil
}

func cmdGet(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("get requires <save999.sav> <var> [var...]")
	}
	save, err := rlsave.ReadFile(args[0])
	if err != nil {
		return err
	}
	for _, ref := range args[1:] {
		index, err := parseIntGRef(ref)
		if err != nil {
			return err
		}
		value, err := save.GlobalInt(index)
		if err != nil {
			return err
		}
		fmt.Printf("intG[%d]=%d\n", index, value)
	}
	return nil
}

func cmdSet(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("set requires <save999.sav> <var=value> [var=value...]")
	}
	save, err := rlsave.ReadFile(args[0])
	if err != nil {
		return err
	}

	type change struct {
		index int
		old   int32
		new   int32
	}
	var changes []change
	for _, assignment := range args[1:] {
		index, value, err := parseAssignment(assignment)
		if err != nil {
			return err
		}
		old, err := save.GlobalInt(index)
		if err != nil {
			return err
		}
		if err := save.SetGlobalInt(index, value); err != nil {
			return err
		}
		changes = append(changes, change{index: index, old: old, new: value})
	}

	result, err := save.WriteFile(args[0], rlsave.WriteOptions{Backup: *flagBackup && !*flagNoBackup})
	if err != nil {
		return err
	}
	for _, c := range changes {
		fmt.Printf("intG[%d] %d -> %d\n", c.index, c.old, c.new)
	}
	if result.BackupPath != "" {
		fmt.Printf("backup: %s\n", result.BackupPath)
	}
	fmt.Printf("written: %s (%d -> %d bytes)\n", result.Path, result.OldSize, result.NewSize)
	return nil
}

func cmdDump(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("dump requires <save999.sav>")
	}
	save, err := rlsave.ReadFile(args[0])
	if err != nil {
		return err
	}
	if save.Kind != rlsave.KindGlobal {
		return fmt.Errorf("dump currently supports only AVG_GLOBAL_SAVE/save999.sav")
	}

	ints, err := dumpInts(save, *flagAll)
	if err != nil {
		return err
	}
	if *flagJSON {
		out := struct {
			File string             `json:"file"`
			Kind rlsave.Kind        `json:"kind"`
			IntG []rlsave.GlobalInt `json:"intG"`
		}{File: args[0], Kind: save.Kind, IntG: ints}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	for _, entry := range ints {
		fmt.Printf("intG[%d]=%d\n", entry.Index, entry.Value)
	}
	return nil
}

func dumpInts(save *rlsave.Save, all bool) ([]rlsave.GlobalInt, error) {
	if !all {
		return save.NonZeroGlobalInts()
	}
	ints := make([]rlsave.GlobalInt, 0, rlsave.GlobalIntCount)
	for i := 0; i < rlsave.GlobalIntCount; i++ {
		v, err := save.GlobalInt(i)
		if err != nil {
			return nil, err
		}
		ints = append(ints, rlsave.GlobalInt{Index: i, Value: v})
	}
	return ints, nil
}

func parseAssignment(text string) (int, int32, error) {
	parts := strings.SplitN(text, "=", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("bad assignment %q; expected intG[n]=value", text)
	}
	index, err := parseIntGRef(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, err
	}
	value, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 0, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("bad value in %q: %w", text, err)
	}
	return index, int32(value), nil
}

func parseIntGRef(text string) (int, error) {
	m := intGRefRE.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return 0, fmt.Errorf("unsupported variable %q; expected intG[n]", text)
	}
	index, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, err
	}
	return index, nil
}
