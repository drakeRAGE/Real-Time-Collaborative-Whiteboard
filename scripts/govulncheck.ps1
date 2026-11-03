# Run govulncheck in each Go module.
# Prefer a local govulncheck.exe. Windows Application Control blocks the
# go-install build on this machine, so the fallback is the local golang image:
#   docker run --rm -v <repo>:/src -w /src golang:1.26-alpine govulncheck ./...
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$local = Join-Path $env:USERPROFILE 'go\bin\govulncheck.exe'
if (Test-Path $local) {
    $ErrorActionPreference = 'Continue'
    cmd /c "`"$local`" -version" | Out-Host
    $ErrorActionPreference = 'Stop'
    if ($LASTEXITCODE -eq 0) {
        & powershell -NoProfile -File (Join-Path $PSScriptRoot 'go-each-module.ps1') -Command 'govulncheck ./...'
        exit $LASTEXITCODE
    }
    Write-Host 'local govulncheck.exe is blocked by Application Control'
}

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Write-Host 'docker is not available and local govulncheck.exe did not run'
    exit 1
}
docker image inspect golang:1.26-alpine 1>$null 2>$null
if ($LASTEXITCODE -ne 0) {
    Write-Host 'image golang:1.26-alpine is not present; local govulncheck.exe did not run'
    exit 1
}

Write-Host 'local govulncheck.exe did not run; using golang:1.26-alpine'
$sh = Join-Path $PSScriptRoot 'govulncheck-docker.sh'
$lf = [IO.File]::ReadAllText($sh) -replace "`r`n", "`n" -replace "`r", "`n"
$utf8 = New-Object System.Text.UTF8Encoding $false
[IO.File]::WriteAllText($sh, $lf, $utf8)
docker run --rm -v "${root}:/src" -v "${env:USERPROFILE}/go/pkg/mod:/go/pkg/mod:ro" -w /src -e CGO_ENABLED=0 -e GOWORK=off -e GOTOOLCHAIN=local golang:1.26-alpine sh /src/scripts/govulncheck-docker.sh
exit $LASTEXITCODE
