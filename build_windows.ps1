<#
.SYNOPSIS
    Builds LeGaJ for Windows: freezes the Python helpers, compiles the Fyne GUI,
    stages both, and produces an Inno Setup installer.

.DESCRIPTION
    Pipeline (Option B distribution — no Python required on the target machine):
      1. PyInstaller freezes scripts/legaj_tools.py -> dist/legaj-tools.exe
      2. fyne packages the Go GUI         -> legaj.exe (with icon + metadata)
      3. Stage legaj.exe + tools/legaj-tools.exe into packaging/staging
      4. ISCC compiles packaging/legaj.iss -> dist/LeGaJ-Setup-<ver>.exe

    Run from the project root in PowerShell:
        ./build_windows.ps1

.NOTES
    Requires: Python 3.12, Go, PyInstaller, the fyne CLI, and Inno Setup (ISCC).
    The script installs PyInstaller and the fyne CLI automatically if missing.
#>

$ErrorActionPreference = "Stop"
$root = $PSScriptRoot
Set-Location $root

# --- Canonical toolchain paths (override via env if your setup differs) ---
$python = $env:LEGAJ_PYTHON
if (-not $python) { $python = "C:\Users\molus\AppData\Local\Programs\Python\Python312\python.exe" }
$go = $env:LEGAJ_GO
if (-not $go) { $go = "C:\Program Files\Go\bin\go.exe" }

if (-not (Test-Path $python)) { throw "Python not found at $python. Set `$env:LEGAJ_PYTHON." }
if (-not (Test-Path $go))     { throw "Go not found at $go. Set `$env:LEGAJ_GO." }

Write-Host "==> Installing Python build/runtime dependencies" -ForegroundColor Cyan
& $python -m pip install --upgrade pip
& $python -m pip install -r requirements.txt
& $python -m pip install pyinstaller

Write-Host "==> Freezing Python helpers with PyInstaller" -ForegroundColor Cyan
& $python -m PyInstaller --noconfirm --clean legaj-tools.spec
if (-not (Test-Path "dist\legaj-tools.exe")) { throw "PyInstaller did not produce dist\legaj-tools.exe" }

Write-Host "==> Building the Fyne GUI" -ForegroundColor Cyan
# fyne package embeds the icon + version metadata defined in FyneApp.toml.
$fyne = (Get-Command fyne -ErrorAction SilentlyContinue)
if (-not $fyne) {
    Write-Host "    fyne CLI not found; installing..." -ForegroundColor Yellow
    & $go install fyne.io/fyne/v2/cmd/fyne@latest
    $gobin = (& $go env GOBIN); if (-not $gobin) { $gobin = Join-Path (& $go env GOPATH) "bin" }
    $env:PATH = "$gobin;$env:PATH"
}

if (Test-Path "Icon.png") {
    & fyne package --os windows --icon Icon.png --release
} else {
    Write-Host "    Icon.png not found — building without an embedded icon." -ForegroundColor Yellow
    & $go build -ldflags="-H windowsgui" -o legaj.exe .
}
if (-not (Test-Path "legaj.exe")) { throw "GUI build did not produce legaj.exe" }

Write-Host "==> Staging application files" -ForegroundColor Cyan
$staging = "packaging\staging"
if (Test-Path $staging) { Remove-Item $staging -Recurse -Force }
New-Item -ItemType Directory -Path "$staging\tools" -Force | Out-Null
Copy-Item "legaj.exe" "$staging\legaj.exe" -Force
Copy-Item "dist\legaj-tools.exe" "$staging\tools\legaj-tools.exe" -Force

Write-Host "==> Compiling the installer with Inno Setup" -ForegroundColor Cyan
$iscc = (Get-Command ISCC.exe -ErrorAction SilentlyContinue)
if (-not $iscc) {
    foreach ($p in @("C:\Program Files (x86)\Inno Setup 6\ISCC.exe", "C:\Program Files\Inno Setup 6\ISCC.exe")) {
        if (Test-Path $p) { $iscc = $p; break }
    }
}
if (-not $iscc) {
    Write-Host "    Inno Setup (ISCC.exe) not found. Install it from https://jrsoftware.org/isdl.php" -ForegroundColor Yellow
    Write-Host "    Staging is ready at $staging — re-run ISCC packaging\legaj.iss once installed." -ForegroundColor Yellow
    exit 0
}
& $iscc "packaging\legaj.iss"

Write-Host "==> Done. Installer is in dist\LeGaJ-Setup-*.exe" -ForegroundColor Green
