package main

import (
	"context"
	"embed"
	"os"

	"cruveno.com/installer/core"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func has(flag string) bool {
	for _, a := range os.Args[1:] {
		if a == flag {
			return true
		}
	}
	return false
}

func main() {
	// Hintergrunddienst: ohne Fenster
	if has("--agent") {
		if has("--loop") {
			core.AgentLoop(context.Background())
			return
		}
		core.AgentLog(core.AgentOnce(context.Background()))
		return
	}
	mode := "install"
	if has("--uninstall") {
		mode = "uninstall"
	}
	app := NewApp(mode)
	err := wails.Run(&options.App{
		Title:            "Cruveno Installer",
		Width:            960,
		Height:           640,
		MinWidth:         960,
		MinHeight:        640,
		DisableResize:    true,
		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		Bind:             []interface{}{app},
		Windows:          &windows.Options{Theme: windows.Light},
		Mac: &mac.Options{
			Appearance: mac.NSAppearanceNameAqua,
			About:      &mac.AboutInfo{Title: "Cruveno Installer", Message: "Version " + core.Version + "\n© Cruveno"},
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
