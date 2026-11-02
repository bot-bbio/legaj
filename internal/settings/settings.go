// Package settings holds the user's preferences (R37). They're stored in the
// DB, one JSON value per key, over defaults that live here, so a setting
// added in a later version simply starts at its default.
package settings

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/bot-bbio/legaj/internal/ai/gemini"
	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/store"
)

// Materials is what an application needs before it's Ready (R26).
type Materials string

// Materials choices.
const (
	MaterialsLetter  Materials = "letter"
	MaterialsAnswers Materials = "answers"
	MaterialsBoth    Materials = "both"
)

// Valid reports whether m is a known choice.
func (m Materials) Valid() bool {
	return m == MaterialsLetter || m == MaterialsAnswers || m == MaterialsBoth
}

// Settings are the user's preferences. Keep it a comparable value type.
type Settings struct {
	// Model is the Gemini model ID (R5, D21).
	Model string `json:"model"`
	// SkipReview makes bulk generation produce PDFs without review (R23).
	SkipReview bool `json:"skipReview"`
	// DefaultMaterials pre-fills each new application's materials choice (R26).
	DefaultMaterials Materials `json:"defaultMaterials"`
	// FollowUpDays is how long after applying a follow-up becomes due (R28).
	FollowUpDays int `json:"followUpDays"`
	// UpdateCheck allows the GitHub release check (R40).
	UpdateCheck bool `json:"updateCheck"`
	// StartWithWindows adds LeGaJ to HKCU\...\Run, so the clipper works
	// right after login (R18, R37).
	StartWithWindows bool `json:"startWithWindows"`
}

// Defaults are the settings before the user changes anything.
func Defaults() Settings {
	return Settings{
		Model:            gemini.DefaultModel,
		SkipReview:       false,
		DefaultMaterials: MaterialsLetter,
		FollowUpDays:     7,
		UpdateCheck:      true,
		StartWithWindows: false,
	}
}

// Follow-up delay bounds, in days.
const (
	minFollowUpDays = 1
	maxFollowUpDays = 60
)

// Validate checks a settings value before it's saved.
func Validate(s Settings) error {
	switch {
	case !gemini.ValidModel(s.Model):
		return apperr.New(apperr.CodeInvalid, "Enter a Gemini model ID such as "+gemini.DefaultModel+".")
	case !s.DefaultMaterials.Valid():
		return apperr.New(apperr.CodeInvalid, "Choose letter, answers, or both as the default materials.")
	case s.FollowUpDays < minFollowUpDays || s.FollowUpDays > maxFollowUpDays:
		return apperr.New(apperr.CodeInvalid, fmt.Sprintf("Follow-up delay must be between %d and %d days.", minFollowUpDays, maxFollowUpDays))
	}
	return nil
}

// Autostart controls the HKCU Run entry. Implemented for Windows in
// autostart_windows.go; tests use a fake.
type Autostart interface {
	Enable(command string) error
	Disable() error
	// Current reports the entry's command and whether it exists.
	Current() (command string, enabled bool, err error)
}

// Service reads and writes settings.
type Service struct {
	saveMu    sync.Mutex // one Save at a time: a revert must not undo another save
	db        *store.DB
	bus       *events.Bus
	autostart Autostart
	exe       func() (string, error)
}

// New creates the service. exe returns this program's path (os.Executable).
func New(db *store.DB, bus *events.Bus, autostart Autostart, exe func() (string, error)) *Service {
	return &Service{db: db, bus: bus, autostart: autostart, exe: exe}
}

// Get returns the saved settings over the defaults.
func (s *Service) Get(ctx context.Context) (Settings, error) {
	rows, err := s.db.Reader().QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return Settings{}, readErr(err)
	}
	defer rows.Close()
	stored := map[string]json.RawMessage{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return Settings{}, readErr(err)
		}
		stored[k] = json.RawMessage(v)
	}
	if err := rows.Err(); err != nil {
		return Settings{}, readErr(err)
	}
	// Overlay stored values on the defaults by round-tripping through JSON:
	// unknown keys are ignored, missing keys keep their default.
	out := Defaults()
	merged, err := json.Marshal(stored)
	if err != nil {
		return Settings{}, readErr(err)
	}
	if err := json.Unmarshal(merged, &out); err != nil {
		return Settings{}, readErr(err)
	}
	return out, nil
}

// Save validates and stores the settings, then applies Start with Windows
// to the registry. If the registry refuses, the previous settings are put
// back and an error returned, so the setting and Windows agree. Saves are
// serialized, so that revert can never undo another save. Emits a settings
// change (R3).
func (s *Service) Save(ctx context.Context, in Settings) (Settings, error) {
	if err := Validate(in); err != nil {
		return Settings{}, err
	}
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	current, err := s.Get(ctx)
	if err != nil {
		return Settings{}, err
	}
	// The DB commits first, then Windows is told. If the registry refuses,
	// the stored settings are put back, so the two never disagree; should
	// even that fail, Reconcile at the next start heals it.
	if err := s.store(ctx, in); err != nil {
		return Settings{}, err
	}
	if in.StartWithWindows != current.StartWithWindows {
		if err := s.applyAutostart(in.StartWithWindows); err != nil {
			if revertErr := s.store(ctx, current); revertErr != nil {
				// Reconcile at the next start re-applies the stored value.
				return Settings{}, errors.Join(err, revertErr)
			}
			return Settings{}, err
		}
	}
	return in, nil
}

// store writes every setting in one transaction and emits a change.
func (s *Service) store(ctx context.Context, in Settings) error {
	values, err := toKeyValues(in)
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, apperr.GenericMessage)
	}
	return events.Mutate(ctx, s.db, s.bus, events.Change(events.EntitySettings), func(tx *sql.Tx) error {
		for k, v := range values {
			if _, err := tx.ExecContext(ctx, `INSERT INTO settings(key, value) VALUES (?, ?)
				ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, string(v)); err != nil {
				return apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't save your settings. Try again.")
			}
		}
		return nil
	})
}

// Reconcile repairs a mismatch between the saved setting and the HKCU Run
// entry, at startup: it adds a missing entry or removes an unwanted one.
// An existing entry is never rewritten, so starting a second copy of
// LeGaJ (say, a dev build) can't take autostart over.
func (s *Service) Reconcile(ctx context.Context) error {
	cur, err := s.Get(ctx)
	if err != nil {
		return err
	}
	_, enabled, err := s.autostart.Current()
	if err != nil {
		return err
	}
	if enabled == cur.StartWithWindows {
		return nil
	}
	return s.applyAutostart(cur.StartWithWindows)
}

func (s *Service) applyAutostart(on bool) error {
	fail := func(err error) error {
		return apperr.Wrap(err, apperr.CodeStorage, "Windows didn't accept the Start with Windows change.")
	}
	if !on {
		if err := s.autostart.Disable(); err != nil {
			return fail(err)
		}
		return nil
	}
	exe, err := s.exe()
	if err != nil {
		return fail(err)
	}
	if err := s.autostart.Enable(`"` + exe + `"`); err != nil {
		return fail(err)
	}
	return nil
}

// toKeyValues splits a Settings value into its stored JSON keys.
func toKeyValues(in Settings) (map[string]json.RawMessage, error) {
	data, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New("settings: nothing to store")
	}
	return out, nil
}

func readErr(err error) error {
	return apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't read your settings.")
}
