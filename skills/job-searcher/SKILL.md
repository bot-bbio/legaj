---
name: legaj:search-jobs
description: Searches for job postings based on role and location, generating direct links and compiling a list of target roles.
---

# Job Searcher

This skill searches for active job listings matching the user's criteria, generates click-ready search board links, and compiles a table of potential targets.

## Workflow

1. **Input Collection:** Ask the user for job keywords (e.g., "Product Manager", "Software Engineer") and location (e.g., "New York, NY", "Remote").
2. **Generate Board Links:** Run the `scripts/search_jobs.py` script to generate direct search links:
   ```powershell
   & "python" <path-to-legaj>/scripts/search_jobs.py "<keywords>" "<location>"
   ```
3. **Live Web Search:** Use the agent's web search capability to lookup actual postings published in the last 7 days matching the keywords.
4. **Compile Results:** Parse findings and compile a table containing:
   - Job Title
   - Company
   - Location
   - Direct Link
   - Key Requirements
5. **Save Report:** Save the compiled table of target jobs to `outputs/Job_Search_Report.md`.
6. **Action:** Present the table to the user and ask which jobs they want to select for resume tailoring or tracking.

## Foundational Mandates
- **Ensure Clickable Links:** Always provide fully formed URLs that the user can click directly to view listings.
- **Python Path:** Always use the full path to `python.exe` located at `python`.
