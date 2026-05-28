//go:build windows

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	debugProcess         = 0x00000001
	debugOnlyThisProcess = 0x00000002
	createUnicodeEnv     = 0x00000400

	createThreadDebugEvent      = 2
	createProcessDebugEvent     = 3
	exitThreadDebugEvent        = 4
	exitProcessDebugEvent       = 5
	loadDLLDebugEvent           = 6
	unloadDLLDebugEvent         = 7
	outputDebugStringDebugEvent = 8
	ripEvent                    = 9
	exceptionDebugEvent         = 1

	dbgContinue            = 0x00010002
	dbgExceptionNotHandled = 0x80010001

	exceptionBreakpoint = 0x80000003
	exceptionSingleStep = 0x80000004

	waitObject0 = 0x00000000
	waitTimeout = 0x00000102
	stillActive = 259

	errorInvalidHandle = syscall.Errno(6)
	errorSemTimeout    = syscall.Errno(121)
)

var (
	kernel32                   = syscall.NewLazyDLL("kernel32.dll")
	ntdll                      = syscall.NewLazyDLL("ntdll.dll")
	procWaitForDebugEvent      = kernel32.NewProc("WaitForDebugEvent")
	procContinueDebugEvent     = kernel32.NewProc("ContinueDebugEvent")
	procDebugSetKillOnExit     = kernel32.NewProc("DebugSetProcessKillOnExit")
	procWaitForSingleObject    = kernel32.NewProc("WaitForSingleObject")
	procGetExitCodeProcess     = kernel32.NewProc("GetExitCodeProcess")
	procWriteProcessMemory     = kernel32.NewProc("WriteProcessMemory")
	procCloseHandle            = kernel32.NewProc("CloseHandle")
	procNtQueryInformationProc = ntdll.NewProc("NtQueryInformationProcess")
)

type debugEvent struct {
	Code      uint32
	ProcessID uint32
	ThreadID  uint32
	Data      [192]byte
}

type launcher struct {
	start      time.Time
	log        io.Writer
	process    syscall.Handle
	thread     syscall.Handle
	processID  uint32
	eventCount int
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	exe, args, err := resolveTarget(os.Args[1:])
	if err != nil {
		return err
	}
	workdir := filepath.Dir(exe)
	logPath := filepath.Join(workdir, "reallive-debug-"+time.Now().Format("20060102-150405")+".log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("create log: %w", err)
	}
	defer logFile.Close()

	l := &launcher{
		start: time.Now(),
		log:   io.MultiWriter(os.Stdout, logFile),
	}
	l.logf("RealLive debug launcher v2")
	l.logf("exe : %s", exe)
	l.logf("cwd : %s", workdir)
	l.logf("args: %v", args)
	l.logf("log : %s", logPath)
	l.logf("---")

	if err := l.startProcess(exe, args, workdir); err != nil {
		return err
	}
	defer syscall.CloseHandle(l.process)
	defer syscall.CloseHandle(l.thread)

	l.setKillOnExit(false)
	l.hidePEB()
	return l.debugLoop()
}

func resolveTarget(args []string) (string, []string, error) {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "/?") {
		return "", nil, usageError{}
	}
	if len(args) == 0 {
		wd, err := os.Getwd()
		if err != nil {
			return "", nil, err
		}
		exe, err := findRealLive(wd)
		return exe, nil, err
	}
	first := args[0]
	info, err := os.Stat(first)
	if err != nil {
		return "", nil, fmt.Errorf("target not found: %s", first)
	}
	if info.IsDir() {
		exe, err := findRealLive(first)
		return exe, args[1:], err
	}
	exe, err := filepath.Abs(first)
	return exe, args[1:], err
}

type usageError struct{}

func (usageError) Error() string {
	return "usage:\n  drop this launcher next to RealLive.exe and double-click it\n  or pass a path:\n    reallive-debug-launcher.exe <path-to-RealLive.exe>\n    reallive-debug-launcher.exe <path-to-game-folder>"
}

func findRealLive(dir string) (string, error) {
	for _, name := range []string{"RealLive.exe", "RealLiveEn.exe", "REALLIVE.EXE"} {
		p := filepath.Join(dir, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return filepath.Abs(p)
		}
	}
	return "", fmt.Errorf("no RealLive.exe found in %s", dir)
}

func (l *launcher) startProcess(exe string, args []string, workdir string) error {
	cmdline, err := syscall.UTF16FromString(joinCommandLine(append([]string{exe}, args...)))
	if err != nil {
		return err
	}
	app, err := syscall.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	cwd, err := syscall.UTF16PtrFromString(workdir)
	if err != nil {
		return err
	}
	var si syscall.StartupInfo
	var pi syscall.ProcessInformation
	si.Cb = uint32(unsafe.Sizeof(si))
	flags := uint32(debugProcess | debugOnlyThisProcess | createUnicodeEnv)
	if err := syscall.CreateProcess(app, &cmdline[0], nil, nil, false, flags, nil, cwd, &si, &pi); err != nil {
		return fmt.Errorf("CreateProcess: %w", err)
	}
	l.process = pi.Process
	l.thread = pi.Thread
	l.processID = pi.ProcessId
	l.logf("CREATE_PROCESS requested pid=%d tid=%d", pi.ProcessId, pi.ThreadId)
	return nil
}

func joinCommandLine(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = quoteCommandArg(arg)
	}
	return strings.Join(quoted, " ")
}

func quoteCommandArg(arg string) string {
	if arg == "" {
		return `""`
	}
	if !strings.ContainsAny(arg, " \t\n\v\"") {
		return arg
	}
	var b strings.Builder
	b.WriteByte('"')
	backslashes := 0
	for _, r := range arg {
		switch r {
		case '\\':
			backslashes++
		case '"':
			b.WriteString(strings.Repeat("\\", backslashes*2+1))
			b.WriteRune(r)
			backslashes = 0
		default:
			if backslashes > 0 {
				b.WriteString(strings.Repeat("\\", backslashes))
				backslashes = 0
			}
			b.WriteRune(r)
		}
	}
	if backslashes > 0 {
		b.WriteString(strings.Repeat("\\", backslashes*2))
	}
	b.WriteByte('"')
	return b.String()
}

func (l *launcher) debugLoop() error {
	var lastHeartbeat time.Time
	for {
		ev, errno := waitDebugEvent(1000)
		if errno != 0 {
			if isTimeout(errno) {
				if l.processExited() {
					l.logExitCode("process exited while waiting")
					l.logf("--- launcher done ---")
					return nil
				}
				if time.Since(lastHeartbeat) >= 5*time.Second {
					l.logf("heartbeat: process alive, debug events=%d", l.eventCount)
					lastHeartbeat = time.Now()
				}
				continue
			}
			if errno == errorInvalidHandle {
				if l.processExited() {
					l.logExitCode("debug handle closed after process exit")
				} else {
					l.logf("WaitForDebugEvent: invalid handle, process still alive; leaving game running")
					l.waitForProcess()
				}
				l.logf("--- launcher done ---")
				return nil
			}
			l.logf("WaitForDebugEvent failed: %v", errno)
			if !l.processExited() {
				l.waitForProcess()
			}
			l.logf("--- launcher done ---")
			return nil
		}

		lastHeartbeat = time.Now()
		l.eventCount++
		status := l.handleEvent(ev)
		if !continueDebugEvent(ev.ProcessID, ev.ThreadID, status) {
			l.logf("ContinueDebugEvent failed for pid=%d tid=%d", ev.ProcessID, ev.ThreadID)
		}
		if ev.Code == exitProcessDebugEvent {
			l.logf("--- launcher done ---")
			return nil
		}
	}
}

func waitDebugEvent(timeoutMS uint32) (*debugEvent, syscall.Errno) {
	var ev debugEvent
	r1, _, err := procWaitForDebugEvent.Call(uintptr(unsafe.Pointer(&ev)), uintptr(timeoutMS))
	if r1 == 0 {
		if errno, ok := err.(syscall.Errno); ok {
			if errno == 0 {
				return nil, errorSemTimeout
			}
			return nil, errno
		}
		return nil, errorSemTimeout
	}
	return &ev, 0
}

func isTimeout(errno syscall.Errno) bool {
	return errno == errorSemTimeout || errno == syscall.Errno(waitTimeout)
}

func continueDebugEvent(pid, tid, status uint32) bool {
	r1, _, _ := procContinueDebugEvent.Call(uintptr(pid), uintptr(tid), uintptr(status))
	return r1 != 0
}

func (l *launcher) handleEvent(ev *debugEvent) uint32 {
	data := ev.Data[:]
	switch ev.Code {
	case exceptionDebugEvent:
		code := u32(data, 0)
		addr := exceptionAddress(data)
		firstChance := exceptionFirstChance(data)
		l.logf("EXCEPTION pid=%d tid=%d code=0x%08x addr=0x%08x firstChance=%d", ev.ProcessID, ev.ThreadID, code, addr, firstChance)
		if code == exceptionBreakpoint || code == exceptionSingleStep {
			return dbgContinue
		}
		return dbgExceptionNotHandled
	case createThreadDebugEvent:
		l.logf("CREATE_THREAD pid=%d tid=%d start=0x%08x", ev.ProcessID, ev.ThreadID, createThreadStart(data))
	case createProcessDebugEvent:
		base, hFile := createProcessInfo(data)
		l.logf("CREATE_PROCESS pid=%d tid=%d image_base=0x%08x", ev.ProcessID, ev.ThreadID, base)
		closeRawHandle(hFile)
		l.hidePEB()
	case exitThreadDebugEvent:
		l.logf("EXIT_THREAD pid=%d tid=%d code=0x%x", ev.ProcessID, ev.ThreadID, u32(data, 0))
	case exitProcessDebugEvent:
		l.logf("EXIT_PROCESS pid=%d tid=%d code=0x%x", ev.ProcessID, ev.ThreadID, u32(data, 0))
	case loadDLLDebugEvent:
		base, hFile := loadDLLInfo(data)
		l.logf("LOAD_DLL pid=%d tid=%d base=0x%08x", ev.ProcessID, ev.ThreadID, base)
		closeRawHandle(hFile)
	case unloadDLLDebugEvent:
		l.logf("UNLOAD_DLL pid=%d tid=%d base=0x%08x", ev.ProcessID, ev.ThreadID, ptr(data, 0))
	case outputDebugStringDebugEvent:
		l.logf("OUTPUT_DEBUG_STRING pid=%d tid=%d len=%d unicode=%d", ev.ProcessID, ev.ThreadID, debugStringLen(data), debugStringUnicode(data))
	case ripEvent:
		l.logf("RIP_EVENT pid=%d tid=%d error=%d type=%d", ev.ProcessID, ev.ThreadID, u32(data, 0), u32(data, 4))
	default:
		l.logf("DEBUG_EVENT code=%d pid=%d tid=%d", ev.Code, ev.ProcessID, ev.ThreadID)
	}
	return dbgContinue
}

func (l *launcher) setKillOnExit(kill bool) {
	var v uintptr
	if kill {
		v = 1
	}
	r1, _, err := procDebugSetKillOnExit.Call(v)
	if r1 == 0 {
		l.logf("DebugSetProcessKillOnExit failed: %v", err)
		return
	}
	l.logf("DebugSetProcessKillOnExit(%v)", kill)
}

func (l *launcher) hidePEB() {
	peb, err := queryPEB(l.process)
	if err != nil {
		l.logf("PEB anti-debug patch skipped: %v", err)
		return
	}
	if err := writeProcessMemory(l.process, peb+2, []byte{0}); err != nil {
		l.logf("PEB BeingDebugged patch failed: %v", err)
	} else {
		l.logf("PEB BeingDebugged cleared at 0x%08x", peb+2)
	}
	ntGlobalFlagOffset := uintptr(0x68)
	if unsafe.Sizeof(uintptr(0)) == 8 {
		ntGlobalFlagOffset = 0xbc
	}
	if err := writeProcessMemory(l.process, peb+ntGlobalFlagOffset, []byte{0, 0, 0, 0}); err != nil {
		l.logf("PEB NtGlobalFlag patch failed: %v", err)
	} else {
		l.logf("PEB NtGlobalFlag cleared at 0x%08x", peb+ntGlobalFlagOffset)
	}
}

func queryPEB(process syscall.Handle) (uintptr, error) {
	var pbi [6]uintptr
	var retLen uintptr
	status, _, _ := procNtQueryInformationProc.Call(
		uintptr(process),
		0,
		uintptr(unsafe.Pointer(&pbi[0])),
		uintptr(len(pbi))*unsafe.Sizeof(uintptr(0)),
		uintptr(unsafe.Pointer(&retLen)),
	)
	if status != 0 {
		return 0, fmt.Errorf("NtQueryInformationProcess status=0x%x", status)
	}
	if pbi[1] == 0 {
		return 0, errors.New("empty PEB address")
	}
	return pbi[1], nil
}

func writeProcessMemory(process syscall.Handle, address uintptr, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	var written uintptr
	r1, _, err := procWriteProcessMemory.Call(
		uintptr(process),
		address,
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(len(data)),
		uintptr(unsafe.Pointer(&written)),
	)
	if r1 == 0 {
		return err
	}
	if written != uintptr(len(data)) {
		return fmt.Errorf("short write %d/%d", written, len(data))
	}
	return nil
}

func (l *launcher) processExited() bool {
	r1, _, _ := procWaitForSingleObject.Call(uintptr(l.process), 0)
	return r1 == waitObject0
}

func (l *launcher) waitForProcess() {
	last := time.Now()
	for {
		r1, _, _ := procWaitForSingleObject.Call(uintptr(l.process), 1000)
		if r1 == waitObject0 {
			l.logExitCode("process exited")
			return
		}
		if time.Since(last) >= 5*time.Second {
			l.logf("waiting for process to exit")
			last = time.Now()
		}
	}
}

func (l *launcher) logExitCode(prefix string) {
	var code uint32
	r1, _, err := procGetExitCodeProcess.Call(uintptr(l.process), uintptr(unsafe.Pointer(&code)))
	if r1 == 0 {
		l.logf("%s; GetExitCodeProcess failed: %v", prefix, err)
		return
	}
	if code == stillActive {
		l.logf("%s; exit code still active", prefix)
		return
	}
	l.logf("%s; exit code=0x%x", prefix, code)
}

func (l *launcher) logf(format string, args ...interface{}) {
	elapsed := time.Since(l.start)
	fmt.Fprintf(l.log, "[%9s] %s\n", formatDuration(elapsed), fmt.Sprintf(format, args...))
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.3fs", d.Seconds())
}

func ptr(data []byte, off int) uintptr {
	if unsafe.Sizeof(uintptr(0)) == 8 {
		return uintptr(binary.LittleEndian.Uint64(data[off:]))
	}
	return uintptr(binary.LittleEndian.Uint32(data[off:]))
}

func u32(data []byte, off int) uint32 {
	return binary.LittleEndian.Uint32(data[off:])
}

func exceptionAddress(data []byte) uintptr {
	if unsafe.Sizeof(uintptr(0)) == 8 {
		return ptr(data, 16)
	}
	return ptr(data, 12)
}

func exceptionFirstChance(data []byte) uint32 {
	if unsafe.Sizeof(uintptr(0)) == 8 {
		return u32(data, 152)
	}
	return u32(data, 80)
}

func createThreadStart(data []byte) uintptr {
	if unsafe.Sizeof(uintptr(0)) == 8 {
		return ptr(data, 16)
	}
	return ptr(data, 8)
}

func createProcessInfo(data []byte) (base uintptr, hFile uintptr) {
	if unsafe.Sizeof(uintptr(0)) == 8 {
		return ptr(data, 24), ptr(data, 0)
	}
	return ptr(data, 12), ptr(data, 0)
}

func loadDLLInfo(data []byte) (base uintptr, hFile uintptr) {
	if unsafe.Sizeof(uintptr(0)) == 8 {
		return ptr(data, 8), ptr(data, 0)
	}
	return ptr(data, 4), ptr(data, 0)
}

func debugStringUnicode(data []byte) uint32 {
	if unsafe.Sizeof(uintptr(0)) == 8 {
		return u32(data, 8)
	}
	return u32(data, 4)
}

func debugStringLen(data []byte) uint32 {
	if unsafe.Sizeof(uintptr(0)) == 8 {
		return u32(data, 12)
	}
	return u32(data, 8)
}

func closeRawHandle(h uintptr) {
	if h == 0 || h == ^uintptr(0) {
		return
	}
	procCloseHandle.Call(h)
}
