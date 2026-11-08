#Requires -Version 5.1
<#
.SYNOPSIS
  The release sweep on a disposable Windows runner (docs/RELEASE_CHECKLIST.md).

.DESCRIPTION
  Prepares the machine, then runs drive.mjs, which installs LeGaJ and drives
  it. Never run this on a real PC: it edits the hosts file, trusts a
  throwaway root certificate, sets Edge policies and file associations,
  and creates a local user.

  Phases:
    main         install, onboarding, every feature, opening files and links,
                 focus, single instance, clipper on local and real sites,
                 sentinel scan, installer prompts, both uninstall answers
    standard     install as a standard (non-admin) user
    nowebview2   remove WebView2, then install: the installer must add it
#>
[CmdletBinding()]
param([ValidateSet('main', 'standard', 'nowebview2')][string]$Phase = 'main')
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$repo = $PWD.Path
$out = Join-Path $repo 'out'
$work = Join-Path $env:RUNNER_TEMP 'sweep'
New-Item -ItemType Directory -Force $out, $work | Out-Null

function Rand([int]$n) {
    $chars = [char[]]'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789'
    -join (1..$n | ForEach-Object { $chars | Get-Random })
}

# A one-page PDF with the given lines, so the resume can be opened in Edge.
function New-Pdf([string]$Path, [string[]]$Lines) {
    $text = ($Lines | ForEach-Object { '(' + ($_ -replace '([\\()])', '\$1') + ') Tj 0 -18 Td' }) -join ' '
    $stream = "BT /F1 12 Tf 72 720 Td $text ET"
    $objs = @(
        '<< /Type /Catalog /Pages 2 0 R >>',
        '<< /Type /Pages /Kids [3 0 R] /Count 1 >>',
        '<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>',
        "<< /Length $($stream.Length) >>`nstream`n$stream`nendstream",
        '<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>'
    )
    $sb = New-Object Text.StringBuilder
    [void]$sb.Append("%PDF-1.4`n")
    $offsets = @()
    for ($i = 0; $i -lt $objs.Count; $i++) {
        $offsets += $sb.Length
        [void]$sb.Append("$($i + 1) 0 obj`n$($objs[$i])`nendobj`n")
    }
    $xref = $sb.Length
    [void]$sb.Append("xref`n0 $($objs.Count + 1)`n0000000000 65535 f `n")
    foreach ($o in $offsets) { [void]$sb.Append(('{0:D10} 00000 n `n' -f $o)) }
    [void]$sb.Append("trailer`n<< /Size $($objs.Count + 1) /Root 1 0 R >>`nstartxref`n$xref`n%%EOF`n")
    [IO.File]::WriteAllText($Path, $sb.ToString(), [Text.Encoding]::ASCII)
}

$setup = (Get-ChildItem (Join-Path $repo 'dist') -Filter 'LeGaJ-Setup-*.exe' | Select-Object -First 1).FullName
$cfg = [ordered]@{
    phase    = $Phase
    repo     = $repo
    out      = $out
    work     = $work
    setup    = $setup
    exe      = (Join-Path $env:LOCALAPPDATA 'Programs\LeGaJ\legaj.exe')
    edgePort = 9444
}

if ($Phase -eq 'main') {
    # Made-up values only. The scan at the end looks for each of them.
    $sentinel = [ordered]@{
        key   = "AIzaSyD-SENTINEL-$(Rand 22)"
        email = "sentinel.$((Rand 6).ToLower())@example.org"
        phone = "+48 601 $(Get-Random -Minimum 100 -Maximum 999) $(Get-Random -Minimum 100 -Maximum 999)"
        line  = "SENTINEL-RESUME-LINE-$(Rand 8)"
        name  = 'Sentinel Person'
    }
    $cfg.sentinel = $sentinel
    $cfg.resume1 = Join-Path $work 'sentinel-resume-1.pdf'
    $cfg.resume2 = Join-Path $work 'sentinel-resume-2.pdf'
    New-Pdf $cfg.resume1 @($sentinel.name, $sentinel.email, $sentinel.phone, $sentinel.line, 'Operations Analyst, Sentinel Works')
    New-Pdf $cfg.resume2 @($sentinel.name, $sentinel.email, $sentinel.phone, "$($sentinel.line) (second version)")

    # The stand-in Gemini: the hosts file sends the API's name here, and a
    # throwaway root certificate makes the app trust it.
    $pw = Rand 24
    $cert = New-SelfSignedCertificate -DnsName 'generativelanguage.googleapis.com' -CertStoreLocation 'Cert:\LocalMachine\My' -KeyExportPolicy Exportable -NotAfter (Get-Date).AddDays(1)
    $pfx = Join-Path $work 'gemini.pfx'
    Export-PfxCertificate -Cert $cert -FilePath $pfx -Password (ConvertTo-SecureString $pw -AsPlainText -Force) -CryptoAlgorithmOption AES256_SHA256 | Out-Null
    $cer = Join-Path $work 'gemini.cer'
    Export-Certificate -Cert $cert -FilePath $cer | Out-Null
    Import-Certificate -FilePath $cer -CertStoreLocation 'Cert:\LocalMachine\Root' | Out-Null
    Add-Content -Path "$env:WINDIR\System32\drivers\etc\hosts" -Value "`r`n127.0.0.1 generativelanguage.googleapis.com" -Encoding ascii
    & ipconfig /flushdns | Out-Null
    $fake = [ordered]@{ pfx = $pfx; passphrase = $pw; key = $sentinel.key; log = (Join-Path $out 'gemini-requests.jsonl'); profile = $sentinel }
    $fakeCfg = Join-Path $work 'fake-gemini.json'
    $fake | ConvertTo-Json | Set-Content -LiteralPath $fakeCfg -Encoding utf8
    Start-Process node -ArgumentList "`"$repo\scripts\sweep\fake-gemini.mjs`"", "`"$fakeCfg`"" -WindowStyle Hidden `
        -RedirectStandardOutput (Join-Path $out 'fake-gemini.out.txt') -RedirectStandardError (Join-Path $out 'fake-gemini.err.txt') | Out-Null
    Start-Sleep -Seconds 2
    try {
        Invoke-WebRequest -UseBasicParsing -Method Post -Uri 'https://generativelanguage.googleapis.com/v1beta/models/x:generateContent' -Body '{}' | Out-Null
        throw 'the stand-in Gemini accepted a call with no key'
    } catch [System.Net.WebException] {
        $code = [int]$_.Exception.Response.StatusCode
        if ($code -ne 400) { throw "the stand-in Gemini isn't reachable over trusted TLS: $($_.Exception.Message)" }
        Write-Host 'stand-in Gemini: reachable over trusted TLS'
    }
    # That check isn't one of LeGaJ's calls, which the log is for.
    Remove-Item -LiteralPath $fake.log -ErrorAction SilentlyContinue

    # A real Windows 10 PC opens PDFs in Edge; the runner has no handler.
    if (Test-Path 'Registry::HKEY_CLASSES_ROOT\MSEdgePDF') {
        New-Item -Path 'HKCU:\Software\Classes\.pdf' -Force | Out-Null
        Set-ItemProperty -Path 'HKCU:\Software\Classes\.pdf' -Name '(default)' -Value 'MSEdgePDF'
    }
}

# Edge without its first-run pages and with the favorites bar showing, so
# the sweep can drop the clipper on it and click it.
$edge = 'HKLM:\SOFTWARE\Policies\Microsoft\Edge'
New-Item -Path $edge -Force | Out-Null
foreach ($p in @{
        HideFirstRunExperience = 1; FavoritesBarEnabled = 1; BrowserSignin = 0; SyncDisabled = 1
        DefaultBrowserSettingEnabled = 0; StartupBoostEnabled = 0; BackgroundModeEnabled = 0
        PromotionalTabsEnabled = 0; ShowRecommendationsEnabled = 0; AutoImportAtFirstRun = 4
    }.GetEnumerator()) {
    New-ItemProperty -Path $edge -Name $p.Key -Value $p.Value -PropertyType DWord -Force | Out-Null
}

$cfgPath = Join-Path $work 'drive.json'
$cfg | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $cfgPath -Encoding utf8
$started = Get-Date
& node (Join-Path $repo 'scripts\sweep\drive.mjs') $cfgPath
$code = $LASTEXITCODE
# Crashes and hangs Windows recorded during the run, for the artifact.
Get-WinEvent -FilterHashtable @{ LogName = 'Application'; StartTime = $started } -ErrorAction SilentlyContinue |
    Where-Object { $_.ProviderName -in 'Application Error', 'Application Hang', 'Windows Error Reporting' } |
    ForEach-Object { "$($_.TimeCreated.ToString('s')) $($_.ProviderName): $(($_.Message -split "`n")[0..8] -join ' / ')" } |
    Set-Content -LiteralPath (Join-Path $out 'windows-events.txt') -Encoding utf8
$summary = Join-Path $out 'summary.md'
if ($env:GITHUB_STEP_SUMMARY -and (Test-Path $summary)) { Get-Content -LiteralPath $summary -Raw | Add-Content -LiteralPath $env:GITHUB_STEP_SUMMARY -Encoding utf8 }
exit $code
