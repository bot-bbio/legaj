package pipeline

import (
	"context"
	"database/sql"
	"errors"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/store"
)

// ResumeRef points at a stored resume file.
type ResumeRef struct {
	ID         int64  `json:"id"`
	FileName   string `json:"fileName"`
	StoredPath string `json:"storedPath"` // relative to the data folder
}

var (
	errNoResume = apperr.New(apperr.CodeNotFound, "No resume has been uploaded yet.")
	// An application marked Applied before any resume existed has no
	// record of what was sent; showing today's resume would be a guess.
	errAppliedWithoutResume = apperr.New(apperr.CodeNotFound, "This application was marked Applied before you uploaded a resume, so LeGaJ doesn't know which version you sent.")
)

// ResumeFor returns the resume an application should open (R10): the
// exact version pinned when it reached Applied, even if the user has
// uploaded a newer resume since; before applying, the current one.
func (s *Service) ResumeFor(ctx context.Context, appID int64) (ResumeRef, error) {
	r := s.cfg.DB.Reader()
	var pinned sql.NullInt64
	var appliedAt sql.NullString
	err := r.QueryRowContext(ctx, `SELECT resume_file_id, applied_at FROM applications WHERE id = ? AND deleted_at IS NULL`, appID).Scan(&pinned, &appliedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ResumeRef{}, ErrNotFound
	}
	if err != nil {
		return ResumeRef{}, readErr(err)
	}
	var id int64
	switch {
	case pinned.Valid:
		id = pinned.Int64
	case appliedAt.Valid:
		return ResumeRef{}, errAppliedWithoutResume
	default:
		err := r.QueryRowContext(ctx, store.NewestResumeIDQuery).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return ResumeRef{}, errNoResume
		}
		if err != nil {
			return ResumeRef{}, readErr(err)
		}
	}
	var ref ResumeRef
	err = r.QueryRowContext(ctx, `SELECT id, file_name, stored_path FROM resume_files WHERE id = ?`, id).Scan(&ref.ID, &ref.FileName, &ref.StoredPath)
	if errors.Is(err, sql.ErrNoRows) {
		return ResumeRef{}, errNoResume
	}
	if err != nil {
		return ResumeRef{}, readErr(err)
	}
	return ref, nil
}
