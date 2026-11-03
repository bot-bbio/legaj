package interviews_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/followups"
	"github.com/bot-bbio/legaj/internal/interviews"
	"github.com/bot-bbio/legaj/internal/pipeline"
	"github.com/bot-bbio/legaj/internal/store"
	"github.com/bot-bbio/legaj/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

var ctx = context.Background()

type fixture struct {
	db  *store.DB
	m   *pipeline.Machine
	svc *interviews.Service
	now time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db, st, err := store.Open(ctx, store.Options{Dir: testenv.Isolate(t)})
	if err != nil || st.Kind != store.StartupReady {
		t.Fatalf("open: %v %+v", err, st)
	}
	t.Cleanup(func() { _ = db.Close() })
	f := &fixture{db: db, now: time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)}
	f.m = pipeline.NewMachine(func() time.Time { return f.now })
	f.svc = interviews.NewService(interviews.Config{
		DB: db, Bus: events.NewBus(&events.Recorder{}), Machine: f.m, Now: func() time.Time { return f.now },
	})
	return f
}

func (f *fixture) app(t *testing.T, s store.Stage) int64 {
	t.Helper()
	var id int64
	if err := f.db.Tx(ctx, func(tx *sql.Tx) error {
		var err error
		id, err = f.m.CreateLead(ctx, tx, pipeline.NewLead{Company: "Acme", Role: "Dev", Source: store.SourcePaste})
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE applications SET stage = ? WHERE id = ?`, string(s), id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *fixture) stage(t *testing.T, id int64) (store.Stage, int) {
	t.Helper()
	var s string
	var n int
	if err := f.db.Reader().QueryRowContext(ctx, `SELECT stage, (SELECT count(*) FROM stage_events WHERE application_id = ?)
		FROM applications WHERE id = ?`, id, id).Scan(&s, &n); err != nil {
		t.Fatal(err)
	}
	return store.Stage(s), n
}

func round(appID int64, local string) interviews.RoundInput {
	return interviews.RoundInput{
		AppID: appID, Local: local, Zone: "America/New_York", DurationMin: 45, Kind: "video",
		Link: "https://meet.example.com/abc", PrepNotes: "Read the JD.",
		Interviewers: []store.Interviewer{{Name: "Grace Hopper", Title: "Admiral", Email: "grace@example.com"}},
	}
}

func TestR30_FirstRoundMovesToInterviewingFromLeadReadyApplied(t *testing.T) {
	f := newFixture(t)
	for _, s := range []store.Stage{store.StageLead, store.StageReady, store.StageApplied} {
		id := f.app(t, s)
		if _, err := f.svc.Add(ctx, round(id, "2026-11-03T18:00")); err != nil {
			t.Fatal(err)
		}
		got, _ := f.stage(t, id)
		if got != store.StageInterviewing {
			t.Errorf("%s row with its first round is %s, want interviewing", s, got)
		}
		var cause, reason string
		if err := f.db.Reader().QueryRowContext(ctx, `SELECT cause, reason FROM stage_events
			WHERE application_id = ? ORDER BY id DESC LIMIT 1`, id).Scan(&cause, &reason); err != nil {
			t.Fatal(err)
		}
		if cause != "auto" || reason != "round_added" {
			t.Errorf("%s: event %s/%s, want auto/round_added", s, cause, reason)
		}
	}
}

func TestR30_RoundOnOfferRowKeepsStage(t *testing.T) {
	f := newFixture(t)
	for _, s := range []store.Stage{store.StageInterviewing, store.StageOffer, store.StageRejected, store.StageWithdrawn} {
		id := f.app(t, s)
		_, before := f.stage(t, id)
		if _, err := f.svc.Add(ctx, round(id, "2026-11-03T18:00")); err != nil {
			t.Fatal(err)
		}
		got, after := f.stage(t, id)
		if got != s || after != before {
			t.Errorf("%s row changed to %s or logged an event", s, got)
		}
	}
}

func TestR30_RoundStoredAsUTCInstantWithZone(t *testing.T) {
	f := newFixture(t)
	id := f.app(t, store.StageApplied)
	r, err := f.svc.Add(ctx, round(id, "2026-11-03T18:00"))
	if err != nil {
		t.Fatal(err)
	}
	// 18:00 in New York on 3 November 2026 (EST, UTC-5) is 23:00 UTC.
	if want := time.Date(2026, 11, 3, 23, 0, 0, 0, time.UTC); !r.StartsAt.Equal(want) || r.Zone != "America/New_York" {
		t.Fatalf("start = %v %s, want %v in America/New_York", r.StartsAt, r.Zone, want)
	}
	if r.Local != "2026-11-03T18:00" || r.DurationMin != 45 || r.Interviewers[0].Email != "grace@example.com" {
		t.Fatalf("round = %+v", r)
	}

	in := round(id, "2026-11-04T09:30")
	in.Zone, in.Outcome, in.PostNotes, in.DurationMin = "Europe/Warsaw", "Passed", "Went well.", 0
	r2, err := f.svc.Update(ctx, r.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 11, 4, 8, 30, 0, 0, time.UTC); !r2.StartsAt.Equal(want) || r2.Outcome != "Passed" || r2.DurationMin != 60 {
		t.Fatalf("updated = %+v (want 08:30Z, default 60 min)", r2)
	}
	if err := f.svc.Delete(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Get(ctx, r.ID); apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("deleted round: %v", err)
	}
	if s, _ := f.stage(t, id); s != store.StageInterviewing {
		t.Fatalf("deleting a round changed the stage to %s", s)
	}
}

func TestR30_InvalidRoundsRejected(t *testing.T) {
	f := newFixture(t)
	id := f.app(t, store.StageApplied)
	bad := map[string]func(*interviews.RoundInput){
		"no zone":        func(r *interviews.RoundInput) { r.Zone = "" },
		"unknown zone":   func(r *interviews.RoundInput) { r.Zone = "Mars/Olympus" },
		"bad time":       func(r *interviews.RoundInput) { r.Local = "tomorrow at six" },
		"bad kind":       func(r *interviews.RoundInput) { r.Kind = "duel" },
		"script link":    func(r *interviews.RoundInput) { r.Link = "javascript:alert(1)" },
		"too long":       func(r *interviews.RoundInput) { r.DurationMin = 24 * 60 },
		"bad email":      func(r *interviews.RoundInput) { r.Interviewers[0].Email = "grace@" },
		"nameless":       func(r *interviews.RoundInput) { r.Interviewers[0].Name = " " },
		"no application": func(r *interviews.RoundInput) { r.AppID = 999 },
	}
	for name, mutate := range bad {
		in := round(id, "2026-11-03T18:00")
		mutate(&in)
		if _, err := f.svc.Add(ctx, in); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestR30_UpcomingPastSorted(t *testing.T) {
	f := newFixture(t) // now = 2 Nov 12:00 UTC
	id := f.app(t, store.StageInterviewing)
	for _, local := range []string{"2026-11-10T09:00", "2026-10-20T09:00", "2026-11-05T09:00", "2026-10-30T09:00", "2026-11-02T07:30"} {
		in := round(id, local)
		in.Zone, in.DurationMin = "UTC", 60
		if _, err := f.svc.Add(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	lists, err := f.svc.Lists(ctx)
	if err != nil {
		t.Fatal(err)
	}
	locals := func(rs []interviews.Round) []string {
		var out []string
		for _, r := range rs {
			out = append(out, r.Local)
		}
		return out
	}
	// Upcoming: soonest first. Past: most recent first. A round that has
	// ended (07:30-08:30 today) is past.
	if got := locals(lists.Upcoming); len(got) != 2 || got[0] != "2026-11-05T09:00" || got[1] != "2026-11-10T09:00" {
		t.Fatalf("upcoming = %v", got)
	}
	if got := locals(lists.Past); len(got) != 3 || got[0] != "2026-11-02T07:30" || got[2] != "2026-10-20T09:00" {
		t.Fatalf("past = %v", got)
	}
	if lists.Upcoming[0].Company != "Acme" {
		t.Fatalf("lists must carry the company: %+v", lists.Upcoming[0])
	}
}

func TestR28_ThankYouDueAtRoundEndNotBefore(t *testing.T) {
	f := newFixture(t)
	id := f.app(t, store.StageApplied)
	in := round(id, "2026-11-03T18:00") // New York, 45 min: ends 23:45 UTC
	if _, err := f.svc.Add(ctx, in); err != nil {
		t.Fatal(err)
	}
	var now time.Time
	fu := followups.NewService(followups.Config{DB: f.db, Now: func() time.Time { return now }})
	end := time.Date(2026, 11, 3, 23, 45, 0, 0, time.UTC)
	now = end.Add(-time.Second)
	if items, _ := fu.Due(ctx); len(items) != 0 {
		t.Fatalf("due before the end: %+v", items)
	}
	now = end
	items, err := fu.Due(ctx)
	if err != nil || len(items) != 1 || items[0].Interviewer != "Grace Hopper" {
		t.Fatalf("due at the end: %+v %v", items, err)
	}
}

func TestR28_RoundInOtherZoneDueAtCorrectLocalTime(t *testing.T) {
	warsaw, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		t.Fatal(err)
	}
	f := newFixture(t)
	id := f.app(t, store.StageApplied)
	in := round(id, "2026-11-03T18:00") // 18:00 New York
	in.DurationMin = 60
	if _, err := f.svc.Add(ctx, in); err != nil {
		t.Fatal(err)
	}
	var now time.Time
	fu := followups.NewService(followups.Config{DB: f.db, Now: func() time.Time { return now }, Loc: func() *time.Location { return warsaw }})
	// It ends at 19:00 New York = 01:00 on 4 November in Warsaw.
	now = time.Date(2026, 11, 4, 0, 59, 0, 0, warsaw)
	if items, _ := fu.Due(ctx); len(items) != 0 {
		t.Fatalf("due at 00:59 Warsaw: %+v", items)
	}
	now = time.Date(2026, 11, 4, 1, 0, 0, 0, warsaw)
	if items, _ := fu.Due(ctx); len(items) != 1 {
		t.Fatalf("not due at 01:00 Warsaw: %+v", items)
	}
}
