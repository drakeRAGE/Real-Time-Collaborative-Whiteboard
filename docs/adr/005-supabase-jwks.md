# ADR 005: Supabase JWTs verified with JWKS

## Status

Accepted.

## Context

The browser signs in with Supabase Auth (`frontend/src/context/SocketProvider.tsx` reads the access token). The gateway has to decide who that token is without sharing a session store with Supabase, and without trusting a user id inside a drawing or chat payload. A shared HMAC secret is simple and is what the older app used. It is also a single secret that, if it leaks, forges any user, and it does not follow Supabase's asymmetric signing keys.

## Decision

Verify access tokens in the gateway with `pkg/authn`. `pkg/authn/authn.go` checks RS256 and ES256 against a JWKS document cached for about 15 minutes. The expected `iss` is required. An empty audience defaults to `authenticated`. The gateway loads this from `SUPABASE_JWKS_URL`, `SUPABASE_JWT_ISSUER`, and `SUPABASE_JWT_AUDIENCE` (`services/gateway/internal/config/config.go`) and constructs the verifier in `services/gateway/internal/app/app.go`.

HS256 is off unless `SUPABASE_JWT_ALLOW_HS256` is true, in which case `SUPABASE_JWT_SECRET` is required. `config.go` rejects a wildcard `FRONTEND_URL` and rejects a JWKS-less process when HS256 is off. Compose sets the HS256 flag so the local stack can boot without fetching a cloud JWKS document (`deploy/compose/docker-compose.yml`). That local secret is a placeholder. Cloud tokens signed with the asymmetric key are the mode `AllowHS256: false` implements.

REST uses `Authorization: Bearer`. The socket uses the `bearer` subprotocol or the first frame, as recorded in `docs/SECURITY.md`. After verification, gRPC metadata is `wb-user-id` (`services/board/internal/service/principal.go`) and NATS headers are `Wb-User-Id` and `Wb-Username` (`services/board/internal/ingest/consumer.go`).

## Consequences

Domain services trust the gateway-stamped principal. Payload fields are not identity. JWKS fetch and rotation are operational: `Verifier.Refresh` reloads the document, and a failed fetch at startup returns an error (`pkg/authn/authn.go`). There is no revocation list. A stolen access token works until `exp` (`docs/THREAT_MODEL.md`). The local Compose gateway and a cloud asymmetric project are different verifiers. Pointing the Vite app at cloud Supabase while the compose gateway still has `SUPABASE_JWT_ALLOW_HS256=true` will not accept those cloud tokens.
