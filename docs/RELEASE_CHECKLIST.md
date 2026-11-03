# LeGaJ release checklist (manual)

Run these on a clean Windows 10 or 11 machine with the built installer before a release. Automated checks live in `scripts/verify.ps1`. This list covers what a headless test can't see.

## Gemini key storage (R4)
- [ ] Run the real Credential Manager round trip: `$env:LEGAJ_HOME = "$env:TEMP\legaj-smoke"; go test -tags credman ./internal/secrets/`. It passes and leaves no `LeGaJ-test` entry in Credential Manager.
- [ ] After saving a key in the app, Credential Manager › Windows Credentials shows **LeGaJ:gemini-api-key**.

## Opening links and files (R33, R10)
- [ ] In Pipeline, **Posting** opens the job page in the default browser, and **Resume** opens the PDF in the default viewer. No console window flashes.
- [ ] Upload a second resume after marking a row Applied: that row's **Resume** still opens the first file.

## Single instance (R42)
- [ ] Launch LeGaJ, then minimize it. Launch it again from the Start menu: no second window appears, and the first window is restored and focused.
- [ ] Launch LeGaJ twice in quick succession, as with a double-click on the shortcut: only one window ever appears.

## Calendar export (R31)
- [ ] Add a round (18:00, America/New_York) and use **Add to calendar**. The `.ics` opens in Outlook (desktop) at the right local time, with the link, interviewers and prep notes in the description.
- [ ] Import the same file at calendar.google.com › Settings › Import & export. The event lands at the same time. Export again after editing the round and re-import: the event is updated, not duplicated.

## Installer (R38)
- [ ] On a standard (non-admin) account, run `LeGaJ-Setup-<version>.exe`: no UAC prompt; it installs to `%LOCALAPPDATA%\Programs\LeGaJ` and adds a Start menu entry.
- [ ] On a machine without WebView2 (or with it removed), the installer installs it first; on a machine that has it, no WebView2 step runs.
- [ ] With LeGaJ running, start the installer or uninstaller: it asks you to close LeGaJ first.
- [ ] Uninstall and answer **Yes** to "Keep your applications and documents?": `%LOCALAPPDATA%\LeGaJ` is still there; reinstall and the data is back.
- [ ] Uninstall and answer **No**: `%LOCALAPPDATA%\LeGaJ`, `%APPDATA%\legaj.exe` and the **LeGaJ:gemini-api-key** credential are gone. In both cases the `LeGaJ` value under `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` is gone.

## Release sweep (item 45)
Automated in `scripts/verify.ps1`: `TestR4_R34_SentinelScan` runs onboarding, a forced Gemini 400, profile, a job, letter generation and approval and form answers with sentinel values, then byte-scans the data folder (no key anywhere; no email, phone or resume line in the logs or in tables that shouldn't hold them).

- [ ] `powershell -NoProfile -File scripts/audit-regressions.ps1` lists no gaps: every V1 bug in the PRD map has a named test.
- [ ] Live Gemini smoke with your own key (it never runs in CI): `$env:LEGAJ_HOME = "$env:TEMP\legaj-live"; $env:LEGAJ_LIVE_GEMINI_KEY = '<key>'; go test -tags live -count=1 ./internal/smoke/`, then `Remove-Item Env:LEGAJ_LIVE_GEMINI_KEY`.
- [ ] On a clean Windows 10 VM: install, finish onboarding with **made-up** sentinel values (a throwaway key that you revoke afterwards, a fake email, phone and resume line), paste and clip jobs, generate and approve a letter, draft answers, open a follow-up, add a round. Close LeGaJ, then run `powershell -NoProfile -File scripts/scan-sentinels.ps1 -Key ... -Email ... -Phone ... -ResumeLine ...`. It must report nothing, including in the WebView2 folder.
- [ ] Generate 10 letters while another app has focus: LeGaJ never steals focus and no console window flashes (R6).
- [ ] Clip on the 6 sites from the spike matrix.
