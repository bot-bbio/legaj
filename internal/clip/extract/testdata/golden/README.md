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

## Still to add (from the bookmarklet spike, plan item 2)

- **Indeed:** it refuses non-browser requests, so its structure is unknown.
- **LinkedIn logged in:** the view most people use. Its title, JSON-LD and text differ from the logged-out pages.
- Real captures for the six sites above, to confirm the copied structure matches what the bookmarklet sees.

Captures saved by the spike (`spike/bookmarklet/out/captures/`) can contain the user's name and other personal data, for example from a logged-in header. Replace every personal detail and the job's own text before a capture becomes a fixture here.
