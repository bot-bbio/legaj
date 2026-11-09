package resume

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/bot-bbio/legaj/internal/profile"
)

// outputSchema is what Gemini must return (R8, A15). It uses only keywords
// Gemini's structured output accepts; maxLength is stripped server-side but
// enforced by ai.ValidateJSON. Date formats and ranges are checked in Go.
const outputSchema = `{
  "type": "object",
  "properties": {
    "fullName": {"type": "string", "maxLength": 200},
    "email":    {"type": "string", "maxLength": 200},
    "phone":    {"type": "string", "maxLength": 200},
    "location": {"type": "string", "maxLength": 200},
    "headline": {"type": "string", "maxLength": 200},
    "summary":  {"type": "string", "maxLength": 4000},
    "links": {
      "type": "array", "maxItems": 10,
      "items": {
        "type": "object",
        "properties": {
          "label": {"type": "string", "maxLength": 200},
          "url":   {"type": "string", "maxLength": 1000}
        },
        "required": ["label", "url"],
        "additionalProperties": false
      }
    },
    "experiences": {
      "type": "array", "maxItems": 20,
      "items": {
        "type": "object",
        "properties": {
          "company":  {"type": "string", "maxLength": 200},
          "title":    {"type": "string", "maxLength": 200},
          "location": {"type": "string", "maxLength": 200},
          "start":    {"type": "string", "maxLength": 7, "description": "YYYY-MM, YYYY, or empty"},
          "end":      {"type": "string", "maxLength": 7, "description": "YYYY-MM, YYYY, or empty when current or unknown"},
          "current":  {"type": "boolean"},
          "bullets":  {"type": "array", "maxItems": 30, "items": {"type": "string", "maxLength": 1000}}
        },
        "required": ["company", "title", "location", "start", "end", "current", "bullets"],
        "additionalProperties": false
      }
    },
    "education": {
      "type": "array", "maxItems": 20,
      "items": {
        "type": "object",
        "properties": {
          "school":   {"type": "string", "maxLength": 200},
          "degree":   {"type": "string", "maxLength": 200},
          "field":    {"type": "string", "maxLength": 200},
          "location": {"type": "string", "maxLength": 200},
          "start":    {"type": "string", "maxLength": 7},
          "end":      {"type": "string", "maxLength": 7},
          "details":  {"type": "string", "maxLength": 1000}
        },
        "required": ["school", "degree", "field", "location", "start", "end", "details"],
        "additionalProperties": false
      }
    },
    "skills": {
      "type": "array", "maxItems": 30,
      "items": {
        "type": "object",
        "properties": {
          "category": {"type": "string", "maxLength": 200},
          "items":    {"type": "array", "maxItems": 100, "items": {"type": "string", "maxLength": 200}}
        },
        "required": ["category", "items"],
        "additionalProperties": false
      }
    }
  },
  "required": ["fullName", "email", "phone", "location", "headline", "summary", "links", "experiences", "education", "skills"],
  "additionalProperties": false
}`

// The model's answer, as the schema describes it.
type (
	modelLink struct {
		Label string `json:"label"`
		URL   string `json:"url"`
	}
	modelExperience struct {
		Company  string   `json:"company"`
		Title    string   `json:"title"`
		Location string   `json:"location"`
		Start    string   `json:"start"`
		End      string   `json:"end"`
		Current  bool     `json:"current"`
		Bullets  []string `json:"bullets"`
	}
	modelEducation struct {
		School   string `json:"school"`
		Degree   string `json:"degree"`
		Field    string `json:"field"`
		Location string `json:"location"`
		Start    string `json:"start"`
		End      string `json:"end"`
		Details  string `json:"details"`
	}
	modelSkillGroup struct {
		Category string   `json:"category"`
		Items    []string `json:"items"`
	}
	modelProfile struct {
		FullName    string            `json:"fullName"`
		Email       string            `json:"email"`
		Phone       string            `json:"phone"`
		Location    string            `json:"location"`
		Headline    string            `json:"headline"`
		Summary     string            `json:"summary"`
		Links       []modelLink       `json:"links"`
		Experiences []modelExperience `json:"experiences"`
		Education   []modelEducation  `json:"education"`
		Skills      []modelSkillGroup `json:"skills"`
	}
)

// toProfile turns schema-valid model JSON into a clean, saveable draft. Any
// failure means the output can't be trusted, and the caller rejects it.
func toProfile(text string) (profile.Profile, error) {
	dec := json.NewDecoder(bytes.NewReader([]byte(text)))
	dec.DisallowUnknownFields()
	var m modelProfile
	if err := dec.Decode(&m); err != nil {
		return profile.Profile{}, errors.New("model output does not decode")
	}
	p := profile.Profile{
		FullName: m.FullName, Email: m.Email, Phone: m.Phone,
		Location: m.Location, Headline: m.Headline, Summary: m.Summary,
	}
	for _, l := range m.Links {
		p.Links = append(p.Links, profile.Link{Label: l.Label, URL: withScheme(l.URL)})
	}
	for i, e := range m.Experiences {
		start, end, err := dates(e.Start, e.End)
		if err != nil {
			return profile.Profile{}, fmt.Errorf("experience %d: %w", i+1, err)
		}
		if e.Company == "" && e.Title == "" {
			continue // the model emitted an empty row; drop it
		}
		p.Experiences = append(p.Experiences, profile.Experience{
			Company: e.Company, Title: e.Title, Location: e.Location,
			Start: start, End: end, Current: e.Current && end.IsZero(), Bullets: e.Bullets,
		})
	}
	for i, e := range m.Education {
		start, end, err := dates(e.Start, e.End)
		if err != nil {
			return profile.Profile{}, fmt.Errorf("education %d: %w", i+1, err)
		}
		if e.School == "" && e.Degree == "" {
			continue
		}
		p.Education = append(p.Education, profile.Education{
			School: e.School, Degree: e.Degree, Field: e.Field, Location: e.Location,
			Start: start, End: end, Details: e.Details,
		})
	}
	for _, g := range m.Skills {
		p.Skills = append(p.Skills, profile.SkillGroup{Category: g.Category, Items: g.Items})
	}
	return profile.Clean(p)
}

func dates(start, end string) (profile.YearMonth, profile.YearMonth, error) {
	s, err := profile.ParseYearMonth(strings.TrimSpace(start))
	if err != nil {
		return s, s, errors.New("start date is not YYYY-MM or YYYY")
	}
	e, err := profile.ParseYearMonth(strings.TrimSpace(end))
	if err != nil {
		return s, e, errors.New("end date is not YYYY-MM or YYYY")
	}
	return s, e, nil
}

// withScheme adds https:// to a bare domain ("github.com/x"). Anything with
// another scheme is left for validation to reject.
func withScheme(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, "://") || strings.Contains(raw, ":") {
		return raw
	}
	if u, err := url.Parse("https://" + raw); err == nil && strings.Contains(u.Host, ".") {
		return u.String()
	}
	return raw
}
