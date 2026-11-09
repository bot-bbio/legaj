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
	if strings.TrimSpace(in.Company) == "" && strings.TrimSpace(in.Role) == "" {
		return PasteResult{}, apperr.New(apperr.CodeInvalid, "Enter at least the company or the role.")
	}
	lead, err := leadFrom(in.URL, in.Company, in.Role, in.Location, in.Description, store.SourcePaste)
	if err != nil {
		return PasteResult{}, err
	}
	return s.addLead(ctx, lead)
}

// leadFrom trims a pasted or clipped job, checks its field limits and
// canonicalizes its URL. Each caller checks its own required fields first.
func leadFrom(url, company, role, location, description string, source store.Source) (NewLead, error) {
	l := NewLead{
		URL: strings.TrimSpace(url), Company: strings.TrimSpace(company), Role: strings.TrimSpace(role),
		Location: strings.TrimSpace(location), Description: strings.TrimSpace(description), Source: source,
	}
	if err := checkDetails(l.Company, l.Role, l.Location); err != nil {
		return l, err
	}
	if len(l.Description) > MaxDescription {
		return l, apperr.New(apperr.CodeInvalid, "That job description is too long (over 100,000 characters).")
	}
	limit := maxURL
	if source == store.SourceClip {
		limit = maxClipURL
	}
	var err error
	l.CanonicalURL, err = canonicalURL(l.URL, limit)
	return l, err
}

// checkDetails enforces the company, role and location limit shared by
// every way a row is created or filled in.
func checkDetails(company, role, location string) error {
	if utf8.RuneCountInString(company) > maxDetail || utf8.RuneCountInString(role) > maxDetail || utf8.RuneCountInString(location) > maxDetail {
		return apperr.New(apperr.CodeInvalid, "Company, role and location must be under 200 characters.")
	}
	return nil
}

// ClipInput is a job the clipper extracted from a web page (R19). Only the
// URL is required: an empty company is the "Needs company" case.
type ClipInput struct {
	URL         string
	Company     string
	Role        string
	Location    string
	Description string
}

// AddClip adds a clipped job as a Lead, or reports the live row that
// already has the same canonical URL (R19: one row per posting).
func (s *Service) AddClip(ctx context.Context, in ClipInput) (PasteResult, error) {
	if strings.TrimSpace(in.URL) == "" {
		return PasteResult{}, apperr.New(apperr.CodeInvalid, "The clip has no page address.")
	}
	lead, err := leadFrom(in.URL, in.Company, in.Role, in.Location, in.Description, store.SourceClip)
	if err != nil {
		return PasteResult{}, err
	}
	return s.addLead(ctx, lead)
}

// addLead creates lead with the Settings default materials (D28), unless a
// live row already has its canonical URL. The check and the insert run in
// one write transaction, so two simultaneous adds can't both create a row.
func (s *Service) addLead(ctx context.Context, lead NewLead) (PasteResult, error) {
	lead.Materials = s.defaultMaterials(ctx) // read before the write transaction
	var res PasteResult
	err := s.cfg.DB.Tx(ctx, func(tx *sql.Tx) error {
		if lead.CanonicalURL != "" {
			var existing int64
			err := tx.QueryRowContext(ctx, `SELECT id FROM applications WHERE canonical_url = ? AND deleted_at IS NULL`, lead.CanonicalURL).Scan(&existing)
			if err == nil {
				res = PasteResult{DuplicateOf: existing, ID: existing}
				return nil
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return storageErr(err)
			}
		}
		id, err := s.cfg.Machine.CreateLead(ctx, tx, lead)
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

// Found is what clip enrichment worked out about a row (R19).
type Found struct {
	Company  string
	Role     string
	Location string
}

// FillMissing writes each non-empty value of f where the live row's own
// field is still empty, in one statement, so a value the user typed in the
// meantime always wins. It reports whether the row changed.
func (s *Service) FillMissing(ctx context.Context, id int64, f Found) (bool, error) {
	f.Company, f.Role, f.Location = strings.TrimSpace(f.Company), strings.TrimSpace(f.Role), strings.TrimSpace(f.Location)
	if err := checkDetails(f.Company, f.Role, f.Location); err != nil {
		return false, err
	}
	if f == (Found{}) {
		return false, nil
	}
	var changed bool
	err := s.cfg.DB.Tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE applications SET
				company  = CASE WHEN company  = '' THEN ? ELSE company  END,
				role     = CASE WHEN role     = '' THEN ? ELSE role     END,
				location = CASE WHEN location = '' THEN ? ELSE location END,
				updated_at = ?
			WHERE id = ? AND deleted_at IS NULL
				AND ((company = '' AND ? <> '') OR (role = '' AND ? <> '') OR (location = '' AND ? <> ''))`,
			f.Company, f.Role, f.Location, store.FormatTime(s.cfg.Now()), id, f.Company, f.Role, f.Location)
		if err != nil {
			return storageErr(err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return storageErr(err)
		}
		changed = n > 0
		return nil
	})
	if err != nil {
		return false, apperr.Public(err)
	}
	if changed {
		s.cfg.Bus.Emit(s.changed(id))
	}
	return changed, nil
}
