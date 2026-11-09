package pipeline_test

import (
	"strings"
	"testing"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/pipeline"
	"github.com/bot-bbio/legaj/internal/store"
)

func TestR15_CanonicalURL(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"empty is no URL", "", ""},
		{"whitespace trimmed", "  https://jobs.lever.co/acme/abc-123  ", "https://jobs.lever.co/acme/abc-123"},
		{"utm and fragment dropped", "https://boards.greenhouse.io/acme/jobs/42?utm_source=x&utm_medium=y#apply", "https://boards.greenhouse.io/acme/jobs/42"},
		{"trailing slash", "https://jobs.lever.co/acme/abc-123/", "https://jobs.lever.co/acme/abc-123"},
		{"host case, www, default port", "https://WWW.Example.COM:443/Careers/Job-7", "https://example.com/Careers/Job-7"},
		{"http folded into https", "http://example.com/jobs/7", "https://example.com/jobs/7"},
		{"greenhouse job id kept, trackers dropped", "https://acme.com/careers?gh_jid=987&gh_src=abc&fbclid=zz", "https://acme.com/careers?gh_jid=987"},
		{"remaining params sorted", "https://example.com/job?b=2&a=1&gclid=x", "https://example.com/job?a=1&b=2"},
		{"linkedin view with slug and refId", "https://www.linkedin.com/jobs/view/go-developer-at-acme-4012345678/?refId=abc&trackingId=def", "https://linkedin.com/jobs/view/4012345678"},
		{"linkedin plain view", "https://pl.linkedin.com/jobs/view/4012345678", "https://linkedin.com/jobs/view/4012345678"},
		{"linkedin currentJobId on search", "https://www.linkedin.com/jobs/search/?currentJobId=4012345678&keywords=go&geoId=1", "https://linkedin.com/jobs/view/4012345678"},
		{"linkedin currentJobId on collections", "https://www.linkedin.com/jobs/collections/recommended/?currentJobId=4012345678", "https://linkedin.com/jobs/view/4012345678"},
		{"indeed viewjob", "https://www.indeed.com/viewjob?jk=a1b2c3d4e5f6&from=serp&tk=xyz", "https://indeed.com/viewjob?jk=a1b2c3d4e5f6"},
		{"indeed vjk on search page", "https://uk.indeed.com/jobs?q=go&l=London&vjk=a1b2c3d4e5f6", "https://uk.indeed.com/viewjob?jk=a1b2c3d4e5f6"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := pipeline.CanonicalURL(c.in)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if got != c.want {
				t.Fatalf("CanonicalURL(%q)\n got %q\nwant %q", c.in, got, c.want)
			}
			// Canonical form is a fixed point.
			again, err := pipeline.CanonicalURL(got)
			if err != nil || again != got {
				t.Fatalf("not idempotent: %q -> %q (%v)", got, again, err)
			}
		})
	}
}

func TestR15_CanonicalURLRejectsNonWebAddresses(t *testing.T) {
	for _, in := range []string{"javascript:alert(1)", "file:///C:/x", "ftp://example.com/a", "not a url", "https://", "mailto:hr@example.com", "https://" + strings.Repeat("a", 3000) + ".com"} {
		if _, err := pipeline.CanonicalURL(in); apperr.CodeOf(err) != apperr.CodeInvalid {
			t.Errorf("CanonicalURL(%q) err = %v, want invalid", in, err)
		}
	}
}

func TestR15_PasteCreatesLeadWithDescription(t *testing.T) {
	sf := newSvc(t)
	res, err := sf.svc.PasteJob(ctx, pipeline.PasteInput{
		URL: "https://jobs.lever.co/acme/abc?utm_source=li", Company: " Acme ", Role: "Go developer", Location: "Remote",
		Description: "We build things.\nRequirements: Go.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Created || res.ID == 0 || res.DuplicateOf != 0 {
		t.Fatalf("result = %+v", res)
	}
	app, err := sf.svc.Get(ctx, res.ID)
	if err != nil {
		t.Fatal(err)
	}
	if app.Stage != store.StageLead || app.Source != store.SourcePaste || app.Company != "Acme" ||
		app.CanonicalURL != "https://jobs.lever.co/acme/abc" || app.URL != "https://jobs.lever.co/acme/abc?utm_source=li" ||
		app.Description != "We build things.\nRequirements: Go." {
		t.Fatalf("app = %+v", app)
	}
}

func TestR15_DuplicateOffersExisting(t *testing.T) {
	sf := newSvc(t)
	first, err := sf.svc.PasteJob(ctx, pipeline.PasteInput{URL: "https://www.linkedin.com/jobs/view/go-dev-at-acme-4012345678/?refId=1", Company: "Acme", Role: "Go dev"})
	if err != nil {
		t.Fatal(err)
	}
	before := len(sf.liveIDs(t))

	dup, err := sf.svc.PasteJob(ctx, pipeline.PasteInput{URL: "https://www.linkedin.com/jobs/search/?currentJobId=4012345678&keywords=go", Company: "Acme Inc", Role: "Go developer"})
	if err != nil {
		t.Fatal(err)
	}
	if dup.Created || dup.DuplicateOf != first.ID {
		t.Fatalf("result = %+v, want the existing row %d offered instead", dup, first.ID)
	}
	if len(sf.liveIDs(t)) != before {
		t.Fatal("a duplicate row was created")
	}
}

func TestR15_PasteWithoutURLNeverDuplicates(t *testing.T) {
	sf := newSvc(t)
	for range 2 {
		res, err := sf.svc.PasteJob(ctx, pipeline.PasteInput{Company: "Referral Co", Role: "Dev", Description: "From a friend."})
		if err != nil || !res.Created {
			t.Fatalf("res = %+v, %v", res, err)
		}
	}
}

func TestR15_PasteValidation(t *testing.T) {
	sf := newSvc(t)
	cases := map[string]pipeline.PasteInput{
		"no company or role": {URL: "https://example.com/j/1", Description: "x"},
		"bad url":            {URL: "javascript:alert(1)", Company: "Acme"},
		"huge description":   {Company: "Acme", Description: strings.Repeat("x", pipeline.MaxDescription+1)},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := sf.svc.PasteJob(ctx, in); apperr.CodeOf(err) != apperr.CodeInvalid {
				t.Fatalf("err = %v, want invalid", err)
			}
		})
	}
	if len(sf.liveIDs(t)) != 0 {
		t.Fatal("an invalid paste created a row")
	}
}
