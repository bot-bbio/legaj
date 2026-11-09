package followups_test

import (
	"strings"
	"testing"

	"github.com/bot-bbio/legaj/internal/followups"
	"github.com/bot-bbio/legaj/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

func TestR27_PlaceholdersFilled(t *testing.T) {
	v := followups.Values{Company: "AT&T Labs", Role: "R&D Engineer", Interviewer: "Łukasz Żółć", DaysSince: 7, Name: "Ada Lovelace"}

	nudge := followups.Render(followups.DefaultTemplate(followups.KindAppliedNudge), v)
	if len(nudge.Problems) != 0 {
		t.Fatalf("problems = %v", nudge.Problems)
	}
	for _, want := range []string{"AT&T Labs", "R&D Engineer", "7 days", "Ada Lovelace"} {
		if !strings.Contains(nudge.Subject+nudge.Body, want) {
			t.Errorf("nudge is missing %q:\n%s\n%s", want, nudge.Subject, nudge.Body)
		}
	}
	if strings.ContainsAny(nudge.Subject+nudge.Body, "{}") {
		t.Fatalf("a placeholder was left: %s %s", nudge.Subject, nudge.Body)
	}

	thanks := followups.Render(followups.DefaultTemplate(followups.KindThankYou), v)
	if len(thanks.Problems) != 0 || !strings.Contains(thanks.Body, "Dear Łukasz Żółć,") {
		t.Fatalf("thank-you = %+v", thanks)
	}

	one := followups.Render(followups.Template{Body: "{days_since}"}, followups.Values{DaysSince: 1})
	if one.Body != "1 day" {
		t.Fatalf("days_since for 1 = %q, want \"1 day\"", one.Body)
	}
}

func TestR27_UnknownOrEmptyPlaceholderFlagged(t *testing.T) {
	r := followups.Render(followups.Template{
		Subject: "About {role}",
		Body:    "Dear {interviewer}, at {company} the salary is {salary}.",
	}, followups.Values{Company: "Acme"})

	if r.Ready() {
		t.Fatal("a draft with unfilled placeholders must be flagged")
	}
	joined := strings.Join(r.Problems, " | ")
	for _, want := range []string{"{salary}", "{interviewer}", "{role}"} {
		if !strings.Contains(joined, want) {
			t.Errorf("problems %q don't mention %s", joined, want)
		}
		if !strings.Contains(r.Subject+r.Body, want) {
			t.Errorf("%s was blanked instead of left visible", want)
		}
	}
	if !strings.Contains(r.Body, "at Acme") {
		t.Fatalf("known values must still be filled: %q", r.Body)
	}

	// The user edits the draft; whatever placeholders remain are flagged.
	if p := followups.Check("Hi {interviewer}, thanks."); len(p) != 1 {
		t.Fatalf("Check = %v, want one problem", p)
	}
	if p := followups.Check("Hi Grace, thanks. {not a placeholder}"); len(p) != 0 {
		t.Fatalf("Check flagged ordinary braces: %v", p)
	}
	if p := followups.Check("   "); len(p) != 1 {
		t.Fatalf("an empty body must be flagged: %v", p)
	}
}
