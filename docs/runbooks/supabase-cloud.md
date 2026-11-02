# Supabase cloud setup

**Status: blocked.** Migrations were not applied to any cloud project.
`confirm_cost` was not called. No project was created. No keys were written.

## What is already in the account

Org `drakeRAGE's Org` (`dutgbjvnnjhysdyveifq`), plan free.

`get_cost` for a new project in that org returned **$0 / month**. That quote
was not confirmed.

| Project | Ref | Region | Status |
|---|---|---|---|
| Blures | `ruvkilwvxypqwjignbnz` | ap-south-1 | INACTIVE |
| Whiteboard | `zistrlooqiwiiokmoldp` | ap-northeast-1 | INACTIVE |

Neither project is named `realtime-whiteboard`. Both are paused. This
schema was not applied to them. `Whiteboard` may be the existing app's
database; do not treat it as the rebuild target without an explicit decision.
Restoring a paused project was not attempted.

The free plan already holds two projects. Creating a third may be rejected
even at $0.

## Why apply stopped

Creating `realtime-whiteboard` is the step the plan uses to get a database.
That step goes through `get_cost` / `confirm_cost` before `create_project`.
This run was told not to spend money and not to confirm a cost, so
`confirm_cost` and `create_project` were not called. `apply_migration` was
not called because there is no active target project.

## Remaining steps

Do these only after an explicit yes to the $0 quote (or to a newer quote if
the org plan changes).

1. Call `get_cost` (`type: project`, `organization_id: dutgbjvnnjhysdyveifq`)
   and repeat the amount. Call `confirm_cost` only after agreement.
   `create_project` name `realtime-whiteboard`, region chosen by the user,
   `confirm_cost_id` only if the API asks for it. If the free-project limit
   blocks creation, pause or delete an unused project first; do not reuse
   `Blures` or `Whiteboard` unless that is the decision.
2. Poll `get_project` until the project is healthy.
3. `apply_migration` once per file, in order, SQL from the `.up.sql` only.
   Names: `auth_stub`, `profiles`, `rooms`, `board`, `chat_calls_audit`, `rls`.
   Do not run the `.down.sql` files against cloud. `0001` is a no-op for
   roles, `auth.users`, and `auth.uid()` when those already exist. `0002`
   replaces trigger `on_auth_user_created`.
4. In the dashboard, Authentication → Providers → Email: enable email and
   password. The MCP tool list used here has no auth-provider call.
5. Record the project URL and the publishable (anon) key in `frontend/.env`.
   Do not commit that file. Record the JWKS URL
   `https://<ref>.supabase.co/auth/v1/.well-known/jwks.json` in
   `docs/CONTRACTS.md`. Never write the service-role key into the repo or
   into a runbook.
6. `list_tables` for schema `public`: every table in `docs/SCHEMA.md` has
   `rls_enabled: true`.
7. `get_advisors` for `security` and `performance`. ERROR-level findings
   stop the task. The restrictive write policies are intentional
   (`WITH CHECK (false)` for every non-bypass role); do not drop them to
   silence an advisor.

Local proof of the same SQL is the scratch `postgres:16` up / down / up
cycle described in `docs/SCHEMA.md`, not this cloud project.
