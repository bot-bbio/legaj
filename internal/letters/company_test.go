package letters_test

import (
	"testing"

	"github.com/bot-bbio/legaj/internal/letters"
)

func TestR21_CompanyNamedWithoutItsLegalSuffixIsAccepted(t *testing.T) {
	f := newFixture(t)
	for _, c := range []struct{ stored, written string }{
		{"Acme, Inc.", "Acme"},
		{"Google LLC", "Google"},
		{"Łódź Software Sp. z o.o.", "Łódź Software"},
	} {
		id := f.app(t, c.stored, "JD")
		before := len(f.ai.Requests())
		f.ai.Push(reply(t, c.written))
		if _, err := f.svc.Draft(ctx, id); err != nil {
			t.Fatalf("%s written as %s: %v", c.stored, c.written, err)
		}
		if n := len(f.ai.Requests()) - before; n != 1 {
			t.Fatalf("%s written as %s took %d requests, want 1", c.stored, c.written, n)
		}
	}
}

func TestR21_AnotherPipelineCompanyIsRejected(t *testing.T) {
	f := newFixture(t)
	f.app(t, "Globex", "JD")
	id := f.app(t, "Acme", "JD")
	f.ai.Push(reply(t, "Acme", func(m map[string]any) {
		m["whyOptions"] = []any{"Acme builds engines.", "Globex's mission is what drew me.", "Acme ships fast."}
	}))
	f.ai.Push(reply(t, "Acme"))
	d, err := f.svc.Draft(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(f.ai.Requests()); n != 2 {
		t.Fatalf("%d requests; a letter naming another pipeline company must be retried", n)
	}
	for _, w := range d.WhyOptions {
		if containsFold(w, "globex") {
			t.Fatalf("accepted a why option about Globex: %q", w)
		}
	}
}

func TestR21_PastEmployersMayBeNamed(t *testing.T) {
	f := newFixture(t)
	// The profile's past employer is also a row in the pipeline.
	f.app(t, "Babbage & Co", "JD")
	id := f.app(t, "Acme", "JD")
	f.ai.Push(reply(t, "Acme")) // its body says "At Babbage & Co I wrote the first algorithm."
	if _, err := f.svc.Draft(ctx, id); err != nil {
		t.Fatal(err)
	}
	if n := len(f.ai.Requests()); n != 1 {
		t.Fatalf("%d requests; naming a past employer isn't naming the wrong company", n)
	}
}

func TestR22_RegeneratedParagraphNamingAnotherCompanyIsRetried(t *testing.T) {
	f := newFixture(t)
	f.app(t, "Globex", "JD")
	id := f.app(t, "Acme", "JD")
	f.draft(t, id, "Acme")
	f.ai.Push(`{"paragraph": "At Globex, the work is the product."}`)
	f.ai.Push(`{"paragraph": "At Acme, the work is the product."}`)
	d, err := f.svc.RegenerateParagraph(ctx, id, letters.PartWhy, 1)
	if err != nil {
		t.Fatal(err)
	}
	if d.WhyOptions[1] != "At Acme, the work is the product." {
		t.Fatalf("why[1] = %q", d.WhyOptions[1])
	}
}

func TestR21_PlaceholderRuleLeavesTechnicalTextAlone(t *testing.T) {
	for _, ok := range []string{
		"I write C++ like std::vector<int> every day.",
		"I cut latency to <10ms and kept uptime >99.9%.",
		"As cited in [1], the method scales.",
	} {
		if m := letters.FindPlaceholder(ok); m != "" {
			t.Errorf("%q flagged %q as a placeholder", ok, m)
		}
	}
	for _, bad := range []string{
		"Dear [Hiring Manager],", "Thank you, <Company>.", "See {{company}}.", "Dear Hiring Manager Name,",
		"I'd love to join [Company Name].", "Regards, <Your Name>",
	} {
		if letters.FindPlaceholder(bad) == "" {
			t.Errorf("%q not flagged", bad)
		}
	}
}
