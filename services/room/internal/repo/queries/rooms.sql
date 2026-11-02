-- name: CreateRoom :one
INSERT INTO rooms (admin_id, title)
VALUES ($1, $2)
RETURNING id, admin_id, title, created_at, deleted_at;

-- name: GetRoom :one
SELECT id, admin_id, title, created_at, deleted_at
FROM rooms
WHERE id = $1;

-- name: SoftDeleteRoom :execrows
UPDATE rooms
SET deleted_at = now(), admin_id = NULL
WHERE id = $1 AND deleted_at IS NULL;

-- name: UpsertMember :one
INSERT INTO room_members (room_id, user_id, role)
VALUES ($1, $2, $3)
ON CONFLICT (room_id, user_id) DO UPDATE
SET role = room_members.role
RETURNING room_id, user_id, role, joined_at;

-- name: DeleteMember :exec
DELETE FROM room_members
WHERE room_id = $1 AND user_id = $2;

-- name: ListMembers :many
SELECT m.user_id, p.username, m.role, m.joined_at
FROM room_members m
JOIN profiles p ON p.id = m.user_id
WHERE m.room_id = $1
ORDER BY m.joined_at ASC, m.user_id ASC;

-- name: GetMember :one
SELECT m.user_id, p.username, m.role, m.joined_at
FROM room_members m
JOIN profiles p ON p.id = m.user_id
WHERE m.room_id = $1 AND m.user_id = $2;

-- name: GetMemberRole :one
SELECT m.role
FROM room_members m
JOIN rooms r ON r.id = m.room_id
WHERE m.room_id = $1
  AND m.user_id = $2
  AND r.deleted_at IS NULL;

-- name: TransferAdmin :execrows
WITH promote AS (
  UPDATE room_members
  SET role = 'admin'
  WHERE room_id = $1 AND user_id = $2
  RETURNING room_id
), demote AS (
  UPDATE room_members
  SET role = 'member'
  WHERE room_id = $1
    AND user_id <> $2
    AND role = 'admin'
    AND EXISTS (SELECT 1 FROM promote)
  RETURNING room_id
)
UPDATE rooms
SET admin_id = $2
WHERE id = $1
  AND deleted_at IS NULL
  AND EXISTS (SELECT 1 FROM promote);

-- name: CreateInvite :one
INSERT INTO room_invites (room_id, token_hash, expires_at, max_uses)
VALUES ($1, $2, $3, $4)
RETURNING id, room_id, token_hash, expires_at, max_uses, uses;

-- name: GetInviteByHash :one
SELECT id, room_id, token_hash, expires_at, max_uses, uses
FROM room_invites
WHERE token_hash = $1
FOR UPDATE;

-- name: IncrementInviteUse :one
UPDATE room_invites
SET uses = uses + 1
WHERE id = $1
  AND uses < max_uses
  AND expires_at > now()
RETURNING id, room_id, token_hash, expires_at, max_uses, uses;

-- name: InsertAudit :exec
INSERT INTO audit_log (actor, action, target, room_id, metadata)
VALUES ($1, $2, $3, $4, $5);
