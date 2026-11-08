# Creates the kind cluster, installs Calico and ingress-nginx, and installs the chart.
$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
Set-Location $Root

function Invoke-Native {
  param([string]$File, [string[]]$ArgumentList)
  & $File @ArgumentList
  if ($LASTEXITCODE -ne 0) {
    throw "$File $($ArgumentList -join ' ') failed ($LASTEXITCODE)"
  }
}

foreach ($name in @("DATABASE_URL", "SUPABASE_JWT_ISSUER", "SUPABASE_JWKS_URL")) {
  if ([string]::IsNullOrWhiteSpace([Environment]::GetEnvironmentVariable($name))) {
    throw "Set $name before task kind-up. See docs/runbooks/k8s.md."
  }
}

$images = @(
  "whiteboard-gateway:dev",
  "whiteboard-room:dev",
  "whiteboard-board:dev",
  "whiteboard-chat:dev",
  "whiteboard-presence:dev"
)
foreach ($image in $images) {
  docker image inspect $image *> $null
  if ($LASTEXITCODE -ne 0) {
    throw "Missing image $image. Build it with: docker compose -f deploy/compose/docker-compose.yml build"
  }
}

# Repackage the local charts. Helm skips a charts/*.tgz that is already present.
Get-ChildItem "deploy/k8s/charts/whiteboard/charts" -Filter "*-0.1.0.tgz" -ErrorAction SilentlyContinue | Remove-Item -Force
Invoke-Native "helm" @("dependency", "build", "deploy/k8s/charts/whiteboard")

$log = Join-Path $env:TEMP "kind-whiteboard-create.log"
$err = Join-Path $env:TEMP "kind-whiteboard-create.err"
if (Test-Path $log) { Remove-Item $log }
if (Test-Path $err) { Remove-Item $err }
$kind = Start-Process -FilePath "kind" -ArgumentList @("create", "cluster", "--config", "deploy/k8s/kind.yaml") -PassThru -RedirectStandardOutput $log -RedirectStandardError $err -WorkingDirectory $Root

$api = $false
for ($i = 0; $i -lt 90; $i++) {
  if ($kind.HasExited) {
    if ($null -ne $kind.ExitCode -and $kind.ExitCode -ne 0) {
      Get-Content $log, $err -ErrorAction SilentlyContinue
      throw "kind create cluster failed ($($kind.ExitCode))"
    }
  }
  $prevEap = $ErrorActionPreference
  $ErrorActionPreference = "SilentlyContinue"
  kubectl --context kind-whiteboard get --raw=/readyz 2>$null | Out-Null
  $readyExit = $LASTEXITCODE
  $ErrorActionPreference = $prevEap
  if ($readyExit -eq 0) {
    $api = $true
    break
  }
  Start-Sleep -Seconds 2
}
if (-not $api) {
  Get-Content $log, $err -ErrorAction SilentlyContinue
  throw "kind API did not become ready"
}

Invoke-Native "kubectl" @("--context", "kind-whiteboard", "apply", "-f", "https://raw.githubusercontent.com/projectcalico/calico/v3.30.3/manifests/calico.yaml")
Invoke-Native "kubectl" @("--context", "kind-whiteboard", "-n", "kube-system", "rollout", "status", "ds/calico-node", "--timeout=600s")
Invoke-Native "kubectl" @("--context", "kind-whiteboard", "wait", "--for=condition=Ready", "node", "--all", "--timeout=180s")

if (-not $kind.HasExited) {
  if (-not $kind.WaitForExit(180000)) {
    throw "kind create cluster did not exit"
  }
}
if ($null -ne $kind.ExitCode -and $kind.ExitCode -ne 0) {
  Get-Content $log, $err -ErrorAction SilentlyContinue
  throw "kind create cluster failed ($($kind.ExitCode))"
}

Invoke-Native "kubectl" @("apply", "-f", "https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.13.2/deploy/static/provider/kind/deploy.yaml")
# kind.yaml labels only the control-plane ingress-ready; hostPort 80 must land there
# for extraPortMappings 8088/8443 (docs/runbooks/k8s.md).
Invoke-Native "kubectl" @(
  "-n", "ingress-nginx", "patch", "deployment", "ingress-nginx-controller",
  "--type", "json", "--patch-file", "deploy/k8s/ingress-control-plane-patch.json"
)
Invoke-Native "kind" @(
  "load", "docker-image", "registry.k8s.io/ingress-nginx/controller:v1.13.2", "--name", "whiteboard"
)
Invoke-Native "kubectl" @("-n", "ingress-nginx", "rollout", "status", "deploy/ingress-nginx-controller", "--timeout=300s")

foreach ($image in $images) {
  Invoke-Native "kind" @("load", "docker-image", $image, "--name", "whiteboard")
}

kubectl create namespace whiteboard --dry-run=client -o yaml | kubectl apply -f -
if ($LASTEXITCODE -ne 0) { throw "namespace whiteboard failed" }
kubectl create namespace nats --dry-run=client -o yaml | kubectl apply -f -
if ($LASTEXITCODE -ne 0) { throw "namespace nats failed" }

kubectl -n whiteboard create secret generic whiteboard-supabase `
  --from-literal=issuer="$env:SUPABASE_JWT_ISSUER" `
  --from-literal=jwksUrl="$env:SUPABASE_JWKS_URL" `
  --from-literal=databaseUrl="$env:DATABASE_URL" `
  --dry-run=client -o yaml | kubectl apply -f -
if ($LASTEXITCODE -ne 0) { throw "whiteboard-supabase secret failed" }

Invoke-Native "helm" @(
  "upgrade", "--install", "whiteboard", "deploy/k8s/charts/whiteboard",
  "--namespace", "whiteboard",
  "--create-namespace",
  "-f", "deploy/k8s/charts/whiteboard/values.yaml",
  "-f", "deploy/k8s/charts/whiteboard/values-dev.yaml",
  "--wait", "--timeout", "8m"
)

Invoke-Native "kubectl" @("apply", "-f", "deploy/k8s/netpol")
Invoke-Native "kubectl" @("-n", "whiteboard", "rollout", "status", "deploy/gateway", "--timeout=180s")

powershell -NoProfile -File (Join-Path $PSScriptRoot "smoke.ps1")
if ($LASTEXITCODE -ne 0) { throw "smoke script failed" }
Write-Output "kind-up ok"
