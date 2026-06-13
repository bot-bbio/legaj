# LeGaJ — Let's Get a Job

[![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?style=flat-square&logo=go)](https://go.dev)
[![Python Version](https://img.shields.io/badge/Python-3.12%2B-3776AB?style=flat-square&logo=python)](https://python.org)
[![Fyne Version](https://img.shields.io/badge/UI-Fyne%20v2-blueviolet?style=flat-square)](https://fyne.io)

LeGaJ (Let's Get a Job) is an offline-first job search suite. It pairs a **Go + Fyne desktop GUI** with a **Python document backend** for parsing resume files, compiling single-page PDF resumes and cover letters, and tracking job applications locally. All personal data stays on your machine.

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
