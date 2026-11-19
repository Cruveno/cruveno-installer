package core

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

const WebRequestURLs = "https://btc.cruveno.com;https://signal.cruveno.com"

// State: eine eingerichtete Lizenz auf diesem Rechner (states/<KEY>.json). Enthaelt nie ein Passwort.
type State struct {
	Version     string `json:"version"`
	Key         string `json:"key"`
	Product     string `json:"product"`
	ProductName string `json:"productName"`
	Login       string `json:"login"`
	Server      string `json:"server"`
	Symbol      string `json:"symbol"`
	Period      string `json:"period"`
	Expert      string `json:"expert"`
	EaFile      string `json:"eaFile"`
	EaVersion   string `json:"eaVersion"`
	ServerURL   string `json:"serverUrl"`
	Lang        string `json:"lang"`
	SetName     string `json:"setName"`
	Dir         string `json:"dir"`
	Prot        Prot   `json:"prot"`
	Broker      string `json:"broker"`
	Installed   string `json:"installed"`
	AuthOK      bool   `json:"authOk"`
	Pending     string `json:"pending,omitempty"` // "set", "ea", "set,ea": wartet auf ruhige Zeit
	PendingEA   string `json:"pendingEa,omitempty"`
	LastReport  string `json:"lastReport,omitempty"`
	ReportedOn  bool   `json:"reportedOnline,omitempty"`
	SymMissing  string `json:"symMissing,omitempty"` // 1.0.8: Symbol der Admin-Zuordnung fehlt auf dem Konto
	Running     bool   `json:"running,omitempty"`    // 1.0.9: lief MetaTrader beim letzten Durchlauf des Dienstes
	BootUnix    int64  `json:"boot,omitempty"`       // 1.0.9: Startzeit des Rechners beim letzten Durchlauf
}

func Suffix(key string) string { return key[:3] + "-" + key[len(key)-4:] }

func MaskKey(k string) string {
	if len(k) < 8 {
		return k
	}
	return k[:4] + "****-****-" + k[len(k)-4:]
}

func MaskLogin(l string) string {
	if len(l) <= 4 {
		return l
	}
	return "••" + l[len(l)-4:]
}

func statesDir() string { return filepath.Join(DataDir(), "states") }

func SaveState(st *State) error {
	if err := os.MkdirAll(statesDir(), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(st, "", "  ")
	tmp := filepath.Join(statesDir(), st.Key+".json.tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(statesDir(), st.Key+".json"))
}

func LoadState(key string) *State {
	b, err := os.ReadFile(filepath.Join(statesDir(), key+".json"))
	if err != nil {
		return nil
	}
	var st State
	if json.Unmarshal(b, &st) != nil || st.Key == "" {
		return nil
	}
	return &st
}

func AllStates() []*State {
	var out []*State
	ents, _ := os.ReadDir(statesDir())
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".json") {
			if st := LoadState(strings.TrimSuffix(e.Name(), ".json")); st != nil {
				out = append(out, st)
			}
		}
	}
	return out
}

func DeleteState(key string) { os.Remove(filepath.Join(statesDir(), key+".json")) }

// WriteUTF16: MetaTrader liest .ini/.set als UTF-16 LE mit BOM
func WriteUTF16(path string, lines []string) error {
	text := strings.Join(lines, "\r\n") + "\r\n"
	u := utf16.Encode([]rune(text))
	b := make([]byte, 2+2*len(u))
	b[0], b[1] = 0xFF, 0xFE
	for i, c := range u {
		b[2+2*i] = byte(c)
		b[3+2*i] = byte(c >> 8)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReadUTF16: MetaTrader-Protokolle (UTF-16 LE, mit oder ohne BOM)
func ReadUTF16(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return DecodeUTF16(b)
}

func DecodeUTF16(b []byte) string {
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		b = b[2:]
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return string(utf16.Decode(u))
}

func fnum(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// StartIni: Startdatei fuer terminal64.exe /portable /config:<ini>
func StartIni(st *State, password string) []string {
	l := []string{"[Common]", "Login=" + st.Login}
	if password != "" {
		l = append(l, "Password="+password)
	}
	l = append(l, "Server="+st.Server, "KeepPrivate=1", "NewsEnable=0", "CertInstall=0", "",
		"[Charts]", "ProfileLast=Default", "",
		"[Experts]", "AllowLiveTrading=1", "AllowDllImport=1", "Enabled=1", "Account=0", "Profile=0",
		"WebRequest=1", "WebRequestUrl="+WebRequestURLs, "",
		"[StartUp]", "Expert="+st.Expert, "ExpertParameters="+st.SetName, "Symbol="+st.Symbol, "Period="+st.Period)
	return l
}

// SetLines: EA-Einstellungen (MQL5\Presets\<SetName>)
func SetLines(st *State) []string {
	lang := "0"
	if st.Lang == "de" {
		lang = "1"
	}
	return []string{
		"; Cruveno " + st.ProductName + " - Cruveno-Installer " + Version,
		"InpLang=" + lang,
		"InpLicKey=" + st.Key,
		"InpSrvURL=" + st.ServerURL,
		"InpLotMode=0",
		"InpRiskPct=" + fnum(st.Prot.Risk),
		"InpDailyDDPct=" + fnum(st.Prot.Day),
		"InpTotalDDPct=" + fnum(st.Prot.Tot),
	}
}

func IniPath(st *State) string { return filepath.Join(st.Dir, "cruveno_start.ini") }

func WriteIni(st *State, password string) error {
	return WriteUTF16(IniPath(st), StartIni(st, password))
}

func WriteSet(st *State) error {
	p := filepath.Join(st.Dir, "MQL5", "Presets")
	if err := os.MkdirAll(p, 0o755); err != nil {
		return err
	}
	return WriteUTF16(filepath.Join(p, st.SetName), SetLines(st))
}

func IniHasPassword(st *State) bool {
	for _, l := range strings.Split(ReadUTF16(IniPath(st)), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "Password=") {
			return true
		}
	}
	return false
}

// LoginResult: "ok", "bad" oder "" (noch nichts) aus den MetaTrader-Protokollen seit 'since'
func LoginResult(dir, login string, since time.Time) string {
	files := recentLogs(filepath.Join(dir, "logs"), since)
	for _, f := range files {
		t := ReadUTF16(f)
		if strings.Contains(t, "'"+login+"': authorized on") {
			return "ok"
		}
		for _, ln := range strings.Split(t, "\n") {
			if strings.Contains(ln, "'"+login+"': authorization on") && strings.Contains(ln, "failed") {
				return "bad"
			}
		}
	}
	return ""
}

// EverAuthorized: hat sich MT5 mit diesem Konto schon einmal angemeldet (Passwort gespeichert)?
func EverAuthorized(dir, login string) bool {
	return LoginResult(dir, login, time.Time{}) == "ok"
}

// ExpertState: "ok" (EA am Chart geladen), "cpu" (EA verlangt AVX2, der Prozessor bzw. Rosetta kann das nicht),
// "fail" (Laden fehlgeschlagen) oder "" (noch nichts). Wertet nur Zeilen ab dem letzten Start dieser Instanz aus.
func ExpertState(dir, eaName string, since time.Time) string {
	base := strings.TrimSuffix(eaName, ".ex5")
	res := ""
	for _, d := range []string{filepath.Join(dir, "logs"), filepath.Join(dir, "MQL5", "Logs")} {
		for _, f := range recentLogs(d, since) {
			t := ReadUTF16(f)
			if i := strings.LastIndex(t, "Startup\tsuccessfully initialized"); i > 0 && d == filepath.Join(dir, "logs") {
				t = t[i:]
			}
			for _, ln := range strings.Split(t, "\n") {
				switch {
				case strings.Contains(ln, base) && strings.Contains(ln, "loaded successfully"):
					return "ok"
				case strings.Contains(ln, "AVX2 required") || (strings.Contains(ln, "CPU architecture does not allow") && strings.Contains(ln, base)):
					res = "cpu"
				case res == "" && strings.Contains(ln, "loading of "+base) && strings.Contains(ln, "failed"):
					res = "fail"
				}
			}
		}
	}
	return res
}

// ExpertLoaded: EA am Chart geladen
func ExpertLoaded(dir, eaName string, since time.Time) bool {
	return ExpertState(dir, eaName, since) == "ok"
}

func recentLogs(d string, since time.Time) []string {
	type fe struct {
		p string
		t time.Time
	}
	var out []fe
	ents, _ := os.ReadDir(d)
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".log") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if !since.IsZero() && info.ModTime().Before(since.Add(-time.Minute)) {
			continue
		}
		out = append(out, fe{filepath.Join(d, e.Name()), info.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].t.After(out[j].t) })
	var ps []string
	for i, f := range out {
		if i >= 3 {
			break
		}
		ps = append(ps, f.p)
	}
	return ps
}

// ClearCharts: gespeicherte Charts entfernen, damit nur der Start-Chart mit dem EA laeuft (kein doppelter EA)
func ClearCharts(dir string) {
	d := filepath.Join(dir, "MQL5", "Profiles", "Charts", "Default")
	ents, _ := os.ReadDir(d)
	for _, e := range ents {
		if strings.HasSuffix(strings.ToLower(e.Name()), ".chr") {
			os.Remove(filepath.Join(d, e.Name()))
		}
	}
}

// CopyDir: komplette MT5-Installation in die eigene Instanz kopieren
// copySkip: nicht in die eigene Instanz uebernehmen – Kursdaten, alte Protokolle, Tester, Charts und gespeicherte
// Konten der Original-Installation (sonst meldet sich die AlgoBreak-Instanz ggf. mit einem anderen Konto an)
var copySkip = []string{"Bases", "logs", "Tester", "MQL5/Logs", "MQL5/Profiles/Charts", "MQL5/Profiles/Tester", "Config/accounts.dat", "Config/terminal.ini", "Config/common.ini"}

func skipRel(rel string) bool {
	r := filepath.ToSlash(rel)
	for _, s := range copySkip {
		if strings.EqualFold(r, s) || strings.HasPrefix(strings.ToLower(r), strings.ToLower(s)+"/") {
			return true
		}
	}
	return false
}

func CopyDir(src, dst string, prog func(n int)) error {
	n := 0
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if rel != "." && skipRel(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		t := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(t, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if err := copyFile(p, t); err != nil {
			return err
		}
		n++
		if prog != nil && n%50 == 0 {
			prog(n)
		}
		return nil
	})
}

func copyFile(a, b string) error {
	data, err := os.ReadFile(a)
	if err != nil {
		return err
	}
	return os.WriteFile(b, data, 0o644)
}

// FindServersDat: Serverliste aus dem Datenordner der Original-Installation (origin.txt zeigt auf deren Programmordner)
func FindServersDat(roots []string, originDir string) string {
	want := strings.ToLower(strings.TrimRight(originDir, `\/`))
	for _, root := range roots {
		ents, _ := os.ReadDir(root)
		for _, e := range ents {
			o := filepath.Join(root, e.Name(), "origin.txt")
			sd := filepath.Join(root, e.Name(), "Config", "servers.dat")
			if _, err := os.Stat(sd); err != nil {
				continue
			}
			b, err := os.ReadFile(o)
			if err != nil {
				continue
			}
			s := b
			if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
				s = []byte(DecodeUTF16(b))
			}
			got := strings.ToLower(strings.Trim(string(s), "\ufeff\x00 \r\n"))
			got = strings.TrimRight(got, `\/`)
			if got == want {
				return sd
			}
		}
	}
	return ""
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }
