package documents

import (
	"fmt"
	"strings"
	"unicode"
)

// maxSegment bounds one folder or file name part, keeping full paths far
// below Windows' 260-character limit.
const maxSegment = 60

// reserved are Windows device names that can't be used as a file name,
// with or without an extension.
var reserved = map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true}

func init() {
	for i := 1; i <= 9; i++ {
		reserved[fmt.Sprintf("COM%d", i)] = true
		reserved[fmt.Sprintf("LPT%d", i)] = true
	}
}

// Sanitize makes s safe as one Windows file or folder name part: path
// separators, the characters Windows forbids (\ / : * ? " < > |) and control
// characters become "-"; leading and trailing dots and spaces go (Windows
// forbids trailing ones); it's shortened to maxSegment runes; and device
// names (CON, LPT1…) get a "_" prefix. Blank results become fallback.
func Sanitize(s, fallback string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`\/:*?"<>|`, r) {
			return '-'
		}
		return r
	}, s)
	s = strings.Trim(s, " .")
	if r := []rune(s); len(r) > maxSegment {
		s = strings.TrimRight(string(r[:maxSegment]), " .")
	}
	if s == "" {
		return fallback
	}
	base := strings.ToUpper(strings.SplitN(s, ".", 2)[0])
	if reserved[strings.TrimSpace(base)] {
		s = "_" + s
	}
	return s
}

// appFolder is the folder holding one application's documents.
func appFolder(appID int64, company string) string {
	return fmt.Sprintf("%d_%s", appID, Sanitize(company, "Application"))
}

// letterFile names version v of an application's cover letter.
func letterFile(appID int64, company, role string, v int) string {
	name := Sanitize(company, fmt.Sprintf("Application_%d", appID))
	if r := Sanitize(role, ""); r != "" {
		name += "_" + r
	}
	return fmt.Sprintf("%s_Cover_Letter_v%d.pdf", strings.ReplaceAll(name, " ", "_"), v)
}
