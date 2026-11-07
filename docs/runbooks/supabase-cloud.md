# Supabase cloud setup

**Status: schema applied (2026-11-07).** The existing `Whiteboard` project
was reused by explicit decision. No project was created and `confirm_cost`
was not called. Dashboard steps below are still open. No keys or passwords
are written in this repo.

## Target project

Org `drakeRAGE's Org` (`dutgbjvnnjhysdyveifq`), plan free.

| Project | Ref | Region | Status |
|---|---|---|---|
| Whiteboard | `zistrlooqiwiiokmoldp` | ap-northeast-1 | ACTIVE_HEALTHY (restored from paused), Postgres 17.4.1.069 |
| Blures | `ruvkilwvxypqwjignbnz` | ap-south-1 | INACTIVE, untouched |

- API URL: `https://zistrlooqiwiiokmoldp.supabase.co`
- JWKS URL: `https://zistrlooqiwiiokmoldp.supabase.co/auth/v1/.well-known/jwks.json`
- Publishable key: the legacy `anon` JWT from `get_publishable_keys`. It goes in
  `frontend/.env` / build env as `VITE_APP_SUPABASE_ANON_KEY`, never committed.

## State before migrations

- `public`: no tables, no functions.
- `auth`: stock Supabase tables, 0 users. No triggers on `auth.users`.
- `list_migrations`: empty.

No conflict with the repo schema, so nothing was dropped.

## Migrations applied

| File | MCP name | Result |
|---|---|---|
| `0001_auth_stub.up.sql` | `auth_stub` | **Skipped.** Fails with `42501: permission denied for schema auth`: `postgres` has no CREATE on `auth`, and `CREATE TABLE IF NOT EXISTS` checks that privilege before the existence check. The file is a no-op on Supabase by design; its targets were verified present (`auth.users` with `id, email, raw_user_meta_data, created_at`, and `auth.uid()`). |
| `0002_profiles.up.sql` | `profiles` | applied |
| `0003_rooms.up.sql` | `rooms` | applied |
| `0004_board.up.sql` | `board` | applied |
| `0005_chat_calls_audit.up.sql` | `chat_calls_audit` | applied |
| `0006_rls.up.sql` | `rls` | applied |

The `.down.sql` files were not run. To make `0001` apply cleanly on cloud
too, its `CREATE SCHEMA` / `CREATE TABLE` would need the same existence
guard already used for `auth.uid()`. That repo change has not been made.

`list_tables public` after: `profiles`, `rooms`, `room_members`,
`room_invites`, `strokes`, `board_snapshots`, `chat_messages`,
`call_sessions`, `audit_log`, all with `rls_enabled: true`.

## Advisors

No ERROR-level findings.

Security (WARN):

- `0027 pg_graphql_authenticated_table_exposed` for the 8 tables that grant
  `SELECT` to `authenticated`. This is intentional: rows are still filtered by
  the member-only RLS policies.
- `0028` / `0029` SECURITY DEFINER functions executable by `anon` and
  `authenticated`: `handle_new_user()` and `is_room_member(uuid)`. Supabase
  default privileges grant EXECUTE to `anon` / `authenticated` directly, so
  `0006`'s `REVOKE ... FROM PUBLIC` does not remove those grants. Impact is low:
  a trigger function cannot be called through RPC, and `is_room_member`
  returns false when there is no `auth.uid()`. Not changed here. A follow-up
  migration could revoke EXECUTE from `anon` (and from `authenticated` on
  `handle_new_user`).
- Leaked password protection disabled; few MFA options; Postgres
  17.4.1.069 has security patches available (upgrade from the dashboard).

Performance (INFO only): 10 `unused_index` findings, expected on empty tables.

The restrictive write policies (`WITH CHECK (false)` for every non-bypass
role) were left as they are.

## Remaining dashboard steps (manual)

The MCP has no auth-config or signing-key calls.

1. **Authentication → Sign In / Providers → Email:** enable Email with
   password. Set the Site URL and Redirect URLs once the frontend origin is
   known.
2. **Project Settings → JWT Keys (JWT Signing Keys):** migrate to asymmetric
   signing keys (ECC P-256 or RSA), then rotate so the new key signs tokens.
   On 2026-11-07 the JWKS endpoint returned `{"keys":[]}`, so the gateway
   cannot verify via `SUPABASE_JWKS_URL` until this is done. The fallback,
   `SUPABASE_JWT_ALLOW_HS256`, is weaker.
3. **`DATABASE_URL`:** use the top-bar **Connect** button → Connection
   string → **Session pooler** (Supavisor, port 5432, user
   `postgres.zistrlooqiwiiokmoldp`). The services open `pgxpool.New` with
   pgx's default prepared-statement cache, which breaks behind the
   **Transaction pooler** (port 6543) unless the URL adds
   `default_query_exec_mode=simple_protocol`. The direct
   `db.<ref>.supabase.co` host is IPv6-only on the free plan. Add
   `sslmode=require`. Put the value only in the deploy platform's env vars.
   The database password is under Project Settings → Database (reset it
   there if it is unknown).
4. Optional: enable leaked password protection and upgrade Postgres.

Local proof of the same SQL is the scratch `postgres:16` up / down / up
cycle described in `docs/SCHEMA.md`.
