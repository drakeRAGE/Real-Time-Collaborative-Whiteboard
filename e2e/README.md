# Browser tests

`npx playwright test` starts the Vite dev server and talks to the compose stack.

Public specs cover the signed-out login page. The four collaboration specs sign in as two local users.

There is no Supabase project in this repo. Global setup inserts `e2e-admin@example.test` and `e2e-guest@example.test` into compose Postgres the same way `load/harness` inserts load users, then the browser's Supabase password call is answered with an HS256 session the gateway already accepts (`SUPABASE_JWT_SECRET` from `deploy/compose/docker-compose.yml`). The token file stays in `e2e/.auth/` and is not committed. Drawing, chat, undo, and the forged-undo rejection then run against gateway `:8080`.

If Postgres or the gateway is down, global setup leaves the seed unwritten and those four specs skip. A skip is not a pass.

Set `E2E_EMAIL` and `E2E_PASSWORD` (and `E2E_GUEST_EMAIL` / `E2E_GUEST_PASSWORD` for the non-admin spec) to use a real Supabase project instead of the local seed. Traces are kept on failure under `e2e/test-results/`.
