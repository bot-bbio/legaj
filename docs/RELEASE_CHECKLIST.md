# LeGaJ release checklist (manual)

Run these on a clean Windows 10 or 11 machine with the built installer before a release. Automated checks live in `scripts/verify.ps1`. This list covers what a headless test can't see.

## Gemini key storage (R4)
- [ ] Run the real Credential Manager round trip: `$env:LEGAJ_HOME = "$env:TEMP\legaj-smoke"; go test -tags credman ./internal/secrets/`. It passes and leaves no `LeGaJ-test` entry in Credential Manager.
- [ ] After saving a key in the app, Credential Manager › Windows Credentials shows **LeGaJ:gemini-api-key**.

## Single instance (R42)
- [ ] Launch LeGaJ, then minimize it. Launch it again from the Start menu: no second window appears, and the first window is restored and focused.
- [ ] Launch LeGaJ twice in quick succession, as with a double-click on the shortcut: only one window ever appears.
