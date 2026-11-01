// Command legaj is the LeGaJ desktop app: a local job-application assistant.
package main

import (
	"embed"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/bot-bbio/legaj/internal/instance"
)

// The built frontend. frontend/dist/.gitkeep is committed so this pattern
// always matches, even on a clean checkout before the frontend is built.
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// The single-instance gate runs before anything else (R42): a second
	// launch focuses the running window and exits without opening the DB,
	// binding the clip port or starting the job queue.
	_, err := instance.Gate(instance.MutexName, func() { instance.ActivateFirst(instance.AppID) }, run)
	if err != nil {
		os.Exit(1)
	}
}

// run starts the Wails app. It only runs in the first instance; everything
// that touches user data starts in App.startup, never before this point.
func run() error {
	app := NewApp()
	return wails.Run(&options.App{
		Title:     "LeGaJ",
		Width:     1280,
		Height:    820,
		MinWidth:  960,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               instance.AppID,
			OnSecondInstanceLaunch: app.onSecondInstanceLaunch,
		},
		Bind: bindings(app),
	})
}

// bindings lists every object whose methods the frontend can call. Keep it
// the single list: the R4 test reflects over it to prove no method returns
// the API key.
func bindings(app *App) []any {
	return []any{
		app,
		&SettingsAPI{app: app},
	}
}
