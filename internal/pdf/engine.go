// Package pdf renders cover letters to PDF in pure Go (R24, D5).
//
// V1 rendered through Python/ReportLab with markup parsing, so "AT&T"
// came out as "AT&T;" (A10) and Polish letters were dropped (A11). Here text
// is drawn as plain text: there is no markup, so nothing needs escaping,
// and an embedded Unicode TrueType font (Tinos, SIL OFL 1.1) carries every
// glyph, so the PDF looks the same on any machine (C14).
package pdf

import (
	_ "embed"
	"fmt"

	"github.com/signintech/gopdf"
)

// Tinos is metrically compatible with Times New Roman, V1's letter font.
//
//go:embed fonts/Tinos-Regular.ttf
var regularTTF []byte

//go:embed fonts/Tinos-Bold.ttf
var boldTTF []byte

const (
	fontRegular = "tinos"
	fontBold    = "tinos-bold"

	// US Letter in points, as V1 used.
	pageW = 612.0
	pageH = 792.0
)

// doc is one single-page PDF being built.
type doc struct {
	gp *gopdf.GoPdf
}

func newDoc() (*doc, error) {
	gp := &gopdf.GoPdf{}
	gp.Start(gopdf.Config{PageSize: *gopdf.PageSizeLetter})
	gp.SetInfo(gopdf.PdfInfo{Creator: "LeGaJ", Producer: "LeGaJ"})
	if err := gp.AddTTFFontData(fontRegular, regularTTF); err != nil {
		return nil, fmt.Errorf("pdf: load regular font: %w", err)
	}
	if err := gp.AddTTFFontData(fontBold, boldTTF); err != nil {
		return nil, fmt.Errorf("pdf: load bold font: %w", err)
	}
	gp.AddPage()
	return &doc{gp: gp}, nil
}

func (d *doc) setFont(bold bool, size float64) error {
	family := fontRegular
	if bold {
		family = fontBold
	}
	return d.gp.SetFont(family, "", size)
}

// measurer returns a width function for one font and size.
func (d *doc) measurer(bold bool, size float64) func(string) (float64, error) {
	return func(s string) (float64, error) {
		if err := d.setFont(bold, size); err != nil {
			return 0, err
		}
		return d.gp.MeasureTextWidth(s)
	}
}

// draw writes text with its top-left corner at (x, y).
func (d *doc) draw(bold bool, size, x, y float64, text string) error {
	if err := d.setFont(bold, size); err != nil {
		return err
	}
	d.gp.SetXY(x, y)
	return d.gp.Cell(nil, text)
}

func (d *doc) bytes() ([]byte, error) {
	return d.gp.GetBytesPdfReturnErr()
}
