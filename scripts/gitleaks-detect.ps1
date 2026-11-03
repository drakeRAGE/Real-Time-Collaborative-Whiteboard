# Worktree scan used by `task security-gitleaks`.
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
$goBin = Join-Path $env:USERPROFILE 'go\bin'
if (Test-Path (Join-Path $goBin 'gitleaks.exe')) {
    $env:Path = "$goBin;$env:Path"
}

& gitleaks detect --source . --redact --no-banner --config .gitleaks.toml
exit $LASTEXITCODE
