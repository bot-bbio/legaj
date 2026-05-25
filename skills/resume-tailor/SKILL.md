---
name: legaj:tailor-resume
description: Tailors experience bullets and skills in references/user-profile.json to align with a target job description, saving the result to user-profile-tailored.json and outputting a report.
---

# Resume Tailor

This skill tailors the applicant's experience bullets and highlighted skills to emphasize target keywords and project matches for a specific job application.

## Workflow

1. **Input Collection:** Ask the user for the job posting text or URL if not already provided.
2. **Fetch Job Details (if URL):** If the input is a URL, use `web_fetch` or similar tools to extract the job responsibilities and requirements.
3. **Analyze & Tailor:** Read `references/user-profile.json` and align the profile contents to match the job posting:
   - Identify critical keywords and key responsibilities in the job posting.
   - For each experience, rewrite relevant bullet points to emphasize accomplishments that align with the requirements, while preserving truthfulness and exact metrics.
   - Adjust the ranking/categories of skills to elevate the most critical keywords.
4. **Save Tailored Profile:** Write the tailored profile output to `references/user-profile-tailored.json`.
5. **Run Diff Report:** Run `scripts/tailor_resume.py` to display a nice visual diff of the changes:
   ```powershell
   & "python" <path-to-legaj>/scripts/tailor_resume.py "references/user-profile.json" "references/user-profile-tailored.json"
   ```

## Foundational Mandates
- **Retain metrics:** Ensure all historical metrics (percentages, dollar values, size of teams) are strictly preserved.
- **Python Path:** Always use the full path to `python.exe` located at `python`.
