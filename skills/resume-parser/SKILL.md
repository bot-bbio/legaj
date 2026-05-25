---
name: legaj:parse-resume
description: Extracts text from a standard PDF/DOCX resume and formats it into the structured JSON user-profile format. Use this when the user wants to ingest or update their base resume/profile.
---

# Resume Parser

This skill automates the extraction and structuring of a resume from PDF, DOCX, or text files into the standard LeGaJ user profile format.

## Workflow

1. **Get File Path:** Ask the user for the path to their existing resume file (PDF, DOCX, TXT, or MD) if not already provided.
2. **Extract Text:** Run `scripts/parse_resume.py` to extract the raw text content:
   ```powershell
   & "python" <path-to-legaj>/scripts/parse_resume.py "<resume_path>"
   ```
3. **Parse and Structure:** Take the output raw text and map it into the JSON schema defined in `references/user-profile.json`. 
   Ensure it includes:
   - `personal_info` (name, email, phone, location, links)
   - `target_roles` (list of target roles)
   - `education` (list of institutions, degree, major, gpa, date, details)
   - `experience` (list of companies, role, location, start/end dates, bullet points)
   - `projects` (list of projects, description, tech stack, details)
   - `skills` (categorized list of technical, business, and domain skills)
4. **Save to Profile:** Save the generated JSON to `references/user-profile.json` in the active project directory, overwriting it or updating it.
5. **Verify:** Confirm with the user that their details were extracted correctly and display a summary of the parsed experience.

## Foundational Mandates
- **Always preserve detail:** Do not summarize bullet points too heavily during parsing; keep the action-result structure of resume bullets.
- **Python Path:** Always use the full path to `python.exe` located at `python`.
