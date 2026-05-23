# LeGaJ - Let's Get a Job

LeGaJ is a CLI extension and set of agent skills designed to assist applicants throughout the job search and application process. It automates preparation, tracking, and editing while keeping the user in full control of submission.

## Tech Stack & Setup
- Python 3.12+ (standard installation at `C:\Users\molus\AppData\Local\Programs\Python\Python312\python.exe`)
- Core packages: `reportlab`, `pypdf`, `python-docx`, `genanki`, `openpyxl`, `beautifulsoup4`, `requests`.

### Installation
Run the following in your terminal to install the python dependencies:
```powershell
pip install -r requirements.txt
```

---

## Capabilities & Commands

The extension exposes the following `/` commands in your Antigravity interface:

### 1. `/legaj:parse-resume`
- **Goal:** Extract clean, structured JSON format of your resume from standard PDF/DOCX files.
- **Workflow:** Ingests a file path and outputs structured data representing education, experiences, and skills.

### 2. `/legaj:tailor-resume`
- **Goal:** Adjust resume bullets/sections dynamically.
- **Workflow:** Compares your base profile (`references/user-profile.json`) against a job description, rewriting points to emphasize relevant skills.

### 3. `/legaj:design-resume`
- **Goal:** Render a publication-quality single-page resume PDF from structured profile data.
- **Workflow:** Uses ReportLab to generate a highly professional layout.

### 4. `/legaj:write-cover-letter`
- **Goal:** Draft and render a single-page cover letter.
- **Workflow:** Combines your profile and target company description to generate a formatted PDF.

### 5. `/legaj:prep-interview`
- **Goal:** Collate study and prep assets.
- **Workflow:** Generates an Anki card deck (`.apkg`) for role study, compiles questions to ask, and designs a 1-page cheatsheet.

### 6. `/legaj:search-jobs`
- **Goal:** Find relevant job listings based on keywords.
- **Workflow:** Safely performs search engine queries to gather postings.

### 7. `/legaj:manage-apps`
- **Goal:** Automate application tracking.
- **Workflow:** Checks email confirmations, parses status, and updates `references/job-tracker.xlsx`.

---

## Security & Privacy Guardrails
1. **No Auto-Submit:** LeGaJ will *never* submit applications on your behalf.
2. **Local Processing:** Resumes, profiles, and trackers remain on your local machine.
3. **Sandbox Enforcement:** Avoid running arbitrary executables. All PDF outputs are generated programmatically using ReportLab.
