# Project LeGaJ: Bug Squashing & Feature Refinement Plan

This document outlines the roadmap for refining the LeGaJ (Job Hunt Engine) application, focusing on UI/UX improvements in the Fyne-based GUI, functional enhancements to the tracking system, and optimizing the job discovery engine.

---

## 1. Frontend & UI/UX (Fyne GUI)

### 📂 File Manager Improvements
*   **Expansion & Scaling:**
    *   **Task:** Increase the default dimensions of the File Manager panel.
    *   **Refinement:** Implement a responsive layout using `container.NewHSplit` to allow users to manually resize the file explorer vs. the preview pane. Set a more generous minimum size for the explorer sidebar.
*   **Icon & Label Clarity:**
    *   **Task:** Fix "awkward" icon text display.
    *   **Refinement:** Replace generic labels with `widget.Card` or custom containers that use `layout.NewVBoxLayout`. Ensure text is truncated with ellipses (`...`) if too long, or wrapped cleanly with a minimum height to prevent grid misalignment.
*   **Grid Mode Spacing:**
    *   **Task:** Increase spacing between folder icons and text.
    *   **Refinement:** Use `layout.NewGridWrapLayout` with an increased cell size to account for padding. Explicitly add `container.NewPadded` around the label component to ensure breathing room between the icon and the filename.
*   **Navigation Logic:**
    *   **Task:** Replace the "parent folder" (`..`) icon with a dedicated "Back" button.
    *   **Refinement:** Add a toolbar at the top of the file manager with a `widget.Button` using `theme.NavigateBackIcon()`.
    *   **Mouse Navigation:** Map the "Back" mouse button (Button 4/XButton1) to the directory navigation stack. This will require intercepting low-level mouse events in `gui.go` or using a global shortcut listener if supported by the driver.

---

## 2. Job Tracker & Status Board

### 📊 Data Management
*   **Enhanced Columns:**
    *   **Task:** Add a "Notes/Details" column to the tracker.
    *   **Refinement:** Update the `references/job-tracker.xlsx` structure and the internal `Application` struct. Add a multi-line `widget.Entry` in the application detail view for rich-text notes.
*   **Storage Refactoring:**
    *   **Task:** Remove the dependency on external spreadsheet software.
    *   **Refinement:** Transition the backend from `.xlsx` to a local `applications.json` or `jobs.db` (SQLite). This allows for faster UI updates, better concurrency, and removes the "Remove spreadsheet" friction mentioned.
*   **Status Management:**
    *   **Task:** Enable editing via a simple dropdown menu.
    *   **Refinement:** Replace the text input for "Status" with a `widget.Select` containing standard options: `Applied`, `Interviewing`, `Offer`, `Rejected`, `Ghosted`.

### 🔗 Connectivity
*   **Hotlinking Files:**
    *   **Task:** Status board should hotlink to local file locations.
    *   **Refinement:** When an application is selected, provide clickable links to the specific tailored resume and cover letter generated for that role. Use `fyne.App.OpenURL` with `file://` protocols.

---

## 3. Job Search & Discovery Engine

### 🔍 Search Optimization
*   **Producing Better Results:**
    *   **Task:** Fix the "Job search not producing jobs" issue.
    *   **Refinement:** 
        *   Audit the `search-jobs` logic (likely in Python or `exec.go`). 
        *   Implement **Meaningful Search Criteria**: Add filters for "Posted within X days" and "Remote/On-site".
        *   **Direct-to-Company Focus:** Prioritize company career pages (`/careers`) over aggregators. Implement a check to detect if a link is a redirect to a known job board (e.g., Indeed, LinkedIn) and flag it.
*   **Site Quality Validation:**
    *   **Task:** Test and rank job board quality.
    *   **Refinement:** Create a blacklist/whitelist for domain sources. Filter out "low-signal" sites that require multiple logins or have expired listings.

### 🌐 Integrations
*   **LinkedIn & External Imports:**
    *   **Task:** Integrate with a browser extension or LinkedIn reader.
    *   **Refinement:** Develop a "Clip to LeGaJ" workflow. This could involve a simple listener in the Go app that accepts POST requests from a browser bookmarklet or extension, allowing users to "Send to LeGaJ" directly from a LinkedIn job page.
*   **Google Tooling:**
    *   **Task:** Interlace with Google-specific search tools.
    *   **Refinement:** Utilize the Google Custom Search API (with specific dorks for direct company listings) to improve the signal-to-noise ratio.

---

## 4. Resume & Security Mandates

### 📄 Document Integrity
*   **Super Strict Formatting:**
    *   **Task:** Super strict formatting on resume; make NO changes.
    *   **Refinement:** Lock the ReportLab template. The "Tailor Resume" function should *only* modify the content (strings) and never the layout (coordinates, fonts, margins). Implement a "Visual Lock" check to ensure the output remains a single page.

### 🛡️ Security & Ethics
*   **Engagement Disclosure:**
    *   **Task:** Security disclosure on engagement of the job hunt tool.
    *   **Refinement:** Add a "Security & Ethics" splash screen or "About" section. Explicitly state that the tool **never auto-submits** and that all data (API keys, PII) stays local. Add a disclaimer regarding the use of AI for tailoring bullets.

---

## 5. Wizard & Onboarding

*   **Substantial Wizard Function:**
    *   **Task:** Make the Wizard more robust.
    *   **Refinement:** Transform the wizard into a multi-step setup guide:
        1.  **Profile Ingestion:** Import/Parse base resume.
        2.  **Preferences:** Set job titles, locations, and target salary.
        3.  **Connection:** Test search engine APIs and directory permissions.
        4.  **Verification:** Run a "Test Search" to ensure the pipeline is hot.
