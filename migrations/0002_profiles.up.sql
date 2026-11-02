CREATE TABLE public.profiles (
  id uuid PRIMARY KEY REFERENCES auth.users (id) ON DELETE CASCADE,
  email text,
  username text NOT NULL CHECK (char_length(username) BETWEEN 1 AND 100),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION public.handle_new_user()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  INSERT INTO public.profiles (id, email, username)
  VALUES (
    NEW.id,
    NEW.email,
    left(
      COALESCE(
        NULLIF(btrim(NEW.raw_user_meta_data ->> 'username'), ''),
        NULLIF(split_part(COALESCE(NEW.email, ''), '@', 1), ''),
        'user'
      ),
      100
    )
  )
  ON CONFLICT (id) DO NOTHING;
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS on_auth_user_created ON auth.users;
CREATE TRIGGER on_auth_user_created
  AFTER INSERT ON auth.users
  FOR EACH ROW
  EXECUTE FUNCTION public.handle_new_user();

REVOKE ALL ON FUNCTION public.handle_new_user() FROM PUBLIC;

DO $do$
DECLARE
  r text;
BEGIN
  FOREACH r IN ARRAY ARRAY['supabase_auth_admin', 'supabase_admin', 'postgres', 'service_role']
  LOOP
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = r) THEN
      EXECUTE format('GRANT EXECUTE ON FUNCTION public.handle_new_user() TO %I', r);
    END IF;
  END LOOP;
END
$do$;
