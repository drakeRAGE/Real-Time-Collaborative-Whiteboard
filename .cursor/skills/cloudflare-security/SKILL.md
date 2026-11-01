---
name: cloudflare-security
description: >-
  Select Cloudflare security controls for this realtime whiteboard (WAF,
  Turnstile, Bot Management, DDoS, API Shield) in front of the Render origin.
  Use when the user mentions Cloudflare security, protecting the app, edge
  rules, bot abuse, rate limits, or securing sign-in, rooms, or Socket.IO.
---

# Cloudflare security for this whiteboard

> **ARCHITECTURE MIGRATION NOTE (check first):** This repo is being rebuilt from Express + Socket.IO into Go microservices (plan: "Senior Portfolio Upgrade"). If `services/gateway/` exists, the facts below about the origin are STALE: realtime is native WebSocket at `/ws` (not `/socket.io/`, no polling), the HTTP API is `/api/v1/*` on the Go gateway (JWT-authenticated, strict CORS), and JWT verification uses Supabase JWKS (not the HS256 shared secret). Apply the same principles (no challenges on the WS path, cache bypass for `/ws` and `/api/`, rate-limit rather than block hot paths) but target the new paths. If `services/gateway/` does not exist yet, the description below is accurate.

Choose the smallest Cloudflare control that matches the abuse, then keep authorization on the origin. Fetch current docs before writing rules or API calls (plan limits, expression fields, and phase order change). Prefer the Cloudflare docs search tools, then [developers.cloudflare.com](https://developers.cloudflare.com/).

Load the product skill that matches the task:

| Abuse | Skill |
| --- | --- |
| Malicious HTTP, room-delete floods, Socket.IO connection floods | `cloudflare-waf` |
| Scripted sign-up or sign-in | `cloudflare-turnstile` |
| Automated clients that are not a single hot path | `cloudflare-bot-management` |
| Volumetric network or HTTP floods | `cloudflare-ddos` |
| Schema, JWT, or endpoint inventory for the HTTP API | `cloudflare-api-shield` |

## What this app actually exposes

- **Origin.** Express plus Socket.IO, deployed on Render. Production serves the Vite build from the same process.
- **Sign-in.** `frontend/src/components/Auth.jsx` calls Supabase `signInWithPassword` and `signUp` directly. That traffic goes to Supabase, not this zone, unless Supabase is on a proxied custom domain.
- **Sockets.** `frontend/src/utils/socket.js` connects to `VITE_BACKEND_URL` with the Supabase access token in `auth.token`. `backend/auth/socketAuthMiddleware.js` checks it with `jsonwebtoken` and `SUPABASE_JWT_SECRET` (legacy HS256 shared secret). Transports are `websocket` and `polling` on `/socket.io/`.
- **HTTP API.** `DELETE /rooms/:roomId` in `backend/routes/room.routes.js`. The controller deletes the room with no token check.
- **In-app limits.** `express-rate-limit` is imported in `backend/index.js` and never applied. Chat uses a per-socket 500ms gap in memory only.
- **CORS.** Express and Socket.IO allow every origin.
- **Media.** WebRTC media is peer-to-peer. Cloudflare sees signaling on the socket, not the media path.
- **Secrets.** `SUPABASE_JWT_SECRET` and `MONGO` are server secrets. `VITE_APP_SUPABASE_ANON_KEY` is a public client key. Turnstile widget secrets are server secrets.

## Shared rules

1. Identify the account and zone. Read the existing ruleset before a change. Appending via a full ruleset replace deletes rules you did not resend.
2. Start hot paths in **log**, confirm Security Events, then enforce. Keep exceptions as narrow as the matched requests.
3. An edge allow or skip does not authorize the origin. `DELETE /rooms/:roomId` still needs an authenticated admin check in `backend/controllers/room.controller.js`.
4. Leave `/socket.io/` challenge-free. Polling and Engine.IO heartbeats look automated. Challenges and Bot Fight blocks on that path drop live rooms. Rate-limit it instead.
5. Bypass cache for `/socket.io/` and `/rooms/`. Cache only hashed static assets.
6. Proxied hostnames must allow WebSockets. Polling requests must finish before the Cloudflare proxy idle timeout. Prefer the websocket transport once the zone is proxied.
7. Multiple origin instances need Socket.IO affinity. Engine.IO session ids are instance-local.
8. Do not put Cloudflare Access in front of the public app. Use Access only for an explicitly internal hostname (admin or preview).
9. Never write secrets into rules, git, logs, or chat. Configure them in the host secret store (Render env, or Cloudflare Secrets Store / Worker secrets when a Worker holds them).

## Order of work

1. Proxy the hostname (orange cloud) and confirm WebSockets plus cache bypass for sockets and room routes.
2. Turn on managed WAF and default HTTP DDoS. Leave sensitivity at the default until a confirmed false positive exists.
3. Add rate limits for `DELETE /rooms/:roomId` and a higher ceiling for `/socket.io/`.
4. Add Turnstile around Supabase sign-in and sign-up, with siteverify on this backend before the client calls Supabase.
5. Add bot controls that exclude `/socket.io/` from interactive challenges.
6. Use API Shield only when the account includes it. Otherwise keep JWT checks in `backend/auth/supabaseAuth.js` and fix the unauthenticated delete route on the origin.
