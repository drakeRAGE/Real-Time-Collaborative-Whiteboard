---
name: cloudflare-turnstile
description: >-
  Add Cloudflare Turnstile to this whiteboard's Supabase sign-in and sign-up,
  with server-side siteverify on the Express backend. Use when the user
  mentions Turnstile, CAPTCHA, signup abuse, credential stuffing, or bot
  checks on authentication.
---

# Turnstile on this whiteboard

Auth does not hit Express today. `frontend/src/components/Auth.jsx` calls `supabase.auth.signInWithPassword` and `supabase.auth.signUp` in the browser. A widget with no server check is bypassable. Siteverify must succeed on this backend before the client calls Supabase.

When the user wants the widget created in their Cloudflare account, follow the installed `turnstile-spin` skill for token creation, storage, and validation. This skill is the app-specific wiring. Fetch [Turnstile docs](https://developers.cloudflare.com/turnstile/) before choosing widget mode or the siteverify URL.

## Wiring

1. **Widget places.** The email form in `Auth.jsx`, covering both Sign In and Sign Up. Use a fresh token per submit. Set `data-action` to `signin` or `signup` to match the button.
2. **Siteverify route.** Add an Express `POST` that accepts the token, calls `https://challenges.cloudflare.com/turnstile/v0/siteverify` with the widget secret, and returns only success or failure. Run it before `signInWithPassword` or `signUp`. Reject reused tokens (Turnstile tokens are single-use).
3. **Client.** `handleSignIn` and `handleSignUp` wait for that route. On failure, do not call Supabase.
4. **Hostname.** The widget hostnames must include the real app host (the Render hostname and any custom domain). Local dev needs `localhost` only on a separate widget or an explicit dev hostname list.
5. **Mode.** `managed` for the public login form. Use `invisible` only after managed mode is validated.

## Secrets

- The sitekey is public and may ship in the Vite client as `VITE_TURNSTILE_SITEKEY`.
- The widget secret is server-only. Store it in the Render (or local backend) environment. Never prefix it with `VITE_`, commit it, log it, or print the siteverify response body.
- Do not ask the user to paste the secret into chat. If `turnstile-spin` is available, use its secret-handling flow.

## Validation

1. Submit the real form once with a fresh token and confirm Supabase is called only after siteverify succeeds.
2. Replay that same token and confirm the backend rejects it.
3. Confirm Sign In and Sign Up each get their own token. One token cannot authorize both actions.

If the backend cannot be run, say siteverify is not yet proven. Do not claim the form is protected.

## Out of scope

- Supabase's own auth rate limits stay in place. Turnstile does not replace them.
- Room join, draw, and chat stay on the authenticated socket. Protect those with `cloudflare-waf`, not a second widget on every stroke.
- Pre-clearance cookies do not remove the need for siteverify.
