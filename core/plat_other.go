//go:build !windows && !darwin

package core

// Nur fuer Tests unter Linux: simuliert MetaTrader ueber eine Marker-Datei.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

func DataDir() string {
	if d := os.Getenv("CRUVENO_DATA"); d != "" {
		return d
	}
	return filepath.Join(os.TempDir(), "cruveno-test")
}
func InstanceDir(suffix string) string { return filepath.Join(DataDir(), "MT5-"+suffix) }
func programDirs() []string            { return nil }
func dataRoots() []string              { return []string{filepath.Join(DataDir(), "AppData")} }
func originOf(p string) string         { return p }
func installMT5(ctx context.Context, br Broker, prog func(float64, string)) (string, error) {
	src := filepath.Join(DataDir(), "fake-mt5")
	if !fileExists(filepath.Join(src, "terminal64.exe")) {
		return "", errors.New("kein MT5")
	}
	prog(100, "")
	return src, nil
}

var Starts int

func IsRunning(dir string) bool { return fileExists(filepath.Join(dir, ".running")) }
func StopTerminal(dir string) bool {
	was := IsRunning(dir)
	os.Remove(filepath.Join(dir, ".running"))
	return was
}

// FakeSyms: Symbole, die der simulierte Broker kennt (MetaTrader legt dafuer Kursdaten an)
var FakeSyms = map[string]bool{"BTCUSD": true}

func StartTerminal(st *State) error {
	Starts++
	if FakeSyms[st.Symbol] {
		os.MkdirAll(filepath.Join(st.Dir, "Bases", st.Server, "history", st.Symbol), 0o755)
	}
	return os.WriteFile(filepath.Join(st.Dir, ".running"), nil, 0o644)
}
func OpenTerminal(st *State) error { return StartTerminal(st) }
func InstallAgent() error          { return nil }
func RemoveAgent() error           { return nil }

// FakeBoot: simulierte Startzeit des Rechners (Tests)
var FakeBoot int64 = 1_700_000_000

func BootUnix() int64 { return FakeBoot }
