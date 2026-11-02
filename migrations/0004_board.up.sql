-- Op-log for stroke, shape, clear, undo, and redo. seq is assigned by
-- board-service inside the insert transaction (see docs/SCHEMA.md).
-- strokes_room_seq_idx is the UNIQUE(room_id, seq) index.
CREATE TABLE public.strokes (
  id text PRIMARY KEY CHECK (id ~ '^[0-7][0-9A-HJKMNP-TV-Z]{25}$'),
  room_id uuid NOT NULL REFERENCES public.rooms (id) ON DELETE CASCADE,
  seq bigint NOT NULL CHECK (seq >= 1),
  user_id uuid NOT NULL REFERENCES public.profiles (id),
  kind text NOT NULL CHECK (kind IN ('stroke', 'shape', 'clear', 'undo', 'redo')),
  payload jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX strokes_room_seq_idx ON public.strokes (room_id, seq);
CREATE INDEX strokes_user_id_idx ON public.strokes (user_id);

CREATE TABLE public.board_snapshots (
  room_id uuid NOT NULL REFERENCES public.rooms (id) ON DELETE CASCADE,
  upto_seq bigint NOT NULL CHECK (upto_seq >= 0),
  payload jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (room_id, upto_seq)
);
