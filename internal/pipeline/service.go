package pipeline

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/store"
)

// UndoWindow is how long a delete can be undone before it's purged (R13).
const UndoWindow = 10 * time.Second

// Timer is the part of *time.Timer the service uses.
type Timer interface{ Stop() bool }

// Config wires the service.
type Config struct {
	DB      *store.DB
	Bus     *events.Bus
	Machine *Machine
	Now     func() time.Time
	// AfterFunc schedules the purge; default time.AfterFunc. Tests fire it by hand.
	AfterFunc func(d time.Duration, f func()) Timer
	// DefaultMaterials is the Settings default for new rows (R26); nil
	// means a letter.
	DefaultMaterials func(ctx context.Context) store.Materials
}

// Service manages applications by stable ID (R12, R13).
type Service struct {
	cfg Config

	mu       sync.Mutex
	onDelete []func(ids []int64)
}

// NewService creates the service.
func NewService(cfg Config) *Service {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.AfterFunc == nil {
		cfg.AfterFunc = func(d time.Duration, f func()) Timer { return time.AfterFunc(d, f) }
	}
	return &Service{cfg: cfg}
}

// OnDelete registers a hook called with the IDs of every delete, after it
// commits. The job queue uses it to cancel work for deleted rows (R13).
func (s *Service) OnDelete(hook func(ids []int64)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onDelete = append(s.onDelete, hook)
}

func (s *Service) changed(ids ...int64) events.Changed {
	return events.Change(events.EntityApplication, ids...)
}

// Create adds a new Lead. Without a materials choice it takes the default.
func (s *Service) Create(ctx context.Context, in NewLead) (int64, error) {
	if in.Materials == store.MaterialsNone {
		in.Materials = s.defaultMaterials(ctx)
	}
	var id int64
	err := s.cfg.DB.Tx(ctx, func(tx *sql.Tx) error {
		var err error
		id, err = s.cfg.Machine.CreateLead(ctx, tx, in)
		return err
	})
	if err != nil {
		return 0, apperr.Public(err)
	}
	s.cfg.Bus.Emit(s.changed(id))
	return id, nil
}

const appColumns = `id, company, role, location, url, canonical_url, description, source, stage,
	materials, applied_at, resume_file_id, notes, created_at, updated_at`

type rowScanner interface{ Scan(dest ...any) error }

func scanApplication(r rowScanner) (store.Application, error) {
	var a store.Application
	var source, stage, materials, created, updated string
	var applied sql.NullString
	var resume sql.NullInt64
	if err := r.Scan(&a.ID, &a.Company, &a.Role, &a.Location, &a.URL, &a.CanonicalURL, &a.Description,
		&source, &stage, &materials, &applied, &resume, &a.Notes, &created, &updated); err != nil {
		return a, err
	}
	a.Source, a.Stage, a.Materials = store.Source(source), store.Stage(stage), store.Materials(materials)
	var err error
	if a.CreatedAt, err = store.ParseTime(created); err != nil {
		return a, err
	}
	if a.UpdatedAt, err = store.ParseTime(updated); err != nil {
		return a, err
	}
	if applied.Valid {
		t, err := store.ParseTime(applied.String)
		if err != nil {
			return a, err
		}
		a.AppliedAt = &t
	}
	if resume.Valid {
		a.ResumeFileID = &resume.Int64
	}
	return a, nil
}

func readErr(err error) error {
	return apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't read your applications.")
}

// List returns every live application, newest first.
func (s *Service) List(ctx context.Context) ([]store.Application, error) {
	rows, err := s.cfg.DB.Reader().QueryContext(ctx, `SELECT `+appColumns+` FROM applications
		WHERE deleted_at IS NULL ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, readErr(err)
	}
	defer rows.Close()
	out := []store.Application{}
	for rows.Next() {
		a, err := scanApplication(rows)
		if err != nil {
			return nil, readErr(err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, readErr(err)
	}
	return out, nil
}

// Get returns one live application.
func (s *Service) Get(ctx context.Context, id int64) (store.Application, error) {
	a, err := scanApplication(s.cfg.DB.Reader().QueryRowContext(ctx, `SELECT `+appColumns+` FROM applications
		WHERE id = ? AND deleted_at IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	if err != nil {
		return a, readErr(err)
	}
	return a, nil
}

// Details are the user-editable descriptive fields. The job description
// isn't among them: nothing in the app clears it (R16, A9).
type Details struct {
	Company  string `json:"company"`
	Role     string `json:"role"`
	Location string `json:"location"`
	Notes    string `json:"notes"`
}

const maxDetail = 200
const maxNotes = 10_000

// UpdateDetails edits company, role, location and notes.
func (s *Service) UpdateDetails(ctx context.Context, id int64, d Details) (store.Application, error) {
	d.Company, d.Role, d.Location, d.Notes = strings.TrimSpace(d.Company), strings.TrimSpace(d.Role), strings.TrimSpace(d.Location), strings.TrimSpace(d.Notes)
	if len([]rune(d.Company)) > maxDetail || len([]rune(d.Role)) > maxDetail || len([]rune(d.Location)) > maxDetail || len([]rune(d.Notes)) > maxNotes {
		return store.Application{}, apperr.New(apperr.CodeInvalid, "One of the fields is too long.")
	}
	err := events.Mutate(ctx, s.cfg.DB, s.cfg.Bus, s.changed(id), func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE applications SET company = ?, role = ?, location = ?, notes = ?, updated_at = ?
			WHERE id = ? AND deleted_at IS NULL`, d.Company, d.Role, d.Location, d.Notes, store.FormatTime(s.cfg.Now()), id)
		if err != nil {
			return storageErr(err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return store.Application{}, err
	}
	return s.Get(ctx, id)
}

// uniqueSorted returns ids deduplicated in ascending order: every bulk
// action processes rows in this stable order (A19).
func uniqueSorted(ids []int64) []int64 {
	out := slices.Clone(ids)
	slices.Sort(out)
	return slices.Compact(out)
}

// Skip is a row a bulk action didn't change, and why.
type Skip struct {
	ID     int64  `json:"id"`
	Reason string `json:"reason"`
}

// StageResult reports a bulk stage change.
type StageResult struct {
	Changed []int64 `json:"changed"`
	Skipped []Skip  `json:"skipped"`
}

// SetStage moves exactly the given rows to a stage, in one transaction and
// in ascending ID order. Rows the state machine doesn't allow are skipped
// with the reason; rows already there are left alone (R11, R12).
func (s *Service) SetStage(ctx context.Context, ids []int64, to store.Stage) (StageResult, error) {
	if !to.Valid() {
		return StageResult{}, apperr.New(apperr.CodeInvalid, "Unknown stage.")
	}
	res := StageResult{Changed: []int64{}, Skipped: []Skip{}}
	err := s.cfg.DB.Tx(ctx, func(tx *sql.Tx) error {
		for _, id := range uniqueSorted(ids) {
			ch, err := s.cfg.Machine.Transition(ctx, tx, id, to, "user")
			switch code := apperr.CodeOf(err); {
			case err == nil:
				if ch.Changed {
					res.Changed = append(res.Changed, id)
				}
			case code == apperr.CodeConflict || code == apperr.CodeNotFound:
				res.Skipped = append(res.Skipped, Skip{ID: id, Reason: apperr.UserMessage(err)})
			default:
				return err
			}
		}
		return nil
	})
	if err != nil {
		return StageResult{}, apperr.Public(err)
	}
	if len(res.Changed) > 0 {
		s.cfg.Bus.Emit(s.changed(res.Changed...))
	}
	return res, nil
}

// Reopen moves exactly the given rows back to an earlier stage, or out of
// Rejected or Withdrawn (R11, D23). Rows Reopen doesn't apply to are skipped
// with the reason.
func (s *Service) Reopen(ctx context.Context, ids []int64, to store.Stage) (StageResult, error) {
	if !to.Valid() {
		return StageResult{}, apperr.New(apperr.CodeInvalid, "Unknown stage.")
	}
	res := StageResult{Changed: []int64{}, Skipped: []Skip{}}
	err := s.cfg.DB.Tx(ctx, func(tx *sql.Tx) error {
		for _, id := range uniqueSorted(ids) {
			_, err := s.cfg.Machine.Reopen(ctx, tx, id, to)
			switch code := apperr.CodeOf(err); {
			case err == nil:
				res.Changed = append(res.Changed, id)
			case code == apperr.CodeConflict || code == apperr.CodeNotFound:
				res.Skipped = append(res.Skipped, Skip{ID: id, Reason: apperr.UserMessage(err)})
			default:
				return err
			}
		}
		return nil
	})
	if err != nil {
		return StageResult{}, apperr.Public(err)
	}
	if len(res.Changed) > 0 {
		s.cfg.Bus.Emit(s.changed(res.Changed...))
	}
	return res, nil
}

// DeleteResult identifies a delete for Undo by its UndoID.
type DeleteResult struct {
	UndoID string  `json:"undoId"`
	IDs    []int64 `json:"ids"`
}

func newToken() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Delete soft-deletes exactly the given live rows as one batch and
// schedules its purge after UndoWindow. Missing or already-deleted IDs are
// ignored. Children stay untouched until the purge (R13, U4).
func (s *Service) Delete(ctx context.Context, ids []int64) (DeleteResult, error) {
	res := DeleteResult{UndoID: newToken(), IDs: []int64{}}
	err := s.cfg.DB.Tx(ctx, func(tx *sql.Tx) error {
		now := store.FormatTime(s.cfg.Now())
		for _, id := range uniqueSorted(ids) {
			r, err := tx.ExecContext(ctx, `UPDATE applications SET deleted_at = ?, delete_token = ?
				WHERE id = ? AND deleted_at IS NULL`, now, res.UndoID, id)
			if err != nil {
				return storageErr(err)
			}
			if n, _ := r.RowsAffected(); n == 1 {
				res.IDs = append(res.IDs, id)
			}
		}
		return nil
	})
	if err != nil {
		return DeleteResult{}, err
	}
	if len(res.IDs) == 0 {
		return res, nil
	}
	token := res.UndoID
	s.cfg.AfterFunc(UndoWindow, func() {
		// Background purge: its own context, errors retried at next start.
		_, _ = s.purge(context.Background(), `delete_token = ?`, token)
	})
	s.mu.Lock()
	hooks := slices.Clone(s.onDelete)
	s.mu.Unlock()
	for _, h := range hooks {
		h(slices.Clone(res.IDs))
	}
	s.cfg.Bus.Emit(s.changed(res.IDs...))
	return res, nil
}

// UndoResult reports an Undo. Skipped rows couldn't come back because a
// live row with the same job URL was added meanwhile; their letters,
// rounds, answers, follow-ups and history moved to that row, listed in
// MergedInto at the same index, so nothing is lost when they're purged.
type UndoResult struct {
	IDs        []int64 `json:"ids"`
	Skipped    []int64 `json:"skipped"`
	MergedInto []int64 `json:"mergedInto"`
}

// ErrUndoExpired is returned once a delete has been purged.
var ErrUndoExpired = apperr.New(apperr.CodeConflict, "It's too late to undo that delete.")

// Undo restores a delete batch with all its children.
func (s *Service) Undo(ctx context.Context, token string) (UndoResult, error) {
	res := UndoResult{IDs: []int64{}, Skipped: []int64{}, MergedInto: []int64{}}
	err := s.cfg.DB.Tx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT id, canonical_url FROM applications
			WHERE delete_token = ? AND deleted_at IS NOT NULL ORDER BY id`, token)
		if err != nil {
			return storageErr(err)
		}
		type pending struct {
			id  int64
			url string
		}
		var batch []pending
		for rows.Next() {
			var p pending
			if err := rows.Scan(&p.id, &p.url); err != nil {
				rows.Close()
				return storageErr(err)
			}
			batch = append(batch, p)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return storageErr(err)
		}
		if len(batch) == 0 {
			return ErrUndoExpired
		}
		for _, p := range batch {
			if p.url != "" {
				var live int64
				err := tx.QueryRowContext(ctx, `SELECT id FROM applications
					WHERE canonical_url = ? AND deleted_at IS NULL`, p.url).Scan(&live)
				if err != nil && !errors.Is(err, sql.ErrNoRows) {
					return storageErr(err)
				}
				if err == nil {
					if err := s.moveChildren(ctx, tx, p.id, live); err != nil {
						return err
					}
					res.Skipped = append(res.Skipped, p.id)
					res.MergedInto = append(res.MergedInto, live)
					continue
				}
			}
			if _, err := tx.ExecContext(ctx, `UPDATE applications SET deleted_at = NULL, delete_token = NULL
				WHERE id = ?`, p.id); err != nil {
				return storageErr(err)
			}
			res.IDs = append(res.IDs, p.id)
		}
		return nil
	})
	if err != nil {
		return UndoResult{}, err
	}
	if touched := append(slices.Clone(res.IDs), res.MergedInto...); len(touched) > 0 {
		s.cfg.Bus.Emit(s.changed(touched...))
	}
	return res, nil
}

// moveChildren re-parents the work a soft-deleted row owns (letters,
// interview rounds, answers, follow-ups) onto the live row that replaced
// it, so the coming purge removes only the empty shell. Approved letter
// versions are renumbered after the live row's own. Stage history stays
// with the old row: copied over, it would contradict the live row's own.
// Moved rounds advance the live row exactly as adding a round does (R30).
func (s *Service) moveChildren(ctx context.Context, tx *sql.Tx, from, to int64) error {
	rows, err := tx.QueryContext(ctx, `SELECT id, kind FROM documents
		WHERE application_id = ? AND version IS NOT NULL ORDER BY kind, version`, from)
	if err != nil {
		return storageErr(err)
	}
	type doc struct {
		id   int64
		kind string
	}
	var versioned []doc
	for rows.Next() {
		var d doc
		if err := rows.Scan(&d.id, &d.kind); err != nil {
			rows.Close()
			return storageErr(err)
		}
		versioned = append(versioned, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return storageErr(err)
	}
	for _, d := range versioned {
		if _, err := tx.ExecContext(ctx, `UPDATE documents SET application_id = ?,
			version = 1 + (SELECT coalesce(max(version), 0) FROM documents WHERE application_id = ? AND kind = ?)
			WHERE id = ?`, to, to, d.kind, d.id); err != nil {
			return storageErr(err)
		}
	}
	movedRounds := int64(0)
	for _, table := range []string{"documents", "rounds", "followups", "answers"} {
		r, err := tx.ExecContext(ctx, `UPDATE `+table+` SET application_id = ? WHERE application_id = ?`, to, from)
		if err != nil {
			return storageErr(err)
		}
		if table == "rounds" {
			movedRounds, _ = r.RowsAffected()
		}
	}
	if movedRounds > 0 {
		if _, err := s.cfg.Machine.AutoAdvance(ctx, tx, to, RuleRoundAdded); err != nil {
			return err
		}
	}
	return nil
}

// PurgeDeleted removes every soft-deleted row, for use at startup: single
// instance (R42) guarantees no undo window outlives the process.
func (s *Service) PurgeDeleted(ctx context.Context) (int64, error) {
	return s.purge(ctx, `1 = 1`)
}

// purge hard-deletes soft-deleted rows matching where; children cascade.
func (s *Service) purge(ctx context.Context, where string, args ...any) (int64, error) {
	var n int64
	err := s.cfg.DB.Tx(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM applications WHERE deleted_at IS NOT NULL AND %s`, where), args...)
		if err != nil {
			return err
		}
		n, err = r.RowsAffected()
		return err
	})
	if err != nil {
		return 0, storageErr(err)
	}
	return n, nil
}
