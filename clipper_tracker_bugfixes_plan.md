# LeGaJ Clipper & Tracker Refinement Plan

This document outlines the design and proposed changes to resolve issues in the **Clipper** (bookmarklet/listening server) and **Tracker** (Fyne GUI status board) components.

---

## 📎 Clipper Bug Fixes & Refinements

### 1. Robust Job Title Fallbacks
When scraping a job page, selectors can fail. We will implement a hierarchical lookup to locate the job title:
1.  **CSS Selectors:** Check for common classes like `.job-title`, `.posting-title`, `.title`, or `.role`.
2.  **Open Graph Meta:** Parse the `<meta property="og:title">` tag.
3.  **Page Title Tag:** Parse the `<title>` element, sanitizing branding suffixes (e.g. `| LinkedIn`, `- Indeed`).
4.  **URL Parsing:** Infer keywords from path segments in the URL if all else fails.

### 2. Location Scraper
Add explicit scraping routines to target job location:
*   Identify tags containing keywords: `location`, `workplace`, `geography`, `remote`, `hybrid`.
*   Look for common geographic patterns (e.g., `City, State` or `Country`) inside secondary heading components.

### 3. Sanitized Cover Letter Formatting
*   **No Referral Citations:** Modify the LLM drafting prompt to ensure it never mentions where the job listing was found (e.g., do not say *"I am writing to apply for the position I saw on LinkedIn"*).
*   **Signature Clean-Up:** Restrict the section below the sign-off (`"Sincerely,"`) to output **only the applicant's name** from the profile, omitting extra contact/address details.
*   **Title/Filename Suffix Strip:** Strip portal brand names (such as `LinkedIn`) from the role title before compiling the cover letter document title and PDF output filename.

### 4. Job Description Extraction
Enhance the browser bookmarklet to extract full job descriptions. This ensures the tailoring system has complete text to run comparisons against your profile.

### 5. Backend JSON Quality Buffer
Introduce `references/clipped-jobs.json` to store scraped details. We will check each entry to filter out generic pages:
*   **Filtering:** Compare titles against low-quality generic strings like `"Jobs at Career Page"`, `"Current Openings"`, `"Apply Now"`.
*   **Correction:** Prompt Gemini or apply string replacements to reconstruct the true role title from headers/description metadata before appending it to the UI inbox.

---

## 📊 Tracker & UI Workflow Improvements

### 1. Discovery Engine & Clipper Inbox Layout Redesign
Instead of layout-heavy cards that stack vertically and clutter the view:
*   Convert the **Discovery Engine** results and the **Clipper Inbox** to use a compact table/grid format.
*   Clearly align columns for Company, Role, Location, Source, and Action buttons.

### 2. Multi-Select Sequential Generation
*   **Row Checkboxes:** Add checkable columns next to each tracked application in the main board.
*   **Bulk Tailor Action:** Provide a button to "Tailor Selected". Clicking this will trigger the AI tailoring and PDF generation sequentially for all checked applications.
