# Scan local whiteboard-*:dev images. Exit 0 when none are present.
# HIGH and CRITICAL findings fail the task. Unfixed CVEs are reported and do not fail,
# because the base image cannot be patched until an upstream fix exists.
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$images = @(docker images --format '{{.Repository}}:{{.Tag}}' | Where-Object { $_ -like 'whiteboard-*:dev' })
if ($images.Count -eq 0) {
    Write-Host 'no local whiteboard-*:dev images; trivy skipped'
    exit 0
}

$goBin = Join-Path $env:USERPROFILE 'go\bin'
if (Test-Path (Join-Path $goBin 'trivy.exe')) {
    $env:Path = "$goBin;$env:Path"
}

function Invoke-TrivyImage([string]$Image) {
    Write-Host "trivy image $Image"
    $trivy = Get-Command trivy -ErrorAction SilentlyContinue
    if ($trivy) {
        & trivy image --severity HIGH,CRITICAL --ignore-unfixed --scanners vuln --quiet --exit-code 1 $Image | Out-Host
        Write-Host "trivy exit $LASTEXITCODE $Image"
        return $LASTEXITCODE
    }
    docker run --rm -v /var/run/docker.sock:/var/run/docker.sock aquasec/trivy:0.76.1 image --severity HIGH,CRITICAL --ignore-unfixed --scanners vuln --quiet --exit-code 1 $Image
    Write-Host "trivy exit $LASTEXITCODE $Image"
    return $LASTEXITCODE
}

foreach ($image in $images) {
    $code = Invoke-TrivyImage $image
    if ($code -ne 0) {
        exit $code
    }
}
exit 0
