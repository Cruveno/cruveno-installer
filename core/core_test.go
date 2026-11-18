//go:build !windows && !darwin

package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf16"
)

func utf16le(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := []byte{0xFF, 0xFE}
	for _, c := range u {
		b = append(b, byte(c), byte(c>>8))
	}
	return b
}

type fakePortal struct {
	mu     sync.Mutex
	prot   *Prot
	eaVer  string
	online bool
	phases []string
	bodies []string
	eaGets int
	terms  bool
}

func (f *fakePortal) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.URL.Path == "/inst/v1/check":
		var b map[string]string
		json.NewDecoder(r.Body).Decode(&b)
		if b["key"] != "BTC-GOOD-GOOD-GOOD" {
			w.WriteHeader(403)
			w.Write([]byte(`{"ok":false,"code":"LIC_UNKNOWN","error":"Unbekannter Lizenzschlüssel."}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "key": b["key"], "product": "btc", "productName": "AlgoBreak BTC", "termsOk": f.terms,
			"symbol": "BTCUSD", "period": "H1", "expert": `Cruveno\Cruveno_BTC_Daily-Breakout_EA`, "eaFile": "Cruveno_BTC_Daily-Breakout_EA.ex5",
			"eaVersion": f.eaVer, "symbols": []map[string]string{{"match": "UltimaMarkets", "btc": "BTCUSD+", "dax": "GER40+"}}, "serverUrl": "https://btc.cruveno.com", "prot": f.prot, "protDefault": map[string]float64{"risk": 1, "day": 3, "tot": 10}, "online": f.online})
	case r.URL.Path == "/inst/v1/status":
		var b map[string]string
		json.NewDecoder(r.Body).Decode(&b)
		f.phases = append(f.phases, b["phase"])
		raw, _ := json.Marshal(b)
		f.bodies = append(f.bodies, string(raw))
		w.Write([]byte(`{"ok":true}`))
	case r.URL.Path == "/inst/v1/ea":
		f.eaGets++
		w.Write(append([]byte("MZ"), make([]byte, 4096)...))
	default:
		w.WriteHeader(404)
	}
}

func setup(t *testing.T) (*fakePortal, string) {
	ProbeWait = 100 * time.Millisecond
	FakeSyms = map[string]bool{"BTCUSD": true}
	t.Helper()
	d := filepath.Join(t.TempDir(), "Cruveno")
	os.MkdirAll(d, 0o755)
	os.Setenv("CRUVENO_DATA", d)
	f := &fakePortal{terms: true, eaVer: "2026-11-10|4098", online: true}
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(srv.Close)
	os.Setenv("CRUVENO_PORTAL", srv.URL)
	src := filepath.Join(d, "fake-mt5")
	os.MkdirAll(filepath.Join(src, "Config"), 0o755)
	os.WriteFile(filepath.Join(src, "terminal64.exe"), []byte("x"), 0o644)
	// Datenordner der Original-Installation mit Serverliste
	ad := filepath.Join(d, "AppData", "ABCDEF")
	os.MkdirAll(filepath.Join(ad, "Config"), 0o755)
	os.WriteFile(filepath.Join(ad, "origin.txt"), utf16le(src), 0o644)
	os.WriteFile(filepath.Join(ad, "Config", "servers.dat"), []byte("SRV"), 0o644)
	return f, d
}

// simuliert MetaTrader: schreibt nach kurzer Zeit das Anmeldeergebnis ins Protokoll
func fakeLogin(dir, line string) {
	go func() {
		time.Sleep(300 * time.Millisecond)
		os.MkdirAll(filepath.Join(dir, "logs"), 0o755)
		os.WriteFile(filepath.Join(dir, "logs", "20261118.log"), utf16le("GH\t0\t12:00:00.000\tNetwork\t"+line+"\r\n"), 0o644)
		os.MkdirAll(filepath.Join(dir, "MQL5", "Logs"), 0o755)
		os.WriteFile(filepath.Join(dir, "MQL5", "Logs", "20261118.log"), utf16le("expert Cruveno_BTC_Daily-Breakout_EA (BTCUSD,H1) loaded successfully\r\n"), 0o644)
	}()
}

func TestKey(t *testing.T) {
	if NormKey(" btc-ab12 cd34-ef56 ") != "BTC-AB12CD34-EF56" || !ValidKey("BTC-AB12-CD34-EF56") || ValidKey("BTC-AB12-CD34-EF5") || ValidKey("XYZ-AB12-CD34-EF56") {
		t.Fatal("key")
	}
	if MaskLogin("87654321") != "••4321" || Suffix("BTC-AB12-CD34-EF56") != "BTC-EF56" {
		t.Fatal("mask")
	}
}

func TestFiles(t *testing.T) {
	d := t.TempDir()
	st := &State{Key: "BTC-AB12-CD34-EF56", Login: "123", Server: "S-Live", Symbol: "BTCUSD", Period: "H1", Expert: `Cruveno\X`, SetName: "C.set", Dir: d, Lang: "de", ServerURL: "https://btc.cruveno.com", Prot: Prot{Risk: 3, Day: 8, Tot: 20}}
	WriteIni(st, "geheim")
	b, _ := os.ReadFile(IniPath(st))
	if b[0] != 0xFF || b[1] != 0xFE {
		t.Fatal("bom")
	}
	txt := DecodeUTF16(b)
	for _, w := range []string{"Password=geheim", "WebRequestUrl=https://btc.cruveno.com;https://signal.cruveno.com", "Expert=Cruveno\\X", "ExpertParameters=C.set", "AllowLiveTrading=1", "\r\n"} {
		if !strings.Contains(txt, w) {
			t.Fatalf("ini fehlt %q", w)
		}
	}
	if !IniHasPassword(st) {
		t.Fatal("pw")
	}
	WriteIni(st, "")
	if IniHasPassword(st) || strings.Contains(ReadUTF16(IniPath(st)), "geheim") {
		t.Fatal("pw raus")
	}
	WriteSet(st)
	s := ReadUTF16(filepath.Join(d, "MQL5", "Presets", "C.set"))
	for _, w := range []string{"InpLang=1", "InpLicKey=BTC-AB12-CD34-EF56", "InpRiskPct=3", "InpDailyDDPct=8", "InpTotalDDPct=20", "InpLotMode=0"} {
		if !strings.Contains(s, w) {
			t.Fatalf("set fehlt %q", w)
		}
	}
	st.Lang = "pl"
	WriteSet(st)
	if !strings.Contains(ReadUTF16(filepath.Join(d, "MQL5", "Presets", "C.set")), "InpLang=0") {
		t.Fatal("lang pl->en")
	}
}

func TestInstallOK(t *testing.T) {
	f, d := setup(t)
	dir := InstanceDir("BTC-GOOD")
	fakeLogin(dir, "'87654321': authorized on Broker-Live through Access Server")
	var evs []Ev
	res, err := Install(context.Background(), InstallReq{Key: "btc-good-good-good", Login: "87654321", Server: "Broker-Live", Password: "Geh3im!", Lang: "de"}, func(e Ev) { evs = append(evs, e) })
	if err != nil {
		t.Fatal(err)
	}
	if res.LoginRes != "ok" || !res.Expert || res.Prot.Risk != 1 {
		t.Fatalf("res %+v", res)
	}
	if !fileExists(filepath.Join(dir, "Config", "servers.dat")) || !fileExists(filepath.Join(dir, "MQL5", "Experts", "Cruveno", "Cruveno_BTC_Daily-Breakout_EA.ex5")) {
		t.Fatal("dateien")
	}
	if IniHasPassword(LoadState("BTC-GOOD-GOOD-GOOD")) {
		t.Fatal("passwort muss nach Anmeldung raus sein")
	}
	st := LoadState("BTC-GOOD-GOOD-GOOD")
	raw, _ := os.ReadFile(filepath.Join(d, "states", "BTC-GOOD-GOOD-GOOD.json"))
	if st == nil || !st.AuthOK || strings.Contains(string(raw), "Geh3im") {
		t.Fatal("state")
	}
	time.Sleep(200 * time.Millisecond)
	f.mu.Lock()
	all := strings.Join(f.bodies, "|")
	f.mu.Unlock()
	if strings.Contains(all, "Geh3im") || !strings.Contains(all, `"phase":"login_ok"`) || !strings.Contains(all, `"phase":"running"`) {
		t.Fatalf("status %s", all)
	}
	if evs[len(evs)-1].Pct != 100 {
		t.Fatal("pct")
	}
}

func TestInstallBadLoginRetry(t *testing.T) {
	_, _ = setup(t)
	dir := InstanceDir("BTC-GOOD")
	fakeLogin(dir, "'87654321': authorization on Broker-Live failed (Invalid account)")
	_, err := Install(context.Background(), InstallReq{Key: "BTC-GOOD-GOOD-GOOD", Login: "87654321", Server: "Broker-Live", Password: "falsch"}, func(Ev) {})
	if err != ErrLogin {
		t.Fatalf("want ErrLogin, got %v", err)
	}
	st := LoadState("BTC-GOOD-GOOD-GOOD")
	if IniHasPassword(st) || IsRunning(dir) {
		t.Fatal("nach Fehlschlag: Passwort raus, Terminal gestoppt")
	}
	time.Sleep(500 * time.Millisecond)
	fakeLogin(dir, "'87654321': authorized on Broker-Live2 through Access Server")
	time.Sleep(10 * time.Millisecond)
	os.Remove(filepath.Join(dir, "logs", "20261118.log"))
	res, err := RetryLogin(context.Background(), "BTC-GOOD-GOOD-GOOD", "Broker-Live2", "richtig", func(Ev) {})
	if err != nil || res.LoginRes != "ok" || LoadState("BTC-GOOD-GOOD-GOOD").Server != "Broker-Live2" {
		t.Fatalf("retry %v %+v", err, res)
	}
}

func TestInstallErrors(t *testing.T) {
	f, _ := setup(t)
	if _, err := Install(context.Background(), InstallReq{Key: "BTC-NOPE-NOPE-NOPE"}, func(Ev) {}); err == nil || !strings.Contains(err.Error(), "Unbekannt") {
		t.Fatal(err)
	}
	f.terms = false
	if _, err := Install(context.Background(), InstallReq{Key: "BTC-GOOD-GOOD-GOOD"}, func(Ev) {}); err == nil || !strings.Contains(err.Error(), "Vertrag") {
		t.Fatal(err)
	}
}

func TestAgent(t *testing.T) {
	f, _ := setup(t)
	dir := InstanceDir("BTC-GOOD")
	fakeLogin(dir, "'87654321': authorized on Broker-Live through Access Server")
	if _, err := Install(context.Background(), InstallReq{Key: "BTC-GOOD-GOOD-GOOD", Login: "87654321", Server: "Broker-Live", Password: "x"}, func(Ev) {}); err != nil {
		t.Fatal(err)
	}
	set := filepath.Join(dir, "MQL5", "Presets", "Cruveno_BTC-GOOD.set")
	// 1) nichts geaendert, laeuft -> kein Neustart
	n0 := Starts
	now = func() time.Time { return time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC) }
	defer func() { now = time.Now }()
	AgentOnce(context.Background())
	if Starts != n0 {
		t.Fatal("unnoetiger Neustart")
	}
	// 2) Schutz im Kundenbereich geaendert, ruhige Zeit -> Set neu + Neustart
	f.mu.Lock()
	f.prot = &Prot{Profile: "aggressive", Risk: 3, Day: 8, Tot: 20}
	f.mu.Unlock()
	AgentOnce(context.Background())
	if !strings.Contains(ReadUTF16(set), "InpRiskPct=3") || Starts != n0+1 || LoadState("BTC-GOOD-GOOD-GOOD").Pending != "" {
		t.Fatal("schutz nicht uebernommen")
	}
	// 3) neue EA-Version waehrend der Handelszeit -> geladen, aber Neustart erst danach
	now = func() time.Time { return time.Date(2026, 11, 18, 21, 45, 0, 0, time.UTC) }
	f.mu.Lock()
	f.eaVer = "2026-11-20|5000"
	f.mu.Unlock()
	AgentOnce(context.Background())
	st := LoadState("BTC-GOOD-GOOD-GOOD")
	if st.Pending != "ea" || Starts != n0+1 || st.EaVersion == "2026-11-20|5000" {
		t.Fatalf("handelszeit: %+v", st)
	}
	now = func() time.Time { return time.Date(2026, 11, 19, 6, 0, 0, 0, time.UTC) }
	AgentOnce(context.Background())
	st = LoadState("BTC-GOOD-GOOD-GOOD")
	if st.Pending != "" || st.EaVersion != "2026-11-20|5000" || Starts != n0+2 {
		t.Fatalf("nach handelszeit: %+v", st)
	}
	// 4) MetaTrader beendet -> Dienst startet neu
	StopTerminal(dir)
	AgentOnce(context.Background())
	if !IsRunning(dir) {
		t.Fatal("watchdog")
	}
	// 5) waehrend der Installer laeuft: Finger weg
	StopTerminal(dir)
	SetInstalling(true)
	AgentOnce(context.Background())
	if IsRunning(dir) {
		t.Fatal("installing lock")
	}
	SetInstalling(false)
}

func TestUninstall(t *testing.T) {
	_, _ = setup(t)
	dir := InstanceDir("BTC-GOOD")
	fakeLogin(dir, "'1': authorized on X through")
	Install(context.Background(), InstallReq{Key: "BTC-GOOD-GOOD-GOOD", Login: "1", Server: "X"}, func(Ev) {})
	if err := Uninstall(); err != nil {
		t.Fatal(err)
	}
	if fileExists(dir) || len(AllStates()) != 0 {
		t.Fatal("uninstall")
	}
}

func TestExpertCPUAndCopySkip(t *testing.T) {
	_, d := setup(t)
	src := filepath.Join(d, "fake-mt5")
	os.MkdirAll(filepath.Join(src, "Bases", "OldSrv"), 0o755)
	os.WriteFile(filepath.Join(src, "Bases", "OldSrv", "big.hcc"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(src, "Config", "accounts.dat"), []byte("x"), 0o644)
	dir := InstanceDir("BTC-GOOD")
	go func() {
		time.Sleep(300 * time.Millisecond)
		os.MkdirAll(filepath.Join(dir, "logs"), 0o755)
		os.WriteFile(filepath.Join(dir, "logs", "20261118.log"), utf16le("A\t0\t1\tStartup\tsuccessfully initialized from start config\r\nB\t0\t1\tNetwork\t'5': authorized on X through\r\nC\t2\t1\tMQL5\tyour CPU architecture does not allow to run the file 'Cruveno_BTC_Daily-Breakout_EA.ex5': AVX2 required, you have X64 only\r\n"), 0o644)
	}()
	res, err := Install(context.Background(), InstallReq{Key: "BTC-GOOD-GOOD-GOOD", Login: "5", Server: "X", Password: "p"}, func(Ev) {})
	if err != nil || res.ExpertErr != "cpu" || res.Expert {
		t.Fatalf("cpu: %v %+v", err, res)
	}
	if fileExists(filepath.Join(dir, "Bases", "OldSrv")) || fileExists(filepath.Join(dir, "Config", "accounts.dat")) || !fileExists(filepath.Join(dir, "Config", "servers.dat")) {
		t.Fatal("copy skip", fileExists(filepath.Join(dir, "Bases", "OldSrv")), fileExists(filepath.Join(dir, "Config", "accounts.dat")), fileExists(filepath.Join(dir, "Config", "servers.dat")))
	}
}

func TestSymbolProbe(t *testing.T) {
	_, d := setup(t)
	FakeSyms = map[string]bool{"BTCUSD+": true}
	dir := InstanceDir("BTC-GOOD")
	fakeLogin(dir, "'1': authorized on Srv through X")
	var evs []Ev
	res, err := Install(context.Background(), InstallReq{Key: "BTC-GOOD-GOOD-GOOD", Login: "1", Server: "Srv", Password: "p"}, func(e Ev) { evs = append(evs, e) })
	if err != nil || res.Symbol != "BTCUSD+" {
		t.Fatalf("probe %v %+v", err, res)
	}
	st := LoadState("BTC-GOOD-GOOD-GOOD")
	if st.Symbol != "BTCUSD+" || !strings.Contains(ReadUTF16(IniPath(st)), "Symbol=BTCUSD+") {
		t.Fatal("state/ini")
	}
	seen := false
	for _, e := range evs {
		if e.Text == "sym:BTCUSD+" {
			seen = true
		}
	}
	if !seen {
		t.Fatal("ui event")
	}
	_ = d
	// Admin-Zuordnung hat Vorrang
	if c := SymbolCandidates("btc", "UltimaMarkets-Live 1", "BTCUSD", []SymMap{{Match: "ultima", Btc: "BTCUSD+"}}); c[0] != "BTCUSD+" || c[1] != "BTCUSD" {
		t.Fatalf("cands %v", c)
	}
	if c := SymbolCandidates("dax", "X", "GER40", nil); c[0] != "GER40" || c[1] != "GER40+" {
		t.Fatalf("dax %v", c)
	}
}

func TestAgentFixesSymbol(t *testing.T) {
	_, _ = setup(t)
	FakeSyms = map[string]bool{}
	dir := InstanceDir("BTC-GOOD")
	fakeLogin(dir, "'1': authorized on Srv through X")
	if _, err := Install(context.Background(), InstallReq{Key: "BTC-GOOD-GOOD-GOOD", Login: "1", Server: "Srv", Password: "p"}, func(Ev) {}); err != nil {
		t.Fatal(err)
	}
	if LoadState("BTC-GOOD-GOOD-GOOD").Symbol != "BTCUSD" {
		t.Fatal("start symbol")
	}
	// Kunde oeffnet spaeter BTCUSD+ selbst -> Kursdaten vorhanden
	os.MkdirAll(filepath.Join(dir, "Bases", "Srv", "history", "BTCUSD+"), 0o755)
	now = func() time.Time { return time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC) }
	defer func() { now = time.Now }()
	n := Starts
	AgentOnce(context.Background())
	st := LoadState("BTC-GOOD-GOOD-GOOD")
	if st.Symbol != "BTCUSD+" || !strings.Contains(ReadUTF16(IniPath(st)), "Symbol=BTCUSD+") || Starts != n+1 {
		t.Fatalf("agent symbol: %+v starts %d", st, Starts-n)
	}
}

func TestMappedSymbolFirstStart(t *testing.T) {
	f, _ := setup(t)
	_ = f
	FakeSyms = map[string]bool{"BTCUSD+": true}
	dir := InstanceDir("BTC-GOOD")
	fakeLogin(dir, "'1': authorized on UltimaMarkets-Live 1 through X")
	n := Starts
	res, err := Install(context.Background(), InstallReq{Key: "BTC-GOOD-GOOD-GOOD", Login: "1", Server: "UltimaMarkets-Live 1", Password: "p"}, func(Ev) {})
	if err != nil || res.Symbol != "BTCUSD+" || Starts != n+1 {
		t.Fatalf("mapped first start: %v %+v starts %d", err, res, Starts-n)
	}
}
