package followups

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"time"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/profile"
	"github.com/bot-bbio/legaj/internal/store"
)

// Config wires the service.
type Config struct {
	DB  *store.DB
	Bus *events.Bus
	Now func() time.Time
	// Loc is the user's time zone; "days" are its calendar days (R28).
	// Default time.Local, which is the Windows zone in the app.
	Loc func() *time.Location
	// Days is the Settings delay before an applied nudge is due (R37).
	Days func(ctx context.Context) int
	// Profile signs drafts with the user's name; nil leaves {name} flagged.
	Profile *profile.Service
}

// Service decides what's due and records what the user sent or skipped.
type Service struct{ cfg Config }

// NewService creates the service.
func NewService(cfg Config) *Service {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Loc == nil {
		cfg.Loc = func() *time.Location { return time.Local }
	}
	if cfg.Days == nil {
		cfg.Days = func(context.Context) int { return 7 }
	}
	return &Service{cfg: cfg}
}

// CalendarDays counts the local calendar days from a's date to b's date in
// loc: Monday 23:30 to Tuesday 00:10 is one day, and a week containing a
// daylight-saving change is still seven (R28). It's negative if b's date
// is earlier.
func CalendarDays(a, b time.Time, loc *time.Location) int {
	date := func(t time.Time) time.Time {
		y, m, d := t.In(loc).Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) // UTC days are all 24 h
	}
	return int(date(b).Sub(date(a)) / (24 * time.Hour))
}

// DueItem is one follow-up waiting for the user.
type DueItem struct {
	Kind        Kind      `json:"kind"`
	AppID       int64     `json:"appId"`
	RoundID     int64     `json:"roundId"` // 0 for an applied nudge
	Company     string    `json:"company"`
	Role        string    `json:"role"`
	Interviewer string    `json:"interviewer"` // first listed, thank-yous only
	DueSince    time.Time `json:"dueSince"`
	DaysSince   int       `json:"daysSince"` // local days since applying (nudges)
}

func readErr(err error) error {
	return apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't read your follow-ups.")
}

// Due lists every follow-up due now, oldest first (R28):
//   - an applied nudge, once the row has sat in Applied for the configured
//     number of local calendar days since its latest stage change, until a
//     nudge is logged as sent or skipped;
//   - an interview thank-you, from the round's end (its UTC start plus its
//     duration) until one is logged, unless the application was rejected
//     or withdrawn.
func (s *Service) Due(ctx context.Context) ([]DueItem, error) {
	now, loc := s.cfg.Now(), s.cfg.Loc()
	days := s.cfg.Days(ctx)
	out := []DueItem{}

	nudges, err := s.cfg.DB.Reader().QueryContext(ctx, `SELECT a.id, a.company, a.role,
			(SELECT max(e.at) FROM stage_events e WHERE e.application_id = a.id)
		FROM applications a
		WHERE a.deleted_at IS NULL AND a.stage = 'applied'
		  AND NOT EXISTS (SELECT 1 FROM followups f WHERE f.application_id = a.id
		                  AND f.kind = 'applied_nudge' AND f.status IN ('sent', 'skipped'))`)
	if err != nil {
		return nil, readErr(err)
	}
	defer nudges.Close()
	for nudges.Next() {
		var it DueItem
		var since sql.NullString
		if err := nudges.Scan(&it.AppID, &it.Company, &it.Role, &since); err != nil {
			return nil, readErr(err)
		}
		if !since.Valid {
			continue
		}
		changed, err := store.ParseTime(since.String)
		if err != nil {
			return nil, readErr(err)
		}
		it.DaysSince = CalendarDays(changed, now, loc)
		if it.DaysSince < days {
			continue
		}
		y, m, d := changed.In(loc).Date()
		it.Kind, it.DueSince = KindAppliedNudge, time.Date(y, m, d+days, 0, 0, 0, 0, loc)
		out = append(out, it)
	}
	if err := nudges.Err(); err != nil {
		return nil, readErr(err)
	}

	rounds, err := s.cfg.DB.Reader().QueryContext(ctx, `SELECT r.id, r.application_id, a.company, a.role,
			r.starts_at, r.duration_min, r.interviewers
		FROM rounds r JOIN applications a ON a.id = r.application_id
		WHERE a.deleted_at IS NULL AND a.stage NOT IN ('rejected', 'withdrawn')
		  AND NOT EXISTS (SELECT 1 FROM followups f WHERE f.round_id = r.id
		                  AND f.kind = 'thank_you' AND f.status IN ('sent', 'skipped'))`)
	if err != nil {
		return nil, readErr(err)
	}
	defer rounds.Close()
	for rounds.Next() {
		var it DueItem
		var starts, interviewers string
		var minutes int
		if err := rounds.Scan(&it.RoundID, &it.AppID, &it.Company, &it.Role, &starts, &minutes, &interviewers); err != nil {
			return nil, readErr(err)
		}
		start, err := store.ParseTime(starts)
		if err != nil {
			return nil, readErr(err)
		}
		end := start.Add(time.Duration(minutes) * time.Minute)
		if now.Before(end) {
			continue
		}
		var names []string
		if json.Unmarshal([]byte(interviewers), &names) == nil && len(names) > 0 {
			it.Interviewer = names[0]
		}
		it.Kind, it.DueSince = KindThankYou, end
		out = append(out, it)
	}
	if err := rounds.Err(); err != nil {
		return nil, readErr(err)
	}

	slices.SortStableFunc(out, func(a, b DueItem) int { return a.DueSince.Compare(b.DueSince) })
	return out, nil
}
