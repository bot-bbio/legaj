// Package core boots LeGaJ's backend: the redacting log, the database and
// the change-event bus. Feature services hang off Core as later items add
// them. Boot never panics. A database that needs the user's decision
// (corrupt, failed migration) yields a typed startup state and a nil DB.
package core

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/bot-bbio/legaj/internal/ai"
	"github.com/bot-bbio/legaj/internal/ai/gemini"
	"github.com/bot-bbio/legaj/internal/answers"
	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/documents"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/followups"
	"github.com/bot-bbio/legaj/internal/interviews"
	"github.com/bot-bbio/legaj/internal/jobs"
	"github.com/bot-bbio/legaj/internal/letters"
	"github.com/bot-bbio/legaj/internal/logx"
	"github.com/bot-bbio/legaj/internal/onboarding"
	"github.com/bot-bbio/legaj/internal/opener"
	"github.com/bot-bbio/legaj/internal/pipeline"
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
	// KeyTester checks a candidate API key; default one small Gemini call.
	KeyTester func(ctx context.Context, key string) error
	// Launcher opens validated links and files; default ShellExecute.
	Launcher opener.Launcher
	// BackupEvery is how often a running app checks whether today's backup
	// exists; default one hour.
	BackupEvery time.Duration
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
	// Opener is the only way to open a link or a file (R33).
	Opener *opener.Opener

	opts      Options
	mu        sync.Mutex
	svc       *Services // nil until the startup state is ready
	logFile   io.Closer
	restoreMu sync.Mutex // one Restore at a time, check through open
	stopLoop  chan struct{}
	loopDone  sync.WaitGroup
}

// Services are the parts of the backend that need the database. They
// become available together, once the startup state is ready.
type Services struct {
	DB         *store.DB
	Settings   *settings.Service
	Profile    *profile.Service
	Resume     *resume.Service
	Onboarding *onboarding.Service
	Stages     *pipeline.Machine
	Pipeline   *pipeline.Service
	Documents  *documents.Service
	Letters    *letters.Service
	// Jobs runs letter generation in the background (R23).
	Jobs *jobs.Queue
	// Answers drafts form answers and keeps the answer bank (R25).
	Answers *answers.Service
	// Followups decides what's due and records what was sent (R27-R29).
	Followups *followups.Service
	// Interviews keeps interview rounds (R30-R32).
	Interviews *interviews.Service
}

// geminiKeyTester makes one tiny call with the candidate key. Any 200
// proves the key works, even if the answer itself was cut short.
func geminiKeyTester(log *slog.Logger, model func(context.Context) string) func(context.Context, string) error {
	return func(ctx context.Context, key string) error {
		client := gemini.New(gemini.Config{Key: func() (string, error) { return key, nil }, Logger: log})
		zero := 0.0
		_, err := client.Generate(ctx, ai.Request{
			Model:       model(ctx),
			Parts:       []ai.Part{ai.Text("Reply with the single word OK.")},
			Temperature: &zero,
		})
		if apperr.CodeOf(err) == apperr.CodeAIInvalidOutput {
			return nil
		}
		return err
	}
}

// model is the Gemini model chosen in Settings, or the default.
func (s *Services) model(ctx context.Context) string {
	st, err := s.Settings.Get(ctx)
	if err != nil {
		return gemini.DefaultModel
	}
	return st.Model
}

// defaultMaterials is the Settings default for new applications (R26).
func (s *Services) defaultMaterials(ctx context.Context) store.Materials {
	st, err := s.Settings.Get(ctx)
	if err != nil {
		return store.MaterialsLetter
	}
	return st.DefaultMaterials
}

// followUpDays is the Settings delay before an applied nudge is due (R28).
func (s *Services) followUpDays(ctx context.Context) int {
	st, err := s.Settings.Get(ctx)
	if err != nil {
		return settings.Defaults().FollowUpDays
	}
	return st.FollowUpDays
}

// skipReview reports the Skip-review setting (R23). If settings can't be
// read, review stays on: a draft is the safe outcome.
func (s *Services) skipReview(ctx context.Context) bool {
	st, err := s.Settings.Get(ctx)
	return err == nil && st.SkipReview
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
	if opts.Launcher == nil {
		opts.Launcher = opener.ShellExecute{}
	}
	c.Opener = opener.New(opts.Dir, opts.Launcher)
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
	tester := c.opts.KeyTester
	if tester == nil {
		tester = geminiKeyTester(c.Log, svc.model)
	}
	svc.Onboarding = onboarding.NewService(onboarding.Config{DB: db, Bus: c.Bus, Secrets: c.Secrets, TestKey: tester})
	svc.Stages = pipeline.NewMachine(c.Now)
	svc.Pipeline = pipeline.NewService(pipeline.Config{
		DB: db, Bus: c.Bus, Machine: svc.Stages, Now: c.Now,
		DefaultMaterials: svc.defaultMaterials,
	})
	svc.Documents = documents.NewService(db, c.Bus, c.Dir, c.Now)
	// Approving a letter applies the Ready rule in the same transaction (R26).
	svc.Documents.OnApproved(func(ctx context.Context, tx *sql.Tx, appID int64) error {
		_, err := svc.Stages.CheckReady(ctx, tx, appID)
		return err
	})
	svc.Letters = letters.NewService(letters.Config{
		DB: db, Bus: c.Bus, AI: c.AI, Profile: svc.Profile, Model: svc.model, Now: c.Now,
		Documents: svc.Documents, SkipReview: svc.skipReview,
	})
	svc.Jobs = jobs.New(jobs.Config{Bus: c.Bus, Work: svc.Letters.Generate})
	svc.Answers = answers.NewService(answers.Config{
		DB: db, Bus: c.Bus, AI: c.AI, Profile: svc.Profile, Machine: svc.Stages, Model: svc.model, Now: c.Now,
	})
	svc.Followups = followups.NewService(followups.Config{
		DB: db, Bus: c.Bus, Now: c.Now, Days: svc.followUpDays, Profile: svc.Profile,
	})
	svc.Interviews = interviews.NewService(interviews.Config{
		DB: db, Bus: c.Bus, Machine: svc.Stages, Now: c.Now, AI: c.AI, Profile: svc.Profile, Model: svc.model,
	})
	svc.Jobs.Start(context.Background())
	// Deleting rows cancels their queued and running generation (R13).
	svc.Pipeline.OnDelete(svc.Jobs.CancelApps)
	// Rows deleted in a previous session are past their undo window (R13).
	if n, err := svc.Pipeline.PurgeDeleted(ctx); err != nil {
		c.Log.Error("purge of deleted applications failed", "err", err)
	} else if n > 0 {
		c.Log.Info("purged deleted applications", "count", n)
	}
	// Make the Run entry match the saved setting, healing any save that
	// changed one but not the other (R37).
	if err := svc.Settings.Reconcile(ctx); err != nil {
		c.Log.Warn("start-with-Windows reconcile failed", "err", err)
	}
	// Publish the services and register the backup loop in one critical
	// section, so a concurrent Close always sees and stops the loop.
	stop := make(chan struct{})
	c.mu.Lock()
	c.svc = svc
	c.stopLoop = stop
	c.loopDone.Add(1)
	c.mu.Unlock()
	c.startBackupLoop(db, stop)
	c.Log.Info("database ready")
	return st, nil
}

// startBackupLoop keeps the "daily" in daily backups true while LeGaJ stays
// open for days (it may start with Windows and never restart).
func (c *Core) startBackupLoop(db *store.DB, stop <-chan struct{}) {
	every := c.opts.BackupEvery
	if every <= 0 {
		every = time.Hour
	}
	go func() {
		defer c.loopDone.Done() // Add(1) happened under c.mu in open
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if _, err := db.DailyBackup(context.Background()); err != nil {
					c.Log.Error("daily backup failed", "err", err)
				}
			}
		}
	}()
}

// Restore replaces the database with a backup the user chose from the
// startup restore offer, then opens it. It's refused while a database is
// open, and concurrent calls are serialized so only one can restore.
func (c *Core) Restore(ctx context.Context, backupName string) (store.Startup, error) {
	c.restoreMu.Lock()
	defer c.restoreMu.Unlock()
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
	if c.stopLoop != nil {
		close(c.stopLoop)
		c.stopLoop = nil
	}
	c.mu.Unlock()
	c.loopDone.Wait() // no backup may run against a closing database
	var errs []error
	if svc != nil {
		svc.Jobs.Close() // stop generation before its database goes away
		errs = append(errs, svc.DB.Close())
	}
	if c.logFile != nil {
		errs = append(errs, c.logFile.Close())
	}
	return errors.Join(errs...)
}
