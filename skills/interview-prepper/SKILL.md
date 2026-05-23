---
name: legaj:prep-interview
description: Generates an Anki card deck (.apkg) and a 1-page markdown cheatsheet based on the target company and role details.
---

# Interview Prepper

This skill automates the creation of study materials (Anki decks) and a 1-page summary sheet tailored to the target company, role description, and typical interview rounds.

## Workflow

1. **Input Collection:** Ask the user for the company name, role description, and optionally details about the interview rounds (e.g., behavioral, technical product, case study).
2. **Formulate Prep Data:** Use the LLM's knowledge (and optionally search engine queries) to generate structured preparation data:
   - **Company Profile:** Summarize mission, key products, and recent news.
   - **Elevator Pitch:** A customized 30-second introductory pitch for the applicant.
   - **Key Achievements:** Top 3 metrics-driven bullets from the user's profile to weave into answers.
   - **Questions to Ask:** 4-5 high-impact questions to ask the interviewer.
   - **Flashcards:** Generate 10-15 Q&A pairs covering:
     - Core company facts.
     - Behavioral questions (e.g., "Tell me about a time you resolved conflict") with STAR structure outline.
     - Industry-specific frameworks or concepts (e.g., RICE prioritization, SQL queries, Product Sense frameworks).
3. **Save Temp JSON:** Save this structured data to a temporary JSON file at `C:\Users\molus\projects\legaj\outputs\temp_prep_data.json`.
4. **Generate Assets:** Run the `scripts/prepare_interview.py` script:
   ```powershell
   & "C:\Users\molus\AppData\Local\Programs\Python\Python312\python.exe" <path-to-legaj>/scripts/prepare_interview.py "outputs/temp_prep_data.json" "all"
   ```
5. **Cleanup:** Delete the temporary `temp_prep_data.json` file.
6. **Notification:** Direct the user to their new Anki deck (`.apkg`) and the cheatsheet Markdown file under the `outputs/` folder.

## Foundational Mandates
- **Anki Ready Formatting:** Format flashcard questions and answers clearly. Use bolding and lists for readability in Anki's template.
- **Python Path:** Always use the full path to `python.exe` located at `C:\Users\molus\AppData\Local\Programs\Python\Python312\python.exe`.
