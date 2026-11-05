# LeGaJ privacy notice

LeGaJ runs on your computer. There is no LeGaJ server and no account, and LeGaJ collects no analytics or telemetry. This notice lists everything the app stores and every way data leaves your computer.

## What stays on your computer

Everything you put into LeGaJ is stored in your Windows user profile, in `%LOCALAPPDATA%\LeGaJ`:

| Item | Where |
| --- | --- |
| Your profile, applications, job descriptions, letter drafts, form answers, interview rounds, follow-ups and settings | `legaj.db` (a SQLite database) |
| Resume files you upload | `resumes\` |
| Approved cover letters (PDF) | `documents\` |
| Calendar files for interviews | `exports\` |
| Daily database backups (the last 7, plus copies taken before upgrades) | `backups\` |
| A log of errors and events, with your key, email addresses and phone numbers removed | `logs\` |

Your **Gemini API key** is not in that folder. It's stored in **Windows Credential Manager** (entry `LeGaJ:gemini-api-key`), encrypted with your Windows login. LeGaJ only ever shows its last four characters.

The app's window is drawn by Microsoft Edge WebView2, which keeps its own small browser profile in `%APPDATA%\legaj.exe` (for example, which mail client you chose on the Follow-ups screen).

## What leaves your computer

### Google Gemini (when you use an AI feature, or a clip is missing details)

When you parse a resume, generate a cover letter, regenerate a paragraph, draft form answers or ask for interview questions, LeGaJ sends a request straight from your computer to Google's Gemini API with your own key. A request contains:

- the job description of **that one application** (never other applications);
- your profile: name, headline, target role, summary, experience, education and skills. Your **email, phone number, location and links are left out**;
- for resume parsing, the resume file you chose.

**Clipping a job can also reach Gemini, without a further click.** The clipper hands the page only to LeGaJ on your computer. But if you have a key saved and LeGaJ can't find a clip's company or role, it then asks Gemini for them. That request contains the page's title and up to 20 KB of the description LeGaJ saved, with your email and phone number removed. LeGaJ adds nothing from your profile, but on a page it doesn't recognise, the description can be all the text it could read there, which can include your name, if the page shows it, and other people's names. To limit what's saved and sent, select the job description before you click the clipper.

How Google may use that data depends on your Gemini plan:

- **Free tier (unpaid):** Google's terms allow it to use your prompts and responses to improve its products, and human reviewers may read them. Don't put anything in your profile or resume that you wouldn't want reviewed.
- **Paid tier (billing enabled on your Google Cloud project):** Google's terms say prompts and responses aren't used to improve its products. They are kept for a limited time to detect abuse.

These terms are Google's, not LeGaJ's, and they can change. Read the current [Gemini API terms](https://ai.google.dev/gemini-api/terms) before you choose.

### GitHub (the update check)

When the **update check** is on (Settings, on by default), LeGaJ asks GitHub once per launch whether a newer release exists: one request to `api.github.com` for this project's latest release. GitHub sees your IP address and the LeGaJ version, as with any web request. LeGaJ never downloads or installs updates itself; **Download** opens the release page in your browser. Turn the check off in Settings and no request is made.

### Your mail client and calendar

LeGaJ never sends email. For a follow-up it opens a draft in Gmail, Outlook or your mail app, and you press Send there. **Add to calendar** writes an `.ics` file and opens it in your calendar app. Links you click (job postings, meeting links) open in your browser.

## Removing your data

- Delete an application in Pipeline and, after the 10-second undo window, it is removed with its letters (the PDF files too), interview rounds and their calendar files, follow-ups and draft answers. Answers you approved stay in your answer bank until you remove them there.
- **Remove** in Settings deletes the Gemini key from Credential Manager.
- When you uninstall, LeGaJ asks **"Keep your applications and documents?"**. **No** deletes `%LOCALAPPDATA%\LeGaJ`, the WebView2 profile and the saved key.
