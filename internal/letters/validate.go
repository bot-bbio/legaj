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

// placeholder matches template leftovers (U7, A10): words in square
// brackets ("[Hiring Manager]"), capitalised tags ("<Company>",
// "<Your Name>"), {{mustaches}} and "Hiring Manager Name". Technical text
// such as "std::vector<int>", "<10ms" or a "[1]" citation isn't one.
var placeholder = regexp.MustCompile(`\[[A-Za-z][^\[\]\n]{0,40}\]|\{\{|\}\}|<[A-Z][A-Za-z .'-]{1,40}>|(?i)hiring manager name`)

// FindPlaceholder returns the first template leftover in s, or "". Form
// answers (R25) use the same rule as letters.
func FindPlaceholder(s string) string { return placeholder.FindString(s) }

// check rejects drafts with placeholders, that never name the company
// they're for, or that name another company from the pipeline (a letter
// written for the wrong company, R21). It returns a reason suitable for the
// retry note, or "" when the draft is acceptable.
func check(a answer, company string, rule CompanyRule) string {
	all := append([]string{a.Salutation, a.Opening, a.Closing, a.SignOff}, append(a.WhyOptions, a.Body...)...)
	for _, s := range all {
		if m := placeholder.FindString(s); m != "" {
			return "it contained the placeholder " + strconvQuote(m) + "."
		}
	}
	if company = strings.TrimSpace(company); company != "" {
		if !rule.Names(strings.Join(append([]string{a.Opening}, a.WhyOptions...), "\n")) {
			return "it never named the company " + strconvQuote(company) + " in the opening or the why paragraphs."
		}
	}
	for _, s := range all {
		if other := rule.Wrong(s); other != "" {
			return "it named another company, " + strconvQuote(other) + ", instead of " + strconvQuote(company) + "."
		}
	}
	return ""
}

func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
