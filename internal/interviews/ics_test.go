package interviews_test

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/bot-bbio/legaj/internal/interviews"
	"github.com/bot-bbio/legaj/internal/store"
)

func goldenRound() interviews.Round {
	r := interviews.Round{
		ID:       42,
		Company:  "Acme, Inc.",
		Role:     "R&D Engineer; Platform",
		StartsAt: time.Date(2026, 11, 3, 23, 0, 0, 0, time.UTC),
	}
	r.Zone, r.DurationMin, r.Kind = "America/New_York", 45, "video"
	r.EndsAt = r.StartsAt.Add(45 * time.Minute)
	r.Location, r.Link = `Room 4\B`, "https://meet.example.com/abc-def"
	r.Interviewers = []store.Interviewer{{Name: "Grace Hopper", Title: "Admiral"}, {Name: "Alan Turing"}}
	r.PrepNotes = "Read the JD.\nAsk about the team — Łódź office."
	r.Created = time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	r.Updated = time.Date(2026, 11, 1, 9, 30, 0, 0, time.UTC)
	return r
}

func TestR31_GoldenICS(t *testing.T) {
	want, err := os.ReadFile("testdata/round.ics")
	if err != nil {
		t.Fatal(err)
	}
	got := interviews.ICS(goldenRound(), time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC))
	if !bytes.Equal(got, want) {
		t.Fatalf("ICS differs from the golden file:\n got %q\nwant %q", got, want)
	}

	// The UID is stable: exporting again (later) updates the same event.
	again := interviews.ICS(goldenRound(), time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC))
	if !bytes.Contains(again, []byte("UID:legaj-round-42-1793520000000@legaj.app\r\n")) {
		t.Fatal("the UID must not depend on the export time")
	}
}

func TestR31_FoldingKeepsUTF8AndUnfoldsExactly(t *testing.T) {
	r := goldenRound()
	for pad := range 6 {
		// Shift multi-byte characters across the 75-octet boundary.
		r.PrepNotes = strings.Repeat("x", 60+pad) + strings.Repeat("łó—", 40)
		out := string(interviews.ICS(r, time.Now()))
		if strings.Contains(strings.ReplaceAll(out, "\r\n", ""), "\n") {
			t.Fatal("bare LF in the output")
		}
		for _, line := range strings.Split(strings.TrimSuffix(out, "\r\n"), "\r\n") {
			if len(line) > 75 {
				t.Fatalf("line of %d octets: %q", len(line), line)
			}
			if !utf8.ValidString(line) {
				t.Fatalf("a fold split a character: %q", line)
			}
		}
		unfolded := strings.ReplaceAll(out, "\r\n ", "")
		if !strings.Contains(unfolded, r.PrepNotes) {
			t.Fatalf("pad %d: unfolding doesn't give back the notes", pad)
		}
	}
}

func TestR31_UIDIsUniqueAcrossReinstallsAndEditsRaiseSequence(t *testing.T) {
	r := goldenRound()
	reinstalled := goldenRound() // same row ID 42, created later
	reinstalled.Created = time.Date(2027, 1, 5, 10, 0, 0, 0, time.UTC)
	reinstalled.Updated = reinstalled.Created
	uid := func(r interviews.Round) string {
		for _, line := range strings.Split(string(interviews.ICS(r, time.Now())), "\r\n") {
			if strings.HasPrefix(line, "UID:") {
				return line
			}
		}
		return ""
	}
	if uid(r) == uid(reinstalled) {
		t.Fatalf("two different rounds share %s", uid(r))
	}
	edited := goldenRound()
	edited.Updated = edited.Updated.Add(time.Hour)
	if uid(edited) != uid(r) {
		t.Fatal("editing a round must keep its UID")
	}
	if !bytes.Contains(interviews.ICS(edited, time.Now()), []byte("SEQUENCE:9000\r\n")) {
		t.Fatal("an edit must raise SEQUENCE")
	}
}
