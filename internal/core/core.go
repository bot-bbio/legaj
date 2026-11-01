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
	"sync"
	"time"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/logx"
	"github.com/bot-bbio/legaj/internal/store"
)

// Options configures Boot.
type Options struct {
	// Dir is the data folder (apphome.Dir()).
	Dir string
	// Now is the clock; default time.Now.
	Now func() time.Time
	// Emitter delivers change events to the frontend.
	Emitter events.Emitter
}

// Core is the running backend.
type Core struct {
	Dir      string
	Now      func() time.Time
	Log      *slog.Logger
	Redactor *logx.Redactor
	Bus      *events.Bus

	mu      sync.Mutex
	db      *store.DB // nil until the startup state is ready
	logFile io.Closer
}

// DB returns the open database, or nil while the startup state isn't ready.
// Safe from any goroutine: Wails runs bound methods concurrently.
func (c *Core) DB() *store.DB {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.db
}

// Boot opens the log and the database. err is non-nil only when even the
// log can't be opened, or the database failed for a reason other than its
// own content (permissions, a lock).
func Boot(ctx context.Context, opts Options) (*Core, store.Startup, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	redactor := logx.NewRedactor()
	log, logFile, err := logx.NewFile(opts.Dir, redactor, slog.LevelInfo)
	if err != nil {
		return nil, store.Startup{}, apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't open its data folder.")
	}
	c := &Core{Dir: opts.Dir, Now: opts.Now, Log: log, Redactor: redactor, Bus: events.NewBus(opts.Emitter), logFile: logFile}
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
	c.mu.Lock()
	c.db = db
	c.mu.Unlock()
	c.Log.Info("database ready")
	return st, nil
}

// Restore replaces the database with a backup the user chose from the
// startup restore offer, then opens it. It's refused while a database is open.
func (c *Core) Restore(ctx context.Context, backupName string) (store.Startup, error) {
	c.mu.Lock()
	open := c.db != nil
	c.mu.Unlock()
	if open {
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
	db := c.db
	c.db = nil
	c.mu.Unlock()
	var errs []error
	if db != nil {
		errs = append(errs, db.Close())
	}
	if c.logFile != nil {
		errs = append(errs, c.logFile.Close())
	}
	return errors.Join(errs...)
}
