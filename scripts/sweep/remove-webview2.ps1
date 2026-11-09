#Requires -Version 5.1
# Release sweep (R38): removes the WebView2 Runtime from a disposable runner,
# so the installer's WebView2 step can be tested. Exits 1 if it stays.
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$client = 'Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}'
function Get-WebView2Versions {
    foreach ($k in "HKLM:\SOFTWARE\WOW6432Node\$client", "HKLM:\SOFTWARE\$client", "HKCU:\Software\$client") {
        $v = Get-ItemProperty -LiteralPath $k -Name pv -ErrorAction SilentlyContinue
        if ($v -and $v.pv -and $v.pv -ne '0.0.0.0') { "$k=$($v.pv)" }
    }
}

"before: $((Get-WebView2Versions) -join ', ')"
$root = "${env:ProgramFiles(x86)}\Microsoft\EdgeWebView\Application"
$latest = Get-ChildItem -LiteralPath $root -Directory | Where-Object { $_.Name -match '^\d+(\.\d+){3}$' } |
    Sort-Object { [version]$_.Name } -Descending | Select-Object -First 1
if (-not $latest) { 'no WebView2 folder'; exit 1 }
$setup = Join-Path $latest.FullName 'Installer\setup.exe'
"uninstalling with $setup"
$p = Start-Process -FilePath $setup -ArgumentList '--uninstall', '--msedgewebview', '--system-level', '--verbose-logging', '--force-uninstall' -Wait -PassThru
"setup exit code: $($p.ExitCode)"
Start-Sleep -Seconds 5
$left = @(Get-WebView2Versions)
"after: $($left -join ', ')"
"files left: $(Test-Path (Join-Path $latest.FullName 'msedgewebview2.exe'))"
if ($left.Count -gt 0) { exit 1 }
exit 0
