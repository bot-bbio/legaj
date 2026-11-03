package main

import (
	"github.com/bot-bbio/legaj/internal/interviews"
	"github.com/bot-bbio/legaj/internal/store"
)

// InterviewsAPI is the Interviews screen's binding (R30-R32).
type InterviewsAPI struct {
	app *App
}

// RoundView is a round for the UI: flat fields, times as RFC 3339 strings.
type RoundView struct {
	ID           int64               `json:"id"`
	AppID        int64               `json:"appId"`
	Company      string              `json:"company"`
	Role         string              `json:"role"`
	Stage        string              `json:"stage"` // the application's stage
	Local        string              `json:"local"` // wall-clock start in Zone
	Zone         string              `json:"zone"`
	StartsAt     string              `json:"startsAt"`
	EndsAt       string              `json:"endsAt"`
	DurationMin  int                 `json:"durationMin"`
	Kind         string              `json:"kind"`
	Location     string              `json:"location"`
	Link         string              `json:"link"`
	Interviewers []store.Interviewer `json:"interviewers"`
	PrepNotes    string              `json:"prepNotes"`
	PostNotes    string              `json:"postNotes"`
	Outcome      string              `json:"outcome"`
	Questions    []string            `json:"questions"`
}

// RoundLists are the Upcoming and Past lists.
type RoundLists struct {
	Upcoming []RoundView `json:"upcoming"`
	Past     []RoundView `json:"past"`
}

func toRoundView(r interviews.Round) RoundView {
	return RoundView{
		ID: r.ID, AppID: r.AppID, Company: r.Company, Role: r.Role, Stage: r.Stage, Local: r.Local, Zone: r.Zone,
		StartsAt: apiTime(r.StartsAt), EndsAt: apiTime(r.EndsAt), DurationMin: r.DurationMin, Kind: r.Kind,
		Location: r.Location, Link: r.Link, Interviewers: r.Interviewers, PrepNotes: r.PrepNotes,
		PostNotes: r.PostNotes, Outcome: r.Outcome, Questions: r.Questions,
	}
}

func toRoundViews(rs []interviews.Round) []RoundView {
	out := make([]RoundView, 0, len(rs))
	for _, r := range rs {
		out = append(out, toRoundView(r))
	}
	return out
}

// Lists returns the Upcoming and Past rounds (R30).
func (i *InterviewsAPI) Lists() (RoundLists, error) {
	_, svc, err := i.app.services()
	if err != nil {
		return RoundLists{}, err
	}
	l, err := svc.Interviews.Lists(i.app.ctx)
	if err != nil {
		return RoundLists{}, i.app.public("InterviewsAPI.Lists", err)
	}
	return RoundLists{Upcoming: toRoundViews(l.Upcoming), Past: toRoundViews(l.Past)}, nil
}

// Add creates a round; the first one moves an early-stage row to Interviewing.
func (i *InterviewsAPI) Add(in interviews.RoundInput) (RoundView, error) {
	_, svc, err := i.app.services()
	if err != nil {
		return RoundView{}, err
	}
	r, err := svc.Interviews.Add(i.app.ctx, in)
	if err != nil {
		return RoundView{}, i.app.public("InterviewsAPI.Add", err)
	}
	return toRoundView(r), nil
}

// Update replaces a round's details.
func (i *InterviewsAPI) Update(id int64, in interviews.RoundInput) (RoundView, error) {
	_, svc, err := i.app.services()
	if err != nil {
		return RoundView{}, err
	}
	r, err := svc.Interviews.Update(i.app.ctx, id, in)
	if err != nil {
		return RoundView{}, i.app.public("InterviewsAPI.Update", err)
	}
	return toRoundView(r), nil
}

// Delete removes a round.
func (i *InterviewsAPI) Delete(id int64) error {
	_, svc, err := i.app.services()
	if err != nil {
		return err
	}
	return i.app.public("InterviewsAPI.Delete", svc.Interviews.Delete(i.app.ctx, id))
}

// AddToCalendar writes the round's .ics file and opens it with the
// default calendar app (R31).
func (i *InterviewsAPI) AddToCalendar(id int64) error {
	c, svc, err := i.app.services()
	if err != nil {
		return err
	}
	path, err := svc.Interviews.Export(i.app.ctx, id, c.Dir)
	if err != nil {
		return i.app.public("InterviewsAPI.AddToCalendar", err)
	}
	return i.app.public("InterviewsAPI.AddToCalendar", c.Opener.OpenFile(path))
}

// SuggestQuestions drafts 8-12 questions to ask for a round and saves them
// on it (R32). Only applications in Interviewing qualify.
func (i *InterviewsAPI) SuggestQuestions(id int64) (RoundView, error) {
	_, svc, err := i.app.services()
	if err != nil {
		return RoundView{}, err
	}
	r, err := svc.Interviews.SuggestQuestions(i.app.ctx, id)
	if err != nil {
		return RoundView{}, i.app.public("InterviewsAPI.SuggestQuestions", err)
	}
	return toRoundView(r), nil
}
