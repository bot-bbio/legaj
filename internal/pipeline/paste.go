package pipeline

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/store"
)

// MaxDescription bounds a job description (pasted or clipped).
const MaxDescription = 100_000

// PasteInput is a job the user entered by hand (R15).
type PasteInput struct {
	URL         string `json:"url"`
	Company     string `json:"company"`
	Role        string `json:"role"`
	Location    string `json:"location"`
	Description string `json:"description"`
}

// PasteResult says whether a Lead was created, or which existing row
// already holds the same posting.
type PasteResult struct {
	Created     bool  `json:"created"`
	ID          int64 `json:"id"`
	DuplicateOf int64 `json:"duplicateOf"` // 0 unless the URL was already in the pipeline
}

// PasteJob adds a Lead from manual entry, or reports the live row that
// already has the same canonical URL. The check and the insert run in one
// write transaction, so two simultaneous pastes can't both create a row.
func (s *Service) PasteJob(ctx context.Context, in PasteInput) (PasteResult, error) {
	in.Company, in.Role, in.Location = strings.TrimSpace(in.Company), strings.TrimSpace(in.Role), strings.TrimSpace(in.Location)
	in.URL, in.Description = strings.TrimSpace(in.URL), strings.TrimSpace(in.Description)
	switch {
	case in.Company == "" && in.Role == "":
		return PasteResult{}, apperr.New(apperr.CodeInvalid, "Enter at least the company or the role.")
	case utf8.RuneCountInString(in.Company) > maxDetail || utf8.RuneCountInString(in.Role) > maxDetail || utf8.RuneCountInString(in.Location) > maxDetail:
		return PasteResult{}, apperr.New(apperr.CodeInvalid, "Company, role and location must be under 200 characters.")
	case len(in.Description) > MaxDescription:
		return PasteResult{}, apperr.New(apperr.CodeInvalid, "That job description is too long (over 100,000 characters).")
	}
	canonical, err := CanonicalURL(in.URL)
	if err != nil {
		return PasteResult{}, err
	}

	materials := s.defaultMaterials(ctx) // read before the write transaction
	var res PasteResult
	err = s.cfg.DB.Tx(ctx, func(tx *sql.Tx) error {
		if canonical != "" {
			var existing int64
			err := tx.QueryRowContext(ctx, `SELECT id FROM applications WHERE canonical_url = ? AND deleted_at IS NULL`, canonical).Scan(&existing)
			if err == nil {
				res = PasteResult{DuplicateOf: existing, ID: existing}
				return nil
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return storageErr(err)
			}
		}
		id, err := s.cfg.Machine.CreateLead(ctx, tx, NewLead{
			Company: in.Company, Role: in.Role, Location: in.Location,
			URL: in.URL, CanonicalURL: canonical, Description: in.Description, Source: store.SourcePaste,
			Materials: materials,
		})
		if err != nil {
			return err
		}
		res = PasteResult{Created: true, ID: id}
		return nil
	})
	if err != nil {
		return PasteResult{}, apperr.Public(err)
	}
	if res.Created {
		s.cfg.Bus.Emit(s.changed(res.ID))
	}
	return res, nil
}
