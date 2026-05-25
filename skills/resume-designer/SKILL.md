---
name: legaj:design-resume
description: Generates a beautiful, single-page PDF resume using ReportLab from a structured JSON profile. Defaults to references/user-profile-tailored.json.
---

# Resume Designer

This skill compiles structured JSON profile data (either the base profile or a tailored profile) into a publication-quality, single-page PDF resume using ReportLab.

## Workflow

1. **Select Source JSON:** Ask the user if they want to build the resume from the base profile (`references/user-profile.json`) or a tailored profile (`references/user-profile-tailored.json`). Default to `references/user-profile-tailored.json` if it exists.
2. **Determine Output Path:** Ask the user for the output PDF file path. Default to:
   `~\projects\legaj\outputs\<Name>_Resume.pdf` (or tailored company-specific naming like `<Company>_Resume_Tailored.pdf`).
3. **Generate PDF:** Run `scripts/generate_resume_pdf.py` passing the source JSON and the output PDF path:
   ```powershell
   & "python" <path-to-legaj>/scripts/generate_resume_pdf.py "<source_json>" "<output_pdf_path>"
   ```
4. **Validation:** Ensure the script exits successfully. Inform the user of the path where the PDF is located.

## Foundational Mandates
- **Single Page Constraints:** Ensure the content in the source JSON is concise. The PDF styling is set with compact margins (0.5 inch) and spacing to fit standard career profiles on exactly one page.
- **Python Path:** Always use the full path to `python.exe` located at `python`.
