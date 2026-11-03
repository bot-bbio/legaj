package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bot-bbio/legaj/internal/interviews"
	"github.com/bot-bbio/legaj/internal/pdf"
	"github.com/bot-bbio/legaj/internal/pipeline"
)

func TestR13_DeletingAnApplicationRemovesItsFilesAfterThePurge(t *testing.T) {
	app, _, _ := newTestAppWithLaunches(t)
	c, svc, err := app.services()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	res, err := svc.Pipeline.PasteJob(ctx, pipeline.PasteInput{Company: "Acme", Role: "Dev"})
	if err != nil {
		t.Fatal(err)
	}
	v, err := svc.Documents.WriteLetter(ctx, res.ID, pdf.Letter{Name: "Ada", Salutation: "Dear Hiring Team,",
		Paragraphs: []string{"Hello."}, Closing: "Sincerely,", Signature: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := svc.Interviews.Add(ctx, interviews.RoundInput{AppID: res.ID, Local: "2026-11-03T18:00", Zone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	ics, err := svc.Interviews.Export(ctx, r.ID, c.Dir)
	if err != nil {
		t.Fatal(err)
	}
	letter := filepath.Join(c.Dir, filepath.FromSlash(v.Path))

	if _, err := (&PipelineAPI{app: app}).DeleteApplications([]int64{res.ID}); err != nil {
		t.Fatal(err)
	}
	// Inside the undo window the files stay.
	for _, p := range []string{letter, ics} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("%s removed before the purge: %v", p, err)
		}
	}
	if _, err := svc.Pipeline.PurgeDeleted(ctx); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{letter, ics} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("%s survived the purge: %v", p, err)
		}
	}
}
