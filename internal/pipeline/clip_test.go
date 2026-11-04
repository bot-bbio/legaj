package pipeline_test

import (
	"context"
	"testing"
	"time"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/pipeline"
	"github.com/bot-bbio/legaj/internal/store"
)

func TestR19_AddClipNeedsOnlyTheURL(t *testing.T) {
	sf := newSvc(t)
	res, err := sf.svc.AddClip(ctx, pipeline.ClipInput{URL: "https://jobs.example.com/42", Description: "Build things."})
	if err != nil || !res.Created {
		t.Fatalf("add clip: %+v %v", res, err)
	}
	a, err := sf.svc.Get(ctx, res.ID)
	if err != nil {
		t.Fatal(err)
	}
	// An empty company is the "Needs company" flag (R19); a clip is a Lead (R11).
	if a.Company != "" || a.Source != store.SourceClip || a.Stage != store.StageLead || a.Description != "Build things." {
		t.Fatalf("row %+v", a)
	}
	if a.Materials != store.MaterialsLetter {
		t.Fatalf("materials %q, want the default", a.Materials)
	}
}

func TestR19_AddClipTakesSettingsDefaultMaterials(t *testing.T) {
	f := newFixture(t)
	svc := pipeline.NewService(pipeline.Config{
		DB: f.db, Bus: events.NewBus(&events.Recorder{}), Machine: f.m, Now: func() time.Time { return f.t0 },
		DefaultMaterials: func(context.Context) store.Materials { return store.MaterialsAnswers },
	})
	res, err := svc.AddClip(ctx, pipeline.ClipInput{URL: "https://jobs.example.com/1", Role: "Analyst"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := svc.Get(ctx, res.ID)
	if err != nil || a.Materials != store.MaterialsAnswers {
		t.Fatalf("materials %q, err %v (D28: clips take the Settings default)", a.Materials, err)
	}
}

func TestR19_SameCanonicalURLOneRow(t *testing.T) {
	sf := newSvc(t)
	first, err := sf.svc.AddClip(ctx, pipeline.ClipInput{URL: "https://www.linkedin.com/jobs/view/go-dev-at-acme-4012345678/?refId=1", Company: "Acme", Role: "Go dev"})
	if err != nil || !first.Created {
		t.Fatalf("first: %+v %v", first, err)
	}
	again, err := sf.svc.AddClip(ctx, pipeline.ClipInput{URL: "https://www.linkedin.com/jobs/search/?currentJobId=4012345678", Company: "Acme", Role: "Go dev"})
	if err != nil {
		t.Fatal(err)
	}
	if again.Created || again.DuplicateOf != first.ID || again.ID != first.ID {
		t.Fatalf("second clip of the same posting: %+v", again)
	}
	if ids := sf.liveIDs(t); len(ids) != 1 {
		t.Fatalf("rows %v, want one", ids)
	}
}

func TestR19_AddClipRejectsBadInput(t *testing.T) {
	sf := newSvc(t)
	cases := map[string]pipeline.ClipInput{
		"no url":               {Company: "Acme"},
		"not a web page":       {URL: "file:///C:/jobs.html", Company: "Acme"},
		"script url":           {URL: "javascript:alert(1)", Company: "Acme"},
		"company too long":     {URL: "https://example.com/1", Company: string(make([]byte, 201))},
		"description too long": {URL: "https://example.com/2", Description: string(make([]byte, pipeline.MaxDescription+1))},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := sf.svc.AddClip(ctx, in); apperr.CodeOf(err) != apperr.CodeInvalid {
				t.Fatalf("err %v, want invalid", err)
			}
		})
	}
	if ids := sf.liveIDs(t); len(ids) != 0 {
		t.Fatalf("rows %v after rejected clips", ids)
	}
}

func TestR19_FillMissingKeepsExistingValues(t *testing.T) {
	sf := newSvc(t)
	res, err := sf.svc.AddClip(ctx, pipeline.ClipInput{URL: "https://example.com/1", Role: "Data Analyst"})
	if err != nil {
		t.Fatal(err)
	}
	before := len(sf.rec.Events())
	changed, err := sf.svc.FillMissing(ctx, res.ID, pipeline.Found{Company: "Contoso", Role: "Analyst II", Location: "Remote"})
	if err != nil || !changed {
		t.Fatalf("fill: %v %v", changed, err)
	}
	a, _ := sf.svc.Get(ctx, res.ID)
	if a.Company != "Contoso" || a.Role != "Data Analyst" || a.Location != "Remote" {
		t.Fatalf("row %q / %q / %q: only empty fields may be filled", a.Company, a.Role, a.Location)
	}
	if len(sf.rec.Events()) == before {
		t.Fatal("no change event after filling the row")
	}
	// The row is written because one field is empty: the others must keep
	// their values (the user typed the company while the AI was answering).
	other, err := sf.svc.AddClip(ctx, pipeline.ClipInput{URL: "https://example.com/2", Company: "Typed Co", Role: "Analyst"})
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := sf.svc.FillMissing(ctx, other.ID, pipeline.Found{Company: "Globex", Role: "Lead", Location: "Berlin"}); err != nil || !changed {
		t.Fatalf("partial fill: %v %v", changed, err)
	}
	if a, _ := sf.svc.Get(ctx, other.ID); a.Company != "Typed Co" || a.Role != "Analyst" || a.Location != "Berlin" {
		t.Fatalf("partial fill gave %q / %q / %q", a.Company, a.Role, a.Location)
	}
	placed, err := sf.svc.AddClip(ctx, pipeline.ClipInput{URL: "https://example.com/3", Role: "Analyst", Location: "Paris"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sf.svc.FillMissing(ctx, placed.ID, pipeline.Found{Company: "Contoso", Location: "Berlin"}); err != nil {
		t.Fatal(err)
	}
	if a, _ := sf.svc.Get(ctx, placed.ID); a.Company != "Contoso" || a.Location != "Paris" {
		t.Fatalf("fill gave %q / %q, want the company filled and the location kept", a.Company, a.Location)
	}
	// Nothing left to fill: no write, no event.
	before = len(sf.rec.Events())
	changed, err = sf.svc.FillMissing(ctx, res.ID, pipeline.Found{Company: "Globex", Location: "Berlin"})
	if err != nil || changed {
		t.Fatalf("second fill: %v %v", changed, err)
	}
	if a, _ := sf.svc.Get(ctx, res.ID); a.Company != "Contoso" || a.Location != "Remote" {
		t.Fatalf("second fill overwrote the row: %+v", a)
	}
	if len(sf.rec.Events()) != before {
		t.Fatal("event emitted for a fill that changed nothing")
	}
}

func TestR19_FillMissingIgnoresDeletedRows(t *testing.T) {
	sf := newSvc(t)
	res, err := sf.svc.AddClip(ctx, pipeline.ClipInput{URL: "https://example.com/1", Role: "Analyst"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sf.svc.Delete(ctx, []int64{res.ID}); err != nil {
		t.Fatal(err)
	}
	if changed, err := sf.svc.FillMissing(ctx, res.ID, pipeline.Found{Company: "Contoso"}); err != nil || changed {
		t.Fatalf("fill of a deleted row: %v %v", changed, err)
	}
}

func TestR19_FillMissingRejectsOverlongValues(t *testing.T) {
	sf := newSvc(t)
	res, err := sf.svc.AddClip(ctx, pipeline.ClipInput{URL: "https://example.com/1", Role: "Analyst"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sf.svc.FillMissing(ctx, res.ID, pipeline.Found{Company: string(make([]byte, 201))}); apperr.CodeOf(err) != apperr.CodeInvalid {
		t.Fatalf("err %v, want invalid", err)
	}
}
