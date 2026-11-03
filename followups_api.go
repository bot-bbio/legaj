package main

import (
	"github.com/bot-bbio/legaj/internal/followups"
)

// FollowupsAPI is the Follow-ups screen's binding (R27-R29).
type FollowupsAPI struct {
	app *App
}

// DueView is a due follow-up, with times as RFC 3339 strings.
type DueView struct {
	Kind        string `json:"kind"`
	AppID       int64  `json:"appId"`
	RoundID     int64  `json:"roundId"`
	Company     string `json:"company"`
	Role        string `json:"role"`
	Interviewer string `json:"interviewer"`
	DueSince    string `json:"dueSince"`
	DaysSince   int    `json:"daysSince"`
}

func toDueView(d followups.DueItem) DueView {
	return DueView{
		Kind: string(d.Kind), AppID: d.AppID, RoundID: d.RoundID, Company: d.Company, Role: d.Role,
		Interviewer: d.Interviewer, DueSince: apiTime(d.DueSince), DaysSince: d.DaysSince,
	}
}

// Due lists the follow-ups due now (R28); the nav badge shows their count.
func (f *FollowupsAPI) Due() ([]DueView, error) {
	_, svc, err := f.app.services()
	if err != nil {
		return nil, err
	}
	items, err := svc.Followups.Due(f.app.ctx)
	if err != nil {
		return nil, f.app.public("FollowupsAPI.Due", err)
	}
	out := make([]DueView, 0, len(items))
	for _, d := range items {
		out = append(out, toDueView(d))
	}
	return out, nil
}
