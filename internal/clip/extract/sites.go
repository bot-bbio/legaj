package extract

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// A site adapter knows one job platform's page conventions. The patterns
// below were checked against public (logged-out) pages on 2026-11-03; the
// spike captures (plan item 2) confirm or extend them for the views people
// actually clip, such as LinkedIn logged in.
type site struct {
	name    string
	domains []string
	// extract reads what the platform puts in its title, meta tags and text.
	extract func(p Page) candidates
	// cleanCompany, when set, fixes this platform's company quirks in every
	// layer's candidates (JSON-LD included).
	cleanCompany func(string) string
}

var sites = []*site{
	{name: "greenhouse", domains: []string{"greenhouse.io"}, extract: greenhouse},
	{name: "lever", domains: []string{"lever.co"}, extract: lever},
	{name: "ashby", domains: []string{"ashbyhq.com"}, extract: ashby},
	{name: "workday", domains: []string{"myworkdayjobs.com", "myworkdaysite.com"}, cleanCompany: workdayCompany},
	{name: "linkedin", domains: []string{"linkedin.com"}, extract: linkedIn},
}

// siteFor returns the adapter for host, or nil.
func siteFor(host string) *site {
	for _, s := range sites {
		for _, d := range s.domains {
			if onDomain(host, d) {
				return s
			}
		}
	}
	return nil
}

// fixCompany applies the site's company cleanup. Safe on a nil site.
func (s *site) fixCompany(c string) string {
	if s == nil || s.cleanCompany == nil {
		return c
	}
	return s.cleanCompany(c)
}

// Greenhouse job boards: "Job Application for <role> at <company>". The
// role group is greedy, so "Engineer at Scale at Acme" splits at the last
// " at ". og:description holds the location there.
var greenhouseTitle = regexp.MustCompile(`^Job Application for (.+) at (.+)$`)

func greenhouse(p Page) candidates {
	var c candidates
	if m := greenhouseTitle.FindStringSubmatch(cleanLine(p.Title, false)); m != nil {
		c.role, c.company = []string{m[1]}, []string{m[2]}
	}
	if loc := cleanLine(p.Meta["og:description"], false); utf8.RuneCountInString(loc) <= 80 {
		c.location = []string{loc}
	}
	if len(p.H1) > 0 {
		c.description = []string{greenhouseDescription(p.Text, cleanLine(p.H1[0], false))}
	}
	return c
}

// greenhouseDescription cuts the posting out of the page text: it starts
// after the "Apply" button below the role heading and ends where the
// application form starts. "" when the layout isn't recognised.
func greenhouseDescription(text, role string) string {
	lines := strings.Split(normalizeText(text), "\n")
	start := -1
	for i := 0; i < len(lines) && i < 20; i++ {
		if lines[i] == role {
			for j := i + 1; j < len(lines) && j <= i+4; j++ {
				if strings.EqualFold(lines[j], "Apply") {
					start = j + 1
					break
				}
			}
			break
		}
	}
	if start < 0 {
		return ""
	}
	end := len(lines)
	for i := start; i < len(lines); i++ {
		if strings.EqualFold(lines[i], "Apply for this job") {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}

// Lever: "<company> - <role>". Company names rarely contain " - "; roles
// often do, so the split is at the first one.
var leverTitle = regexp.MustCompile(`^(.+?) - (.+)$`)

func lever(p Page) candidates {
	var c candidates
	if m := leverTitle.FindStringSubmatch(cleanLine(p.Title, false)); m != nil {
		c.company, c.role = []string{m[1]}, []string{m[2]}
	}
	return c
}

// Ashby: "<role> @ <company>", split at the last " @ ".
var ashbyTitle = regexp.MustCompile(`^(.+) @ (.+)$`)

func ashby(p Page) candidates {
	var c candidates
	if m := ashbyTitle.FindStringSubmatch(cleanLine(p.Title, false)); m != nil {
		c.role, c.company = []string{m[1]}, []string{m[2]}
	}
	return c
}

// Workday puts a numeric legal-entity code before the employer name
// ("2100 NVIDIA USA"). Elsewhere a leading number can be part of a name
// ("500 Startups"), so this runs only on Workday pages.
var workdayEntityCode = regexp.MustCompile(`^\d{2,}\s+(\pL.*)$`)

func workdayCompany(c string) string {
	if m := workdayEntityCode.FindStringSubmatch(c); m != nil {
		return m[1]
	}
	return c
}

// LinkedIn logged-out titles, English and Polish:
// "<role> at <company> — <place> | LinkedIn Jobs" and
// "<role> w firmie <company> — <place> | Oferty pracy na LinkedIn".
// A "(3) " unread-notification count may come first.
var (
	linkedInTitles = []*regexp.Regexp{
		regexp.MustCompile(`^(.+) at (.+?) — (.+?) \| [^|]*LinkedIn[^|]*$`),
		regexp.MustCompile(`^(.+) w firmie (.+?) — (.+?) \| [^|]*LinkedIn[^|]*$`),
	}
	notificationCount = regexp.MustCompile(`^\(\d+\+?\)\s*`)
)

func linkedIn(p Page) candidates {
	var c candidates
	title := notificationCount.ReplaceAllString(cleanLine(p.Title, false), "")
	for _, re := range linkedInTitles {
		if m := re.FindStringSubmatch(title); m != nil {
			c.role, c.company, c.location = []string{m[1]}, []string{m[2]}, []string{m[3]}
			break
		}
	}
	return c
}

// careersTitle matches "<role> | <company> Careers" and similar. The
// separator needs spaces around it, so hyphenated roles stay whole.
var (
	careersSuffix = regexp.MustCompile(`(?i)^(.+?)\s+[|·–—-]\s+(.+?)\s+(?:careers|jobs|career site|job board)$`)
	careersAt     = regexp.MustCompile(`(?i)^(.+?)\s+[|·–—-]\s+(?:careers|jobs)\s+(?:at|@)\s+(.+)$`)
)

// splitCareersTitle returns the role and, when the title names it, the
// company. Other titles come back whole as the role.
func splitCareersTitle(t string) (role, company string) {
	for _, re := range []*regexp.Regexp{careersSuffix, careersAt} {
		if m := re.FindStringSubmatch(t); m != nil {
			return m[1], m[2]
		}
	}
	return t, ""
}

// generic are page-chrome words that are never a company or a role. The
// list started from V1's clipper, which saved "Careers" as an employer.
var generic = []string{
	"jobs", "job", "careers", "career", "apply", "apply now", "apply here", "job search", "search jobs",
	"find jobs", "all jobs", "view job", "job posting", "job postings", "job details", "job description",
	"job board", "job listings", "job application", "hiring", "we're hiring", "join us", "join our team",
	"open roles", "open positions", "current openings", "work with us", "career opportunities",
	"job opportunities", "home", "sign in", "log in", "join now", "page not found", "not found",
}

// platforms are job boards and applicant-tracking systems: the site, not
// the employer.
var platforms = []string{
	"linkedin", "linkedin jobs", "indeed", "indeed.com", "glassdoor", "greenhouse", "lever", "ashby",
	"workday", "icims", "smartrecruiters", "ziprecruiter", "monster", "google", "pracuj.pl", "justjoin.it",
}

func isGeneric(s string) bool {
	l := strings.ToLower(s)
	return slices.Contains(generic, l) || slices.Contains(platforms, l)
}

// hasLetterOrDigit rejects candidates made only of punctuation.
func hasLetterOrDigit(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0
}

// plausibleCompany rejects empty, over-long or generic names, links, and
// the page's own host.
func plausibleCompany(c, host string) bool {
	n := utf8.RuneCountInString(c)
	l := strings.ToLower(c)
	switch {
	case n < 2 || n > MaxDetail || !hasLetterOrDigit(c) || isGeneric(c):
		return false
	case strings.ContainsAny(c, `/\`), strings.HasPrefix(l, "http:"), strings.HasPrefix(l, "https:"), strings.HasPrefix(l, "www."):
		return false
	case host != "" && (l == host || onDomain(host, l)):
		return false
	}
	return true
}

// plausibleRole rejects empty, over-long or generic roles, and a role that
// is just the company's name or the host.
func plausibleRole(r, company, host string) bool {
	n := utf8.RuneCountInString(r)
	switch {
	case n < 2 || n > MaxDetail || !hasLetterOrDigit(r) || isGeneric(r):
		return false
	case company != "" && strings.EqualFold(r, company):
		return false
	case host != "" && strings.EqualFold(r, host):
		return false
	}
	return true
}
