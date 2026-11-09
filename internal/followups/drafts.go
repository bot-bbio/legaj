package followups

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/store"
)

// Follow-up statuses; the values match the followups table.
const (
	StatusDraft   = "draft"
	StatusSent    = "sent"
	StatusSkipped = "skipped"
)

const (
	maxSubject = 300
	maxBody    = 20_000
)

// DraftInput names a follow-up (kind, application, and the round for a
// thank-you) and carries the user's text.
type DraftInput struct {
	Kind    Kind   `json:"kind"`
	AppID   int64  `json:"appId"`
	RoundID int64  `json:"roundId"` // thank-yous only
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// DraftView is a draft with what must be fixed before sending (R27) and
// what the user should know (R29).
type DraftView struct {
	Kind     Kind     `json:"kind"`
	AppID    int64    `json:"appId"`
	RoundID  int64    `json:"roundId"`
	To       string   `json:"to"`
	Subject  string   `json:"subject"`
	Body     string   `json:"body"`
	Problems []string `json:"problems"`
	Warnings []string `json:"warnings"`
}

var (
	errNoApp   = apperr.New(apperr.CodeNotFound, "That application no longer exists.")
	errNoRound = apperr.New(apperr.CodeNotFound, "That interview round no longer exists.")
)

func writeErr(err error) error {
	return apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't save the follow-up. Try again.")
}

// subject is what a follow-up is about.
type subject struct {
	company, role string
	interviewer   store.Interviewer
	daysSince     int
}

// target checks the kind and that the application (and round) exist, and
// returns what the template needs.
func (s *Service) target(ctx context.Context, k Kind, appID, roundID int64) (subject, error) {
	var sub subject
	switch k {
	case KindAppliedNudge:
		if roundID != 0 {
			return sub, apperr.New(apperr.CodeInvalid, "An application follow-up doesn't belong to an interview round.")
		}
	case KindThankYou:
	default:
		return sub, apperr.New(apperr.CodeInvalid, "Unknown follow-up type.")
	}
	var changed sql.NullString
	err := s.cfg.DB.Reader().QueryRowContext(ctx, `SELECT company, role,
			(SELECT max(e.at) FROM stage_events e WHERE e.application_id = a.id)
		FROM applications a WHERE id = ? AND deleted_at IS NULL`, appID).Scan(&sub.company, &sub.role, &changed)
	if errors.Is(err, sql.ErrNoRows) {
		return sub, errNoApp
	}
	if err != nil {
		return sub, readErr(err)
	}
	sub.daysSince = -1
	if changed.Valid {
		if t, err := store.ParseTime(changed.String); err == nil {
			sub.daysSince = CalendarDays(t, s.cfg.Now(), s.cfg.Loc())
		}
	}
	if k == KindThankYou {
		var interviewers string
		err := s.cfg.DB.Reader().QueryRowContext(ctx, `SELECT interviewers FROM rounds
			WHERE id = ? AND application_id = ?`, roundID, appID).Scan(&interviewers)
		if errors.Is(err, sql.ErrNoRows) {
			return sub, errNoRound
		}
		if err != nil {
			return sub, readErr(err)
		}
		sub.interviewer = store.FirstInterviewer(interviewers)
	}
	return sub, nil
}

// findDraft returns the stored draft row for a follow-up, if any.
func findDraft(ctx context.Context, q interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}, k Kind, appID, roundID int64) (int64, DraftInput, error) {
	in := DraftInput{Kind: k, AppID: appID, RoundID: roundID}
	var id int64
	err := q.QueryRowContext(ctx, `SELECT id, recipient, subject, body FROM followups
		WHERE application_id = ? AND kind = ? AND coalesce(round_id, 0) = ? AND status = 'draft'
		ORDER BY id DESC LIMIT 1`, appID, string(k), roundID).Scan(&id, &in.To, &in.Subject, &in.Body)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, in, nil
	}
	return id, in, err
}

func view(in DraftInput) DraftView {
	m := Message{To: in.To, Subject: in.Subject, Body: in.Body}
	problems := check(in.Subject, in.Body)
	if strings.TrimSpace(in.Subject) == "" {
		problems = append(problems, "Add a subject.")
	}
	return DraftView{
		Kind: in.Kind, AppID: in.AppID, RoundID: in.RoundID, To: in.To, Subject: in.Subject, Body: in.Body,
		Problems: problems, Warnings: m.Warnings(),
	}
}

// Draft returns the user's saved draft for a follow-up, or a fresh one
// rendered from the default template (R27).
func (s *Service) Draft(ctx context.Context, k Kind, appID, roundID int64) (DraftView, error) {
	sub, err := s.target(ctx, k, appID, roundID)
	if err != nil {
		return DraftView{}, err
	}
	id, saved, err := findDraft(ctx, s.cfg.DB.Reader(), k, appID, roundID)
	if err != nil {
		return DraftView{}, readErr(err)
	}
	if id != 0 {
		return view(saved), nil
	}
	name := ""
	if s.cfg.Profile != nil {
		if p, err := s.cfg.Profile.Get(ctx); err == nil {
			name = p.FullName
		}
	}
	r := Render(DefaultTemplate(k), Values{
		Company: sub.company, Role: sub.role, Interviewer: sub.interviewer.Name, DaysSince: sub.daysSince, Name: name,
	})
	// A thank-you goes to the interviewer when their address is known.
	to := ""
	if e := strings.TrimSpace(sub.interviewer.Email); recipientRE.MatchString(e) {
		to = e
	}
	return view(DraftInput{Kind: k, AppID: appID, RoundID: roundID, To: to, Subject: r.Subject, Body: r.Body}), nil
}

func validate(in DraftInput) error {
	switch {
	case utf8.RuneCountInString(in.Subject) > maxSubject:
		return apperr.New(apperr.CodeInvalid, "The subject is too long.")
	case utf8.RuneCountInString(in.Body) > maxBody:
		return apperr.New(apperr.CodeInvalid, "The message is too long.")
	}
	if to := strings.TrimSpace(in.To); to != "" && (len(to) > 254 || !recipientRE.MatchString(to)) {
		return apperr.New(apperr.CodeInvalid, "Enter one email address, like name@example.com, or leave it empty.")
	}
	return nil
}

func nullRound(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

// SaveDraft stores the user's edits; one draft per follow-up.
func (s *Service) SaveDraft(ctx context.Context, in DraftInput) (DraftView, error) {
	in.To = strings.TrimSpace(in.To)
	if err := validate(in); err != nil {
		return DraftView{}, err
	}
	if _, err := s.target(ctx, in.Kind, in.AppID, in.RoundID); err != nil {
		return DraftView{}, err
	}
	now := store.FormatTime(s.cfg.Now())
	err := events.Mutate(ctx, s.cfg.DB, s.cfg.Bus, events.Change(events.EntityFollowup, in.AppID), func(tx *sql.Tx) error {
		id, _, err := findDraft(ctx, tx, in.Kind, in.AppID, in.RoundID)
		if err != nil {
			return writeErr(err)
		}
		if id != 0 {
			_, err = tx.ExecContext(ctx, `UPDATE followups SET recipient = ?, subject = ?, body = ? WHERE id = ?`,
				in.To, in.Subject, in.Body, id)
		} else {
			_, err = tx.ExecContext(ctx, `INSERT INTO followups
				(application_id, round_id, kind, status, recipient, subject, body, created_at)
				VALUES (?, ?, ?, 'draft', ?, ?, ?, ?)`,
				in.AppID, nullRound(in.RoundID), string(in.Kind), in.To, in.Subject, in.Body, now)
		}
		if err != nil {
			return writeErr(err)
		}
		return nil
	})
	if err != nil {
		return DraftView{}, apperr.Public(err)
	}
	return view(in), nil
}

// Compose saves the draft and returns its compose link (R29). A draft with
// problems (unfilled placeholders, no subject) is refused, so nothing is
// sent with blanks (R27).
func (s *Service) Compose(ctx context.Context, in DraftInput, c Client) (string, error) {
	v, err := s.SaveDraft(ctx, in)
	if err != nil {
		return "", err
	}
	if len(v.Problems) > 0 {
		return "", apperr.New(apperr.CodeInvalid, v.Problems[0])
	}
	return ComposeURL(c, Message{To: v.To, Subject: v.Subject, Body: v.Body})
}

// Log records the user's answer to "Sent?" (R29): sent or skipped. It
// resolves the stored draft, or records one if there was none, and the
// follow-up stops being due.
func (s *Service) Log(ctx context.Context, in DraftInput, status string) error {
	if status != StatusSent && status != StatusSkipped {
		return apperr.New(apperr.CodeInvalid, "A follow-up can only be logged as sent or skipped.")
	}
	in.To = strings.TrimSpace(in.To)
	if err := validate(in); err != nil {
		return err
	}
	if _, err := s.target(ctx, in.Kind, in.AppID, in.RoundID); err != nil {
		return err
	}
	now := store.FormatTime(s.cfg.Now())
	err := events.Mutate(ctx, s.cfg.DB, s.cfg.Bus, events.Change(events.EntityFollowup, in.AppID), func(tx *sql.Tx) error {
		id, saved, err := findDraft(ctx, tx, in.Kind, in.AppID, in.RoundID)
		if err != nil {
			return writeErr(err)
		}
		// Keep what was sent; a skip without text keeps the saved draft's.
		if in.Subject == "" && in.Body == "" {
			in.To, in.Subject, in.Body = saved.To, saved.Subject, saved.Body
		}
		if id != 0 {
			_, err = tx.ExecContext(ctx, `UPDATE followups SET status = ?, recipient = ?, subject = ?, body = ?, resolved_at = ?
				WHERE id = ?`, status, in.To, in.Subject, in.Body, now, id)
		} else {
			_, err = tx.ExecContext(ctx, `INSERT INTO followups
				(application_id, round_id, kind, status, recipient, subject, body, created_at, resolved_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				in.AppID, nullRound(in.RoundID), string(in.Kind), status, in.To, in.Subject, in.Body, now, now)
		}
		if err != nil {
			return writeErr(err)
		}
		return nil
	})
	return apperr.Public(err)
}
