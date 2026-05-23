---
name: legaj:write-cover-letter
description: Analyzes a job description and generates a customized PDF cover letter based on the user's active profile details.
---

# Cover Letter Writer

This skill drafts and generates a single-page cover letter PDF by matching the applicant's experience to a specific job posting.

## Workflow

1. **Input Collection:** Ask the user for the target company, job title, and the job description URL/text.
2. **Draft Content:** Read `references/user-profile.json` (or `references/user-profile-tailored.json` if tailored first) and generate a compelling, professional cover letter draft:
   - Max 4 paragraphs.
   - Standard greeting (e.g., "To Whom It May Concern," or hiring manager's name).
   - Paragraph 1: State the position applied for and a hook showing alignment with company mission.
   - Paragraph 2-3: Map 2 specific, quantified achievements from experience to target requirements.
   - Paragraph 4: Professional closing and call to action.
3. **Save Draft Text:** Save the drafted text to a temporary text file in `C:\Users\molus\projects\legaj\outputs\temp_cover_letter.txt`.
4. **Generate PDF:** Run `scripts/generate_cover_letter_pdf.py` passing user profile path, temp draft path, and target PDF path:
   ```powershell
   & "C:\Users\molus\AppData\Local\Programs\Python\Python312\python.exe" <path-to-legaj>/scripts/generate_cover_letter_pdf.py "references/user-profile.json" "outputs/temp_cover_letter.txt" "outputs/<Company>_Cover_Letter.pdf"
   ```
5. **Cleanup:** Delete the temporary `temp_cover_letter.txt` file.
6. **Notification:** Advise the user that their Cover Letter PDF has been successfully generated.

## Foundational Mandates
- **Strict 1-Page Limit:** Ensure the generated letter text fits perfectly on a single page under standard letter margins.
- **Python Path:** Always use the full path to `python.exe` located at `C:\Users\molus\AppData\Local\Programs\Python\Python312\python.exe`.
- **Delete Temp Files:** Always clean up the temporary text draft after compiling the PDF.
