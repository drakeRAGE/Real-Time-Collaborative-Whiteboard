package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/proto"

	boardv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/board/v1"
	realtimev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/realtime/v1"
	"github.com/drakeRAGE/realtime-whiteboard/services/board/internal/repo"
)

const (
	snapshotEvery = 1000
	historyMax    = 500
	hotLimit      = 500

	subjectStrokeApplied = "wb.board.stroke_applied.v1"
	subjectShapeApplied  = "wb.board.shape_applied.v1"
	subjectCleared       = "wb.board.cleared.v1"
	subjectUndoApplied   = "wb.board.undo_applied.v1"
	subjectRedoApplied   = "wb.board.redo_applied.v1"
)

// Members is room-service CheckMembership. The role is "admin", "member", or "".
type Members interface {
	Role(ctx context.Context, roomID, userID uuid.UUID) (string, error)
}

// HotCache is the Redis LIST of recent encoded ops.
type HotCache interface {
	Append(ctx context.Context, roomID string, payload []byte) error
	Replace(ctx context.Context, roomID string, payloads [][]byte) error
	Exists(ctx context.Context, roomID string) (bool, error)
}

// Publisher emits one applied event. msgID is the op ULID.
type Publisher interface {
	Publish(ctx context.Context, subject, msgID string, msg proto.Message) error
}

// Service applies board ops and serves history.
type Service struct {
	store   repo.Store
	members Members
	cache   HotCache
	pub     Publisher
}

// New returns a board service.
func New(store repo.Store, members Members, cache HotCache, pub Publisher) *Service {
	return &Service{store: store, members: members, cache: cache, pub: pub}
}

// ApplyStroke appends a stroke for principal. principal is the NATS Wb-User-Id,
// never a field of the payload.
func (s *Service) ApplyStroke(ctx context.Context, principal uuid.UUID, in *realtimev1.StrokeInput) error {
	if in == nil || !ValidID(in.GetStrokeId()) {
		return ErrInvalidArgument
	}
	roomID, err := uuid.Parse(in.GetRoomId())
	if err != nil {
		return ErrInvalidArgument
	}
	if err := s.requireMember(ctx, roomID, principal); err != nil {
		return err
	}
	body, err := json.Marshal(strokeBody{
		Points: pointsFrom(in.GetPoints()),
		Color:  in.GetColor(),
		Width:  in.GetWidth(),
		Tool:   toolName(in.GetTool()),
	})
	if err != nil {
		return fmt.Errorf("stroke body: %w", err)
	}
	return s.commit(ctx, principal, roomID, in.GetStrokeId(), repo.KindStroke, body)
}

// ApplyShape appends a shape for principal.
func (s *Service) ApplyShape(ctx context.Context, principal uuid.UUID, in *realtimev1.ShapeInput) error {
	if in == nil || !ValidID(in.GetShapeId()) || in.GetStart() == nil || in.GetEnd() == nil {
		return ErrInvalidArgument
	}
	roomID, err := uuid.Parse(in.GetRoomId())
	if err != nil {
		return ErrInvalidArgument
	}
	if err := s.requireMember(ctx, roomID, principal); err != nil {
		return err
	}
	body, err := json.Marshal(shapeBody{
		Shape: shapeName(in.GetKind()),
		Start: point{X: in.GetStart().GetX(), Y: in.GetStart().GetY()},
		End:   point{X: in.GetEnd().GetX(), Y: in.GetEnd().GetY()},
		Color: in.GetColor(),
		Width: in.GetWidth(),
	})
	if err != nil {
		return fmt.Errorf("shape body: %w", err)
	}
	return s.commit(ctx, principal, roomID, in.GetShapeId(), repo.KindShape, body)
}

// ApplyClear appends a clear. Admin only.
func (s *Service) ApplyClear(ctx context.Context, principal, roomID uuid.UUID, opID string) error {
	if err := s.requireAdmin(ctx, roomID, principal); err != nil {
		return err
	}
	if !ValidID(opID) {
		return ErrInvalidArgument
	}
	return s.commit(ctx, principal, roomID, opID, repo.KindClear, []byte(`{}`))
}

// ApplyUndo marks the current undo-stack top inactive. Admin only.
func (s *Service) ApplyUndo(ctx context.Context, principal, roomID uuid.UUID, opID string) error {
	if err := s.requireAdmin(ctx, roomID, principal); err != nil {
		return err
	}
	if !ValidID(opID) {
		return ErrInvalidArgument
	}
	return s.commit(ctx, principal, roomID, opID, repo.KindUndo, nil)
}

// ApplyRedo re-activates the redo-stack top, or rejects when a later op cleared it.
func (s *Service) ApplyRedo(ctx context.Context, principal, roomID uuid.UUID, opID string) error {
	if err := s.requireAdmin(ctx, roomID, principal); err != nil {
		return err
	}
	if !ValidID(opID) {
		return ErrInvalidArgument
	}
	return s.commit(ctx, principal, roomID, opID, repo.KindRedo, nil)
}

// GetHistory returns one page of the latest snapshot plus the op-log tail.
// The caller is the gRPC principal.
func (s *Service) GetHistory(ctx context.Context, roomID uuid.UUID, cursor string, limit int32) ([]repo.Op, string, error) {
	caller, err := principal(ctx)
	if err != nil {
		return nil, "", err
	}
	if roomID == uuid.Nil {
		return nil, "", ErrInvalidArgument
	}
	if err := s.requireMember(ctx, roomID, caller); err != nil {
		return nil, "", err
	}
	if err := s.warm(ctx, roomID); err != nil {
		return nil, "", err
	}
	prefix, after, err := s.snapshotPrefix(ctx, roomID)
	if err != nil {
		return nil, "", err
	}
	tail, err := s.store.ListAfter(ctx, roomID, after)
	if err != nil {
		return nil, "", err
	}
	combined := make([]repo.Op, 0, len(prefix)+len(tail))
	combined = append(combined, prefix...)
	combined = append(combined, tail...)
	start := 0
	if cursor != "" {
		seq, convErr := strconv.ParseInt(cursor, 10, 64)
		if convErr != nil || seq < 0 {
			return nil, "", ErrInvalidArgument
		}
		for start < len(combined) && combined[start].Seq <= seq {
			start++
		}
	}
	if limit <= 0 || limit > historyMax {
		limit = historyMax
	}
	end := start + int(limit)
	if end > len(combined) {
		end = len(combined)
	}
	page := combined[start:end]
	next := ""
	if end < len(combined) && len(page) > 0 {
		next = strconv.FormatInt(page[len(page)-1].Seq, 10)
	}
	return page, next, nil
}

// GetSnapshot returns the effective board and the highest seq in the log.
func (s *Service) GetSnapshot(ctx context.Context, roomID uuid.UUID) ([]repo.Op, int64, time.Time, error) {
	caller, err := principal(ctx)
	if err != nil {
		return nil, 0, time.Time{}, err
	}
	if roomID == uuid.Nil {
		return nil, 0, time.Time{}, ErrInvalidArgument
	}
	if err := s.requireMember(ctx, roomID, caller); err != nil {
		return nil, 0, time.Time{}, err
	}
	if err := s.warm(ctx, roomID); err != nil {
		return nil, 0, time.Time{}, err
	}
	ops, err := s.store.ListRoom(ctx, roomID)
	if err != nil {
		return nil, 0, time.Time{}, err
	}
	if len(ops) == 0 {
		return nil, 0, time.Now().UTC(), nil
	}
	return effective(ops), ops[len(ops)-1].Seq, ops[len(ops)-1].CreatedAt, nil
}

func (s *Service) commit(ctx context.Context, principal, roomID uuid.UUID, opID, kind string, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if principal == uuid.Nil {
		return ErrUnauthenticated
	}
	var stored repo.Op
	var inserted bool
	err := s.store.WithRoomLock(ctx, roomID, func(q repo.Tx) error {
		existing, err := q.Get(ctx, opID)
		if err == nil {
			if existing.RoomID != roomID || existing.Kind != kind {
				return ErrInvalidArgument
			}
			stored = existing
			return nil
		}
		if !errors.Is(err, repo.ErrNotFound) {
			return err
		}
		body := payload
		if kind == repo.KindUndo || kind == repo.KindRedo {
			ops, err := q.ListRoom(ctx, roomID)
			if err != nil {
				return err
			}
			_, undo, redo := replay(ops)
			var target repo.Op
			if kind == repo.KindUndo {
				if len(undo) == 0 {
					return ErrNothingToUndo
				}
				target = undo[len(undo)-1]
			} else {
				if len(redo) == 0 {
					return ErrRedoRejected
				}
				target = redo[len(redo)-1]
			}
			body, err = json.Marshal(map[string]string{"op_id": target.ID})
			if err != nil {
				return err
			}
		}
		op, err := q.Insert(ctx, repo.Op{
			ID: opID, RoomID: roomID, UserID: principal, Kind: kind, Payload: body,
		})
		if errors.Is(err, repo.ErrDuplicate) {
			stored = op
			return nil
		}
		if err != nil {
			return err
		}
		stored = op
		inserted = true
		if op.Seq%snapshotEvery == 0 {
			all, err := q.ListRoom(ctx, roomID)
			if err != nil {
				return err
			}
			visible := effective(all)
			if visible == nil {
				visible = []repo.Op{}
			}
			raw, err := json.Marshal(visible)
			if err != nil {
				return err
			}
			if err := q.InsertSnapshot(ctx, roomID, op.Seq, raw); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	if err := s.remember(ctx, stored.RoomID, stored, inserted); err != nil {
		return err
	}
	return s.publish(ctx, stored)
}

func (s *Service) remember(ctx context.Context, roomID uuid.UUID, op repo.Op, inserted bool) error {
	if !inserted {
		return s.rebuild(ctx, roomID)
	}
	raw, err := Encode(op)
	if err != nil {
		return err
	}
	return s.cache.Append(ctx, roomID.String(), raw)
}

func (s *Service) warm(ctx context.Context, roomID uuid.UUID) error {
	ok, err := s.cache.Exists(ctx, roomID.String())
	if err != nil || ok {
		return err
	}
	return s.rebuild(ctx, roomID)
}

func (s *Service) rebuild(ctx context.Context, roomID uuid.UUID) error {
	recent, err := s.store.ListRecent(ctx, roomID, hotLimit)
	if err != nil {
		return err
	}
	payloads := make([][]byte, 0, len(recent))
	for i := len(recent) - 1; i >= 0; i-- {
		raw, err := Encode(recent[i])
		if err != nil {
			return err
		}
		payloads = append(payloads, raw)
	}
	return s.cache.Replace(ctx, roomID.String(), payloads)
}

func (s *Service) snapshotPrefix(ctx context.Context, roomID uuid.UUID) ([]repo.Op, int64, error) {
	snap, err := s.store.LatestSnapshot(ctx, roomID)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	var ops []repo.Op
	if len(snap.Payload) > 0 {
		if err := json.Unmarshal(snap.Payload, &ops); err != nil {
			return nil, 0, fmt.Errorf("snapshot payload: %w", err)
		}
	}
	return ops, snap.UptoSeq, nil
}

func (s *Service) publish(ctx context.Context, op repo.Op) error {
	msg, err := BoardProto(op)
	if err != nil {
		return err
	}
	var subject string
	var body proto.Message
	switch applied := msg.GetOp().(type) {
	case *boardv1.BoardOp_StrokeAdded:
		subject, body = subjectStrokeApplied, applied.StrokeAdded
	case *boardv1.BoardOp_ShapeAdded:
		subject, body = subjectShapeApplied, applied.ShapeAdded
	case *boardv1.BoardOp_BoardCleared:
		subject, body = subjectCleared, applied.BoardCleared
	case *boardv1.BoardOp_UndoApplied:
		subject, body = subjectUndoApplied, applied.UndoApplied
	case *boardv1.BoardOp_RedoApplied:
		subject, body = subjectRedoApplied, applied.RedoApplied
	default:
		return fmt.Errorf("publish kind %s", op.Kind)
	}
	// The ingest message and its applied event are distinct JetStream records.
	// Reusing op.ID here makes JetStream's duplicate window drop the applied
	// event, leaving gateway fan-out with no stroke to deliver.
	if err := s.pub.Publish(ctx, subject, appliedMessageID(op.ID), body); err != nil {
		return fmt.Errorf("publish applied: %w", err)
	}
	return nil
}

// appliedMessageID returns a deterministic, valid ULID in a namespace separate
// from the ingest op. JetStream deduplicates Nats-Msg-Id per stream, not subject.
func appliedMessageID(opID string) string {
	id, err := ulid.Parse(opID)
	if err != nil {
		return opID + ":applied"
	}
	digest := sha256.Sum256([]byte(opID + ":applied"))
	copy(id[6:], digest[:10])
	return id.String()
}

func (s *Service) requireMember(ctx context.Context, roomID, userID uuid.UUID) error {
	if userID == uuid.Nil {
		return ErrUnauthenticated
	}
	role, err := s.members.Role(ctx, roomID, userID)
	if err != nil {
		return err
	}
	if role != "admin" && role != "member" {
		return ErrNotMember
	}
	return nil
}

func (s *Service) requireAdmin(ctx context.Context, roomID, userID uuid.UUID) error {
	if userID == uuid.Nil {
		return ErrUnauthenticated
	}
	role, err := s.members.Role(ctx, roomID, userID)
	if err != nil {
		return err
	}
	switch role {
	case "admin":
		return nil
	case "member":
		return ErrNotAdmin
	default:
		return ErrNotMember
	}
}
