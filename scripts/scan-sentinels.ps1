#Requires -Version 5.1
<#
.SYNOPSIS
  Release check (R4, R34): after a real end-to-end run with made-up
  sentinel values, look for them on disk.

.DESCRIPTION
  Use made-up values only, never your real key or details. Steps:
    1. Close LeGaJ (WebView2 locks its files while the app runs).
    2. Run this script with the sentinel key, email, phone and resume line
       you used during the run.

  The key must appear in no file under the data folder or the WebView2
  folder. The email, phone and resume line must not appear in the logs or
  the WebView2 folder (the database and your documents hold them on
  purpose). Text is searched as UTF-8 and UTF-16.

.EXAMPLE
  powershell -NoProfile -File scripts/scan-sentinels.ps1 -Key 'AIzaSyD-SENTINEL-...' `
    -Email 'sentinel.person@example.org' -Phone '+48 601 234 567' -ResumeLine 'SENTINEL-RESUME-LINE'
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$Key,
    [Parameter(Mandatory)][string]$Email,
    [Parameter(Mandatory)][string]$Phone,
    [Parameter(Mandatory)][string]$ResumeLine,
    [string]$DataDir = (Join-Path $env:LOCALAPPDATA 'LeGaJ'),
    [string]$WebViewDir = (Join-Path $env:APPDATA 'legaj.exe')
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if (Get-Process -Name 'legaj' -ErrorAction SilentlyContinue) {
    Write-Host 'Close LeGaJ first: WebView2 keeps its files locked while it runs.' -ForegroundColor Red
    exit 2
}

# Bytes are read as Latin-1, which maps each byte to one character, so a
# byte sequence can be found with an ordinary string search.
$latin1 = [Text.Encoding]::GetEncoding(28591)
function Get-Needles([string]$Text) {
    @(
        $latin1.GetString([Text.Encoding]::UTF8.GetBytes($Text)),
        $latin1.GetString([Text.Encoding]::Unicode.GetBytes($Text))
    )
}

function Find-In([string]$Root, [hashtable]$Needles) {
    $hits = @()
    if (-not (Test-Path $Root)) {
        Write-Host "  (not found: $Root)" -ForegroundColor Yellow
        return $hits
    }
    foreach ($file in Get-ChildItem -LiteralPath $Root -Recurse -File -Force -ErrorAction SilentlyContinue) {
        try {
            $text = $latin1.GetString([IO.File]::ReadAllBytes($file.FullName))
        } catch {
            Write-Host "  skipped (unreadable): $($file.FullName)" -ForegroundColor Yellow
            continue
        }
        foreach ($name in $Needles.Keys) {
            foreach ($n in $Needles[$name]) {
                if ($text.Contains($n)) {
                    $hits += [pscustomobject]@{ What = $name; File = $file.FullName }
                    break
                }
            }
        }
    }
    return $hits
}

# PowerShell names ignore case: a variable called $key would be the [string]
# -Key parameter itself, and a table assigned to it would turn into the text
# "System.Collections.Hashtable".
$keyNeedles = @{ 'API key' = (Get-Needles $Key) }
$allNeedles = @{
    'API key'     = (Get-Needles $Key)
    'email'       = (Get-Needles $Email)
    'phone'       = (Get-Needles $Phone)
    'resume line' = (Get-Needles $ResumeLine)
}

Write-Host "Data folder (key only): $DataDir"
$hits = @(Find-In $DataDir $keyNeedles)
Write-Host "Logs (key, email, phone, resume line): $(Join-Path $DataDir 'logs')"
$hits += @(Find-In (Join-Path $DataDir 'logs') $allNeedles)
Write-Host "WebView2 folder (key, email, phone, resume line): $WebViewDir"
$hits += @(Find-In $WebViewDir $allNeedles)

if ($hits.Count -gt 0) {
    Write-Host ''
    Write-Host 'FOUND sentinel values:' -ForegroundColor Red
    # One line per hit with the whole path: a table cuts long paths short.
    $hits | Sort-Object What, File -Unique | ForEach-Object { Write-Host "  $($_.What): $($_.File)" }
    exit 1
}
Write-Host 'scan-sentinels: nothing found' -ForegroundColor Green
exit 0
