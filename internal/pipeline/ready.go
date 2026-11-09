package pipeline

import (
	"context"
	"database/sql"
	"errors"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/store"
)

// CheckReady applies R26 inside the caller's transaction: a Lead whose
// chosen materials are all approved moves to Ready through the state
// machine (cause auto, reason materials_approved). Rows in any other stage,
// and rows with nothing chosen, never move. Callers run it whenever
// materials are approved or the choice changes.
//
// "Approved" means: the letter has at least one approved version; the
// answers have at least one approved answer and none still in draft.
func (m *Machine) CheckReady(ctx context.Context, tx *sql.Tx, appID int64) (Change, error) {
	var stage, materials string
	err := tx.QueryRowContext(ctx, `SELECT stage, materials FROM applications
		WHERE id = ? AND deleted_at IS NULL`, appID).Scan(&stage, &materials)
	if errors.Is(err, sql.ErrNoRows) {
		return Change{}, ErrNotFound
	}
	if err != nil {
		return Change{}, storageErr(err)
	}
	from := store.Stage(stage)
	mat := store.Materials(materials)
	if from != store.StageLead || !mat.Valid() {
		return Change{From: from, To: from}, nil
	}
	done, err := materialsApproved(ctx, tx, appID, mat)
	if err != nil {
		return Change{}, err
	}
	if !done {
		return Change{From: from, To: from}, nil
	}
	return m.AutoAdvance(ctx, tx, appID, RuleMaterialsApproved)
}

func materialsApproved(ctx context.Context, tx *sql.Tx, appID int64, mat store.Materials) (bool, error) {
	if mat.NeedsLetter() {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM documents
			WHERE application_id = ? AND kind = 'letter' AND status = 'approved'`, appID).Scan(&n); err != nil {
			return false, storageErr(err)
		}
		if n == 0 {
			return false, nil
		}
	}
	if mat.NeedsAnswers() {
		var approved, drafts int
		if err := tx.QueryRowContext(ctx, `SELECT
			coalesce(sum(status = 'approved'), 0), coalesce(sum(status = 'draft'), 0)
			FROM answers WHERE application_id = ?`, appID).Scan(&approved, &drafts); err != nil {
			return false, storageErr(err)
		}
		if approved == 0 || drafts > 0 {
			return false, nil
		}
	}
	return true, nil
}

// SetMaterials changes one application's materials choice and re-checks
// the Ready rule, since the new choice may already be satisfied.
func (s *Service) SetMaterials(ctx context.Context, id int64, mat store.Materials) (store.Application, error) {
	if !mat.Valid() {
		return store.Application{}, apperr.New(apperr.CodeInvalid, "Choose a letter, answers, or both.")
	}
	err := events.Mutate(ctx, s.cfg.DB, s.cfg.Bus, s.changed(id), func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE applications SET materials = ?, updated_at = ?
			WHERE id = ? AND deleted_at IS NULL`, string(mat), store.FormatTime(s.cfg.Now()), id)
		if err != nil {
			return storageErr(err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return storageErr(err)
		} else if n == 0 {
			return ErrNotFound
		}
		_, err = s.cfg.Machine.CheckReady(ctx, tx, id)
		return err
	})
	if err != nil {
		return store.Application{}, apperr.Public(err)
	}
	return s.Get(ctx, id)
}

// defaultMaterials is the choice a new row starts with.
func (s *Service) defaultMaterials(ctx context.Context) store.Materials {
	if s.cfg.DefaultMaterials == nil {
		return store.MaterialsLetter
	}
	if m := s.cfg.DefaultMaterials(ctx); m.Valid() {
		return m
	}
	return store.MaterialsLetter
}
