---
name: cloudflare-waf
description: >-
  Configure Cloudflare WAF managed rules, custom rules, and rate limiting for
  this whiteboard's HTTP API and Socket.IO paths. Use when adding WAF rules,
  rate limits, skip or exception rules, or investigating blocked whiteboard,
  room, or socket traffic.
---

# Cloudflare WAF for this whiteboard

Fetch current WAF docs before writing an expression or API body. Start at the [WAF get started](https://developers.cloudflare.com/waf/get-started/) guide, then the page for the change: [managed rules](https://developers.cloudflare.com/waf/managed-rules/), [custom rules](https://developers.cloudflare.com/waf/custom-rules/), [rate limiting](https://developers.cloudflare.com/waf/rate-limiting-rules/), [skip options](https://developers.cloudflare.com/waf/custom-rules/skip/options/), [phases](https://developers.cloudflare.com/waf/reference/phases/).

Also follow `cloudflare-security` for zone scope, log-then-enforce, and the Socket.IO constraints.

## Surfaces to protect

| Path | Intent | Edge action |
| --- | --- | --- |
| `DELETE /rooms/:roomId` | Destructive, currently unauthenticated in `backend/controllers/room.controller.js` | Tight rate limit per IP. Block unexpected methods on `/rooms/*`. Origin must still require the room admin. |
| `/socket.io/` | Live draw, chat, cursor, call signaling | High rate-limit ceiling. Action `block` when exceeded. Do not `managed_challenge` this path. |
| Static SPA (`/`, assets) | Vite build | Managed rules only. Cache hashed assets. |
| Everything else | No other product routes exist today | Custom rule: log, then block methods other than `GET`, `HEAD`, `OPTIONS` outside `/socket.io/` and `/rooms/`. |

`joinRoom` creates a room for any id after the JWT check. Edge rate limits slow that down. Room authorization still belongs in the `joinRoom` handler in `backend/index.js`.

## Implementation rules

- Read the zone ruleset, then add one rule. A PUT of a ruleset replaces it.
- Deploy managed rulesets in log mode first on this zone. Move a ruleset to block only after Security Events show no Engine.IO or asset false positives.
- Skip rules name the phase or product they skip. A skip that does not name the managed ruleset leaves that ruleset active.
- Rate-limit **counting** and **matching** are separate. Match the route. Count by IP. For `DELETE /rooms/:roomId`, also count by the `roomId` path segment when the plan supports a counting expression, so one client cannot rotate IDs freely.
- Shared NATs make a very low IP threshold block classrooms and offices. Size the `/socket.io/` limit for a room of concurrent polling clients, not for a single page view.
- `express-rate-limit` in `backend/index.js` is unused. An edge limit does not replace a shared limit in the app if the origin is ever reached directly. When editing the app, apply a real limiter on `DELETE /rooms/:roomId` and keep the edge rule as the outer control.
- Register new HTTP routes before the production catch-all `app.get(/.*/)` in `backend/index.js`, or the SPA will swallow them.
- Investigate a block from the ray id and rule id in Security Events before adding an exception. Narrow the exception to that rule id and path.

## Do not

- Challenge or JS-detect `/socket.io/`.
- Cache `POST` polling responses or room API responses.
- Put `SUPABASE_JWT_SECRET` in a WAF expression.
- Treat a WAF skip as permission to delete a room.
