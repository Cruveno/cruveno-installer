package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Ev: Fortschritt fuer die Oberflaeche
type Ev struct {
	Task  string  `json:"task"`  // lic, mt5, ea, login, chart, agent, online
	State string  `json:"state"` // run, ok, warn, fail
	Pct   float64 `json:"pct"`   // Gesamtfortschritt 0..100
	Text  string  `json:"text"`
}

type InstallReq struct {
	Key      string `json:"key"`
	Lang     string `json:"lang"`
	Login    string `json:"login"`
	Server   string `json:"server"`
	Password string `json:"password"`
	Broker   string `json:"broker"`   // Name aus der Brokerliste, "" = automatisch
	Existing string `json:"existing"` // vorhandene MT5-Installation verwenden (Programmordner)
}

type Result struct {
	Login     string `json:"login"`
	Server    string `json:"server"`
	Symbol    string `json:"symbol"`
	Prot      Prot   `json:"prot"`
	Online    bool   `json:"online"`
	Expert    bool   `json:"expert"`
	ExpertErr string `json:"expertErr"` // cpu, fail
	LoginRes  string `json:"loginRes"`  // ok, timeout
	Dir       string `json:"dir"`
	Portal    string `json:"portal"`
}

// ErrLogin: Broker hat die Anmeldung abgelehnt (Installation selbst ist fertig)
var ErrLogin = errors.New("login")

const GenericMT5Win = "https://download.mql5.com/cdn/web/metaquotes.software.corp/mt5/mt5setup.exe"
const GenericMT5Mac = "https://download.mql5.com/cdn/web/metaquotes.software.corp/mt5/MetaTrader5.pkg.zip"

func pickBroker(list []Broker, name string) Broker {
	for _, b := range list {
		if strings.EqualFold(b.Name, name) {
			return b
		}
	}
	return Broker{Name: "MetaTrader 5", Win: GenericMT5Win, Mac: GenericMT5Mac}
}

// ExistingTerminals: vorhandene MetaTrader-5-Installationen (fuer die Auswahl in der Oberflaeche)
func ExistingTerminals() []string { return programDirs() }

// Install: kompletter Ablauf. Bei ErrLogin ist alles eingerichtet bis auf die Anmeldung (RetryLogin).
func Install(ctx context.Context, rq InstallReq, emit func(Ev)) (*Result, error) {
	e := func(task, state string, pct float64, text string) { emit(Ev{task, state, pct, text}) }
	key := NormKey(rq.Key)
	e("lic", "run", 1, "")
	c, err := Check(ctx, key, rq.Lang)
	if err != nil {
		return nil, errors.New("Keine Verbindung zu Cruveno. Bitte Internetverbindung prüfen.")
	}
	if !c.OK {
		return nil, errors.New(nz(c.Error, "Lizenz nicht gefunden."))
	}
	if !c.TermsOk {
		return nil, errors.New("Bitte bestätige zuerst Vertrag und Hinweise im Kundenbereich.")
	}
	go Status(context.Background(), key, "start", "", rq.Login)
	e("lic", "ok", 3, "")

	suffix := Suffix(key)
	dir := InstanceDir(suffix)
	old := LoadState(key)
	haveInstance := fileExists(filepath.Join(dir, "terminal64.exe"))

	// 1) MetaTrader 5
	e("mt5", "run", 4, "")
	if !haveInstance {
		src := rq.Existing
		if src == "" || !fileExists(filepath.Join(src, "terminal64.exe")) {
			br := pickBroker(c.Brokers, rq.Broker)
			src, err = installMT5(ctx, br, func(p float64, t string) { e("mt5", "run", 4+p*0.5, t) })
			if err != nil {
				Status(context.Background(), key, "error", "MT5: "+err.Error(), rq.Login)
				return nil, err
			}
		}
		e("mt5", "run", 55, "copy")
		StopTerminal(dir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		if err := CopyDir(src, dir, func(n int) { e("mt5", "run", 55, fmt.Sprintf("copy:%d", n)) }); err != nil {
			return nil, fmt.Errorf("MetaTrader konnte nicht kopiert werden: %v", err)
		}
		if !fileExists(filepath.Join(dir, "Config", "servers.dat")) {
			if sd := FindServersDat(dataRoots(), originOf(src)); sd != "" {
				os.MkdirAll(filepath.Join(dir, "Config"), 0o755)
				copyFile(sd, filepath.Join(dir, "Config", "servers.dat"))
			}
		}
	}
	for _, sub := range []string{"MQL5/Experts/Cruveno", "MQL5/Presets", "logs"} {
		os.MkdirAll(filepath.Join(dir, filepath.FromSlash(sub)), 0o755)
	}
	go Status(context.Background(), key, "mt5", "", rq.Login)
	e("mt5", "ok", 62, "")

	// 2) EA + Einstellungen
	e("ea", "run", 63, "")
	StopTerminal(dir)
	eaFile := nz(c.EaFile, "Cruveno_BTC_Daily-Breakout_EA.ex5")
	if _, err := DownloadEA(ctx, key, filepath.Join(dir, "MQL5", "Experts", "Cruveno", eaFile)); err != nil {
		Status(context.Background(), key, "error", "EA: "+err.Error(), rq.Login)
		return nil, err
	}
	st := &State{Version: Version, Key: key, Product: c.Product, ProductName: c.ProductName, Login: strings.TrimSpace(rq.Login), Server: strings.TrimSpace(rq.Server),
		Symbol: nz(c.Symbol, "BTCUSD"), Period: nz(c.Period, "H1"), Expert: nz(c.Expert, `Cruveno\Cruveno_BTC_Daily-Breakout_EA`), EaFile: eaFile, EaVersion: c.EaVersion,
		ServerURL: c.ServerUrl, Lang: nz(rq.Lang, "de"), SetName: "Cruveno_" + suffix + ".set", Dir: dir, Prot: c.Effective(), Broker: rq.Broker, Installed: time.Now().UTC().Format(time.RFC3339)}
	// Admin-Zuordnung (Servername -> Symbol) gleich beim ersten Start verwenden; sonst ein bekanntes Symbol der Instanz
	if cs := SymbolCandidates(st.Product, st.Server, st.Symbol, c.Symbols); len(cs) > 0 {
		if k := KnownSymbol(dir, st.Server, cs); k != "" {
			st.Symbol = k
		} else {
			st.Symbol = cs[0]
		}
	}
	if old != nil && st.Login == old.Login && old.AuthOK {
		st.AuthOK = true
	}
	if err := WriteSet(st); err != nil {
		return nil, err
	}
	if err := WriteIni(st, rq.Password); err != nil {
		return nil, err
	}
	if err := SaveState(st); err != nil {
		return nil, err
	}
	go Status(context.Background(), key, "ea", "", st.Login)
	e("ea", "ok", 68, "")

	// 3) Starten + anmelden
	return startAndVerify(ctx, st, c, rq.Password != "", emit)
}

// RetryLogin: nach abgelehnter Anmeldung mit neuem Passwort (und ggf. Server) erneut
func RetryLogin(ctx context.Context, key, server, password string, emit func(Ev)) (*Result, error) {
	st := LoadState(NormKey(key))
	if st == nil {
		return nil, errors.New("Keine Einrichtung gefunden.")
	}
	if s := strings.TrimSpace(server); s != "" {
		st.Server = s
	}
	StopTerminal(st.Dir)
	if err := WriteIni(st, password); err != nil {
		return nil, err
	}
	SaveState(st)
	c, err := Check(ctx, st.Key, st.Lang)
	if err != nil || !c.OK {
		c = &CheckResp{PortalURL: Portal()}
	}
	return startAndVerify(ctx, st, c, password != "", emit)
}

func startAndVerify(ctx context.Context, st *State, c *CheckResp, pwSet bool, emit func(Ev)) (*Result, error) {
	e := func(task, state string, pct float64, text string) { emit(Ev{task, state, pct, text}) }
	res := &Result{Login: st.Login, Server: st.Server, Symbol: st.Symbol, Prot: st.Prot, Dir: st.Dir, Portal: nz(c.PortalURL, Portal())}
	e("login", "run", 70, "")
	since := time.Now()
	ClearCharts(st.Dir)
	if err := StartTerminal(st); err != nil {
		Status(context.Background(), st.Key, "error", "Start: "+err.Error(), st.Login)
		return nil, fmt.Errorf("MetaTrader konnte nicht gestartet werden: %v", err)
	}
	r := ""
	for i := 0; i < 75 && r == ""; i++ { // bis 150 s
		if !sleepCtx(ctx, 2*time.Second) {
			return nil, ctx.Err()
		}
		r = LoginResult(st.Dir, st.Login, since)
	}
	switch r {
	case "ok":
		st.AuthOK = true
		if pwSet {
			sleepCtx(ctx, 5*time.Second)
			WriteIni(st, "") // MT5 hat das Konto gespeichert -> Passwort raus
		}
		SaveState(st)
		res.LoginRes = "ok"
		go Status(context.Background(), st.Key, "login_ok", "", st.Login)
		e("login", "ok", 82, "")
		// Symbol des Brokers ermitteln (BTCUSD vs. BTCUSD+ ...) und Chart ggf. umstellen
		if sym, changed, err := ResolveSymbol(ctx, st, c.Symbols, func(t string) { e("chart", "run", 82, t) }); err != nil {
			return nil, err
		} else if sym != "" {
			res.Symbol = sym
			if changed {
				since = time.Now()
				go Status(context.Background(), st.Key, "agent", "Symbol beim Broker: "+sym, st.Login)
			}
		}
	case "bad":
		StopTerminal(st.Dir)
		WriteIni(st, "")
		Status(context.Background(), st.Key, "login_fail", "Server "+st.Server, st.Login)
		e("login", "fail", 72, "")
		return res, ErrLogin
	default:
		res.LoginRes = "timeout" // Dienst entfernt das Passwort, sobald die Anmeldung klappt
		e("login", "warn", 82, "")
	}

	// 4) EA am Chart
	e("chart", "run", 83, "")
	for i := 0; i < 30 && res.ExpertErr == ""; i++ { // bis 45 s
		switch ExpertState(st.Dir, st.EaFile, since) {
		case "ok":
			res.Expert = true
		case "cpu":
			res.ExpertErr = "cpu"
		case "fail":
			if i >= 6 { // kurz warten, ob ein zweiter Versuch klappt
				res.ExpertErr = "fail"
			}
		}
		if res.Expert {
			break
		}
		if !sleepCtx(ctx, 1500*time.Millisecond) {
			return nil, ctx.Err()
		}
	}
	switch {
	case res.Expert:
		go Status(context.Background(), st.Key, "running", "", st.Login)
		e("chart", "ok", 88, "")
	case res.ExpertErr == "cpu":
		Status(context.Background(), st.Key, "error", "EA verlangt AVX2, Prozessor unterstuetzt es nicht ("+OSName()+")", st.Login)
		e("chart", "fail", 88, "cpu")
	case res.ExpertErr == "fail":
		Status(context.Background(), st.Key, "error", "EA konnte nicht geladen werden", st.Login)
		e("chart", "fail", 88, "fail")
	default:
		go Status(context.Background(), st.Key, "running", "", st.Login)
		e("chart", "warn", 88, "")
	}

	// 5) Hintergrunddienst
	e("agent", "run", 89, "")
	if err := InstallAgent(); err != nil {
		e("agent", "warn", 92, err.Error())
	} else {
		e("agent", "ok", 92, "")
	}

	// 6) Erste Meldung des EAs
	e("online", "run", 93, "")
	for i := 0; i < 24 && !res.Online && res.ExpertErr == ""; i++ { // bis 2 Min.
		if c2, err := Check(ctx, st.Key, st.Lang); err == nil && c2.OK && c2.Online {
			res.Online = true
			break
		}
		if !sleepCtx(ctx, 5*time.Second) {
			return nil, ctx.Err()
		}
	}
	if res.Online {
		st.ReportedOn = true
		SaveState(st)
		go Status(context.Background(), st.Key, "online", "", st.Login)
		e("online", "ok", 100, "")
	} else {
		e("online", "warn", 100, "")
	}
	return res, nil
}

// Uninstall: Dienst, Autostart und alle Cruveno-Instanzen entfernen (MetaTrader des Brokers bleibt)
func Uninstall() error {
	var errs []string
	for _, st := range AllStates() {
		StopTerminal(st.Dir)
		if strings.Contains(st.Dir, "Cruveno") {
			if err := os.RemoveAll(st.Dir); err != nil {
				errs = append(errs, err.Error())
			}
		}
		Status(context.Background(), st.Key, "uninstall", "", "")
		DeleteState(st.Key)
	}
	if err := RemoveAgent(); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

func nz(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
