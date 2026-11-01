#Requires -Version 5.1
<#
.SYNOPSIS
  LeGaJ verify gate. A change is complete only when this script exits 0.

.DESCRIPTION
  Order matters: Go bindings and the frontend are built first, so the
  //go:embed of frontend/dist and the generated TypeScript bindings exist
  before any Go or TypeScript check runs. CI runs this same script on
  windows-latest from a clean checkout.

.PARAMETER SkipAppBuild
  Skip the final `wails build` (for fast local loops only; CI never skips it).
#>
[CmdletBinding()]
param(
    [switch]$SkipAppBuild
)

# Native tools report failure through exit codes, which every step checks.
# 'Continue' stops PowerShell 5.1 from treating harmless stderr output from
# npm or go as a terminating error.
$ErrorActionPreference = 'Continue'
Set-StrictMode -Version Latest

$root = Split-Path -Parent $PSScriptRoot
$frontend = Join-Path $root 'frontend'
Set-Location $root

function Resolve-Tool {
    param([string]$Name, [string[]]$Fallbacks)
    $cmd = Get-Command $Name -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($cmd) { return $cmd.Source }
    foreach ($candidate in $Fallbacks) {
        if ($candidate -and (Test-Path $candidate)) { return $candidate }
    }
    Write-Host "verify: $Name not found. Install it or add it to PATH." -ForegroundColor Red
    exit 1
}

function Invoke-Step {
    param([string]$Name, [scriptblock]$Body)
    Write-Host "==> $Name" -ForegroundColor Cyan
    $sw = [Diagnostics.Stopwatch]::StartNew()
    $global:LASTEXITCODE = 0
    & $Body
    if ($LASTEXITCODE -ne 0) {
        Write-Host ("FAIL {0} (exit {1})" -f $Name, $LASTEXITCODE) -ForegroundColor Red
        exit 1
    }
    Write-Host ("ok   {0} ({1:N1}s)" -f $Name, $sw.Elapsed.TotalSeconds) -ForegroundColor Green
}

function Invoke-InFrontend {
    param([scriptblock]$Body)
    Push-Location $frontend
    try { & $Body } finally { Pop-Location }
}

# --- Toolchain -------------------------------------------------------------
$go = Resolve-Tool 'go' @('C:\Program Files\Go\bin\go.exe')
$npm = Resolve-Tool 'npm.cmd' @('C:\Program Files\nodejs\npm.cmd')
$gopath = (& $go env GOPATH).Trim()
$wails = Resolve-Tool 'wails' @((Join-Path $gopath 'bin\wails.exe'))
# Wails shells out to go and npm itself, so both must be on PATH.
$env:PATH = "$(Split-Path $go);$(Split-Path $npm);$env:PATH"
# -race needs cgo (and therefore gcc).
$env:CGO_ENABLED = '1'
$goroot = (& $go env GOROOT).Trim()
$gofmt = Join-Path $goroot 'bin\gofmt.exe'

# --- Generate + build inputs -----------------------------------------------
Invoke-Step 'wails bindings' { & $wails generate module }
Invoke-Step 'frontend install' { Invoke-InFrontend { & $npm ci --no-audit --no-fund } }
Invoke-Step 'frontend build' { Invoke-InFrontend { & $npm run build } }

# --- Go --------------------------------------------------------------------
Invoke-Step 'go mod tidy' { & $go mod tidy -diff }
Invoke-Step 'gofmt' {
    $dirs = & $go list -f '{{.Dir}}' ./...
    if ($LASTEXITCODE -ne 0) { return }
    $files = foreach ($d in $dirs) { Get-ChildItem -LiteralPath $d -Filter *.go -File | ForEach-Object FullName }
    $unformatted = & $gofmt -l @files
    if ($unformatted) {
        Write-Host 'Not gofmt-formatted:' -ForegroundColor Red
        $unformatted | ForEach-Object { Write-Host "  $_" }
        $global:LASTEXITCODE = 1
    }
}
Invoke-Step 'go vet' { & $go vet ./... }
Invoke-Step 'gcc (for -race)' {
    if (-not (Get-Command gcc -ErrorAction SilentlyContinue)) {
        Write-Host 'gcc not on PATH: go test -race needs cgo. Install MinGW-w64.' -ForegroundColor Red
        $global:LASTEXITCODE = 1
    }
}
Invoke-Step 'go test -race' { & $go test -race -count=1 ./... }

# --- Frontend --------------------------------------------------------------
Invoke-Step 'svelte-check' { Invoke-InFrontend { & $npm run check } }
Invoke-Step 'eslint' { Invoke-InFrontend { & $npm run lint } }
Invoke-Step 'vitest' { Invoke-InFrontend { & $npm run test } }

# --- App -------------------------------------------------------------------
if ($SkipAppBuild) {
    Write-Host 'skip wails build (-SkipAppBuild)' -ForegroundColor Yellow
} else {
    # Frontend and bindings were built above: -s skips the frontend build,
    # -skipbindings the regeneration, -m the mod tidy (checked above).
    Invoke-Step 'wails build' { & $wails build -s -m -skipbindings -trimpath }
}

Write-Host 'verify: all green' -ForegroundColor Green
exit 0
