package letters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/bot-bbio/legaj/internal/ai"
	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/documents"
)

// Part names a paragraph of a draft (R22: regenerate one paragraph).
type Part string

// Draft parts.
const (
	PartOpening Part = "opening"
	PartWhy     Part = "why"
	PartBody    Part = "body"
	PartClosing Part = "closing"
)

// Edit limits, matching the drafting schema with some room for the user.
const (
	maxParagraph = 2000
	maxShortLine = 200
)

func invalid(msg string) error { return apperr.New(apperr.CodeInvalid, msg) }

// validateEdit checks a draft the user edited.
func validateEdit(d Draft) error {
	if d.ChosenWhy < 0 || d.ChosenWhy >= len(d.WhyOptions) {
		return invalid("Pick one of the why-this-company paragraphs.")
	}
	if len(d.Body) == 0 || len(d.Body) > 5 || len(d.WhyOptions) != 3 {
		return invalid("The letter needs its opening, three why options and one to five body paragraphs.")
	}
	long := func(s string, max int) bool { return utf8.RuneCountInString(s) > max }
	if long(d.Salutation, maxShortLine) || long(d.SignOff, maxShortLine) {
		return invalid("The greeting or sign-off is too long.")
	}
	for _, p := range append(append([]string{d.Opening, d.Closing}, d.WhyOptions...), d.Body...) {
		if long(p, maxParagraph) {
			return invalid(fmt.Sprintf("A paragraph is over %d characters. Shorten it.", maxParagraph))
		}
	}
	return nil
}

// SaveDraft stores the user's edits over the draft revision they were made
// on (d.Rev). If that draft was approved or replaced meanwhile, nothing is
// saved and the user is told to reopen it.
func (s *Service) SaveDraft(ctx context.Context, appID int64, d Draft) (Draft, error) {
	if err := validateEdit(d); err != nil {
		return Draft{}, err
	}
	if d.Rev <= 0 {
		return Draft{}, errDraftChanged
	}
	return s.updateDraft(ctx, appID, d.Rev, func(cur *Draft) error {
		*cur = d
		return nil
	})
}

// paragraphSchema is the answer to a one-paragraph rewrite.
const paragraphSchema = `{
  "type": "object",
  "properties": {"paragraph": {"type": "string", "minLength": 1, "maxLength": 1500}},
  "required": ["paragraph"],
  "additionalProperties": false
}`

// target returns a pointer to the paragraph a Part and index name.
func target(d *Draft, part Part, i int) (*string, error) {
	switch {
	case part == PartOpening:
		return &d.Opening, nil
	case part == PartClosing:
		return &d.Closing, nil
	case part == PartWhy && i >= 0 && i < len(d.WhyOptions):
		return &d.WhyOptions[i], nil
	case part == PartBody && i >= 0 && i < len(d.Body):
		return &d.Body[i], nil
	}
	return nil, invalid("That paragraph doesn't exist in the draft.")
}

// RegenerateParagraph rewrites one paragraph and leaves every other one
// exactly as it was (R22). The model sees the whole draft for context.
func (s *Service) RegenerateParagraph(ctx context.Context, appID int64, part Part, index int) (Draft, error) {
	_, d, err := s.currentDraft(ctx, appID)
	if err != nil {
		return Draft{}, err
	}
	rev := d.Rev
	slot, err := target(&d, part, index)
	if err != nil {
		return Draft{}, err
	}
	app, err := s.loadApp(ctx, appID)
	if err != nil {
		return Draft{}, err
	}
	p, err := s.cfg.Profile.Get(ctx)
	if err != nil {
		return Draft{}, err
	}
	// No HTML escaping: "AT&T" must reach the model as written (A10).
	var ctxJSON bytes.Buffer
	enc := json.NewEncoder(&ctxJSON)
	enc.SetEscapeHTML(false)
	d.Rev = 0
	if err := enc.Encode(d); err != nil {
		return Draft{}, apperr.Wrap(err, apperr.CodeInternal, apperr.GenericMessage)
	}
	current := bytes.TrimSpace(ctxJSON.Bytes())
	reason := ""
	for attempt := 1; attempt <= 2; attempt++ {
		base, err := userPrompt(app, p, Style{Instruction: StyleInstruction(d.Style)}, reason)
		if err != nil {
			return Draft{}, apperr.Wrap(err, apperr.CodeInternal, apperr.GenericMessage)
		}
		task := fmt.Sprintf("Rewrite only this paragraph of the current draft (part %q, number %d), in a fresh way:\n%s\n\nCurrent draft, for context only:\n%s\n\n%s",
			part, index+1, *slot, stripMarkers(string(current)), base)
		temp := 0.9
		resp, err := s.cfg.AI.Generate(ctx, ai.Request{
			Model: s.cfg.Model(ctx), System: systemPrompt, Parts: []ai.Part{ai.Text(task)},
			Temperature: &temp, Schema: []byte(paragraphSchema),
		})
		if apperr.CodeOf(err) == apperr.CodeAIInvalidOutput {
			reason = "it didn't match the required JSON shape."
			continue
		}
		if err != nil {
			return Draft{}, err
		}
		var out struct {
			Paragraph string `json:"paragraph"`
		}
		dec := json.NewDecoder(bytes.NewReader([]byte(resp.Text)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&out); err != nil {
			reason = "it wasn't the expected JSON."
			continue
		}
		if m := placeholder.FindString(out.Paragraph); m != "" {
			reason = "it contained the placeholder " + strconvQuote(m) + "."
			continue
		}
		paragraph := strings.TrimSpace(out.Paragraph)
		// Write only this paragraph into the draft as stored now: edits made
		// while Gemini worked survive, and an approved or replaced draft is
		// left alone.
		return s.updateDraft(ctx, appID, rev, func(cur *Draft) error {
			at, err := target(cur, part, index)
			if err != nil {
				return err
			}
			*at = paragraph
			return nil
		})
	}
	return Draft{}, apperr.Wrap(errors.New("paragraph rewrite rejected twice: "+reason), apperr.CodeAIInvalidOutput,
		"Gemini's rewrite didn't pass LeGaJ's checks twice. Try again or edit it yourself.")
}

// Approve renders the current draft to the application's next PDF version
// (R22) and retires the draft in the same transaction, so one draft is
// never approved twice. The document is recorded only if the application
// still exists and the letter fits on one page; recording it also applies
// the Ready rule (R26).
func (s *Service) Approve(ctx context.Context, appID int64) (documents.Version, error) {
	draftID, d, err := s.currentDraft(ctx, appID)
	if err != nil {
		return documents.Version{}, err
	}
	if err := validateEdit(d); err != nil {
		return documents.Version{}, err
	}
	app, err := s.loadApp(ctx, appID)
	if err != nil {
		return documents.Version{}, err
	}
	p, err := s.cfg.Profile.Get(ctx)
	if err != nil {
		return documents.Version{}, err
	}
	return s.cfg.Documents.ApproveDraft(ctx, appID, draftID, d.Rev, ToPDF(d, p, app, s.cfg.Now()))
}
