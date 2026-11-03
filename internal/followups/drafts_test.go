package followups_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/followups"
	"github.com/bot-bbio/legaj/internal/profile"
	"github.com/bot-bbio/legaj/internal/store"
)

// withProfile rebuilds the fixture's service with a saved profile name.
func (f *dueFixture) withProfile(t *testing.T, loc *time.Location) {
	t.Helper()
	bus := events.NewBus(&events.Recorder{})
	prof := profile.NewService(f.db, bus, time.Now)
	if _, err := prof.Save(ctx, profile.Profile{FullName: "Ada Lovelace"}); err != nil {
		t.Fatal(err)
	}
	f.svc = followups.NewService(followups.Config{
		DB: f.db, Bus: bus, Profile: prof,
		Now:  func() time.Time { return f.now },
		Loc:  func() *time.Location { return loc },
		Days: func(_ context.Context) int { return f.days },
	})
}

func TestR27_DraftFilledFromTheApplication(t *testing.T) {
	loc := zone(t, "Europe/Warsaw")
	f := newDue(t, loc)
	f.withProfile(t, loc)
	app := f.applied(t, "AT&T Labs", store.StageApplied, time.Date(2026, 11, 2, 9, 0, 0, 0, loc))
	f.now = time.Date(2026, 11, 10, 9, 0, 0, 0, loc)

	d, err := f.svc.Draft(ctx, followups.KindAppliedNudge, app, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Problems) != 0 || !strings.Contains(d.Body, "AT&T Labs") || !strings.Contains(d.Body, "8 days") || !strings.Contains(d.Body, "Ada Lovelace") {
		t.Fatalf("nudge draft = %+v", d)
	}

	round := f.exec(t, `INSERT INTO rounds (application_id, starts_at, zone, duration_min, interviewers, created_at, updated_at)
		VALUES (?, ?, 'Europe/Warsaw', 60, '[]', '', '')`, app, store.FormatTime(f.now.Add(-2*time.Hour)))
	ty, err := f.svc.Draft(ctx, followups.KindThankYou, app, round)
	if err != nil {
		t.Fatal(err)
	}
	if len(ty.Problems) == 0 || !strings.Contains(ty.Body, "{interviewer}") {
		t.Fatalf("a thank-you without an interviewer name must be flagged, not blank: %+v", ty)
	}

	other := f.applied(t, "Globex", store.StageApplied, f.now)
	if _, err := f.svc.Draft(ctx, followups.KindThankYou, other, round); apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("a round of another application: err = %v", err)
	}
	if _, err := f.svc.Draft(ctx, "postcard", app, 0); apperr.CodeOf(err) != apperr.CodeInvalid {
		t.Fatalf("unknown kind: err = %v", err)
	}
}

func TestR29_SaveDraftKeepsEditsAndRechecks(t *testing.T) {
	loc := zone(t, "Europe/Warsaw")
	f := newDue(t, loc)
	f.withProfile(t, loc)
	app := f.applied(t, "Acme", store.StageApplied, time.Date(2026, 11, 2, 9, 0, 0, 0, loc))
	f.now = time.Date(2026, 11, 10, 9, 0, 0, 0, loc)

	in := followups.DraftInput{Kind: followups.KindAppliedNudge, AppID: app, To: "grace@example.com", Subject: "Checking in", Body: "Hello {interviewer}, any news?"}
	d, err := f.svc.SaveDraft(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if d.Subject != "Checking in" || d.To != "grace@example.com" || len(d.Problems) != 1 {
		t.Fatalf("saved draft = %+v", d)
	}
	again, err := f.svc.Draft(ctx, followups.KindAppliedNudge, app, 0)
	if err != nil {
		t.Fatal(err)
	}
	if again.Body != in.Body || again.To != in.To {
		t.Fatalf("the edit wasn't kept: %+v", again)
	}
	in.Body = "Hello, any news?"
	if _, err := f.svc.SaveDraft(ctx, in); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := f.db.Reader().QueryRowContext(ctx, `SELECT count(*) FROM followups WHERE application_id = ?`, app).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("%d draft rows, want 1", n)
	}

	in.To = "a@b.com?bcc=x@y.com"
	if _, err := f.svc.SaveDraft(ctx, in); apperr.CodeOf(err) != apperr.CodeInvalid {
		t.Fatalf("bad recipient saved: %v", err)
	}
}

func TestR29_ComposeRefusesFlaggedDrafts(t *testing.T) {
	loc := zone(t, "Europe/Warsaw")
	f := newDue(t, loc)
	f.withProfile(t, loc)
	app := f.applied(t, "Acme", store.StageApplied, time.Date(2026, 11, 2, 9, 0, 0, 0, loc))
	f.now = time.Date(2026, 11, 10, 9, 0, 0, 0, loc)

	in := followups.DraftInput{Kind: followups.KindAppliedNudge, AppID: app, Subject: "Hi", Body: "Dear {interviewer},"}
	if _, err := f.svc.Compose(ctx, in, followups.Gmail); apperr.CodeOf(err) != apperr.CodeInvalid {
		t.Fatalf("flagged draft was composed: %v", err)
	}
	in.Body = "Dear Grace,\nThanks & regards"
	u, err := f.svc.Compose(ctx, in, followups.Gmail)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(u, "https://mail.google.com/") || !strings.Contains(u, "Thanks%20%26%20regards") {
		t.Fatalf("compose URL = %s", u)
	}
	if d, _ := f.svc.Draft(ctx, followups.KindAppliedNudge, app, 0); d.Body != in.Body {
		t.Fatal("Compose must save the draft it opens")
	}
}

func TestR29_LoggingSentOrSkippedClearsDue(t *testing.T) {
	loc := zone(t, "Europe/Warsaw")
	f := newDue(t, loc)
	f.withProfile(t, loc)
	sent := f.applied(t, "Acme", store.StageApplied, time.Date(2026, 11, 2, 9, 0, 0, 0, loc))
	skipped := f.applied(t, "Globex", store.StageApplied, time.Date(2026, 11, 2, 9, 0, 0, 0, loc))
	f.now = time.Date(2026, 11, 10, 9, 0, 0, 0, loc)
	if len(f.due(t)) != 2 {
		t.Fatal("setup: both rows should be due")
	}

	msg := followups.DraftInput{Kind: followups.KindAppliedNudge, AppID: sent, Subject: "Hi", Body: "Any news?"}
	if _, err := f.svc.SaveDraft(ctx, msg); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Log(ctx, msg, followups.StatusSent); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Log(ctx, followups.DraftInput{Kind: followups.KindAppliedNudge, AppID: skipped}, followups.StatusSkipped); err != nil {
		t.Fatal(err)
	}
	if got := f.due(t); len(got) != 0 {
		t.Fatalf("still due after logging: %+v", got)
	}
	var status, body, resolved string
	if err := f.db.Reader().QueryRowContext(ctx, `SELECT status, body, coalesce(resolved_at, '') FROM followups
		WHERE application_id = ?`, sent).Scan(&status, &body, &resolved); err != nil {
		t.Fatal(err)
	}
	if status != "sent" || body != "Any news?" || resolved == "" {
		t.Fatalf("logged row = %s %q %q", status, body, resolved)
	}
	if err := f.svc.Log(ctx, msg, "draft"); apperr.CodeOf(err) != apperr.CodeInvalid {
		t.Fatalf("logging a non-final status: %v", err)
	}
}

func TestR29_ThankYouGoesToTheInterviewer(t *testing.T) {
	loc := zone(t, "Europe/Warsaw")
	f := newDue(t, loc)
	f.withProfile(t, loc)
	app := f.applied(t, "Acme", store.StageInterviewing, time.Date(2026, 11, 2, 9, 0, 0, 0, loc))
	f.now = time.Date(2026, 11, 10, 9, 0, 0, 0, loc)
	round := f.exec(t, `INSERT INTO rounds (application_id, starts_at, zone, duration_min, interviewers, created_at, updated_at)
		VALUES (?, ?, 'Europe/Warsaw', 60, '[{"name":"Grace Hopper","title":"Admiral","email":"grace@example.com"}]', '', '')`,
		app, store.FormatTime(f.now.Add(-2*time.Hour)))
	d, err := f.svc.Draft(ctx, followups.KindThankYou, app, round)
	if err != nil {
		t.Fatal(err)
	}
	if d.To != "grace@example.com" || !strings.Contains(d.Body, "Dear Grace Hopper,") || len(d.Problems) != 0 {
		t.Fatalf("thank-you draft = %+v", d)
	}
}
