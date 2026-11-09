package pipeline

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/bot-bbio/legaj/internal/store"
)

// recentCount is how many new applications the header lists (R14).
const recentCount = 5

// Stats is the Pipeline header (R14).
type Stats struct {
	// Counts has an entry for every stage, zero included.
	Counts map[store.Stage]int `json:"counts"`
	Total  int                 `json:"total"`
	// Recent is the newest applications by creation time (A14).
	Recent []Recent `json:"recent"`
	// NextInterview is the earliest round still ahead, or nil.
	NextInterview *Upcoming `json:"nextInterview"`
}

// Recent is one recently added application.
type Recent struct {
	ID        int64       `json:"id"`
	Company   string      `json:"company"`
	Role      string      `json:"role"`
	Stage     store.Stage `json:"stage"`
	CreatedAt time.Time   `json:"createdAt"`
}

// Upcoming is the next interview round.
type Upcoming struct {
	ApplicationID int64     `json:"applicationId"`
	RoundID       int64     `json:"roundId"`
	Company       string    `json:"company"`
	Role          string    `json:"role"`
	StartsAt      time.Time `json:"startsAt"`
	Zone          string    `json:"zone"`
	Kind          string    `json:"kind"`
}

// Stats computes the header over live applications only.
func (s *Service) Stats(ctx context.Context) (Stats, error) {
	r := s.cfg.DB.Reader()
	st := Stats{Counts: map[store.Stage]int{}, Recent: []Recent{}}
	for _, stage := range store.Stages {
		st.Counts[stage] = 0
	}

	rows, err := r.QueryContext(ctx, `SELECT stage, count(*) FROM applications WHERE deleted_at IS NULL GROUP BY stage`)
	if err != nil {
		return Stats{}, readErr(err)
	}
	for rows.Next() {
		var stage string
		var n int
		if err := rows.Scan(&stage, &n); err != nil {
			rows.Close()
			return Stats{}, readErr(err)
		}
		st.Counts[store.Stage(stage)] = n
		st.Total += n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Stats{}, readErr(err)
	}

	rows, err = r.QueryContext(ctx, `SELECT id, company, role, stage, created_at FROM applications
		WHERE deleted_at IS NULL ORDER BY created_at DESC, id DESC LIMIT ?`, recentCount)
	if err != nil {
		return Stats{}, readErr(err)
	}
	for rows.Next() {
		var rec Recent
		var stage, created string
		if err := rows.Scan(&rec.ID, &rec.Company, &rec.Role, &stage, &created); err != nil {
			rows.Close()
			return Stats{}, readErr(err)
		}
		rec.Stage = store.Stage(stage)
		if rec.CreatedAt, err = store.ParseTime(created); err != nil {
			rows.Close()
			return Stats{}, readErr(err)
		}
		st.Recent = append(st.Recent, rec)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Stats{}, readErr(err)
	}

	var up Upcoming
	var starts string
	err = r.QueryRowContext(ctx, `SELECT a.id, r.id, a.company, a.role, r.starts_at, r.zone, r.kind
		FROM rounds r JOIN applications a ON a.id = r.application_id
		WHERE a.deleted_at IS NULL AND r.starts_at > ?
		ORDER BY r.starts_at, r.id LIMIT 1`, store.FormatTime(s.cfg.Now())).
		Scan(&up.ApplicationID, &up.RoundID, &up.Company, &up.Role, &starts, &up.Zone, &up.Kind)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return Stats{}, readErr(err)
	default:
		if up.StartsAt, err = store.ParseTime(starts); err != nil {
			return Stats{}, readErr(err)
		}
		st.NextInterview = &up
	}
	return st, nil
}
