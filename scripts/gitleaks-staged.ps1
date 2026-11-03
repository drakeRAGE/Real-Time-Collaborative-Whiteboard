# Pre-commit: reject staged secrets. lefthook.yml calls this script.
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
$goBin = Join-Path $env:USERPROFILE 'go\bin'
if (Test-Path (Join-Path $goBin 'gitleaks.exe')) {
    $env:Path = "$goBin;$env:Path"
}

& gitleaks protect --staged --redact --no-banner --config .gitleaks.toml
exit $LASTEXITCODE
