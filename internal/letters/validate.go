package letters

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

// outputSchema is the drafting answer. Exactly three "why" options; Gemini
// strips minLength/maxLength, so ai.ValidateJSON enforces them client-side.
const outputSchema = `{
  "type": "object",
  "properties": {
    "salutation": {"type": "string", "minLength": 1, "maxLength": 120},
    "opening":    {"type": "string", "minLength": 1, "maxLength": 1200},
    "whyOptions": {"type": "array", "minItems": 3, "maxItems": 3,
                   "items": {"type": "string", "minLength": 1, "maxLength": 1200}},
    "body":       {"type": "array", "minItems": 1, "maxItems": 3,
                   "items": {"type": "string", "minLength": 1, "maxLength": 1500}},
    "closing":    {"type": "string", "minLength": 1, "maxLength": 800},
    "signOff":    {"type": "string", "minLength": 1, "maxLength": 60}
  },
  "required": ["salutation", "opening", "whyOptions", "body", "closing", "signOff"],
  "additionalProperties": false
}`

// answer is the model's reply, as the schema describes it.
type answer struct {
	Salutation string   `json:"salutation"`
	Opening    string   `json:"opening"`
	WhyOptions []string `json:"whyOptions"`
	Body       []string `json:"body"`
	Closing    string   `json:"closing"`
	SignOff    string   `json:"signOff"`
}

func decodeAnswer(text string) (answer, error) {
	dec := json.NewDecoder(bytes.NewReader([]byte(text)))
	dec.DisallowUnknownFields()
	var a answer
	if err := dec.Decode(&a); err != nil {
		return answer{}, errors.New("the answer was not the expected JSON")
	}
	return a, nil
}

// placeholder matches template leftovers: [brackets], <Company>-style tags,
// {{mustaches}} and "Hiring Manager Name" (U7, A10).
var placeholder = regexp.MustCompile(`[\[\]]|\{\{|\}\}|<[^<>\n]{1,40}>|(?i)hiring manager name`)

// check rejects drafts with placeholders, or that never name the company
// they're for (a letter written for the wrong company). It returns a reason
// suitable for the retry note, or "" when the draft is acceptable.
func check(a answer, company string) string {
	all := append([]string{a.Salutation, a.Opening, a.Closing, a.SignOff}, append(a.WhyOptions, a.Body...)...)
	for _, s := range all {
		if m := placeholder.FindString(s); m != "" {
			return "it contained the placeholder " + strconvQuote(m) + "."
		}
	}
	if company = strings.TrimSpace(company); company != "" {
		mentions := strings.ToLower(strings.Join(append([]string{a.Opening}, a.WhyOptions...), "\n"))
		if !strings.Contains(mentions, strings.ToLower(company)) {
			return "it never named the company " + strconvQuote(company) + " in the opening or the why paragraphs."
		}
	}
	return ""
}

func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
