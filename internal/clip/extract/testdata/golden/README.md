# Clipper golden fixtures (R19)

Each folder holds one page snapshot (`input.json`, the clip payload the bookmarklet sends) and the job LeGaJ must extract from it (`expected.json`). `TestR19_Golden` runs every folder, and fails if one of the required sites is missing.

## Where they come from

The structure of each `input.json` copies a public job page fetched on 2026-11-03: the title format, the JSON-LD layout and escaping, and which meta tags carry what. **The company, role and description text is made up.** The fixtures carry no third-party job text and no personal data.

| Folder | Structure copied from | Notes |
|---|---|---|
| `greenhouse` | `job-boards.greenhouse.io` posting | No JSON-LD; the title is `Job Application for <role> at <company>`; `og:description` holds the location; the description sits between the `Apply` button and the `Apply for this job` form |
| `lever` | `jobs.lever.co` posting | JSON-LD with spaces before colons; locality `City, City` with null region and country; `&nbsp;` spacer paragraphs; no `h1` |
| `ashby` | `jobs.ashbyhq.com` posting | JSON-LD title with a leading space; `TELECOMMUTE` plus a place and applicant countries; list items wrapping paragraphs |
| `workday` | `*.myworkdayjobs.com` posting | Pretty-printed JSON-LD; a legal-entity code before the company (`2100 … USA`); locality written `US, CA, City`; plain-text description; empty `<title>` |
| `linkedin-guest-pl` | `pl.linkedin.com` logged-out view | JSON-LD whose description is HTML escaped twice (`&lt;strong&gt;`); Polish title format |
| `linkedin-guest-en` | `www.linkedin.com` logged-out view | No JSON-LD; English title format; sign-in prompts, each ending with a "By clicking Continue to join or sign in…" line, before the description, and "Show more" / "Show less" after it |

The `text` field of the Workday and Ashby fixtures is an approximation: those pages render their content with JavaScript, and the bookmarklet reads the rendered page.

These fixtures copy the structure of real bookmarklet captures from the spike (plan item 2, Firefox, 2026-11-04), again with made-up content:

| Folder | Structure copied from | Notes |
|---|---|---|
| `linkedin-member` | `www.linkedin.com/jobs/view/…` logged in | Title `<role> \| <company> \| LinkedIn`; no `h1`, meta or JSON-LD; header lines company, role, `<place> · <posted> · <applicants>` with blank lines between them; the description between "About the job" and "… more" |
| `linkedin-member-search` | `www.linkedin.com/jobs/search-results/?currentJobId=…` logged in | The results list comes first and repeats the role; the selected job's header and "About the job" follow the page footer |
| `indeed-viewjob` | `www.indeed.com/viewjob?jk=…` | Title `<role> - <place> - Indeed.com` (the role can contain ` \| `); the company on the line after the role; the description between "Full job description" and "Explore other jobs" |
| `indeed-search` | `www.indeed.com/jobs?…&vjk=…` | The title and `h1` describe the search; the selected job sits after "Job Post Details" as role, company, `•`, place; the description ends at "Report job" |

The same captures confirmed the Greenhouse, Lever, Ashby and Workday structures above. Greenhouse's real page adds a "Create a Job Alert" box before the application form, now in the `greenhouse` fixture.

Spike captures (`spike/bookmarklet/out/captures/`, never committed) contain the user's name, past employers and other personal data, for example in LinkedIn's "People you can reach out to" box. Replace every personal detail and the job's own text before a capture's structure becomes a fixture here.
