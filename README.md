# LeGaJ — Let's Get a Job

[![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?style=flat-square&logo=go)](https://go.dev)
[![Python Version](https://img.shields.io/badge/Python-3.12%2B-3776AB?style=flat-square&logo=python)](https://python.org)
[![Fyne Version](https://img.shields.io/badge/UI-Fyne%20v2-blueviolet?style=flat-square)](https://fyne.io)

LeGaJ (Let's Get a Job) is an offline-first job search suite. It pairs a **Go + Fyne desktop GUI** with a **Python document backend** for parsing resume files, compiling single-page PDF resumes and cover letters, and tracking job applications locally. All personal data stays on your machine.

---

## Getting Started

This guide covers the full workflow from a fresh install to actively applying for jobs. Follow the steps in order on first use.

---

### Step 1 — Get a Gemini API Key

LeGaJ uses Google's Gemini API to parse your resume, extract job details from clipped postings, and draft cover letters. The free tier is sufficient for normal job searching.

1. Go to [aistudio.google.com](https://aistudio.google.com) and sign in with a Google account.
2. Create a new project when prompted. The name does not matter.
3. Navigate to **Get API key** and generate a key for your project. Copy it somewhere safe — you will paste it into LeGaJ during setup.
4. When configuring the model inside LeGaJ, use **`gemini-3.1-flash-lite`**. This model has the most generous free-tier rate limits and is fast enough that you will not notice any delay during normal use.

---

### Step 2 — Install LeGaJ

Download the latest Windows installer from the [Releases page](https://github.com/bot-bbio/legaj/releases). Run the `.exe` — it bundles all required runtimes, so no separate Python or Go installation is needed. Once the installer finishes, launch LeGaJ from the Start Menu or desktop shortcut.

---

### Step 3 — Complete the Onboarding Wizard

The setup wizard starts automatically the first time you run LeGaJ. Work through each step:

- **API Key** — Paste the Gemini API key you generated in Step 1 and set the model to `gemini-3.1-flash-lite`.
- **Resume Upload** — Select your existing resume file (PDF or DOCX). LeGaJ parses it locally and builds your base profile, which it uses to generate all future cover letters and resume PDFs.
- **Output Folder** — Choose a folder on your machine where LeGaJ will save all generated cover letters and PDFs. Pick somewhere easy to find, such as a dedicated `Job Applications` folder on your desktop. This folder becomes your central archive — every cover letter LeGaJ generates will land here, named by company and role, so you can attach the right file when you go to apply.

If you need to re-run the wizard at any time, open it from the **Settings** tab.

---

### Step 4 — Review and Complete Your Base Profile

After the wizard finishes, navigate to the **Base Profile** tab. If the tab appears empty, close and reopen LeGaJ — the parsed profile data loads on startup. Review each section (contact details, experience, education, skills) and fill in anything the parser missed or got wrong. The quality of your base profile directly affects the quality of every cover letter LeGaJ generates, so it is worth spending a few minutes here before you start clipping jobs.

---

### Step 5 — Install the Browser Clipper

The clipper is a small bookmarklet that sends any job posting page to LeGaJ with one click. It is the core of the intake workflow.

1. Open the **Help** tab in LeGaJ and click **Copy Bookmarklet Code**.
2. Make your browser's bookmarks bar visible (`Ctrl+Shift+B` on Windows).
3. Right-click the bookmarks bar, select **Add bookmark**, give it a short name like **Clip to LeGaJ**, and paste the copied code as the URL field.
4. Keep it on the bookmarks bar so it is always one click away while you browse.

The clipper works best on LinkedIn, where it reliably extracts the job title, company, location, and description. It also works on Indeed, Greenhouse, Lever, Workday, Ashby, and iCIMS, though extraction accuracy varies by site. When the API key is configured, LeGaJ uses the AI model to clean up any noisy or incomplete data the clipper scrapes.

---

### Step 6 — Clip Jobs and Add Them to the Tracker

With the clipper installed and your profile ready, browse for jobs as normal. When you find a role you want to apply for, click the **Clip to LeGaJ** bookmark. The posting is sent to your **Job Hunt** tab inbox in the background.

When you are ready to process your inbox:

1. Open the **Job Hunt** tab — all clipped postings appear here as leads.
2. Select the roles you want to pursue and click **Track and Apply**. This adds them to the Job Tracker and triggers cover letter generation in the background. LeGaJ compiles a tailored cover letter PDF for each selected role and saves it to your output folder.

---

### Step 7 — Apply

Open the **Job Tracker** tab. Select one or more jobs and click **Open Job URL** in the toolbar — this opens the original posting in your browser so you can go straight to the application form. Your cover letter PDF for that role is already waiting in the output folder you configured in Step 3. Attach it and submit.

---

### Step 8 — Manage the Tracker and Inbox

Keep the tracker and clipper inbox maintained as you go:

- **Clear the Job Leads inbox** — Once clipped jobs have been processed (added to the tracker or dismissed), remove them from the inbox. A crowded inbox slows down the Job Hunt tab, so clearing it regularly keeps things responsive.
- **File Manager** — Use the **File Manager** tab to browse, rename, or delete generated PDFs if you need to tidy up your output folder from within the app.

---

### Step 9 — Report Bugs

LeGaJ is in early alpha. If you run into something broken or unexpected, please open an issue on the [GitHub Issues page](https://github.com/bot-bbio/legaj/issues). The more detail the better — include the exact steps to reproduce the problem, what you expected to happen, what actually happened, and any error messages or screenshots if applicable.

---

## Key Features

- **Onboarding Setup Wizard** — Step-by-step GUI setup for Gemini API configuration, workspace paths, and base resume import on first launch.
- **Local Resume Parser** — Extracts data from PDF, DOCX, TXT, and Markdown files into a standardized profile JSON structure.
- **Print-Ready PDF Compiler** — Uses ReportLab to generate professional, single-page PDF documents adhering to strict geometry constraints.
- **Local Job Tracker** — Tracks applications locally (`job-tracker.json`) with editable status dropdowns, notes, and direct job URL access from the toolbar.
- **One-Click Browser Clipper** — Saves job postings from LinkedIn, Indeed, Greenhouse, Lever, Workday, Ashby, and iCIMS straight into your Job Leads inbox via a bookmarklet or unpacked browser extension.

> **Alpha scope:** AI resume tailoring is implemented but disabled in the 1.0 alpha (`resumeTailoringEnabled = false`). The Job Hunt tab shows a Job Leads inbox; clipped jobs are tracked and base-profile PDFs are compiled. The discovery engine (automated job search) is deferred to a future release.

---

## Architecture

LeGaJ uses a Go/Fyne frontend that shells out to Python scripts for document processing:

```
Go Fyne GUI
├── Local HTTP listener  <── Browser clipper (bookmarklet / extension)
├── exec.Command         ──> parse_resume.py
│                        ──> generate_resume_pdf.py
│                        ──> generate_cover_letter_pdf.py
└── HTTPS               ──> Gemini API (field extraction, cover letter drafting)
```

### Stack

| Component | Technology |
| :--- | :--- |
| Desktop GUI | Go, [Fyne v2](https://fyne.io) |
| Resume parsing | Python, `pypdf`, `python-docx` |
| PDF compilation | Python, `reportlab` |
| AI extraction | Gemini API (key stored locally) |
| Job data storage | Local JSON (`job-tracker.json`) |

---

## Installation and Setup

### Prerequisites (source builds only)

- **Python 3.12+** and **Go 1.25+** — only required if building from source. The Windows installer bundles both runtimes.

### Windows

Download the latest installer from the [Releases page](https://github.com/bot-bbio/legaj/releases). Run the `.exe` installer — it bundles everything, no separate Python or Go installation required. Launch LeGaJ from the Start Menu or desktop shortcut after install.

> **Building from source on Windows:** requires Python 3.12+ and Go 1.25+. See the macOS section below for the general source build pattern.

### macOS — Quick Install (Recommended)

The setup script checks for every dependency, installs anything missing via [Homebrew](https://brew.sh), and launches the app automatically:

```bash
git clone https://github.com/bot-bbio/legaj.git
cd legaj
chmod +x install_mac.sh && ./install_mac.sh
```

The script will walk you through anything requiring a manual step (e.g. the Xcode CLT install dialog). Re-run it after each prompt and it will pick up where it left off.

### macOS — Manual Install

**1. Xcode Command Line Tools**

Fyne uses `cgo`, which requires Apple's C compiler:
```bash
xcode-select --install
```
Skip this step if you have already installed Xcode or the CLT.

**2. Go 1.25+**

Download from [go.dev/dl](https://go.dev/dl/) or use Homebrew:
```bash
brew install go
```
Verify: `go version` must print `go1.25` or higher.

**3. Python 3.12+**

Download from [python.org/downloads](https://www.python.org/downloads/) or use Homebrew:
```bash
brew install python@3.12
```

**4. Clone and install dependencies**

```bash
git clone https://github.com/bot-bbio/legaj.git
cd legaj
pip3 install -r requirements.txt
```

**5. Run**

```bash
go run .
```

Go downloads its own module dependencies on first run. To build a persistent binary:
```bash
go build -o legaj && ./legaj
```

> **First launch:** The setup wizard runs automatically. You can also trigger it explicitly: `go run . wizard`
>
> **Data location (macOS):** LeGaJ writes your profile, job tracker, and generated PDFs to `~/Library/Application Support/LeGaJ/`, created automatically on first launch.
>
> **Gemini API key:** Required for AI-assisted field extraction and cover letter drafting. Enter it in the **Settings** tab during setup.

---

### Python Dependencies

| Package | Used for |
| :--- | :--- |
| `reportlab` | Generating PDF resumes and cover letters |
| `pypdf` | Parsing PDF resume files |
| `python-docx` | Parsing Word (.docx) resume files |
| `requests` | HTTP calls in the clipper and API helpers |
| `beautifulsoup4` | Parsing job board HTML |
| `openpyxl` | Spreadsheet export |

---

## GUI Guide

LeGaJ provides seven main views accessible from the top navigation bar:

| Tab | Description |
| :--- | :--- |
| **Dashboard** | High-level summary of active applications, status breakdown, and clipper listener state. |
| **Job Hunt** | Job Leads inbox — review and bulk-add clipped postings to your tracker. |
| **Base Profile** | Interactive form to manage your master profile stored in `references/user-profile.json`. |
| **Job Tracker** | Application table with status dropdowns (`Wishlist`, `Applied`, `Interviewing`, `Offer`, `Rejected`, `Ghosted`), notes, and a toolbar button to open job URLs directly in the browser. |
| **File Manager** | Browse, open, and rename assets in your local outputs and workspace paths. |
| **Settings** | Configure your Gemini API key, model selection, and output directory. |
| **Help** | Collapsible guides, bookmarklet installer, Chrome/Edge extension setup, and security disclosures. |

---

## CLI Usage

| Command | Arguments / Flags | Description |
| :--- | :--- | :--- |
| `wizard` | — | Force-launches the onboarding configuration wizard. |
| `parse-resume` | `-resume <path>` `[-output <path>]` | Parses a resume PDF/DOCX into unstructured text and exports a base JSON profile skeleton. |
| `tailor-resume` | `[-base <path>]` `[-tailored <path>]` | Displays a diff between base and tailored profile JSON. |
| `design-resume` | `[-profile <path>]` `[-output <path>]` | Compiles profile JSON into a print-ready PDF resume. |
| `write-cover-letter` | `-company <name> -draft <path/text>` `[-output <path>]` | Formats and compiles a cover letter PDF. |
| `manage-apps` | `<action> [flags]` | Manages the local job tracker. Actions: `list`, `add`, `update`. |

### Example Workflow

```powershell
# Parse your resume to build a base profile
./legaj.exe parse-resume -resume my_resume.pdf -output references/user-profile.json

# Compile base profile into a PDF
./legaj.exe design-resume -profile references/user-profile.json -output outputs/My_Resume.pdf

# Add an application to the tracker
./legaj.exe manage-apps add -company "Acme Corp" -role "Software Engineer" -location "Remote" -link "https://..."
```

---

## Browser Clipper

LeGaJ runs a local HTTP listener on the port configured in Settings. You can send job postings to it from any browser.

### Option A: Bookmarklet

1. Open the **Help** tab and click **Copy Bookmarklet Code**.
2. Make the bookmarks bar visible (`Ctrl+Shift+B` on Windows/Linux, `Cmd+Shift+B` on macOS).
3. Right-click the bookmarks bar and select **Add bookmark**.
4. Name it **Clip to LeGaJ** and paste the copied code as the URL.
5. Click the bookmark on any job posting page to send it to your Job Leads inbox.

### Option B: Chrome / Edge Extension

For sites with strict Content Security Policies that block external scripts:

1. Navigate to `chrome://extensions` or `edge://extensions`.
2. Enable **Developer mode** (top-right toggle).
3. Click **Load unpacked** and select the `extension` directory in your LeGaJ installation.
4. Pin the **LeGaJ Clipper** icon to clip listings from any tab.

**Supported job boards:** LinkedIn, Indeed, Greenhouse, Lever, Workday, Ashby, iCIMS.

---

## Setup Wizard

The wizard runs automatically on first launch and walks through these steps:

1. **Profile Ingestion** — Import and parse your base resume to seed your profile.
2. **Preferences** — Set target job titles, locations, and salary range.
3. **Connection** — Verify API key and directory permissions.
4. **Browser Clipper Setup** — Install the bookmarklet or load the unpacked extension.

---

## Security and Privacy

- **No auto-submit** — LeGaJ never submits applications on your behalf. You review and send all files yourself.
- **Local-first storage** — Your profile, resumes, credentials, and job logs never leave your disk.
- **Direct API calls** — Gemini requests go directly to Google; no server proxy sits in between.
- **No hardcoded secrets** — API keys are stored only in your local settings file, which is excluded from source control.
