package pdf_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/pdf"
	"github.com/bot-bbio/legaj/internal/testenv"
)

const para = "In my last role I rebuilt the release pipeline, cut flaky tests by half, and wrote the runbook the on-call team still uses today."

func withParas(n int) pdf.Letter {
	l := sample()
	l.Paragraphs = nil
	for range n {
		l.Paragraphs = append(l.Paragraphs, para)
	}
	return l
}

func TestR24_OverflowFailsNoFileWritten(t *testing.T) {
	dir := testenv.Isolate(t)
	path := filepath.Join(dir, "documents", "Acme_v1.pdf")
	_, err := pdf.WriteLetter(withParas(60), path)
	if !pdf.IsDoesntFit(err) {
		t.Fatalf("err = %v, want doesn't-fit (A12)", err)
	}
	if apperr.CodeOf(err) != apperr.CodeInvalid || !strings.Contains(err.Error(), "doesn't fit on one page") {
		t.Fatalf("message = %q", err.Error())
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a PDF was written for a letter that doesn't fit")
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 0 {
		t.Fatalf("left %d files behind (temp files count too)", len(entries))
	}
}

func TestR24_ExactlyOnePagePasses(t *testing.T) {
	// Find the longest letter that still fits, by adding paragraphs.
	n := 0
	for ; n < 60; n++ {
		if _, _, err := pdf.Fit(withParas(n + 1)); err != nil {
			if !pdf.IsDoesntFit(err) {
				t.Fatal(err)
			}
			break
		}
	}
	if n < 3 {
		t.Fatalf("only %d paragraphs fit; the layout is wasting space", n)
	}
	data, preset, err := pdf.Fit(withParas(n))
	if err != nil {
		t.Fatalf("the longest fitting letter (%d paragraphs) failed: %v", n, err)
	}
	if preset != pdf.Presets[len(pdf.Presets)-1] {
		t.Fatalf("a letter at the limit should need the most compact preset, got %+v", preset)
	}
	extract(t, data) // fails unless exactly one page
}

func TestR24_ShortLetterUsesStandardPreset(t *testing.T) {
	_, preset, err := pdf.Fit(sample())
	if err != nil {
		t.Fatal(err)
	}
	if preset != pdf.Presets[0] {
		t.Fatalf("preset = %+v, want the standard layout", preset)
	}
}

func TestR24_WriteLetterWritesAtomically(t *testing.T) {
	dir := testenv.Isolate(t)
	path := filepath.Join(dir, "documents", "Acme & Co_v1.pdf")
	if _, err := pdf.WriteLetter(sample(), path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := squash(extract(t, data)); !strings.Contains(got, "<Company>") {
		t.Fatal("written file isn't the rendered letter")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("%d files in the folder, want just the PDF (no temp leftovers)", len(entries))
	}
}
