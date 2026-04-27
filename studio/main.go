package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

// Version is set at build time via -ldflags.
var Version = "dev"

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "FlutterProbe Studio",
		Width:     1480,
		Height:    920,
		MinWidth:  1040,
		MinHeight: 640,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
		Mac: &mac.Options{
			TitleBar: &mac.TitleBar{
				TitlebarAppearsTransparent: true,
				HideTitle:                  true,
				FullSizeContent:            true,
			},
			Appearance: mac.NSAppearanceNameDarkAqua,
			About: &mac.AboutInfo{
				Title:   "FlutterProbe Studio",
				Message: "Visual ProbeScript test authoring with embedded device view.\n\nCopyright © 2026 Alpha Wave Systems",
			},
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
