package main

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// version is overridden at release time with -ldflags "-X main.version=…".
var version = "2.0.0-dev"

// App is the object bound to the frontend. Its exported methods become the
// generated TypeScript functions in frontend/wailsjs/go/main/App.
type App struct {
	ctx context.Context
}

// NewApp creates the bound app object.
func NewApp() *App {
	return &App{}
}

// startup runs once the window exists. Services that touch user data are
// wired here in later items, never in main, so a second instance can't reach them.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// onSecondInstanceLaunch restores and focuses this window when the user
// launches LeGaJ again (R42).
func (a *App) onSecondInstanceLaunch(options.SecondInstanceData) {
	if a.ctx == nil {
		return
	}
	runtime.WindowUnminimise(a.ctx)
	runtime.Show(a.ctx)
}

// Version returns the app version for display in the UI.
func (a *App) Version() string {
	return version
}
