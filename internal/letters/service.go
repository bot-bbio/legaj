// Package letters drafts cover letters grounded in the job description and
// the user's profile (R21, D8), and turns approved drafts into PDF letters.
package letters

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"time"

	"github.com/bot-bbio/legaj/internal/ai"
	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/documents"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/pdf"
	"github.com/bot-bbio/legaj/internal/profile"
	"github.com/bot-bbio/legaj/internal/store"
)

// Draft is a letter awaiting review: the user picks one of the three why
// paragraphs, edits, and approves (R22).
type Draft struct {
	// Rev identifies the generation this draft came from: a random number
	// made when the draft is generated and kept through edits. A save must
	// name the current revision, so an edit can never bring back a draft
	// that was approved or replaced meanwhile. (Row IDs can't do this:
	// SQLite reuses a deleted draft's ID for the next one.)
	Rev        int64    `json:"rev"`
	Style      string   `json:"style"`
	Salutation string   `json:"salutation"`
	Opening    string   `json:"opening"`
	WhyOptions []string `json:"whyOptions"`
	ChosenWhy  int      `json:"chosenWhy"`
	Body       []string `json:"body"`
	Closing    string   `json:"closing"`
	SignOff    string   `json:"signOff"`
}

// Config wires the service.
type Config struct {
	DB      *store.DB
	Bus     *events.Bus
	AI      ai.Provider
	Profile *profile.Service
	Model   func(ctx context.Context) string
	Now     func() time.Time
	// Documents writes approved letters as versioned PDFs (R22).
	Documents *documents.Service
	// SkipReview reports the Skip-review setting: Generate then makes the
	// PDF without waiting for review (R23). nil means review is on.
	SkipReview func(ctx context.Context) bool
}

// Service drafts and stores letters.
type Service struct{ cfg Config }

// NewService creates the service.
func NewService(cfg Config) *Service { return &Service{cfg: cfg} }

// errNotFound is a missing or deleted application.
var errNotFound = apperr.New(apperr.CodeNotFound, "That application no longer exists.")

func (s *Service) loadApp(ctx context.Context, appID int64) (store.Application, error) {
	var a store.Application
	err := s.cfg.DB.Reader().QueryRowContext(ctx, `SELECT id, company, role, location, description FROM applications
		WHERE id = ? AND deleted_at IS NULL`, appID).Scan(&a.ID, &a.Company, &a.Role, &a.Location, &a.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return a, errNotFound
	}
	if err != nil {
		return a, apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't read that application.")
	}
	return a, nil
}

// Draft writes a new draft for an application and saves it as the
// application's current draft. Output that fails the schema or the
// placeholder/company check is retried once with the reason (R21, U7).
func (s *Service) Draft(ctx context.Context, appID int64) (Draft, error) {
	app, err := s.loadApp(ctx, appID)
	if err != nil {
		return Draft{}, err
	}
	p, err := s.cfg.Profile.Get(ctx)
	if err != nil {
		return Draft{}, err
	}
	var style Style
	if err := s.cfg.DB.Tx(ctx, func(tx *sql.Tx) error {
		var err error
		style, err = takeStyle(ctx, tx)
		return err
	}); err != nil {
		return Draft{}, apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't prepare the letter.")
	}

	var a answer
	reason := ""
	for attempt := 1; attempt <= 2; attempt++ {
		a, reason, err = s.attempt(ctx, app, p, style, reason)
		if err != nil {
			return Draft{}, err
		}
		if reason == "" {
			break
		}
	}
	if reason != "" {
		return Draft{}, apperr.Wrap(errors.New("letter draft rejected twice: "+reason), apperr.CodeAIInvalidOutput,
			"Gemini's letter didn't pass LeGaJ's checks twice. Try generating it again.")
	}
	d := Draft{
		Style: style.ID, Salutation: a.Salutation, Opening: a.Opening, WhyOptions: a.WhyOptions,
		Body: a.Body, Closing: a.Closing, SignOff: a.SignOff,
	}
	rev, err := s.saveDraft(ctx, appID, d)
	if err != nil {
		return Draft{}, err
	}
	d.Rev = rev
	return d, nil
}

// attempt makes one AI call. A schema failure or a failed check comes back
// as a reason to retry; any other error is returned as is.
func (s *Service) attempt(ctx context.Context, app store.Application, p profile.Profile, style Style, retryReason string) (answer, string, error) {
	prompt, err := userPrompt(app, p, style, retryReason)
	if err != nil {
		return answer{}, "", apperr.Wrap(err, apperr.CodeInternal, apperr.GenericMessage)
	}
	temp := 0.9 // variety between letters (C20); grounding comes from the prompt
	resp, err := s.cfg.AI.Generate(ctx, ai.Request{
		Model: s.cfg.Model(ctx), System: systemPrompt, Parts: []ai.Part{ai.Text(prompt)},
		Temperature: &temp, Schema: []byte(outputSchema),
	})
	if apperr.CodeOf(err) == apperr.CodeAIInvalidOutput {
		return answer{}, "it didn't match the required JSON shape (for example, exactly three why paragraphs).", nil
	}
	if err != nil {
		return answer{}, "", err
	}
	a, err := decodeAnswer(resp.Text)
	if err != nil {
		return answer{}, "it wasn't the expected JSON.", nil
	}
	return a, check(a, app.Company), nil
}

// saveDraft stores d as the application's only current draft.
func (s *Service) saveDraft(ctx context.Context, appID int64, d Draft) (int64, error) {
	rev, err := newRev()
	if err != nil {
		return 0, apperr.Wrap(err, apperr.CodeInternal, apperr.GenericMessage)
	}
	d.Rev = rev
	body, err := json.Marshal(d)
	if err != nil {
		return 0, apperr.Wrap(err, apperr.CodeInternal, apperr.GenericMessage)
	}
	now := store.FormatTime(s.cfg.Now())
	err = events.Mutate(ctx, s.cfg.DB, s.cfg.Bus, events.Change(events.EntityDocument, appID), func(tx *sql.Tx) error {
		var live int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM applications WHERE id = ? AND deleted_at IS NULL`, appID).Scan(&live); err != nil {
			return apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't save the draft.")
		}
		if live == 0 {
			return errNotFound // deleted while the draft was being written (R13)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM documents WHERE application_id = ? AND kind = 'letter' AND status = 'draft'`, appID); err != nil {
			return apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't save the draft.")
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO documents (application_id, kind, status, body, created_at, updated_at)
			VALUES (?, 'letter', 'draft', ?, ?, ?)`, appID, string(body), now, now)
		if err != nil {
			return apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't save the draft.")
		}
		return nil
	})
	return rev, err
}

// newRev returns a random revision that survives a round trip through
// JavaScript numbers (at most 2^53 - 1).
func newRev() (int64, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1<<53-1))
	if err != nil {
		return 0, err
	}
	return n.Int64() + 1, nil
}

// effectiveRev is a stored draft's revision; drafts saved before
// revisions existed use their row ID.
func effectiveRev(rowID int64, d Draft) int64 {
	if d.Rev > 0 {
		return d.Rev
	}
	return rowID
}

var errDraftChanged = apperr.New(apperr.CodeConflict,
	"This draft was approved or replaced by a newer one. Reopen the review to see the latest.")

// updateDraft changes the stored draft rev in place: mutate sees the draft
// as stored now, so concurrent edits to other paragraphs survive. If that
// draft was approved or replaced, nothing is written (a conflict).
func (s *Service) updateDraft(ctx context.Context, appID, rev int64, mutate func(*Draft) error) (Draft, error) {
	var out Draft
	err := events.Mutate(ctx, s.cfg.DB, s.cfg.Bus, events.Change(events.EntityDocument, appID), func(tx *sql.Tx) error {
		var rowID int64
		var body string
		err := tx.QueryRowContext(ctx, `SELECT d.id, d.body FROM documents d JOIN applications a ON a.id = d.application_id
			WHERE d.application_id = ? AND d.kind = 'letter' AND d.status = 'draft' AND a.deleted_at IS NULL
			ORDER BY d.id DESC LIMIT 1`, appID).Scan(&rowID, &body)
		if errors.Is(err, sql.ErrNoRows) {
			return errDraftChanged
		}
		if err != nil {
			return apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't save the draft.")
		}
		var cur Draft
		if err := json.Unmarshal([]byte(body), &cur); err != nil {
			return apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't read the draft.")
		}
		if effectiveRev(rowID, cur) != rev {
			return errDraftChanged
		}
		if err := mutate(&cur); err != nil {
			return err
		}
		cur.Rev = rev // edits keep the generation's revision
		next, err := json.Marshal(cur)
		if err != nil {
			return apperr.Wrap(err, apperr.CodeInternal, apperr.GenericMessage)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE documents SET body = ?, updated_at = ? WHERE id = ?`,
			string(next), store.FormatTime(s.cfg.Now()), rowID); err != nil {
			return apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't save the draft.")
		}
		out = cur
		return nil
	})
	if err != nil {
		return Draft{}, apperr.Public(err)
	}
	return out, nil
}

// CurrentDraft returns the application's draft awaiting review.
func (s *Service) CurrentDraft(ctx context.Context, appID int64) (Draft, error) {
	_, d, err := s.currentDraft(ctx, appID)
	return d, err
}

// currentDraft also returns the draft's row ID, so Approve can retire
// exactly the draft it rendered.
func (s *Service) currentDraft(ctx context.Context, appID int64) (int64, Draft, error) {
	var id int64
	var body string
	err := s.cfg.DB.Reader().QueryRowContext(ctx, `SELECT id, body FROM documents
		WHERE application_id = ? AND kind = 'letter' AND status = 'draft' ORDER BY id DESC LIMIT 1`, appID).Scan(&id, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, Draft{}, apperr.New(apperr.CodeNotFound, "There's no draft for this application yet.")
	}
	if err != nil {
		return 0, Draft{}, apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't read the draft.")
	}
	var d Draft
	if err := json.Unmarshal([]byte(body), &d); err != nil {
		return 0, Draft{}, apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't read the draft.")
	}
	d.Rev = effectiveRev(id, d)
	return id, d, nil
}

// ToPDF assembles the letter: the chosen why paragraph follows the opening,
// then the body and closing; the header comes from the profile.
func ToPDF(d Draft, p profile.Profile, app store.Application, date time.Time) pdf.Letter {
	why := ""
	if d.ChosenWhy >= 0 && d.ChosenWhy < len(d.WhyOptions) {
		why = d.WhyOptions[d.ChosenWhy]
	}
	paras := []string{d.Opening}
	if why != "" {
		paras = append(paras, why)
	}
	paras = append(append(paras, d.Body...), d.Closing)
	return pdf.Letter{
		Name:       p.FullName,
		Contact:    joinNonEmpty(" · ", p.Email, p.Phone, p.Location),
		Date:       date.Format("2 January 2006"),
		Recipient:  nonEmpty("Hiring Team", app.Company, app.Location),
		Salutation: d.Salutation,
		Paragraphs: paras,
		Closing:    d.SignOff,
		Signature:  p.FullName,
	}
}

func nonEmpty(xs ...string) []string {
	var out []string
	for _, x := range xs {
		if strings.TrimSpace(x) != "" {
			out = append(out, x)
		}
	}
	return out
}

func joinNonEmpty(sep string, xs ...string) string {
	return strings.Join(nonEmpty(xs...), sep)
}
