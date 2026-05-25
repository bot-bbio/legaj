# 📎 LeGaJ — Let's Get a Job

LeGaJ (Let's Get a Job) is a modern, offline-first application designed to streamline, structure, and automate the job search process. Built with a **Go + Fyne GUI/CLI frontend** and a **Python script backend**, LeGaJ combines local data parsing, ReportLab document generation, and Gemini AI capabilities to act as a personal recruitment coach.

Designed specifically to hold the user's hand through the job application lifecycle, it is equally friendly to fresh graduates with zero terminal experience (via a full-featured graphical interface) and power users (via a complete command-line interface).

---

## 🚀 Key Features

*   **Step-by-Step Onboarding Wizard**: A structured graphical guide to set up your API credentials, directories, and base profile immediately on first launch.
*   **Local Resume Parser**: Extracts unstructured text from PDF, DOCX, TXT, and Markdown resumes into a clean, structured JSON profile.
*   **Gemini AI-Powered Resume Tailoring**: Compares your base profile against target job descriptions to rewrite experience bullet points and reorder skills, emphasizing critical keywords while strictly preserving historical metrics.
*   **Publication-Quality PDF Compiler**: Compiles your tailored profile or cover letter into a professional, single-page, print-ready PDF using standard typography and geometry constraints (ReportLab).
*   **Job Tracker & Email Sync**: A local job application tracker (`job-tracker.json`) that can automatically scan your email inbox (via secure IMAP) for application confirmations, interview invitations, or rejections, updating their status automatically.
*   **One-Click Browser Clipper**: Clip job listings from LinkedIn, Indeed, Greenhouse, Lever, Workday, Ashby, and iCIMS directly into your application's inbox using a browser bookmarklet or custom unpacked Chrome/Edge extension.
*   **Interview Preparation Coach**: Automatically drafts elevator pitches, answers to tough behavioral questions (using the STAR method), and compiles a downloadable **Anki flashcard deck (`.apkg`)** and Markdown cheatsheet for study.

---

## 🛠️ Architecture & Tech Stack

LeGaJ uses a hybrid frontend/backend model to optimize cross-platform performance:
*   **Frontend / UI Shell**: Written in Go (1.25+) using the [Fyne v2](https://fyne.io/) GUI library. Handles application state, the local HTTP clipper server, API communication with Gemini, and GUI navigation.
*   **Core Backend**: A set of modular Python scripts located in `scripts/` that handle heavy file I/O:
    *   `parse_resume.py` (uses `pypdf` and `python-docx`)
    *   `generate_resume_pdf.py` / `generate_cover_letter_pdf.py` (uses `reportlab`)
    *   `prepare_interview.py` (uses `genanki`)
    *   `manage_applications.py` (uses `openpyxl` / JSON parsing)
    *   `search_jobs.py` (performs queries to gather active listings)

---

## 📦 Installation & Setup

### Prerequisites
1. **Python 3.12+**: Installed and accessible on your system.
2. **Go**: Installed if compiling from source.

### Setup Instructions

#### Option A: One-Click Setup (Recommended for Windows / GUI Users)
Simply double-click the [setup_and_run.bat](setup_and_run.bat) file in your File Explorer. 

This script will automatically:
1. Detect your local Python installation.
2. Upgrade `pip` and install all required Python libraries.
3. Launch the LeGaJ GUI application immediately.

#### Option B: Manual Terminal Setup
1. Install Python package dependencies:
   ```powershell
   pip install -r requirements.txt
   ```
2. Run the onboarding wizard to configure your directories and Gemini API Key:
   ```powershell
   ./legaj.exe wizard
   ```
3. Alternatively, launch the graphical dashboard:
   ```powershell
   ./legaj.exe
   ```

---

## 🖥️ Graphical User Interface (GUI) Guide

The GUI contains nine tabs accessible from the main navigation panel:

1.  **Dashboard**: Overview of your active applications, current search statistics, quick links to tailors/preps, and system status.
2.  **Job Hunt**:
    *   *Discovery Engine*: Search for active job postings by keyword and location.
    *   *Clipper Inbox*: Review listings clipped from your browser. Easily convert clipped listings into tracked applications with one click.
3.  **Base Profile**: Edit your core information (contact details, target roles, experience bullets, and skills) in a clean form. This data is saved locally to `references/user-profile.json`.
4.  **Job Tracker**: View, edit, add, or delete tracked applications. Update application statuses (`Wishlist`, `Applied`, `Interviewing`, `Offer`, `Rejected`) and append interview notes.
5.  **Tailor Assets**: Select a tracked job application, paste the job description, and call the Gemini API to automatically rewrite experience bullets and draft cover letters tailored specifically to that role.
6.  **File Manager**: Explore, open, or select files within your configured local directories.
7.  **Interview Prep**: Generate STAR-method study flashcards and elevator pitches based on your profile and target company description. Play interactive flashcards directly inside the app or export them to Anki.
8.  **Settings**: Update your Gemini API key, select models, adjust save directories (e.g., pointing to a local Google Drive folder), and enter IMAP credentials for email synchronization.
9.  **Help**: Full installation instructions for the browser clipper bookmarklet, Chrome extension instructions, and documentation.

---

## 💻 CLI Usage Reference

For automation or command-line workflows, LeGaJ provides a robust set of CLI commands:

| Command | Arguments / Flags | Description |
| :--- | :--- | :--- |
| `wizard` | *None* | Force-launches the onboarding configuration wizard. |
| `parse-resume` | `-resume <path>` `[-output <path>]` | Parses a resume PDF/DOCX into unstructured text and exports a base JSON profile skeleton. |
| `tailor-resume` | `[-base <path>]` `[-tailored <path>]` | Computes and displays a difference report between your base profile and a tailored profile JSON. |
| `design-resume` | `[-profile <path>]` `[-output <path>]` | Compiles profile JSON data into a print-ready PDF resume. |
| `write-cover-letter` | `-company <name> -draft <path/text>` `[-output <path>]` | Formats and compiles a cover letter PDF using profile details and a raw text draft. |
| `prep-interview` | `-data <json_path>` `[-mode <anki/cheatsheet/all>]` | Generates a study deck (`.apkg`) and/or a Markdown cheatsheet from interview data. |
| `search-jobs` | `-keywords <query> -location <loc>` | Queries job listings and saves search results to JSON. |
| `manage-apps` | `<action> [flags]` | Manages the local job tracker. Actions: `list`, `add`, `update`, `sync`. |

### CLI Example: Tailoring and Compiling
```powershell
# 1. Parse your resume to initialize base profile skeleton
./legaj.exe parse-resume -resume my_resume.pdf -output references/user-profile.json

# 2. Compile base profile into a PDF resume
./legaj.exe design-resume -profile references/user-profile.json -output outputs/My_Base_Resume.pdf

# 3. Add a new application to the tracker
./legaj.exe manage-apps add -company "Google" -role "Product Manager" -location "Remote" -link "https://careers.google.com/..."
```

---

## 📎 The Browser Clipper System

LeGaJ includes a local HTTP server (running in the background on the port configured in Settings) that listens for POST requests from job boards to capture job information. You can use two methods to send data to this server:

### 1. The Bookmarklet (Easiest)
Copy the bookmarklet code generated inside the **Help** tab of the GUI, add a new bookmark in your browser, and paste the code as the URL. When viewing a job description on LinkedIn or Indeed, click the bookmark to send the company name, role title, description, and link to your LeGaJ Clipper Inbox.

### 2. Unpacked Chrome/Edge Extension (Bypasses HTTPS restrictions)
If a site restricts bookmarklets via Content Security Policies (CSP), load the unpackaged extension:
1. Open your browser and navigate to `chrome://extensions` or `edge://extensions`.
2. Enable **Developer mode** (top-right toggle switch).
3. Click **Load unpacked** (top-left button).
4. Select the `extension` folder inside the LeGaJ directory.
5. Click the extension icon when viewing a job listing to clip it instantly.

---

## 🔒 Security & Privacy Guardrails

1.  **No Auto-Submit**: LeGaJ will **never** submit job applications automatically. It compiles documentation and organizes data, leaving you in complete control of submission.
2.  **100% Local Storage**: Your PII (name, phone, email, addresses), resume contents, and tracker data remain entirely on your local disk.
3.  **Local API Keys**: API keys for Gemini are kept in a local, uncommitted `.env` file. No middleman servers receive your API requests.
4.  **Sandbox Policy**: Avoid executing external or unverified scripts directly. All document generation and parsing are executed via isolated Python packages.
