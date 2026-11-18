# Cruveno Installer

Ein-Klick-Installer für AlgoBreak (Windows `.exe`, Mac `.dmg`), gebaut mit Wails v2.

- `core/` – gesamte Logik ohne Fremdpakete: Portal-API (`/inst/v1/check|ea|status`), MT5-Dateien (UTF-16 `.ini`/`.set`),
  Einrichtung (`Install`, `RetryLogin`, `Uninstall`) und Hintergrunddienst (`AgentOnce`, `AgentLoop`).
  Tests: `go test ./core` (Linux, MetaTrader wird simuliert).
- `app.go`, `main.go` – Wails-Fenster und Bindings. `--agent` (einmal), `--agent --loop` (Windows-Autostart), `--uninstall`.
- `frontend/dist/` – Oberfläche (statisches HTML/JS, kein Build-Schritt).
- `.github/workflows/build.yml` – baut bei jedem Push auf `main` beide Dateien als Artefakte; ein Tag `v*` legt ein Release an.

## Sicherheit
- Das MT5-Passwort bleibt auf dem Rechner: es steht nur bis zur ersten erfolgreichen Anmeldung in der Startdatei
  der eigenen Instanz und wird nie an Cruveno gesendet. Statusmeldungen enthalten nur die maskierte Kontonummer.
- Keine Geheimnisse im Repo. Der Lizenzschlüssel ist die einzige Berechtigung gegenüber dem Portal.

## Ablage
- Windows: `%LOCALAPPDATA%\Cruveno\` (Instanz `MT5-<BTC-XXXX>`, `Cruveno.exe` als Dienst, Autostart unter HKCU\...\Run,
  Eintrag unter „Apps“ zum Entfernen)
- Mac: Instanz in der Wine-Umgebung der MetaTrader-5-App (`C:\Cruveno\MT5-…`), Dienst als LaunchAgent
  `com.cruveno.agent` (alle 5 Min.), Daten unter `~/Library/Application Support/Cruveno/`.

## Noch offen
- Code-Signatur (Windows: Zertifikat; Mac: Developer ID + Notarisierung). Bis dahin zeigen Windows/macOS beim ersten Öffnen eine Warnung.
