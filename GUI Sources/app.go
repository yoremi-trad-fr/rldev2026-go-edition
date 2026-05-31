package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

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

	filesUpper, _ := filepath.Glob(filepath.Join(inputDir, "*.TXT"))
	filesLower, _ := filepath.Glob(filepath.Join(inputDir, "*.txt"))
	seen := map[string]bool{}
	files := make([]string, 0, len(filesUpper)+len(filesLower))
	for _, file := range append(filesUpper, filesLower...) {
		key := strings.ToLower(file)
		if !seen[key] {
			seen[key] = true
			files = append(files, file)
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return a.failIf(fmt.Errorf("aucun fichier .TXT trouve dans %s", inputDir))
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

func (a *App) RldevCompile(orgFile, kfnFile, gameexe, interpreter, encoding, outputTransform string, forceTransform bool, outputDir string) string {
	a.log("========================================")
	a.log("  RLdev - Compilation Kepago")
	a.log("========================================")

	if err := required("script .org/.ke", orgFile); err != nil {
		return a.failIf(err)
	}
	if err := required("dossier de sortie", outputDir); err != nil {
		return a.failIf(err)
	}
	closeLog := a.startLogFile(outputDir, "rlc-compile")
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
	args = append(args, orgFile)

	if err := a.runTool("rlc", args...); err != nil {
		return err.Error()
	}
	a.logOK("Compilation terminee.")
	return ""
}

func (a *App) RldevCompileBatch(inputDir, kfnFile, gameexe, interpreter, encoding, outputTransform string, forceTransform bool, outputDir string) string {
	a.log("========================================")
	a.log("  RLdev - Compilation batch Kepago")
	a.log("========================================")

	if err := required("dossier d'entree", inputDir); err != nil {
		return a.failIf(err)
	}
	if err := required("dossier de sortie", outputDir); err != nil {
		return a.failIf(err)
	}
	closeLog := a.startLogFile(outputDir, "rlc-batch")
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
	interpreter = a.resolveInterpreter(gameexe, interpreter)

	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return a.failIf(fmt.Errorf("lecture du dossier impossible: %w", err))
	}

	var sources []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".org" || ext == ".ke" {
			sources = append(sources, entry.Name())
		}
	}
	sort.Strings(sources)
	if len(sources) == 0 {
		return a.failIf(fmt.Errorf("aucun fichier .org ou .ke trouve dans %s", inputDir))
	}

	okCount := 0
	errCount := 0
	for i, name := range sources {
		base := strings.TrimSuffix(name, filepath.Ext(name))
		inputFile := filepath.Join(inputDir, name)
		a.log(fmt.Sprintf("[%d/%d] %s", i+1, len(sources), name))

		args := []string{"-v", "-e", encoding, "-d", outputDir, "-o", base}
		args = appendTransformArgs(args, outputTransform, forceTransform)
		args = append(args, "-K", kfnFile)
		if gameexe != "" {
			args = append(args, "-i", gameexe)
		}
		if interpreter != "" {
			args = append(args, "-I", interpreter)
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

func g00BatchFiles(inputDir, ext string) ([]string, error) {
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
		files, err := g00BatchFiles(g00Input, ".g00")
		if err != nil {
			return a.failIf(err)
		}
		a.log(fmt.Sprintf("Batch G00: %d fichier(s)", len(files)))
		args = append(args, files...)
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
		files, err := g00BatchFiles(pngInput, ".png")
		if err != nil {
			return a.failIf(err)
		}
		a.log(fmt.Sprintf("Batch PNG: %d fichier(s)", len(files)))
		args = append(args, "-d", outputDir)
		args = append(args, files...)
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
