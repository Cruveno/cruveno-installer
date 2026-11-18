//go:build darwin

package core

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const mt5App = "/Applications/MetaTrader 5.app"

func home() string { h, _ := os.UserHomeDir(); return h }

func DataDir() string { return filepath.Join(home(), "Library", "Application Support", "Cruveno") }

// Wine-Umgebung der offiziellen MetaTrader-5-App von MetaQuotes
func winePrefix() string {
	return filepath.Join(home(), "Library", "Application Support", "net.metaquotes.wine.metatrader5")
}
func driveC() string         { return filepath.Join(winePrefix(), "drive_c") }
func defaultProgram() string { return filepath.Join(driveC(), "Program Files", "MetaTrader 5") }

// Eigene Instanz liegt in derselben Wine-Umgebung unter C:\Cruveno
func InstanceDir(suffix string) string { return filepath.Join(driveC(), "Cruveno", "MT5-"+suffix) }

// winPath: Mac-Pfad in der Wine-Umgebung -> Windows-Pfad (C:\...)
func winPath(p string) string {
	rel, err := filepath.Rel(driveC(), p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return p
	}
	return `C:\` + strings.ReplaceAll(rel, "/", `\`)
}

func programDirs() []string {
	if fileExists(filepath.Join(defaultProgram(), "terminal64.exe")) {
		return []string{defaultProgram()}
	}
	return nil
}

func dataRoots() []string {
	ms, _ := filepath.Glob(filepath.Join(driveC(), "users", "*", "AppData", "Roaming", "MetaQuotes", "Terminal"))
	return ms
}

func originOf(programDir string) string { return winPath(programDir) }

// wineBin: Wine aus der MetaTrader-App (wine64 bevorzugt)
func wineBin() string {
	var w64, w string
	filepath.Walk(filepath.Join(mt5App, "Contents"), func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || filepath.Base(filepath.Dir(p)) != "bin" {
			return nil
		}
		switch fi.Name() {
		case "wine64":
			if w64 == "" {
				w64 = p
			}
		case "wine":
			if w == "" {
				w = p
			}
		}
		return nil
	})
	return nz(w64, w)
}

func installMT5(ctx context.Context, br Broker, prog func(p float64, t string)) (string, error) {
	if !fileExists(mt5App) {
		url := nz(br.Mac, GenericMT5Mac)
		tmp, _ := os.MkdirTemp("", "cruveno")
		defer os.RemoveAll(tmp)
		dl := filepath.Join(tmp, "mt5.download")
		prog(0, "download")
		if err := Download(ctx, url, dl, func(done, total int64) {
			if total > 0 {
				prog(float64(done)/float64(total)*40, fmt.Sprintf("dl:%d:%d", done, total))
			}
		}); err != nil {
			return "", errors.New("MetaTrader 5 konnte nicht heruntergeladen werden. Bitte Internetverbindung prüfen.")
		}
		pkg := dl
		if isZip(dl) {
			p, err := unzipFind(dl, tmp, ".pkg")
			if err != nil {
				return "", errors.New("Das MetaTrader-Paket ist beschädigt. Bitte erneut versuchen.")
			}
			pkg = p
		} else {
			os.Rename(dl, dl+".pkg")
			pkg = dl + ".pkg"
		}
		prog(42, "install")
		script := fmt.Sprintf(`do shell script "/usr/sbin/installer -pkg %s -target /" with administrator privileges with prompt "Cruveno installiert MetaTrader 5."`, shq(pkg))
		if out, err := exec.CommandContext(ctx, "osascript", "-e", script).CombinedOutput(); err != nil {
			if strings.Contains(string(out), "-128") {
				return "", errors.New("Die Installation wurde abgebrochen. Bitte das Mac-Passwort eingeben, damit MetaTrader 5 installiert werden kann.")
			}
			return "", fmt.Errorf("MetaTrader 5 konnte nicht installiert werden: %s", strings.TrimSpace(string(out)))
		}
	}
	// Beim ersten Start richtet die App ihre Windows-Umgebung ein und laedt das Terminal
	if !fileExists(filepath.Join(defaultProgram(), "terminal64.exe")) {
		prog(55, "firstrun")
		exec.Command("open", "-a", mt5App).Run()
		ok := false
		for i := 0; i < 180; i++ { // bis 6 Min.
			if fileExists(filepath.Join(defaultProgram(), "terminal64.exe")) {
				ok = true
				break
			}
			prog(55+float64(i)/180*35, "firstrun")
			if !sleepCtx(ctx, 2*time.Second) {
				return "", ctx.Err()
			}
		}
		if !ok {
			return "", errors.New("MetaTrader 5 hat die Einrichtung nicht abgeschlossen. Bitte MetaTrader 5 einmal öffnen, warten bis das Fenster erscheint, und den Installer erneut starten.")
		}
		for i := 0; i < 20 && FindServersDat(dataRoots(), originOf(defaultProgram())) == ""; i++ {
			sleepCtx(ctx, 2*time.Second)
		}
		sleepCtx(ctx, 3*time.Second)
		exec.Command("osascript", "-e", `quit app "MetaTrader 5"`).Run()
		sleepCtx(ctx, 3*time.Second)
		exec.Command("pkill", "-f", `Program Files\\MetaTrader 5\\terminal64`).Run()
	}
	prog(100, "")
	return defaultProgram(), nil
}

func shq(s string) string { return `'` + strings.ReplaceAll(s, `'`, `'\''`) + `'` }

func isZip(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	b := make([]byte, 4)
	io.ReadFull(f, b)
	return string(b[:2]) == "PK"
}

func unzipFind(zp, dst, ext string) (string, error) {
	r, err := zip.OpenReader(zp)
	if err != nil {
		return "", err
	}
	defer r.Close()
	found := ""
	for _, f := range r.File {
		name := filepath.Clean(f.Name)
		if strings.HasPrefix(name, "..") || strings.HasPrefix(name, "__MACOSX") {
			continue
		}
		t := filepath.Join(dst, name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(t, 0o755)
			continue
		}
		os.MkdirAll(filepath.Dir(t), 0o755)
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		w, err := os.Create(t)
		if err != nil {
			rc.Close()
			return "", err
		}
		_, err = io.Copy(w, rc)
		rc.Close()
		w.Close()
		if err != nil {
			return "", err
		}
		if found == "" && strings.HasSuffix(strings.ToLower(name), ext) {
			found = t
		}
	}
	if found == "" {
		return "", errors.New("no pkg")
	}
	return found, nil
}

// Prozess der eigenen Instanz: Befehlszeile enthaelt C:\Cruveno\MT5-<suffix>\terminal64.exe
func pattern(dir string) string { return "Cruveno.MT5-" + filepath.Base(dir)[4:] + ".terminal64" }

func IsRunning(dir string) bool { return exec.Command("pgrep", "-f", pattern(dir)).Run() == nil }

func StopTerminal(dir string) bool {
	if !IsRunning(dir) {
		return false
	}
	exec.Command("pkill", "-TERM", "-f", pattern(dir)).Run()
	for i := 0; i < 30 && IsRunning(dir); i++ {
		time.Sleep(time.Second)
	}
	exec.Command("pkill", "-KILL", "-f", pattern(dir)).Run()
	return true
}

func StartTerminal(st *State) error {
	w := wineBin()
	if w == "" {
		return errors.New("MetaTrader 5 (App) nicht gefunden")
	}
	cmd := exec.Command(w, winPath(filepath.Join(st.Dir, "terminal64.exe")), "/portable", "/config:"+winPath(IniPath(st)))
	cmd.Dir = st.Dir
	cmd.Env = append(os.Environ(), "WINEPREFIX="+winePrefix(), "WINEDEBUG=-all")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func OpenTerminal(st *State) error {
	if IsRunning(st.Dir) {
		exec.Command("osascript", "-e", `tell application "System Events" to set frontmost of (first process whose name contains "wine") to true`).Run()
		return nil
	}
	ClearCharts(st.Dir)
	return StartTerminal(st)
}

func plistPath() string {
	return filepath.Join(home(), "Library", "LaunchAgents", "com.cruveno.agent.plist")
}

func InstallAgent() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(DataDir(), 0o700); err != nil {
		return err
	}
	dst := filepath.Join(DataDir(), "cruveno-agent")
	if self != dst {
		if err := copyFile(self, dst+".tmp"); err != nil {
			return fmt.Errorf("Dienst konnte nicht eingerichtet werden: %v", err)
		}
		os.Chmod(dst+".tmp", 0o755)
		os.Rename(dst+".tmp", dst)
		exec.Command("xattr", "-d", "com.apple.quarantine", dst).Run()
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>com.cruveno.agent</string>
<key>ProgramArguments</key><array><string>` + dst + `</string><string>--agent</string></array>
<key>StartInterval</key><integer>300</integer>
<key>RunAtLoad</key><true/>
<key>ProcessType</key><string>Background</string>
<key>StandardOutPath</key><string>` + filepath.Join(DataDir(), "agent.out") + `</string>
<key>StandardErrorPath</key><string>` + filepath.Join(DataDir(), "agent.out") + `</string>
</dict></plist>
`
	os.MkdirAll(filepath.Dir(plistPath()), 0o755)
	if err := os.WriteFile(plistPath(), []byte(plist), 0o644); err != nil {
		return err
	}
	uid := fmt.Sprint(os.Getuid())
	exec.Command("launchctl", "bootout", "gui/"+uid+"/com.cruveno.agent").Run()
	if out, err := exec.Command("launchctl", "bootstrap", "gui/"+uid, plistPath()).CombinedOutput(); err != nil {
		if out2, err2 := exec.Command("launchctl", "load", "-w", plistPath()).CombinedOutput(); err2 != nil {
			return fmt.Errorf("Autostart: %s %s", strings.TrimSpace(string(out)), strings.TrimSpace(string(out2)))
		}
	}
	return nil
}

func RemoveAgent() error {
	uid := fmt.Sprint(os.Getuid())
	exec.Command("launchctl", "bootout", "gui/"+uid+"/com.cruveno.agent").Run()
	os.Remove(plistPath())
	os.Remove(filepath.Join(DataDir(), "cruveno-agent"))
	return nil
}
