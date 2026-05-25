---
name: legaj:manage-apps
description: Add, update, or sync job application status inside references/job-tracker.xlsx and optionally match against incoming email updates.
---

# Application Manager

This skill manages the local `references/job-tracker.xlsx` application spreadsheet. It supports manually logging entries, changing statuses, and automatically syncing details by scanning a configured IMAP email inbox.

## Workflow

### Action 1: Add Application
1. **Details:** Collect company name, job title, location, and listing URL.
2. **Paths:** Ask if a tailored resume or cover letter was generated for this application and capture their paths.
3. **Execute:** Run the tracker script:
   ```powershell
   & "python" <path-to-legaj>/scripts/manage_applications.py "references/job-tracker.xlsx" add "<company>" "<role>" "<location>" "<link>" "<resume_used>" "<cover_letter_used>" "<notes>"
   ```

### Action 2: Update Application Status
1. **Details:** Collect company name, job title, and the new status (e.g., Wishlist, Applied, Interviewing, Offer, Rejected, Withdrawn).
2. **Execute:** Run the tracker script:
   ```powershell
   & "python" <path-to-legaj>/scripts/manage_applications.py "references/job-tracker.xlsx" update "<company>" "<role>" "<new_status>" "<optional_notes>"
   ```

### Action 3: Sync Emails
1. **Config Check:** Look for IMAP configuration in the local `.env` file (`LEGAJ_IMAP_SERVER`, `LEGAJ_EMAIL`, `LEGAJ_PASSWORD`). If missing, ask the user to input them or write them to the local `.env` file.
2. **Execute:** Run the tracker script in sync mode:
   ```powershell
   & "python" <path-to-legaj>/scripts/manage_applications.py "references/job-tracker.xlsx" sync "<email>" "<password>" "<imap_server>"
   ```
3. **Report:** Display any status updates matching active jobs.

## Foundational Mandates
- **Credential Protection:** Never log, print, or commit the email password to git. Keep all configuration variables strictly confined to a local-only `.env` file.
- **Python Path:** Always use the full path to `python.exe` located at `python`.
