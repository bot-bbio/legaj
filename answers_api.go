package main

import (
	"github.com/bot-bbio/legaj/internal/answers"
)

// AnswersAPI is the binding for form answers and the answer bank (R25).
type AnswersAPI struct {
	app *App
}

// Generate drafts three answers to a question for an application; an empty
// question means "Why this company?".
func (a *AnswersAPI) Generate(appID int64, question string) (answers.Answer, error) {
	_, svc, err := a.app.services()
	if err != nil {
		return answers.Answer{}, err
	}
	out, err := svc.Answers.Generate(a.app.ctx, appID, question)
	return out, a.app.public("AnswersAPI.Generate", err)
}

// List returns an application's answers.
func (a *AnswersAPI) List(appID int64) ([]answers.Answer, error) {
	_, svc, err := a.app.services()
	if err != nil {
		return nil, err
	}
	out, err := svc.Answers.List(a.app.ctx, appID)
	return out, a.app.public("AnswersAPI.List", err)
}

// Approve saves the chosen text and adds it to the bank.
func (a *AnswersAPI) Approve(id int64, text string) (answers.Answer, error) {
	_, svc, err := a.app.services()
	if err != nil {
		return answers.Answer{}, err
	}
	out, err := svc.Answers.Approve(a.app.ctx, id, text)
	return out, a.app.public("AnswersAPI.Approve", err)
}

// Delete removes an answer from its application.
func (a *AnswersAPI) Delete(id int64) error {
	_, svc, err := a.app.services()
	if err != nil {
		return err
	}
	return a.app.public("AnswersAPI.Delete", svc.Answers.Delete(a.app.ctx, id))
}

// Bank returns every banked answer.
func (a *AnswersAPI) Bank() ([]answers.Answer, error) {
	_, svc, err := a.app.services()
	if err != nil {
		return nil, err
	}
	out, err := svc.Answers.Bank(a.app.ctx)
	return out, a.app.public("AnswersAPI.Bank", err)
}

// Suggest returns banked answers to questions similar to this one.
func (a *AnswersAPI) Suggest(question string) ([]answers.Suggestion, error) {
	_, svc, err := a.app.services()
	if err != nil {
		return nil, err
	}
	out, err := svc.Answers.Suggest(a.app.ctx, question)
	return out, a.app.public("AnswersAPI.Suggest", err)
}

// RemoveFromBank takes an answer out of the bank.
func (a *AnswersAPI) RemoveFromBank(id int64) error {
	_, svc, err := a.app.services()
	if err != nil {
		return err
	}
	return a.app.public("AnswersAPI.RemoveFromBank", svc.Answers.RemoveFromBank(a.app.ctx, id))
}
