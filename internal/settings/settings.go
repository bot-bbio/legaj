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
}

// Service reads and writes settings.
type Service struct {
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

// Save validates and stores the settings, applying Start with Windows to
// the registry in the same step: if the registry write fails, nothing is
// saved. It emits a settings change on success (R3).
func (s *Service) Save(ctx context.Context, in Settings) (Settings, error) {
	if err := Validate(in); err != nil {
		return Settings{}, err
	}
	current, err := s.Get(ctx)
	if err != nil {
		return Settings{}, err
	}
	values, err := toKeyValues(in)
	if err != nil {
		return Settings{}, apperr.Wrap(err, apperr.CodeInternal, apperr.GenericMessage)
	}
	err = events.Mutate(ctx, s.db, s.bus, events.Change(events.EntitySettings), func(tx *sql.Tx) error {
		for k, v := range values {
			if _, err := tx.ExecContext(ctx, `INSERT INTO settings(key, value) VALUES (?, ?)
				ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, string(v)); err != nil {
				return apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't save your settings. Try again.")
			}
		}
		// Last, so a registry failure rolls the DB writes back.
		if in.StartWithWindows != current.StartWithWindows {
			return s.applyAutostart(in.StartWithWindows)
		}
		return nil
	})
	if err != nil {
		return Settings{}, err
	}
	return in, nil
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
