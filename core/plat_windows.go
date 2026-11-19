//go:build windows

package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	shell32                   = syscall.NewLazyDLL("shell32.dll")
	procQueryFullProcessImage = kernel32.NewProc("QueryFullProcessImageNameW")
	procWaitForSingleObject   = kernel32.NewProc("WaitForSingleObject")
	procGetExitCodeProcess    = kernel32.NewProc("GetExitCodeProcess")
	procShellExecuteExW       = shell32.NewProc("ShellExecuteExW")
)

const createNoWindow = 0x08000000

func DataDir() string {
	if d := os.Getenv("LOCALAPPDATA"); d != "" {
		return filepath.Join(d, "Cruveno")
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "AppData", "Local", "Cruveno")
}

func InstanceDir(suffix string) string { return filepath.Join(DataDir(), "MT5-"+suffix) }

func hidden(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return cmd
}

// programDirs: MetaTrader-5-Installationen unter Program Files (2 Ebenen tief), ohne eigene Instanzen
func programDirs() []string {
	seen := map[string]bool{}
	var out []string
	for _, b := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramW6432")} {
		if b == "" || seen[strings.ToLower(b)] {
			continue
		}
		seen[strings.ToLower(b)] = true
		l1, _ := os.ReadDir(b)
		for _, d := range l1 {
			if !d.IsDir() {
				continue
			}
			p := filepath.Join(b, d.Name())
			if fileExists(filepath.Join(p, "terminal64.exe")) {
				out = append(out, p)
				continue
			}
			l2, _ := os.ReadDir(p)
			for _, d2 := range l2 {
				p2 := filepath.Join(p, d2.Name())
				if d2.IsDir() && fileExists(filepath.Join(p2, "terminal64.exe")) {
					out = append(out, p2)
				}
			}
		}
	}
	return out
}

func dataRoots() []string {
	return []string{filepath.Join(os.Getenv("APPDATA"), "MetaQuotes", "Terminal")}
}

func originOf(programDir string) string { return programDir }

// installMT5: offizielles Installationsprogramm (Broker oder MetaQuotes) still installieren (/auto, mit Windows-Rueckfrage)
func installMT5(ctx context.Context, br Broker, prog func(p float64, t string)) (string, error) {
	url := nz(br.Win, GenericMT5Win)
	before := map[string]bool{}
	for _, d := range programDirs() {
		before[strings.ToLower(d)] = true
	}
	setup := filepath.Join(os.TempDir(), "cruveno_mt5setup.exe")
	prog(0, "download")
	err := Download(ctx, url, setup, func(done, total int64) {
		if total > 0 {
			prog(float64(done)/float64(total)*40, fmt.Sprintf("dl:%d:%d", done, total))
		}
	})
	if err != nil {
		return "", errors.New("MetaTrader 5 konnte nicht heruntergeladen werden. Bitte Internetverbindung prüfen.")
	}
	defer os.Remove(setup)
	prog(42, "install")
	started := time.Now()
	code, err := runElevatedWait(setup, "/auto")
	if err != nil {
		return "", errors.New("Die Installation von MetaTrader 5 wurde abgebrochen. Bitte die Windows-Rückfrage mit „Ja“ bestätigen.")
	}
	_ = code
	// neuen Programmordner finden
	var dir string
	for i := 0; i < 60 && dir == ""; i++ {
		for _, d := range programDirs() {
			if !before[strings.ToLower(d)] {
				dir = d
				break
			}
		}
		if dir == "" {
			if !sleepCtx(ctx, 2*time.Second) {
				return "", ctx.Err()
			}
		}
	}
	if dir == "" { // schon vorher installiert: neueste Installation nehmen
		var best time.Time
		for _, d := range programDirs() {
			if fi, err := os.Stat(filepath.Join(d, "terminal64.exe")); err == nil && fi.ModTime().After(best) {
				best, dir = fi.ModTime(), d
			}
		}
	}
	if dir == "" {
		return "", errors.New("MetaTrader 5 wurde nicht gefunden. Bitte erneut versuchen.")
	}
	prog(80, "servers")
	// Das Installationsprogramm startet MetaTrader einmal; warten, bis die Serverliste angelegt ist, dann schliessen
	for i := 0; i < 30; i++ {
		if FindServersDat(dataRoots(), dir) != "" && time.Since(started) > 10*time.Second {
			break
		}
		if !sleepCtx(ctx, 2*time.Second) {
			return "", ctx.Err()
		}
	}
	stopByPath(filepath.Join(dir, "terminal64.exe"))
	prog(100, "")
	return dir, nil
}

type shellExecuteInfo struct {
	cbSize         uint32
	fMask          uint32
	hwnd           uintptr
	lpVerb         *uint16
	lpFile         *uint16
	lpParameters   *uint16
	lpDirectory    *uint16
	nShow          int32
	hInstApp       uintptr
	lpIDList       uintptr
	lpClass        *uint16
	hkeyClass      uintptr
	dwHotKey       uint32
	hIconOrMonitor uintptr
	hProcess       syscall.Handle
}

func runElevatedWait(file, params string) (uint32, error) {
	verb, _ := syscall.UTF16PtrFromString("runas")
	f, _ := syscall.UTF16PtrFromString(file)
	p, _ := syscall.UTF16PtrFromString(params)
	sei := shellExecuteInfo{fMask: 0x40 | 0x100, lpVerb: verb, lpFile: f, lpParameters: p, nShow: 1}
	sei.cbSize = uint32(unsafe.Sizeof(sei))
	r, _, e := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&sei)))
	if r == 0 {
		return 0, e
	}
	if sei.hProcess == 0 {
		return 0, nil
	}
	defer syscall.CloseHandle(sei.hProcess)
	procWaitForSingleObject.Call(uintptr(sei.hProcess), uintptr(30*60*1000))
	var code uint32
	procGetExitCodeProcess.Call(uintptr(sei.hProcess), uintptr(unsafe.Pointer(&code)))
	return code, nil
}

// Prozesse: alle terminal64.exe mit vollem Pfad
func terminalPIDs(exe string) []uint32 {
	var out []uint32
	h, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer syscall.CloseHandle(h)
	var pe syscall.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = syscall.Process32First(h, &pe); err == nil; err = syscall.Process32Next(h, &pe) {
		name := syscall.UTF16ToString(pe.ExeFile[:])
		if !strings.EqualFold(name, "terminal64.exe") {
			continue
		}
		ph, e := syscall.OpenProcess(0x1000, false, pe.ProcessID)
		if e != nil {
			continue
		}
		buf := make([]uint16, 1024)
		n := uint32(len(buf))
		r, _, _ := procQueryFullProcessImage.Call(uintptr(ph), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
		syscall.CloseHandle(ph)
		if r != 0 && strings.EqualFold(syscall.UTF16ToString(buf[:n]), exe) {
			out = append(out, pe.ProcessID)
		}
	}
	return out
}

// BootUnix: Beginn der Sitzung. Unter Windows startet der Dienst (--agent --loop) bei jeder Anmeldung ueber den
// Autostart neu; dessen Startzeit ist verlaesslicher als die Laufzeit des Rechners (Schnellstart = Ruhezustand).
var agentStart = time.Now().Unix()

func BootUnix() int64 { return agentStart }

func IsRunning(dir string) bool { return len(terminalPIDs(filepath.Join(dir, "terminal64.exe"))) > 0 }

func stopByPath(exe string) bool {
	pids := terminalPIDs(exe)
	if len(pids) == 0 {
		return false
	}
	for _, pid := range pids { // freundlich schliessen (WM_CLOSE)
		hidden(exec.Command("taskkill", "/PID", fmt.Sprint(pid))).Run()
	}
	for i := 0; i < 30 && len(terminalPIDs(exe)) > 0; i++ {
		time.Sleep(time.Second)
	}
	for _, pid := range terminalPIDs(exe) {
		if h, err := syscall.OpenProcess(0x0001, false, pid); err == nil {
			syscall.TerminateProcess(h, 1)
			syscall.CloseHandle(h)
		}
	}
	return true
}

func StopTerminal(dir string) bool { return stopByPath(filepath.Join(dir, "terminal64.exe")) }

func StartTerminal(st *State) error {
	cmd := exec.Command(filepath.Join(st.Dir, "terminal64.exe"), "/portable", "/config:"+IniPath(st))
	cmd.Dir = st.Dir
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// OpenTerminal: laufendes Fenster nach vorn holen bzw. starten
func OpenTerminal(st *State) error {
	if IsRunning(st.Dir) {
		return nil
	}
	ClearCharts(st.Dir)
	return StartTerminal(st)
}

const runKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
const uninstKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\CruvenoInstaller`

// InstallAgent: Programm in den Datenordner kopieren, Autostart (Benutzer) und Eintrag unter "Apps" anlegen
func InstallAgent() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	dst := filepath.Join(DataDir(), "Cruveno.exe")
	if !strings.EqualFold(self, dst) {
		hidden(exec.Command("taskkill", "/IM", "Cruveno.exe", "/F")).Run()
		time.Sleep(500 * time.Millisecond)
		if err := copyFile(self, dst); err != nil {
			return fmt.Errorf("Dienst konnte nicht eingerichtet werden: %v", err)
		}
	}
	q := `"` + dst + `"`
	cmds := [][]string{
		{"reg", "add", runKey, "/v", "Cruveno", "/t", "REG_SZ", "/d", q + " --agent --loop", "/f"},
		{"reg", "add", uninstKey, "/v", "DisplayName", "/t", "REG_SZ", "/d", "Cruveno AlgoBreak", "/f"},
		{"reg", "add", uninstKey, "/v", "Publisher", "/t", "REG_SZ", "/d", "Cruveno", "/f"},
		{"reg", "add", uninstKey, "/v", "DisplayVersion", "/t", "REG_SZ", "/d", Version, "/f"},
		{"reg", "add", uninstKey, "/v", "DisplayIcon", "/t", "REG_SZ", "/d", dst, "/f"},
		{"reg", "add", uninstKey, "/v", "UninstallString", "/t", "REG_SZ", "/d", q + " --uninstall", "/f"},
		{"reg", "add", uninstKey, "/v", "NoModify", "/t", "REG_DWORD", "/d", "1", "/f"},
		{"reg", "add", uninstKey, "/v", "NoRepair", "/t", "REG_DWORD", "/d", "1", "/f"},
	}
	for _, c := range cmds {
		if out, err := hidden(exec.Command(c[0], c[1:]...)).CombinedOutput(); err != nil {
			return fmt.Errorf("Autostart: %v %s", err, strings.TrimSpace(string(out)))
		}
	}
	// Dienst sofort starten
	cmd := hidden(exec.Command(dst, "--agent", "--loop"))
	if err := cmd.Start(); err == nil {
		cmd.Process.Release()
	}
	return nil
}

func RemoveAgent() error {
	hidden(exec.Command("reg", "delete", runKey, "/v", "Cruveno", "/f")).Run()
	hidden(exec.Command("reg", "delete", uninstKey, "/f")).Run()
	self, _ := os.Executable()
	dst := filepath.Join(DataDir(), "Cruveno.exe")
	if !strings.EqualFold(self, dst) {
		hidden(exec.Command("taskkill", "/IM", "Cruveno.exe", "/F")).Run()
		os.Remove(dst)
	} else {
		// laufende Datei kann sich nicht selbst loeschen: nach dem Beenden entfernen
		hidden(exec.Command("cmd", "/C", "ping -n 3 127.0.0.1 >nul & del /f /q \""+dst+"\"")).Start()
	}
	return nil
}
