# Packaging & Distribution

LeGaJ ships as a **Go + Fyne desktop GUI** that calls a set of **Python helpers**
(`pypdf`, `python-docx`, `reportlab`, …) for resume parsing and PDF generation.
To make installation painless for non-technical users, we use **Option B**:

> Freeze the Python helpers into a single self-contained executable
> (`legaj-tools`) with **PyInstaller**, so the end user needs **no Python install
> and no `pip`**. The Go GUI calls that frozen binary; if it is absent (developer
> machines) it transparently falls back to running `scripts/*.py` with a system
> Python.

The fallback lives in [`exec.go`](exec.go) (`toolsBinaryPath` / `runPythonScript`).

```
GUI (legaj.exe / LeGaJ.app)
   └─ calls ─> tools/legaj-tools[.exe]  <-- PyInstaller bundle (no Python needed)
                  └─ dispatches ─> parse_resume / generate_resume_pdf / ...
```

---

## Repository artifacts

| File | Purpose |
| :--- | :--- |
| `scripts/legaj_tools.py` | Dispatcher: `legaj-tools <tool> [args]` runs the matching script's `__main__`. |
| `legaj-tools.spec` | PyInstaller spec that freezes the dispatcher + all tools into one binary. |
| `FyneApp.toml` | App metadata (name, ID, version, icon) used by `fyne package`. |
| `build_windows.ps1` | End-to-end Windows build → Inno Setup installer. |
| `build_macos.sh` | End-to-end macOS build → `.app` (and optional `.dmg`). Run **on a Mac**. |
| `packaging/legaj.iss` | Inno Setup installer definition (per-user, no admin). |

> Add an `Icon.png` (512×512 recommended) to the project root before building to
> get a branded icon; both build scripts fall back gracefully if it is missing.

---

## Prerequisites

- **Python 3.12+** and **Go 1.25+**
- **PyInstaller** and the **fyne CLI** — the build scripts install these for you
- **Windows only:** [Inno Setup 6](https://jrsoftware.org/isdl.php) (provides `ISCC.exe`)
- **macOS only:** Xcode command-line tools (for cgo); build must run **on a Mac**

---

## Building on Windows

```powershell
# from the project root
./build_windows.ps1
```

This will:
1. Install Python deps + PyInstaller, then freeze → `dist\legaj-tools.exe`.
2. Build the GUI with `fyne package` → `legaj.exe`.
3. Stage `legaj.exe` + `tools\legaj-tools.exe` into `packaging\staging\`.
4. Compile `packaging\legaj.iss` → `dist\LeGaJ-Setup-<version>.exe`.

The installer is **per-user** (`%LOCALAPPDATA%\LeGaJ`, no admin prompt). That
location is writable, which matters because the app creates `references\`,
`extension\`, and `outputs\` next to itself and the Start Menu shortcut sets the
working directory to the install folder.

Hand the friend a single file: **`LeGaJ-Setup-<version>.exe`**.

---

## Building on macOS

```bash
# from the project root, ON A MAC
./build_macos.sh
```

This produces `LeGaJ.app` (and `dist/LeGaJ.dmg` if `hdiutil` is available), with
the frozen `legaj-tools` embedded at `LeGaJ.app/Contents/MacOS/tools/`.

> **You cannot cross-compile the macOS GUI from Windows** — Fyne uses cgo. Run
> the macOS build on a Mac, and run PyInstaller on that same Mac so the frozen
> binary matches the CPU architecture (Apple Silicon vs Intel).

### Data location & the working directory

The app references its data folders (`references/`, `extension/`, `outputs/`)
with relative paths. To keep those writes working regardless of how the program
is launched, [`apphome.go`](apphome.go) pins the working directory at startup via
`resolveAppHome()`:

| Context | Working directory used |
| :--- | :--- |
| Source checkout (`go.mod` present) | unchanged — **identical to the original build** |
| `$LEGAJ_HOME` set | that path (explicit override) |
| Packaged **Windows** | `%LOCALAPPDATA%\LeGaJ` (same as the install dir) |
| Packaged **macOS** | `~/Library/Application Support/LeGaJ` |
| Packaged **Linux** | `$XDG_DATA_HOME/LeGaJ` or `~/.local/share/LeGaJ` |

This resolves the macOS Finder/Gatekeeper problem (a `.app` starts at `/` and its
bundle is read-only) by writing data to a per-user, writable location. The frozen
`tools/legaj-tools` binary is located relative to the executable, so it keeps
working inside the read-only bundle. Development and Windows behavior are
unchanged — the switch is a no-op in a source checkout and lands in the install
directory on Windows.

---

## Code signing with a self-signed certificate

Self-signing removes the "unidentified developer" hard-block but does **not** give
you full reputation — users still see a one-time prompt. For trusted distribution
you eventually want a real certificate (Authenticode / Apple Developer ID), but
self-signing is fine for friends stress-testing.

### Windows (Authenticode, PowerShell + signtool)

```powershell
# 1. Create a self-signed code-signing certificate (valid 3 years)
$cert = New-SelfSignedCertificate `
    -Type CodeSigningCert `
    -Subject "CN=Roberto Montero (LeGaJ)" `
    -KeyUsage DigitalSignature `
    -CertStoreLocation Cert:\CurrentUser\My `
    -NotAfter (Get-Date).AddYears(3)

# 2. Export it to a password-protected .pfx (keep this file private)
$pwd = ConvertTo-SecureString -String "choose-a-strong-password" -Force -AsPlainText
Export-PfxCertificate -Cert $cert -FilePath legaj-codesign.pfx -Password $pwd

# 3. Sign the installer (and/or legaj.exe) with a timestamp.
#    signtool.exe ships with the Windows SDK; adjust the path to your version.
$signtool = "C:\Program Files (x86)\Windows Kits\10\bin\10.0.22621.0\x64\signtool.exe"
& $signtool sign /f legaj-codesign.pfx /p "choose-a-strong-password" `
    /fd SHA256 /tr http://timestamp.digicert.com /td SHA256 `
    "dist\LeGaJ-Setup-1.0.0.exe"

# 4. Verify
& $signtool verify /pa "dist\LeGaJ-Setup-1.0.0.exe"
```

For the friend to see a *trusted* publisher (no warning), they would import the
public cert into **Trusted Root Certification Authorities** and **Trusted
Publishers** — only do this on machines you control:

```powershell
Export-Certificate -Cert $cert -FilePath legaj-codesign.cer   # public part only
# On the target machine (per-user, no admin):
Import-Certificate -FilePath legaj-codesign.cer -CertStoreLocation Cert:\CurrentUser\Root
Import-Certificate -FilePath legaj-codesign.cer -CertStoreLocation Cert:\CurrentUser\TrustedPublisher
```

Otherwise, tell them: **"More info → Run anyway"** on the SmartScreen prompt.

> Never commit `legaj-codesign.pfx`, the `.cer`, or the password. Add `*.pfx` and
> `*.cer` to `.gitignore`.

### macOS (codesign with a self-signed identity)

1. **Create the identity:** Keychain Access → *Certificate Assistant* →
   *Create a Certificate…* → Name `LeGaJ Self-Signed`, Identity Type
   *Self Signed Root*, Certificate Type **Code Signing** → Create.
2. **Sign the bundle:**

   ```bash
   codesign --deep --force --options runtime \
       --sign "LeGaJ Self-Signed" LeGaJ.app
   codesign --verify --deep --strict --verbose=2 LeGaJ.app
   ```

3. Self-signed apps are **not notarized**, so Gatekeeper still quarantines
   downloads. The friend opens it once via **right-click → Open → Open**, or you
   strip the quarantine flag before sending:

   ```bash
   xattr -dr com.apple.quarantine LeGaJ.app
   ```

For frictionless macOS distribution you need an Apple Developer ID certificate
($99/yr) plus notarization (`xcrun notarytool`). That is out of scope for
self-signed testing builds.

---

## Security checklist before shipping a build

- [ ] Bundle contains **no** `.env`, `references/` PII, or `*.clip_token`.
- [ ] `legaj-codesign.pfx` / `.cer` / passwords are **not** in the repo.
- [ ] Installer is per-user (no unnecessary admin elevation).
- [ ] The clip server still binds localhost-only with its session token.
