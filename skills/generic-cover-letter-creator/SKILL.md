---
name: legaj:create-generic-cover-letter
description: Generates a generic, versatile cover letter template with placeholder variables by parsing the user's pasted cover letter for style, tone, and structure.
---

# Generic Cover Letter Creator

This skill extracts the style, tone, and structure of an existing cover letter provided by the user, and converts it into a generic, versatile cover letter template with placeholder bracketed variables (such as [Full Name], [Target Role], [Target Company], etc.). It writes the template to `references/generic_cover_letter_template.txt` and compiles a PDF version at `outputs/generic_cover_letter_template.pdf`.

## Workflow

1. **Input Collection:** Prompt the user to paste their existing ideal cover letter text.
2. **Genericize and Style Parse:** Call Gemini with the pasted letter text. The LLM parses it for layout, paragraph sequence, and tone, replacing all PII and specific company/role/metric details with bracketed placeholder variables while maintaining the original prose structure.
3. **Save Template File:** Save the generated template text to `references/generic_cover_letter_template.txt`.
4. **Compile PDF:** Compile the PDF representation of the template to `outputs/generic_cover_letter_template.pdf` using the PDF generation script:
   ```powershell
   & "C:\Users\molus\AppData\Local\Programs\Python\Python312\python.exe" scripts/generate_cover_letter_pdf.py "references/user-profile.json" "references/generic_cover_letter_template.txt" "outputs/generic_cover_letter_template.pdf"
   ```
5. **Notify User:** Inform the user that their generic cover letter template has been created and saved at `references/generic_cover_letter_template.txt` and `outputs/generic_cover_letter_template.pdf`.
