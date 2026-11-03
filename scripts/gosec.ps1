# Run gosec in each Go module.
# The go-install binary is blocked by Windows Application Control on this machine.
# securego/gosec:2.22.10 ships Go 1.25 and cannot load these go 1.26 modules,
# so the fallback builds that same gosec release inside golang:1.26-alpine
# from the host module cache (read-only). go install still calls the proxy.
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$local = Join-Path $env:USERPROFILE 'go\bin\gosec.exe'
$useLocal = $false
if (Test-Path $local) {
    $ErrorActionPreference = 'Continue'
    cmd /c "`"$local`" -version" | Out-Host
    $verCode = $LASTEXITCODE
    $ErrorActionPreference = 'Stop'
    if ($verCode -ne 0) {
        Write-Host 'local gosec.exe is blocked by Application Control'
    } else {
        # -version can succeed while the binary still cannot typecheck go 1.26,
        # or while Application Control blocks the real scan. A clean probe uses
        # the local binary. An internal error falls through to the container.
        $probeDir = Join-Path $root 'pkg\logger'
        $ErrorActionPreference = 'Continue'
        $probe = cmd /c "cd /d `"$probeDir`" && `"$local`" -exclude-generated -fmt text ./..." 2>&1 | Out-String
        $probeCode = $LASTEXITCODE
        $ErrorActionPreference = 'Stop'
        if ($probeCode -eq 0) {
            $useLocal = $true
        } elseif ($probe -match 'internal error|Application Control|is blocked') {
            Write-Host 'local gosec.exe cannot scan these modules'
        } else {
            Write-Host $probe
            exit $probeCode
        }
    }
}
if ($useLocal) {
    & powershell -NoProfile -File (Join-Path $PSScriptRoot 'go-each-module.ps1') -Command 'gosec -exclude-generated -fmt text ./...'
    exit $LASTEXITCODE
}

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Write-Host 'docker is not available and local gosec.exe did not run'
    exit 1
}
docker image inspect golang:1.26-alpine 1>$null 2>$null
if ($LASTEXITCODE -ne 0) {
    Write-Host 'image golang:1.26-alpine is not present; local gosec.exe did not run'
    exit 1
}

Write-Host 'using golang:1.26-alpine to run gosec v2.22.10'
$sh = Join-Path $PSScriptRoot 'gosec-docker.sh'
$lf = [IO.File]::ReadAllText($sh) -replace "`r`n", "`n" -replace "`r", "`n"
$utf8 = New-Object System.Text.UTF8Encoding $false
[IO.File]::WriteAllText($sh, $lf, $utf8)
docker run --rm -v "${root}:/src" -v "${env:USERPROFILE}/go/pkg/mod:/go/pkg/mod:ro" -w /src -e GOWORK=off -e GOTOOLCHAIN=local golang:1.26-alpine sh /src/scripts/gosec-docker.sh
exit $LASTEXITCODE
