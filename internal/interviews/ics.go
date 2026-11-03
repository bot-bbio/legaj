package interviews

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bot-bbio/legaj/internal/apperr"
)

// ICS renders a round as an RFC 5545 calendar file (R31): CRLF line ends,
// lines folded at 75 octets without splitting a character, escaped text,
// UTC start and end, and a UID that's the same on every export, so
// importing it again updates the event instead of duplicating it. stamp is
// the export time (DTSTAMP).
func ICS(r Round, stamp time.Time) []byte {
	const utc = "20060102T150405Z"
	about := r.Company
	if r.Role != "" {
		about = fmt.Sprintf("%s (%s)", r.Company, r.Role)
	}
	kind := r.Kind
	if kind == "" || kind == "other" {
		kind = "Job"
	}
	summary := strings.ToUpper(kind[:1]) + kind[1:] + " interview: "

	var desc []string
	desc = append(desc, about)
	if r.Link != "" {
		desc = append(desc, "Meeting link: "+r.Link)
	}
	if len(r.Interviewers) > 0 {
		names := make([]string, 0, len(r.Interviewers))
		for _, p := range r.Interviewers {
			if p.Title != "" {
				names = append(names, fmt.Sprintf("%s (%s)", p.Name, p.Title))
			} else {
				names = append(names, p.Name)
			}
		}
		desc = append(desc, "Interviewers: "+strings.Join(names, ", "))
	}
	if r.PrepNotes != "" {
		desc = append(desc, "", "Prep notes:", r.PrepNotes)
	}

	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//LeGaJ//Interviews//EN",
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
		"BEGIN:VEVENT",
		fmt.Sprintf("UID:legaj-round-%d@legaj.app", r.ID),
		"DTSTAMP:" + stamp.UTC().Format(utc),
		"DTSTART:" + r.StartsAt.UTC().Format(utc),
		"DTEND:" + r.EndsAt.UTC().Format(utc),
		"SUMMARY:" + summary + escapeText(about),
	}
	if r.Location != "" {
		lines = append(lines, "LOCATION:"+escapeText(r.Location))
	}
	if r.Link != "" {
		lines = append(lines, "URL:"+r.Link)
	}
	lines = append(lines, "DESCRIPTION:"+escapeText(strings.Join(desc, "\n")), "END:VEVENT", "END:VCALENDAR")

	var b strings.Builder
	for _, l := range lines {
		fold(&b, l)
	}
	return []byte(b.String())
}

// escapeText escapes a TEXT value (RFC 5545 3.3.11).
func escapeText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\n", `\n`, "\r", `\n`).Replace(s)
}

// fold writes one content line, folded so no line exceeds 75 octets and
// no UTF-8 character is split (RFC 5545 3.1); continuation lines start
// with a space.
func fold(b *strings.Builder, line string) {
	n := 0
	for _, r := range line {
		size := utf8.RuneLen(r)
		if n+size > 75 {
			b.WriteString("\r\n ")
			n = 1
		}
		b.WriteRune(r)
		n += size
	}
	b.WriteString("\r\n")
}

// ExportFolder holds exported calendar files inside the data folder.
const ExportFolder = "exports"

// Export writes a round's calendar file to <dataDir>/exports and returns
// its path. The name depends only on the round, so exporting again
// replaces the file (and the stable UID updates the calendar event).
func (s *Service) Export(ctx context.Context, id int64, dataDir string) (string, error) {
	r, err := s.Get(ctx, id)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(dataDir, ExportFolder)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", exportErr(err)
	}
	path := filepath.Join(dir, fmt.Sprintf("interview-%d.ics", r.ID))
	tmp, err := os.CreateTemp(dir, ".interview-*.tmp")
	if err != nil {
		return "", exportErr(err)
	}
	_, werr := tmp.Write(ICS(r, s.cfg.Now()))
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(tmp.Name())
		return "", exportErr(errors.Join(werr, cerr))
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return "", exportErr(err)
	}
	return path, nil
}

func exportErr(err error) error {
	return apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't write the calendar file.")
}
