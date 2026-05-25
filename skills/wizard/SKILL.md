---
name: legaj:wizard
description: Interactive wizard that holds the user's hand through job search profiles, resume parsing, job application tracking, document tailoring, and interview prep.
---

# LeGaJ Wizard

This orchestrator skill guides the user step-by-step through their job search preparation, tracking, and application compiling. It is designed for non-technical users, conducting conversational check-ins at every step and executing tools in the background.

## Workflow

### Phase 1: Onboarding & Job Profile Building
1. **Interactive Greeting**: Welcome the user to LeGaJ and explain that you are here to guide them through setting up their job search pipeline.
2. **Collect Job Goals**: Ask the user what types of roles they are looking for (e.g., Software Engineer, Product Manager, Data Analyst), what industries they prefer, and any location constraints.
3. **Build Job Profile**: Save these details to a JSON file at `references/user-job-profile.json` for persistence:
   ```json
   {
     "target_roles": ["Product Manager"],
     "preferred_industries": ["Tech", "Finance"],
     "location_preferences": ["New York, NY", "Remote"]
   }
   ```

### Phase 2: Resume Ingestion & Parsing
1. **Request Resume**: Prompt the user to provide the absolute path to their current resume (PDF, DOCX, TXT, or MD format). If they are unsure how to find the path, gently guide them.
2. **Execute Parser**: Once the path is provided, execute the `legaj:parse-resume` command (which runs `scripts/parse_resume.py` and structures the output).
3. **Confirm Details**: Present the parsed data back to the user in a clean, human-readable summary. Ask if they want to make any corrections or additions (e.g., missing phone number, degree details, or projects).

### Phase 3: Select Experience Emphases
1. **Highlighting Strengths**: Based on the target job profile (Phase 1) and parsed resume (Phase 2), suggest key themes and technical skills they should emphasize.
2. **Interactive Choices**: Ask the user what specific aspects of their background they want to lead with (e.g., a specific project, technical tools, or leadership experience). Store these preferences in `references/user-profile.json` under an `emphasis_notes` field.

### Phase 4: Job Tracker Setup
1. **Initialize spreadsheet**: Check if `references/job-tracker.xlsx` exists. If not, run the creation script:
   ```powershell
   & "python" scripts/create_tracker.py
   ```
2. **Status Report**: Confirm the spreadsheet is ready. Explain that they can view, add, or update jobs by talking to you directly. Show them a formatted Markdown list/table of any existing entries.

### Phase 5: Continuous Application & Tailoring Companion
Once onboarding is complete, explain to the user that they can run commands or ask you to perform any of the following tasks:
- **Search Jobs**: Prompt for keywords and location, execute `legaj:search-jobs` and display a clean list of job search links.
- **Track Application**: Ask for details (company, role, location, listing URL) and run `legaj:manage-apps` to add it as a new entry.
- **Tailor Documents**: Walk through tailoring documents for a target company:
  1. Prompt for the job description text/requirements.
  2. Run `legaj:tailor-resume` to rewrite bullet points.
  3. Compile the tailored PDF via `legaj:design-resume` and draft a cover letter PDF using `legaj:write-cover-letter`.
  4. Save both outputs inside the `outputs/` folder and log their paths in the tracker.
- **Interview Prep**: Run `legaj:prep-interview` to generate custom flashcards (Anki `.apkg` deck) and a cheatsheet markdown document, letting them practice behavioral/technical concepts.

## Foundational Mandates
- **Conversational Tone**: Use warm, encouraging language. Avoid technical jargon or exposing raw terminal syntax unless troubleshooting.
- **Python Path**: Always use the full path to `python.exe` located at `python`.
