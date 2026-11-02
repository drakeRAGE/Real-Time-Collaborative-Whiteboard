-- Roles are left in place: they are Supabase-owned in cloud, and the
-- following up migration recreates them with IF NOT EXISTS locally.
-- auth.users / auth.uid() are dropped only when this is the local stub
-- (Supabase auth.users has an aud column; the stub does not).

DO $outer$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'auth'
      AND table_name = 'users'
      AND column_name = 'aud'
  ) THEN
    RETURN;
  END IF;

  DROP FUNCTION IF EXISTS auth.uid();
  DROP TABLE IF EXISTS auth.users;
  DROP SCHEMA IF EXISTS auth;
END
$outer$;
