// Package letters drafts cover letters grounded in the job description and
// the user's profile (R21, D8), and turns approved drafts into PDF letters.
package letters

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
	if err := s.saveDraft(ctx, appID, d); err != nil {
		return Draft{}, err
	}
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
func (s *Service) saveDraft(ctx context.Context, appID int64, d Draft) error {
	body, err := json.Marshal(d)
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, apperr.GenericMessage)
	}
	now := store.FormatTime(s.cfg.Now())
	return events.Mutate(ctx, s.cfg.DB, s.cfg.Bus, events.Change(events.EntityDocument, appID), func(tx *sql.Tx) error {
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
