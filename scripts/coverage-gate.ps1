# Fails when statement coverage is under 80% for pkg/ or under 70% for services/.
# Uses `go test -cover` (unit tests only; the integration build tag stays off).
# A cached result still prints "coverage: N% of statements". Statement weights come
# from `go tool cover` block metadata, which does not execute a test binary.
$ErrorActionPreference = 'Stop'

$Go = 'C:\Program Files\Go\bin\go.exe'
if (-not (Test-Path -LiteralPath $Go)) {
    throw "go not found at $Go"
}

$Root = Split-Path -Parent $PSScriptRoot
$PkgFloor = 80.0
$ServiceFloor = 70.0

function Get-BlockStatements {
    param([Parameter(Mandatory = $true)][string]$File)
    $text = & $Go tool cover -mode=set -var=Cover $File
    if ($LASTEXITCODE -ne 0) {
        throw "go tool cover failed for $File"
    }
    $sum = 0
    $inNum = $false
    foreach ($line in $text) {
        if ($line -match '^\s*NumStmt:') { $inNum = $true; continue }
        if (-not $inNum) { continue }
        if ($line -match '^\s*\},') { break }
        if ($line -match '^\s*(\d+),') { $sum += [int]$Matches[1] }
    }
    return $sum
}

function Get-PackageStatements {
    param([Parameter(Mandatory = $true)][string]$Dir)
    $list = & $Go list -f '{{.Dir}}|{{range .GoFiles}}{{.}} {{end}}' ./...
    if ($LASTEXITCODE -ne 0) { throw "go list failed in $Dir" }
    $byDir = @{}
    foreach ($line in $list) {
        $parts = $line -split '\|', 2
        if ($parts.Count -lt 2) { continue }
        $pkgDir = $parts[0].Trim()
        $names = $parts[1].Trim() -split '\s+' | Where-Object { $_ }
        $sum = 0
        foreach ($name in $names) {
            $sum += Get-BlockStatements -File (Join-Path $pkgDir $name)
        }
        $byDir[$pkgDir] = $sum
    }
    return $byDir
}

function Get-CoveredCount {
    param([int]$Statements, [double]$Percent)
    if ($Statements -le 0) { return 0 }
    $best = 0
    $bestDiff = [double]::MaxValue
    for ($c = 0; $c -le $Statements; $c++) {
        $shown = [math]::Round((100.0 * $c / $Statements), 1, [MidpointRounding]::AwayFromZero)
        $diff = [math]::Abs($shown - $Percent)
        if ($diff -lt $bestDiff) {
            $best = $c
            $bestDiff = $diff
        }
    }
    return $best
}

function Measure-Module {
    param([Parameter(Mandatory = $true)][string]$Dir)
    Push-Location -LiteralPath $Dir
    try {
        $weights = Get-PackageStatements -Dir $Dir
        $output = & $Go test -cover ./... 2>&1
        $exit = $LASTEXITCODE
        $joined = $output | Out-String
        if ($exit -ne 0 -and $joined -match 'Application Control policy') {
            Write-Host '  host blocked a test binary; go test -cover in golang:1.26-alpine'
            $rel = $Dir.Substring($Root.Length).TrimStart('\').Replace('\', '/')
            $prev = $ErrorActionPreference
            $ErrorActionPreference = 'Continue'
            try {
                $output = & docker run --rm --network whiteboard_default `
                    -v "${Root}:/src" `
                    -v "C:\Users\Interloop12\go\pkg\mod:/go/pkg/mod" `
                    -v wb-gocache:/root/.cache/go-build `
                    -w "/src/$rel" `
                    -e GOWORK=off -e CGO_ENABLED=0 `
                    -e REDIS_ADDR=redis:6379 `
                    -e NATS_URL=nats://nats:4222 `
                    golang:1.26-alpine go test -mod=readonly -cover ./... 2>&1
                $exit = $LASTEXITCODE
            }
            finally {
                $ErrorActionPreference = $prev
            }
        }
        $output | ForEach-Object { Write-Host "  $_" }
        if ($exit -ne 0) { throw "go test -cover failed in $Dir (exit $exit)" }

        $totalStmts = 0
        $totalCovered = 0
        foreach ($line in $output) {
            $text = "$line"
            if ($text -match '^ok\s+' -and $text -notmatch 'coverage:\s+[0-9.]+% of statements') {
                throw "go test -cover did not report coverage: $text"
            }
            if ($text -notmatch '^ok\s+(\S+)\s+.*coverage:\s+([0-9.]+)% of statements') { continue }
            $importPath = $Matches[1]
            $pct = [double]$Matches[2]
            $pkgDir = (& $Go list -f '{{.Dir}}' $importPath).Trim()
            $pkgDir = [System.IO.Path]::GetFullPath($pkgDir)
            $weightKey = $weights.Keys | Where-Object { [System.IO.Path]::GetFullPath($_) -eq $pkgDir } | Select-Object -First 1
            if (-not $weightKey) {
                throw "no statement weight for $importPath ($pkgDir)"
            }
            $stmts = [int]$weights[$weightKey]
            $covered = Get-CoveredCount -Statements $stmts -Percent $pct
            Write-Host ("  {0} {1}% -> {2}/{3}" -f $importPath, $pct, $covered, $stmts)
            $totalStmts += $stmts
            $totalCovered += $covered
        }
        return @{ Statements = $totalStmts; Covered = $totalCovered }
    }
    finally {
        Pop-Location
    }
}

function Get-Percent {
    param($Part)
    if ($Part.Statements -le 0) { return 0.0 }
    return 100.0 * $Part.Covered / $Part.Statements
}

function Measure-Group {
    param(
        [Parameter(Mandatory = $true)][string]$Label,
        [Parameter(Mandatory = $true)][string[]]$Dirs
    )
    $total = @{ Statements = 0; Covered = 0 }
    foreach ($dir in $Dirs) {
        $full = Join-Path $Root $dir
        if (-not (Test-Path -LiteralPath (Join-Path $full 'go.mod'))) {
            throw "missing go.mod in $dir"
        }
        Write-Host "go test -cover $dir"
        $part = Measure-Module -Dir $full
        $pct = Get-Percent $part
        Write-Host ("  module {0:N1}%  ({1}/{2} statements)" -f $pct, $part.Covered, $part.Statements)
        if ($Label -eq 'services' -and $pct -lt $ServiceFloor) {
            $script:ServiceLows += ("{0} {1:N1}%" -f $dir, $pct)
        }
        $total.Statements += $part.Statements
        $total.Covered += $part.Covered
    }
    $pct = Get-Percent $total
    Write-Host ("{0} total {1:N1}%  ({2}/{3} statements)" -f $Label, $pct, $total.Covered, $total.Statements)
    return $pct
}

$pkgDirs = Get-ChildItem -Path (Join-Path $Root 'pkg') -Directory | ForEach-Object { "pkg/$($_.Name)" }
$serviceDirs = Get-ChildItem -Path (Join-Path $Root 'services') -Directory | ForEach-Object { "services/$($_.Name)" }

$script:ServiceLows = @()
$pkgPct = Measure-Group -Label 'pkg' -Dirs $pkgDirs
$servicePct = Measure-Group -Label 'services' -Dirs $serviceDirs

$failed = $false
if ($pkgPct -lt $PkgFloor) {
    Write-Host "FAIL pkg coverage $([math]::Round($pkgPct, 1))% is under $PkgFloor%"
    $failed = $true
}
if ($servicePct -lt $ServiceFloor) {
    Write-Host "FAIL service coverage $([math]::Round($servicePct, 1))% is under $ServiceFloor%"
    $failed = $true
}
if ($script:ServiceLows.Count -gt 0) {
    Write-Host "FAIL service module under ${ServiceFloor}%: $($script:ServiceLows -join ', ')"
    $failed = $true
}
if ($failed) { exit 1 }

Write-Host "PASS pkg $([math]::Round($pkgPct, 1))% >= $PkgFloor%; services $([math]::Round($servicePct, 1))% >= $ServiceFloor%"
exit 0
