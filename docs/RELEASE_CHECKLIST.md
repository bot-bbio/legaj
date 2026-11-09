# LeGaJ release checklist (manual)

Run these on a clean Windows 10 or 11 machine with the built installer before a release. Automated checks live in `scripts/verify.ps1`. This list covers what a headless test can't see.

## Automated sweep on clean runners
`.github/workflows/sweep.yml` (Actions › sweep › Run workflow) builds the installer as a release does and runs `scripts/sweep/` on fresh GitHub-hosted Windows runners, a new machine per job. It drives the installed app through its window, as a user does: UI Automation, the real mouse and keyboard, the Windows file dialog. Edge runs over the DevTools protocol, with a throwaway profile. A stand-in Gemini answers the app's calls, made reachable by a hosts-file entry and a throwaway root certificate, so no real key is used. The scripts change machine settings, so never run them on a real PC.

It checks: the silent install and an install by a standard user; the WebView2 step on a machine without WebView2; the wizard, with the key in Credential Manager and the resume chosen in the file dialog; the clipper (dragging it to Edge's favorites bar, copying it, real favorite clicks on a local page and on live Greenhouse, Lever, Ashby, Workday, LinkedIn and Indeed postings); the reinstall banner with the port taken; opening Posting, Letter and Resume, including the resume an Applied row keeps; focus during bulk letter generation; single instance; Start with Windows; the sentinel scan; Setup's and the uninstaller's prompts while LeGaJ runs; and both uninstall answers. Each check is reported as PASS, FAIL or SKIP in the run's summary.

It can't stand in for: a real Windows 10 or 11 desktop (runners are Windows Server, signed in as an administrator, with Edge's first-run pages turned off by policy); LinkedIn signed in; Outlook and Google Calendar; real Gemini output, which the live smoke test covers. Those items stay on this list.

## Gemini key storage (R4)
- [ ] Run the real Credential Manager round trip: `$env:LEGAJ_HOME = "$env:TEMP\legaj-smoke"; go test -tags credman ./internal/secrets/`. It passes and leaves no `LeGaJ-test` entry in Credential Manager.
- [ ] After saving a key in the app, Credential Manager › Windows Credentials shows **LeGaJ:gemini-api-key**.

## Opening links and files (R33, R10)
- [ ] In Pipeline, **Posting** opens the job page in the default browser, and **Resume** opens the PDF in the default viewer. No console window flashes.
- [ ] Upload a second resume after marking a row Applied: that row's **Resume** still opens the first file.

## Clipper (R18)
Automated in `scripts/verify.ps1`: the built bookmarklet and the popup's real page and script run together in jsdom, and the server's checks run over real HTTP. No real browser runs in verify, so this list is the only browser check of a release build.

- [ ] In the setup wizard, drag **Clip to LeGaJ** onto the bookmarks bar, in Edge and in Chrome. Dragging out of the app window can't be scripted, so this is its first real test. If the drop does nothing, try **Copy the clipper**, paste it as a new bookmark's address, and report the failed drag.
- [ ] Click the new bookmark on a job page, then **Save**. The job appears in Pipeline at once, and **Install the clipper** leaves the setup checklist.
- [ ] Close LeGaJ. In PowerShell, hold the port from the bookmark's address: `$l = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, <port>); $l.Start()`. Start LeGaJ: the reinstall banner appears, and **Reinstall bookmarklet** opens the install page in Settings. Add the new bookmark and clip a job with it: the banner clears without a restart. Run `$l.Stop()`.

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
Automated in `scripts/verify.ps1`: `TestR4_R34_SentinelScan` runs onboarding, a forced Gemini 400, profile, a job, letter generation and approval and form answers with sentinel values, then byte-scans the data folder (no key anywhere; no email, phone or resume line in the logs or in tables that shouldn't hold them). `TestR34_ScanScript*` run `scripts/scan-sentinels.ps1` itself on folders they build.

- [ ] Live Gemini smoke with your own key (it never runs in CI): `$env:LEGAJ_HOME = "$env:TEMP\legaj-live"; $env:LEGAJ_LIVE_GEMINI_KEY = '<key>'; go test -tags live -count=1 ./internal/smoke/`, then `Remove-Item Env:LEGAJ_LIVE_GEMINI_KEY`.
- [ ] On a clean Windows 10 VM: install, finish onboarding with **made-up** sentinel values (a throwaway key that you revoke afterwards, a fake email, phone and resume line), paste and clip jobs, generate and approve a letter, draft answers, open a follow-up, add a round. Close LeGaJ, then run `powershell -NoProfile -File scripts/scan-sentinels.ps1 -Key ... -Email ... -Phone ... -ResumeLine ...`. It must report nothing, including in the WebView2 folder. The script allows the email and phone in WebView2's autofill store (`Web Data`), which keeps what you type into LeGaJ's forms and can't be turned off; the key may be nowhere.
- [ ] Generate 10 letters while another app has focus: LeGaJ never steals focus and no console window flashes (R6).
- [ ] Clip on the 6 sites (LinkedIn logged in, Indeed, Greenhouse, Lever, Ashby, Workday) **in Edge**, Windows' default browser. The spike ran these sites in Firefox and Chrome only, and Edge has so far run the clipper headlessly on local pages, not from a real bookmark on a real site. Note any blocked popup or "connect to devices on your local network" prompt.
