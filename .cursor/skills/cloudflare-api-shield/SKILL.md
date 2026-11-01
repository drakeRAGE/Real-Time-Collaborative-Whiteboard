---
name: cloudflare-api-shield
description: >-
  Apply Cloudflare API Shield discovery, schema validation, JWT validation,
  and abuse or BOLA detection to this whiteboard HTTP API when the account
  includes API Shield. Use when the user mentions API Shield, endpoint
  discovery, schema validation, mTLS, BOLA, or edge JWT validation.
---

# API Shield for this whiteboard

API Shield is an Enterprise add-on. If the account does not include it, stop and use `cloudflare-waf` rate limits plus origin checks. Do not imitate API Shield with a custom ruleset and call it schema validation. Fetch [API Shield](https://developers.cloudflare.com/api-shield/) and the feature page you are changing (schema validation, JWT, mTLS, discovery) first.

## Inventory

The HTTP API in this repo is:

| Method and path | Auth today | Note |
| --- | --- | --- |
| `DELETE /rooms/:roomId` | None in `backend/controllers/room.controller.js` | Deletes the Mongo room and emits `roomDeleted`. |
| `POST`/`GET` `/socket.io/` | Supabase access token on the handshake | Engine.IO, not a REST API. Keep it out of schema enforcement. |

Sign-in traffic goes to Supabase from `frontend/src/components/Auth.jsx`. It is not an endpoint on this zone unless the project later proxies it.

There is no OpenAPI file in the repo. Discovery will see a small surface. Do not invent paths.

## JWT validation

`backend/auth/supabaseAuth.js` verifies tokens with the shared `SUPABASE_JWT_SECRET` (HS256). Newer Supabase projects sign with asymmetric keys and publish `{SUPABASE_URL}/auth/v1/.well-known/jwks.json`.

1. Check the project's Supabase JWT settings before choosing a credential.
2. Prefer the JWKS URL when asymmetric signing keys are enabled.
3. If the project is still on the legacy secret, store that secret in the Cloudflare credential the JWT rule references. Do not put the secret in a rule expression, a commit, or chat.
4. A passing edge JWT check does not make `DELETE /rooms/:roomId` safe. The origin must allow only that room's admin (`adminId` on the room document).

## Schema and discovery

- Learn from production traffic or a written schema. Enforce in log, then block.
- Validate `DELETE /rooms/:roomId` only: method, path shape, no unexpected body.
- Exclude `/socket.io/` from schema and sequence rules. Polling payloads are Engine.IO frames, not JSON API calls.
- mTLS does not fit the public browser app. Skip it unless a separate machine client is part of the request.
- BOLA and abuse detection need a session identifier. The room id in the path is an object id, not a session. Use the Supabase subject (`sub`) once delete requests actually send the bearer token.

## Origin gap to fix in the same change

Edge inventory will not add a missing check. When enabling API Shield for rooms, update `deleteRoom` to require `verifySupabaseToken` and `room.adminId === user.sub`, and register that route before the production SPA catch-all in `backend/index.js`.
