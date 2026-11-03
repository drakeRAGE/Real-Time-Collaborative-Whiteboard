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

foreach ($mod in $groups.Keys) {
    Push-Location (Join-Path $root $mod)
    try {
        & golangci-lint run --timeout 5m @($groups[$mod].ToArray())
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    } finally {
        Pop-Location
    }
}
exit 0
