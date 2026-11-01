package profile

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/bot-bbio/legaj/internal/apperr"
)

// Limits keep the profile (and every prompt built from it) a sane size.
const (
	maxShort       = 200  // names, titles, places, single skills
	maxBullet      = 1000 // one bullet, one details paragraph, one URL
	maxSummary     = 4000
	maxExperiences = 50 // R8 caps an import at 20; manual editing allows more
	maxEducation   = 20
	maxBullets     = 30 // per experience
	maxSkillGroups = 30
	maxSkillItems  = 100 // per group
	maxLinks       = 10
	minYear        = 1800 // early enough for A5's 1842 test dates
	maxYear        = 2100
	maxIDLen       = 32
)

// normalize trims every string, drops empty bullets, skill items and skill
// groups (R9), and replaces nil lists with empty ones.
func normalize(p Profile) Profile {
	t := strings.TrimSpace
	p.FullName, p.Email, p.Phone, p.Location = t(p.FullName), t(p.Email), t(p.Phone), t(p.Location)
	p.Headline, p.TargetRole, p.Summary = t(p.Headline), t(p.TargetRole), t(p.Summary)

	links := []Link{}
	for _, l := range p.Links {
		l.Label, l.URL = t(l.Label), t(l.URL)
		if l.URL != "" {
			links = append(links, l)
		}
	}
	p.Links = links

	exps := make([]Experience, 0, len(p.Experiences))
	for _, e := range p.Experiences {
		e.ID, e.Company, e.Title, e.Location = t(e.ID), t(e.Company), t(e.Title), t(e.Location)
		e.Bullets = nonEmpty(e.Bullets)
		exps = append(exps, e)
	}
	p.Experiences = exps

	edu := make([]Education, 0, len(p.Education))
	for _, e := range p.Education {
		e.ID, e.School, e.Degree, e.Field, e.Location, e.Details = t(e.ID), t(e.School), t(e.Degree), t(e.Field), t(e.Location), t(e.Details)
		edu = append(edu, e)
	}
	p.Education = edu

	skills := []SkillGroup{}
	for _, g := range p.Skills {
		g.Category = t(g.Category)
		g.Items = nonEmpty(g.Items)
		if len(g.Items) > 0 {
			skills = append(skills, g)
		}
	}
	p.Skills = skills
	return p
}

func nonEmpty(in []string) []string {
	out := []string{}
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

var emailRE = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

func invalid(format string, args ...any) error {
	return apperr.New(apperr.CodeInvalid, fmt.Sprintf(format, args...))
}

func tooLong(s string, limit int) bool { return utf8.RuneCountInString(s) > limit }

// Validate checks a normalized profile. Messages name the item to fix.
func Validate(p Profile) error {
	for name, v := range map[string]string{"Name": p.FullName, "Phone": p.Phone, "Location": p.Location, "Headline": p.Headline, "Target role": p.TargetRole, "Email": p.Email} {
		if tooLong(v, maxShort) {
			return invalid("%s is too long (at most %d characters).", name, maxShort)
		}
	}
	if tooLong(p.Summary, maxSummary) {
		return invalid("Summary is too long (at most %d characters).", maxSummary)
	}
	if p.Email != "" && !emailRE.MatchString(p.Email) {
		return invalid("That email address doesn't look right.")
	}
	if len(p.Links) > maxLinks {
		return invalid("Keep it to %d links.", maxLinks)
	}
	for i, l := range p.Links {
		u, err := url.Parse(l.URL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || tooLong(l.URL, maxBullet) || tooLong(l.Label, maxShort) {
			return invalid("Link %d must be a web address starting with https://.", i+1)
		}
	}
	if len(p.Experiences) > maxExperiences {
		return invalid("Keep it to %d experiences.", maxExperiences)
	}
	for i, e := range p.Experiences {
		n := i + 1
		if e.Company == "" && e.Title == "" {
			return invalid("Experience %d needs a company or a title.", n)
		}
		if tooLong(e.Company, maxShort) || tooLong(e.Title, maxShort) || tooLong(e.Location, maxShort) || tooLong(e.ID, maxIDLen) {
			return invalid("Experience %d has a field that's too long.", n)
		}
		if err := checkRange(e.Start, e.End); err != nil {
			return invalid("Experience %d: %s", n, err.Error())
		}
		if e.Current && !e.End.IsZero() {
			return invalid("Experience %d is marked current, so it can't have an end date.", n)
		}
		if len(e.Bullets) > maxBullets {
			return invalid("Experience %d has more than %d bullets.", n, maxBullets)
		}
		for _, b := range e.Bullets {
			if tooLong(b, maxBullet) {
				return invalid("Experience %d has a bullet over %d characters.", n, maxBullet)
			}
		}
	}
	if len(p.Education) > maxEducation {
		return invalid("Keep it to %d education entries.", maxEducation)
	}
	for i, e := range p.Education {
		n := i + 1
		if e.School == "" && e.Degree == "" {
			return invalid("Education %d needs a school or a degree.", n)
		}
		if tooLong(e.School, maxShort) || tooLong(e.Degree, maxShort) || tooLong(e.Field, maxShort) || tooLong(e.Location, maxShort) || tooLong(e.Details, maxBullet) || tooLong(e.ID, maxIDLen) {
			return invalid("Education %d has a field that's too long.", n)
		}
		if err := checkRange(e.Start, e.End); err != nil {
			return invalid("Education %d: %s", n, err.Error())
		}
	}
	if len(p.Skills) > maxSkillGroups {
		return invalid("Keep it to %d skill groups.", maxSkillGroups)
	}
	for i, g := range p.Skills {
		if tooLong(g.Category, maxShort) || len(g.Items) > maxSkillItems {
			return invalid("Skill group %d is too large.", i+1)
		}
		for _, s := range g.Items {
			if tooLong(s, maxShort) {
				return invalid("Skill group %d has a skill over %d characters.", i+1, maxShort)
			}
		}
	}
	return nil
}

func (d YearMonth) validate() error {
	if d.IsZero() {
		return nil
	}
	if d.Year < minYear || d.Year > maxYear {
		return fmt.Errorf("the year must be between %d and %d", minYear, maxYear)
	}
	if d.Month < 0 || d.Month > 12 {
		return fmt.Errorf("the month must be 1-12")
	}
	return nil
}

func checkRange(start, end YearMonth) error {
	if start.Year == 0 && start.Month != 0 || end.Year == 0 && end.Month != 0 {
		return fmt.Errorf("a month needs a year")
	}
	if err := start.validate(); err != nil {
		return fmt.Errorf("start date: %w", err)
	}
	if err := end.validate(); err != nil {
		return fmt.Errorf("end date: %w", err)
	}
	if !start.IsZero() && !end.IsZero() && end.before(start) {
		return fmt.Errorf("the end date is before the start date")
	}
	return nil
}
