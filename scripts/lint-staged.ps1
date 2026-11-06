# Pre-commit: golangci-lint on staged Go files, one module at a time.
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
$goBin = Join-Path $env:USERPROFILE 'go\bin'
if (Test-Path (Join-Path $goBin 'golangci-lint.exe')) {
    $env:Path = "$goBin;$env:Path"
}

$files = @(git diff --cached --name-only --diff-filter=ACM -- '*.go')
if ($files.Count -eq 0) { exit 0 }

$groups = @{}
foreach ($file in $files) {
    $mod = Split-Path $file
    if ([string]::IsNullOrEmpty($mod)) { $mod = '.' }
    while ($mod -ne '.' -and -not (Test-Path (Join-Path $mod 'go.mod'))) {
        $parent = Split-Path $mod
        if ([string]::IsNullOrEmpty($parent) -or $parent -eq $mod) {
            $mod = '.'
            break
        }
        $mod = $parent
    }
    if (-not $groups.ContainsKey($mod)) {
        $groups[$mod] = New-Object System.Collections.Generic.List[string]
    }
    if ($mod -eq '.') {
        $rel = $file
    } else {
        $rel = $file.Substring($mod.Length).TrimStart('\', '/')
    }
    $groups[$mod].Add(($rel -replace '\\', '/'))
}

function Invoke-Golangci {
    param(
        [Parameter(Mandatory = $true)][string]$Module,
        [Parameter(Mandatory = $true)][string[]]$Files
    )
    $pkgs = New-Object System.Collections.Generic.List[string]
    $seen = @{}
    foreach ($file in $Files) {
        $native = $file -replace '/', '\'
        $dir = Split-Path $native
        if ([string]::IsNullOrEmpty($dir)) { $pkg = '.' } else { $pkg = './' + ($dir -replace '\\', '/') }
        if ($seen.ContainsKey($pkg)) { continue }
        $seen[$pkg] = $true
        $pkgs.Add($pkg)
    }
    $args = @($pkgs.ToArray())
    $modRoot = if ($Module -eq '.') { $root } else { Join-Path $root $Module }
    $work = '/src'
    if ($Module -ne '.') { $work = '/src/' + ($Module -replace '\\', '/') }
    Push-Location $modRoot
    try {
        try {
            & golangci-lint run --build-tags=integration --timeout 5m @args
            if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
        } catch {
            if ($_.Exception.Message -notmatch 'Application Control policy') { throw }
            Write-Host 'host blocked golangci-lint; running v2.10.1 in docker'
            & docker run --rm -v "${root}:/src" -w $work golangci/golangci-lint:v2.10.1 golangci-lint run --build-tags=integration --timeout 5m @args
            if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
        }
    } finally {
        Pop-Location
    }
}

foreach ($mod in $groups.Keys) {
    Invoke-Golangci -Module $mod -Files @($groups[$mod].ToArray())
}
exit 0
