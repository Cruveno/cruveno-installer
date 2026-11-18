"use strict";
// Cruveno-Installer – Oberflaeche. Go-Bindings: window.go.main.App.*, Ereignisse: window.runtime.EventsOn
const LANG = /^de/i.test(navigator.language || "de") ? "de" : "en";
const T = (de, en) => (LANG === "de" ? de : en);
const $ = (s) => document.querySelector(s);
const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
const ICO = {
  ok: `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6L9 17l-5-5"/></svg>`,
  lock: `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="#7a8090" stroke-width="2" stroke-linecap="round"><rect x="4" y="10" width="16" height="11" rx="2.5"/><path d="M8 10V7a4 4 0 018 0v3"/></svg>`,
  info: `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="#7a8090" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8h.01"/></svg>`,
  infoBad: `<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#c4423a" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8h.01"/></svg>`,
  eye: `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z"/><circle cx="12" cy="12" r="3"/></svg>`,
};
const go = () => (window.go && window.go.main && window.go.main.App) || null;
const S = { view: "key", info: { os: "win", version: "", mode: "install", existing: [] }, key: "", chk: null, chkBusy: false, login: "", server: "", pw: "", broker: "", existing: "", tasks: {}, times: {}, pct: 0, sub: "", res: null, err: "", started: 0 };
const PROF = { careful: T("Vorsichtig", "Careful"), standard: "Standard", aggressive: T("Offensiv", "Aggressive"), custom: T("Individuell", "Custom") };
const pc = (v) => String(v).replace(".", LANG === "de" ? "," : ".") + " %";
const fmtDate = (d) => (d ? new Date(d + "T00:00:00").toLocaleDateString(LANG === "de" ? "de-DE" : "en-GB") : "");
const MB = (b) => (b / 1048576).toFixed(0) + " MB";
const SUPPORT = "support@cruveno.com";

function steps() {
  const st = [["key", T("Lizenz", "License")], ["broker", T("Brokerkonto", "Broker account")], ["install", T("Installation", "Installation")], ["done", T("Fertig", "Done")]];
  const cur = { loginerr: "install", err: "install" }[S.view] || S.view;
  const ci = st.findIndex((s) => s[0] === cur);
  $("#steps").innerHTML = S.view === "uninstall" ? "" : st.map((s, i) => `<li class="${i < ci ? "done" : i === ci ? "on" : ""}"><i>${i < ci ? "✓" : i + 1}</i>${s[1]}</li>`).join("");
  $("#ver").textContent = `Installer ${S.info.version || ""} · ${S.info.os === "mac" ? "macOS" : "Windows"}`;
  $("#disc").textContent = T("Software, keine Anlageberatung.", "Software, not investment advice.");
  if (S.chk && S.chk.productName) $("#prodname").textContent = S.chk.productName;
}

function prot() {
  const c = S.chk || {};
  const p = c.prot && c.prot.risk ? c.prot : { ...(c.protDefault || { risk: 1, day: 3, tot: 10 }), profile: "standard" };
  return p;
}
function protCard() {
  const p = prot();
  return `<div class="card"><span>${T("Profil", "Profile")}</span><span>${T("Risiko je Trade", "Risk per trade")}</span><span>${T("Tag max.", "Day max.")}</span><span>${T("Gesamt max.", "Total max.")}</span><b>${esc(PROF[p.profile] || p.profile || "Standard")}</b><b>${pc(p.risk)}</b><b>${pc(p.day)}</b><b>${pc(p.tot)}</b></div>`;
}
function foot(left, right) {
  return `<div class="foot">${left || `<span class="meta">${T("Hilfe", "Help")}: ${SUPPORT}</span>`}<div class="sp">${right || ""}</div></div>`;
}

// ---------------------------------------------------------------- Ansichten
const V = {
  key() {
    const c = S.chk;
    let state = "";
    if (S.chkBusy) state = `<div class="check wait"><span class="spin"></span><span>${T("Lizenz wird geprüft …", "Checking license …")}</span></div>`;
    else if (c && c.err === "offline") state = `<div class="check bad">${ICO.infoBad}<span><b>${T("Keine Verbindung", "No connection")}</b> · ${T("Bitte Internetverbindung prüfen.", "Please check your internet connection.")}</span></div>`;
    else if (c && c.ok === false) state = `<div class="check bad">${ICO.infoBad}<span><b>${esc(c.error || T("Lizenz nicht gefunden", "License not found"))}</b> ${c.code === "LIC_UNKNOWN" ? T("Bitte den Schlüssel aus deiner E-Mail von Cruveno kopieren.", "Please copy the key from your Cruveno e-mail.") : ""}</span></div>`;
    else if (c && c.ok && !c.termsOk) state = `<div class="check bad">${ICO.infoBad}<span><b>${T("Noch nicht freigegeben", "Not released yet")}</b> · ${T("Bitte zuerst Vertrag und Hinweise im Kundenbereich bestätigen.", "Please confirm the agreement and notices in the customer area first.")} <button class="lk" data-act="portal">${T("Kundenbereich öffnen", "Open customer area")}</button></span></div>`;
    else if (c && c.ok) state = `<div class="check">${ICO.ok}<span><b>${T("Lizenz gefunden", "License found")}</b> · ${esc(c.productName)}${c.status === "trial" && c.trialEnd ? ` · ${T("Testphase bis", "Trial until")} ${fmtDate(c.trialEnd)}` : ""}</span></div>
${protCard()}<p class="note">${ICO.info}<span>${T("Diese Werte kommen aus deinen Risiko-Einstellungen. Änderst du sie später im Kundenbereich, übernimmt Cruveno sie automatisch.", "These values come from your risk settings. If you change them later in the customer area, Cruveno applies them automatically.")}</span></p>`;
    const ok = c && c.ok && c.termsOk;
    return `<span class="k">${T("Schritt 1 von 4", "Step 1 of 4")}</span><h1>${T("Willkommen. Gib deinen Lizenzschlüssel ein.", "Welcome. Enter your license key.")}</h1>
<p class="lede">${T("Der Installer richtet MetaTrader 5 und AlgoBreak vollständig für dich ein – mit den Einstellungen aus deinem Kundenbereich. Dauer: ca. 3 bis 5 Minuten.", "The installer sets up MetaTrader 5 and AlgoBreak completely – with the settings from your customer area. Takes about 3 to 5 minutes.")}</p>
<div class="body"><label class="f">${T("Lizenzschlüssel", "License key")} <small>${T("steht in deiner E-Mail von Cruveno", "in your e-mail from Cruveno")}</small><input id="key" class="in mono${ok ? " ok" : c && (c.ok === false || c.err) ? " bad" : ""}" value="${esc(S.key)}" placeholder="BTC-XXXX-XXXX-XXXX" maxlength="24" autocomplete="off" spellcheck="false" autofocus></label>
${state}</div>${foot("", `<button class="btn p" data-act="next" ${ok ? "" : "disabled"}>${T("Weiter", "Continue")} →</button>`)}`;
  },
  broker() {
    const c = S.chk || {}, sv = c.saved;
    const opts = [];
    (S.info.existing || []).forEach((d) => opts.push([`x:${d}`, `${T("Vorhanden", "Installed")}: ${d.split(/[\\/]/).filter(Boolean).slice(-1)[0]}`]));
    (c.brokers || []).forEach((b) => (S.info.os === "mac" ? b.mac : b.win) && opts.push([`b:${b.name}`, `${b.name} – ${T("MetaTrader 5 installieren", "install MetaTrader 5")}`]));
    opts.push(["g:", T("MetaTrader 5 (offizieller Installer von MetaQuotes)", "MetaTrader 5 (official MetaQuotes installer)")]);
    if (!S.broker || !opts.some((o) => o[0] === S.broker)) S.broker = (sv && sv.broker && opts.find((o) => o[0] === "b:" + sv.broker) ? "b:" + sv.broker : opts[0][0]);
    const keep = sv && sv.authOk && S.login === sv.login;
    const ready = /^\d{3,12}$/.test(S.login.trim()) && S.server.trim().length > 1 && (keep || S.pw.length >= 4);
    return `<span class="k">${T("Schritt 2 von 4", "Step 2 of 4")}</span><h1>${T("Dein Brokerkonto", "Your broker account")}</h1>
<p class="lede">${T("Damit MetaTrader sich nach der Installation direkt anmeldet. Die Daten stehen in der E-Mail deines Brokers.", "So MetaTrader signs in right after the installation. The details are in your broker's e-mail.")}</p>
<div class="body"><div class="row"><label class="f">${T("Kontonummer (MT5-Login)", "Account number (MT5 login)")}<input id="login" class="in mono" inputmode="numeric" value="${esc(S.login)}" placeholder="12345678" autocomplete="off"></label>
<label class="f">Server<input id="server" class="in" value="${esc(S.server)}" placeholder="${T("z. B. Broker-Live", "e.g. Broker-Live")}" autocomplete="off" spellcheck="false"></label></div>
<label class="f">${T("Handelspasswort", "Trading password")} ${keep ? `<small>${T("leer lassen = gespeichertes Passwort weiter verwenden", "leave empty = keep the saved password")}</small>` : `<small>${T("nicht das Investor-Passwort", "not the investor password")}</small>`}<div class="pw"><input id="pw" class="in" type="password" value="${esc(S.pw)}" autocomplete="off"><button type="button" data-act="eye" aria-label="${T("Passwort anzeigen", "Show password")}">${ICO.eye}</button></div></label>
<label class="f">MetaTrader 5<div class="sel"><select id="broker" class="in">${opts.map((o) => `<option value="${esc(o[0])}"${o[0] === S.broker ? " selected" : ""}>${esc(o[1])}</option>`).join("")}</select></div></label>
<p class="note">${ICO.lock}<span><b>${T("Dein Passwort bleibt auf diesem Rechner.", "Your password stays on this computer.")}</b> ${T("Es wird nur an MetaTrader übergeben, nach der ersten Anmeldung aus der Startdatei gelöscht und nie an Cruveno gesendet.", "It is only handed to MetaTrader, removed from the start file after the first login and never sent to Cruveno.")}</span></p></div>
${foot(`<button class="btn s" data-act="back">← ${T("Zurück", "Back")}</button>`, `<button class="btn g" data-act="install" ${ready ? "" : "disabled"}>${T("Installieren", "Install")}</button>`)}`;
  },
  install() {
    const sym = (S.chk && S.chk.symbol) || "BTCUSD";
    const L = [
      ["lic", T("Lizenz und Einstellungen geladen", "License and settings loaded")],
      ["mt5", mt5Text()],
      ["ea", T(`${(S.chk && S.chk.productName) || "AlgoBreak"} und Einstellungen übernommen`, `${(S.chk && S.chk.productName) || "AlgoBreak"} and settings applied`)],
      ["login", T("Mit dem Brokerkonto anmelden", "Sign in with the broker account")],
      ["chart", T(`Chart ${sym} öffnen, Algo Trading einschalten`, `Open chart ${sym}, enable algo trading`)],
      ["agent", T("Cruveno-Dienst für Einstellungen und Updates", "Cruveno service for settings and updates")],
      ["online", T("Erste Meldung des EAs abwarten", "Wait for the EA's first report")],
    ];
    const cur = L.find((l) => S.tasks[l[0]] === "run");
    const dot = (s) => (s === "ok" ? `<span class="dot ok">${ICO.ok}</span>` : s === "run" ? `<span class="dot run"></span>` : s === "warn" ? `<span class="dot warn">!</span>` : s === "fail" ? `<span class="dot fail">✕</span>` : `<span class="dot wait"></span>`);
    const lede = S.info.os === "mac" ? T("Du musst nichts tun. macOS fragt gleich einmal nach deinem Mac-Passwort – das ist für die Installation von MetaTrader nötig.", "Nothing to do. macOS will ask once for your Mac password – needed to install MetaTrader.") : T("Du musst nichts tun. Windows fragt gleich einmal, ob MetaTrader installiert werden darf – bitte mit „Ja“ bestätigen.", "Nothing to do. Windows will ask once whether MetaTrader may be installed – please confirm with “Yes”.");
    return `<span class="k">${T("Schritt 3 von 4", "Step 3 of 4")}</span><h1>${T("AlgoBreak wird eingerichtet …", "Setting up AlgoBreak …")}</h1>
<p class="lede">${lede}</p>
<div class="body"><div class="prog"><i style="width:${Math.round(S.pct)}%;transition:width .4s"></i></div><div class="pct"><span>${esc(cur ? cur[1] : "")}</span><b>${Math.round(S.pct)} %</b></div>
<ul class="tasks">${L.map((l) => `<li class="${S.tasks[l[0]] === "run" ? "cur" : ""}">${dot(S.tasks[l[0]])}${esc(l[1])}<em>${S.times[l[0]] || (S.tasks[l[0]] === "run" ? "…" : "")}</em></li>`).join("")}</ul></div>
${foot(`<span class="meta">${T("Bitte das Fenster offen lassen.", "Please keep this window open.")}</span>`, "")}`;
  },
  done() {
    const r = S.res || {}, p = r.prot || prot(), sym = r.symbol || (S.chk && S.chk.symbol) || "BTCUSD";
    const live = r.online ? `<span class="live"><i></i>${T("EA ONLINE · ERSTE MELDUNG EMPFANGEN", "EA ONLINE · FIRST REPORT RECEIVED")}</span>` : `<span class="live" style="color:#a87a1d;background:#fff7e6"><i style="background:#F2B84B;box-shadow:0 0 0 4px rgba(242,184,75,.2)"></i>${T("EA STARTET · MELDET SICH IN KÜRZE", "EA STARTING · WILL REPORT SHORTLY")}</span>`;
    const warn = r.loginRes === "timeout" ? `<p class="note">${ICO.info}<span>${T("Der Broker hat die Anmeldung noch nicht bestätigt. Falls MetaTrader nach Login und Passwort fragt: einmal selbst anmelden und „Passwort speichern“ anhaken.", "The broker has not confirmed the login yet. If MetaTrader asks for login and password: sign in once yourself and tick “Save password”.")}</span></p>` : "";
    return `<span class="k">${T("Schritt 4 von 4", "Step 4 of 4")}</span>
<div class="body hero" style="margin-top:14px"><div class="big"><svg width="34" height="34" viewBox="0 0 24 24" fill="none" stroke="#1f9d6b" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6L9 17l-5-5"/></svg></div>
<h1 style="margin:0">${T("Fertig. AlgoBreak läuft.", "Done. AlgoBreak is running.")}</h1>${live}
<p class="lede" style="margin:0">${T(`MetaTrader ist eingerichtet, der EA hängt am Chart ${esc(sym)} und hält sich an deine Grenzen. Der Cruveno-Dienst übernimmt künftige Änderungen und Updates selbst.`, `MetaTrader is set up, the EA is attached to the ${esc(sym)} chart and respects your limits. The Cruveno service applies future changes and updates by itself.`)}</p>
<div class="sum"><div><span>${T("Konto", "Account")}</span><b>${esc(r.login ? "••" + String(r.login).slice(-4) : "—")}</b><small>${esc(r.server || "")}</small></div><div><span>${T("Schutz", "Protection")}</span><b>${esc(PROF[p.profile] || "Standard")}</b><small>${pc(p.risk)} · ${T("Tag", "day")} ${pc(p.day)} · ${T("ges.", "total")} ${pc(p.tot)}</small></div><div><span>${T("Freigabe", "Release")}</span><b>${T("Im Kundenbereich", "In the customer area")}</b><small>${T("Handel freigeben", "release trading")}</small></div></div>
${warn}<p class="note">${ICO.info}<span>${T(`Wichtig: ${S.info.os === "mac" ? "Der Mac" : "Der Rechner"} muss zu den Handelszeiten eingeschaltet und mit dem Internet verbunden sein.`, `Important: the ${S.info.os === "mac" ? "Mac" : "computer"} must be switched on and online during trading hours.`)}</span></p></div>
${foot(`<span class="meta">${T("Fenster kann geschlossen werden.", "You can close this window.")}</span>`, `<button class="btn s" data-act="mt5">${T("MetaTrader öffnen", "Open MetaTrader")}</button><button class="btn g" data-act="portal">${T("Zum Kundenbereich", "Customer area")} →</button>`)}`;
  },
  loginerr() {
    return `<span class="k">${T("Schritt 3 von 4", "Step 3 of 4")}</span><h1>${T("Anmeldung beim Broker hat nicht geklappt", "The broker login did not work")}</h1>
<p class="lede">${T("MetaTrader und AlgoBreak sind installiert – nur die Anmeldung fehlt noch.", "MetaTrader and AlgoBreak are installed – only the login is missing.")}</p>
<div class="body"><div class="err">${ICO.infoBad}<span><b>${T(`Server „${esc(S.server)}“ hat das Login ${esc(S.login)} abgelehnt.`, `Server “${esc(S.server)}” rejected login ${esc(S.login)}.`)}</b> ${T("Meist ist das Passwort falsch oder es ist das Investor-Passwort (nur Lesen). Bitte das Handelspasswort aus der E-Mail deines Brokers verwenden und den Servernamen prüfen.", "Usually the password is wrong or it is the investor password (read-only). Please use the trading password from your broker's e-mail and check the server name.")}</span></div>
<div class="row"><label class="f">Server<input id="server" class="in" value="${esc(S.server)}" spellcheck="false"></label>
<label class="f">${T("Handelspasswort", "Trading password")}<div class="pw"><input id="pw" class="in focus" type="password" value="" autocomplete="off" autofocus><button type="button" data-act="eye">${ICO.eye}</button></div></label></div>
<p class="note">${ICO.lock}<span>${T("Dein Passwort bleibt auf diesem Rechner und wird nie an Cruveno gesendet.", "Your password stays on this computer and is never sent to Cruveno.")}</span></p></div>
${foot(`<span class="meta">${T("Hilfe", "Help")}: ${SUPPORT}</span>`, `<button class="btn p" data-act="retry" disabled>${T("Erneut anmelden", "Sign in again")}</button>`)}`;
  },
  err() {
    return `<span class="k">${T("Installation", "Installation")}</span><h1>${T("Das hat leider nicht geklappt", "Unfortunately that did not work")}</h1>
<p class="lede">${T("Es wurde nichts an deinem Broker-Konto geändert. Du kannst es einfach noch einmal versuchen.", "Nothing was changed on your broker account. You can simply try again.")}</p>
<div class="body"><div class="err">${ICO.infoBad}<span><b>${esc(S.err)}</b></span></div>
<p class="note">${ICO.info}<span>${T(`Hilft das nicht weiter, schreib an ${SUPPORT} – mit einem Bildschirmfoto dieses Fensters.`, `If this does not help, write to ${SUPPORT} – with a screenshot of this window.`)}</span></p></div>
${foot(`<span class="meta">${T("Hilfe", "Help")}: ${SUPPORT}</span>`, `<button class="btn s" data-act="toBroker">← ${T("Angaben prüfen", "Check details")}</button><button class="btn p" data-act="install">${T("Erneut versuchen", "Try again")}</button>`)}`;
  },
  uninstall() {
    const n = (S.info.installed || []).length;
    return `<span class="k">${T("Entfernen", "Remove")}</span><h1>${T("Cruveno von diesem Rechner entfernen?", "Remove Cruveno from this computer?")}</h1>
<p class="lede">${T("Entfernt den Cruveno-Dienst und die eigene MetaTrader-Instanz mit AlgoBreak. MetaTrader 5 deines Brokers, dein Broker-Konto und deine Lizenz bleiben unberührt.", "Removes the Cruveno service and the dedicated MetaTrader instance with AlgoBreak. Your broker's MetaTrader 5, your broker account and your license stay untouched.")}</p>
<div class="body">${n ? `<div class="card" style="grid-template-columns:1fr"><span>${T("Eingerichtete Lizenzen", "Installed licenses")}</span>${S.info.installed.map((k) => `<b class="mono" style="font-family:'JetBrains Mono',ui-monospace,monospace">${esc(k)}</b>`).join("")}</div>` : `<p class="note">${ICO.info}<span>${T("Auf diesem Rechner ist keine Lizenz eingerichtet.", "No license is set up on this computer.")}</span></p>`}
<div id="unres"></div></div>
${foot("", `<button class="btn s" data-act="quit">${T("Abbrechen", "Cancel")}</button><button class="btn p" data-act="uninstall" style="background:var(--bad)">${T("Entfernen", "Remove")}</button>`)}`;
  },
};

function mt5Text() {
  const s = S.sub || "";
  if (s.startsWith("dl:")) { const [, d, t] = s.split(":"); return T(`MetaTrader 5 wird heruntergeladen (${MB(+d)} von ${MB(+t)})`, `Downloading MetaTrader 5 (${MB(+d)} of ${MB(+t)})`); }
  if (s === "download") return T("MetaTrader 5 wird heruntergeladen (offizieller Installer)", "Downloading MetaTrader 5 (official installer)");
  if (s === "install") return S.info.os === "mac" ? T("MetaTrader 5 wird installiert – bitte Mac-Passwort eingeben", "Installing MetaTrader 5 – please enter your Mac password") : T("MetaTrader 5 wird installiert – bitte Rückfrage mit „Ja“ bestätigen", "Installing MetaTrader 5 – please confirm with “Yes”");
  if (s === "firstrun") return T("MetaTrader 5 wird zum ersten Mal eingerichtet", "MetaTrader 5 is being set up for the first time");
  if (s === "servers") return T("Serverliste des Brokers wird übernommen", "Copying the broker's server list");
  if (s.startsWith("copy") || s === "copy") return T("Eigene MetaTrader-Instanz für AlgoBreak wird angelegt", "Creating a dedicated MetaTrader instance for AlgoBreak");
  return S.tasks.mt5 === "ok" ? T("MetaTrader 5 eingerichtet", "MetaTrader 5 set up") : T("MetaTrader 5 vorbereiten", "Preparing MetaTrader 5");
}

function render() {
  steps();
  $("#main").innerHTML = V[S.view]();
  const k = $("#key");
  if (k) { k.focus(); k.setSelectionRange(k.value.length, k.value.length); }
}

// ---------------------------------------------------------------- Logik
let chkT = 0, chkN = 0;
async function check() {
  const k = S.key.toUpperCase().replace(/\s+/g, "");
  if (!/^(BTC|DAX)-[A-Z0-9]{4}-[A-Z0-9]{4}-[A-Z0-9]{4}$/.test(k)) { S.chk = null; S.chkBusy = false; render(); return; }
  const n = ++chkN;
  S.chkBusy = true; render();
  const r = go() ? await go().Check(k, LANG) : { ok: false, error: "Vorschau" };
  if (n !== chkN) return;
  S.chkBusy = false; S.chk = r; S.key = k;
  if (r && r.saved) { S.login = S.login || r.saved.login || ""; S.server = S.server || r.saved.server || ""; }
  render();
}

function startInstall() {
  S.tasks = {}; S.times = {}; S.pct = 0; S.sub = ""; S.started = Date.now(); S.err = "";
  const b = S.broker || "g:";
  const rq = { key: S.key, lang: LANG, login: S.login.trim(), server: S.server.trim(), password: S.pw, broker: b.startsWith("b:") ? b.slice(2) : "", existing: b.startsWith("x:") ? b.slice(2) : "" };
  S.view = "install"; render();
  go().Install(rq).then((e) => { if (e) { S.err = e === "busy" ? T("Die Installation läuft bereits.", "The installation is already running.") : T("Bitte alle Angaben ausfüllen.", "Please fill in all fields."); S.view = "err"; render(); } });
  S.pw = "";
}

const el = (n) => { const m = Math.floor(n / 60), s = n % 60; return `${m}:${String(s).padStart(2, "0")}`; };
function onProg(e) {
  if (S.view !== "install") { S.view = "install"; }
  if (e.task === "mt5" && e.text) S.sub = e.text;
  if (S.tasks[e.task] !== e.state) { S.tasks[e.task] = e.state; if (e.state !== "run") S.times[e.task] = el(Math.round((Date.now() - S.started) / 1000)); }
  S.pct = Math.max(S.pct, e.pct || 0);
  render();
}

document.addEventListener("input", (ev) => {
  const t = ev.target;
  if (t.id === "key") { S.key = t.value; S.chk = null; clearTimeout(chkT); chkT = setTimeout(check, 350); const ok = /^(BTC|DAX)-[A-Z0-9]{4}-[A-Z0-9]{4}-[A-Z0-9]{4}$/i.test(t.value.replace(/\s+/g, "")); if (!ok) { const b = $('[data-act="next"]'); if (b) b.disabled = true; } }
  if (t.id === "login") { S.login = t.value.replace(/[^0-9]/g, ""); if (t.value !== S.login) t.value = S.login; }
  if (t.id === "server") S.server = t.value;
  if (t.id === "pw") S.pw = t.value;
  if (S.view === "broker") { const sv = S.chk && S.chk.saved, keep = sv && sv.authOk && S.login === sv.login; const b = $('[data-act="install"]'); if (b) b.disabled = !(/^\d{3,12}$/.test(S.login) && S.server.trim().length > 1 && (keep || S.pw.length >= 4)); }
  if (S.view === "loginerr") { const b = $('[data-act="retry"]'); if (b) b.disabled = !(S.pw.length >= 4 && S.server.trim().length > 1); }
});
document.addEventListener("change", (ev) => { if (ev.target.id === "broker") S.broker = ev.target.value; });
document.addEventListener("keydown", (ev) => {
  if (ev.key !== "Enter") return;
  const b = $('.foot .sp .btn:last-child');
  if (b && !b.disabled) b.click();
});
document.addEventListener("click", async (ev) => {
  const b = ev.target.closest("[data-act]");
  if (!b || b.disabled) return;
  const a = b.dataset.act;
  if (a === "next") { S.view = "broker"; render(); $("#login").focus(); }
  else if (a === "back") { S.view = "key"; render(); }
  else if (a === "toBroker") { S.view = "broker"; render(); }
  else if (a === "eye") { const i = b.parentElement.querySelector("input"); i.type = i.type === "password" ? "text" : "password"; }
  else if (a === "install") { if (S.view === "err" && !S.pw && !(S.chk && S.chk.saved && S.chk.saved.authOk)) { S.view = "broker"; render(); return; } startInstall(); }
  else if (a === "retry") { const pw = S.pw; S.pw = ""; S.view = "install"; S.tasks.login = "run"; S.tasks.chart = S.tasks.agent = S.tasks.online = undefined; render(); go().RetryLogin(S.server.trim(), pw); }
  else if (a === "portal") go() && go().OpenURL(((S.res && S.res.portal) || (S.chk && S.chk.portal) || "https://app.cruveno.com") + "/?tab=risk");
  else if (a === "mt5") go() && go().OpenMT5();
  else if (a === "quit") go() && go().Quit();
  else if (a === "uninstall") {
    b.disabled = true; b.textContent = T("Wird entfernt …", "Removing …");
    const e = await go().Uninstall();
    $("#unres").innerHTML = e ? `<div class="err">${ICO.infoBad}<span>${esc(e)}</span></div>` : `<div class="check">${ICO.ok}<span><b>${T("Cruveno wurde entfernt.", "Cruveno has been removed.")}</b></span></div>`;
    b.outerHTML = `<button class="btn p" data-act="quit">${T("Schließen", "Close")}</button>`;
  }
});

async function init() {
  if (go()) {
    S.info = await go().Info();
    window.runtime.EventsOn("prog", onProg);
    window.runtime.EventsOn("done", (r) => { S.res = r; S.view = "done"; render(); });
    window.runtime.EventsOn("loginfail", () => { S.view = "loginerr"; S.pw = ""; render(); });
    window.runtime.EventsOn("fail", (m) => { S.err = m; S.view = "err"; render(); });
  } else { // Vorschau im Browser
    const q = new URLSearchParams(location.search); S.info.os = q.get("os") || "win"; S.view = q.get("s") || "key";
  }
  if (S.info.mode === "uninstall") S.view = "uninstall";
  document.documentElement.lang = LANG;
  render();
}
// Wails stellt window.go kurz nach dem Laden bereit
(function wait(n) { if (go() || n > 40) init(); else setTimeout(() => wait(n + 1), 50); })(0);
