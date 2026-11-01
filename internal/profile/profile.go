// Package profile stores the user's profile (R9): contact details, target
// role, experience, education and skills. Letters and answers are grounded
// in it (R21, R25).
//
// The profile is one JSON document, saved whole. V1 rebuilt it field by
// field and by list position, which crashed on a changed experience count
// (A3), dropped education location and details (A4), and broke dates by
// splitting "1842-01 - 1843-09" on "-" (A5). Here dates are structured,
// lists carry IDs, and nothing is split.
package profile

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/store"
)

// Profile is the user's profile.
type Profile struct {
	FullName    string       `json:"fullName"`
	Email       string       `json:"email"`
	Phone       string       `json:"phone"`
	Location    string       `json:"location"`
	Headline    string       `json:"headline"`
	TargetRole  string       `json:"targetRole"`
	Summary     string       `json:"summary"`
	Links       []Link       `json:"links"`
	Experiences []Experience `json:"experiences"`
	Education   []Education  `json:"education"`
	Skills      []SkillGroup `json:"skills"`
}

// Link is a labelled URL (portfolio, LinkedIn, GitHub).
type Link struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// Experience is one job.
type Experience struct {
	ID       string    `json:"id"`
	Company  string    `json:"company"`
	Title    string    `json:"title"`
	Location string    `json:"location"`
	Start    YearMonth `json:"start"`
	End      YearMonth `json:"end"`
	Current  bool      `json:"current"` // End must be empty
	Bullets  []string  `json:"bullets"`
}

// Education is one school, degree or course.
type Education struct {
	ID       string    `json:"id"`
	School   string    `json:"school"`
	Degree   string    `json:"degree"`
	Field    string    `json:"field"`
	Location string    `json:"location"`
	Start    YearMonth `json:"start"`
	End      YearMonth `json:"end"`
	Details  string    `json:"details"`
}

// SkillGroup is a named list of skills.
type SkillGroup struct {
	Category string   `json:"category"`
	Items    []string `json:"items"`
}

// YearMonth is a date to month precision. Month 0 means year only; the
// zero value means "not set".
type YearMonth struct {
	Year  int `json:"year"`
	Month int `json:"month"`
}

// IsZero reports whether the date is unset.
func (d YearMonth) IsZero() bool { return d.Year == 0 && d.Month == 0 }

// String renders "YYYY-MM", "YYYY", or "" when unset.
func (d YearMonth) String() string {
	switch {
	case d.IsZero():
		return ""
	case d.Month == 0:
		return fmt.Sprintf("%04d", d.Year)
	default:
		return fmt.Sprintf("%04d-%02d", d.Year, d.Month)
	}
}

func (d YearMonth) before(o YearMonth) bool {
	if d.Year != o.Year {
		return d.Year < o.Year
	}
	// A year-only date sorts as the whole year, so it's never "before" a
	// month in the same year.
	return d.Month != 0 && o.Month != 0 && d.Month < o.Month
}

var yearMonthRE = regexp.MustCompile(`^(\d{4})(?:-(\d{2}))?$`)

// ParseYearMonth reads "YYYY-MM", "YYYY" or "" (unset). It is the only way
// text becomes a date; nothing splits ranges on "-".
func ParseYearMonth(s string) (YearMonth, error) {
	if s == "" {
		return YearMonth{}, nil
	}
	m := yearMonthRE.FindStringSubmatch(s)
	if m == nil {
		return YearMonth{}, fmt.Errorf("%q is not YYYY or YYYY-MM", s)
	}
	y, _ := strconv.Atoi(m[1])
	d := YearMonth{Year: y}
	if m[2] != "" {
		d.Month, _ = strconv.Atoi(m[2])
	}
	if err := d.validate(); err != nil {
		return YearMonth{}, err
	}
	return d, nil
}

// Service loads and saves the profile.
type Service struct {
	db  *store.DB
	bus *events.Bus
	now func() time.Time
}

// NewService creates the service.
func NewService(db *store.DB, bus *events.Bus, now func() time.Time) *Service {
	return &Service{db: db, bus: bus, now: now}
}

// Get returns the saved profile, or an empty one.
func (s *Service) Get(ctx context.Context) (Profile, error) {
	var data string
	err := s.db.Reader().QueryRowContext(ctx, `SELECT data FROM profile WHERE id = 1`).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return normalize(Profile{}), nil
	}
	if err != nil {
		return Profile{}, apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't read your profile.")
	}
	var p Profile
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		return Profile{}, apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't read your profile.")
	}
	return normalize(p), nil
}

// Save normalizes, validates and stores the whole profile, then emits a
// profile change. It returns what was stored.
func (s *Service) Save(ctx context.Context, in Profile) (Profile, error) {
	p := normalize(in)
	if err := Validate(p); err != nil {
		return Profile{}, err
	}
	assignIDs(&p)
	data, err := json.Marshal(p)
	if err != nil {
		return Profile{}, apperr.Wrap(err, apperr.CodeInternal, apperr.GenericMessage)
	}
	err = events.Mutate(ctx, s.db, s.bus, events.Change(events.EntityProfile), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO profile (id, data, updated_at) VALUES (1, ?, ?)
			ON CONFLICT(id) DO UPDATE SET data = excluded.data, updated_at = excluded.updated_at`,
			string(data), store.FormatTime(s.now()))
		return err
	})
	if err != nil {
		return Profile{}, apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't save your profile. Try again.")
	}
	return p, nil
}

// assignIDs gives new list items a random ID; existing IDs are kept.
func assignIDs(p *Profile) {
	for i := range p.Experiences {
		if p.Experiences[i].ID == "" {
			p.Experiences[i].ID = newID()
		}
	}
	for i := range p.Education {
		if p.Education[i].ID == "" {
			p.Education[i].ID = newID()
		}
	}
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b) // crypto/rand.Read never fails on supported platforms
	return hex.EncodeToString(b)
}
