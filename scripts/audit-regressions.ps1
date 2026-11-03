#Requires -Version 5.1
<#
.SYNOPSIS
  Release check: every V1 bug (U/A item) in the PRD's regression map has
  a named test for each requirement it maps to.

.DESCRIPTION
  Reads the "V1 bug -> requirement map" line from .agent/PRD.md (local
  only, so this runs on a developer machine, not in CI) and, for each
  requirement, looks for a Go test named TestR<n>_... or a frontend test
  that names R<n>. Lists every gap and exits 1 if there is one.
#>
[CmdletBinding()]
param([string]$Prd = '')
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
# $PSScriptRoot is empty in parameter defaults on Windows PowerShell 5.1.
$root = Split-Path -Parent $PSScriptRoot
if (-not $Prd) { $Prd = Join-Path $root '.agent\PRD.md' }

if (-not (Test-Path $Prd)) {
    Write-Host "No PRD at $Prd; nothing to audit." -ForegroundColor Yellow
    exit 2
}
$line = Get-Content $Prd -Encoding UTF8 | Where-Object { $_ -match 'requirement map' } | Select-Object -First 1
if (-not $line) { throw 'The PRD has no regression map line.' }

# Test names and descriptions, once.
$goTests = Get-ChildItem -Path $root -Recurse -Filter '*_test.go' -File |
    Where-Object { $_.FullName -notmatch '\\(node_modules|spike)\\' } |
    Select-String -Pattern '^func (Test\w+)\(' | ForEach-Object { $_.Matches[0].Groups[1].Value }
$tsText = (Get-ChildItem -Path (Join-Path $root 'frontend\src') -Recurse -Include '*.test.ts' -File |
    Get-Content -Raw -Encoding UTF8) -join "`n"

function Test-Covered([string]$Req) {
    if ($goTests | Where-Object { $_ -like "Test${Req}_*" -or $_ -like "Test*_${Req}_*" }) { return $true }
    return $tsText -match "\b$Req\b"
}

$gaps = @()
$rows = @()
# \u2192 is the arrow, \u2013 the en dash and \u00B7 the separator dot: the
# script stays ASCII because Windows PowerShell reads it as ANSI.
foreach ($m in [regex]::Matches($line, '([UA]\d+(?:[\u2013-][AU]?\d+)?)\s*\u2192\s*([^\u00B7]+)')) {
    $item = $m.Groups[1].Value.Trim()
    $target = $m.Groups[2].Value.Trim()
    if ($target -match 'parked') {
        $rows += [pscustomobject]@{ Item = $item; Requirement = $target; Status = 'parked' }
        continue
    }
    foreach ($r in [regex]::Matches($target, 'R\d+')) {
        $ok = Test-Covered $r.Value
        $rows += [pscustomobject]@{ Item = $item; Requirement = $r.Value; Status = $(if ($ok) { 'tested' } else { 'GAP' }) }
        if (-not $ok) { $gaps += "$item -> $($r.Value)" }
    }
}
$rows | Format-Table -AutoSize | Out-String | Write-Host
if ($gaps.Count -gt 0) {
    Write-Host ("Gaps: " + ($gaps -join ', ')) -ForegroundColor Red
    exit 1
}
Write-Host 'audit-regressions: every mapped requirement has a test' -ForegroundColor Green
exit 0
