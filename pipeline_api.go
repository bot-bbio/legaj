package main

import (
	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/pipeline"
	"github.com/bot-bbio/legaj/internal/store"
)

// PipelineAPI is the Pipeline screen's binding (R12-R17).
type PipelineAPI struct {
	app *App
}

// ApplicationRow is one Pipeline table row. The job description is left
// out to keep the list small; the detail view loads it.
type ApplicationRow struct {
	ID        int64  `json:"id"`
	Company   string `json:"company"`
	Role      string `json:"role"`
	Location  string `json:"location"`
	URL       string `json:"url"`
	Stage     string `json:"stage"`
	Source    string `json:"source"`
	CreatedAt string `json:"createdAt"` // RFC 3339, UTC
	AppliedAt string `json:"appliedAt"` // "" when not applied
}

const rfc3339 = "2006-01-02T15:04:05Z"

func toRow(a store.Application) ApplicationRow {
	r := ApplicationRow{
		ID: a.ID, Company: a.Company, Role: a.Role, Location: a.Location, URL: a.URL,
		Stage: string(a.Stage), Source: string(a.Source), CreatedAt: a.CreatedAt.UTC().Format(rfc3339),
	}
	if a.AppliedAt != nil {
		r.AppliedAt = a.AppliedAt.UTC().Format(rfc3339)
	}
	return r
}

// SetStage moves exactly the given applications to a stage (R12).
func (p *PipelineAPI) SetStage(ids []int64, stage string) (pipeline.StageResult, error) {
	_, svc, err := p.app.services()
	if err != nil {
		return pipeline.StageResult{}, err
	}
	res, err := svc.Pipeline.SetStage(p.app.ctx, ids, store.Stage(stage))
	return res, apperr.Public(err)
}

// DeleteApplications soft-deletes exactly the given applications; the
// result's UndoID undoes it for 10 seconds (R13).
func (p *PipelineAPI) DeleteApplications(ids []int64) (pipeline.DeleteResult, error) {
	_, svc, err := p.app.services()
	if err != nil {
		return pipeline.DeleteResult{}, err
	}
	res, err := svc.Pipeline.Delete(p.app.ctx, ids)
	return res, apperr.Public(err)
}

// UndoDelete restores a delete within its undo window.
func (p *PipelineAPI) UndoDelete(undoID string) (pipeline.UndoResult, error) {
	_, svc, err := p.app.services()
	if err != nil {
		return pipeline.UndoResult{}, err
	}
	res, err := svc.Pipeline.Undo(p.app.ctx, undoID)
	return res, apperr.Public(err)
}

// StatsView is the Pipeline header (R14), with times as RFC 3339 strings.
type StatsView struct {
	Counts        map[string]int  `json:"counts"`
	Total         int             `json:"total"`
	Recent        []RecentView    `json:"recent"`
	NextInterview *InterviewBrief `json:"nextInterview"`
}

// RecentView is one recently added application.
type RecentView struct {
	ID        int64  `json:"id"`
	Company   string `json:"company"`
	Role      string `json:"role"`
	Stage     string `json:"stage"`
	CreatedAt string `json:"createdAt"`
}

// InterviewBrief is the next interview round.
type InterviewBrief struct {
	ApplicationID int64  `json:"applicationId"`
	Company       string `json:"company"`
	Role          string `json:"role"`
	StartsAt      string `json:"startsAt"`
	Zone          string `json:"zone"`
	Kind          string `json:"kind"`
}

// PipelineStats returns per-stage counts, the newest 5 and the next interview.
func (p *PipelineAPI) PipelineStats() (StatsView, error) {
	_, svc, err := p.app.services()
	if err != nil {
		return StatsView{}, err
	}
	st, err := svc.Pipeline.Stats(p.app.ctx)
	if err != nil {
		return StatsView{}, apperr.Public(err)
	}
	v := StatsView{Counts: map[string]int{}, Total: st.Total, Recent: []RecentView{}}
	for k, n := range st.Counts {
		v.Counts[string(k)] = n
	}
	for _, r := range st.Recent {
		v.Recent = append(v.Recent, RecentView{ID: r.ID, Company: r.Company, Role: r.Role, Stage: string(r.Stage), CreatedAt: r.CreatedAt.UTC().Format(rfc3339)})
	}
	if n := st.NextInterview; n != nil {
		v.NextInterview = &InterviewBrief{ApplicationID: n.ApplicationID, Company: n.Company, Role: n.Role, StartsAt: n.StartsAt.UTC().Format(rfc3339), Zone: n.Zone, Kind: n.Kind}
	}
	return v, nil
}

// ListApplications returns every live application, newest first.
func (p *PipelineAPI) ListApplications() ([]ApplicationRow, error) {
	_, svc, err := p.app.services()
	if err != nil {
		return nil, err
	}
	apps, err := svc.Pipeline.List(p.app.ctx)
	if err != nil {
		return nil, apperr.Public(err)
	}
	rows := make([]ApplicationRow, len(apps))
	for i, a := range apps {
		rows[i] = toRow(a)
	}
	return rows, nil
}
