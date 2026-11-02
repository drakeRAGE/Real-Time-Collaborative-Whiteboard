// Package repo persists the board op-log.
package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/drakeRAGE/realtime-whiteboard/services/board/internal/db"
)

const (
	KindStroke = "stroke"
	KindShape  = "shape"
	KindClear  = "clear"
	KindUndo   = "undo"
	KindRedo   = "redo"
)

var (
	// ErrNotFound is a missing row or a foreign key to a missing room or profile.
	ErrNotFound = errors.New("not found")
	// ErrDuplicate is an id that already exists. The returned op is the stored row.
	ErrDuplicate = errors.New("duplicate")
	// ErrSeqConflict is a unique violation on (room_id, seq). The caller retries.
	ErrSeqConflict = errors.New("seq conflict")
)

// Op is one strokes row.
type Op struct {
	ID        string          `json:"id"`
	RoomID    uuid.UUID       `json:"room_id"`
	Seq       int64           `json:"seq"`
	UserID    uuid.UUID       `json:"user_id"`
	Kind      string          `json:"kind"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

// Snapshot is one board_snapshots row.
type Snapshot struct {
	RoomID    uuid.UUID
	UptoSeq   int64
	Payload   json.RawMessage
	CreatedAt time.Time
}

// Store is the op-log. WithRoomLock is the only writer.
type Store interface {
	WithRoomLock(ctx context.Context, roomID uuid.UUID, fn func(Tx) error) error
	ListRoom(ctx context.Context, roomID uuid.UUID) ([]Op, error)
	ListAfter(ctx context.Context, roomID uuid.UUID, afterSeq int64) ([]Op, error)
	ListRecent(ctx context.Context, roomID uuid.UUID, limit int32) ([]Op, error)
	LatestSnapshot(ctx context.Context, roomID uuid.UUID) (Snapshot, error)
}

// Tx is the locked transaction. Insert assigns seq.
type Tx interface {
	Get(ctx context.Context, id string) (Op, error)
	ListRoom(ctx context.Context, roomID uuid.UUID) ([]Op, error)
	Insert(ctx context.Context, in Op) (Op, error)
	InsertSnapshot(ctx context.Context, roomID uuid.UUID, uptoSeq int64, payload []byte) error
}

// PGStore runs sqlc queries on a pgx pool.
type PGStore struct {
	pool *pgxpool.Pool
}

// New returns a store bound to pool.
func New(pool *pgxpool.Pool) *PGStore {
	return &PGStore{pool: pool}
}

// WithRoomLock runs fn while holding the per-room advisory lock.
// A strokes_room_seq_idx violation retries the whole transaction.
func (s *PGStore) WithRoomLock(ctx context.Context, roomID uuid.UUID, fn func(Tx) error) error {
	var err error
	for attempt := 0; attempt < 8; attempt++ {
		err = s.attempt(ctx, roomID, fn)
		if !errors.Is(err, ErrSeqConflict) {
			return err
		}
	}
	return fmt.Errorf("assign seq: %w", err)
}

func (s *PGStore) attempt(ctx context.Context, roomID uuid.UUID, fn func(Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := &pgTx{q: db.New(tx)}
	if _, err := q.q.LockRoomSeq(ctx, roomID.String()); err != nil {
		return fmt.Errorf("lock room: %w", err)
	}
	if err := fn(q); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (s *PGStore) ListRoom(ctx context.Context, roomID uuid.UUID) ([]Op, error) {
	return (&pgTx{q: db.New(s.pool)}).ListRoom(ctx, roomID)
}

func (s *PGStore) ListAfter(ctx context.Context, roomID uuid.UUID, afterSeq int64) ([]Op, error) {
	rows, err := db.New(s.pool).ListStrokesAfter(ctx, db.ListStrokesAfterParams{RoomID: roomID, Seq: afterSeq})
	if err != nil {
		return nil, fmt.Errorf("list after: %w", err)
	}
	return opsFrom(rows), nil
}

func (s *PGStore) ListRecent(ctx context.Context, roomID uuid.UUID, limit int32) ([]Op, error) {
	rows, err := db.New(s.pool).ListRecentStrokes(ctx, db.ListRecentStrokesParams{RoomID: roomID, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("list recent: %w", err)
	}
	return opsFrom(rows), nil
}

func (s *PGStore) LatestSnapshot(ctx context.Context, roomID uuid.UUID) (Snapshot, error) {
	row, err := db.New(s.pool).LatestSnapshot(ctx, roomID)
	if err != nil {
		return Snapshot{}, mapRead("latest snapshot", err)
	}
	return Snapshot{RoomID: row.RoomID, UptoSeq: row.UptoSeq, Payload: row.Payload, CreatedAt: row.CreatedAt}, nil
}

type pgTx struct {
	q *db.Queries
}

func (t *pgTx) Get(ctx context.Context, id string) (Op, error) {
	row, err := t.q.GetStroke(ctx, id)
	if err != nil {
		return Op{}, mapRead("get stroke", err)
	}
	return opFrom(row), nil
}

func (t *pgTx) ListRoom(ctx context.Context, roomID uuid.UUID) ([]Op, error) {
	rows, err := t.q.ListRoomStrokes(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("list room: %w", err)
	}
	return opsFrom(rows), nil
}

func (t *pgTx) Insert(ctx context.Context, in Op) (Op, error) {
	payload := in.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	row, err := t.q.InsertStroke(ctx, db.InsertStrokeParams{
		ID:      in.ID,
		RoomID:  in.RoomID,
		UserID:  in.UserID,
		Kind:    in.Kind,
		Payload: payload,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			existing, gerr := t.Get(ctx, in.ID)
			if gerr != nil {
				return Op{}, gerr
			}
			return existing, ErrDuplicate
		}
		return Op{}, mapWrite(err)
	}
	return opFrom(row), nil
}

func (t *pgTx) InsertSnapshot(ctx context.Context, roomID uuid.UUID, uptoSeq int64, payload []byte) error {
	if len(payload) == 0 {
		payload = []byte("[]")
	}
	if err := t.q.InsertSnapshot(ctx, db.InsertSnapshotParams{
		RoomID:  roomID,
		UptoSeq: uptoSeq,
		Payload: json.RawMessage(payload),
	}); err != nil {
		return mapWrite(err)
	}
	return nil
}

func opFrom(row db.Stroke) Op {
	return Op{
		ID: row.ID, RoomID: row.RoomID, Seq: row.Seq, UserID: row.UserID,
		Kind: row.Kind, Payload: row.Payload, CreatedAt: row.CreatedAt,
	}
}

func opsFrom(rows []db.Stroke) []Op {
	out := make([]Op, 0, len(rows))
	for _, row := range rows {
		out = append(out, opFrom(row))
	}
	return out
}

func mapRead(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return fmt.Errorf("%s: %w", op, err)
}

func mapWrite(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrSeqConflict
		case "23503":
			return ErrNotFound
		}
	}
	return err
}
