package main

import (
	"context"
	"errors"
	"strings"
	"sync"

	"cruveno.com/installer/core"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App: Bindings fuer die Oberflaeche (window.go.main.App.*)
type App struct {
	ctx  context.Context
	mode string
	mu   sync.Mutex
	busy bool
	key  string
}

func NewApp(mode string) *App { return &App{mode: mode} }

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

type Info struct {
	Version   string   `json:"version"`
	OS        string   `json:"os"`
	Mode      string   `json:"mode"`
	Existing  []string `json:"existing"`
	Installed []string `json:"installed"` // bereits eingerichtete Lizenzen (maskiert)
}

func (a *App) Info() Info {
	in := Info{Version: core.Version, OS: core.OSName(), Mode: a.mode, Existing: core.ExistingTerminals()}
	for _, st := range core.AllStates() {
		in.Installed = append(in.Installed, core.MaskKey(st.Key))
	}
	return in
}

type CheckOut struct {
	*core.CheckResp
	Err     string `json:"err"`
	Offline bool   `json:"offline"`
	Saved   *Saved `json:"saved"`
}

// Saved: was bei einer frueheren Einrichtung dieser Lizenz eingegeben wurde (ohne Passwort)
type Saved struct {
	Login  string `json:"login"`
	Server string `json:"server"`
	Broker string `json:"broker"`
	AuthOK bool   `json:"authOk"`
}

func (a *App) Check(key, lang string) CheckOut {
	k := core.NormKey(key)
	if !core.ValidKey(k) {
		return CheckOut{Err: "format"}
	}
	c, err := core.Check(a.ctx, k, lang)
	if err != nil {
		return CheckOut{Err: "offline", Offline: true}
	}
	out := CheckOut{CheckResp: c}
	if st := core.LoadState(k); st != nil {
		out.Saved = &Saved{Login: st.Login, Server: st.Server, Broker: st.Broker, AuthOK: st.AuthOK && core.EverAuthorized(st.Dir, st.Login)}
	}
	return out
}

func (a *App) emit(e core.Ev) { runtime.EventsEmit(a.ctx, "prog", e) }

func (a *App) finish(res *core.Result, err error) {
	a.mu.Lock()
	a.busy = false
	a.mu.Unlock()
	core.SetInstalling(false)
	switch {
	case errors.Is(err, core.ErrLogin):
		runtime.EventsEmit(a.ctx, "loginfail", res)
	case err != nil:
		runtime.EventsEmit(a.ctx, "fail", err.Error())
	default:
		runtime.EventsEmit(a.ctx, "done", res)
	}
}

func (a *App) start() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.busy {
		return false
	}
	a.busy = true
	core.SetInstalling(true)
	return true
}

// Install: laeuft im Hintergrund, meldet "prog", am Ende "done", "loginfail" oder "fail"
func (a *App) Install(rq core.InstallReq) string {
	rq.Key = core.NormKey(rq.Key)
	rq.Login = strings.TrimSpace(rq.Login)
	rq.Server = strings.TrimSpace(rq.Server)
	if !core.ValidKey(rq.Key) || rq.Login == "" || rq.Server == "" {
		return "input"
	}
	if !a.start() {
		return "busy"
	}
	a.key = rq.Key
	go func() {
		res, err := core.Install(a.ctx, rq, a.emit)
		rq.Password = ""
		a.finish(res, err)
	}()
	return ""
}

func (a *App) RetryLogin(server, password string) string {
	if a.key == "" {
		return "input"
	}
	if !a.start() {
		return "busy"
	}
	go func() {
		res, err := core.RetryLogin(a.ctx, a.key, server, password, a.emit)
		a.finish(res, err)
	}()
	return ""
}

func (a *App) OpenMT5() string {
	st := core.LoadState(a.key)
	if st == nil {
		return "none"
	}
	if err := core.OpenTerminal(st); err != nil {
		return err.Error()
	}
	return ""
}

func (a *App) OpenURL(u string) {
	if strings.HasPrefix(u, "https://") {
		runtime.BrowserOpenURL(a.ctx, u)
	}
}

func (a *App) Uninstall() string {
	if err := core.Uninstall(); err != nil {
		return err.Error()
	}
	return ""
}

func (a *App) Quit() { runtime.Quit(a.ctx) }
