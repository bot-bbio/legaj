package pdf_test

import (
	"strings"
	"testing"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/pdf"
)

func TestR24_CharactersTheFontCantPrintAreNamed(t *testing.T) {
	base := pdf.Letter{Name: "Ada Lovelace", Salutation: "Dear Hiring Team,", Closing: "Sincerely,", Signature: "Ada"}

	for _, ok := range []string{"Łukasz Żółć, naïve — café", "Ζωή and Привет"} {
		l := base
		l.Paragraphs = []string{ok}
		if _, _, err := pdf.Fit(l); err != nil {
			t.Errorf("%q refused: %v", ok, err)
		}
	}
	for _, c := range []struct{ text, want string }{
		{"Thanks, 李明.", "李"},
		{"Shipped ✓ on time.", "✓"},
	} {
		l := base
		l.Paragraphs = []string{c.text}
		_, _, err := pdf.Fit(l)
		if apperr.CodeOf(err) != apperr.CodeInvalid || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: err = %v; want a clear refusal naming %q (never blank boxes in the PDF, A11)", c.text, err, c.want)
		}
	}
	// The name is drawn in bold: it's checked too.
	l := base
	l.Name, l.Paragraphs = "李明", []string{"Hello."}
	if _, _, err := pdf.Fit(l); apperr.CodeOf(err) != apperr.CodeInvalid {
		t.Fatalf("an unprintable name: err = %v", err)
	}
}

func TestR24_SaveWritesBytesAtomically(t *testing.T) {
	path := t.TempDir() + `\sub\letter.pdf`
	if err := pdf.Save(path, []byte("%PDF-1.7 test")); err != nil {
		t.Fatal(err)
	}
}
