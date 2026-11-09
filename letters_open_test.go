package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/pdf"
	"github.com/bot-bbio/legaj/internal/pipeline"
)

func TestR22_OpenTheApprovedLetterAndItsFolder(t *testing.T) {
	app, _, opened := newTestAppWithLaunches(t)
	c, svc, err := app.services()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	res, err := svc.Pipeline.PasteJob(ctx, pipeline.PasteInput{Company: "Acme", Role: "Dev"})
	if err != nil {
		t.Fatal(err)
	}
	api := &LettersAPI{app: app}
	if err := api.OpenLetter(res.ID); apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("no letter yet: err = %v, want not found", err)
	}
	l := pdf.Letter{Name: "Ada", Salutation: "Dear Hiring Team,", Paragraphs: []string{"Hello."}, Closing: "Sincerely,", Signature: "Ada"}
	if _, err := svc.Documents.WriteLetter(ctx, res.ID, l); err != nil {
		t.Fatal(err)
	}
	v2, err := svc.Documents.WriteLetter(ctx, res.ID, l)
	if err != nil {
		t.Fatal(err)
	}
	if err := api.OpenLetter(res.ID); err != nil {
		t.Fatal(err)
	}
	if err := api.ShowLetterFolder(res.ID); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(c.Dir, filepath.FromSlash(v2.Path))
	if len(opened.targets) != 2 || opened.targets[0] != want || opened.targets[1] != filepath.Dir(want) {
		t.Fatalf("opened %v; want the latest version %s, then its folder", opened.targets, want)
	}

	rows, err := (&PipelineAPI{app: app}).ListApplications()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].LetterVersion != 2 {
		t.Fatalf("rows = %+v, want letterVersion 2", rows)
	}
}
