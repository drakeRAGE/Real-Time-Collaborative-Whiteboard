param(
    [Parameter(Mandatory = $true)]
    [string]$Command
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$mods = @(
    '.',
    './services/gateway',
    './services/room',
    './services/board',
    './services/chat',
    './services/presence',
    './pkg/config',
    './pkg/logger',
    './pkg/authn',
    './pkg/natsx',
    './pkg/redisx',
    './pkg/otelx',
    './pkg/grpcx'
)

foreach ($m in $mods) {
    Write-Host "$Command $m"
    Push-Location $m
    try {
        Invoke-Expression $Command
        if ($LASTEXITCODE -ne 0) {
            exit $LASTEXITCODE
        }
    } finally {
        Pop-Location
    }
}
