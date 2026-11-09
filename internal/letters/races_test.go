package letters_test

import (
	"strings"
	"testing"

	"github.com/bot-bbio/legaj/internal/ai"
	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/letters"
)

func TestR22_SaveAfterApprovalDoesNotResurrectTheDraft(t *testing.T) {
	f := newFixture(t)
	id := f.app(t, "Acme", "JD")
	f.draft(t, id, "Acme")
	d, err := f.svc.CurrentDraft(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Approve(ctx, id); err != nil {
		t.Fatal(err)
	}
	// The editor's blur save lands after Approve committed.
	d.Opening = "I'm applying to Acme, edited."
	if _, err := f.svc.SaveDraft(ctx, id, d); apperr.CodeOf(err) != apperr.CodeConflict {
		t.Fatalf("save after approval: err = %v, want conflict", err)
	}
	if _, err := f.svc.CurrentDraft(ctx, id); apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatal("an approved draft came back for review")
	}
}

func TestR22_SaveRefusesADraftThatWasReplaced(t *testing.T) {
	f := newFixture(t)
	id := f.app(t, "Acme", "JD")
	f.draft(t, id, "Acme")
	old, _ := f.svc.CurrentDraft(ctx, id)
	f.ai.Push(reply(t, "Acme", func(m map[string]any) { m["opening"] = "A newer generation for Acme." }))
	if _, err := f.svc.Draft(ctx, id); err != nil {
		t.Fatal(err)
	}
	old.Opening = "Stale edit of the old draft at Acme."
	if _, err := f.svc.SaveDraft(ctx, id, old); apperr.CodeOf(err) != apperr.CodeConflict {
		t.Fatalf("saving a replaced draft: err = %v, want conflict", err)
	}
	cur, _ := f.svc.CurrentDraft(ctx, id)
	if cur.Opening != "A newer generation for Acme." {
		t.Fatalf("the newer draft was overwritten: %q", cur.Opening)
	}
	// Saving the current draft (its own revision) works.
	cur.Opening = "Edited newer draft at Acme."
	if _, err := f.svc.SaveDraft(ctx, id, cur); err != nil {
		t.Fatal(err)
	}
}

func TestR22_RegenerateDuringApprovalWritesNothing(t *testing.T) {
	f := newFixture(t)
	id := f.app(t, "Acme", "JD")
	f.draft(t, id, "Acme")
	// While Gemini rewrites, the user approves the draft.
	f.ai.Respond = func(ai.Request) (string, error) {
		if _, err := f.svc.Approve(ctx, id); err != nil {
			t.Errorf("approve during regenerate: %v", err)
		}
		return `{"paragraph": "A late rewrite for Acme."}`, nil
	}
	if _, err := f.svc.RegenerateParagraph(ctx, id, letters.PartOpening, 0); apperr.CodeOf(err) != apperr.CodeConflict {
		t.Fatalf("regenerate after approval: err = %v, want conflict", err)
	}
	if _, err := f.svc.CurrentDraft(ctx, id); apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatal("the late rewrite brought an approved draft back")
	}
	if n := f.versions(t, id); n != 1 {
		t.Fatalf("versions = %d, want 1", n)
	}
}

func TestR22_RegenerateKeepsEditsMadeMeanwhile(t *testing.T) {
	f := newFixture(t)
	id := f.app(t, "Acme", "JD")
	f.draft(t, id, "Acme")
	f.ai.Respond = func(ai.Request) (string, error) {
		d, err := f.svc.CurrentDraft(ctx, id)
		if err != nil {
			return "", err
		}
		d.Opening = "Edited while Gemini worked, at Acme."
		if _, err := f.svc.SaveDraft(ctx, id, d); err != nil {
			t.Errorf("save during regenerate: %v", err)
		}
		return `{"paragraph": "Acme's compiler team is where I belong."}`, nil
	}
	got, err := f.svc.RegenerateParagraph(ctx, id, letters.PartWhy, 2)
	if err != nil {
		t.Fatal(err)
	}
	stored, _ := f.svc.CurrentDraft(ctx, id)
	for _, d := range []letters.Draft{got, stored} {
		if d.Opening != "Edited while Gemini worked, at Acme." || d.WhyOptions[2] != "Acme's compiler team is where I belong." {
			t.Fatalf("draft = opening %q, why[2] %q; want both changes kept", d.Opening, d.WhyOptions[2])
		}
	}
}

func TestR21_RegeneratePromptKeepsAmpersands(t *testing.T) {
	f := newFixture(t)
	id := f.app(t, "AT&T", "JD")
	f.ai.Push(reply(t, "AT&T"))
	if _, err := f.svc.Draft(ctx, id); err != nil {
		t.Fatal(err)
	}
	f.ai.Push(`{"paragraph": "AT&T's network is where I want to work."}`)
	if _, err := f.svc.RegenerateParagraph(ctx, id, letters.PartBody, 0); err != nil {
		t.Fatal(err)
	}
	text := allText(f.ai.Requests()[1])
	escaped := "\\" + "u0026" // the JSON escape of "&"
	if strings.Contains(text, escaped) || !strings.Contains(text, "I'm applying for the Engineer role at AT&T.") {
		t.Fatal("the draft context must reach the model unescaped (A10)")
	}
}
