-- Local postgres:16 has no Supabase auth schema. On Supabase the schema,
-- auth.users, and auth.uid() already exist, so the statements below do not
-- change them. Down will not drop the real Supabase table (it has an aud
-- column; this stub does not).
--
-- anon, authenticated, and service_role are NOT created here. CREATE ROLE
-- cannot run inside a transaction, and an unconditional CREATE ROLE would
-- fail on Supabase where those roles already exist. A scratch database
-- needs them first, with service_role BYPASSRLS. See docs/SCHEMA.md.

CREATE SCHEMA IF NOT EXISTS auth;

CREATE TABLE IF NOT EXISTS auth.users (
  id uuid PRIMARY KEY,
  email text,
  raw_user_meta_data jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);

-- Supabase already provides auth.uid(). Replace it only on the local stub.
DO $outer$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_proc p
    JOIN pg_namespace n ON n.oid = p.pronamespace
    WHERE n.nspname = 'auth' AND p.proname = 'uid'
  ) THEN
    EXECUTE $fn$
      CREATE FUNCTION auth.uid()
      RETURNS uuid
      LANGUAGE sql
      STABLE
      AS $body$
        SELECT NULLIF(current_setting('request.jwt.claim.sub', true), '')::uuid
      $body$
    $fn$;
  END IF;
END
$outer$;
