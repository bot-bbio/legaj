// Package onboarding backs the first-run wizard (R7).
//
// Step completion is derived from the data itself: a key is saved, a
// resume is uploaded, the profile has a name, a job has been clipped. So
// a skipped step stays on the resumable checklist until it's actually
// done, and there's no separate bookkeeping to drift. Only "the user went
// through the wizard" is stored.
package onboarding

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/secrets"
	"github.com/bot-bbio/legaj/internal/store"
)

// StepID names a wizard step.
type StepID string

// Wizard steps, in order (R7).
const (
	StepKey     StepID = "key"
	StepResume  StepID = "resume"
	StepProfile StepID = "profile"
	StepClipper StepID = "clipper"
)

// Step is one checklist entry.
type Step struct {
	ID   StepID `json:"id"`
	Done bool   `json:"done"`
}

// State is the wizard's and the checklist's view of setup.
type State struct {
	// Finished means the user went through (or skipped through) the wizard.
	Finished bool   `json:"finished"`
	Steps    []Step `json:"steps"`
}

// KeyResult reports a saved key's verification.
type KeyResult struct {
	Verified bool   `json:"verified"`
	Message  string `json:"message"`
}

// Config wires the service.
type Config struct {
	DB      *store.DB
	Bus     *events.Bus
	Secrets *secrets.Manager
	// TestKey makes one tiny AI call with key. It returns nil when the key
	// works, an apperr with CodeAIKeyRejected when it doesn't, and other
	// errors when it couldn't tell.
	TestKey func(ctx context.Context, key string) error
}

// Service computes onboarding state and saves the key.
type Service struct{ cfg Config }

// NewService creates the service.
func NewService(cfg Config) *Service { return &Service{cfg: cfg} }

// finishedKey is stored in the settings table (unknown to settings.Settings).
const finishedKey = "onboarding.finished"

// State derives each step's completion from the data.
func (s *Service) State(ctx context.Context) (State, error) {
	key, err := s.cfg.Secrets.Status()
	if err != nil {
		return State{}, err
	}
	r := s.cfg.DB.Reader()
	var resume, profile, clipped, finished bool
	var finishedJSON sql.NullString
	err = r.QueryRowContext(ctx, `SELECT
		EXISTS (SELECT 1 FROM resume_files),
		EXISTS (SELECT 1 FROM profile WHERE id = 1 AND trim(coalesce(json_extract(data, '$.fullName'), '')) <> ''),
		EXISTS (SELECT 1 FROM applications WHERE source = 'clip'),
		(SELECT value FROM settings WHERE key = ?)`, finishedKey).Scan(&resume, &profile, &clipped, &finishedJSON)
	if err != nil {
		return State{}, apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't read your setup progress.")
	}
	if finishedJSON.Valid {
		_ = json.Unmarshal([]byte(finishedJSON.String), &finished)
	}
	return State{
		Finished: finished,
		Steps: []Step{
			{ID: StepKey, Done: key.Saved},
			{ID: StepResume, Done: resume},
			{ID: StepProfile, Done: profile},
			{ID: StepClipper, Done: clipped},
		},
	}, nil
}

// Finish records that the user went through the wizard. Unfinished steps
// remain on the checklist.
func (s *Service) Finish(ctx context.Context) error {
	err := events.Mutate(ctx, s.cfg.DB, s.cfg.Bus, events.Change(events.EntitySettings), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO settings(key, value) VALUES (?, 'true')
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`, finishedKey)
		return err
	})
	if err != nil {
		return apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't save your setup progress.")
	}
	return nil
}

// SaveKey tests a pasted key with one small Gemini call, then saves it.
// A rejected key is not saved. When Gemini is busy or unreachable the key
// is saved anyway, with a note that it couldn't be verified yet.
func (s *Service) SaveKey(ctx context.Context, raw string) (KeyResult, error) {
	key, err := secrets.Normalize(raw)
	if err != nil {
		return KeyResult{}, err
	}
	s.cfg.Secrets.Protect(key)
	res := KeyResult{Verified: true, Message: "Key works. Saved to Windows Credential Manager."}
	if err := s.cfg.TestKey(ctx, key); err != nil {
		// Checked first: the AI client reports a cancelled call as
		// "unavailable", and an untested key must not be saved on shutdown.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			return KeyResult{}, err
		}
		switch apperr.CodeOf(err) {
		case apperr.CodeAIKeyRejected:
			return KeyResult{}, err
		case apperr.CodeAIRateLimited, apperr.CodeAIUnavailable:
			res = KeyResult{Message: "Saved. Gemini couldn't be reached to check the key, so LeGaJ will check it on first use."}
		case apperr.CodeAIBilling:
			// The key is fine; Gemini needs billing on in this region (R63).
			res = KeyResult{Message: "Saved. " + err.Error()}
		default:
			res = KeyResult{Message: "Saved, but LeGaJ couldn't confirm the key works."}
		}
	}
	if err := s.cfg.Secrets.Save(key); err != nil {
		return KeyResult{}, err
	}
	s.cfg.Bus.Emit(events.Change(events.EntitySettings))
	return res, nil
}
