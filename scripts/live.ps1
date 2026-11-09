# Runs the live Gemini suite (R69) against the real API with your own key.
#
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts\live.ps1
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts\live.ps1 -Run 'Diagnose|ResumeRead' -Model gemini-3.1-flash-lite
#
# The key is asked for with hidden input, so it never lands in PowerShell's
# history file. It lives only in this process's environment for the run and
# is removed afterwards. The suite uses a scratch LEGAJ_HOME in %TEMP%, never
# LeGaJ's real data folder, and sends only synthetic data. It never runs in CI.
param(
    [string]$Run = '',
    [string]$Model = ''
)

$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
$go = 'C:\Program Files\Go\bin\go.exe'

$secure = Read-Host -AsSecureString 'Gemini API key (input hidden)'
$bstr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
try {
    $env:LEGAJ_LIVE_GEMINI_KEY = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($bstr)
} finally {
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr)
}
if (-not $env:LEGAJ_LIVE_GEMINI_KEY) {
    Write-Host 'No key entered; nothing was sent.' -ForegroundColor Yellow
    exit 1
}

$scratch = Join-Path $env:TEMP ('legaj-live-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $scratch | Out-Null
$env:LEGAJ_HOME = $scratch
if ($Model) { $env:LEGAJ_LIVE_GEMINI_MODEL = $Model }

$testArgs = @('test', '-tags', 'live', '-count=1', '-v')
if ($Run) { $testArgs += @('-run', $Run) }
$testArgs += './internal/live/...'

$code = 1
Push-Location $repo
try {
    & $go @testArgs
    $code = $LASTEXITCODE
} finally {
    Pop-Location
    Remove-Item Env:LEGAJ_LIVE_GEMINI_KEY -ErrorAction SilentlyContinue
    Remove-Item Env:LEGAJ_LIVE_GEMINI_MODEL -ErrorAction SilentlyContinue
    Remove-Item -Recurse -Force $scratch -ErrorAction SilentlyContinue
}
exit $code
