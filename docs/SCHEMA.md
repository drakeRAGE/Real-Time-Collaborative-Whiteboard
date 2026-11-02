# Postgres schema

Postgres 16 (local) and Supabase Postgres (cloud). Services connect as
`service_role` (a role with `BYPASSRLS`). PostgREST is a public surface:
the anon key maps to `anon`, a logged-in user JWT maps to `authenticated`.
Neither may write. `authenticated` may read only rows for rooms the JWT
belongs to.

`auth.users` is owned by Supabase Auth. `0001_auth_stub` creates a minimal
stand-in only when that table is absent, so the same files apply to a
scratch `postgres:16`. The stub has no `aud` column; down uses that to
avoid dropping the real Supabase table.

`anon`, `authenticated`, and `service_role` are not created by the
migrations. `CREATE ROLE` cannot run inside a transaction block, and a bare
`CREATE ROLE` would fail on Supabase, where the roles already exist. A
scratch cluster needs them before `migrate up`, with `service_role`
`BYPASSRLS` so it matches Supabase:

```sql
CREATE ROLE anon NOLOGIN NOINHERIT;
CREATE ROLE authenticated NOLOGIN NOINHERIT;
CREATE ROLE service_role NOLOGIN NOINHERIT BYPASSRLS;
```

Down does not drop those roles.

## Tables

### profiles

One row per auth user. `id` is `auth.users.id`. `username` is 1–100
characters. `email` is copied for service-side lookups and is not granted
to `authenticated` (the client reads email from Auth).

Signup trigger `on_auth_user_created` (function `handle_new_user`,
`SECURITY DEFINER`, `search_path = public`) inserts the profile from
`raw_user_meta_data.username`, otherwise the local-part of the email,
otherwise `user`. `ON CONFLICT (id) DO NOTHING`.

### rooms

A whiteboard. `id` defaults to `gen_random_uuid()`. `title` is 1–200
characters (the room proto limit). `admin_id` references `profiles` and
is null when the room has no admin. `deleted_at` is the soft-delete flag;
rows are not removed when the last member leaves.

### room_members

Membership. Primary key `(room_id, user_id)`. `role` is `admin` or
`member`. `room_members_user_id_idx` supports "rooms for this user",
which is the RLS lookup.

### room_invites

One-time invite. `token_hash` is the SHA-256 (32 bytes) of the plaintext
token. `CreateInvite` returns the plaintext once; only the hash is stored.
`ValidateInvite` compares hashes. `max_uses` is 1–1000. `uses` cannot
exceed `max_uses`. `room_invites_token_hash_idx` is unique.

`authenticated` may select `id`, `room_id`, `expires_at`, `max_uses`, and
`uses` for rooms they belong to. `token_hash` has no grant for `anon` or
`authenticated`.

### strokes

The board op-log. One row per applied stroke, shape, clear, undo, or redo.

| Column | Meaning |
|---|---|
| `id` | ULID primary key (Crockford, 26 chars). Idempotency key. |
| `room_id` | Room the op belongs to. |
| `seq` | Gapless per-room position, starting at 1. |
| `user_id` | Principal who produced the op. Not trusted from the client payload. |
| `kind` | `stroke`, `shape`, `clear`, `undo`, or `redo`. |
| `payload` | Op body (see below). |
| `created_at` | Insert time. |

`strokes_room_seq_idx` is `UNIQUE (room_id, seq)`.

`payload` by kind:

| kind | payload |
|---|---|
| `stroke` | `{"points":[{"x","y"}], "color", "width", "tool"}` with `tool` `pen` or `eraser` |
| `shape` | `{"shape":"line"\|"rect"\|"circle", "start":{"x","y"}, "end":{"x","y"}, "color", "width"}` |
| `clear` | `{}` |
| `undo` | `{"op_id":"<target ulid>"}` |
| `redo` | `{"op_id":"<target ulid>"}` |

Whether an op is on the effective board is derived by replaying the log in
`seq` order: `undo` deactivates `op_id`, `redo` reactivates it, `clear`
deactivates prior drawing ops. There is no `active` column. `UPDATE` exists
so board-service (service role) can patch a row; `authenticated` cannot.

### board_snapshots

Compaction checkpoint. Primary key `(room_id, upto_seq)`. `payload` is the
flattened active ops with `seq <= upto_seq`, written when `seq % 1000 = 0`.
`GetHistory` reads the newest snapshot plus the op-log tail.
`upto_seq = 0` is an empty board.

### chat_messages

`id` is a ULID. `body` is `varchar(2000)`, at least 1 character.
`chat_messages_room_created_at_idx` is `(room_id, created_at)` for history
pages. Username is not stored; readers join `profiles`.

### call_sessions

A durable record that a call in a room started and, once `ended_at` is set,
ended. The live roster (who is in the call, mic, camera) stays in Redis, as
in the realtime contracts. `started_by` is the principal who opened it.

### audit_log

Append-only operator log written by room-service. `actor` is a profile id,
`action` is a short verb (`room.join`, `room.delete`, …), `target` is the
object id, `room_id` scopes the row when the action is about a room,
`metadata` is jsonb, `at` is the time. No client grant: metadata can echo
invite or membership details, so PostgREST cannot read it.

## Relationships

```
auth.users 1──1 profiles
profiles 1──* rooms.admin_id
rooms 1──* room_members *──1 profiles
rooms 1──* room_invites
rooms 1──* strokes *──1 profiles
rooms 1──* board_snapshots
rooms 1──* chat_messages *──1 profiles
rooms 1──* call_sessions *──1 profiles.started_by
profiles 1──* audit_log.actor
rooms 1──* audit_log.room_id
```

Foreign keys on `strokes.user_id`, `chat_messages.user_id`, and
`audit_log.actor` are `ON DELETE NO ACTION` so history is not dropped with
a profile. `rooms.admin_id` and `call_sessions.started_by` are
`ON DELETE SET NULL`. Membership, invites, ops, snapshots, and chat cascade
when a room row is removed. Rooms are soft-deleted in normal operation, so
that cascade is for teardown.

## Per-room seq

`seq` is assigned by board-service in the same transaction as the insert:

```sql
BEGIN;
SELECT pg_advisory_xact_lock(hashtext($room_id::text));
INSERT INTO strokes (id, room_id, seq, user_id, kind, payload)
VALUES (
  $id,
  $room_id,
  (SELECT COALESCE(MAX(seq), 0) + 1 FROM strokes WHERE room_id = $room_id),
  $user_id,
  $kind,
  $payload
)
ON CONFLICT (id) DO NOTHING;
COMMIT;
```

A Postgres sequence is the wrong tool. `nextval` is not rolled back, so a
failed transaction leaves a gap. History pages, snapshot `upto_seq`, and
compaction (`seq % 1000 = 0`) assume a gapless per-room order.

`MAX(seq) + 1` is computed in the insert statement, so a rollback drops the
number with the row. `UNIQUE (room_id, seq)` rejects two transactions that
read the same max. The advisory lock is transaction-scoped and per room
(hash collisions only add waiting, they do not pick the wrong room), so
those two transactions queue instead of unique-violating. On
`strokes_room_seq_idx` unique violation, retry the transaction.

`ON CONFLICT (id) DO NOTHING` covers JetStream redelivery of the same ULID.
The existing row keeps the seq it was assigned. Do not compute a new seq
for an id that already exists.

## RLS

Enabled and forced on every table above. Forcing means a non-superuser
table owner does not skip policies. Superusers and `BYPASSRLS` still skip
them. That is the service-role write path. Do not point board-service,
room-service, or chat-service at `anon` or `authenticated`.

`public.is_room_member(room_id)` is `SECURITY DEFINER` with
`search_path = public`. It is true when `auth.uid()` has a `room_members`
row. The definer has to be a superuser (the migration role) so the
membership read does not re-enter RLS. `authenticated` may execute it;
`anon` may not.

| Table | anon | authenticated | service_role |
|---|---|---|---|
| profiles | none | `SELECT` of `id`, `username`, `created_at` for self or a user who shares a room | all, bypass |
| rooms | none | `SELECT` if member or `admin_id` | all, bypass |
| room_members | none | `SELECT` if member of that room | all, bypass |
| room_invites | none | `SELECT` of every column except `token_hash`, if member | all, bypass |
| strokes | none | `SELECT` if member | all, bypass |
| board_snapshots | none | `SELECT` if member | all, bypass |
| chat_messages | none | `SELECT` if member | all, bypass |
| call_sessions | none | `SELECT` if member | all, bypass |
| audit_log | none | none | all, bypass |

Every table also has restrictive `INSERT`, `UPDATE`, and `DELETE` policies
with `WITH CHECK (false)` / `USING (false)` for `PUBLIC`. A role that does
not bypass RLS cannot write, including if someone later adds a permissive
write policy or grants `INSERT` to `authenticated`. `anon` has no table
grants, so the anon key cannot read or write through PostgREST.

`SELECT *` on `profiles` or `room_invites` fails for `authenticated` because
of the column grants. Clients name the granted columns. `service_role` is
unaffected.

## T2.R — strokes writes are service-role only

Direct PostgREST with the anon key uses `anon`. That role has no privileges
on `strokes` and no permissive policy. A logged-in JWT uses `authenticated`,
which has `SELECT` only, and only where `is_room_member(room_id)`.

`strokes` has no `INSERT`, `UPDATE`, or `DELETE` grant for `anon` or
`authenticated`, so those commands fail with permission denied. If a grant
is added later, `strokes_insert_service_only` (`WITH CHECK (false)`) still
rejects `INSERT`, and `strokes_update_service_only` / `strokes_delete_service_only`
(`USING (false)`) make `UPDATE` and `DELETE` match zero rows. `service_role`
has `BYPASSRLS` and `GRANT ALL`, so board-service writes succeed and the
policies are not consulted.

The same restrictive write policies sit on the other eight tables.
`strokes` is the one the draw path depends on: a client that talks to
PostgREST instead of the gateway cannot append, rewrite, or delete the op-log.
