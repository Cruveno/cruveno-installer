package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Broker-Symbole: jeder Broker benennt BTC/DAX anders (BTCUSD, BTCUSD+, BTCUSD.r, BTCUSDm, GER40, DE40.cash ...).
// Die Symbolliste von MetaTrader ist verschluesselt. Erkennbar ist aber, fuer welches Symbol MetaTrader Kursdaten
// anlegt: Bases/<Server>/history/<Symbol> entsteht nur, wenn es das Symbol beim Broker gibt.

var symSuffixes = []string{"", "+", ".r", "m", ".a", "#", ".", "_", ".pro", ".raw", "-", ".i", ".ecn", ".c", "pro"}

// SymbolCandidates: Admin-Zuordnung zuerst, dann Wunsch, dann gaengige Schreibweisen
func SymbolCandidates(product, server, want string, maps []SymMap) []string {
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		for _, x := range out {
			if x == s {
				return
			}
		}
		out = append(out, s)
	}
	for _, m := range maps {
		if m.Match != "" && strings.Contains(strings.ToLower(server), strings.ToLower(m.Match)) {
			if product == "dax" {
				add(m.Dax)
			} else {
				add(m.Btc)
			}
		}
	}
	add(want)
	bases := []string{"BTCUSD"}
	if product == "dax" {
		bases = []string{"GER40", "DE40", "GER40Cash", "DE40Cash"}
	}
	for _, b := range bases {
		for _, suf := range symSuffixes {
			add(b + suf)
		}
	}
	if product == "dax" {
		add("GER40.cash")
		add("DE40.cash")
		add("DAX40")
		add("GER30")
		add("DE30")
	} else {
		add("XBTUSD")
		add("BTCUSDT")
	}
	return out
}

func historyDir(dir, server string) string {
	h := filepath.Join(dir, "Bases", server, "history")
	if fileExists(h) {
		return h
	}
	if ents, err := os.ReadDir(filepath.Join(dir, "Bases")); err == nil {
		for _, e := range ents {
			if e.IsDir() && strings.EqualFold(e.Name(), server) {
				return filepath.Join(dir, "Bases", e.Name(), "history")
			}
		}
	}
	return h
}

// KnownSymbol: erstes Kandidaten-Symbol, fuer das MetaTrader bereits Kursdaten hat
func KnownSymbol(dir, server string, cands []string) string {
	h := historyDir(dir, server)
	for _, c := range cands {
		if fileExists(filepath.Join(h, c)) {
			return c
		}
	}
	return ""
}

// ProbeWait: wie lange je Versuch auf Kursdaten gewartet wird
var ProbeWait = 25 * time.Second

// ResolveSymbol: laeuft MetaTrader mit st.Symbol und gibt es das Symbol nicht, werden die Kandidaten der Reihe nach
// geoeffnet (Neustart je Versuch), bis MetaTrader Kursdaten anlegt. Rueckgabe: Symbol, ob umgestellt wurde.
func ResolveSymbol(ctx context.Context, st *State, maps []SymMap, note func(string)) (string, bool, error) {
	cands := SymbolCandidates(st.Product, st.Server, st.Symbol, maps)
	wait := func(sym string) bool {
		for t := time.Duration(0); t < ProbeWait; t += time.Second {
			if fileExists(filepath.Join(historyDir(st.Dir, st.Server), sym)) {
				return true
			}
			if !sleepCtx(ctx, time.Second) {
				return false
			}
		}
		return false
	}
	switchTo := func(sym string) error {
		StopTerminal(st.Dir)
		st.Symbol = sym
		WriteIni(st, "")
		SaveState(st)
		ClearCharts(st.Dir)
		return StartTerminal(st)
	}
	// schon bekannt (z. B. erneute Installation)?
	if k := KnownSymbol(st.Dir, st.Server, cands); k != "" {
		if k == st.Symbol {
			return k, false, nil
		}
		note("sym:" + k)
		return k, true, switchTo(k)
	}
	orig := st.Symbol
	for i, c := range cands {
		if ctx.Err() != nil {
			return "", false, ctx.Err()
		}
		if c != st.Symbol {
			note(fmt.Sprintf("probe:%s", c))
			if err := switchTo(c); err != nil {
				return "", false, err
			}
		}
		if wait(c) {
			note("sym:" + c)
			return c, c != orig, nil
		}
		if i >= 14 {
			break
		}
	}
	// nichts gefunden: zurueck auf den Wunsch, der Dienst prueft spaeter erneut
	if st.Symbol != orig {
		switchTo(orig)
	}
	return "", false, nil
}
