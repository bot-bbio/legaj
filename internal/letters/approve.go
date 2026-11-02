package letters

import (
	"context"
	"time"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/documents"
	"github.com/bot-bbio/legaj/internal/store"
)

// Generate is one background job (R23): draft the letter, then, when the
// Skip-review setting is on, approve it straight to a PDF. If that approval
// fails (say the letter doesn't fit on one page) the job fails with the
// reason and the draft stays for review.
func (s *Service) Generate(ctx context.Context, appID int64) error {
	if _, err := s.Draft(ctx, appID); err != nil {
		return err
	}
	if s.cfg.SkipReview == nil || !s.cfg.SkipReview(ctx) {
		return nil
	}
	_, err := s.Approve(ctx, appID)
	return err
}

// Failure is one application an action couldn't complete, with a
// user-safe reason.
type Failure struct {
	AppID  int64  `json:"appId"`
	Reason string `json:"reason"`
}

// ApproveAllResult reports each application separately: one failure never
// stops the others (R23).
type ApproveAllResult struct {
	Approved []documents.Version `json:"approved"`
	Failed   []Failure           `json:"failed"`
}

// ApproveAll approves the draft of each application, in the order given.
func (s *Service) ApproveAll(ctx context.Context, appIDs []int64) ApproveAllResult {
	res := ApproveAllResult{Approved: []documents.Version{}, Failed: []Failure{}}
	seen := map[int64]bool{}
	for _, id := range appIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		v, err := s.Approve(ctx, id)
		if err != nil {
			res.Failed = append(res.Failed, Failure{AppID: id, Reason: apperr.UserMessage(err)})
			continue
		}
		res.Approved = append(res.Approved, v)
	}
	return res
}

// Pending is a draft awaiting review.
type Pending struct {
	AppID   int64     `json:"appId"`
	Company string    `json:"company"`
	Role    string    `json:"role"`
	Updated time.Time `json:"updated"`
}

// Pending lists every live application's draft awaiting review, oldest
// first (the order they were generated in).
func (s *Service) Pending(ctx context.Context) ([]Pending, error) {
	rows, err := s.cfg.DB.Reader().QueryContext(ctx, `SELECT a.id, a.company, a.role, d.updated_at
		FROM documents d JOIN applications a ON a.id = d.application_id
		WHERE d.kind = 'letter' AND d.status = 'draft' AND a.deleted_at IS NULL
		ORDER BY d.updated_at, d.id`)
	if err != nil {
		return nil, readErr(err)
	}
	defer rows.Close()
	out := []Pending{}
	for rows.Next() {
		var p Pending
		var updated string
		if err := rows.Scan(&p.AppID, &p.Company, &p.Role, &updated); err != nil {
			return nil, readErr(err)
		}
		if p.Updated, err = store.ParseTime(updated); err != nil {
			return nil, readErr(err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, readErr(err)
	}
	return out, nil
}

func readErr(err error) error {
	return apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't read the review queue.")
}
