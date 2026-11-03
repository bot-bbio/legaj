// Package followups drafts follow-up emails, decides when they're due,
// and builds compose links for the user's own mail client (R27-R29, D10).
// LeGaJ never holds email credentials: the user sends from Gmail, Outlook
// or their mail app, and tells LeGaJ whether they did.
package followups

import (
	"fmt"
	"regexp"
	"strings"
)

// Kind is a follow-up type; the values match the followups table.
type Kind string

// Follow-up kinds.
const (
	KindAppliedNudge Kind = "applied_nudge"
	KindThankYou     Kind = "thank_you"
)

// Template is a subject and body with {placeholders}.
type Template struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// Values fill a template's placeholders. An empty string, or a negative
// DaysSince, means "unknown": that placeholder stays visible and the draft
// is flagged (R27).
type Values struct {
	Company     string
	Role        string
	Interviewer string
	DaysSince   int
	Name        string // the user, for the signature
}

var defaults = map[Kind]Template{
	KindAppliedNudge: {
		Subject: "Following up on my {role} application",
		Body: "Hello,\n\n" +
			"I applied for the {role} position at {company} {days_since} ago and wanted to follow up. " +
			"I'm still very interested in the role and would welcome the chance to talk about how I could contribute.\n\n" +
			"Thank you for your time.\n\n" +
			"Best regards,\n{name}",
	},
	KindThankYou: {
		Subject: "Thank you for the {role} interview",
		Body: "Dear {interviewer},\n\n" +
			"Thank you for taking the time to speak with me about the {role} position at {company}. " +
			"I enjoyed our conversation and I'm even more excited about the opportunity.\n\n" +
			"Best regards,\n{name}",
	},
}

// DefaultTemplate returns the built-in template for a kind.
func DefaultTemplate(k Kind) Template { return defaults[k] }

// placeholderRE matches {word} placeholders: lower-case letters and
// underscores only, so ordinary text in braces isn't mistaken for one.
var placeholderRE = regexp.MustCompile(`\{([a-z_]+)\}`)

// Rendered is a filled template. Problems lists what must be fixed before
// sending; a flagged draft is never sent with blanks (R27).
type Rendered struct {
	Subject  string   `json:"subject"`
	Body     string   `json:"body"`
	Problems []string `json:"problems"`
}

// Ready reports whether the draft can be sent as is.
func (r Rendered) Ready() bool { return len(r.Problems) == 0 }

func (v Values) lookup(name string) (string, bool) {
	switch name {
	case "company":
		return v.Company, true
	case "role":
		return v.Role, true
	case "interviewer":
		return v.Interviewer, true
	case "name":
		return v.Name, true
	case "days_since":
		switch {
		case v.DaysSince < 0:
			return "", true
		case v.DaysSince == 1:
			return "1 day", true
		default:
			return fmt.Sprintf("%d days", v.DaysSince), true
		}
	}
	return "", false
}

// fill replaces known placeholders that have a value and leaves the rest.
func fill(text string, v Values) string {
	return placeholderRE.ReplaceAllStringFunc(text, func(m string) string {
		val, known := v.lookup(m[1 : len(m)-1])
		if !known || strings.TrimSpace(val) == "" {
			return m
		}
		return val
	})
}

// Render fills a template. Unknown placeholders and ones without a value
// stay visible in the text and are listed as problems.
func Render(t Template, v Values) Rendered {
	r := Rendered{Subject: fill(t.Subject, v), Body: fill(t.Body, v)}
	r.Problems = check(r.Subject, r.Body)
	return r
}

// Check lists the problems in a draft the user may have edited: an empty
// body, or any placeholder still in the text.
func Check(body string) []string { return check("", body) }

func check(subject, body string) []string {
	problems := []string{}
	if strings.TrimSpace(body) == "" {
		problems = append(problems, "The message is empty.")
	}
	seen := map[string]bool{}
	for _, m := range placeholderRE.FindAllString(subject+"\n"+body, -1) {
		if seen[m] {
			continue
		}
		seen[m] = true
		if _, known := (Values{}).lookup(m[1 : len(m)-1]); known {
			problems = append(problems, fmt.Sprintf("Fill in %s before sending.", m))
		} else {
			problems = append(problems, fmt.Sprintf("%s isn't a placeholder LeGaJ knows. Replace it before sending.", m))
		}
	}
	return problems
}
