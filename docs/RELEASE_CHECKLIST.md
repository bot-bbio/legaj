# LeGaJ release checklist (manual)

Run these on a clean Windows 10 or 11 machine with the built installer before a release. Automated checks live in `scripts/verify.ps1`. This list covers what a headless test can't see.

## Single instance (R42)
- [ ] Launch LeGaJ, then minimize it. Launch it again from the Start menu: no second window appears, and the first window is restored and focused.
- [ ] Launch LeGaJ twice in quick succession, as with a double-click on the shortcut: only one window ever appears.
