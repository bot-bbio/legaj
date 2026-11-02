// Package pipeline owns applications and their stages (R11-R16).
//
// stage.go is the state machine (C19). Every stage change, whether made by
// the user, by Reopen or by an automatic rule (R26, R30), goes through it,
// runs inside the caller's transaction, and is logged to stage_events with
// its cause.
package pipeline

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/store"
)

// order ranks the active stages; terminal stages have no rank.
var order = map[store.Stage]int{
	store.StageLead:         0,
	store.StageReady:        1,
	store.StageApplied:      2,
	store.StageInterviewing: 3,
	store.StageOffer:        4,
}

func terminal(s store.Stage) bool {
	return s == store.StageRejected || s == store.StageWithdrawn
}

// CanTransition reports whether the user may move a row from one stage to
// another without Reopen: forward (skipping is fine), to Rejected or
// Withdrawn from any active stage, or Ready back to Lead (nothing has been
// sent yet). Other backward moves, and anything out of a terminal stage,
// need Reopen.
func CanTransition(from, to store.Stage) bool {
	if from == to || !from.Valid() || !to.Valid() || terminal(from) {
		return false
	}
	if terminal(to) {
		return true
	}
	if from == store.StageReady && to == store.StageLead {
		return true
	}
	return order[to] > order[from]
}

// CanReopen reports whether Reopen may move a row to an earlier active
// stage, or out of Rejected or Withdrawn.
func CanReopen(from, to store.Stage) bool {
	if from == to || !from.Valid() || !to.Valid() || terminal(to) {
		return false
	}
	return terminal(from) || order[to] < order[from]
}

// AutoRule is an automatic transition: it moves a row to Target only when
// its current stage is one of From.
type AutoRule struct {
	Name   string
	Target store.Stage
	From   []store.Stage
}

// The automatic rules. Callers must use these, never Transition.
var (
	// RuleMaterialsApproved: all chosen materials approved -> Ready, from Lead only (R26).
	RuleMaterialsApproved = AutoRule{Name: "materials_approved", Target: store.StageReady, From: []store.Stage{store.StageLead}}
	// RuleRoundAdded: first interview round -> Interviewing, from Lead, Ready or Applied (R30).
	RuleRoundAdded = AutoRule{Name: "round_added", Target: store.StageInterviewing, From: []store.Stage{store.StageLead, store.StageReady, store.StageApplied}}
)

// Change describes what a call did.
type Change struct {
	From    store.Stage `json:"from"`
	To      store.Stage `json:"to"`
	Changed bool        `json:"changed"`
}

// Machine applies stage changes.
type Machine struct {
	now func() time.Time
}

// NewMachine returns a state machine using the given clock.
func NewMachine(now func() time.Time) *Machine {
	if now == nil {
		now = time.Now
	}
	return &Machine{now: now}
}

var stageLabel = map[store.Stage]string{
	store.StageLead: "Lead", store.StageReady: "Ready", store.StageApplied: "Applied",
	store.StageInterviewing: "Interviewing", store.StageOffer: "Offer",
	store.StageRejected: "Rejected", store.StageWithdrawn: "Withdrawn",
}

// Transition is a user's stage change. Moving to the current stage is a
// no-op. reason is recorded on the event (e.g. "bulk").
func (m *Machine) Transition(ctx context.Context, tx *sql.Tx, appID int64, to store.Stage, reason string) (Change, error) {
	from, err := currentStage(ctx, tx, appID)
	if err != nil {
		return Change{}, err
	}
	if from == to {
		return Change{From: from, To: to}, nil
	}
	if !CanTransition(from, to) {
		return Change{From: from, To: from}, apperr.New(apperr.CodeConflict,
			fmt.Sprintf("Can't move from %s to %s. Use Reopen to move a row back.", stageLabel[from], label(to)))
	}
	return m.apply(ctx, tx, appID, from, to, store.CauseUser, reason)
}

// Reopen moves a row back to an earlier stage, or out of Rejected or Withdrawn.
func (m *Machine) Reopen(ctx context.Context, tx *sql.Tx, appID int64, to store.Stage) (Change, error) {
	from, err := currentStage(ctx, tx, appID)
	if err != nil {
		return Change{}, err
	}
	if !CanReopen(from, to) {
		return Change{From: from, To: from}, apperr.New(apperr.CodeConflict,
			fmt.Sprintf("Can't reopen from %s to %s.", stageLabel[from], label(to)))
	}
	return m.apply(ctx, tx, appID, from, to, store.CauseUser, "reopen")
}

// AutoAdvance applies rule if the row's current stage is one of its sources,
// and otherwise does nothing. It never returns an error for a stage the rule
// doesn't cover; only a missing row or a database failure is an error.
func (m *Machine) AutoAdvance(ctx context.Context, tx *sql.Tx, appID int64, rule AutoRule) (Change, error) {
	from, err := currentStage(ctx, tx, appID)
	if err != nil {
		return Change{}, err
	}
	for _, src := range rule.From {
		if from == src && from != rule.Target {
			return m.apply(ctx, tx, appID, from, rule.Target, store.CauseAuto, rule.Name)
		}
	}
	return Change{From: from, To: from}, nil
}

// NewLead is the data for a new application.
type NewLead struct {
	Company      string
	Role         string
	Location     string
	URL          string
	CanonicalURL string
	Description  string
	Source       store.Source
}

// CreateLead inserts a new application in Lead (U9) and logs its creation.
func (m *Machine) CreateLead(ctx context.Context, tx *sql.Tx, in NewLead) (int64, error) {
	if in.Source != store.SourceClip && in.Source != store.SourcePaste {
		return 0, apperr.New(apperr.CodeInvalid, "Unknown application source.")
	}
	now := store.FormatTime(m.now())
	res, err := tx.ExecContext(ctx, `INSERT INTO applications
		(company, role, location, url, canonical_url, description, source, stage, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'lead', ?, ?)`,
		in.Company, in.Role, in.Location, in.URL, in.CanonicalURL, in.Description, string(in.Source), now, now)
	if err != nil {
		return 0, storageErr(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, storageErr(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO stage_events (application_id, from_stage, to_stage, cause, reason, at)
		VALUES (?, NULL, 'lead', 'user', ?, ?)`, id, string(in.Source), now); err != nil {
		return 0, storageErr(err)
	}
	return id, nil
}

// apply writes the stage, keeps applied_at and the pinned resume version
// consistent with it (R10), and logs the event.
func (m *Machine) apply(ctx context.Context, tx *sql.Tx, appID int64, from, to store.Stage, cause store.Cause, reason string) (Change, error) {
	now := store.FormatTime(m.now())
	var err error
	switch {
	case !terminal(to) && order[to] >= order[store.StageApplied]:
		// Entering Applied or later: the first such move stamps applied_at
		// and pins the resume current at that moment. Later moves never
		// pin: a resume uploaded after applying wasn't the one sent (R10).
		// (SET expressions see the row's old values.)
		_, err = tx.ExecContext(ctx, `UPDATE applications SET stage = ?, updated_at = ?,
			resume_file_id = CASE WHEN applied_at IS NULL THEN (`+store.NewestResumeIDQuery+`) ELSE resume_file_id END,
			applied_at = coalesce(applied_at, ?)
			WHERE id = ?`, string(to), now, now, appID)
	case !terminal(to):
		// Lead or Ready (only reachable backwards via Reopen): not applied.
		_, err = tx.ExecContext(ctx, `UPDATE applications SET stage = ?, updated_at = ?,
			applied_at = NULL, resume_file_id = NULL WHERE id = ?`, string(to), now, appID)
	default:
		// Rejected or Withdrawn keep their history.
		_, err = tx.ExecContext(ctx, `UPDATE applications SET stage = ?, updated_at = ? WHERE id = ?`, string(to), now, appID)
	}
	if err != nil {
		return Change{}, storageErr(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO stage_events (application_id, from_stage, to_stage, cause, reason, at)
		VALUES (?, ?, ?, ?, ?, ?)`, appID, string(from), string(to), string(cause), reason, now); err != nil {
		return Change{}, storageErr(err)
	}
	return Change{From: from, To: to, Changed: true}, nil
}

// currentStage reads a live row's stage. Soft-deleted rows don't exist here.
func currentStage(ctx context.Context, tx *sql.Tx, appID int64) (store.Stage, error) {
	var s string
	err := tx.QueryRowContext(ctx, `SELECT stage FROM applications WHERE id = ? AND deleted_at IS NULL`, appID).Scan(&s)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", storageErr(err)
	}
	return store.Stage(s), nil
}

// ErrNotFound is returned for a missing or deleted application.
var ErrNotFound = apperr.New(apperr.CodeNotFound, "That application no longer exists.")

func storageErr(err error) error {
	return apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't save the change. Try again.")
}

func label(s store.Stage) string {
	if l, ok := stageLabel[s]; ok {
		return l
	}
	return "an unknown stage"
}
