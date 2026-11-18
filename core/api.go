// Package core: Cruveno-Installer ohne Fremdpakete (Portal-API, MT5-Dateien, Einrichtung, Hintergrunddienst).
package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

const Version = "1.0.7"

// Portal: Standard app.cruveno.com, zum Testen ueber CRUVENO_PORTAL umstellbar.
func Portal() string {
	if p := strings.TrimRight(os.Getenv("CRUVENO_PORTAL"), "/"); strings.HasPrefix(p, "https://") || strings.HasPrefix(p, "http://127.0.0.1") {
		return p
	}
	return "https://app.cruveno.com"
}

func OSName() string {
	if runtime.GOOS == "darwin" {
		return "mac"
	}
	return "win"
}

type Prot struct {
	Profile string  `json:"profile,omitempty"`
	Risk    float64 `json:"risk"`
	Day     float64 `json:"day"`
	Tot     float64 `json:"tot"`
	At      string  `json:"at,omitempty"`
}

// Same: gleiche Grenzen (Profilname und Zeitstempel zaehlen nicht)
func (p Prot) Same(o Prot) bool { return p.Risk == o.Risk && p.Day == o.Day && p.Tot == o.Tot }

type Broker struct {
	Name string `json:"name"`
	Win  string `json:"win"`
	Mac  string `json:"mac"`
}

// SymMap: Zuordnung aus dem Admin, z. B. "UltimaMarkets" -> BTC "BTCUSD+", DAX "GER40+"
type SymMap struct {
	Match string `json:"match"`
	Btc   string `json:"btc"`
	Dax   string `json:"dax"`
}

type CheckResp struct {
	OK          bool     `json:"ok"`
	Code        string   `json:"code"`
	Error       string   `json:"error"`
	Key         string   `json:"key"`
	Product     string   `json:"product"`
	ProductName string   `json:"productName"`
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	TrialEnd    string   `json:"trialEnd"`
	TermsOk     bool     `json:"termsOk"`
	Lang        string   `json:"lang"`
	Symbol      string   `json:"symbol"`
	Period      string   `json:"period"`
	Expert      string   `json:"expert"`
	EaFile      string   `json:"eaFile"`
	EaVersion   string   `json:"eaVersion"`
	ServerUrl   string   `json:"serverUrl"`
	Prot        *Prot    `json:"prot"`
	ProtDefault *Prot    `json:"protDefault"`
	Brokers     []Broker `json:"brokers"`
	Symbols     []SymMap `json:"symbols"`   // Admin: Server-Teil -> Symbol
	Where       string   `json:"where"`     // self | fxv | ""
	StopLocal   bool     `json:"stopLocal"` // AlgoBreak laeuft im FXV-VPS: hier beenden
	Online      bool     `json:"online"`
	LastSeen    string   `json:"lastSeen"`
	Accounts    []string `json:"accounts"`
	PortalURL   string   `json:"portal"`
}

// Effective: Schutz aus dem Kundenbereich, sonst Standard
func (c *CheckResp) Effective() Prot {
	if c.Prot != nil && c.Prot.Risk > 0 {
		return *c.Prot
	}
	if c.ProtDefault != nil && c.ProtDefault.Risk > 0 {
		p := *c.ProtDefault
		if p.Profile == "" {
			p.Profile = "standard"
		}
		return p
	}
	return Prot{Profile: "standard", Risk: 1, Day: 3, Tot: 10}
}

var ErrOffline = errors.New("offline")

// ServerErr: Server erreicht, aber keine gueltige Antwort (z. B. Adresse fehlt noch, 404/500)
type ServerErr struct{ Status int }

func (e *ServerErr) Error() string { return fmt.Sprintf("server %d", e.Status) }

var client = &http.Client{Timeout: 60 * time.Second}

func ua() string { return "Cruveno-Installer/" + Version + " (" + OSName() + ")" }

func postJSON(ctx context.Context, path string, body any, out any) (int, error) {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", Portal()+path, bytes.NewReader(b))
	if err != nil {
		return 0, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("user-agent", ua())
	res, err := client.Do(req)
	if err != nil {
		return 0, ErrOffline
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return res.StatusCode, &ServerErr{Status: res.StatusCode}
		}
	}
	return res.StatusCode, nil
}

// NormKey: Grossbuchstaben, ohne Leerzeichen
func NormKey(k string) string {
	k = strings.ToUpper(strings.Join(strings.Fields(k), ""))
	return k
}

func ValidKey(k string) bool {
	if len(k) != 18 || !(strings.HasPrefix(k, "BTC-") || strings.HasPrefix(k, "DAX-")) {
		return false
	}
	for i, r := range k {
		if i == 3 || i == 8 || i == 13 {
			if r != '-' {
				return false
			}
			continue
		}
		if i > 3 && !((r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func Check(ctx context.Context, key, lang string) (*CheckResp, error) {
	var r CheckResp
	_, err := postJSON(ctx, "/inst/v1/check", map[string]string{"key": key, "os": OSName(), "ver": Version, "lang": lang}, &r)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// Status: Fortschritt an den Kundenbereich (nie Passwoerter; Login wird dort maskiert)
func Status(ctx context.Context, key, phase, msg, login string) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	_, _ = postJSON(ctx, "/inst/v1/status", map[string]string{"key": key, "phase": phase, "os": OSName(), "ver": Version, "msg": msg, "login": login}, nil)
}

// DownloadEA: laedt die EA-Datei ueber das Portal (prueft Lizenz und Vertragsbestaetigung)
func DownloadEA(ctx context.Context, key, target string) (int64, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", Portal()+"/inst/v1/ea?key="+key, nil)
	req.Header.Set("user-agent", ua())
	res, err := (&http.Client{Timeout: 3 * time.Minute}).Do(req)
	if err != nil {
		return 0, ErrOffline
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		var e struct {
			Error string `json:"error"`
			Code  string `json:"code"`
		}
		_ = json.NewDecoder(io.LimitReader(res.Body, 1<<16)).Decode(&e)
		if e.Error == "" {
			e.Error = fmt.Sprintf("EA-Download fehlgeschlagen (%d)", res.StatusCode)
		}
		return 0, errors.New(e.Error)
	}
	tmp := target + ".download"
	f, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(f, io.LimitReader(res.Body, 50<<20))
	f.Close()
	if err != nil {
		os.Remove(tmp)
		return 0, ErrOffline
	}
	head := make([]byte, 1)
	if g, e := os.Open(tmp); e == nil {
		g.Read(head)
		g.Close()
	}
	if n < 2048 || head[0] == '<' || head[0] == '{' {
		os.Remove(tmp)
		return 0, errors.New("Die geladene EA-Datei ist ungültig. Bitte support@cruveno.com kontaktieren.")
	}
	return n, os.Rename(tmp, target)
}

// Download: beliebige Datei (MetaTrader-Installer) mit Fortschritt
func Download(ctx context.Context, url, target string, prog func(done, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("user-agent", ua())
	res, err := (&http.Client{Timeout: 20 * time.Minute}).Do(req)
	if err != nil {
		return ErrOffline
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("Download fehlgeschlagen (%d)", res.StatusCode)
	}
	f, err := os.Create(target + ".part")
	if err != nil {
		return err
	}
	var done int64
	buf := make([]byte, 256<<10)
	for {
		n, rerr := res.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				return werr
			}
			done += int64(n)
			if prog != nil {
				prog(done, res.ContentLength)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			return ErrOffline
		}
	}
	f.Close()
	return os.Rename(target+".part", target)
}
