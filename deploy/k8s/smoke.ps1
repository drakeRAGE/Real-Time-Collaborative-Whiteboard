# Proves the gateway through ingress and that a debug pod cannot reach room.
$ErrorActionPreference = "Stop"
$Root = Resolve-Path (Join-Path $PSScriptRoot "..\..")
$HostAddr = if ($env:SMOKE_HOST) { $env:SMOKE_HOST } else { "127.0.0.1" }
$Port = if ($env:SMOKE_PORT) { $env:SMOKE_PORT } else { "8088" }
$Origin = if ($env:SMOKE_ORIGIN) { $env:SMOKE_ORIGIN } else { "http://localhost:5173" }

function Fail([string]$Message) {
  Write-Error $Message
  exit 1
}

$ready = $false
for ($i = 0; $i -lt 30; $i++) {
  try {
    $resp = Invoke-WebRequest -Uri "http://${HostAddr}:${Port}/readyz" -UseBasicParsing -TimeoutSec 5
    if ($resp.StatusCode -eq 200) {
      $ready = $true
      break
    }
  } catch {
    Start-Sleep -Seconds 2
  }
}
if (-not $ready) {
  Fail "readyz via ingress did not return 200 at http://${HostAddr}:${Port}/readyz"
}
Write-Output "readyz ok"

$env:SMOKE_HOST = $HostAddr
$env:SMOKE_PORT = $Port
$env:SMOKE_ORIGIN = $Origin
node (Join-Path $PSScriptRoot "smoke-ws.mjs")
if ($LASTEXITCODE -ne 0) {
  Fail "websocket echo check failed"
}
Write-Output "ws echo ok"

$endpoints = kubectl -n whiteboard get endpoints room -o jsonpath="{.subsets}"
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($endpoints)) {
  Fail "room has no endpoints; the network policy check would not be meaningful"
}

kubectl -n whiteboard delete pod netpol-debug --ignore-not-found | Out-Null
kubectl -n whiteboard run netpol-debug --image=curlimages/curl:8.11.1 --restart=Never --command -- sleep 90
if ($LASTEXITCODE -ne 0) {
  Fail "could not start the network policy debug pod"
}
kubectl -n whiteboard wait --for=condition=Ready pod/netpol-debug --timeout=120s
if ($LASTEXITCODE -ne 0) {
  Fail "network policy debug pod did not become Ready"
}

kubectl -n whiteboard exec netpol-debug -- curl -sS --max-time 5 "http://room:8001/healthz"
$curlExit = $LASTEXITCODE
kubectl -n whiteboard delete pod netpol-debug --ignore-not-found | Out-Null
if ($curlExit -eq 0) {
  Fail "network policy allowed the debug pod to reach room:8001"
}
Write-Output "netpol blocked room (curl exit $curlExit)"
