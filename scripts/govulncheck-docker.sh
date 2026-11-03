#!/bin/sh
set -eu
export CGO_ENABLED=0
export GOWORK=off
export GOTOOLCHAIN=local

# The host module cache is mounted at /go/pkg/mod. When that tree is warm,
# build with the network disabled. go install still calls the proxy.
src="/go/pkg/mod/golang.org/x/vuln@v1.9.0"
if [ ! -f "$src/go.mod" ]; then
  echo "golang.org/x/vuln@v1.9.0 is not in the mounted module cache" >&2
  exit 1
fi
cp -a "$src" /tmp/vuln-src
(
  cd /tmp/vuln-src
  GOPROXY=off GOSUMDB=off go build -mod=readonly -o /usr/local/bin/govulncheck ./cmd/govulncheck
)

for m in . services/gateway services/room services/board services/chat services/presence pkg/config pkg/logger pkg/authn pkg/natsx pkg/redisx pkg/otelx pkg/grpcx; do
  echo "govulncheck ./... $m"
  (cd "/src/$m" && GOPROXY=off govulncheck ./...)
done
