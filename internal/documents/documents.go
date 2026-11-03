// Package documents writes approved cover letters as versioned PDFs (R22).
//
// Every approval is a new version in the application's own folder:
// documents/<appID>_<Company>/<Company>_<Role>_Cover_Letter_v<N>.pdf. A
// version is never overwritten, so a second letter for the same company
// can't destroy the first (V1 bug U3), and a v1 held open by a PDF viewer
// can't block writing v2.
package documents

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/pdf"
	"github.com/bot-bbio/legaj/internal/store"
)

// Folder holds all documents inside the data folder.
const Folder = "documents"

// Version is one written letter.
type Version struct {
	DocumentID int64  `json:"documentId"`
	AppID      int64  `json:"appId"`
	Version    int    `json:"version"`
	Path       string `json:"path"` // relative to the data folder, slash-separated
}

// ApprovedHook runs inside the transaction that records an approved
// letter. The pipeline uses it to apply the Ready rule (R26), so the
// approval and the stage change commit together.
type ApprovedHook func(ctx context.Context, tx *sql.Tx, appID int64) error

// Service writes and records documents.
type Service struct {
	db         *store.DB
	bus        *events.Bus
	dir        string
	now        func() time.Time
	onApproved ApprovedHook
}

// NewService creates the service. dir is the data folder.
func NewService(db *store.DB, bus *events.Bus, dir string, now func() time.Time) *Service {
	return &Service{db: db, bus: bus, dir: dir, now: now}
}

// OnApproved sets the hook run when a letter is approved. Call it while
// wiring the app, before any letter is written.
func (s *Service) OnApproved(h ApprovedHook) { s.onApproved = h }

var errAppGone = apperr.New(apperr.CodeNotFound, "That application no longer exists, so no letter was saved.")

var errDraftGone = apperr.New(apperr.CodeConflict, "This draft was already approved or has changed. Reopen the review to see the latest.")

// WriteLetter renders the letter to the application's next version and
// records it. In one transaction it checks the application is still live
// (a deleted row gets no file, R13), picks the next unused version, writes
// the PDF atomically, and inserts the document row; if any step fails,
// nothing is recorded. A letter that doesn't fit on one page fails before
// any file is written (R24).
func (s *Service) WriteLetter(ctx context.Context, appID int64, l pdf.Letter) (Version, error) {
	return s.write(ctx, appID, 0, 0, l)
}

// ApproveDraft is WriteLetter for a reviewed draft: in the same
// transaction it retires the draft row, so a draft becomes exactly one
// version even if Approve is clicked twice or Approve all races a single
// Approve. The row must still hold the same draft revision (draftRev, the
// "rev" in its body, or its row ID for drafts saved before revisions):
// a draft that's gone or was replaced, even under a reused row ID, is a
// conflict.
func (s *Service) ApproveDraft(ctx context.Context, appID, draftID, draftRev int64, l pdf.Letter) (Version, error) {
	if draftID <= 0 || draftRev <= 0 {
		return Version{}, errDraftGone
	}
	return s.write(ctx, appID, draftID, draftRev, l)
}

func (s *Service) write(ctx context.Context, appID, draftID, draftRev int64, l pdf.Letter) (Version, error) {
	body, err := json.Marshal(l)
	if err != nil {
		return Version{}, apperr.Wrap(err, apperr.CodeInternal, apperr.GenericMessage)
	}
	var out Version
	err = s.db.Tx(ctx, func(tx *sql.Tx) error {
		var company, role string
		err := tx.QueryRowContext(ctx, `SELECT company, role FROM applications WHERE id = ? AND deleted_at IS NULL`, appID).Scan(&company, &role)
		if errors.Is(err, sql.ErrNoRows) {
			return errAppGone
		}
		if err != nil {
			return storageErr(err)
		}
		if draftID > 0 {
			res, err := tx.ExecContext(ctx, `DELETE FROM documents
				WHERE id = ? AND application_id = ? AND kind = 'letter' AND status = 'draft'
				  AND coalesce(nullif(json_extract(body, '$.rev'), 0), id) = ?`, draftID, appID, draftRev)
			if err != nil {
				return storageErr(err)
			}
			if n, err := res.RowsAffected(); err != nil {
				return storageErr(err)
			} else if n == 0 {
				return errDraftGone
			}
		}
		var maxV int
		if err := tx.QueryRowContext(ctx, `SELECT coalesce(max(version), 0) FROM documents
			WHERE application_id = ? AND kind = 'letter'`, appID).Scan(&maxV); err != nil {
			return storageErr(err)
		}
		folder := path.Join(Folder, appFolder(appID, company))
		v := maxV + 1
		rel := path.Join(folder, letterFile(appID, company, role, v))
		// Never overwrite: skip any version whose file already exists
		// (e.g. left by a commit that failed after the write).
		for exists(s.abs(rel)) {
			v++
			rel = path.Join(folder, letterFile(appID, company, role, v))
		}
		if _, err := pdf.WriteLetter(l, s.abs(rel)); err != nil {
			return err
		}
		now := store.FormatTime(s.now())
		res, err := tx.ExecContext(ctx, `INSERT INTO documents
			(application_id, kind, status, version, body, file_path, created_at, updated_at, approved_at)
			VALUES (?, 'letter', 'approved', ?, ?, ?, ?, ?, ?)`, appID, v, string(body), rel, now, now, now)
		if err != nil {
			_ = os.Remove(s.abs(rel)) // the row didn't land; don't leave an orphan
			return storageErr(err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return storageErr(err)
		}
		if s.onApproved != nil {
			if err := s.onApproved(ctx, tx, appID); err != nil {
				_ = os.Remove(s.abs(rel))
				return err
			}
		}
		out = Version{DocumentID: id, AppID: appID, Version: v, Path: rel}
		return nil
	})
	if err != nil {
		return Version{}, apperr.Public(err)
	}
	// The approval may also have moved the row to Ready (R26).
	s.bus.Emit(events.Change(events.EntityDocument, appID))
	s.bus.Emit(events.Change(events.EntityApplication, appID))
	return out, nil
}

// abs turns a stored relative path into an absolute one.
func (s *Service) abs(rel string) string {
	return filepath.Join(s.dir, filepath.FromSlash(rel))
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func storageErr(err error) error {
	return apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't save the letter. Try again.")
}

// RemoveFiles deletes the letter folders of applications that were purged
// for good (R13, PRIVACY.md). It runs after the purge commits, so it's best
// effort: errors are returned for the log, and nothing is retried.
func (s *Service) RemoveFiles(appIDs []int64) error {
	root := filepath.Join(s.dir, Folder)
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var errs []error
	for _, id := range appIDs {
		prefix := fmt.Sprintf("%d_", id)
		for _, e := range entries {
			if e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
				errs = append(errs, os.RemoveAll(filepath.Join(root, e.Name())))
			}
		}
	}
	return errors.Join(errs...)
}

// Latest returns each live application's newest approved letter version.
func (s *Service) Latest(ctx context.Context) (map[int64]Version, error) {
	rows, err := s.db.Reader().QueryContext(ctx, `SELECT d.id, d.application_id, d.version, d.file_path
		FROM documents d JOIN applications a ON a.id = d.application_id
		WHERE d.kind = 'letter' AND d.status = 'approved' AND a.deleted_at IS NULL
		  AND d.version = (SELECT max(version) FROM documents x
		                   WHERE x.application_id = d.application_id AND x.kind = 'letter' AND x.status = 'approved')`)
	if err != nil {
		return nil, readErr(err)
	}
	defer rows.Close()
	out := map[int64]Version{}
	for rows.Next() {
		var v Version
		var path sql.NullString
		if err := rows.Scan(&v.DocumentID, &v.AppID, &v.Version, &path); err != nil {
			return nil, readErr(err)
		}
		v.Path = path.String
		out[v.AppID] = v
	}
	if err := rows.Err(); err != nil {
		return nil, readErr(err)
	}
	return out, nil
}

var errNoLetter = apperr.New(apperr.CodeNotFound, "This application has no approved letter yet.")

// LatestFor returns an application's newest approved letter as an
// absolute path, for opening it or its folder.
func (s *Service) LatestFor(ctx context.Context, appID int64) (string, error) {
	all, err := s.Latest(ctx)
	if err != nil {
		return "", err
	}
	v, ok := all[appID]
	if !ok || v.Path == "" {
		return "", errNoLetter
	}
	return s.abs(v.Path), nil
}

func readErr(err error) error {
	return apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't read your letters.")
}
