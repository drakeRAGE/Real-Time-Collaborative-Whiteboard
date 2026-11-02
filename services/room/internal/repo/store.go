package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/drakeRAGE/realtime-whiteboard/services/room/internal/db"
)

// PGStore runs sqlc queries on a pgx pool. CheckMembership calls GetMemberRole
// on the pool directly so that path is one indexed statement.
type PGStore struct {
	pool *pgxpool.Pool
	pgTx
}

var _ Store = (*PGStore)(nil)

// New returns a store bound to pool.
func New(pool *pgxpool.Pool) *PGStore {
	return &PGStore{pool: pool, pgTx: pgTx{q: db.New(pool)}}
}

// WithTx runs fn in one transaction and commits only when fn returns nil.
func (s *PGStore) WithTx(ctx context.Context, fn func(Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(&pgTx{q: db.New(tx)}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

type pgTx struct {
	q *db.Queries
}

func (t *pgTx) CreateRoom(ctx context.Context, adminID uuid.UUID, title string) (Room, error) {
	row, err := t.q.CreateRoom(ctx, db.CreateRoomParams{AdminID: &adminID, Title: title})
	if err != nil {
		return Room{}, mapErr("create room", err)
	}
	return roomFrom(row), nil
}

func (t *pgTx) GetRoom(ctx context.Context, id uuid.UUID) (Room, error) {
	row, err := t.q.GetRoom(ctx, id)
	if err != nil {
		return Room{}, mapErr("get room", err)
	}
	return roomFrom(row), nil
}

func (t *pgTx) SoftDeleteRoom(ctx context.Context, id uuid.UUID) error {
	n, err := t.q.SoftDeleteRoom(ctx, id)
	if err != nil {
		return mapErr("soft delete room", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (t *pgTx) UpsertMember(ctx context.Context, roomID, userID uuid.UUID, role string) (Member, error) {
	row, err := t.q.UpsertMember(ctx, db.UpsertMemberParams{RoomID: roomID, UserID: userID, Role: role})
	if err != nil {
		return Member{}, mapErr("upsert member", err)
	}
	return Member{UserID: row.UserID, Role: row.Role, JoinedAt: row.JoinedAt}, nil
}

func (t *pgTx) DeleteMember(ctx context.Context, roomID, userID uuid.UUID) error {
	if err := t.q.DeleteMember(ctx, db.DeleteMemberParams{RoomID: roomID, UserID: userID}); err != nil {
		return mapErr("delete member", err)
	}
	return nil
}

func (t *pgTx) ListMembers(ctx context.Context, roomID uuid.UUID) ([]Member, error) {
	rows, err := t.q.ListMembers(ctx, roomID)
	if err != nil {
		return nil, mapErr("list members", err)
	}
	out := make([]Member, 0, len(rows))
	for _, row := range rows {
		out = append(out, Member{UserID: row.UserID, Username: row.Username, Role: row.Role, JoinedAt: row.JoinedAt})
	}
	return out, nil
}

func (t *pgTx) GetMember(ctx context.Context, roomID, userID uuid.UUID) (Member, error) {
	row, err := t.q.GetMember(ctx, db.GetMemberParams{RoomID: roomID, UserID: userID})
	if err != nil {
		return Member{}, mapErr("get member", err)
	}
	return Member{UserID: row.UserID, Username: row.Username, Role: row.Role, JoinedAt: row.JoinedAt}, nil
}

func (t *pgTx) GetMemberRole(ctx context.Context, roomID, userID uuid.UUID) (string, error) {
	role, err := t.q.GetMemberRole(ctx, db.GetMemberRoleParams{RoomID: roomID, UserID: userID})
	if err != nil {
		return "", mapErr("get member role", err)
	}
	return role, nil
}

func (t *pgTx) TransferAdmin(ctx context.Context, roomID, newAdmin uuid.UUID) error {
	n, err := t.q.TransferAdmin(ctx, db.TransferAdminParams{ID: roomID, AdminID: &newAdmin})
	if err != nil {
		return mapErr("transfer admin", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (t *pgTx) CreateInvite(ctx context.Context, roomID uuid.UUID, hash []byte, expiresAt time.Time, maxUses int32) (Invite, error) {
	row, err := t.q.CreateInvite(ctx, db.CreateInviteParams{
		RoomID:    roomID,
		TokenHash: hash,
		ExpiresAt: expiresAt,
		MaxUses:   maxUses,
	})
	if err != nil {
		return Invite{}, mapErr("create invite", err)
	}
	return inviteFrom(row), nil
}

func (t *pgTx) GetInviteByHash(ctx context.Context, hash []byte) (Invite, error) {
	row, err := t.q.GetInviteByHash(ctx, hash)
	if err != nil {
		return Invite{}, mapErr("get invite", err)
	}
	return inviteFrom(row), nil
}

func (t *pgTx) IncrementInviteUse(ctx context.Context, id uuid.UUID) error {
	if _, err := t.q.IncrementInviteUse(ctx, id); err != nil {
		return mapErr("increment invite", err)
	}
	return nil
}

func (t *pgTx) InsertAudit(ctx context.Context, actor uuid.UUID, action, target string, roomID uuid.UUID, meta []byte) error {
	if len(meta) == 0 {
		meta = []byte(`{}`)
	}
	id := roomID
	if err := t.q.InsertAudit(ctx, db.InsertAuditParams{
		Actor:    actor,
		Action:   action,
		Target:   target,
		RoomID:   &id,
		Metadata: json.RawMessage(meta),
	}); err != nil {
		return mapErr("insert audit", err)
	}
	return nil
}

func roomFrom(row db.Room) Room {
	out := Room{ID: row.ID, Title: row.Title, CreatedAt: row.CreatedAt, Deleted: row.DeletedAt != nil}
	if row.AdminID != nil {
		out.AdminID = *row.AdminID
	}
	return out
}

func inviteFrom(row db.RoomInvite) Invite {
	return Invite{
		ID:        row.ID,
		RoomID:    row.RoomID,
		TokenHash: row.TokenHash,
		ExpiresAt: row.ExpiresAt,
		MaxUses:   row.MaxUses,
		Uses:      row.Uses,
	}
}

func mapErr(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return fmt.Errorf("%s: %w", op, err)
}
