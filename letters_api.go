package main

import (
	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/documents"
	"github.com/bot-bbio/legaj/internal/jobs"
	"github.com/bot-bbio/legaj/internal/letters"
	"github.com/bot-bbio/legaj/internal/store"
)

// LettersAPI is the review editor's binding (R21-R23).
type LettersAPI struct {
	app *App
}

// GetDraft returns an application's draft awaiting review.
func (l *LettersAPI) GetDraft(appID int64) (letters.Draft, error) {
	_, svc, err := l.app.services()
	if err != nil {
		return letters.Draft{}, err
	}
	d, err := svc.Letters.CurrentDraft(l.app.ctx, appID)
	return d, l.app.public("LettersAPI.GetDraft", err)
}

// SaveDraft stores the user's edits (including the chosen why paragraph).
func (l *LettersAPI) SaveDraft(appID int64, d letters.Draft) (letters.Draft, error) {
	_, svc, err := l.app.services()
	if err != nil {
		return letters.Draft{}, err
	}
	out, err := svc.Letters.SaveDraft(l.app.ctx, appID, d)
	return out, l.app.public("LettersAPI.SaveDraft", err)
}

// RegenerateParagraph rewrites one paragraph: part is opening, why, body or
// closing; index picks among the why options or body paragraphs.
func (l *LettersAPI) RegenerateParagraph(appID int64, part string, index int) (letters.Draft, error) {
	_, svc, err := l.app.services()
	if err != nil {
		return letters.Draft{}, err
	}
	d, err := svc.Letters.RegenerateParagraph(l.app.ctx, appID, letters.Part(part), index)
	return d, l.app.public("LettersAPI.RegenerateParagraph", err)
}

// ApproveLetter turns the draft into the application's next PDF version.
func (l *LettersAPI) ApproveLetter(appID int64) (documents.Version, error) {
	_, svc, err := l.app.services()
	if err != nil {
		return documents.Version{}, err
	}
	v, err := svc.Letters.Approve(l.app.ctx, appID)
	return v, l.app.public("LettersAPI.ApproveLetter", err)
}

// ReviewItem is one row of the Review panel (R23): a generation job that
// hasn't produced a draft yet, a draft awaiting review, or a failure.
type ReviewItem struct {
	AppID   int64  `json:"appId"`
	Company string `json:"company"`
	Role    string `json:"role"`
	Status  string `json:"status"` // queued | running | draft | failed
	Reason  string `json:"reason"` // why the latest job failed; "" otherwise
}

// reviewItems merges the session's jobs with the drafts in the DB, one
// item per live application. A queued or running job wins; then a draft
// (carrying the reason if its job failed after drafting, e.g. a Skip-review
// PDF that didn't fit); then a failure. Finished jobs with nothing left to
// review drop out. Items keep the order they first appeared in.
func reviewItems(js []jobs.Job, pending []letters.Pending, live map[int64]store.Application) []ReviewItem {
	latest := map[int64]jobs.Job{}
	var order []int64
	for _, j := range js {
		if _, seen := latest[j.AppID]; !seen {
			order = append(order, j.AppID)
		}
		if prev, ok := latest[j.AppID]; !ok || j.ID > prev.ID {
			latest[j.AppID] = j
		}
	}
	hasDraft := map[int64]bool{}
	for _, p := range pending {
		if !hasDraft[p.AppID] {
			hasDraft[p.AppID] = true
			if _, ok := latest[p.AppID]; !ok {
				order = append(order, p.AppID)
			}
		}
	}
	items := []ReviewItem{}
	for _, id := range order {
		a, ok := live[id]
		if !ok {
			continue // deleted meanwhile
		}
		it := ReviewItem{AppID: id, Company: a.Company, Role: a.Role}
		j, hasJob := latest[id]
		switch {
		case hasJob && (j.Status == jobs.Queued || j.Status == jobs.Running):
			it.Status = string(j.Status)
		case hasDraft[id]:
			it.Status = "draft"
			if hasJob && j.Status == jobs.Failed {
				it.Reason = j.Error
			}
		case hasJob && j.Status == jobs.Failed:
			it.Status, it.Reason = "failed", j.Error
		default:
			continue
		}
		items = append(items, it)
	}
	return items
}

// ReviewQueue lists what the Review panel shows.
func (l *LettersAPI) ReviewQueue() ([]ReviewItem, error) {
	_, svc, err := l.app.services()
	if err != nil {
		return nil, err
	}
	pending, err := svc.Letters.Pending(l.app.ctx)
	if err != nil {
		return nil, l.app.public("LettersAPI.ReviewQueue", err)
	}
	apps, err := svc.Pipeline.List(l.app.ctx)
	if err != nil {
		return nil, l.app.public("LettersAPI.ReviewQueue", err)
	}
	live := make(map[int64]store.Application, len(apps))
	for _, a := range apps {
		live[a.ID] = a
	}
	return reviewItems(svc.Jobs.List(), pending, live), nil
}

// GenerateResult names the applications that got a new generation job;
// one that was already queued or running isn't queued twice (U6).
type GenerateResult struct {
	Queued []int64 `json:"queued"`
}

// maxBatch bounds one bulk request from the UI.
const maxBatch = 1000

// Generate queues letter generation for each application (R23). Drafts are
// made in the background; the Review panel follows their progress.
func (l *LettersAPI) Generate(appIDs []int64) (GenerateResult, error) {
	_, svc, err := l.app.services()
	if err != nil {
		return GenerateResult{}, err
	}
	if len(appIDs) > maxBatch {
		return GenerateResult{}, apperr.New(apperr.CodeInvalid, "Select at most 1,000 applications at a time.")
	}
	res := GenerateResult{Queued: []int64{}}
	for _, j := range svc.Jobs.Enqueue(appIDs) {
		res.Queued = append(res.Queued, j.AppID)
	}
	return res, nil
}

// ApproveAll approves every listed application's draft; each succeeds or
// fails on its own (R23).
func (l *LettersAPI) ApproveAll(appIDs []int64) (letters.ApproveAllResult, error) {
	_, svc, err := l.app.services()
	if err != nil {
		return letters.ApproveAllResult{}, err
	}
	if len(appIDs) > maxBatch {
		return letters.ApproveAllResult{}, apperr.New(apperr.CodeInvalid, "Select at most 1,000 applications at a time.")
	}
	res := svc.Letters.ApproveAll(l.app.ctx, appIDs)
	for _, f := range res.Failed {
		l.app.log().Info("approve failed", "op", "LettersAPI.ApproveAll", "app", f.AppID, "reason", f.Reason)
	}
	return res, nil
}

// Dismiss removes an application's failed generation from the Review panel.
func (l *LettersAPI) Dismiss(appID int64) error {
	_, svc, err := l.app.services()
	if err != nil {
		return err
	}
	svc.Jobs.Dismiss([]int64{appID})
	return nil
}
