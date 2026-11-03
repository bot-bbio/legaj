// Package interviews keeps interview rounds (R30), exports them to
// calendars (R31) and drafts questions to ask (R32).
//
// A round's start is stored as a UTC instant plus the IANA zone it was
// entered in, so it stays right when the user travels or clocks change.
package interviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	_ "time/tzdata" // IANA zones on Windows machines without Go installed
	"unicode/utf8"

	"github.com/bot-bbio/legaj/internal/ai"
	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/pipeline"
	"github.com/bot-bbio/legaj/internal/profile"
	"github.com/bot-bbio/legaj/internal/store"
)

// LocalLayout is how a round's wall-clock start travels to and from the UI.
const LocalLayout = "2006-01-02T15:04"

// Kinds are the round types the UI offers.
var Kinds = []string{"phone", "video", "onsite", "technical", "panel", "other"}

const (
	defaultMinutes  = 60
	maxMinutes      = 12 * 60
	maxInterviewers = 10
	maxShort        = 200
	maxLink         = 2000
	maxNotes        = 10_000
)

// RoundInput is a round as the user enters it: wall-clock time in a zone.
type RoundInput struct {
	AppID        int64               `json:"appId"`
	Local        string              `json:"local"` // "2026-11-03T18:00", in Zone
	Zone         string              `json:"zone"`  // IANA, e.g. "America/New_York"
	DurationMin  int                 `json:"durationMin"`
	Kind         string              `json:"kind"`
	Location     string              `json:"location"`
	Link         string              `json:"link"`
	Interviewers []store.Interviewer `json:"interviewers"`
	PrepNotes    string              `json:"prepNotes"`
	PostNotes    string              `json:"postNotes"`
	Outcome      string              `json:"outcome"`
}

// Round is a stored round.
type Round struct {
	RoundInput
	ID        int64     `json:"id"`
	Company   string    `json:"company"`
	Role      string    `json:"role"`
	Stage     string    `json:"stage"`    // the application's stage
	StartsAt  time.Time `json:"startsAt"` // UTC
	EndsAt    time.Time `json:"endsAt"`
	Questions []string  `json:"questions"` // questions to ask (R32)
	Created   time.Time `json:"created"`
	Updated   time.Time `json:"updated"`
}

// Lists are the Interviews view's two lists (R30).
type Lists struct {
	Upcoming []Round `json:"upcoming"` // not yet ended, soonest first
	Past     []Round `json:"past"`     // ended, most recent first
}

// Config wires the service.
type Config struct {
	DB      *store.DB
	Bus     *events.Bus
	Machine *pipeline.Machine
	Now     func() time.Time
	// AI, Profile and Model draft questions to ask (R32).
	AI      ai.Provider
	Profile *profile.Service
	Model   func(ctx context.Context) string
}

// Service manages rounds.
type Service struct{ cfg Config }

// NewService creates the service.
func NewService(cfg Config) *Service {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Service{cfg: cfg}
}

var (
	errNoRound = apperr.New(apperr.CodeNotFound, "That interview round no longer exists.")
	emailRE    = regexp.MustCompile(`^[A-Za-z0-9._+-]+@[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+$`)
)

func invalid(msg string) error { return apperr.New(apperr.CodeInvalid, msg) }

func readErr(err error) error {
	return apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't read your interviews.")
}

func writeErr(err error) error {
	return apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't save the interview round. Try again.")
}

// clean trims and validates input, returning the UTC start.
func clean(in *RoundInput) (time.Time, error) {
	in.Zone, in.Local = strings.TrimSpace(in.Zone), strings.TrimSpace(in.Local)
	in.Kind, in.Location, in.Link = strings.TrimSpace(in.Kind), strings.TrimSpace(in.Location), strings.TrimSpace(in.Link)
	in.PrepNotes, in.PostNotes, in.Outcome = strings.TrimSpace(in.PrepNotes), strings.TrimSpace(in.PostNotes), strings.TrimSpace(in.Outcome)
	if in.Zone == "" {
		return time.Time{}, invalid("Choose the time zone the interview is in.")
	}
	loc, err := time.LoadLocation(in.Zone)
	if err != nil {
		return time.Time{}, invalid("That time zone isn't one LeGaJ knows.")
	}
	start, err := time.ParseInLocation(LocalLayout, in.Local, loc)
	if err != nil {
		return time.Time{}, invalid("Enter the interview's date and time.")
	}
	if in.DurationMin == 0 {
		in.DurationMin = defaultMinutes
	}
	if in.Kind == "" {
		in.Kind = "other"
	}
	switch {
	case in.DurationMin < 5 || in.DurationMin > maxMinutes:
		return time.Time{}, invalid("An interview lasts between 5 minutes and 12 hours.")
	case !slices.Contains(Kinds, in.Kind):
		return time.Time{}, invalid("Unknown interview type.")
	case utf8.RuneCountInString(in.Location) > maxShort || utf8.RuneCountInString(in.Outcome) > maxShort:
		return time.Time{}, invalid("The location or outcome is too long.")
	case utf8.RuneCountInString(in.PrepNotes) > maxNotes || utf8.RuneCountInString(in.PostNotes) > maxNotes:
		return time.Time{}, invalid("The notes are too long (over 10,000 characters).")
	case len(in.Interviewers) > maxInterviewers:
		return time.Time{}, invalid("Add at most 10 interviewers.")
	}
	if in.Link != "" {
		u, err := url.Parse(in.Link)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || len(in.Link) > maxLink {
			return time.Time{}, invalid("The meeting link must be a web address starting with https://.")
		}
	}
	people := make([]store.Interviewer, 0, len(in.Interviewers))
	for _, p := range in.Interviewers {
		p.Name, p.Title, p.Email = strings.TrimSpace(p.Name), strings.TrimSpace(p.Title), strings.TrimSpace(p.Email)
		switch {
		case p.Name == "":
			return time.Time{}, invalid("Each interviewer needs a name.")
		case utf8.RuneCountInString(p.Name) > maxShort || utf8.RuneCountInString(p.Title) > maxShort:
			return time.Time{}, invalid("An interviewer's name or title is too long.")
		case p.Email != "" && (len(p.Email) > 254 || !emailRE.MatchString(p.Email)):
			return time.Time{}, invalid("Check the interviewer's email address.")
		}
		people = append(people, p)
	}
	in.Interviewers = people
	return start.UTC(), nil
}

func (s *Service) emit(appID int64) {
	s.cfg.Bus.Emit(events.Change(events.EntityRound, appID))
	s.cfg.Bus.Emit(events.Change(events.EntityApplication, appID))
}

// Add creates a round. Adding one to a row in Lead, Ready or Applied moves
// it to Interviewing through the state machine; other stages never change
// (R30).
func (s *Service) Add(ctx context.Context, in RoundInput) (Round, error) {
	start, err := clean(&in)
	if err != nil {
		return Round{}, err
	}
	people, err := json.Marshal(in.Interviewers)
	if err != nil {
		return Round{}, apperr.Wrap(err, apperr.CodeInternal, apperr.GenericMessage)
	}
	now := store.FormatTime(s.cfg.Now())
	var id int64
	err = s.cfg.DB.Tx(ctx, func(tx *sql.Tx) error {
		var live int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM applications WHERE id = ? AND deleted_at IS NULL`, in.AppID).Scan(&live); err != nil {
			return writeErr(err)
		}
		if live == 0 {
			return pipeline.ErrNotFound
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO rounds (application_id, starts_at, zone, duration_min, kind, location, link,
			interviewers, prep_notes, post_notes, outcome, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			in.AppID, store.FormatTime(start), in.Zone, in.DurationMin, in.Kind, in.Location, in.Link,
			string(people), in.PrepNotes, in.PostNotes, in.Outcome, now, now)
		if err != nil {
			return writeErr(err)
		}
		if id, err = res.LastInsertId(); err != nil {
			return writeErr(err)
		}
		_, err = s.cfg.Machine.AutoAdvance(ctx, tx, in.AppID, pipeline.RuleRoundAdded)
		return err
	})
	if err != nil {
		return Round{}, apperr.Public(err)
	}
	s.emit(in.AppID)
	return s.Get(ctx, id)
}

// Update replaces a round's details. The application can't change.
func (s *Service) Update(ctx context.Context, id int64, in RoundInput) (Round, error) {
	start, err := clean(&in)
	if err != nil {
		return Round{}, err
	}
	people, err := json.Marshal(in.Interviewers)
	if err != nil {
		return Round{}, apperr.Wrap(err, apperr.CodeInternal, apperr.GenericMessage)
	}
	var appID int64
	err = s.cfg.DB.Tx(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `SELECT r.application_id FROM rounds r JOIN applications a ON a.id = r.application_id
			WHERE r.id = ? AND a.deleted_at IS NULL`, id).Scan(&appID)
		if errors.Is(err, sql.ErrNoRows) {
			return errNoRound
		}
		if err != nil {
			return writeErr(err)
		}
		_, err = tx.ExecContext(ctx, `UPDATE rounds SET starts_at = ?, zone = ?, duration_min = ?, kind = ?, location = ?,
			link = ?, interviewers = ?, prep_notes = ?, post_notes = ?, outcome = ?, updated_at = ? WHERE id = ?`,
			store.FormatTime(start), in.Zone, in.DurationMin, in.Kind, in.Location, in.Link, string(people),
			in.PrepNotes, in.PostNotes, in.Outcome, store.FormatTime(s.cfg.Now()), id)
		if err != nil {
			return writeErr(err)
		}
		return nil
	})
	if err != nil {
		return Round{}, apperr.Public(err)
	}
	s.emit(appID)
	return s.Get(ctx, id)
}

// Delete removes a round; the application's stage is left alone.
func (s *Service) Delete(ctx context.Context, id int64) error {
	var appID int64
	err := s.cfg.DB.Tx(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `SELECT application_id FROM rounds WHERE id = ?`, id).Scan(&appID)
		if errors.Is(err, sql.ErrNoRows) {
			return errNoRound
		}
		if err != nil {
			return writeErr(err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM rounds WHERE id = ?`, id); err != nil {
			return writeErr(err)
		}
		return nil
	})
	if err != nil {
		return apperr.Public(err)
	}
	s.emit(appID)
	return nil
}

const roundColumns = `r.id, r.application_id, a.company, a.role, a.stage, r.starts_at, r.zone, r.duration_min, r.kind,
	r.location, r.link, r.interviewers, r.prep_notes, r.post_notes, r.outcome, r.questions, r.created_at, r.updated_at`

func scan(row interface{ Scan(dest ...any) error }) (Round, error) {
	var r Round
	var starts, people, questions, created, updated string
	if err := row.Scan(&r.ID, &r.AppID, &r.Company, &r.Role, &r.Stage, &starts, &r.Zone, &r.DurationMin, &r.Kind,
		&r.Location, &r.Link, &people, &r.PrepNotes, &r.PostNotes, &r.Outcome, &questions, &created, &updated); err != nil {
		return r, err
	}
	var err error
	if r.StartsAt, err = store.ParseTime(starts); err != nil {
		return r, err
	}
	r.EndsAt = r.StartsAt.Add(time.Duration(r.DurationMin) * time.Minute)
	// Timestamps only feed the calendar export's identity; a row written
	// without them still loads.
	r.Created, _ = store.ParseTime(created)
	r.Updated, _ = store.ParseTime(updated)
	loc, err := time.LoadLocation(r.Zone)
	if err != nil {
		loc = time.UTC
	}
	r.Local = r.StartsAt.In(loc).Format(LocalLayout)
	if err := json.Unmarshal([]byte(people), &r.Interviewers); err != nil {
		return r, err
	}
	if err := json.Unmarshal([]byte(questions), &r.Questions); err != nil {
		return r, err
	}
	if r.Interviewers == nil {
		r.Interviewers = []store.Interviewer{}
	}
	if r.Questions == nil {
		r.Questions = []string{}
	}
	return r, nil
}

// Get returns one round of a live application.
func (s *Service) Get(ctx context.Context, id int64) (Round, error) {
	r, err := scan(s.cfg.DB.Reader().QueryRowContext(ctx, `SELECT `+roundColumns+`
		FROM rounds r JOIN applications a ON a.id = r.application_id
		WHERE r.id = ? AND a.deleted_at IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return r, errNoRound
	}
	if err != nil {
		return r, readErr(err)
	}
	return r, nil
}

func (s *Service) query(ctx context.Context, where string, args ...any) ([]Round, error) {
	rows, err := s.cfg.DB.Reader().QueryContext(ctx, `SELECT `+roundColumns+`
		FROM rounds r JOIN applications a ON a.id = r.application_id
		WHERE a.deleted_at IS NULL `+where+` ORDER BY r.starts_at, r.id`, args...)
	if err != nil {
		return nil, readErr(err)
	}
	defer rows.Close()
	out := []Round{}
	for rows.Next() {
		r, err := scan(rows)
		if err != nil {
			return nil, readErr(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, readErr(err)
	}
	return out, nil
}

// ForApp returns an application's rounds in time order.
func (s *Service) ForApp(ctx context.Context, appID int64) ([]Round, error) {
	return s.query(ctx, `AND r.application_id = ?`, appID)
}

// Lists splits every round into Upcoming (not yet ended, soonest first)
// and Past (ended, most recent first) (R30).
func (s *Service) Lists(ctx context.Context) (Lists, error) {
	all, err := s.query(ctx, ``)
	if err != nil {
		return Lists{}, err
	}
	now := s.cfg.Now()
	out := Lists{Upcoming: []Round{}, Past: []Round{}}
	for _, r := range all {
		if r.EndsAt.After(now) {
			out.Upcoming = append(out.Upcoming, r)
		} else {
			out.Past = append(out.Past, r)
		}
	}
	slices.Reverse(out.Past)
	return out, nil
}
