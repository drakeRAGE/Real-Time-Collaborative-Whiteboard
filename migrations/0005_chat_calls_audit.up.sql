CREATE TABLE public.chat_messages (
  id text PRIMARY KEY CHECK (id ~ '^[0-7][0-9A-HJKMNP-TV-Z]{25}$'),
  room_id uuid NOT NULL REFERENCES public.rooms (id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES public.profiles (id),
  body varchar(2000) NOT NULL CHECK (char_length(body) BETWEEN 1 AND 2000),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX chat_messages_room_created_at_idx
  ON public.chat_messages (room_id, created_at);
CREATE INDEX chat_messages_user_id_idx ON public.chat_messages (user_id);

-- Durable record that a call happened. The live roster stays in Redis.
CREATE TABLE public.call_sessions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  room_id uuid NOT NULL REFERENCES public.rooms (id) ON DELETE CASCADE,
  started_by uuid REFERENCES public.profiles (id) ON DELETE SET NULL,
  started_at timestamptz NOT NULL DEFAULT now(),
  ended_at timestamptz,
  CONSTRAINT call_sessions_ended_after_start CHECK (ended_at IS NULL OR ended_at >= started_at)
);

CREATE INDEX call_sessions_room_started_idx
  ON public.call_sessions (room_id, started_at);
CREATE INDEX call_sessions_started_by_idx ON public.call_sessions (started_by);

CREATE TABLE public.audit_log (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  actor uuid NOT NULL REFERENCES public.profiles (id),
  action text NOT NULL CHECK (char_length(action) BETWEEN 1 AND 100),
  target text NOT NULL CHECK (char_length(target) BETWEEN 1 AND 200),
  room_id uuid REFERENCES public.rooms (id) ON DELETE SET NULL,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_log_actor_idx ON public.audit_log (actor);
CREATE INDEX audit_log_room_at_idx ON public.audit_log (room_id, at);
