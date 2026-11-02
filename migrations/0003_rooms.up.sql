CREATE TABLE public.rooms (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  admin_id uuid REFERENCES public.profiles (id) ON DELETE SET NULL,
  title text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
  created_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE INDEX rooms_admin_id_idx ON public.rooms (admin_id);

CREATE TABLE public.room_members (
  room_id uuid NOT NULL REFERENCES public.rooms (id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES public.profiles (id) ON DELETE CASCADE,
  role text NOT NULL CHECK (role IN ('admin', 'member')),
  joined_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (room_id, user_id)
);

CREATE INDEX room_members_user_id_idx ON public.room_members (user_id);

-- token_hash is SHA-256 (32 bytes) of the one-time plaintext invite token.
-- The plaintext is returned once by RoomService.CreateInvite and is never stored.
CREATE TABLE public.room_invites (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  room_id uuid NOT NULL REFERENCES public.rooms (id) ON DELETE CASCADE,
  token_hash bytea NOT NULL CHECK (octet_length(token_hash) = 32),
  expires_at timestamptz NOT NULL,
  max_uses integer NOT NULL CHECK (max_uses BETWEEN 1 AND 1000),
  uses integer NOT NULL DEFAULT 0 CHECK (uses >= 0 AND uses <= max_uses)
);

CREATE UNIQUE INDEX room_invites_token_hash_idx ON public.room_invites (token_hash);
CREATE INDEX room_invites_room_id_idx ON public.room_invites (room_id);
