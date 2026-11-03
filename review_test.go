package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/bot-bbio/legaj/internal/jobs"
	"github.com/bot-bbio/legaj/internal/letters"
	"github.com/bot-bbio/legaj/internal/pipeline"
	"github.com/bot-bbio/legaj/internal/store"
)

func TestR23_ReviewQueueMergesJobsAndDrafts(t *testing.T) {
	live := map[int64]store.Application{}
	for id := int64(1); id <= 6; id++ {
		live[id] = store.Application{ID: id, Company: fmt.Sprint("Co", id), Role: "Dev"}
	}
	js := []jobs.Job{
		{ID: 1, AppID: 1, Status: jobs.Failed, Error: "Gemini is busy."}, // retried below
		{ID: 2, AppID: 2, Status: jobs.Failed, Error: "The letter doesn't fit on one page."},
		{ID: 3, AppID: 3, Status: jobs.Done},
		{ID: 4, AppID: 4, Status: jobs.Running},
		{ID: 5, AppID: 1, Status: jobs.Queued},
		{ID: 6, AppID: 5, Status: jobs.Failed, Error: "Gemini is unavailable."},
		{ID: 7, AppID: 99, Status: jobs.Running}, // deleted row
		{ID: 8, AppID: 6, Status: jobs.Done},     // approved directly (Skip review): nothing left
	}
	pending := []letters.Pending{{AppID: 2}, {AppID: 3}, {AppID: 4}, {AppID: 99}}

	got := reviewItems(js, pending, live)
	want := []ReviewItem{
		{AppID: 1, Company: "Co1", Role: "Dev", Status: "queued"},
		{AppID: 2, Company: "Co2", Role: "Dev", Status: "draft", Reason: "The letter doesn't fit on one page."},
		{AppID: 3, Company: "Co3", Role: "Dev", Status: "draft"},
		{AppID: 4, Company: "Co4", Role: "Dev", Status: "running"},
		{AppID: 5, Company: "Co5", Role: "Dev", Status: "failed", Reason: "Gemini is unavailable."},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("review items:\n got %+v\nwant %+v", got, want)
	}
}

func TestR23_BulkGenerateSkipsRowsThatDontNeedALetter(t *testing.T) {
	app, _, _ := newTestAppWithLaunches(t)
	_, svc, err := app.services()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	add := func(company string) int64 {
		res, err := svc.Pipeline.PasteJob(ctx, pipeline.PasteInput{Company: company, Role: "Dev"})
		if err != nil {
			t.Fatal(err)
		}
		return res.ID
	}
	lead, applied, answersOnly := add("Acme"), add("Globex"), add("Initech")
	if _, err := svc.Pipeline.SetStage(ctx, []int64{applied}, store.StageApplied); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Pipeline.SetMaterials(ctx, answersOnly, store.MaterialsAnswers); err != nil {
		t.Fatal(err)
	}
	res, err := (&LettersAPI{app: app}).Generate([]int64{lead, applied, answersOnly, 999})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(res.Queued) != fmt.Sprint([]int64{lead}) {
		t.Fatalf("queued %v, want only the Lead that needs a letter", res.Queued)
	}
	skipped := map[int64]string{}
	for _, s := range res.Skipped {
		skipped[s.AppID] = s.Reason
	}
	for _, id := range []int64{applied, answersOnly, 999} {
		if skipped[id] == "" {
			t.Errorf("row %d not reported as skipped: %+v", id, res.Skipped)
		}
	}
}
