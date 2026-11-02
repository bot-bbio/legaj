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
	"os"
	"path"
	"path/filepath"
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

// Service writes and records documents.
type Service struct {
	db  *store.DB
	bus *events.Bus
	dir string
	now func() time.Time
}

// NewService creates the service. dir is the data folder.
func NewService(db *store.DB, bus *events.Bus, dir string, now func() time.Time) *Service {
	return &Service{db: db, bus: bus, dir: dir, now: now}
}

var errAppGone = apperr.New(apperr.CodeNotFound, "That application no longer exists, so no letter was saved.")

// WriteLetter renders the letter to the application's next version and
// records it. In one transaction it checks the application is still live
// (a deleted row gets no file, R13), picks the next unused version, writes
// the PDF atomically, and inserts the document row; if any step fails,
// nothing is recorded. A letter that doesn't fit on one page fails before
// any file is written (R24).
func (s *Service) WriteLetter(ctx context.Context, appID int64, l pdf.Letter) (Version, error) {
	body, err := json.Marshal(l)
	if err != nil {
		return Version{}, apperr.Wrap(err, apperr.CodeInternal, apperr.GenericMessage)
	}
	var out Version
	err = events.Mutate(ctx, s.db, s.bus, events.Change(events.EntityDocument, appID), func(tx *sql.Tx) error {
		var company, role string
		err := tx.QueryRowContext(ctx, `SELECT company, role FROM applications WHERE id = ? AND deleted_at IS NULL`, appID).Scan(&company, &role)
		if errors.Is(err, sql.ErrNoRows) {
			return errAppGone
		}
		if err != nil {
			return storageErr(err)
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
		out = Version{DocumentID: id, AppID: appID, Version: v, Path: rel}
		return nil
	})
	if err != nil {
		return Version{}, apperr.Public(err)
	}
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
