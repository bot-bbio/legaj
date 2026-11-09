package extract_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/bot-bbio/legaj/internal/clip/extract"
	"github.com/bot-bbio/legaj/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

// posting is a JSON-LD JobPosting block for tests.
const posting = `{"@context":"https://schema.org","@type":"JobPosting",
	"title":"Platform Engineer",
	"hiringOrganization":{"@type":"Organization","name":"Northwind Robotics"},
	"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"Warsaw","addressCountry":"PL"}},
	"description":"<p>Build the billing platform.</p>"}`

func TestR19_JSONLDPreferred(t *testing.T) {
	// Every later layer offers a different answer; JSON-LD must win each field.
	p := extract.Page{
		URL:       "https://job-boards.greenhouse.io/northwind/jobs/1",
		Title:     "Job Application for Some Other Role at Other Company",
		JSONLD:    []string{`{"@type":"WebSite","name":"Northwind"}`, posting},
		Meta:      map[string]string{"og:title": "Meta Role", "og:site_name": "Meta Company", "og:description": "Remote, Meta"},
		H1:        []string{"Heading Role"},
		Selection: strings.Repeat("A selected description. ", 10),
		Text:      "Heading Role\nRemote, Meta\nApply\nAdapter description text that is long enough to count as one.\nApply for this job",
	}
	got := extract.Extract(p)
	want := extract.Result{
		Company: "Northwind Robotics", Role: "Platform Engineer", Location: "Warsaw, PL",
		Description: "Build the billing platform.",
		From:        extract.Sources{Company: extract.LayerJSONLD, Role: extract.LayerJSONLD, Location: extract.LayerJSONLD, Description: extract.LayerJSONLD},
	}
	if got != want {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
}

func TestR19_LayerOrder(t *testing.T) {
	long := strings.Repeat("The user selected this description. ", 5)
	cases := []struct {
		name string
		page extract.Page
		want extract.Result
	}{
		{
			name: "adapter beats meta and headings",
			page: extract.Page{
				URL:   "https://jobs.ashbyhq.com/contoso/0b5d",
				Title: "Data Analyst @ Contoso",
				Meta:  map[string]string{"og:site_name": "Meta Company"},
				H1:    []string{"Heading Role"},
				Text:  "Page text.",
			},
			want: extract.Result{Company: "Contoso", Role: "Data Analyst", Description: "Page text.",
				From: extract.Sources{Company: extract.LayerAdapter, Role: extract.LayerAdapter, Description: extract.LayerText}},
		},
		{
			name: "headings and meta on an unknown site",
			page: extract.Page{
				URL:   "https://careers.example.com/jobs/42",
				Title: "Careers",
				Meta:  map[string]string{"og:site_name": "Contoso"},
				H1:    []string{"Data Analyst"},
				Text:  "Page text.",
			},
			want: extract.Result{Company: "Contoso", Role: "Data Analyst", Description: "Page text.",
				From: extract.Sources{Company: extract.LayerMeta, Role: extract.LayerMeta, Description: extract.LayerText}},
		},
		{
			name: "a real selection beats the page text",
			page: extract.Page{URL: "https://example.com/j", H1: []string{"Data Analyst"}, Selection: long, Text: "Page text."},
			want: extract.Result{Role: "Data Analyst", Description: strings.TrimSpace(long),
				From: extract.Sources{Role: extract.LayerMeta, Description: extract.LayerSelection}},
		},
		{
			name: "a stray short selection is ignored",
			page: extract.Page{URL: "https://example.com/j", H1: []string{"Data Analyst"}, Selection: "Analyst", Text: "Page text."},
			want: extract.Result{Role: "Data Analyst", Description: "Page text.",
				From: extract.Sources{Role: extract.LayerMeta, Description: extract.LayerText}},
		},
		{
			name: "meta description only when the page has no text",
			page: extract.Page{URL: "https://example.com/j", Title: "Data Analyst", Meta: map[string]string{"description": "A short teaser."}},
			want: extract.Result{Role: "Data Analyst", Description: "A short teaser.",
				From: extract.Sources{Role: extract.LayerMeta, Description: extract.LayerMeta}},
		},
		{
			name: "careers suffix in the title names the company",
			page: extract.Page{URL: "https://example.com/j", Title: "Data Analyst | Contoso Careers"},
			want: extract.Result{Company: "Contoso", Role: "Data Analyst",
				From: extract.Sources{Company: extract.LayerMeta, Role: extract.LayerMeta}},
		},
		{
			name: "careers title with a separator inside the role",
			page: extract.Page{URL: "https://example.com/j", Title: "Senior Engineer - Backend | Acme Careers"},
			want: extract.Result{Company: "Acme", Role: "Senior Engineer - Backend",
				From: extract.Sources{Company: extract.LayerMeta, Role: extract.LayerMeta}},
		},
		{
			name: "separator right before Careers (gate 4)",
			page: extract.Page{URL: "https://example.com/j", Title: "Software Engineer - Acme | Careers"},
			want: extract.Result{Company: "Acme", Role: "Software Engineer",
				From: extract.Sources{Company: extract.LayerMeta, Role: extract.LayerMeta}},
		},
		{
			name: "separator right before Jobs, other order",
			page: extract.Page{URL: "https://example.com/j", Title: "Engineer | Acme - Jobs"},
			want: extract.Result{Company: "Acme", Role: "Engineer",
				From: extract.Sources{Company: extract.LayerMeta, Role: extract.LayerMeta}},
		},
		{
			name: "nothing found",
			page: extract.Page{URL: "https://example.com/j"},
			want: extract.Result{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := extract.Extract(c.page); got != c.want {
				t.Fatalf("got  %+v\nwant %+v", got, c.want)
			}
		})
	}
}

func TestR19_HTMLStrippedFromDescription(t *testing.T) {
	want := "About us\n\nBuild AT&T and R&D tools for <5% of the cost.\n\n• Go\n• Rust and C++\n\nWrite to <Company> hiring."
	html := `<h1><strong>About us</strong></h1>` +
		`<p style="min-height:1.5em">Build <b>AT&amp;T</b> and R&amp;D tools for &lt;5% of the cost.</p>` +
		`<p>&nbsp;</p>` +
		"<ul>\n<li><p>Go</p></li>\n<li><p>Rust and C++</p></li>\n</ul>" +
		`<script>alert("x")</script><style>p{color:red}</style>` +
		`<p>Write to &lt;Company&gt; hiring.</p>`
	// LinkedIn escapes its markup a second time (seen 2026-11-03).
	doubled := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(html)
	for name, desc := range map[string]string{"html": html, "escaped twice": doubled} {
		t.Run(name, func(t *testing.T) {
			block := `{"@type":"JobPosting","title":"Engineer","description":` + jsonString(desc) + `}`
			got := extract.Extract(extract.Page{URL: "https://example.com/j", JSONLD: []string{block}})
			if got.Description != want {
				t.Fatalf("description:\n%q\nwant\n%q", got.Description, want)
			}
		})
	}
}

func TestR19_PlainTextKeepsAngleBrackets(t *testing.T) {
	// Page text and selections are plain text: nothing in them is markup.
	text := "Use <div> layouts and std::vector<int>; AT&amp;T stays as typed."
	got := extract.Extract(extract.Page{URL: "https://example.com/j", Text: text})
	if got.Description != text {
		t.Fatalf("got %q", got.Description)
	}
	block := `{"@type":"JobPosting","description":"We are hiring for <5% churn, plain text."}`
	if got := extract.Extract(extract.Page{URL: "https://example.com/j", JSONLD: []string{block}}); got.Description != "We are hiring for <5% churn, plain text." {
		t.Fatalf("plain JSON-LD description changed: %q", got.Description)
	}
}

func TestR19_JSONLDShapes(t *testing.T) {
	cases := []struct {
		name, block, company, role string
	}{
		{"graph", `{"@context":"https://schema.org","@graph":[{"@type":"WebPage"},{"@type":"JobPosting","title":"Role A","hiringOrganization":{"name":"Org A"}}]}`, "Org A", "Role A"},
		{"top-level array", `[{"@type":"Organization","name":"Wrong"},{"@type":"JobPosting","title":"Role B","hiringOrganization":"Org B"}]`, "Org B", "Role B"},
		{"nested in a page", `{"@type":"WebPage","mainEntity":{"@type":["Thing","JobPosting"],"title":"Role C","hiringOrganization":[{"name":"Org C"}]}}`, "Org C", "Role C"},
		{"schema prefix", `{"@type":"http://schema.org/JobPosting","title":"Role D","hiringOrganization":{"@type":"Organization","name":"Org D"}}`, "Org D", "Role D"},
		{"raw line breaks in strings", "{\"@type\":\"JobPosting\",\"title\":\"Role\nE\",\"hiringOrganization\":{\"name\":\"Org E\"}}", "Org E", "Role E"},
		{"comment wrapper", `<!-- {"@type":"JobPosting","title":"Role F","hiringOrganization":{"name":"Org F"}} -->`, "Org F", "Role F"},
		{"entities in names", `{"@type":"JobPosting","title":"R&amp;D Engineer","hiringOrganization":{"name":"Babbage &amp; Co"}}`, "Babbage & Co", "R&D Engineer"},
		{"entities escaped twice, no tags (gate 4)", `{"@type":"JobPosting","title":"R&amp;amp;D Engineer","hiringOrganization":{"name":"AT&amp;amp;T"}}`, "AT&T", "R&D Engineer"},
		{"no job posting", `{"@type":"Organization","name":"Org G"}`, "", ""},
		{"broken json", `{"@type":"JobPosting","title":`, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extract.Extract(extract.Page{URL: "https://example.com/j", JSONLD: []string{c.block}})
			if got.Company != c.company || got.Role != c.role {
				t.Fatalf("got company %q role %q, want %q %q", got.Company, got.Role, c.company, c.role)
			}
		})
	}
}

func TestR19_JSONLDLocation(t *testing.T) {
	cases := []struct{ name, fields, want string }{
		{"duplicate segments (Lever)", `"jobLocation":{"@type":"Place","address":{"addressLocality":"Singapore, Singapore","addressRegion":null,"addressCountry":null}}`, "Singapore"},
		{"remote with requirements (Ashby)", `"jobLocationType":"TELECOMMUTE","applicantLocationRequirements":[{"@type":"Country","name":"Canada"},{"@type":"Country","name":"United States"}],"jobLocation":{"address":{"addressLocality":"New York City","addressRegion":"NY","addressCountry":"USA"}}`, "Remote (Canada, United States); New York City, NY, USA"},
		{"remote only", `"jobLocationType":["TELECOMMUTE"]`, "Remote"},
		{"country object", `"jobLocation":{"address":{"addressLocality":"Lyon","addressCountry":{"@type":"Country","name":"France"}}}`, "Lyon, France"},
		{"address as text", `"jobLocation":{"@type":"Place","address":"Kraków, Poland"}`, "Kraków, Poland"},
		{"many places", `"jobLocation":[{"address":{"addressLocality":"A"}},{"address":{"addressLocality":"B"}},{"address":{"addressLocality":"A"}},{"address":{"addressLocality":"C"}},{"address":{"addressLocality":"D"}},{"address":{"addressLocality":"E"}}]`, "A; B; C (+2 more)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			block := `{"@type":"JobPosting",` + c.fields + `}`
			if got := extract.Extract(extract.Page{URL: "https://example.com/j", JSONLD: []string{block}}).Location; got != c.want {
				t.Fatalf("location %q, want %q", got, c.want)
			}
		})
	}
}

func TestR19_GenericNamesRejected(t *testing.T) {
	cases := []struct {
		name    string
		page    extract.Page
		company string
		role    string
	}{
		{"site chrome as company", extract.Page{URL: "https://example.com/j", Meta: map[string]string{"og:site_name": "Careers"}, H1: []string{"Data Analyst"}}, "", "Data Analyst"},
		{"job board as company", extract.Page{URL: "https://www.linkedin.com/jobs/view/1", Meta: map[string]string{"og:site_name": "LinkedIn"}}, "", ""},
		{"host as company", extract.Page{URL: "https://jobs.lever.co/acme/1", JSONLD: []string{`{"@type":"JobPosting","hiringOrganization":"jobs.lever.co"}`}}, "", ""},
		{"url as company", extract.Page{URL: "https://example.com/j", JSONLD: []string{`{"@type":"JobPosting","hiringOrganization":"https://acme.example"}`}}, "", ""},
		{"generic heading falls through to the title", extract.Page{URL: "https://example.com/j", H1: []string{"Job details"}, Title: "Data Analyst"}, "", "Data Analyst"},
		{"role equal to company rejected", extract.Page{URL: "https://example.com/j", JSONLD: []string{`{"@type":"JobPosting","hiringOrganization":"Contoso"}`}, H1: []string{"Contoso"}, Title: "Data Analyst"}, "Contoso", "Data Analyst"},
		// Spike captures: search pages and portals aren't a job.
		{"search heading as role", extract.Page{URL: "https://example.com/jobs?q=pm", H1: []string{"product manager jobs in New York State"}, Title: "Now Hiring: 75 Product Manager Jobs in New York State | Indeed"}, "", ""},
		{"greenhouse candidate portal", extract.Page{URL: "https://my.greenhouse.io/jobs/search?query=x", Title: "MyGreenhouse"}, "", ""},
		// Review fixes: real employers that share a name with a job board or a domain.
		{"employer named in JSON-LD on its own board", extract.Page{URL: "https://www.linkedin.com/jobs/view/1", JSONLD: []string{`{"@type":"JobPosting","title":"Engineer","hiringOrganization":"LinkedIn"}`}}, "LinkedIn", "Engineer"},
		{"board name elsewhere is an employer", extract.Page{URL: "https://www.workday.com/en-us/company/careers/1", Meta: map[string]string{"og:site_name": "Workday"}, H1: []string{"Engineer"}}, "Workday", "Engineer"},
		{"google careers", extract.Page{URL: "https://www.google.com/about/careers/applications/jobs/results/1", JSONLD: []string{`{"@type":"JobPosting","title":"SRE","hiringOrganization":{"name":"Google"}}`}}, "Google", "SRE"},
		{"company named like its domain", extract.Page{URL: "https://jobs.booking.com/booking/jobs/1", JSONLD: []string{`{"@type":"JobPosting","title":"Engineer","hiringOrganization":"Booking.com"}`}}, "Booking.com", "Engineer"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extract.Extract(c.page)
			if got.Company != c.company || got.Role != c.role {
				t.Fatalf("got company %q role %q, want %q %q", got.Company, got.Role, c.company, c.role)
			}
		})
	}
}

func TestR19_CompanyCheckForOtherSources(t *testing.T) {
	// extract.Company checks an AI answer strictly: a job board's name is
	// never taken as the employer from a model, on any site.
	for _, c := range []struct{ name, url, want string }{
		{"LinkedIn", "https://careers.example.com/1", ""},
		{"Workday", "https://www.workday.com/careers/1", ""},
		{"Careers", "https://careers.example.com/1", ""},
		{"  Contoso\u00a0Ltd ", "https://careers.example.com/1", "Contoso Ltd"},
		{"2100 Contoso USA", "https://contoso.wd5.myworkdayjobs.com/x/job/1", "Contoso USA"},
		{"Booking.com", "https://jobs.booking.com/1", "Booking.com"},
		{"jobs.booking.com", "https://jobs.booking.com/1", ""},
	} {
		if got := extract.Company(c.name, c.url); got != c.want {
			t.Errorf("Company(%q, %s) = %q, want %q", c.name, c.url, got, c.want)
		}
	}
}

func TestR19_SiteTitlePatterns(t *testing.T) {
	cases := []struct{ name, url, title, company, role, location string }{
		{"greenhouse", "https://job-boards.greenhouse.io/acme/jobs/1", "Job Application for Engineer at Scale at Acme", "Acme", "Engineer at Scale", ""},
		{"lever", "https://jobs.lever.co/acme/1", "Acme Corp - Data Engineer - Platform", "Acme Corp", "Data Engineer - Platform", ""},
		{"ashby", "https://jobs.ashbyhq.com/acme/1", " Security Engineer, Cloud @ Acme", "Acme", "Security Engineer, Cloud", ""},
		{"linkedin guest english", "https://www.linkedin.com/jobs/view/1", "Go Engineer at Fabrikam — Cracow, Małopolskie, Poland | LinkedIn Jobs", "Fabrikam", "Go Engineer", "Cracow, Małopolskie, Poland"},
		{"linkedin guest polish", "https://pl.linkedin.com/jobs/view/1", "Go Engineer w firmie Fabrikam — Kraków | Oferty pracy na LinkedIn", "Fabrikam", "Go Engineer", "Kraków"},
		{"linkedin notification count", "https://www.linkedin.com/jobs/view/1", "(3) Go Engineer at Fabrikam — Remote | LinkedIn Jobs", "Fabrikam", "Go Engineer", "Remote"},
		{"pattern on another site is ignored", "https://example.com/j", "Job Application for Engineer at Acme", "", "Job Application for Engineer at Acme", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extract.Extract(extract.Page{URL: c.url, Title: c.title})
			if got.Company != c.company || got.Role != c.role || got.Location != c.location {
				t.Fatalf("got %q / %q / %q, want %q / %q / %q", got.Company, got.Role, got.Location, c.company, c.role, c.location)
			}
		})
	}
}

func TestR19_WorkdayEntityCodeRemoved(t *testing.T) {
	block := `{"@type":"JobPosting","title":"Engineer","hiringOrganization":{"name":"2100 Northwind Robotics USA"}}`
	got := extract.Extract(extract.Page{URL: "https://northwind.wd5.myworkdayjobs.com/en-US/Careers/job/1", JSONLD: []string{block}})
	if got.Company != "Northwind Robotics USA" {
		t.Fatalf("workday company %q", got.Company)
	}
	// Elsewhere a leading number is part of the name.
	block = `{"@type":"JobPosting","title":"Partner","hiringOrganization":{"name":"500 Startups"}}`
	if got := extract.Extract(extract.Page{URL: "https://example.com/j", JSONLD: []string{block}}); got.Company != "500 Startups" {
		t.Fatalf("company %q", got.Company)
	}
}

func TestR19_GreenhouseDescriptionSlice(t *testing.T) {
	text := "Back to jobs\nPlatform Engineer\nRemote, Poland\nApply\n" +
		"Northwind builds robots.\n\nWhat you'll do\nOwn billing end to end and keep it fast.\n" +
		"Apply for this job\n*\nindicates a required field\nFirst Name*\nEmail*\nSubmit application"
	got := extract.Extract(extract.Page{
		URL: "https://job-boards.greenhouse.io/northwind/jobs/1", Title: "Job Application for Platform Engineer at Northwind",
		Meta: map[string]string{"og:description": "Remote, Poland"}, H1: []string{"Platform Engineer"}, Text: text,
	})
	want := "Northwind builds robots.\n\nWhat you'll do\nOwn billing end to end and keep it fast."
	if got.Description != want || got.From.Description != extract.LayerAdapter {
		t.Fatalf("description (%s):\n%q\nwant\n%q", got.From.Description, got.Description, want)
	}
	if got.Location != "Remote, Poland" {
		t.Fatalf("location %q", got.Location)
	}
	// A list of places is a location, however long (Chrome spike capture).
	places := "Berlin, Germany; Chemnitz, Sachsen, Germany; Dresden, Sachsen, Germany; München, Bayern, Germany"
	got = extract.Extract(extract.Page{URL: "https://job-boards.greenhouse.io/staffbase/jobs/1", Meta: map[string]string{"og:description": places}})
	if got.Location != places {
		t.Fatalf("place list: %q", got.Location)
	}
	// A long og:description is a teaser, not a place.
	got = extract.Extract(extract.Page{
		URL:  "https://job-boards.greenhouse.io/northwind/jobs/1",
		Meta: map[string]string{"og:description": strings.Repeat("We build robots for warehouses. ", 5)},
	})
	if got.Location != "" {
		t.Fatalf("teaser taken as location: %q", got.Location)
	}
}

func TestR19_LinkedInGuestDescriptionSlice(t *testing.T) {
	// Logged-out LinkedIn without JSON-LD (seen 2026-11-03): sign-in
	// prompts, each ending "By clicking Continue…", then the posting, then
	// "Show more" / "Show less" and the job criteria.
	consent := "By clicking Continue to join or sign in, you agree to LinkedIn's User Agreement, Privacy Policy, and Cookie Policy."
	text := strings.Join([]string{
		"Go Engineer", "Fabrikam", "Apply", "Email or phone", "Password", consent,
		"Save", "Report this job", "Email or phone", "Password", consent,
		"Meet the Team", "We build ledgers.", "Show more", "Show less", "Seniority level", "Mid-Senior level",
	}, "\n")
	got := extract.Extract(extract.Page{URL: "https://www.linkedin.com/jobs/view/1", Title: "Go Engineer at Fabrikam — Remote | LinkedIn Jobs", Text: text})
	if got.Description != "Meet the Team\nWe build ledgers." || got.From.Description != extract.LayerAdapter {
		t.Fatalf("description (%s): %q", got.From.Description, got.Description)
	}
	// The Polish site (seen 2026-11-03): its consent line, then a "report
	// this job" line, then the posting; "Show more" stays English.
	pl := strings.Join([]string{
		"Go Engineer", "Fabrikam", "Hasło", "Klikając Kontynuuj, aby dołączyć lub się zalogować, wyrażasz zgodę na warunki LinkedIn.",
		"Zgłoś tę ofertę pracy", "Meet the Team", "We build ledgers.", "Show more", "Show less", "Poziom w hierarchii",
	}, "\n")
	got = extract.Extract(extract.Page{URL: "https://pl.linkedin.com/jobs/view/1", Text: pl})
	if got.Description != "Meet the Team\nWe build ledgers." {
		t.Fatalf("polish description (%s): %q", got.From.Description, got.Description)
	}
	// The user's own selection beats the slice (gate 4, round 2).
	sel := strings.Repeat("The selected posting text. ", 5)
	got = extract.Extract(extract.Page{URL: "https://www.linkedin.com/jobs/view/1", Text: text, Selection: sel})
	if got.From.Description != extract.LayerSelection {
		t.Fatalf("description came from %s, want the selection", got.From.Description)
	}
	// An unrecognised layout falls back to the page text.
	got = extract.Extract(extract.Page{URL: "https://www.linkedin.com/jobs/view/1", Text: "Some other layout\nShow more"})
	if got.From.Description != extract.LayerText {
		t.Fatalf("fallback came from %s", got.From.Description)
	}
}

func TestR19_UntrustedTextCleaned(t *testing.T) {
	got := extract.Extract(extract.Page{
		URL:  "https://example.com/j",
		H1:   []string{"  Data\xc2\xa0Analyst\xe2\x80\x8b\x07  "},
		Text: "Line one\r\nLine\ttwo\xc2\xa0here\x00\n\n\n\nLine three  \xe2\x80\xa8Line four",
	})
	if got.Role != "Data Analyst" {
		t.Fatalf("role %q", got.Role)
	}
	if want := "Line one\nLine two here\n\nLine three\nLine four"; got.Description != want {
		t.Fatalf("description %q, want %q", got.Description, want)
	}
}

func TestR19_FieldsBounded(t *testing.T) {
	long := strings.Repeat("Engineer ", 40) // 360 characters: a paragraph, not a role
	got := extract.Extract(extract.Page{URL: "https://example.com/j", H1: []string{long}, Title: "Data Analyst"})
	if got.Role != "Data Analyst" {
		t.Fatalf("over-long heading taken as role: %q", got.Role)
	}
	// A long description is cut at a character boundary, never mid-rune.
	text := strings.Repeat("ż", extract.MaxDescription) // 2 bytes each
	d := extract.Extract(extract.Page{URL: "https://example.com/j", Text: text}).Description
	if len(d) > extract.MaxDescription || !utf8.ValidString(d) || len(d) < extract.MaxDescription-4 {
		t.Fatalf("description %d bytes, valid %v", len(d), utf8.ValidString(d))
	}
	// Many places are cut to fit the location field.
	var places []string
	for i := 0; i < 3; i++ {
		places = append(places, `{"address":{"addressLocality":"`+strings.Repeat("x", 90)+string(rune('a'+i))+`"}}`)
	}
	block := `{"@type":"JobPosting","jobLocation":[` + strings.Join(places, ",") + `]}`
	loc := extract.Extract(extract.Page{URL: "https://example.com/j", JSONLD: []string{block}}).Location
	if n := utf8.RuneCountInString(loc); n == 0 || n > extract.MaxDetail {
		t.Fatalf("location %d runes: %q", n, loc)
	}
}

// FuzzR19_ExtractBounded feeds hostile pages to Extract: it must never
// panic, and every field must be valid UTF-8 within its limit. The seeds
// run with every `go test`; `go test -fuzz FuzzR19 ./internal/clip/extract/`
// explores further.
func FuzzR19_ExtractBounded(f *testing.F) {
	f.Add("https://jobs.lever.co/a/1", "A - B", posting, "<p>x</p>", "text")
	f.Add("https://job-boards.greenhouse.io/a/jobs/1", "Job Application for R at C", `[{"@graph":[{"@type":["JobPosting"]}]}]`, "&lt;li&gt;&amp;amp;", "R\nApply\nx\nApply for this job")
	f.Add("::bad url", "(9) x at y — z | LinkedIn", `{"@type":"JobPosting","jobLocation":[{"address":"a"},{"address":{"addressCountry":{"name":"b"}}}],"jobLocationType":"TELECOMMUTE"}`, "<li><p><br><br>", "\x00\xff\xfe")
	f.Fuzz(func(t *testing.T, url, title, block, h1, text string) {
		r := extract.Extract(extract.Page{
			URL: url, Title: title, JSONLD: []string{block, `{"@type":"JobPosting","description":` + jsonString(h1) + `}`},
			Meta: map[string]string{"og:title": h1, "og:site_name": title, "og:description": text},
			H1:   []string{h1}, Selection: text, Text: text + h1,
		})
		for name, v := range map[string]string{"company": r.Company, "role": r.Role, "location": r.Location} {
			if !utf8.ValidString(v) || utf8.RuneCountInString(v) > extract.MaxDetail {
				t.Fatalf("%s out of bounds: %q", name, v)
			}
		}
		if !utf8.ValidString(r.Description) || len(r.Description) > extract.MaxDescription {
			t.Fatalf("description out of bounds (%d bytes)", len(r.Description))
		}
	})
}

// jsonString quotes s as a JSON string without HTML escaping.
func jsonString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return `"` + r.Replace(s) + `"`
}

// Gate 5 fixes: each page below lost or invented data in review.

func TestR19_LinkedInMemberPostingWithItsOwnAboutTheCompany(t *testing.T) {
	// Postings often have an "About the company" heading of their own; only
	// the last one is LinkedIn's box.
	text := strings.Join([]string{
		"Go Engineer", "Warsaw · 1 week ago · 20 applicants", "About the job", "Join our team.", "",
		"About the company", "We build robots.", "", "Responsibilities", "Ship the planner.", "",
		"About the company", "Northwind", "1,000 followers",
	}, "\n")
	got := extract.Extract(extract.Page{URL: "https://www.linkedin.com/jobs/view/1/", Title: "Go Engineer | Northwind | LinkedIn", Text: text})
	if !strings.Contains(got.Description, "Ship the planner.") {
		t.Fatalf("the posting was cut short:\n%s", got.Description)
	}
	if strings.Contains(got.Description, "followers") {
		t.Fatalf("LinkedIn's company box got in:\n%s", got.Description)
	}
}

func TestR19_LinkedInMemberPostingEndsAtTheToggleFirst(t *testing.T) {
	text := strings.Join([]string{
		"About the job", "Join our team.", "… more", "About the company", "Northwind",
	}, "\n")
	got := extract.Extract(extract.Page{URL: "https://www.linkedin.com/jobs/view/1/", Title: "Go Engineer | Northwind | LinkedIn", Text: text})
	if got.Description != "Join our team." {
		t.Fatalf("description %q", got.Description)
	}
}

func TestR19_LinkedInGuestPostingWithAnAboutTheJobHeading(t *testing.T) {
	// The logged-in cut starts at "About the job", which would drop
	// everything above that line on a logged-out page.
	consent := "By clicking Continue to join or sign in, you agree to LinkedIn's User Agreement, Privacy Policy, and Cookie Policy."
	text := strings.Join([]string{
		"Go Engineer", "Fabrikam", consent, "Fabrikam makes engines.", "About the job", "Tune the engines.", "Show more", "Show less", "Seniority level",
	}, "\n")
	got := extract.Extract(extract.Page{URL: "https://www.linkedin.com/jobs/view/1/", Title: "Go Engineer at Fabrikam — Warsaw | LinkedIn Jobs", Text: text})
	want := "Fabrikam makes engines.\nAbout the job\nTune the engines."
	if got.Description != want {
		t.Fatalf("description:\n%q\nwant\n%q", got.Description, want)
	}
}

func TestR19_GreenhouseFormHeadingInAnyCase(t *testing.T) {
	for _, heading := range []string{"Apply for this job", "Apply for this Job", "APPLY FOR THIS JOB"} {
		text := strings.Join([]string{"Platform Engineer", "Remote", "Apply", "Northwind builds robots.", heading, "First Name*", "Email*"}, "\n")
		got := extract.Extract(extract.Page{URL: "https://job-boards.greenhouse.io/northwind/jobs/1", H1: []string{"Platform Engineer"}, Text: text})
		if got.Description != "Northwind builds robots." {
			t.Fatalf("%q: the form got into the description: %q", heading, got.Description)
		}
	}
}

func TestR19_GreenhouseAbbreviatedPlacesAreLocations(t *testing.T) {
	for _, place := range []string{
		"St. Louis, MO", "Ft. Worth, TX", "Washington, D.C.", "San Diego, Calif.", "Boston, Mass.", "Berlin, Germany; St. Louis, MO",
		"Mt. Vernon, NY; Washington, D.C.", "Frankfurt am Main, Germany", "Rio de Janeiro, Brazil", "Trinidad and Tobago",
	} {
		got := extract.Extract(extract.Page{URL: "https://job-boards.greenhouse.io/northwind/jobs/1", Meta: map[string]string{"og:description": place}})
		if got.Location != place {
			t.Errorf("%q came back as %q", place, got.Location)
		}
	}
	for _, teaser := range []string{
		"We build robots. Join us", "We are hiring.", "We are hiring now.", "Apply now.", "Join us.", "Founded in 2012.", "Now Hiring.",
		"Berlin, Germany; We are hiring.", "Build the future of AI.", "Build the future of AI", "Join us at Acme to build things",
	} {
		got := extract.Extract(extract.Page{URL: "https://job-boards.greenhouse.io/northwind/jobs/1", Meta: map[string]string{"og:description": teaser}})
		if got.Location != "" {
			t.Errorf("teaser %q taken as location %q", teaser, got.Location)
		}
	}
}

func TestR19_IndeedLineUnderTheRoleMustBeACompany(t *testing.T) {
	// With no employer shown, the next line is the pay or the address. A
	// wrong company would also stop enrichment from ever fixing the row.
	for _, line := range []string{
		"$90,000 - $105,000 a year", "€4,500 a month", "£45,000", "From 25 an hour", "From $25 an hour - Full-time",
		"1230 York Avenue, New York, NY 10065", "New York, NY", "Full-time", "Part time", "Contract", "Remote",
	} {
		text := strings.Join([]string{"Analyst", line, "Full time", "Full job description", "Do the work.", "Report job"}, "\n")
		got := extract.Extract(extract.Page{URL: "https://www.indeed.com/viewjob?jk=1", Title: "Analyst - New York, NY - Indeed.com", Text: text})
		if got.Company != "" {
			t.Errorf("%q taken as the company", got.Company)
		}
		if got.Role != "Analyst" || got.Description != "Do the work." {
			t.Errorf("role %q description %q", got.Role, got.Description)
		}
	}
	// Real employers can have digits and those words in their names.
	for _, employer := range []string{"7-Eleven, Inc.", "Walgreens #12345", "Store 10423", "A Day Away Kennels"} {
		text := strings.Join([]string{"Analyst", employer, "Full job description", "Do the work."}, "\n")
		got := extract.Extract(extract.Page{URL: "https://www.indeed.com/viewjob?jk=1", Title: "Analyst - New York, NY - Indeed.com", Text: text})
		if got.Company != employer {
			t.Errorf("a real employer was refused: %q became %q", employer, got.Company)
		}
	}
}

func TestR19_IndeedSearchLayoutChecksTheCompanyToo(t *testing.T) {
	text := strings.Join([]string{"Job Post Details", "Analyst", "$90,000 a year", "•", "New York, NY", "Full job description", "Do the work."}, "\n")
	got := extract.Extract(extract.Page{URL: "https://www.indeed.com/jobs?q=analyst", Title: "Now Hiring: 75 Analyst Jobs in New York | Indeed", Text: text})
	if got.Company != "" || got.Role != "Analyst" {
		t.Fatalf("company %q role %q", got.Company, got.Role)
	}
}

func TestR19_IndeedWithoutTheEnglishMarkerKeepsTheTitle(t *testing.T) {
	// The title is reliable anywhere. The line under the role is only the
	// company on the layout LeGaJ knows, so here enrichment finds it.
	text := strings.Join([]string{"Projektmanager", "Vollzeit", "Vollständige Stellenbeschreibung", "Die Arbeit."}, "\n")
	got := extract.Extract(extract.Page{URL: "https://de.indeed.com/viewjob?jk=1", Title: "Projektmanager - Berlin - Indeed.com", Text: text})
	if got.Role != "Projektmanager" || got.Location != "Berlin" {
		t.Fatalf("role %q location %q", got.Role, got.Location)
	}
	if got.Company != "" {
		t.Fatalf("%q taken as the company", got.Company)
	}
	if !strings.Contains(got.Description, "Die Arbeit.") || got.From.Description != extract.LayerText {
		t.Fatalf("description (%s): %q", got.From.Description, got.Description)
	}
	// An expired page has no marker either.
	text = strings.Join([]string{"Analyst", "This job has expired on Indeed", "Find similar jobs"}, "\n")
	got = extract.Extract(extract.Page{URL: "https://www.indeed.com/viewjob?jk=1", Title: "Analyst - New York, NY - Indeed.com", Text: text})
	if got.Company != "" {
		t.Fatalf("%q taken as the company", got.Company)
	}
}

func TestR19_SearchHeadingWordsAreFineInStructuredData(t *testing.T) {
	got := extract.Extract(extract.Page{
		URL:    "https://example.com/j",
		JSONLD: []string{`{"@type":"JobPosting","title":"Now Hiring: Delivery Driver","hiringOrganization":"Acme"}`},
	})
	if got.Role != "Now Hiring: Delivery Driver" {
		t.Fatalf("a posting's own title was refused: %q", got.Role)
	}
}

// A model asked about a search page copies its heading, which is exactly
// what the page-text check refused.
func TestR19_AISearchHeadingIsNotARole(t *testing.T) {
	if role := extract.Role("product manager jobs in New York State", "", "https://example.com/j"); role != "" {
		t.Fatalf("an AI answer brought back a search heading: %q", role)
	}
}

func TestR19_LinkedInMemberExpandedPostingStopsAtTheBox(t *testing.T) {
	// Expanded, the posting has no toggle of its own, so the next "… more"
	// belongs to the company box.
	text := strings.Join([]string{
		"About the job", "Join our team.", "About the company", "Northwind", "1,000 followers", "We build robots.", "… more", "Show more",
	}, "\n")
	got := extract.Extract(extract.Page{URL: "https://www.linkedin.com/jobs/view/1/", Title: "Go Engineer | Northwind | LinkedIn", Text: text})
	if got.Description != "Join our team." {
		t.Fatalf("description %q", got.Description)
	}
	// With a heading of its own too, the cut is still at the box.
	text = strings.Join([]string{
		"About the job", "Join our team.", "About the company", "We build robots.", "Responsibilities", "Ship.",
		"About the company", "Northwind", "… more",
	}, "\n")
	got = extract.Extract(extract.Page{URL: "https://www.linkedin.com/jobs/view/1/", Title: "Go Engineer | Northwind | LinkedIn", Text: text})
	if !strings.HasSuffix(got.Description, "Ship.") {
		t.Fatalf("description %q", got.Description)
	}
}

func TestR19_LinkedInMemberCompanyBoxAboveThePosting(t *testing.T) {
	// A line from page text must never decide a slice's bounds: with the box
	// above "About the job", there's no end to cut at.
	text := strings.Join([]string{"About the company", "Northwind", "About the job", "Join our team."}, "\n")
	got := extract.Extract(extract.Page{URL: "https://www.linkedin.com/jobs/view/1/", Title: "Go Engineer | Northwind | LinkedIn", Text: text})
	if got.Description != "Join our team." {
		t.Fatalf("description %q", got.Description)
	}
}
