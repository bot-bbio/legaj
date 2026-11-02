package main

import (
	"github.com/bot-bbio/legaj/internal/documents"
	"github.com/bot-bbio/legaj/internal/letters"
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
