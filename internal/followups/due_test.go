package followups_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/bot-bbio/legaj/internal/followups"
	"github.com/bot-bbio/legaj/internal/store"
	"github.com/bot-bbio/legaj/internal/testenv"
)

var ctx = context.Background()

func zone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("zone %s: %v", name, err)
	}
	return loc
}

type dueFixture struct {
	db   *store.DB
	svc  *followups.Service
	now  time.Time
	days int
}

func newDue(t *testing.T, loc *time.Location) *dueFixture {
	t.Helper()
	db, st, err := store.Open(ctx, store.Options{Dir: testenv.Isolate(t)})
	if err != nil || st.Kind != store.StartupReady {
		t.Fatalf("open: %v %+v", err, st)
	}
	t.Cleanup(func() { _ = db.Close() })
	f := &dueFixture{db: db, days: 7}
	f.svc = followups.NewService(followups.Config{
		DB:   db,
		Now:  func() time.Time { return f.now },
		Loc:  func() *time.Location { return loc },
		Days: func(context.Context) int { return f.days },
	})
	return f
}

func (f *dueFixture) exec(t *testing.T, q string, args ...any) int64 {
	t.Helper()
	var id int64
	if err := f.db.Tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, q, args...)
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

// applied creates an application that entered stage s at the given time.
func (f *dueFixture) applied(t *testing.T, company string, s store.Stage, at time.Time) int64 {
	t.Helper()
	ts := store.FormatTime(at)
	id := f.exec(t, `INSERT INTO applications (company, role, source, stage, applied_at, created_at, updated_at)
		VALUES (?, 'Engineer', 'clip', ?, ?, ?, ?)`, company, string(s), ts, ts, ts)
	f.exec(t, `INSERT INTO stage_events (application_id, from_stage, to_stage, cause, reason, at)
		VALUES (?, 'ready', ?, 'user', '', ?)`, id, string(s), ts)
	return id
}

func (f *dueFixture) due(t *testing.T) []followups.DueItem {
	t.Helper()
	items, err := f.svc.Due(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func TestR28_DueAfter7LocalDaysNoActivity(t *testing.T) {
	warsaw := zone(t, "Europe/Warsaw")
	f := newDue(t, warsaw)
	// Applied late on 2 November, local time.
	id := f.applied(t, "Acme", store.StageApplied, time.Date(2026, 11, 2, 23, 30, 0, 0, warsaw))

	f.now = time.Date(2026, 11, 8, 23, 59, 0, 0, warsaw) // 6 local days later
	if got := f.due(t); len(got) != 0 {
		t.Fatalf("due after 6 days: %+v", got)
	}
	f.now = time.Date(2026, 11, 9, 0, 10, 0, 0, warsaw) // the 7th local day, though < 7×24 h
	got := f.due(t)
	if len(got) != 1 || got[0].AppID != id || got[0].Kind != followups.KindAppliedNudge || got[0].DaysSince != 7 {
		t.Fatalf("due on day 7: %+v", got)
	}

	// A logged follow-up (sent or skipped) clears it; a draft doesn't.
	f.exec(t, `INSERT INTO followups (application_id, kind, status, created_at) VALUES (?, 'applied_nudge', 'draft', '')`, id)
	if len(f.due(t)) != 1 {
		t.Fatal("a draft isn't a logged follow-up")
	}
	f.exec(t, `INSERT INTO followups (application_id, kind, status, created_at, resolved_at) VALUES (?, 'applied_nudge', 'sent', '', '')`, id)
	if got := f.due(t); len(got) != 0 {
		t.Fatalf("still due after a sent follow-up: %+v", got)
	}
}

func TestR28_LaterStageClearsDue(t *testing.T) {
	loc := zone(t, "Europe/Warsaw")
	f := newDue(t, loc)
	applied := time.Date(2026, 11, 2, 9, 0, 0, 0, loc)
	for _, s := range []store.Stage{store.StageInterviewing, store.StageOffer, store.StageRejected, store.StageWithdrawn, store.StageLead, store.StageReady} {
		f.applied(t, string(s), s, applied)
	}
	f.now = applied.AddDate(0, 0, 30)
	if got := f.due(t); len(got) != 0 {
		t.Fatalf("rows past (or before) Applied got a nudge: %+v", got)
	}

	// Reopened back to Applied: the clock restarts at the reopen.
	id := f.applied(t, "Reopened", store.StageApplied, applied)
	f.exec(t, `INSERT INTO stage_events (application_id, from_stage, to_stage, cause, reason, at)
		VALUES (?, 'interviewing', 'applied', 'user', 'reopen', ?)`, id, store.FormatTime(applied.AddDate(0, 0, 25)))
	if got := f.due(t); len(got) != 0 {
		t.Fatalf("due 5 days after the latest stage change: %+v", got)
	}
}

func TestR28_DelayConfigurable(t *testing.T) {
	loc := zone(t, "Europe/Warsaw")
	f := newDue(t, loc)
	f.applied(t, "Acme", store.StageApplied, time.Date(2026, 11, 2, 9, 0, 0, 0, loc))
	f.days = 3
	f.now = time.Date(2026, 11, 4, 22, 0, 0, 0, loc)
	if len(f.due(t)) != 0 {
		t.Fatal("due after 2 days with a 3-day delay")
	}
	f.now = time.Date(2026, 11, 5, 8, 0, 0, 0, loc)
	if got := f.due(t); len(got) != 1 || got[0].DaysSince != 3 {
		t.Fatalf("3-day delay: %+v", got)
	}
}

func TestR28_DSTWeekCountsCalendarDays(t *testing.T) {
	warsaw := zone(t, "Europe/Warsaw")
	// Clocks go back on Sunday 25 October 2026: that week has a 25-hour day.
	from := time.Date(2026, 10, 20, 10, 0, 0, 0, warsaw)
	if got := followups.CalendarDays(from, time.Date(2026, 10, 27, 9, 30, 0, 0, warsaw), warsaw); got != 7 {
		t.Fatalf("CalendarDays across the DST change = %d, want 7", got)
	}
	f := newDue(t, warsaw)
	f.applied(t, "Acme", store.StageApplied, from)
	// Just after local midnight on the 27th: 7 calendar days, but only
	// 6 days 15.5 h elapsed (and the UTC date is still the 26th), so a
	// 24-hour-multiple or UTC-date rule would say "not yet".
	f.now = time.Date(2026, 10, 27, 0, 30, 0, 0, warsaw)
	if got := f.due(t); len(got) != 1 {
		t.Fatalf("not due on the 7th local day of a DST week: %+v", got)
	}

	// Days are the user's local dates, not UTC dates: 23:30 in Warsaw is
	// already the next day there.
	a := time.Date(2026, 11, 1, 12, 0, 0, 0, warsaw)
	b := time.Date(2026, 11, 1, 23, 30, 0, 0, warsaw) // 22:30 UTC, same UTC date
	if followups.CalendarDays(a, b, warsaw) != 0 || followups.CalendarDays(a, b.Add(time.Hour), warsaw) != 1 {
		t.Fatal("calendar days must follow the local date")
	}
}

func TestR28_ThankYouDueAtRoundEndAcrossZones(t *testing.T) {
	warsaw, ny := zone(t, "Europe/Warsaw"), zone(t, "America/New_York")
	f := newDue(t, warsaw)
	id := f.applied(t, "Acme", store.StageInterviewing, time.Date(2026, 10, 20, 9, 0, 0, 0, warsaw))
	start := time.Date(2026, 11, 3, 10, 0, 0, 0, ny) // entered in New York; stored as UTC
	round := f.exec(t, `INSERT INTO rounds (application_id, starts_at, zone, duration_min, interviewers, created_at, updated_at)
		VALUES (?, ?, 'America/New_York', 45, '[{"name":"Grace Hopper","title":"Admiral","email":"grace@example.com"},{"name":"Alan Turing"}]', '', '')`, id, store.FormatTime(start))

	f.now = start.Add(44 * time.Minute)
	if got := f.due(t); len(got) != 0 {
		t.Fatalf("thank-you due before the round ended: %+v", got)
	}
	f.now = start.Add(45 * time.Minute)
	got := f.due(t)
	if len(got) != 1 || got[0].Kind != followups.KindThankYou || got[0].RoundID != round || got[0].Interviewer != "Grace Hopper" {
		t.Fatalf("thank-you at the round's end: %+v", got)
	}
	if !got[0].DueSince.Equal(start.Add(45 * time.Minute)) {
		t.Fatalf("due since %v, want the round's end", got[0].DueSince)
	}

	f.exec(t, `UPDATE applications SET stage = 'rejected' WHERE id = ?`, id)
	if got := f.due(t); len(got) != 0 {
		t.Fatalf("thank-you due for a rejected application: %+v", got)
	}
	f.exec(t, `UPDATE applications SET stage = 'interviewing' WHERE id = ?`, id)
	f.exec(t, `INSERT INTO followups (application_id, round_id, kind, status, created_at, resolved_at)
		VALUES (?, ?, 'thank_you', 'skipped', '', '')`, id, round)
	if got := f.due(t); len(got) != 0 {
		t.Fatalf("still due after it was skipped: %+v", got)
	}
}

func TestR28_DeletedApplicationsAreNeverDue(t *testing.T) {
	loc := zone(t, "Europe/Warsaw")
	f := newDue(t, loc)
	id := f.applied(t, "Acme", store.StageApplied, time.Date(2026, 10, 1, 9, 0, 0, 0, loc))
	f.exec(t, `UPDATE applications SET deleted_at = '2026-10-02T00:00:00.000Z' WHERE id = ?`, id)
	f.now = time.Date(2026, 11, 1, 9, 0, 0, 0, loc)
	if got := f.due(t); len(got) != 0 {
		t.Fatalf("deleted row is due: %+v", got)
	}
}
