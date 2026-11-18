package core

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// QuietHour: keine Neustarts in der Handelszeit des EAs (UTC 20:00-23:59 deckt 22:30-00:00 Uhr MEZ/MESZ ab)
func QuietHour(t time.Time) bool { h := t.UTC().Hour(); return h >= 20 && h <= 23 }

var now = time.Now

// AgentOnce: ein Durchlauf des Hintergrunddienstes fuer alle Lizenzen auf diesem Rechner
func AgentOnce(ctx context.Context) []string {
	var log []string
	if installing() {
		return nil
	}
	for _, st := range AllStates() {
		log = append(log, agentOne(ctx, st)...)
	}
	return log
}

func agentOne(ctx context.Context, st *State) (log []string) {
	add := func(f string, a ...any) { log = append(log, st.Key[:3]+": "+fmt.Sprintf(f, a...)) }
	if !fileExists(filepath.Join(st.Dir, "terminal64.exe")) {
		return
	}
	// Pause: von Hand (beliebiger Inhalt) -> nichts tun; "cloud" -> Betrieb laeuft im FXV-VPS (siehe unten)
	pauseF := filepath.Join(st.Dir, "cruveno_pause.txt")
	pb, perr := os.ReadFile(pauseF)
	cloudPause := perr == nil && strings.TrimSpace(string(pb)) == "cloud"
	if perr == nil && !cloudPause {
		return
	}
	// Passwort aus der Startdatei entfernen, sobald MT5 die Anmeldung gespeichert hat
	if IniHasPassword(st) && EverAuthorized(st.Dir, st.Login) {
		WriteIni(st, "")
		st.AuthOK = true
		add("Passwort aus der Startdatei entfernt")
		Status(ctx, st.Key, "login_ok", "", st.Login)
	}
	restart := false
	c, err := Check(ctx, st.Key, st.Lang)
	// Betrieb in der Cloud: AlgoBreak laeuft im VPS bei FXVirtualNet -> hier beenden, nie zwei Plaetze gleichzeitig
	if err == nil && c.OK && c.StopLocal {
		if IsRunning(st.Dir) {
			StopTerminal(st.Dir)
			add("MetaTrader beendet – AlgoBreak läuft jetzt in der Cloud (FXVirtualNet)")
		}
		if !cloudPause {
			os.WriteFile(pauseF, []byte("cloud"), 0o644)
		}
		if len(log) > 0 {
			Status(ctx, st.Key, "agent", strings.Join(log, "; "), st.Login)
		}
		SaveState(st)
		return
	}
	if cloudPause {
		if err != nil || !c.OK || c.Where == "fxv" {
			return // weiter Cloud oder keine Auskunft: nichts starten
		}
		os.Remove(pauseF)
		add("Betrieb wieder auf diesem Rechner")
	}
	if err == nil && c.OK {
		if p := c.Effective(); !p.Same(st.Prot) {
			st.Prot = p
			if err := WriteSet(st); err == nil {
				st.Pending = addFlag(st.Pending, "set")
				add("Schutz übernommen: %s %s/%s/%s %%", p.Profile, fnum(p.Risk), fnum(p.Day), fnum(p.Tot))
			}
		}
		if c.EaVersion != "" && c.EaVersion != st.EaVersion && !strings.Contains(st.Pending, "ea") {
			tmp := filepath.Join(st.Dir, "MQL5", "Experts", "Cruveno", st.EaFile+".new")
			if _, err := DownloadEA(ctx, st.Key, tmp); err == nil {
				st.Pending = addFlag(st.Pending, "ea")
				st.PendingEA = c.EaVersion
				add("neue EA-Version geladen")
			}
		}
		if c.Online && !st.ReportedOn {
			st.ReportedOn = true
			Status(ctx, st.Key, "online", "", st.Login)
		}
	}
	// Chart-Symbol pruefen: hat MetaTrader fuer st.Symbol keine Kursdaten, aber fuer ein passendes anderes Symbol
	// (z. B. BTCUSD+ statt BTCUSD), Chart darauf umstellen
	if !IniHasPassword(st) {
		var maps []SymMap
		if err == nil && c.OK {
			maps = c.Symbols
		}
		if !fileExists(filepath.Join(historyDir(st.Dir, st.Server), st.Symbol)) {
			if k := KnownSymbol(st.Dir, st.Server, SymbolCandidates(st.Product, st.Server, st.Symbol, maps)); k != "" && k != st.Symbol {
				add("Chart-Symbol %s -> %s", st.Symbol, k)
				st.Symbol = k
				WriteIni(st, "")
				st.Pending = addFlag(st.Pending, "sym")
			}
		}
	}
	running := IsRunning(st.Dir)
	if st.Pending != "" && (!QuietHour(now()) || !running) {
		if running {
			StopTerminal(st.Dir)
		}
		if strings.Contains(st.Pending, "ea") {
			cur := filepath.Join(st.Dir, "MQL5", "Experts", "Cruveno", st.EaFile)
			if os.Rename(cur+".new", cur) == nil {
				st.EaVersion = st.PendingEA
				add("EA aktualisiert")
			}
			st.PendingEA = ""
		}
		st.Pending = ""
		restart = true
	}
	if restart || !IsRunning(st.Dir) {
		ClearCharts(st.Dir)
		if err := StartTerminal(st); err == nil {
			add("MetaTrader gestartet")
		} else {
			add("Start fehlgeschlagen: %v", err)
		}
	}
	// Meldung an den Kundenbereich: bei Aenderungen sofort, sonst stuendlich
	msg := strings.Join(log, "; ")
	if msg != "" || st.LastReport < now().UTC().Add(-time.Hour).Format(time.RFC3339) {
		Status(ctx, st.Key, "agent", nz(msg, "ok"), st.Login)
		st.LastReport = now().UTC().Format(time.RFC3339)
	}
	SaveState(st)
	return
}

func addFlag(s, f string) string {
	if strings.Contains(s, f) {
		return s
	}
	if s == "" {
		return f
	}
	return s + "," + f
}

// AgentLoop: Windows-Autostart (laeuft dauerhaft, alle 5 Minuten). Nur eine Instanz gleichzeitig.
func AgentLoop(ctx context.Context) {
	l, err := net.Listen("tcp", "127.0.0.1:47391")
	if err != nil {
		return // laeuft schon
	}
	defer l.Close()
	for {
		AgentOnce(ctx)
		if !sleepCtx(ctx, 5*time.Minute) {
			return
		}
	}
}

// AgentLog: kurzes Protokoll im Datenordner (max. 256 KB)
func AgentLog(lines []string) {
	if len(lines) == 0 {
		return
	}
	p := filepath.Join(DataDir(), "agent.log")
	if fi, err := os.Stat(p); err == nil && fi.Size() > 256<<10 {
		os.Remove(p)
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	for _, l := range lines {
		fmt.Fprintf(f, "%s  %s\n", now().Format("2006-01-02 15:04:05"), l)
	}
}

// SetInstalling: waehrend die Oberflaeche einrichtet, greift der Dienst nicht ein (max. 20 Min.)
func SetInstalling(on bool) {
	p := filepath.Join(DataDir(), "installing.lock")
	if on {
		os.MkdirAll(DataDir(), 0o700)
		os.WriteFile(p, []byte(now().UTC().Format(time.RFC3339)), 0o600)
	} else {
		os.Remove(p)
	}
}

func installing() bool {
	fi, err := os.Stat(filepath.Join(DataDir(), "installing.lock"))
	return err == nil && time.Since(fi.ModTime()) < 20*time.Minute
}
