package letters

import (
	"context"
	"strings"
	"unicode"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/profile"
	"github.com/bot-bbio/legaj/internal/store"
)

// legalSuffixes are trailing words of a registered company name that
// people (and models) usually drop: "Acme, Inc." is written "Acme".
var legalSuffixes = map[string]bool{
	"inc": true, "incorporated": true, "llc": true, "llp": true, "ltd": true, "limited": true,
	"corp": true, "corporation": true, "co": true, "company": true, "plc": true, "gmbh": true,
	"ag": true, "kg": true, "sa": true, "sas": true, "spa": true, "srl": true, "bv": true,
	"nv": true, "oy": true, "ab": true, "as": true, "pty": true, "pte": true,
	// Polish "Sp. z o.o.".
	"sp": true, "z": true, "o": true,
}

// words splits text into lower-case words (runs of letters and digits).
func words(s string) []string { return caseWords(strings.ToLower(s)) }

// caseWords splits text into words, keeping their case.
func caseWords(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// core is a company name without its trailing legal suffixes.
func core(name string) []string {
	w := words(name)
	end := len(w)
	for end > 1 && legalSuffixes[w[end-1]] {
		end--
	}
	return w[:end]
}

// contains reports whether needle occurs in hay as consecutive words.
func contains(hay, needle []string) bool {
	if len(needle) == 0 || len(needle) > len(hay) {
		return false
	}
	for i := 0; i+len(needle) <= len(hay); i++ {
		match := true
		for j, w := range needle {
			if hay[i+j] != w {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// CompanyRule checks that generated text is about the right company
// (R21): it names this application's company and no other company from
// the user's pipeline. Past employers from the profile are always allowed.
type CompanyRule struct {
	own    []string
	others []otherCompany
}

type otherCompany struct {
	name string
	core []string // lower case, for comparing names
	// exact is the core as written ("Ramp"): another company is only
	// recognised as a proper noun, so "ramp up" or "out of the box" in
	// ordinary prose never counts as naming Ramp or Box.
	exact []string
}

// NewCompanyRule builds the rule. others are the pipeline's other company
// names; exempt are names that may appear anyway (past employers). Names
// that overlap with this company's (Acme vs Acme Labs) or are too short to
// be distinctive are left out, so a correct letter is never rejected.
func NewCompanyRule(company string, others, exempt []string) CompanyRule {
	r := CompanyRule{own: core(company)}
	var skip [][]string
	for _, e := range exempt {
		if c := core(e); len(c) > 0 {
			skip = append(skip, c)
		}
	}
	seen := map[string]bool{}
	for _, name := range others {
		c := core(name)
		key := strings.Join(c, " ")
		if len(c) == 0 || seen[key] || (len(c) == 1 && len([]rune(c[0])) < 3) {
			continue
		}
		if contains(r.own, c) || contains(c, r.own) {
			continue
		}
		overlaps := false
		for _, s := range skip {
			if contains(s, c) || contains(c, s) {
				overlaps = true
				break
			}
		}
		if overlaps {
			continue
		}
		seen[key] = true
		r.others = append(r.others, otherCompany{name: strings.TrimSpace(name), core: c, exact: caseWords(name)[:len(c)]})
	}
	return r
}

// Names reports whether text names this company (ignoring case,
// punctuation and legal suffixes). With no company name, anything goes.
func (r CompanyRule) Names(text string) bool {
	return len(r.own) == 0 || contains(words(text), r.own)
}

// Wrong returns another pipeline company that text names, or "". The
// name must appear as written (a proper noun), not just as lower-case words.
func (r CompanyRule) Wrong(text string) string {
	w := caseWords(text)
	for _, o := range r.others {
		if contains(w, o.exact) {
			return o.name
		}
	}
	return ""
}

// maxOtherCompanies bounds the names checked per draft.
const maxOtherCompanies = 2000

// LoadCompanyRule builds the rule for an application from the live
// pipeline and the profile's past employers.
func LoadCompanyRule(ctx context.Context, db *store.DB, app store.Application, p profile.Profile) (CompanyRule, error) {
	rows, err := db.Reader().QueryContext(ctx, `SELECT DISTINCT company FROM applications
		WHERE deleted_at IS NULL AND id <> ? AND company <> '' LIMIT ?`, app.ID, maxOtherCompanies)
	if err != nil {
		return CompanyRule{}, apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't read your applications.")
	}
	defer rows.Close()
	var others []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return CompanyRule{}, apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't read your applications.")
		}
		others = append(others, c)
	}
	if err := rows.Err(); err != nil {
		return CompanyRule{}, apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't read your applications.")
	}
	exempt := make([]string, 0, len(p.Experiences))
	for _, e := range p.Experiences {
		exempt = append(exempt, e.Company)
	}
	return NewCompanyRule(app.Company, others, exempt), nil
}
