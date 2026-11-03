# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOWORK=off
COPY . .
WORKDIR /src/services/room
RUN --mount=type=cache,target=/go/pkg/mod go build -ldflags "-s -w" -o /out/room ./cmd

FROM busybox:1.36.1-musl AS busybox

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /out/room /room
COPY --from=busybox /bin/busybox /busybox
USER nonroot:nonroot
EXPOSE 7001 8001
HEALTHCHECK --interval=5s --timeout=3s --start-period=20s --retries=12 \
  CMD ["/busybox", "wget", "-q", "-T", "2", "-O", "/dev/null", "http://127.0.0.1:8001/healthz"]
ENTRYPOINT ["/room"]
