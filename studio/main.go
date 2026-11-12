package main

import (
	"embed"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/zalando/go-keyring"
)

//go:embed all:frontend/dist
var assets embed.FS

// Version is set at build time via -ldflags.
var Version = "dev"

// keychainMemoryRequested says whether keys are kept in memory instead of the platform keychain. Only the headless build does,
// and only when asked, so a person running the app never loses a saved key to it.
func keychainMemoryRequested(headless bool, env string) bool {
	return headless && env == "memory"
}

func main() {
	app := NewApp()
	headless := os.Getenv("PROBE_STUDIO_HEADLESS") == "1"
	if keychainMemoryRequested(headless, os.Getenv("PROBE_STUDIO_KEYCHAIN")) {
		keyring.MockInit() // the browser suite keeps keys in memory, so no test touches the real keychain
	}
	if headless {
		runHeadless(app)
		return
	}

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
			// Wails makes the web view full-size whenever the title bar is transparent,
			// which puts it under the title bar and its drag area, so keep it opaque.
			TitleBar: &mac.TitleBar{
				TitlebarAppearsTransparent: false,
				HideTitle:                  true,
				FullSizeContent:            false,
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
