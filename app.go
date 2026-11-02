package main

import (
	"context"
	"log/slog"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/apphome"
	"github.com/bot-bbio/legaj/internal/core"
	"github.com/bot-bbio/legaj/internal/store"
)

// version is overridden at release time with -ldflags "-X main.version=…".
var version = "2.0.0-dev"

// App is the object bound to the frontend. Its exported methods become the
// generated TypeScript functions in frontend/wailsjs/go/main/App.
type App struct {
	ctx    context.Context
	booted chan struct{} // closed once startup has run
	// pickFile shows the native open-file dialog; tests replace it.
	pickFile func(ctx context.Context) (string, error)

	mu    sync.Mutex
	core  *core.Core
	state StartupView
}

// NewApp creates the bound app object.
func NewApp() *App {
	return &App{booted: make(chan struct{}), pickFile: pickResumeFile}
}

// log returns the core's logger, or a discarding one before boot.
func (a *App) log() *slog.Logger {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.core == nil {
		return slog.New(slog.DiscardHandler)
	}
	return a.core.Log
}

// wailsEmitter delivers backend events to the frontend.
type wailsEmitter struct{ ctx context.Context }

func (e wailsEmitter) Emit(name string, payload any) {
	runtime.EventsEmit(e.ctx, name, payload)
}

// startup runs once the window exists, and only in the first instance
// (R42). It is the only place user data is opened.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	defer close(a.booted)

	dir, err := apphome.Dir()
	if err != nil {
		a.setStartup(nil, errorView(apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't find its data folder.")))
		return
	}
	c, st, err := core.Boot(ctx, core.Options{Dir: dir, Emitter: wailsEmitter{ctx}})
	if err != nil {
		a.setStartup(c, errorView(err))
		return
	}
	a.setStartup(c, toStartupView(st))
}

// shutdown closes the database and log when the window closes.
func (a *App) shutdown(context.Context) {
	a.mu.Lock()
	c := a.core
	a.mu.Unlock()
	if err := c.Close(); err != nil && c != nil {
		c.Log.Error("shutdown", "err", err)
	}
}

func (a *App) setStartup(c *core.Core, v StartupView) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.core, a.state = c, v
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

// StartupView tells the frontend whether the database is ready, or what
// the user must decide first (item 4's typed startup state).
type StartupView struct {
	// Kind is ready, corrupt, migration_failed or error.
	Kind       string       `json:"kind"`
	MovedAside string       `json:"movedAside"`
	Backups    []BackupView `json:"backups"`
	// Message is a user-safe explanation for the error kind.
	Message string `json:"message"`
}

// BackupView is one restorable backup.
type BackupView struct {
	Name      string `json:"name"`
	TakenAt   string `json:"takenAt"` // RFC 3339, UTC
	SizeBytes int64  `json:"sizeBytes"`
}

func toStartupView(st store.Startup) StartupView {
	v := StartupView{Kind: string(st.Kind), MovedAside: st.MovedAside, Backups: []BackupView{}}
	for _, b := range st.Backups {
		v.Backups = append(v.Backups, BackupView{Name: b.Name, TakenAt: b.TakenAt.UTC().Format("2006-01-02T15:04:05Z"), SizeBytes: b.SizeBytes})
	}
	return v
}

func errorView(err error) StartupView {
	return StartupView{Kind: "error", Backups: []BackupView{}, Message: apperr.UserMessage(err)}
}

// Startup returns the startup state, waiting for boot to finish.
func (a *App) Startup() StartupView {
	<-a.booted
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state
}

// RestoreBackup restores the named backup from the startup restore offer
// and reopens the database.
func (a *App) RestoreBackup(name string) (StartupView, error) {
	<-a.booted
	a.mu.Lock()
	c := a.core
	a.mu.Unlock()
	if c == nil {
		return StartupView{}, apperr.New(apperr.CodeConflict, "LeGaJ isn't ready to restore a backup.")
	}
	st, err := c.Restore(a.ctx, name)
	if err != nil {
		return StartupView{}, apperr.Public(err)
	}
	v := toStartupView(st)
	a.setStartup(c, v)
	return v, nil
}

// coreOnly returns the booted core, for calls that don't need the database.
func (a *App) coreOnly() (*core.Core, error) {
	<-a.booted
	a.mu.Lock()
	c := a.core
	a.mu.Unlock()
	if c == nil {
		return nil, core.ErrNotReady
	}
	return c, nil
}

// services returns the core and its database-backed services.
func (a *App) services() (*core.Core, *core.Services, error) {
	c, err := a.coreOnly()
	if err != nil {
		return nil, nil, err
	}
	svc, err := c.Ready()
	if err != nil {
		return nil, nil, err
	}
	return c, svc, nil
}

// Quit closes LeGaJ (the "Quit" choice on the restore screen).
func (a *App) Quit() {
	runtime.Quit(a.ctx)
}
