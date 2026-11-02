-- service_role (BYPASSRLS on Supabase, and created that way locally) is the
-- only writer. Restrictive WITH CHECK/USING (false) blocks every role that
-- does not bypass RLS, even if a later GRANT or permissive policy is added.
-- anon has no grants and no permissive policies.

GRANT USAGE ON SCHEMA public TO anon, authenticated, service_role;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO service_role;

DO $do$
DECLARE
  t text;
BEGIN
  FOREACH t IN ARRAY ARRAY[
    'profiles',
    'rooms',
    'room_members',
    'room_invites',
    'strokes',
    'board_snapshots',
    'chat_messages',
    'call_sessions',
    'audit_log'
  ]
  LOOP
    EXECUTE format('ALTER TABLE public.%I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE public.%I FORCE ROW LEVEL SECURITY', t);
    EXECUTE format('REVOKE ALL ON TABLE public.%I FROM PUBLIC, anon, authenticated', t);
    EXECUTE format('GRANT ALL ON TABLE public.%I TO service_role', t);
    EXECUTE format(
      'CREATE POLICY %I ON public.%I AS RESTRICTIVE FOR INSERT TO PUBLIC WITH CHECK (false)',
      t || '_insert_service_only', t);
    EXECUTE format(
      'CREATE POLICY %I ON public.%I AS RESTRICTIVE FOR UPDATE TO PUBLIC USING (false) WITH CHECK (false)',
      t || '_update_service_only', t);
    EXECUTE format(
      'CREATE POLICY %I ON public.%I AS RESTRICTIVE FOR DELETE TO PUBLIC USING (false)',
      t || '_delete_service_only', t);
  END LOOP;
END
$do$;

CREATE OR REPLACE FUNCTION public.is_room_member(p_room_id uuid)
RETURNS boolean
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT EXISTS (
    SELECT 1
    FROM public.room_members
    WHERE room_id = p_room_id
      AND user_id = (SELECT auth.uid())
  );
$$;

REVOKE ALL ON FUNCTION public.is_room_member(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.is_room_member(uuid) TO authenticated, service_role;

-- Email stays off the authenticated grant; the client reads it from Auth.
GRANT SELECT (id, username, created_at) ON public.profiles TO authenticated;
-- token_hash stays service-role only.
GRANT SELECT (id, room_id, expires_at, max_uses, uses) ON public.room_invites TO authenticated;

GRANT SELECT ON public.rooms TO authenticated;
GRANT SELECT ON public.room_members TO authenticated;
GRANT SELECT ON public.strokes TO authenticated;
GRANT SELECT ON public.board_snapshots TO authenticated;
GRANT SELECT ON public.chat_messages TO authenticated;
GRANT SELECT ON public.call_sessions TO authenticated;

CREATE POLICY profiles_select_visible ON public.profiles
  FOR SELECT TO authenticated
  USING (
    id = (SELECT auth.uid())
    OR EXISTS (
      SELECT 1
      FROM public.room_members mine
      JOIN public.room_members theirs ON theirs.room_id = mine.room_id
      WHERE mine.user_id = (SELECT auth.uid())
        AND theirs.user_id = profiles.id
    )
  );

CREATE POLICY rooms_select_member ON public.rooms
  FOR SELECT TO authenticated
  USING (
    public.is_room_member(id)
    OR admin_id = (SELECT auth.uid())
  );

CREATE POLICY room_members_select_member ON public.room_members
  FOR SELECT TO authenticated
  USING (public.is_room_member(room_id));

CREATE POLICY room_invites_select_member ON public.room_invites
  FOR SELECT TO authenticated
  USING (public.is_room_member(room_id));

CREATE POLICY strokes_select_member ON public.strokes
  FOR SELECT TO authenticated
  USING (public.is_room_member(room_id));

CREATE POLICY board_snapshots_select_member ON public.board_snapshots
  FOR SELECT TO authenticated
  USING (public.is_room_member(room_id));

CREATE POLICY chat_messages_select_member ON public.chat_messages
  FOR SELECT TO authenticated
  USING (public.is_room_member(room_id));

CREATE POLICY call_sessions_select_member ON public.call_sessions
  FOR SELECT TO authenticated
  USING (public.is_room_member(room_id));
