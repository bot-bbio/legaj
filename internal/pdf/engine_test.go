package pdf_test

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"unicode"

	readpdf "github.com/ledongthuc/pdf"

	"github.com/bot-bbio/legaj/internal/pdf"
	"github.com/bot-bbio/legaj/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

// extract returns all text in a PDF, as a PDF reader sees it.
func extract(t *testing.T, data []byte) string {
	t.Helper()
	r, err := readpdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("not a readable PDF: %v", err)
	}
	if r.NumPage() != 1 {
		t.Fatalf("%d pages, want 1", r.NumPage())
	}
	tr, err := r.GetPlainText()
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(tr)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// squash drops whitespace, so line wrapping and run splitting don't matter
// but every character and its order still do.
func squash(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

func sample() pdf.Letter {
	return pdf.Letter{
		Name:       "Łukasz Żółć",
		Contact:    "lukasz@example.com · +48 601 234 567 · Kraków",
		Date:       "2 November 2026",
		Recipient:  []string{"Hiring Team", "AT&T", "Dallas, TX"},
		Salutation: "Dear Hiring Team,",
		Paragraphs: []string{
			"I cut AT&T's R&D build times by <5% per release, and I'd bring that care to <Company>.",
			"My first prototype was naïve — rewritten twice — and now serves Łódź and Gdańsk.",
		},
		Closing:   "Sincerely,",
		Signature: "Łukasz Żółć",
	}
}

func TestR24_TextRoundTrip(t *testing.T) {
	data, err := pdf.Render(sample(), pdf.Presets[0])
	if err != nil {
		t.Fatal(err)
	}
	got := squash(extract(t, data))
	for _, want := range []string{"AT&T", "R&D", "<5%", "<Company>", "Łukasz Żółć", "naïve —", "Łódź", "Gdańsk", "Kraków"} {
		if !strings.Contains(got, squash(want)) {
			t.Errorf("text read back from the PDF lacks %q (A10/A11)", want)
		}
	}
	for _, bad := range []string{"AT&T;", "&amp;", "&lt;"} {
		if strings.Contains(got, bad) {
			t.Errorf("markup escaping leaked into the PDF text: %q", bad)
		}
	}
}

func TestR24_FontsAreEmbedded(t *testing.T) {
	data, err := pdf.Render(sample(), pdf.Presets[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("/FontFile2")) {
		t.Fatal("no embedded TrueType font: glyphs would depend on the reader's machine (C14)")
	}
}

func TestR24_WrapKeepsWordsAndBreaksOverlongOnes(t *testing.T) {
	// A fake measure: every rune is 1 unit wide.
	measure := func(s string) (float64, error) { return float64(len([]rune(s))), nil }
	lines, err := pdf.Wrap(measure, "the quick brown fox jumps", 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"the quick", "brown fox", "jumps"}
	if strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Fatalf("lines = %q, want %q", lines, want)
	}
	lines, err = pdf.Wrap(measure, "supercalifragilistic ok", 8)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lines {
		if len([]rune(l)) > 8 {
			t.Fatalf("line %q exceeds the width", l)
		}
	}
	if squash(strings.Join(lines, "")) != "supercalifragilisticok" {
		t.Fatalf("characters lost while breaking a long word: %q", lines)
	}
	if lines, _ := pdf.Wrap(measure, "   ", 8); len(lines) != 0 {
		t.Fatalf("blank text produced lines: %q", lines)
	}
}
