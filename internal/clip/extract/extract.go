// Package extract turns a page snapshot from the bookmarklet into a job:
// company, role, location and description (R19, D15).
//
// Layers run in a fixed order: JSON-LD JobPosting, then a site adapter,
// then meta tags and headings, then the user's selection, then the page's
// main text; for the description a selection comes before the adapter's
// cut of the page text (D30). Each layer only fills the fields earlier
// layers left empty,
// and every candidate is cleaned and checked before it's accepted, so a
// later layer can still answer when an earlier one offers junk ("Careers",
// a host name).
//
// Everything in a Page comes from an arbitrary web page. Nothing here
// executes, fetches or renders it: values are parsed as data, stripped to
// plain text and bounded in size.
package extract

import (
	"net/url"
	"strings"
	"unicode/utf8"
)

// Page is the page snapshot the bookmarklet collects (clip payload v1).
// Field names match the payload's JSON.
type Page struct {
	URL       string            `json:"url"`
	Title     string            `json:"title"`
	JSONLD    []string          `json:"jsonld"` // the text of each <script type="application/ld+json">
	Meta      map[string]string `json:"meta"`   // og:*, twitter:*, description, title, author
	H1        []string          `json:"h1"`
	Selection string            `json:"selection"` // what the user had selected, if anything
	Text      string            `json:"text"`      // the main element's rendered text
}

// Layer names the extraction layer a field came from.
type Layer string

// Extraction layers, in the order they run (R19).
const (
	LayerNone      Layer = ""
	LayerJSONLD    Layer = "jsonld"
	LayerAdapter   Layer = "adapter"
	LayerMeta      Layer = "meta" // meta tags, headings and the page title
	LayerSelection Layer = "selection"
	LayerText      Layer = "text"
)

const (
	// MaxDetail bounds company, role and location, in characters. It matches
	// the pipeline's limit for the same fields.
	MaxDetail = 200
	// MaxDescription bounds the description, in bytes. It matches
	// pipeline.MaxDescription.
	MaxDescription = 100_000
	// minSelection is the shortest selection taken as a description, in
	// characters. A shorter one is usually a stray double-click.
	minSelection = 100
)

// Sources records which layer supplied each field.
type Sources struct {
	Company     Layer `json:"company"`
	Role        Layer `json:"role"`
	Location    Layer `json:"location"`
	Description Layer `json:"description"`
}

// Result is the extracted job. An empty field means no layer found a
// plausible value: an empty Company is the "Needs company" case.
type Result struct {
	Company     string  `json:"company"`
	Role        string  `json:"role"`
	Location    string  `json:"location"`
	Description string  `json:"description"`
	From        Sources `json:"from"`
}

// candidates are one layer's offers for each field, best first.
type candidates struct {
	company, role, location, description []string
	// markup is true when values may hold HTML tags or entities (JSON-LD
	// strings). Values read from the rendered page are plain text, where
	// "<div>" is literal text.
	markup bool
}

// Extract runs the layers over p.
func Extract(p Page) Result {
	host := hostOf(p.URL)
	s := siteFor(host)
	var r Result
	r.offer(LayerJSONLD, fromJSONLD(p.JSONLD), s, host)
	var adapter candidates
	if s != nil && s.extract != nil {
		adapter = s.extract(p)
	}
	// An adapter's description is a heuristic cut of the same page text the
	// user can select from, so a real selection beats it (D30).
	adapterDescription := adapter.description
	adapter.description = nil
	r.offer(LayerAdapter, adapter, s, host)
	r.offer(LayerMeta, fromMeta(p), s, host)
	if utf8.RuneCountInString(strings.TrimSpace(p.Selection)) >= minSelection {
		r.offer(LayerSelection, candidates{description: []string{p.Selection}}, s, host)
	}
	r.offer(LayerAdapter, candidates{description: adapterDescription, markup: adapter.markup}, s, host)
	r.offer(LayerText, candidates{description: []string{p.Text}}, s, host)
	// A meta description is a teaser, so it's the last resort.
	r.offer(LayerMeta, candidates{description: []string{p.Meta["description"], p.Meta["og:description"]}}, s, host)
	return r
}

// offer fills each still-empty field with the layer's first acceptable
// candidate.
func (r *Result) offer(l Layer, c candidates, s *site, host string) {
	if r.Company == "" {
		rule := boardsOnTheirSite
		if l == LayerJSONLD {
			rule = boardsAllowed
		}
		for _, v := range c.company {
			v = s.fixCompany(cleanLine(v, c.markup))
			if plausibleCompany(v, host, rule) {
				r.Company, r.From.Company = v, l
				break
			}
		}
	}
	if r.Role == "" {
		for _, v := range c.role {
			v = cleanLine(v, c.markup)
			if plausibleRole(v, r.Company, host) && (l == LayerJSONLD || !searchHeading.MatchString(v)) {
				r.Role, r.From.Role = v, l
				break
			}
		}
	}
	if r.Location == "" {
		for _, v := range c.location {
			if v = fitDetail(cleanLine(v, c.markup)); v != "" {
				r.Location, r.From.Location = v, l
				break
			}
		}
	}
	if r.Description == "" {
		for _, v := range c.description {
			if v = cleanBlock(v, c.markup); v != "" {
				r.Description, r.From.Description = v, l
				break
			}
		}
	}
}

// fromMeta offers the role from the first heading, the social titles or
// the page title, and the company from og:site_name or a "… | Acme
// Careers" title.
func fromMeta(p Page) candidates {
	var c candidates
	var titles []string
	if len(p.H1) > 0 {
		titles = append(titles, p.H1[0])
	}
	titles = append(titles, p.Meta["og:title"], p.Meta["twitter:title"], p.Title)
	for _, t := range titles {
		role, company := splitCareersTitle(cleanLine(t, false))
		c.role = append(c.role, role)
		if company != "" {
			c.company = append(c.company, company)
		}
	}
	c.company = append(c.company, p.Meta["og:site_name"])
	return c
}

// Company cleans a company name from another source (an AI answer) and
// checks it as if it came from the page at pageURL, more strictly: a job
// board's name is never accepted. "" when it isn't a plausible employer.
func Company(name, pageURL string) string {
	host := hostOf(pageURL)
	c := siteFor(host).fixCompany(cleanLine(name, false))
	if !plausibleCompany(c, host, boardsNever) {
		return ""
	}
	return c
}

// Role cleans and checks a role the same way: "" when it isn't plausible.
func Role(role, company, pageURL string) string {
	r := cleanLine(role, false)
	if !plausibleRole(r, company, hostOf(pageURL)) {
		return ""
	}
	return r
}

// Location cleans a location and bounds it to MaxDetail characters.
func Location(loc string) string { return fitDetail(cleanLine(loc, false)) }

// hostOf returns the lower-case host of a page URL without "www.", or "".
func hostOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

// onDomain reports whether host is domain or one of its subdomains.
func onDomain(host, domain string) bool {
	return host == domain || strings.HasSuffix(host, "."+domain)
}
