// rlsave inspects and edits RealLive save files.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"
	"unicode/utf8"

	"github.com/yoremi/rldev-go/pkg/rlsave"
)

const version = "0.1.0"

var (
	flagAll      = flag.Bool("all", false, "include zero values when dumping intG")
	flagJSON     = flag.Bool("json", false, "write dump output as JSON")
	flagBackup   = flag.Bool("backup", true, "create a timestamped .bak file before writing")
	flagNoBackup = flag.Bool("no-backup", false, "disable automatic backup")
	flagLossless = flag.Bool("lossless", false, "embed binary data in text exports so they can be rebuilt")
	flagJobs     = flag.Int("jobs", 0, "parallel jobs for batch commands; 0 uses an automatic value")
	intGRefRE    = regexp.MustCompile(`(?i)^intG\[(\d+)\]$`)
	seenRefRE    = regexp.MustCompile(`(?i)^(?:seen|read)\[(\d+)\]$`)
	seenBareRE   = regexp.MustCompile(`(?i)^seen(\d+)$`)
	dwordRefRE   = regexp.MustCompile(`(?i)^dword\[(\d+)\]$`)
)

type refKind string

const (
	refIntG  refKind = "intG"
	refSeen  refKind = "seen"
	refDWord refKind = "dword"
)

type variableRef struct {
	kind  refKind
	index int
}

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
	case "map":
		err = cmdMap(args)
	case "export":
		err = cmdExport(args)
	case "build":
		err = cmdBuild(args)
	case "diff":
		err = cmdDiff(args)
	case "doctor":
		err = cmdDoctor(args)
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
	fmt.Fprintln(os.Stderr, "  rlsave map <save.sav|save-dir> [...]")
	fmt.Fprintln(os.Stderr, "  rlsave export [-lossless] <save.sav> [out.txt]")
	fmt.Fprintln(os.Stderr, "  rlsave build <in.txt> <out.sav>")
	fmt.Fprintln(os.Stderr, "  rlsave diff [-json] <before.sav> <after.sav>")
	fmt.Fprintln(os.Stderr, "  rlsave doctor [-json] <save.sav|save-dir> [...]")
	fmt.Fprintln(os.Stderr, "  rlsave get <save.sav> intG[30]|seen[100]|dword[1] [...]")
	fmt.Fprintln(os.Stderr, "  rlsave set <save.sav> intG[30]=0|seen[100]=0|dword[1]=0 [...]")
	fmt.Fprintln(os.Stderr, "  rlsave dump [-all] [-json] <save999.sav>")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Notes:")
	fmt.Fprintln(os.Stderr, "  intG editing targets AVG_GLOBAL_SAVE/save999.sav.")
	fmt.Fprintln(os.Stderr, "  seen[n] targets read.sav progression entries; dword[n] targets raw body dwords.")
	fmt.Fprintln(os.Stderr, "  Rebuild requires an export created with -lossless.")
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
	fmt.Printf("label: %s\n", formatLabel(save.Label))
	fmt.Printf("kind: %s\n", save.Kind)
	fmt.Printf("container: %s\n", save.Container)
	fmt.Printf("file size: %d bytes\n", save.RawSize())
	fmt.Printf("header: %d bytes\n", save.HeaderLen)
	if save.Container == rlsave.ContainerCompressed {
		fmt.Printf("compressed body: %d bytes\n", save.CompressedSize)
		fmt.Printf("uncompressed body: %d bytes\n", save.UncompressedSize)
	} else {
		fmt.Printf("body: %d bytes\n", save.UncompressedSize)
	}
	stats := save.BodyStats()
	fmt.Printf("non-zero bytes: %d\n", stats.NonZeroBytes)
	fmt.Printf("non-zero dwords: %d\n", stats.NonZeroDWords)
	if save.Kind == rlsave.KindGlobal {
		ints, err := save.NonZeroGlobalInts()
		if err == nil {
			fmt.Printf("non-zero intG values: %d\n", len(ints))
		}
	}
	return nil
}

func cmdMap(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("map requires <save.sav|save-dir> [...]")
	}
	paths, err := collectSavePaths(args)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("no .sav files found")
	}

	type mapResult struct {
		summary rlsave.Summary
		err     error
	}
	results := make([]mapResult, len(paths))
	runIndexedJobs(len(paths), func(index int) {
		path := paths[index]
		save, err := rlsave.ReadFile(path)
		if err != nil {
			results[index].err = err
			return
		}
		results[index].summary = save.Summary()
	})

	var summaries []rlsave.Summary
	for index, result := range results {
		if result.err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: %v\n", paths[index], result.err)
			continue
		}
		summaries = append(summaries, result.summary)
	}
	if len(summaries) == 0 {
		return fmt.Errorf("no supported save files found")
	}
	if *flagJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(summaries)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "file\tkind\tcontainer\tlabel\theader\tbody\tcomp\tnz bytes\tnz dwords")
	for _, summary := range summaries {
		comp := "-"
		if summary.CompressedSize > 0 {
			comp = strconv.Itoa(summary.CompressedSize)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%d\t%s\t%d\t%d\n",
			displayPath(summary.Path),
			summary.Kind,
			summary.Container,
			formatLabel(summary.Label),
			summary.HeaderLen,
			summary.BodySize,
			comp,
			summary.NonZeroBytes,
			summary.NonZeroDWords)
	}
	return w.Flush()
}

func cmdExport(args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return fmt.Errorf("export requires <save.sav> [out.txt]")
	}
	save, err := rlsave.ReadFile(args[0])
	if err != nil {
		return err
	}
	if len(args) == 1 {
		return rlsave.ExportText(os.Stdout, save, rlsave.ExportOptions{Lossless: *flagLossless})
	}

	out, err := os.Create(args[1])
	if err != nil {
		return err
	}
	defer out.Close()
	if err := rlsave.ExportText(out, save, rlsave.ExportOptions{Lossless: *flagLossless}); err != nil {
		return err
	}
	fmt.Printf("written: %s\n", args[1])
	return nil
}

func cmdBuild(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("build requires <in.txt> <out.sav>")
	}
	in, err := os.Open(args[0])
	if err != nil {
		return err
	}
	save, err := rlsave.ImportText(in)
	closeErr := in.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	out, err := save.Bytes()
	if err != nil {
		return err
	}
	backupPath, err := backupExisting(args[1])
	if err != nil {
		return err
	}
	if err := os.WriteFile(args[1], out, 0666); err != nil {
		return err
	}
	if backupPath != "" {
		fmt.Printf("backup: %s\n", backupPath)
	}
	fmt.Printf("written: %s (%d bytes)\n", args[1], len(out))
	return nil
}

func cmdDiff(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("diff requires <before.sav> <after.sav>")
	}
	before, err := rlsave.ReadFile(args[0])
	if err != nil {
		return err
	}
	after, err := rlsave.ReadFile(args[1])
	if err != nil {
		return err
	}
	diff, err := rlsave.DiffSaves(before, after)
	if err != nil {
		return err
	}

	if *flagJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(diff)
	}

	fmt.Printf("before: %s\n", args[0])
	fmt.Printf("after: %s\n", args[1])
	fmt.Printf("kind: %s\n", diff.Kind)
	if diff.Label != "" {
		fmt.Printf("label: %s\n", formatLabel(diff.Label))
	}
	fmt.Printf("changes: %d\n", len(diff.Changes))
	for _, entry := range diff.Changes {
		if entry.Offset >= 0 {
			fmt.Printf("%s %d -> %d ; offset=0x%06X\n", entry.Name, entry.Old, entry.New, entry.Offset)
		} else {
			fmt.Printf("%s %d -> %d\n", entry.Name, entry.Old, entry.New)
		}
	}
	return nil
}

func cmdDoctor(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("doctor requires <save.sav|save-dir> [...]")
	}
	paths, err := collectSavePaths(args)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("no .sav files found")
	}

	findingsByPath := make([][]rlsave.DoctorFinding, len(paths))
	runIndexedJobs(len(paths), func(index int) {
		path := paths[index]
		save, err := rlsave.ReadFile(path)
		if err != nil {
			findingsByPath[index] = []rlsave.DoctorFinding{{
				Severity: rlsave.SeverityError,
				Path:     path,
				Message:  err.Error(),
			}}
			return
		}
		findingsByPath[index] = rlsave.DiagnoseSave(save)
	})

	var findings []rlsave.DoctorFinding
	for _, group := range findingsByPath {
		findings = append(findings, group...)
	}
	warnings, errors := countDoctorFindings(findings)

	if *flagJSON {
		out := struct {
			Files    int                    `json:"files"`
			Warnings int                    `json:"warnings"`
			Errors   int                    `json:"errors"`
			Findings []rlsave.DoctorFinding `json:"findings"`
		}{Files: len(paths), Warnings: warnings, Errors: errors, Findings: findings}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	for _, finding := range findings {
		path := displayPath(finding.Path)
		if path == "" {
			path = "-"
		}
		fmt.Printf("%s %s: %s\n", strings.ToUpper(string(finding.Severity)), path, finding.Message)
	}
	fmt.Printf("summary: %d file(s), %d warning(s), %d error(s)\n", len(paths), warnings, errors)
	return nil
}

func countDoctorFindings(findings []rlsave.DoctorFinding) (warnings, errors int) {
	for _, finding := range findings {
		switch finding.Severity {
		case rlsave.SeverityWarning:
			warnings++
		case rlsave.SeverityError:
			errors++
		}
	}
	return warnings, errors
}

func runIndexedJobs(count int, fn func(index int)) {
	if count <= 0 {
		return
	}
	jobs := batchJobs(count)
	indexes := make(chan int)
	var wg sync.WaitGroup
	for worker := 0; worker < jobs; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range indexes {
				fn(index)
			}
		}()
	}
	for index := 0; index < count; index++ {
		indexes <- index
	}
	close(indexes)
	wg.Wait()
}

func batchJobs(count int) int {
	jobs := *flagJobs
	if jobs <= 0 {
		jobs = runtime.NumCPU()
		if jobs > 8 {
			jobs = 8
		}
	}
	if jobs < 1 {
		jobs = 1
	}
	if jobs > count {
		jobs = count
	}
	return jobs
}

func cmdGet(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("get requires <save.sav> <var> [var...]")
	}
	save, err := rlsave.ReadFile(args[0])
	if err != nil {
		return err
	}
	for _, ref := range args[1:] {
		parsed, err := parseVariableRef(ref)
		if err != nil {
			return err
		}
		value, err := getVariable(save, parsed)
		if err != nil {
			return err
		}
		fmt.Printf("%s=%s\n", parsed, value)
	}
	return nil
}

func cmdSet(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("set requires <save.sav> <var=value> [var=value...]")
	}
	save, err := rlsave.ReadFile(args[0])
	if err != nil {
		return err
	}

	type change struct {
		ref variableRef
		old string
		new string
	}
	var changes []change
	for _, assignment := range args[1:] {
		ref, value, err := parseAssignment(assignment)
		if err != nil {
			return err
		}
		oldValue, newValue, err := setVariable(save, ref, value)
		if err != nil {
			return err
		}
		changes = append(changes, change{ref: ref, old: oldValue, new: newValue})
	}

	result, err := save.WriteFile(args[0], rlsave.WriteOptions{Backup: *flagBackup && !*flagNoBackup})
	if err != nil {
		return err
	}
	for _, c := range changes {
		fmt.Printf("%s %s -> %s\n", c.ref, c.old, c.new)
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

func parseAssignment(text string) (variableRef, string, error) {
	parts := strings.SplitN(text, "=", 2)
	if len(parts) != 2 {
		return variableRef{}, "", fmt.Errorf("bad assignment %q; expected name[n]=value", text)
	}
	ref, err := parseVariableRef(strings.TrimSpace(parts[0]))
	if err != nil {
		return variableRef{}, "", err
	}
	return ref, strings.TrimSpace(parts[1]), nil
}

func parseVariableRef(text string) (variableRef, error) {
	text = strings.TrimSpace(text)
	if m := intGRefRE.FindStringSubmatch(text); m != nil {
		index, err := strconv.Atoi(m[1])
		if err != nil {
			return variableRef{}, err
		}
		return variableRef{kind: refIntG, index: index}, nil
	}
	if m := seenRefRE.FindStringSubmatch(text); m != nil {
		index, err := strconv.Atoi(m[1])
		if err != nil {
			return variableRef{}, err
		}
		return variableRef{kind: refSeen, index: index}, nil
	}
	if m := seenBareRE.FindStringSubmatch(text); m != nil {
		index, err := strconv.Atoi(m[1])
		if err != nil {
			return variableRef{}, err
		}
		return variableRef{kind: refSeen, index: index}, nil
	}
	if m := dwordRefRE.FindStringSubmatch(text); m != nil {
		index, err := strconv.Atoi(m[1])
		if err != nil {
			return variableRef{}, err
		}
		return variableRef{kind: refDWord, index: index}, nil
	}
	return variableRef{}, fmt.Errorf("unsupported variable %q; expected intG[n], seen[n], seenNNNN, or dword[n]", text)
}

func getVariable(save *rlsave.Save, ref variableRef) (string, error) {
	switch ref.kind {
	case refIntG:
		value, err := save.GlobalInt(ref.index)
		if err != nil {
			return "", err
		}
		return strconv.FormatInt(int64(value), 10), nil
	case refSeen:
		value, err := save.ReadProgress(ref.index)
		if err != nil {
			return "", err
		}
		return strconv.FormatUint(uint64(value), 10), nil
	case refDWord:
		value, err := save.BodyDWord(ref.index)
		if err != nil {
			return "", err
		}
		return strconv.FormatUint(uint64(value), 10), nil
	default:
		return "", fmt.Errorf("unsupported variable kind %q", ref.kind)
	}
}

func setVariable(save *rlsave.Save, ref variableRef, valueText string) (string, string, error) {
	oldValue, err := getVariable(save, ref)
	if err != nil {
		return "", "", err
	}
	switch ref.kind {
	case refIntG:
		value, err := strconv.ParseInt(valueText, 0, 32)
		if err != nil {
			return "", "", fmt.Errorf("bad value %q: %w", valueText, err)
		}
		if err := save.SetGlobalInt(ref.index, int32(value)); err != nil {
			return "", "", err
		}
		return oldValue, strconv.FormatInt(value, 10), nil
	case refSeen:
		value, err := strconv.ParseUint(valueText, 0, 32)
		if err != nil {
			return "", "", fmt.Errorf("bad value %q: %w", valueText, err)
		}
		if err := save.SetReadProgress(ref.index, uint32(value)); err != nil {
			return "", "", err
		}
		return oldValue, strconv.FormatUint(value, 10), nil
	case refDWord:
		value, err := strconv.ParseUint(valueText, 0, 32)
		if err != nil {
			return "", "", fmt.Errorf("bad value %q: %w", valueText, err)
		}
		if err := save.SetBodyDWord(ref.index, uint32(value)); err != nil {
			return "", "", err
		}
		return oldValue, strconv.FormatUint(value, 10), nil
	default:
		return "", "", fmt.Errorf("unsupported variable kind %q", ref.kind)
	}
}

func (r variableRef) String() string {
	switch r.kind {
	case refIntG:
		return fmt.Sprintf("intG[%d]", r.index)
	case refSeen:
		return fmt.Sprintf("seen[%d]", r.index)
	case refDWord:
		return fmt.Sprintf("dword[%d]", r.index)
	default:
		return fmt.Sprintf("%s[%d]", r.kind, r.index)
	}
}

func collectSavePaths(args []string) ([]string, error) {
	seen := map[string]bool{}
	var paths []string
	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			addSavePath(&paths, seen, arg)
			continue
		}
		if err := filepath.WalkDir(arg, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			if strings.EqualFold(filepath.Ext(path), ".sav") {
				addSavePath(&paths, seen, path)
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func addSavePath(paths *[]string, seen map[string]bool, path string) {
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	key := strings.ToLower(path)
	if seen[key] {
		return
	}
	seen[key] = true
	*paths = append(*paths, path)
}

func displayPath(path string) string {
	rel, err := filepath.Rel(".", path)
	if err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
		return rel
	}
	return path
}

func formatLabel(label string) string {
	if utf8.ValidString(label) {
		return label
	}
	return strconv.Quote(label)
}

func backupExisting(path string) (string, error) {
	if !*flagBackup || *flagNoBackup {
		return "", nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	backup := path + ".bak-" + time.Now().Format("20060102-150405")
	if err := os.WriteFile(backup, raw, 0666); err != nil {
		return "", err
	}
	return backup, nil
}
