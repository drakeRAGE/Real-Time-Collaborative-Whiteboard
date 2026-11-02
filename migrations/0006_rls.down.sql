DROP POLICY IF EXISTS profiles_select_visible ON public.profiles;
DROP POLICY IF EXISTS rooms_select_member ON public.rooms;
DROP POLICY IF EXISTS room_members_select_member ON public.room_members;
DROP POLICY IF EXISTS room_invites_select_member ON public.room_invites;
DROP POLICY IF EXISTS strokes_select_member ON public.strokes;
DROP POLICY IF EXISTS board_snapshots_select_member ON public.board_snapshots;
DROP POLICY IF EXISTS chat_messages_select_member ON public.chat_messages;
DROP POLICY IF EXISTS call_sessions_select_member ON public.call_sessions;

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
    EXECUTE format('DROP POLICY IF EXISTS %I ON public.%I', t || '_insert_service_only', t);
    EXECUTE format('DROP POLICY IF EXISTS %I ON public.%I', t || '_update_service_only', t);
    EXECUTE format('DROP POLICY IF EXISTS %I ON public.%I', t || '_delete_service_only', t);
  END LOOP;
END
$do$;

DROP FUNCTION IF EXISTS public.is_room_member(uuid);
