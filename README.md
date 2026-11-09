# LeGaJ: Let's Get a Job

A free Windows desktop app that keeps a job hunt in one place: every posting you're considering, a tailored cover letter for each, answers to application-form questions, follow-up reminders, and your interviews. Everything is stored on your computer; the AI parts use your own free Google Gemini key.

## Mission

Applying for jobs is repetitive work spread across browser tabs, documents and calendars. LeGaJ takes the busywork out of it without taking you out of it:

- **One pipeline.** Each job moves from Lead to Ready, Applied, Interviewing and Offer (or Rejected or Withdrawn), and the app knows what each one still needs.
- **Grounded writing.** Cover letters and form answers are drafted only from your own profile and that job's description: no invented experience, no placeholders. You review and approve every word.
- **You stay in charge.** LeGaJ never applies, sends email or installs anything on its own. It drafts and reminds; you decide and send.
- **Private by default.** No LeGaJ server, no account, no tracking. See [PRIVACY.md](PRIVACY.md).

## Setup

### Install

Download the installer from the [Releases page](https://github.com/bot-bbio/legaj/releases) and follow [INSTALL.md](INSTALL.md). It installs for your Windows account only, with no administrator password.

On first launch a short setup asks for:

1. A **Gemini API key**, free from [Google AI Studio](https://aistudio.google.com/apikey). LeGaJ tests it and stores it in Windows Credential Manager.
2. Your **resume** (PDF or Word). Gemini turns it into a profile draft.
3. A quick **review of that profile**. Everything LeGaJ writes for you is based on it, so it's worth getting right.

Any step can be skipped and finished later from the checklist at the top of the app.

### Build from source

You need Windows, Go 1.26+, Node.js 24+, a MinGW-w64 `gcc` on `PATH` (for the race detector) and the Wails CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1`). Inno Setup 6 is optional (to build the installer).

```powershell
powershell -NoProfile -File scripts/verify.ps1   # every check and test, then build/bin/legaj.exe
wails dev                                        # live development
```

`go test` refuses to run unless `LEGAJ_HOME` points to a scratch folder, so tests can never touch your real data; `verify.ps1` sets it for you.

## Usage

LeGaJ has five sections: **Profile**, **Pipeline**, **Follow-ups**, **Interviews** and **Settings**.

### Add jobs

The quickest way is the **clipper**, a bookmark you install during setup (or from **Settings**). On a job posting, click the bookmark, then click **Save** in the small window that opens. LeGaJ has to be running; **Start LeGaJ with Windows** in Settings keeps it ready. On a site LeGaJ doesn't know, select the job description first.

A page that doesn't let LeGaJ read it, or a job you only have as text, goes in through **Paste job** in **Pipeline**, with the posting's link and description. Either way the same posting is never added twice: links are compared after removing tracking parameters.

### Cover letters

1. Choose each job's **materials**: a letter, form answers, or both.
2. Click **Generate** on a row (or select several rows and use **Generate letters**). Letters are drafted in the background, two at a time, and appear in the **Review** panel beside the table.
3. **Approve** a draft as it is, or open **Edit letter** to rewrite any paragraph, pick one of three "why this company" paragraphs, or regenerate a single paragraph. **Approve all** handles a batch.
4. Each approval saves a one-page PDF as a new version in that application's folder. Once its chosen materials are approved, a Lead moves to Ready on its own.

With **Skip review** on (Settings), generated letters go straight to PDF.

### Form answers

Open a row's **Answers** to draft three answers to any application question ("Why this company?" if you leave it empty). If you've answered a similar question before, the **answer bank** suggests that answer first. Copy, edit and approve; approved answers are saved in the **Answer bank** tab.

### Follow-ups

**Follow-ups** lists what's due: a nudge a week after you apply (the delay is in Settings) and a thank-you note after each interview. Drafts are filled in from the application. LeGaJ opens them in Gmail, Outlook or your mail app; you press Send there, then tell LeGaJ whether you sent it. The nav shows how many are due.

### Interviews

Add each round with its date, time zone, type, link, interviewers and notes. The first round moves the application to Interviewing. **Add to calendar** opens the round in your calendar app, and **Suggest questions** drafts 8 to 12 questions to ask that the job description doesn't already answer.

## Architecture

LeGaJ is a single Windows executable built with [Wails](https://wails.io) v2: a Go backend and a Svelte 5 + TypeScript frontend rendered by Microsoft Edge WebView2.

```
frontend/            Svelte 5 + strict TypeScript UI
  src/lib/api.ts       the only door to Go (generated, typed bindings)
  src/lib/stores/      entity stores: load on use, refetch on Go's change events
main.go, *_api.go    the bound API objects (one per screen); errors are user-safe
internal/
  core/              boots everything and wires the services
  store/             SQLite (pure Go), migrations, daily backups, integrity checks
  pipeline/          applications, the stage state machine, undoable deletes
  letters/ answers/  grounded drafting with validation and one retry
  jobs/              background generation: 2 at a time, rate-limit backoff
  pdf/ documents/    one-page cover-letter PDFs, versioned per application
  followups/         templates, due rules (local calendar days), compose links
  interviews/        rounds, .ics export, questions to ask
  ai/gemini/         the Gemini client (structured JSON output, schema-checked)
  secrets/ logx/     Credential Manager storage; logs with secrets redacted
  opener/ proc/      the only ways to open links/files or start processes
```

Some rules the code keeps:

- **The database is the single source of truth.** Every write runs in a transaction on a single writer connection and is announced after it commits; the UI never keeps its own copy of the data.
- **Rows are addressed by stable IDs**, never by position or by company name, and bulk actions act on exactly the selected IDs.
- **Untrusted text stays data.** Job descriptions and resume text go to Gemini inside marked blocks the text can't forge, and every AI answer is checked against a JSON schema and LeGaJ's own rules before it's used.
- **Nothing runs a shell.** Links and files open through one validated opener (web and mail links only, files only inside LeGaJ's folder).

Data lives in `%LOCALAPPDATA%\LeGaJ`; the Gemini key lives in Windows Credential Manager. Details are in [PRIVACY.md](PRIVACY.md).

## License and credit

LeGaJ is released under the [MIT License](LICENSE).

Made by Roberto Montero ([LinkedIn](https://linkedin.com/in/alvvays)).
