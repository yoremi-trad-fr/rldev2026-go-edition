package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx        context.Context
	mu         sync.Mutex
	cancelFunc context.CancelFunc
	logMu      sync.Mutex
	logFile    *os.File
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) log(msg string) {
	a.logMu.Lock()
	if a.logFile != nil {
		_, _ = fmt.Fprintln(a.logFile, msg)
	}
	a.logMu.Unlock()

	if a.ctx != nil {
		wailsRuntime.EventsEmit(a.ctx, "log", msg)
	}
}

func (a *App) logError(msg string) {
	a.log("[ERROR] " + msg)
}

func (a *App) logOK(msg string) {
	a.log("[OK] " + msg)
}

func (a *App) startLogFile(outputDir, prefix string) func() {
	outputDir = strings.TrimSpace(outputDir)
	if outputDir == "" {
		return func() {}
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		a.logError(fmt.Sprintf("journal impossible: %v", err))
		return func() {}
	}

	name := fmt.Sprintf("%s-%s.log", prefix, time.Now().Format("20060102-150405"))
	path := filepath.Join(outputDir, name)
	file, err := os.Create(path)
	if err != nil {
		a.logError(fmt.Sprintf("journal impossible: %v", err))
		return func() {}
	}

	a.logMu.Lock()
	previous := a.logFile
	a.logFile = file
	a.logMu.Unlock()

	a.logOK("Log complet: " + path)

	return func() {
		a.logMu.Lock()
		if a.logFile == file {
			a.logFile = previous
		}
		a.logMu.Unlock()
		_ = file.Close()
	}
}

func (a *App) executableDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("impossible de localiser l'executable: %w", err)
	}
	return filepath.Dir(exePath), nil
}

func (a *App) binDir() (string, error) {
	exeDir, err := a.executableDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(exeDir, "bin"), nil
}

func (a *App) toolPath(toolName string) (string, error) {
	allowed := map[string]string{
		"kprl":   "kprl16.exe",
		"rlc":    "rlc2026.exe",
		"vaconv": "vaconv.exe",
		"rlxml":  "rlxml.exe",
		"rlsave": "rlsave.exe",
	}

	exeName, ok := allowed[toolName]
	if !ok {
		return "", fmt.Errorf("outil non pris en charge: %s", toolName)
	}

	binDir, err := a.binDir()
	if err != nil {
		return "", err
	}

	path := filepath.Join(binDir, exeName)
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return "", fmt.Errorf("binaire manquant: %s", path)
	}
	return path, nil
}

func (a *App) findKFN() string {
	var candidates []string

	binDir, err := a.binDir()
	if err == nil {
		candidates = append(candidates,
			filepath.Join(binDir, "lib", "reallive.kfn"),
			filepath.Join(binDir, "reallive.kfn"),
		)
	}

	if exeDir, err := a.executableDir(); err == nil {
		dir := exeDir
		for i := 0; i < 4 && dir != ""; i++ {
			candidates = append(candidates, filepath.Join(dir, "KFN", "reallive.kfn"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "KFN", "reallive.kfn"))
	}

	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func (a *App) DefaultKFN() string {
	return a.findKFN()
}

func (a *App) DefaultBabelRoot() string {
	var candidates []string
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, "BABEL"),
			filepath.Join(filepath.Dir(wd), "ResCODEX", "Rldev2026-go", "BABEL"),
			filepath.Join(filepath.Dir(filepath.Dir(wd)), "ResCODEX", "Rldev2026-go", "BABEL"),
		)
	}
	if exeDir, err := a.executableDir(); err == nil {
		dir := exeDir
		for i := 0; i < 5 && dir != ""; i++ {
			candidates = append(candidates, filepath.Join(dir, "BABEL"))
			candidates = append(candidates, filepath.Join(dir, "ResCODEX", "Rldev2026-go", "BABEL"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	for _, candidate := range candidates {
		if isBabelRoot(candidate) {
			return candidate
		}
	}
	return ""
}

func isBabelRoot(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	if info, err := os.Stat(filepath.Join(path, "rtl", "rlBabel.dll")); err != nil || info.IsDir() {
		return false
	}
	if info, err := os.Stat(filepath.Join(path, "rtl", "rlBabelF.dll")); err != nil || info.IsDir() {
		return false
	}
	return true
}

var realLiveInterpreterCandidates = []string{
	"RealLive.exe",
	"RealLiveEn.exe",
	"Kinetic.exe",
	"kinetic.exe",
	"AVG2000.exe",
	"avg2000.exe",
	"SiglusEngine.exe",
	"siglusengine.exe",
	"SiglusEngine_Steam.exe",
	"siglusengine_steam.exe",
	"reallive.exe",
}

func findInterpreterInDir(dir string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return ""
	}
	for _, name := range realLiveInterpreterCandidates {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

func versionString(v [4]int) string {
	return fmt.Sprintf("%d.%d.%d.%d", v[0], v[1], v[2], v[3])
}

func peVersionFromExe(path string) ([4]int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return [4]int{}, err
	}
	if len(data) < 0x40 || data[0] != 'M' || data[1] != 'Z' {
		return [4]int{}, fmt.Errorf("ce n'est pas un executable PE")
	}
	peOff := int(uint32(data[0x3c]) | uint32(data[0x3d])<<8 | uint32(data[0x3e])<<16 | uint32(data[0x3f])<<24)
	if peOff+4 > len(data) || string(data[peOff:peOff+4]) != "PE\x00\x00" {
		return [4]int{}, fmt.Errorf("signature PE introuvable")
	}

	coffOff := peOff + 4
	if coffOff+20 > len(data) {
		return [4]int{}, fmt.Errorf("en-tete PE tronque")
	}
	nsec := int(uint16(data[coffOff+2]) | uint16(data[coffOff+3])<<8)
	optSize := int(uint16(data[coffOff+16]) | uint16(data[coffOff+17])<<8)
	secStart := coffOff + 20 + optSize

	rsrcOff := 0
	rsrcSize := 0
	for i := 0; i < nsec; i++ {
		s := secStart + i*40
		if s+40 > len(data) {
			break
		}
		name := strings.TrimRight(string(data[s:s+8]), "\x00")
		if name == ".rsrc" {
			rsrcSize = int(uint32(data[s+16]) | uint32(data[s+17])<<8 | uint32(data[s+18])<<16 | uint32(data[s+19])<<24)
			rsrcOff = int(uint32(data[s+20]) | uint32(data[s+21])<<8 | uint32(data[s+22])<<16 | uint32(data[s+23])<<24)
			break
		}
	}
	if rsrcOff == 0 {
		return [4]int{}, fmt.Errorf("section .rsrc introuvable")
	}
	end := rsrcOff + rsrcSize
	if end > len(data) {
		end = len(data)
	}

	idx := findVSFixedFileInfo(data, rsrcOff, end)
	if idx < 0 {
		idx = findVSFixedFileInfo(data, 0, len(data))
	}
	if idx < 0 {
		if v, err := peVersionFromStringFileInfo(data); err == nil {
			return v, nil
		}
		return [4]int{}, fmt.Errorf("version RealLive introuvable")
	}
	if idx+16 > len(data) {
		return [4]int{}, fmt.Errorf("version RealLive tronquee")
	}
	fvms := uint32(data[idx+8]) | uint32(data[idx+9])<<8 | uint32(data[idx+10])<<16 | uint32(data[idx+11])<<24
	fvls := uint32(data[idx+12]) | uint32(data[idx+13])<<8 | uint32(data[idx+14])<<16 | uint32(data[idx+15])<<24
	return [4]int{int(fvms >> 16), int(fvms & 0xffff), int(fvls >> 16), int(fvls & 0xffff)}, nil
}

func findVSFixedFileInfo(data []byte, start, end int) int {
	if start < 0 {
		start = 0
	}
	if end > len(data) {
		end = len(data)
	}
	if start >= end {
		return -1
	}
	sig := []byte{0xbd, 0x04, 0xef, 0xfe}
	for i := start; i+16 < end; i++ {
		if data[i] == sig[0] && data[i+1] == sig[1] && data[i+2] == sig[2] && data[i+3] == sig[3] {
			return i
		}
	}
	return -1
}

func peVersionFromStringFileInfo(data []byte) ([4]int, error) {
	key := utf16LEBytes("FileVersion")
	for i := 0; i+len(key) < len(data); i++ {
		if !bytesEqual(data[i:i+len(key)], key) {
			continue
		}
		searchEnd := i + len(key) + 256
		if searchEnd > len(data) {
			searchEnd = len(data)
		}
		for j := i + len(key); j+2 <= searchEnd; j += 2 {
			s := readUTF16LEString(data[j:searchEnd], 64)
			if v, ok := parseFileVersionString(s); ok {
				return v, nil
			}
		}
	}
	return [4]int{}, fmt.Errorf("FileVersion introuvable")
}

func utf16LEBytes(s string) []byte {
	words := utf16.Encode([]rune(s))
	out := make([]byte, 0, len(words)*2)
	for _, w := range words {
		out = append(out, byte(w), byte(w>>8))
	}
	return out
}

func readUTF16LEString(data []byte, maxRunes int) string {
	words := make([]uint16, 0, maxRunes)
	for i := 0; i+1 < len(data) && len(words) < maxRunes; i += 2 {
		w := uint16(data[i]) | uint16(data[i+1])<<8
		if w == 0 {
			break
		}
		words = append(words, w)
	}
	return string(utf16.Decode(words))
}

func parseFileVersionString(s string) ([4]int, bool) {
	if !strings.ContainsAny(s, ".,") {
		return [4]int{}, false
	}
	parts := make([]int, 0, 4)
	current := -1
	flush := func() bool {
		if current < 0 {
			return true
		}
		if current > 9999 {
			return false
		}
		parts = append(parts, current)
		current = -1
		return len(parts) <= 4
	}
	for _, r := range s {
		if r >= '0' && r <= '9' {
			if current < 0 {
				current = 0
			}
			current = current*10 + int(r-'0')
			continue
		}
		if !flush() {
			return [4]int{}, false
		}
	}
	if !flush() || len(parts) < 2 || len(parts) > 4 || parts[0] > 20 {
		return [4]int{}, false
	}
	var v [4]int
	for i, p := range parts {
		v[i] = p
	}
	return v, true
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (a *App) resolveInterpreter(gameexe, interpreter string) string {
	interpreter = strings.TrimSpace(interpreter)
	if interpreter != "" {
		return interpreter
	}

	gameexe = strings.TrimSpace(gameexe)
	if gameexe == "" {
		return ""
	}

	dir := gameexe
	if info, err := os.Stat(gameexe); err == nil {
		if !info.IsDir() {
			dir = filepath.Dir(gameexe)
		}
	} else if filepath.Ext(gameexe) != "" {
		dir = filepath.Dir(gameexe)
	}

	if found := findInterpreterInDir(dir); found != "" {
		a.logOK("Interpreteur detecte: " + found)
		return found
	}
	return ""
}

func (a *App) DetectRealLiveVersion(gameexe, interpreter string) string {
	interpreter = a.resolveInterpreter(gameexe, interpreter)
	if interpreter == "" {
		return ""
	}
	version, err := peVersionFromExe(interpreter)
	if err != nil {
		a.log("Version RealLive non detectee: " + err.Error())
		return ""
	}
	text := versionString(version)
	a.logOK("Version RealLive detectee: " + text)
	return text
}

func (a *App) runTool(toolName string, args ...string) error {
	toolPath, err := a.toolPath(toolName)
	if err != nil {
		a.logError(err.Error())
		return err
	}

	a.log(fmt.Sprintf("> %s %s", filepath.Base(toolPath), strings.Join(args, " ")))

	ctx, cancel := context.WithCancel(a.ctx)
	a.mu.Lock()
	a.cancelFunc = cancel
	a.mu.Unlock()
	defer func() {
		cancel()
		a.mu.Lock()
		a.cancelFunc = nil
		a.mu.Unlock()
	}()

	cmd := exec.CommandContext(ctx, toolPath, args...)
	cmd.Dir = filepath.Dir(toolPath)
	hideWindow(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		a.logError(fmt.Sprintf("stdout: %v", err))
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		a.logError(fmt.Sprintf("stderr: %v", err))
		return err
	}

	if err := cmd.Start(); err != nil {
		a.logError(fmt.Sprintf("demarrage impossible: %v", err))
		return err
	}

	done := make(chan struct{}, 2)
	streamLines := func(reader io.Reader) {
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			a.log(scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			a.logError(fmt.Sprintf("lecture console: %v", err))
		}
		done <- struct{}{}
	}

	go streamLines(stdout)
	go streamLines(stderr)

	<-done
	<-done

	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			a.log("[STOPPED] Operation arretee par l'utilisateur.")
			return fmt.Errorf("operation arretee")
		}
		a.logError(fmt.Sprintf("processus termine en erreur: %v", err))
		return err
	}

	return nil
}

func (a *App) StopProcess() {
	a.mu.Lock()
	cancel := a.cancelFunc
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (a *App) SelectFile(title string, pattern string, desc string) string {
	file, _ := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: title,
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: desc, Pattern: pattern},
			{DisplayName: "Tous les fichiers", Pattern: "*.*"},
		},
	})
	return file
}

func (a *App) SelectDirectory(title string) string {
	dir, _ := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: title,
	})
	return dir
}

func (a *App) SelectSaveFile(title string, defaultName string, pattern string, desc string) string {
	file, _ := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		Title:           title,
		DefaultFilename: defaultName,
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: desc, Pattern: pattern},
			{DisplayName: "Tous les fichiers", Pattern: "*.*"},
		},
	})
	return file
}

func required(label string, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s est requis", label)
	}
	return nil
}

func (a *App) failIf(err error) string {
	if err != nil {
		a.logError(err.Error())
		return err.Error()
	}
	return ""
}

func (a *App) RldevList(seenFile string) string {
	a.log("========================================")
	a.log("  RLdev - Liste SEEN.txt")
	a.log("========================================")

	if err := required("SEEN.txt", seenFile); err != nil {
		return a.failIf(err)
	}
	if err := a.runTool("kprl", "-l", seenFile); err != nil {
		return err.Error()
	}
	return ""
}

func (a *App) RldevDisassemble(seenFile, kfnFile, encoding, gameID string, debugInfo bool, outputDir string) string {
	a.log("========================================")
	a.log("  RLdev - Desassemblage SEEN.txt")
	a.log("========================================")

	if err := required("SEEN.txt", seenFile); err != nil {
		return a.failIf(err)
	}
	if err := required("dossier de sortie", outputDir); err != nil {
		return a.failIf(err)
	}
	closeLog := a.startLogFile(outputDir, "kprl-disasm")
	defer closeLog()

	if encoding == "" {
		encoding = "UTF-8"
	}
	if kfnFile == "" {
		kfnFile = a.findKFN()
	}
	if err := required("KFN", kfnFile); err != nil {
		return a.failIf(err)
	}

	args := []string{"-d", "-v", "1", "-e", encoding, "-o", outputDir}
	a.log("KFN: " + kfnFile)
	args = append(args, "-kfn", kfnFile)
	if gameID != "" {
		args = append(args, "-G", gameID)
	}
	if debugInfo {
		args = append(args, "-g")
		a.log("Sources debug RealLive: oui (-g / #line)")
	}
	args = append(args, seenFile)

	if err := a.runTool("kprl", args...); err != nil {
		return err.Error()
	}
	a.logOK("Desassemblage termine.")
	return ""
}

func (a *App) RldevExtract(seenFile, outputDir string) string {
	a.log("========================================")
	a.log("  RLdev - Extraction brute SEEN.txt")
	a.log("========================================")

	if err := required("SEEN.txt", seenFile); err != nil {
		return a.failIf(err)
	}
	if err := required("dossier de sortie", outputDir); err != nil {
		return a.failIf(err)
	}

	if err := a.runTool("kprl", "-x", "-v", "1", "-o", outputDir, seenFile); err != nil {
		return err.Error()
	}
	a.logOK("Extraction terminee.")
	return ""
}

func (a *App) RldevArchive(outputSeen, inputDir, templateSeen string) string {
	a.log("========================================")
	a.log("  RLdev - Reconstruction SEEN.txt")
	a.log("========================================")

	if err := required("SEEN.txt de sortie", outputSeen); err != nil {
		return a.failIf(err)
	}
	if err := required("dossier d'entree", inputDir); err != nil {
		return a.failIf(err)
	}

	seen := map[string]bool{}
	var files []string
	for _, pattern := range []string{"*.TXT", "*.txt", "*.AVG", "*.avg"} {
		matches, _ := filepath.Glob(filepath.Join(inputDir, pattern))
		for _, file := range matches {
			key := strings.ToLower(file)
			if !seen[key] {
				seen[key] = true
				files = append(files, file)
			}
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return a.failIf(fmt.Errorf("aucun fichier .TXT ou .avg trouve dans %s", inputDir))
	}

	args := []string{"-a"}
	if strings.TrimSpace(templateSeen) != "" {
		args = append(args, "-template", templateSeen)
		a.log("Template SEEN.txt: " + templateSeen)
	}
	args = append(args, outputSeen)
	args = append(args, files...)
	if err := a.runTool("kprl", args...); err != nil {
		return err.Error()
	}
	a.logOK(fmt.Sprintf("Archive reconstruite avec %d fichier(s).", len(files)))
	return ""
}

func appendTransformArgs(args []string, outputTransform string, forceTransform bool) []string {
	outputTransform = strings.TrimSpace(outputTransform)
	hasTransform := outputTransform != "" && !strings.EqualFold(outputTransform, "NONE")
	if hasTransform {
		args = append(args, "-x", outputTransform)
	}
	if hasTransform && forceTransform {
		args = append(args, "--force-transform")
	}
	return args
}

func (a *App) RldevCompile(orgFile, kfnFile, gameexe, interpreter, targetVersion, encoding, outputTransform string, forceTransform bool, outputDir string) string {
	a.log("========================================")
	a.log("  RLdev - Compilation script")
	a.log("========================================")

	if err := required("script .org/.ke/.avg", orgFile); err != nil {
		return a.failIf(err)
	}
	if err := required("dossier de sortie", outputDir); err != nil {
		return a.failIf(err)
	}
	closeLog := a.startLogFile(outputDir, "rlc-compile")
	defer closeLog()

	if isAVG32SourceFile(orgFile) {
		if err := a.compileAVG32Source(orgFile, outputDir, outputTransform, forceTransform); err != nil {
			return err.Error()
		}
		a.logOK("Compilation AVG32 terminee.")
		return ""
	}

	if encoding == "" {
		encoding = "UTF-8"
	}
	if kfnFile == "" {
		kfnFile = a.findKFN()
	}
	if err := required("KFN", kfnFile); err != nil {
		return a.failIf(err)
	}
	interpreter = a.resolveInterpreter(gameexe, interpreter)

	args := []string{"-v", "-e", encoding, "-d", outputDir}
	args = appendTransformArgs(args, outputTransform, forceTransform)
	args = append(args, "-K", kfnFile)
	if gameexe != "" {
		args = append(args, "-i", gameexe)
	}
	if interpreter != "" {
		args = append(args, "-I", interpreter)
	}
	targetVersion = strings.TrimSpace(targetVersion)
	if targetVersion != "" {
		args = append(args, "--target-version", targetVersion)
		a.log("Version RealLive forcee: " + targetVersion)
	}
	args = append(args, orgFile)

	if err := a.runTool("rlc", args...); err != nil {
		return err.Error()
	}
	a.logOK("Compilation terminee.")
	return ""
}

func (a *App) RldevCompileBatch(inputDir, kfnFile, gameexe, interpreter, targetVersion, encoding, outputTransform string, forceTransform bool, outputDir string) string {
	a.log("========================================")
	a.log("  RLdev - Compilation batch scripts")
	a.log("========================================")

	if err := required("dossier d'entree", inputDir); err != nil {
		return a.failIf(err)
	}
	if err := required("dossier de sortie", outputDir); err != nil {
		return a.failIf(err)
	}
	closeLog := a.startLogFile(outputDir, "rlc-batch")
	defer closeLog()

	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return a.failIf(fmt.Errorf("lecture du dossier impossible: %w", err))
	}

	var sources []string
	hasKepago := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".org" || ext == ".ke" || ext == ".avg" {
			sources = append(sources, entry.Name())
			if ext == ".org" || ext == ".ke" {
				hasKepago = true
			}
		}
	}
	sort.Strings(sources)
	if len(sources) == 0 {
		return a.failIf(fmt.Errorf("aucun fichier .org, .ke ou .avg trouve dans %s", inputDir))
	}

	if hasKepago {
		if encoding == "" {
			encoding = "UTF-8"
		}
		if kfnFile == "" {
			kfnFile = a.findKFN()
		}
		if err := required("KFN", kfnFile); err != nil {
			return a.failIf(err)
		}
		interpreter = a.resolveInterpreter(gameexe, interpreter)
	}

	okCount := 0
	errCount := 0
	for i, name := range sources {
		base := strings.TrimSuffix(name, filepath.Ext(name))
		inputFile := filepath.Join(inputDir, name)
		a.log(fmt.Sprintf("[%d/%d] %s", i+1, len(sources), name))

		if isAVG32SourceFile(inputFile) {
			if err := a.compileAVG32Source(inputFile, outputDir, outputTransform, forceTransform); err != nil {
				errCount++
				a.logError(fmt.Sprintf("%s: %v", name, err))
				continue
			}
			okCount++
			continue
		}

		args := []string{"-v", "-e", encoding, "-d", outputDir, "-o", base}
		args = appendTransformArgs(args, outputTransform, forceTransform)
		args = append(args, "-K", kfnFile)
		if gameexe != "" {
			args = append(args, "-i", gameexe)
		}
		if interpreter != "" {
			args = append(args, "-I", interpreter)
		}
		targetVersion = strings.TrimSpace(targetVersion)
		if targetVersion != "" {
			args = append(args, "--target-version", targetVersion)
		}
		args = append(args, inputFile)

		if err := a.runTool("rlc", args...); err != nil {
			errCount++
			a.logError(fmt.Sprintf("%s: %v", name, err))
			continue
		}
		okCount++
	}

	result := fmt.Sprintf("%d fichier(s) compile(s), %d erreur(s)", okCount, errCount)
	if errCount > 0 {
		a.logError(result)
		return result
	}
	a.logOK(result)
	return ""
}

func isAVG32SourceFile(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".avg")
}

func (a *App) compileAVG32Source(avgFile, outputDir, outputTransform string, forceTransform bool) error {
	args := []string{"-c", "-t", "AVG32", "-v", "1", "-o", outputDir}
	args = appendKPRLTransformArgs(args, outputTransform, forceTransform)
	args = append(args, avgFile)
	return a.runTool("kprl", args...)
}

func appendKPRLTransformArgs(args []string, outputTransform string, forceTransform bool) []string {
	outputTransform = strings.TrimSpace(outputTransform)
	hasTransform := outputTransform != "" && !strings.EqualFold(outputTransform, "NONE")
	if hasTransform {
		args = append(args, "-transform-output", outputTransform)
	}
	if hasTransform && forceTransform {
		args = append(args, "-force-transform")
	}
	return args
}

func orgTextBatchFiles(inputDir string) ([]string, error) {
	return assetBatchFilesAny(inputDir, ".org", ".ke")
}

func (a *App) RldevOrgTextExport(orgInput, outputDir, encoding string, batch bool) string {
	a.log("========================================")
	a.log("  RLdev - Export texte ORG/KE")
	a.log("========================================")

	label := "fichier .org/.ke"
	if batch {
		label = "dossier .org/.ke"
	}
	if err := required(label, orgInput); err != nil {
		return a.failIf(err)
	}
	if err := required("dossier de sortie", outputDir); err != nil {
		return a.failIf(err)
	}
	closeLog := a.startLogFile(outputDir, "orgtext-export")
	defer closeLog()
	if encoding == "" {
		encoding = "UTF-8"
	}

	if batch {
		files, err := orgTextBatchFiles(orgInput)
		if err != nil {
			return a.failIf(err)
		}
		a.log(fmt.Sprintf("Batch ORG/KE: %d fichier(s)", len(files)))
		for i, file := range files {
			a.log(fmt.Sprintf("[%d/%d] %s", i+1, len(files), filepath.Base(file)))
			if err := a.runTool("rlc", "--text-export", "-e", encoding, "-d", outputDir, file); err != nil {
				return err.Error()
			}
		}
		a.logOK("Export texte termine.")
		return ""
	}

	if err := a.runTool("rlc", "--text-export", "-e", encoding, "-d", outputDir, orgInput); err != nil {
		return err.Error()
	}
	a.logOK("Export texte termine.")
	return ""
}

func (a *App) RldevOrgTextImport(orgInput, utfInput, outputDir, encoding string, batch bool) string {
	a.log("========================================")
	a.log("  RLdev - Import texte ORG/KE")
	a.log("========================================")

	orgLabel := "fichier .org/.ke"
	utfLabel := "fichier .utf"
	if batch {
		orgLabel = "dossier .org/.ke"
		utfLabel = "dossier .utf"
	}
	if err := required(orgLabel, orgInput); err != nil {
		return a.failIf(err)
	}
	if err := required(utfLabel, utfInput); err != nil {
		return a.failIf(err)
	}
	if err := required("dossier de sortie", outputDir); err != nil {
		return a.failIf(err)
	}
	closeLog := a.startLogFile(outputDir, "orgtext-import")
	defer closeLog()
	if encoding == "" {
		encoding = "UTF-8"
	}

	if batch {
		files, err := orgTextBatchFiles(orgInput)
		if err != nil {
			return a.failIf(err)
		}
		a.log(fmt.Sprintf("Batch ORG/KE: %d fichier(s)", len(files)))
		for i, file := range files {
			base := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
			utfFile := filepath.Join(utfInput, base+".utf")
			if info, err := os.Stat(utfFile); err != nil || info.IsDir() {
				a.log(fmt.Sprintf("[SKIP] %s: .utf absent", filepath.Base(file)))
				continue
			}
			a.log(fmt.Sprintf("[%d/%d] %s", i+1, len(files), filepath.Base(file)))
			if err := a.runTool("rlc", "--text-import", "--text-file", utfFile, "-e", encoding, "-d", outputDir, file); err != nil {
				return err.Error()
			}
		}
		a.logOK("Import texte termine.")
		return ""
	}

	if err := a.runTool("rlc", "--text-import", "--text-file", utfInput, "-e", encoding, "-d", outputDir, orgInput); err != nil {
		return err.Error()
	}
	a.logOK("Import texte termine.")
	return ""
}

func assetBatchFiles(inputDir, ext string) ([]string, error) {
	seen := map[string]bool{}
	var files []string
	for _, suffix := range []string{strings.ToLower(ext), strings.ToUpper(ext)} {
		matches, err := filepath.Glob(filepath.Join(inputDir, "*"+suffix))
		if err != nil {
			return nil, err
		}
		for _, file := range matches {
			if !seen[file] {
				seen[file] = true
				files = append(files, file)
			}
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("aucun fichier %s trouve dans %s", ext, inputDir)
	}
	return files, nil
}

func assetBatchFilesAny(inputDir string, exts ...string) ([]string, error) {
	seen := map[string]bool{}
	var files []string
	for _, ext := range exts {
		for _, suffix := range []string{strings.ToLower(ext), strings.ToUpper(ext)} {
			matches, err := filepath.Glob(filepath.Join(inputDir, "*"+suffix))
			if err != nil {
				return nil, err
			}
			for _, file := range matches {
				if !seen[file] {
					seen[file] = true
					files = append(files, file)
				}
			}
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("aucun fichier %s trouve dans %s", strings.Join(exts, "/"), inputDir)
	}
	return files, nil
}

func appendG00MetadataArg(args []string, xmlPath string) []string {
	xmlPath = strings.TrimSpace(xmlPath)
	if xmlPath != "" {
		args = append(args, "-m", xmlPath)
	}
	return args
}

func appendG00FormatArg(args []string, g00Format string) []string {
	g00Format = strings.TrimSpace(g00Format)
	if g00Format != "" && !strings.EqualFold(g00Format, "auto") {
		args = append(args, "-g", g00Format)
	}
	return args
}

func (a *App) RldevG00ToPng(g00Input, outputDir, xmlPath string, batch bool) string {
	a.log("========================================")
	a.log("  RLdev - G00 vers PNG")
	a.log("========================================")

	label := "fichier G00"
	if batch {
		label = "dossier G00"
	}
	if err := required(label, g00Input); err != nil {
		return a.failIf(err)
	}
	if err := required("dossier de sortie", outputDir); err != nil {
		return a.failIf(err)
	}

	args := []string{"-v", "-d", outputDir}
	args = appendG00MetadataArg(args, xmlPath)
	if batch {
		files, err := assetBatchFiles(g00Input, ".g00")
		if err != nil {
			return a.failIf(err)
		}
		a.log(fmt.Sprintf("Batch G00: %d fichier(s)", len(files)))
		args = append(args, "-i", "g00", g00Input)
	} else {
		args = append(args, g00Input)
	}
	if err := a.runTool("vaconv", args...); err != nil {
		return err.Error()
	}
	a.logOK("Conversion terminee.")
	return ""
}

func (a *App) RldevPngToG00(pngInput, outputDir, xmlPath, g00Format string, batch bool) string {
	a.log("========================================")
	a.log("  RLdev - PNG vers G00")
	a.log("========================================")

	label := "fichier PNG"
	if batch {
		label = "dossier PNG"
	}
	if err := required(label, pngInput); err != nil {
		return a.failIf(err)
	}
	if err := required("dossier de sortie", outputDir); err != nil {
		return a.failIf(err)
	}

	args := []string{"-v"}
	args = appendG00FormatArg(args, g00Format)
	args = appendG00MetadataArg(args, xmlPath)
	if batch {
		files, err := assetBatchFiles(pngInput, ".png")
		if err != nil {
			return a.failIf(err)
		}
		a.log(fmt.Sprintf("Batch PNG: %d fichier(s)", len(files)))
		args = append(args, "-i", "png", "-d", outputDir, pngInput)
	} else {
		base := strings.TrimSuffix(filepath.Base(pngInput), filepath.Ext(pngInput))
		outputFile := filepath.Join(outputDir, base+".g00")
		args = append(args, "-o", outputFile, "-i", pngInput)
	}
	if err := a.runTool("vaconv", args...); err != nil {
		return err.Error()
	}
	a.logOK("Conversion terminee.")
	return ""
}

func (a *App) RldevGanToXml(ganFile, outputDir string) string {
	a.log("========================================")
	a.log("  RLdev - GAN vers XML")
	a.log("========================================")

	if err := required("fichier GAN", ganFile); err != nil {
		return a.failIf(err)
	}
	if err := required("dossier de sortie", outputDir); err != nil {
		return a.failIf(err)
	}

	base := strings.TrimSuffix(filepath.Base(ganFile), filepath.Ext(ganFile))
	outputFile := filepath.Join(outputDir, base+".ganxml")
	if err := a.runTool("rlxml", "-v", "-o", outputFile, ganFile); err != nil {
		return err.Error()
	}
	a.logOK("Conversion terminee: " + outputFile)
	return ""
}

func (a *App) RldevXmlToGan(xmlFile, outputDir string) string {
	a.log("========================================")
	a.log("  RLdev - XML vers GAN")
	a.log("========================================")

	if err := required("fichier GANXML", xmlFile); err != nil {
		return a.failIf(err)
	}
	if err := required("dossier de sortie", outputDir); err != nil {
		return a.failIf(err)
	}

	base := strings.TrimSuffix(filepath.Base(xmlFile), filepath.Ext(xmlFile))
	outputFile := filepath.Join(outputDir, base+".gan")
	if err := a.runTool("rlxml", "-v", "-o", outputFile, xmlFile); err != nil {
		return err.Error()
	}
	a.logOK("Conversion terminee: " + outputFile)
	return ""
}

func (a *App) RldevNwaToAudio(nwaInput, outputDir, audioFormat string, batch bool) string {
	a.log("========================================")
	a.log("  RLdev - NWA vers audio")
	a.log("========================================")

	label := "fichier NWA"
	if batch {
		label = "dossier NWA"
	}
	if err := required(label, nwaInput); err != nil {
		return a.failIf(err)
	}
	if err := required("dossier de sortie", outputDir); err != nil {
		return a.failIf(err)
	}

	audioFormat = strings.TrimSpace(strings.ToLower(audioFormat))
	if audioFormat == "" {
		audioFormat = "mp3"
	}

	args := []string{"-v", "-audio", audioFormat, "-d", outputDir}
	if batch {
		files, err := assetBatchFiles(nwaInput, ".nwa")
		if err != nil {
			return a.failIf(err)
		}
		a.log(fmt.Sprintf("Batch NWA: %d fichier(s)", len(files)))
		args = append(args, "-i", "nwa", nwaInput)
	} else {
		args = append(args, nwaInput)
	}
	if err := a.runTool("vaconv", args...); err != nil {
		return err.Error()
	}
	a.logOK("Conversion terminee.")
	return ""
}

func (a *App) RldevDatToJson(datInput, outputDir string, batch bool) string {
	a.log("========================================")
	a.log("  RLdev - CGM/TCC vers JSON")
	a.log("========================================")

	label := "fichier CGM/TCC"
	if batch {
		label = "dossier CGM/TCC"
	}
	if err := required(label, datInput); err != nil {
		return a.failIf(err)
	}
	if err := required("dossier de sortie", outputDir); err != nil {
		return a.failIf(err)
	}

	args := []string{"-v", "-d", outputDir}
	if batch {
		files, err := assetBatchFilesAny(datInput, ".cgm", ".tcc")
		if err != nil {
			return a.failIf(err)
		}
		a.log(fmt.Sprintf("Batch DAT: %d fichier(s)", len(files)))
		args = append(args, "-i", "dat", datInput)
	} else {
		args = append(args, datInput)
	}
	if err := a.runTool("vaconv", args...); err != nil {
		return err.Error()
	}
	a.logOK("Conversion terminee.")
	return ""
}

func (a *App) RldevDatJsonToBinary(jsonInput, outputDir string, batch bool) string {
	a.log("========================================")
	a.log("  RLdev - JSON vers CGM/TCC")
	a.log("========================================")

	label := "fichier JSON DAT"
	if batch {
		label = "dossier JSON DAT"
	}
	if err := required(label, jsonInput); err != nil {
		return a.failIf(err)
	}
	if err := required("dossier de sortie", outputDir); err != nil {
		return a.failIf(err)
	}

	args := []string{"-v", "-d", outputDir}
	if batch {
		files, err := assetBatchFiles(jsonInput, ".json")
		if err != nil {
			return a.failIf(err)
		}
		a.log(fmt.Sprintf("Batch JSON DAT: %d fichier(s)", len(files)))
		args = append(args, "-i", "json", jsonInput)
	} else {
		args = append(args, jsonInput)
	}
	if err := a.runTool("vaconv", args...); err != nil {
		return err.Error()
	}
	a.logOK("Conversion terminee.")
	return ""
}

func (a *App) RldevSaveInfo(saveFile string) string {
	a.log("========================================")
	a.log("  RLdev - Infos sauvegarde RealLive")
	a.log("========================================")

	if err := required("fichier .sav", saveFile); err != nil {
		return a.failIf(err)
	}
	if err := a.runTool("rlsave", "info", saveFile); err != nil {
		return err.Error()
	}
	return ""
}

func (a *App) RldevSaveGet(saveFile, refs string) string {
	a.log("========================================")
	a.log("  RLdev - Lecture sauvegarde RealLive")
	a.log("========================================")

	if err := required("fichier .sav", saveFile); err != nil {
		return a.failIf(err)
	}
	fields, err := saveArgFields("variables", refs)
	if err != nil {
		return a.failIf(err)
	}
	args := append([]string{"get", saveFile}, fields...)
	if err := a.runTool("rlsave", args...); err != nil {
		return err.Error()
	}
	return ""
}

func (a *App) RldevSaveSet(saveFile, assignments string, backup bool) string {
	a.log("========================================")
	a.log("  RLdev - Edition sauvegarde RealLive")
	a.log("========================================")

	if err := required("fichier .sav", saveFile); err != nil {
		return a.failIf(err)
	}
	fields, err := saveArgFields("assignations", assignments)
	if err != nil {
		return a.failIf(err)
	}
	args := []string{"set"}
	if !backup {
		args = append(args, "-no-backup")
	}
	args = append(args, saveFile)
	args = append(args, fields...)
	if err := a.runTool("rlsave", args...); err != nil {
		return err.Error()
	}
	a.logOK("Sauvegarde mise a jour.")
	return ""
}

func (a *App) RldevSaveDump(saveFile string, includeAll, jsonOutput bool) string {
	a.log("========================================")
	a.log("  RLdev - Dump sauvegarde RealLive")
	a.log("========================================")

	if err := required("fichier .sav", saveFile); err != nil {
		return a.failIf(err)
	}
	args := []string{"dump"}
	if includeAll {
		args = append(args, "-all")
	}
	if jsonOutput {
		args = append(args, "-json")
	}
	args = append(args, saveFile)
	if err := a.runTool("rlsave", args...); err != nil {
		return err.Error()
	}
	return ""
}

func saveArgFields(label, value string) ([]string, error) {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return nil, fmt.Errorf("%s requis", label)
	}
	return fields, nil
}

func (a *App) RldevBabelPrepareRuntime(babelRoot, gameDir, version, dllMode, nameEnc string, updateGameexe bool) string {
	a.log("========================================")
	a.log("  RLdev - Preparation runtime Babel")
	a.log("========================================")

	if err := required("dossier BABEL", babelRoot); err != nil {
		return a.failIf(err)
	}
	if err := required("dossier du jeu", gameDir); err != nil {
		return a.failIf(err)
	}
	if !isBabelRoot(babelRoot) {
		return a.failIf(fmt.Errorf("dossier BABEL invalide: %s", babelRoot))
	}
	if info, err := os.Stat(gameDir); err != nil || !info.IsDir() {
		return a.failIf(fmt.Errorf("dossier du jeu invalide: %s", gameDir))
	}

	version = strings.TrimSpace(version)
	dllName := resolveBabelDLLName(version, dllMode)
	srcDLL := filepath.Join(babelRoot, "rtl", dllName)
	dstDLL := filepath.Join(gameDir, dllName)
	if err := copyFile(srcDLL, dstDLL); err != nil {
		return a.failIf(err)
	}
	a.logOK("DLL copiee: " + dstDLL)

	if version != "" {
		mapSrc := filepath.Join(babelRoot, "rtl", version+".map")
		if info, err := os.Stat(mapSrc); err == nil && !info.IsDir() {
			mapDst := filepath.Join(gameDir, version+".map")
			if err := copyFile(mapSrc, mapDst); err != nil {
				return a.failIf(err)
			}
			a.logOK("Map copiee: " + mapDst)
		} else {
			a.log("Map non trouvee pour " + version + " (utiliser rlbabel-genmap si cette version n'est pas integree a la DLL).")
		}
	}

	if updateGameexe {
		gameexe := filepath.Join(gameDir, "GAMEEXE.INI")
		if err := updateBabelGameexe(gameexe, dllName, nameEnc); err != nil {
			return a.failIf(err)
		}
		a.logOK("GAMEEXE.INI mis a jour: " + gameexe)
	} else {
		a.log("GAMEEXE.INI laisse intact.")
	}

	if dllName == "rlBabelF.dll" {
		a.log("Note: rlBabelF sert aux vieux RealLive 1.2.x; il faut charger la DLL au demarrage avec LoadDLL(0, 'rlBabelF') ou via rlcInit().")
	} else {
		a.log("Note: pour RealLive 1.2.5+, GAMEEXE doit contenir une ligne #DLL.xxx = \"rlBabel\".")
	}
	a.logOK("Preparation Babel terminee.")
	return ""
}

func (a *App) RldevBabelWriteHeader(outputDir string, enableGlosses bool) string {
	a.log("========================================")
	a.log("  RLdev - Header Babel")
	a.log("========================================")

	if err := required("dossier de sortie", outputDir); err != nil {
		return a.failIf(err)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return a.failIf(err)
	}

	var b strings.Builder
	b.WriteString("{- RLdev 2026 Babel helper -}\r\n")
	b.WriteString("#define __DynamicLineation__ = 1\r\n")
	if enableGlosses {
		b.WriteString("#define __EnableGlosses__\r\n")
	}
	b.WriteString("#load 'rlBabel'\r\n")
	path := filepath.Join(outputDir, "global.kh")
	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		return a.failIf(err)
	}
	a.logOK("Header cree: " + path)
	a.log("Copie ces lignes au debut du script a tester, ou dans le header commun du projet.")
	return ""
}

func resolveBabelDLLName(version, dllMode string) string {
	mode := strings.ToLower(strings.TrimSpace(dllMode))
	switch mode {
	case "old", "rlbabelf", "rlbabelf.dll":
		return "rlBabelF.dll"
	case "new", "rlbabel", "rlbabel.dll":
		return "rlBabel.dll"
	}
	if babelVersionBefore125(version) {
		return "rlBabelF.dll"
	}
	return "rlBabel.dll"
}

func babelVersionBefore125(version string) bool {
	parts := strings.Split(strings.TrimSpace(version), ".")
	if len(parts) < 3 {
		return false
	}
	nums := make([]int, 4)
	for i := 0; i < len(nums) && i < len(parts); i++ {
		fmt.Sscanf(parts[i], "%d", &nums[i])
	}
	if nums[0] != 1 {
		return false
	}
	if nums[1] < 2 {
		return true
	}
	if nums[1] > 2 {
		return false
	}
	return nums[2] < 5
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func updateBabelGameexe(path, dllName, nameEnc string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	backup := path + ".babel-" + time.Now().Format("20060102-150405") + ".bak"
	if err := os.WriteFile(backup, data, 0644); err != nil {
		return err
	}
	text := string(data)
	if dllName == "rlBabel.dll" && !regexp.MustCompile(`(?im)^#DLL\.\d{3}\s*=\s*"rlBabel"\s*$`).MatchString(text) {
		next := nextDLLSlot(text)
		text = appendGameexeLine(text, fmt.Sprintf("#DLL.%03d = \"rlBabel\"", next))
	}
	if encLine, ok := babelNameEncLine(nameEnc); ok {
		re := regexp.MustCompile(`(?im)^#NAME_ENC\s*=.*$`)
		if re.MatchString(text) {
			text = re.ReplaceAllString(text, encLine)
		} else {
			text = appendGameexeLine(text, encLine)
		}
	}
	return os.WriteFile(path, []byte(text), 0644)
}

func nextDLLSlot(text string) int {
	re := regexp.MustCompile(`(?im)^#DLL\.(\d{3})\s*=`)
	matches := re.FindAllStringSubmatch(text, -1)
	maxSlot := -1
	for _, m := range matches {
		var slot int
		if _, err := fmt.Sscanf(m[1], "%d", &slot); err == nil && slot > maxSlot {
			maxSlot = slot
		}
	}
	return maxSlot + 1
}

func appendGameexeLine(text, line string) string {
	if text != "" && !strings.HasSuffix(text, "\n") && !strings.HasSuffix(text, "\r") {
		text += "\r\n"
	}
	return text + line + "\r\n"
}

func babelNameEncLine(nameEnc string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(nameEnc)) {
	case "", "none":
		return "", false
	case "chinese", "1":
		return "#NAME_ENC = 1", true
	case "western", "2":
		return "#NAME_ENC = 2", true
	case "korean", "3":
		return "#NAME_ENC = 3", true
	default:
		return "", false
	}
}
