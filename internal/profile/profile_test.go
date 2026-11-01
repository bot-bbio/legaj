package profile_test

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/profile"
	"github.com/bot-bbio/legaj/internal/store"
	"github.com/bot-bbio/legaj/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

var ctx = context.Background()

func newService(t *testing.T) (*profile.Service, *events.Recorder) {
	t.Helper()
	db, st, err := store.Open(ctx, store.Options{Dir: testenv.Isolate(t)})
	if err != nil || st.Kind != store.StartupReady {
		t.Fatalf("open: %v %+v", err, st)
	}
	t.Cleanup(func() { _ = db.Close() })
	rec := &events.Recorder{}
	return profile.NewService(db, events.NewBus(rec), time.Now), rec
}

func ym(y, m int) profile.YearMonth { return profile.YearMonth{Year: y, Month: m} }

func sample() profile.Profile {
	return profile.Profile{
		FullName:   "Ada Lovelace",
		Email:      "ada@example.com",
		Phone:      "+44 20 7946 0000",
		Location:   "London",
		TargetRole: "Analytical Engine Programmer",
		Summary:    "Wrote the first algorithm.",
		Links:      []profile.Link{{Label: "Site", URL: "https://example.com"}},
		Experiences: []profile.Experience{
			{Company: "Babbage & Co", Title: "Analyst", Location: "London", Start: ym(1842, 1), End: ym(1843, 9), Bullets: []string{"Translated Menabrea's memoir", "Added notes A-G"}},
		},
		Education: []profile.Education{
			{School: "Home tutoring", Degree: "Mathematics", Location: "London", Details: "Tutored by De Morgan.", Start: ym(1830, 0), End: ym(1835, 0)},
		},
		Skills: []profile.SkillGroup{{Category: "Math", Items: []string{"Calculus", "Bernoulli numbers"}}},
	}
}

func save(t *testing.T, s *profile.Service, p profile.Profile) profile.Profile {
	t.Helper()
	out, err := s.Save(ctx, p)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	return out
}

func get(t *testing.T, s *profile.Service) profile.Profile {
	t.Helper()
	p, err := s.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	return p
}

func TestR9_EmptyProfileHasNoNilLists(t *testing.T) {
	s, _ := newService(t)
	p := get(t, s)
	if p.Experiences == nil || p.Education == nil || p.Skills == nil || p.Links == nil {
		t.Fatalf("nil lists in an empty profile: %+v", p)
	}
}

func TestR9_SaveWithoutSkillsNoCrash(t *testing.T) {
	s, rec := newService(t)
	p := sample()
	p.Skills = nil
	out := save(t, s, p)
	if out.Skills == nil || len(out.Skills) != 0 {
		t.Fatalf("skills = %#v", out.Skills)
	}
	if len(rec.Events()) != 1 {
		t.Fatal("save must emit one profile change (R3)")
	}
}

func TestR9_SaveWithChangedExperienceCount(t *testing.T) {
	s, _ := newService(t)
	first := save(t, s, sample())

	more := first
	more.Experiences = append(append([]profile.Experience{}, first.Experiences...),
		profile.Experience{Company: "Royal Society", Title: "Correspondent", Start: ym(1844, 3)},
		profile.Experience{Company: "Self", Title: "Writer", Start: ym(1845, 0)},
	)
	if got := save(t, s, more); len(got.Experiences) != 3 {
		t.Fatalf("after adding: %d experiences, want 3", len(got.Experiences))
	}

	fewer := get(t, s)
	fewer.Experiences = fewer.Experiences[1:2] // keep only the middle one
	got := save(t, s, fewer)
	if len(got.Experiences) != 1 || got.Experiences[0].Company != "Royal Society" {
		t.Fatalf("after removing: %+v", got.Experiences)
	}
	if !reflect.DeepEqual(get(t, s).Experiences, got.Experiences) {
		t.Fatal("stored experiences differ from what Save returned")
	}
}

func TestR9_EducationLocationDetailsPreserved(t *testing.T) {
	s, _ := newService(t)
	save(t, s, sample())
	edu := get(t, s).Education
	if len(edu) != 1 || edu[0].Location != "London" || edu[0].Details != "Tutored by De Morgan." {
		t.Fatalf("education = %+v", edu)
	}
}

func TestR9_HyphenatedDatesRoundTrip(t *testing.T) {
	s, _ := newService(t)
	save(t, s, sample())
	e := get(t, s).Experiences[0]
	if e.Start != ym(1842, 1) || e.End != ym(1843, 9) {
		t.Fatalf("dates = %+v - %+v", e.Start, e.End)
	}
	if e.Start.String() != "1842-01" || e.End.String() != "1843-09" {
		t.Fatalf("formatted = %s / %s", e.Start, e.End)
	}
	for _, in := range []string{"1842-01", "1843-09", "2026", ""} {
		got, err := profile.ParseYearMonth(in)
		if err != nil || got.String() != in {
			t.Errorf("ParseYearMonth(%q) = %v, %v; must round-trip", in, got, err)
		}
	}
	for _, bad := range []string{"1842-13", "18-01", "1842/01", "Jan 1842", "1842-01-05"} {
		if _, err := profile.ParseYearMonth(bad); err == nil {
			t.Errorf("ParseYearMonth(%q) accepted", bad)
		}
	}
}

func TestR9_EmptyBulletsDropped(t *testing.T) {
	s, _ := newService(t)
	p := sample()
	p.Experiences[0].Bullets = []string{"", "  Real work  ", "\t", "More work"}
	p.Skills[0].Items = []string{"Calculus", " ", ""}
	p.Skills = append(p.Skills, profile.SkillGroup{Category: "  ", Items: []string{" "}})
	out := save(t, s, p)
	if got := out.Experiences[0].Bullets; !reflect.DeepEqual(got, []string{"Real work", "More work"}) {
		t.Fatalf("bullets = %q", got)
	}
	if len(out.Skills) != 1 || !reflect.DeepEqual(out.Skills[0].Items, []string{"Calculus"}) {
		t.Fatalf("skills = %+v", out.Skills)
	}
}

func TestR9_ItemsGetStableIDs(t *testing.T) {
	s, _ := newService(t)
	first := save(t, s, sample())
	id := first.Experiences[0].ID
	if id == "" || first.Education[0].ID == "" {
		t.Fatal("list items need IDs for the UI")
	}
	again := save(t, s, first)
	if again.Experiences[0].ID != id {
		t.Fatal("an existing item's ID changed on save")
	}
}

func TestR7_TargetRoleSavedAsEntered(t *testing.T) {
	s, _ := newService(t)
	p := sample()
	p.TargetRole = "  Junior Data Engineer (remote, EU)  "
	if got := save(t, s, p).TargetRole; got != "Junior Data Engineer (remote, EU)" {
		t.Fatalf("target role = %q", got)
	}
}

func TestR9_InvalidProfilesRejected(t *testing.T) {
	cases := map[string]func(*profile.Profile){
		"end before start": func(p *profile.Profile) { p.Experiences[0].End = ym(1841, 1) },
		"current with end": func(p *profile.Profile) { p.Experiences[0].Current = true },
		"month 13":         func(p *profile.Profile) { p.Experiences[0].Start = ym(1842, 13) },
		"bad email":        func(p *profile.Profile) { p.Email = "not-an-email" },
		"huge bullet":      func(p *profile.Profile) { p.Experiences[0].Bullets = []string{strings.Repeat("x", 3000)} },
		"too many jobs": func(p *profile.Profile) {
			p.Experiences = nil
			for range 51 {
				p.Experiences = append(p.Experiences, profile.Experience{Company: "Acme", Title: "Dev"})
			}
		},
		"blank experience":   func(p *profile.Profile) { p.Experiences = append(p.Experiences, profile.Experience{}) },
		"non-http link":      func(p *profile.Profile) { p.Links = []profile.Link{{Label: "x", URL: "javascript:alert(1)"}} },
		"month without year": func(p *profile.Profile) { p.Education[0].Start = ym(0, 5) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s, rec := newService(t)
			p := sample()
			mutate(&p)
			_, err := s.Save(ctx, p)
			if apperr.CodeOf(err) != apperr.CodeInvalid {
				t.Fatalf("err = %v, want invalid", err)
			}
			if len(rec.Events()) != 0 {
				t.Fatal("rejected save emitted a change")
			}
		})
	}
}
