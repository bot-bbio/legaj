package pdf

import (
	"errors"
	"strings"

	"github.com/bot-bbio/legaj/internal/apperr"
)

// Letter is a cover letter's content. Every field is plain text: nothing is
// parsed as markup.
type Letter struct {
	Name       string   // centered, bold, at the top
	Contact    string   // centered under the name: email · phone · city
	Date       string   // e.g. "2 November 2026"
	Recipient  []string // address lines: "Hiring Team", company, city
	Salutation string   // "Dear Hiring Team,"
	Paragraphs []string
	Closing    string // "Sincerely,"
	Signature  string // usually the name again
}

// Preset is a set of type sizes and spacing, all in points. Presets[0] is
// V1's standard layout; later ones are progressively more compact.
type Preset struct {
	Base, Leading, NameSize, InfoSize, Margin, SpaceAfter float64
}

// Presets are V1's four layouts, standard first.
var Presets = []Preset{
	{Base: 11, Leading: 14.5, NameSize: 13, InfoSize: 10, Margin: 72, SpaceAfter: 12},
	{Base: 10.5, Leading: 14, NameSize: 12.5, InfoSize: 9.5, Margin: 60, SpaceAfter: 10},
	{Base: 10, Leading: 13.5, NameSize: 12, InfoSize: 9, Margin: 50, SpaceAfter: 8},
	{Base: 9.5, Leading: 12.5, NameSize: 11.5, InfoSize: 8.5, Margin: 40, SpaceAfter: 6},
}

// ErrDoesntFit means the letter needs more than one page with this preset.
var ErrDoesntFit = apperr.New(apperr.CodeInvalid, "The letter doesn't fit on one page. Shorten it and try again.")

// op is one line of text to draw.
type op struct {
	bold       bool
	size, x, y float64
	text       string
}

// planner lays lines out top to bottom.
type planner struct {
	d   *doc
	p   Preset
	y   float64
	ops []op
}

func (pl *planner) width() float64 { return pageW - 2*pl.p.Margin }

// lines wraps text and adds its lines, left-aligned or centered.
func (pl *planner) lines(text string, bold bool, size, leading float64, center bool) error {
	measure := pl.d.measurer(bold, size)
	wrapped, err := Wrap(measure, text, pl.width())
	if err != nil {
		return err
	}
	for _, l := range wrapped {
		x := pl.p.Margin
		if center {
			w, err := measure(l)
			if err != nil {
				return err
			}
			x = (pageW - w) / 2
		}
		pl.ops = append(pl.ops, op{bold: bold, size: size, x: x, y: pl.y, text: l})
		pl.y += leading
	}
	return nil
}

// plan lays out the whole letter without drawing it, returning the lines
// and where the text ends.
func plan(d *doc, l Letter, p Preset) ([]op, float64, error) {
	pl := &planner{d: d, p: p, y: p.Margin}
	steps := []func() error{
		func() error { return pl.lines(l.Name, true, p.NameSize, p.NameSize*1.2, true) },
		func() error { pl.y += 2; return pl.lines(l.Contact, false, p.InfoSize, p.InfoSize*1.2, true) },
		func() error { pl.y += 12 + 10; return pl.lines(l.Date, false, p.Base, p.Leading, false) },
		func() error {
			pl.y += 6
			for _, r := range l.Recipient {
				if err := pl.lines(r, false, p.Base, p.Base*1.2, false); err != nil {
					return err
				}
			}
			pl.y += 6
			return nil
		},
		func() error {
			if err := pl.lines(l.Salutation, false, p.Base, p.Leading, false); err != nil {
				return err
			}
			pl.y += p.SpaceAfter
			return nil
		},
		func() error {
			for _, para := range l.Paragraphs {
				if strings.TrimSpace(para) == "" {
					continue
				}
				if err := pl.lines(para, false, p.Base, p.Leading, false); err != nil {
					return err
				}
				pl.y += p.SpaceAfter
			}
			return nil
		},
		func() error {
			if err := pl.lines(l.Closing, false, p.Base, p.Leading, false); err != nil {
				return err
			}
			pl.y += 22
			return pl.lines(l.Signature, false, p.Base, p.Leading, false)
		},
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return nil, 0, err
		}
	}
	return pl.ops, pl.y, nil
}

// Render lays the letter out with preset p and returns the PDF. It returns
// ErrDoesntFit, and no PDF, when the text would run past the bottom margin.
func Render(l Letter, p Preset) ([]byte, error) {
	d, err := newDoc()
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, apperr.GenericMessage)
	}
	ops, bottom, err := plan(d, l, p)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "LeGaJ couldn't lay out the letter.")
	}
	if bottom > pageH-p.Margin {
		return nil, ErrDoesntFit
	}
	for _, o := range ops {
		if err := d.draw(o.bold, o.size, o.x, o.y, o.text); err != nil {
			return nil, apperr.Wrap(err, apperr.CodeInternal, "LeGaJ couldn't draw the letter.")
		}
	}
	data, err := d.bytes()
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "LeGaJ couldn't write the PDF.")
	}
	return data, nil
}

// IsDoesntFit reports whether err is the one-page overflow.
func IsDoesntFit(err error) bool {
	return errors.Is(err, ErrDoesntFit)
}
