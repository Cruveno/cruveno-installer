package core

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Broker-Symbole: jeder Broker benennt BTC/DAX anders (BTCUSD, BTCUSD+, BTCUSD.r, BTCUSDm, GER40, DE40.cash ...).
// Nach der Anmeldung legt MetaTrader die Symbolliste unter Bases/<Server>/symbols ab (UTF-16-Namen in Binaerdateien).

var symRe = map[string]*regexp.Regexp{
	"btc": regexp.MustCompile(`^(BTC|XBT)[/_-]?USD([.+#_\-]?[A-Za-z0-9]{0,6}|[.+#_\-])?$`),
	"dax": regexp.MustCompile(`^(GER|DE|DAX|GERMANY)[ _-]?(40|30)?([.+#_\-]?[A-Za-z0-9]{0,6}|[.+#_\-])?$`),
}

// utf16Strings: druckbare ASCII-Folgen in UTF-16 LE (3..32 Zeichen)
func utf16Strings(b []byte) []string {
	var out []string
	var cur []byte
	flush := func() {
		if len(cur) >= 3 && len(cur) <= 32 {
			out = append(out, string(cur))
		}
		cur = cur[:0]
	}
	for start := 0; start < 2; start++ { // Namen koennen auf geraden oder ungeraden Positionen beginnen
		for i := start; i+1 < len(b); i += 2 {
			c, h := b[i], b[i+1]
			if h == 0 && c >= 0x21 && c <= 0x7e {
				cur = append(cur, c)
			} else {
				flush()
			}
		}
		flush()
	}
	return out
}

// BrokerSymbols: alle Symbolnamen des Servers, die zum Produkt passen
func BrokerSymbols(dir, server, product string) []string {
	re := symRe[product]
	if re == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	roots := []string{filepath.Join(dir, "Bases", server, "symbols")}
	if ents, err := os.ReadDir(filepath.Join(dir, "Bases")); err == nil { // Servername kann abweichen (Gross/Klein)
		for _, e := range ents {
			if e.IsDir() && strings.EqualFold(e.Name(), server) && e.Name() != server {
				roots = append(roots, filepath.Join(dir, "Bases", e.Name(), "symbols"))
			}
		}
	}
	for _, r := range roots {
		files, _ := os.ReadDir(r)
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			b, err := os.ReadFile(filepath.Join(r, f.Name()))
			if err != nil || len(b) > 64<<20 {
				continue
			}
			for _, s := range utf16Strings(b) {
				if re.MatchString(s) && !seen[s] {
					seen[s] = true
					out = append(out, s)
				}
			}
		}
	}
	return out
}

// PickSymbol: bevorzugt das gewuenschte Symbol (z. B. BTCUSD), sonst gleiche Basis mit Endung (BTCUSD+, BTCUSD.r),
// dann Basis mit Buchstaben-Endung (BTCUSDm), USDT zuletzt. "" = nichts passendes gefunden.
func PickSymbol(cands []string, want, product string) string {
	if len(cands) == 0 {
		return ""
	}
	bases := map[string][]string{"btc": {"BTCUSD", "XBTUSD", "BTC/USD", "BTC-USD", "BTC_USD"}, "dax": {"GER40", "DE40", "DAX40", "GER30", "DE30", "DAX", "GERMANY40", "GERMANY"}}[product]
	if want != "" {
		bases = append([]string{want}, bases...)
	}
	rank := func(s string) int {
		for i, b := range bases {
			if strings.EqualFold(s, b) {
				return i * 10
			}
			if len(s) > len(b) && strings.EqualFold(s[:len(b)], b) {
				suf := s[len(b):]
				switch {
				case strings.ContainsAny(suf[:1], ".+#_-"):
					return i*10 + 1
				case product == "btc" && strings.HasPrefix(strings.ToUpper(suf), "T"):
					return i*10 + 8
				default:
					return i*10 + 3
				}
			}
		}
		return 1000
	}
	sort.SliceStable(cands, func(i, j int) bool {
		ri, rj := rank(cands[i]), rank(cands[j])
		if ri != rj {
			return ri < rj
		}
		return len(cands[i]) < len(cands[j])
	})
	if rank(cands[0]) >= 1000 {
		return ""
	}
	return cands[0]
}

// SymWaitTries: wie oft (je 2 s) nach der Anmeldung auf die Symbolliste gewartet wird
var SymWaitTries = 15
