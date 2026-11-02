// Package core boots LeGaJ's backend: the redacting log, the database and
// the change-event bus. Feature services hang off Core as later items add
// them. Boot never panics. A database that needs the user's decision
// (corrupt, failed migration) yields a typed startup state and a nil DB.
package core

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/bot-bbio/legaj/internal/ai"
	"github.com/bot-bbio/legaj/internal/ai/gemini"
	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/logx"
	"github.com/bot-bbio/legaj/internal/profile"
	"github.com/bot-bbio/legaj/internal/resume"
	"github.com/bot-bbio/legaj/internal/secrets"
	"github.com/bot-bbio/legaj/internal/settings"
	"github.com/bot-bbio/legaj/internal/store"
)

// Options configures Boot. Zero values select the real implementations.
type Options struct {
	// Dir is the data folder (apphome.Dir()).
	Dir string
	// Now is the clock; default time.Now.
	Now func() time.Time
	// Emitter delivers change events to the frontend.
	Emitter events.Emitter
	// Secrets stores the API key; default Windows Credential Manager.
	Secrets secrets.Store
	// Autostart manages the HKCU Run entry; default the registry.
	Autostart settings.Autostart
	// Exe returns this program's path; default os.Executable.
	Exe func() (string, error)
	// AI is the model provider; default the Gemini client.
	AI ai.Provider
}

// Core is the running backend.
type Core struct {
	Dir      string
	Now      func() time.Time
	Log      *slog.Logger
	Redactor *logx.Redactor
	Bus      *events.Bus
	// Secrets works before the database is ready (it doesn't use it).
	Secrets *secrets.Manager
	// AI reads the key from Secrets on every call.
	AI ai.Provider

	opts    Options
	mu      sync.Mutex
	svc     *Services // nil until the startup state is ready
	logFile io.Closer
}

// Services are the parts of the backend that need the database. They
// become available together, once the startup state is ready.
type Services struct {
	DB       *store.DB
	Settings *settings.Service
	Profile  *profile.Service
	Resume   *resume.Service
}

// model is the Gemini model chosen in Settings, or the default.
func (s *Services) model(ctx context.Context) string {
	st, err := s.Settings.Get(ctx)
	if err != nil {
		return gemini.DefaultModel
	}
	return st.Model
}

// ErrNotReady is returned by Ready before the database is open.
var ErrNotReady = apperr.New(apperr.CodeConflict, "LeGaJ is still starting. Try again in a moment.")

// Ready returns the services, or ErrNotReady. Safe from any goroutine:
// Wails runs bound methods concurrently.
func (c *Core) Ready() (*Services, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.svc == nil {
		return nil, ErrNotReady
	}
	return c.svc, nil
}

// DB returns the open database, or nil while the startup state isn't ready.
func (c *Core) DB() *store.DB {
	s, err := c.Ready()
	if err != nil {
		return nil
	}
	return s.DB
}

// Boot opens the log and the database. err is non-nil only when even the
// log can't be opened, or the database failed for a reason other than its
// own content (permissions, a lock).
func Boot(ctx context.Context, opts Options) (*Core, store.Startup, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Secrets == nil {
		opts.Secrets = secrets.NewKeyring()
	}
	if opts.Autostart == nil {
		opts.Autostart = settings.RegistryAutostart{}
	}
	if opts.Exe == nil {
		opts.Exe = os.Executable
	}
	redactor := logx.NewRedactor()
	log, logFile, err := logx.NewFile(opts.Dir, redactor, slog.LevelInfo)
	if err != nil {
		return nil, store.Startup{}, apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't open its data folder.")
	}
	c := &Core{
		Dir: opts.Dir, Now: opts.Now, Log: log, Redactor: redactor,
		Bus:     events.NewBus(opts.Emitter),
		Secrets: secrets.NewManager(opts.Secrets, redactor),
		opts:    opts, logFile: logFile,
	}
	c.AI = opts.AI
	if c.AI == nil {
		c.AI = gemini.New(gemini.Config{Key: c.Secrets.Load, Logger: log})
	}
	// Register a saved key with the log redactor right away (R4, R34).
	if _, err := c.Secrets.Status(); err != nil {
		c.Log.Warn("could not read the API key status", "err", err)
	}
	st, err := c.open(ctx)
	if err != nil {
		return c, st, err
	}
	return c, st, nil
}

// open opens the database and, when ready, takes the daily backup.
func (c *Core) open(ctx context.Context) (store.Startup, error) {
	db, st, err := store.Open(ctx, store.Options{Dir: c.Dir, Now: c.Now})
	if err != nil {
		c.Log.Error("database open failed", "err", err)
		return st, apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't open its database. Close other programs that might be using it and start LeGaJ again.")
	}
	if st.Kind != store.StartupReady {
		c.Log.Warn("database needs attention", "kind", string(st.Kind), "backups", len(st.Backups))
		return st, nil
	}
	if _, err := db.DailyBackup(ctx); err != nil {
		// A failed backup must not stop the user from working; it's retried
		// on the next start.
		c.Log.Error("daily backup failed", "err", err)
	}
	svc := &Services{
		DB:       db,
		Settings: settings.New(db, c.Bus, c.opts.Autostart, c.opts.Exe),
		Profile:  profile.NewService(db, c.Bus, c.Now),
	}
	svc.Resume = resume.NewService(resume.Config{
		DB: db, Bus: c.Bus, Dir: c.Dir, AI: c.AI, Now: c.Now,
		Model: svc.model,
	})
	c.mu.Lock()
	c.svc = svc
	c.mu.Unlock()
	c.Log.Info("database ready")
	return st, nil
}

// Restore replaces the database with a backup the user chose from the
// startup restore offer, then opens it. It's refused while a database is open.
func (c *Core) Restore(ctx context.Context, backupName string) (store.Startup, error) {
	if _, err := c.Ready(); err == nil {
		return store.Startup{}, apperr.New(apperr.CodeConflict, "A backup can only be restored when LeGaJ starts with a damaged database.")
	}
	if err := store.Restore(c.Dir, backupName, c.Now); err != nil {
		c.Log.Error("restore failed", "err", err)
		return store.Startup{}, apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't restore that backup.")
	}
	c.Log.Info("restored backup", "name", backupName)
	return c.open(ctx)
}

// Close releases the database and the log file.
func (c *Core) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	svc := c.svc
	c.svc = nil
	c.mu.Unlock()
	var errs []error
	if svc != nil {
		errs = append(errs, svc.DB.Close())
	}
	if c.logFile != nil {
		errs = append(errs, c.logFile.Close())
	}
	return errors.Join(errs...)
}
