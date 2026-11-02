package main

import (
	"github.com/bot-bbio/legaj/internal/apperr"
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
