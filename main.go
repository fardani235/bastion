package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon128.png
var appIcon []byte

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "Bastion",
		Width:     1200,
		Height:    780,
		MinWidth:  900,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		Linux: &linux.Options{
			Icon: appIcon,
		},
		Mac: &mac.Options{
			About: &mac.AboutInfo{
				Icon: appIcon,
			},
		},
		Windows: &windows.Options{
			DisableWindowIcon: false,
		},
		BackgroundColour: &options.RGBA{R: 11, G: 15, B: 20, A: 1},
		// File upload uses a native file picker (PickFilesForUpload), not native
		// drag-and-drop: on Linux/WebKit2GTK a file dropped on the webview is
		// opened by the webview (navigating to file://) rather than yielding its
		// path, and the only suppression (gtk_drag_dest_unset) also kills path
		// delivery — so drag-drop can't both work and stay safe there.
		// DisableWebViewDrop neutralizes that accidental file-open, making a stray
		// drag a harmless no-op. EnableFileDrop is left off since we don't rely on
		// dropped paths.
		DragAndDrop: &options.DragAndDrop{
			DisableWebViewDrop: true,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []any{
			app,
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
