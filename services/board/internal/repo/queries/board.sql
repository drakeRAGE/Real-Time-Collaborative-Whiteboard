-- name: LockRoomSeq :one
SELECT 1::int AS locked
FROM (SELECT pg_advisory_xact_lock(hashtext(sqlc.arg(room_id)::text))) AS acquired;

-- name: GetStroke :one
SELECT id, room_id, seq, user_id, kind, payload, created_at
FROM strokes
WHERE id = $1;

-- name: InsertStroke :one
INSERT INTO strokes (id, room_id, seq, user_id, kind, payload)
VALUES (
  $1,
  $2,
  (SELECT COALESCE(MAX(seq), 0) + 1 FROM strokes WHERE room_id = $2),
  $3,
  $4,
  $5
)
ON CONFLICT (id) DO NOTHING
RETURNING id, room_id, seq, user_id, kind, payload, created_at;

-- name: ListRoomStrokes :many
SELECT id, room_id, seq, user_id, kind, payload, created_at
FROM strokes
WHERE room_id = $1
ORDER BY seq ASC;

-- name: ListStrokesAfter :many
SELECT id, room_id, seq, user_id, kind, payload, created_at
FROM strokes
WHERE room_id = $1 AND seq > $2
ORDER BY seq ASC;

-- name: ListRecentStrokes :many
SELECT id, room_id, seq, user_id, kind, payload, created_at
FROM strokes
WHERE room_id = $1
ORDER BY seq DESC
LIMIT $2;

-- name: LatestSnapshot :one
SELECT room_id, upto_seq, payload, created_at
FROM board_snapshots
WHERE room_id = $1
ORDER BY upto_seq DESC
LIMIT 1;

-- name: InsertSnapshot :exec
INSERT INTO board_snapshots (room_id, upto_seq, payload)
VALUES ($1, $2, $3)
ON CONFLICT (room_id, upto_seq) DO NOTHING;
