---
name: cloudflare-bot-management
description: >-
  Configure Cloudflare Bot Fight Mode, Super Bot Fight Mode, or Bot Management
  for this whiteboard without breaking Socket.IO. Use when the user mentions
  bots, bot scores, Bot Fight Mode, Super Bot Fight Mode, verified bots, or
  automated traffic against the app.
---

# Bot protection for this whiteboard

Pick the control the zone's plan actually has. Fetch [Bot Fight Mode](https://developers.cloudflare.com/bots/get-started/bot-fight-mode/), [Super Bot Fight Mode](https://developers.cloudflare.com/bots/get-started/super-bot-fight-mode/), and [Bot Management](https://developers.cloudflare.com/bots/get-started/bot-management/) before changing settings. Score fields and plan gates differ.

| Plan | What you can do here |
| --- | --- |
| Free | Bot Fight Mode. It is zone-wide and can challenge or block definite bots, including Socket.IO polling. Prefer WAF rate limits (`cloudflare-waf`) plus Turnstile on auth if Fight Mode breaks rooms. |
| Pro / Business | Super Bot Fight Mode. Configure actions per traffic class and exclude `/socket.io/`. |
| Enterprise | Bot Management scores 1–99. Definitely automated is 1. Scores under 30 are the usual bot band. Groupings on lower plans are not the same scale. |

## Rules for this app

- Leave `/socket.io/` off interactive challenges and JavaScript detections. Engine.IO polling is script-like on purpose. `frontend/src/utils/socket.js` is a legitimate automated client.
- Verified bots (search and uptime) must stay allowed. Confirm the current field (`cf.bot_management.verified_bot` or the plan's equivalent) in the docs before shipping an expression.
- Static asset fetches and the SPA document are the right place for definitely-automated blocking. The auth form is covered by `cloudflare-turnstile`.
- A bot score of 0 means detection did not run (internal request, earlier redirect, or orange-to-orange). Do not treat 0 as "human" or as "bot".
- JavaScript detections need an HTML document first and a CSP that allows `/cdn-cgi/challenge-platform/`. They do nothing on a socket POST. Use Managed Challenge, not Block, when a detection depends on that script.
- JA3/JA4 are client fingerprints, not user ids. Many browsers share one fingerprint. Do not key room membership or rate limits on them.

## Enterprise expression shape

Confirm field names, then use the same intent:

```txt
(cf.bot_management.score lt 30 and not cf.bot_management.verified_bot and not starts_with(http.request.uri.path, "/socket.io/"))
```

Action: Managed Challenge for the document and API. Block only after Security Events show the challenged population is automated. Keep `/socket.io/` on the WAF rate limit instead.

## False positives

Read Bot Analytics for the path, IP, and detection source before adding a skip. Scope the skip to that path and the Bot Management product. A WAF managed-rule block of a verified crawler is a WAF exception (`cloudflare-waf`), not a bot-score change.
