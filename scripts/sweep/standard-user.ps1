#Requires -Version 5.1
# Release sweep (R38): installs LeGaJ as a new standard (non-admin) user on a
# disposable runner. If Setup asked for elevation, Windows would refuse to
# start it for this user, so a finished install shows no UAC prompt is needed.
[CmdletBinding()]
param([Parameter(Mandatory)][string]$Setup, [Parameter(Mandatory)][string]$Out)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# What Setup's manifest asks Windows for.
$bytes = [IO.File]::ReadAllBytes($Setup)
$text = [Text.Encoding]::ASCII.GetString($bytes)
$m = [regex]::Match($text, 'requestedExecutionLevel\s+level="([^"]+)"')
"manifest: requestedExecutionLevel=$(if ($m.Success) { $m.Groups[1].Value } else { '(none found)' })"

$name = 'sweepuser'
$chars = [char[]]'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789'
$pw = (-join (1..20 | ForEach-Object { $chars | Get-Random })) + '!a1Z'
$secure = ConvertTo-SecureString $pw -AsPlainText -Force
New-LocalUser -Name $name -Password $secure -PasswordNeverExpires -AccountNeverExpires | Out-Null
Add-LocalGroupMember -Group 'Users' -Member $name -ErrorAction SilentlyContinue
if (Get-LocalGroupMember -Group 'Administrators' | Where-Object { $_.Name -like "*\$name" }) { throw 'the test user is an administrator' }
"OK $name is a standard user (not in Administrators)"

$dir = 'C:\sweep-standard'
New-Item -ItemType Directory -Force $dir | Out-Null
Copy-Item -LiteralPath $Setup -Destination $dir
& icacls.exe $dir /grant "${name}:(OI)(CI)M" | Out-Null
$exe = Join-Path $dir (Split-Path $Setup -Leaf)
$log = Join-Path $dir 'install.log'

$cred = New-Object System.Management.Automation.PSCredential(".\$name", $secure)
try {
    $p = Start-Process -FilePath $exe -ArgumentList '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', "/LOG=$log" `
        -Credential $cred -LoadUserProfile -WorkingDirectory $dir -PassThru -Wait
} catch {
    throw "Windows refused to start Setup for a standard user: $($_.Exception.Message)"
}
"Setup exit code: $($p.ExitCode)"
if (Test-Path $log) { Copy-Item $log (Join-Path $Out 'install-standard-user.log') }
$logText = if (Test-Path $log) { Get-Content -LiteralPath $log -Raw } else { '' }

$userDir = (Get-CimInstance Win32_UserProfile | Where-Object { $_.LocalPath -like "*\$name*" } | Select-Object -First 1).LocalPath
if (-not $userDir) { throw 'no profile folder for the test user' }
$installed = Join-Path $userDir 'AppData\Local\Programs\LeGaJ\legaj.exe'
$menu = Join-Path $userDir 'AppData\Roaming\Microsoft\Windows\Start Menu\Programs\LeGaJ.lnk'
$problems = @()
if ($p.ExitCode -ne 0) { $problems += "exit code $($p.ExitCode)" }
if (-not (Test-Path $installed)) { $problems += "no $installed" }
if (-not (Test-Path $menu)) { $problems += 'no Start menu entry' }
if ($logText -notmatch 'User privileges: None') { $problems += 'the log does not show a non-admin user' }
if ($logText -notmatch 'Administrative install mode: No') { $problems += 'not a per-user install' }
if (Test-Path (Join-Path $env:LOCALAPPDATA 'Programs\LeGaJ')) { $problems += "it installed into the runner account's folder instead" }
if ($problems) { throw ($problems -join '; ') }
"OK installed to $installed with no elevation"
"OK Start menu entry at $menu"
