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
	{name: "indeed", domains: []string{"indeed.com", "indeed.co.uk", "indeed.ca", "indeed.de", "indeed.fr", "indeed.com.au", "indeed.co.in"}, extract: indeed},
}

// lineIndex returns the index of the first line in lines[from:to] equal to
// one of want, or -1.
func lineIndex(lines []string, from, to int, want ...string) int {
	for i := max(from, 0); i < min(to, len(lines)); i++ {
		if slices.Contains(want, lines[i]) {
			return i
		}
	}
	return -1
}

// nextNonEmpty returns the index of the first non-empty line at or after i
// (len(lines) when there's none).
func nextNonEmpty(lines []string, i int) int {
	for i < len(lines) && lines[i] == "" {
		i++
	}
	return i
}

// lastLineIndex returns the index of the last line in lines[:before] equal
// to want, or -1.
func lastLineIndex(lines []string, before int, want string) int {
	for i := min(before, len(lines)) - 1; i >= 0; i-- {
		if lines[i] == want {
			return i
		}
	}
	return -1
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
	// The posting ends at the job-alert box, or at the application form.
	end := lineIndex(lines, start, len(lines), "Create a Job Alert", "Apply for this job")
	if end < 0 {
		end = len(lines)
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

// linkedInMemberTitle is the logged-in title (spike captures, 2026-11-04):
// "<role> | <company> | LinkedIn".
var linkedInMemberTitle = regexp.MustCompile(`^(.+) \| (.+?) \| LinkedIn$`)

// linkedInMemberEnd ends the posting on a logged-in page: the "… more"
// toggle, or the boxes that follow it.
var linkedInMemberEnd = []string{"… more", "See more", "Show more", "Show less", "Benefits found in job post",
	"Set alert for similar jobs", "About the company", "Meet the hiring team"}

func linkedIn(p Page) candidates {
	var c candidates
	title := notificationCount.ReplaceAllString(cleanLine(p.Title, false), "")
	for _, re := range linkedInTitles {
		if m := re.FindStringSubmatch(title); m != nil {
			c.role, c.company, c.location = []string{m[1]}, []string{m[2]}, []string{m[3]}
			break
		}
	}
	lines := strings.Split(normalizeText(p.Text), "\n")
	if m := linkedInMemberTitle.FindStringSubmatch(title); m != nil && c.role == nil {
		c.role, c.company = []string{m[1]}, []string{m[2]}
		c.location = []string{linkedInMemberLocation(lines, m[1])}
	}
	c.description = []string{linkedInMemberDescription(lines), linkedInDescription(lines)}
	return c
}

// linkedInMemberLocation reads the line under the job's own header: the
// last role line before "About the job" (a search page lists the role in
// its results first), then "<place> · <posted> · <applicants>".
func linkedInMemberLocation(lines []string, role string) string {
	about := lineIndex(lines, 0, len(lines), "About the job")
	if about < 0 {
		return ""
	}
	r := lastLineIndex(lines, about, role)
	next := nextNonEmpty(lines, r+1)
	if r < 0 || next >= about {
		return ""
	}
	place, _, found := strings.Cut(lines[next], " · ")
	if !found {
		return ""
	}
	return place
}

// linkedInMemberDescription cuts the posting out of a logged-in page: after
// "About the job", up to its "… more" toggle or the next box. "" when the
// layout isn't recognised.
func linkedInMemberDescription(lines []string) string {
	about := lineIndex(lines, 0, len(lines), "About the job")
	if about < 0 {
		return ""
	}
	end := lineIndex(lines, about+1, len(lines), linkedInMemberEnd...)
	if end < 0 {
		end = len(lines)
	}
	return strings.TrimSpace(strings.Join(lines[about+1:end], "\n"))
}

// indeedTitle is the job page's title (spike captures, 2026-11-04):
// "<role> - <place> - Indeed.com". The role group is greedy, since roles
// contain " - " and " | " ("Product Manager - Apparel").
var indeedTitle = regexp.MustCompile(`^(.+) - (.+?) - Indeed(?:\.com)?$`)

// indeed reads both Indeed layouts. On a job page the role and place come
// from the title and the company is the line under the role; on a search
// page the selected job sits after "Job Post Details" as role, company,
// "•", place. The posting runs from "Full job description" to "Explore
// other jobs" or "Report job".
func indeed(p Page) candidates {
	var c candidates
	lines := strings.Split(normalizeText(p.Text), "\n")
	full := lineIndex(lines, 0, len(lines), "Full job description")
	if full < 0 {
		return c
	}
	// The header is read line by line, ignoring blank lines.
	header := slices.DeleteFunc(slices.Clone(lines[:full]), func(l string) bool { return l == "" })
	if d := lastLineIndex(header, len(header), "Job Post Details"); d >= 0 && d+2 < len(header) {
		c.role, c.company = []string{header[d+1]}, []string{header[d+2]}
		if d+4 < len(header) && header[d+3] == "•" {
			c.location = []string{header[d+4]}
		}
	} else if m := indeedTitle.FindStringSubmatch(cleanLine(p.Title, false)); m != nil {
		c.role, c.location = []string{m[1]}, []string{m[2]}
		if r := lineIndex(header, 0, len(header), m[1]); r >= 0 && r+1 < len(header) {
			c.company = []string{header[r+1]}
		}
	}
	end := lineIndex(lines, full+1, len(lines), "Explore other jobs", "Report job")
	if end < 0 {
		end = len(lines)
	}
	c.description = []string{strings.Join(lines[full+1:end], "\n")}
	return c
}

// LinkedIn's sign-in consent line and "report this job" link, in English
// and Polish, as seen on logged-out pages on 2026-11-03.
var (
	linkedInConsent = []string{"By clicking Continue to join or sign in", "Klikając Kontynuuj, aby dołączyć lub się zalogować"}
	linkedInReport  = []string{"Report this job", "Zgłoś tę ofertę pracy"}
)

// linkedInDescription cuts the posting out of a logged-out LinkedIn page:
// sign-in prompts, each ending with a consent line (sometimes followed by
// "report this job"), come before it, and "Show more" / "Show less" (in
// English on the Polish site too) right after it. "" when the layout
// isn't recognised.
func linkedInDescription(lines []string) string {
	end := -1
	for i := 0; i+1 < len(lines); i++ {
		if lines[i] == "Show more" && lines[i+1] == "Show less" {
			end = i
			break
		}
	}
	for i := end - 1; i >= 0; i-- {
		if !slices.ContainsFunc(linkedInConsent, func(c string) bool { return strings.HasPrefix(lines[i], c) }) {
			continue
		}
		start := i + 1
		for start < end && slices.Contains(linkedInReport, lines[start]) {
			start++
		}
		return strings.TrimSpace(strings.Join(lines[start:end], "\n"))
	}
	return ""
}

// careersTitle matches "<role> | <company> Careers" and similar. The
// separator needs spaces around it, so hyphenated roles stay whole, and
// the role group is greedy: "Engineer - Backend | Acme Careers" splits at
// the last separator.
var (
	// "<role> - <company> | Careers": a separator before the word too.
	careersSeparated = regexp.MustCompile(`(?i)^(.+)\s+[|·–—-]\s+(.+?)\s+[|·–—-]\s+(?:careers|jobs|career site|job board)$`)
	careersSuffix    = regexp.MustCompile(`(?i)^(.+)\s+[|·–—-]\s+(.+?)\s+(?:careers|jobs|career site|job board)$`)
	careersAt        = regexp.MustCompile(`(?i)^(.+?)\s+[|·–—-]\s+(?:careers|jobs)\s+(?:at|@)\s+(.+)$`)
	// spacedSeparator in a company means a split went wrong ("Acme |").
	spacedSeparator = regexp.MustCompile(`(?:^|\s)[|·–—-](?:\s|$)`)
)

// splitCareersTitle returns the role and, when the title names it, the
// company. Other titles come back whole as the role.
func splitCareersTitle(t string) (role, company string) {
	for _, re := range []*regexp.Regexp{careersSeparated, careersSuffix, careersAt} {
		if m := re.FindStringSubmatch(t); m != nil && !spacedSeparator.MatchString(m[2]) {
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

// boards are job boards and applicant-tracking systems, by name, with the
// host labels of their own sites. On those sites the name is the site, not
// the employer; elsewhere it can be a real employer (Workday hiring on
// workday.com).
var boards = map[string][]string{
	"linkedin": {"linkedin"}, "linkedin jobs": {"linkedin"}, "indeed": {"indeed"}, "indeed.com": {"indeed"},
	"glassdoor": {"glassdoor"}, "greenhouse": {"greenhouse"}, "lever": {"lever"}, "ashby": {"ashbyhq"},
	"workday": {"myworkdayjobs", "myworkdaysite"}, "icims": {"icims"}, "smartrecruiters": {"smartrecruiters"},
	"ziprecruiter": {"ziprecruiter"}, "monster": {"monster"}, "pracuj.pl": {"pracuj"}, "justjoin.it": {"justjoin"},
	"mygreenhouse": {"greenhouse"}, // Greenhouse's candidate portal (spike capture)
}

// searchHeading is a search page's heading or title, never a role: "product
// manager jobs in New York State", "Now Hiring: 75 … Jobs".
var searchHeading = regexp.MustCompile(`(?i)^now hiring\b|\bjobs (?:in|near|at)\b|^browse\b`)

// boardRule says when a job board's name is page chrome.
type boardRule int

const (
	// boardsAllowed: JSON-LD's hiringOrganization names the employer
	// explicitly, even when it's the board itself (LinkedIn hiring on
	// LinkedIn).
	boardsAllowed boardRule = iota
	// boardsOnTheirSite: on page text, a board's name is chrome only on
	// that board's own site.
	boardsOnTheirSite
	// boardsNever: a model's answer never names a board as the employer.
	boardsNever
)

// isBoard reports whether name is a job board's, under rule, for a page on host.
func isBoard(name, host string, rule boardRule) bool {
	labels, ok := boards[strings.ToLower(name)]
	switch {
	case !ok || rule == boardsAllowed:
		return false
	case rule == boardsNever:
		return true
	}
	hostLabels := strings.Split(host, ".")
	return slices.ContainsFunc(labels, func(l string) bool { return slices.Contains(hostLabels, l) })
}

func isGeneric(s string) bool { return slices.Contains(generic, strings.ToLower(s)) }

// hasLetterOrDigit rejects candidates made only of punctuation.
func hasLetterOrDigit(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0
}

// plausibleCompany rejects empty, over-long or generic names, links, job
// boards (see boardRule) and the page's own host. A name that is a domain
// ("Booking.com" on jobs.booking.com) is fine.
func plausibleCompany(c, host string, rule boardRule) bool {
	n := utf8.RuneCountInString(c)
	l := strings.ToLower(c)
	switch {
	case n < 2 || n > MaxDetail || !hasLetterOrDigit(c) || isGeneric(c) || isBoard(c, host, rule):
		return false
	case strings.ContainsAny(c, `/\`), strings.HasPrefix(l, "http:"), strings.HasPrefix(l, "https:"), strings.HasPrefix(l, "www."):
		return false
	case host != "" && l == host:
		return false
	}
	return true
}

// plausibleRole rejects empty, over-long or generic roles, a board's name,
// and a role that is just the company's name or the host.
func plausibleRole(r, company, host string) bool {
	n := utf8.RuneCountInString(r)
	switch {
	case n < 2 || n > MaxDetail || !hasLetterOrDigit(r) || isGeneric(r) || isBoard(r, host, boardsNever) || searchHeading.MatchString(r):
		return false
	case company != "" && strings.EqualFold(r, company):
		return false
	case host != "" && strings.EqualFold(r, host):
		return false
	}
	return true
}
