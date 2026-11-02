-- name: InsertChatMessage :execrows
INSERT INTO chat_messages (id, room_id, user_id, body)
VALUES ($1, $2, $3, $4)
ON CONFLICT (id) DO NOTHING;

-- name: GetChatMessageByID :one
SELECT
  cm.id,
  cm.room_id,
  cm.user_id,
  cm.body,
  cm.created_at,
  p.username
FROM chat_messages cm
JOIN profiles p ON p.id = cm.user_id
WHERE cm.id = $1;

-- name: ListChatHistoryLatest :many
SELECT
  cm.id,
  cm.room_id,
  cm.user_id,
  cm.body,
  cm.created_at,
  p.username
FROM chat_messages cm
JOIN profiles p ON p.id = cm.user_id
WHERE cm.room_id = $1
ORDER BY cm.created_at DESC, cm.id DESC
LIMIT $2;

-- name: ListChatHistoryBefore :many
SELECT
  cm.id,
  cm.room_id,
  cm.user_id,
  cm.body,
  cm.created_at,
  p.username
FROM chat_messages cm
JOIN profiles p ON p.id = cm.user_id
WHERE cm.room_id = $1
  AND (
    cm.created_at < $2
    OR (cm.created_at = $2 AND cm.id < $3)
  )
ORDER BY cm.created_at DESC, cm.id DESC
LIMIT $4;

-- name: IsRoomMember :one
SELECT EXISTS(
  SELECT 1
  FROM room_members
  WHERE room_id = $1 AND user_id = $2
)::bool AS is_member;
