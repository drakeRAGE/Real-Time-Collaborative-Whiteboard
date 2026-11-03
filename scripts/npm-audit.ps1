# Frontend npm audit gate used by `task security-npm`.
# Runs `npm audit --audit-level=high`. Advisories listed in
# scripts/npm-audit-exceptions.json are the only highs that may remain,
# and only when clearing them requires `npm audit fix --force`.
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
$frontend = Join-Path $root 'frontend'
$exceptPath = Join-Path $PSScriptRoot 'npm-audit-exceptions.json'

$allowed = @{}
if (Test-Path $exceptPath) {
    $doc = Get-Content -Raw -Path $exceptPath | ConvertFrom-Json
    foreach ($row in @($doc.advisories)) {
        if ($row.id) { $allowed[$row.id] = $true }
    }
}

Push-Location $frontend
try {
    $raw = & npm audit --audit-level=high --json
    $code = $LASTEXITCODE
} finally {
    Pop-Location
}

if ($code -eq 0) {
    Write-Host 'npm audit --audit-level=high: clean'
    exit 0
}

$parsed = $raw | ConvertFrom-Json
$open = New-Object System.Collections.Generic.List[string]
$vulns = $parsed.vulnerabilities
if ($null -eq $vulns) {
    Write-Host 'npm audit failed and returned no vulnerability list'
    exit $code
}

foreach ($prop in $vulns.PSObject.Properties) {
    $vuln = $prop.Value
    $sev = [string]$vuln.severity
    if ($sev -ne 'high' -and $sev -ne 'critical') { continue }
    $ids = New-Object System.Collections.Generic.List[string]
    foreach ($via in @($vuln.via)) {
        if ($via -is [string]) { continue }
        $url = [string]$via.url
        if ($url -match 'GHSA-[0-9a-z]{4}-[0-9a-z]{4}-[0-9a-z]{4}') {
            $ids.Add($Matches[0])
        }
    }
    if ($ids.Count -eq 0) {
        $open.Add(("{0} ({1}) has no advisory id" -f $prop.Name, $sev))
        continue
    }
    foreach ($id in $ids) {
        if (-not $allowed.ContainsKey($id)) {
            $open.Add(("{0} {1} ({2})" -f $prop.Name, $id, $sev))
        }
    }
}

if ($open.Count -gt 0) {
    Write-Host 'npm audit --audit-level=high findings outside scripts/npm-audit-exceptions.json:'
    $open | Sort-Object -Unique | ForEach-Object { Write-Host $_ }
    exit 1
}

Write-Host 'npm audit --audit-level=high: remaining highs are listed in scripts/npm-audit-exceptions.json'
exit 0
