package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	boardv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/board/v1"
	realtimev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/realtime/v1"
	"github.com/drakeRAGE/realtime-whiteboard/services/board/internal/repo"
)

type mem struct {
	mu    sync.Mutex
	ops   []repo.Op
	snaps []repo.Snapshot
}

func (m *mem) WithRoomLock(ctx context.Context, roomID uuid.UUID, fn func(repo.Tx) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return fn(memTx{m})
}

func (m *mem) ListRoom(ctx context.Context, roomID uuid.UUID) ([]repo.Op, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return memTx{m}.listRoom(roomID), nil
}

func (m *mem) ListAfter(ctx context.Context, roomID uuid.UUID, afterSeq int64) ([]repo.Op, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []repo.Op
	for _, op := range m.ops {
		if op.RoomID == roomID && op.Seq > afterSeq {
			out = append(out, op)
		}
	}
	return out, nil
}

func (m *mem) ListRecent(ctx context.Context, roomID uuid.UUID, limit int32) ([]repo.Op, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var matched []repo.Op
	for _, op := range m.ops {
		if op.RoomID == roomID {
			matched = append(matched, op)
		}
	}
	if int(limit) < len(matched) {
		matched = matched[len(matched)-int(limit):]
	}
	out := make([]repo.Op, len(matched))
	for i := range matched {
		out[i] = matched[len(matched)-1-i]
	}
	return out, nil
}

func (m *mem) LatestSnapshot(ctx context.Context, roomID uuid.UUID) (repo.Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var best repo.Snapshot
	found := false
	for _, snap := range m.snaps {
		if snap.RoomID == roomID && (!found || snap.UptoSeq > best.UptoSeq) {
			best = snap
			found = true
		}
	}
	if !found {
		return repo.Snapshot{}, repo.ErrNotFound
	}
	return best, nil
}

func (m *mem) copyOps(roomID uuid.UUID) []repo.Op {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]repo.Op(nil), memTx{m}.listRoom(roomID)...)
}

type memTx struct{ m *mem }

func (t memTx) Get(ctx context.Context, id string) (repo.Op, error) {
	for _, op := range t.m.ops {
		if op.ID == id {
			return op, nil
		}
	}
	return repo.Op{}, repo.ErrNotFound
}

func (t memTx) ListRoom(ctx context.Context, roomID uuid.UUID) ([]repo.Op, error) {
	return t.listRoom(roomID), nil
}

func (t memTx) listRoom(roomID uuid.UUID) []repo.Op {
	var out []repo.Op
	for _, op := range t.m.ops {
		if op.RoomID == roomID {
			out = append(out, op)
		}
	}
	return out
}

func (t memTx) Insert(ctx context.Context, in repo.Op) (repo.Op, error) {
	for _, op := range t.m.ops {
		if op.ID == in.ID {
			return op, repo.ErrDuplicate
		}
	}
	var max int64
	for _, op := range t.m.ops {
		if op.RoomID == in.RoomID && op.Seq > max {
			max = op.Seq
		}
	}
	in.Seq = max + 1
	in.CreatedAt = time.Unix(in.Seq, 0).UTC()
	if len(in.Payload) == 0 {
		in.Payload = json.RawMessage(`{}`)
	}
	t.m.ops = append(t.m.ops, in)
	return in, nil
}

func (t memTx) InsertSnapshot(ctx context.Context, roomID uuid.UUID, uptoSeq int64, payload []byte) error {
	t.m.snaps = append(t.m.snaps, repo.Snapshot{
		RoomID: roomID, UptoSeq: uptoSeq, Payload: append(json.RawMessage(nil), payload...), CreatedAt: time.Now().UTC(),
	})
	return nil
}

type roles struct{ role string }

func (r roles) Role(context.Context, uuid.UUID, uuid.UUID) (string, error) { return r.role, nil }

type nopCache struct{}

func (nopCache) Append(context.Context, string, []byte) error    { return nil }
func (nopCache) Replace(context.Context, string, [][]byte) error { return nil }
func (nopCache) Exists(context.Context, string) (bool, error)    { return true, nil }

type nopPub struct{}

func (nopPub) Publish(context.Context, string, string, proto.Message) error { return nil }

type orderCache struct{ order *[]string }

func (c orderCache) Append(context.Context, string, []byte) error {
	*c.order = append(*c.order, "redis")
	return nil
}
func (c orderCache) Replace(context.Context, string, [][]byte) error { return nil }
func (c orderCache) Exists(context.Context, string) (bool, error)    { return true, nil }

type orderPub struct{ order *[]string }

func (p orderPub) Publish(context.Context, string, string, proto.Message) error {
	*p.order = append(*p.order, "publish")
	return nil
}

type orderStore struct {
	*mem
	order *[]string
}

func (s orderStore) WithRoomLock(ctx context.Context, roomID uuid.UUID, fn func(repo.Tx) error) error {
	err := s.mem.WithRoomLock(ctx, roomID, fn)
	if err == nil {
		*s.order = append(*s.order, "commit")
	}
	return err
}

func testID(t *testing.T) string {
	t.Helper()
	id, err := ulid.New(ulid.Timestamp(time.Now()), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func strokeOf(room uuid.UUID, id string) *realtimev1.StrokeInput {
	return &realtimev1.StrokeInput{
		StrokeId: id,
		RoomId:   room.String(),
		Points:   []*boardv1.Point{{X: 1, Y: 2}},
		Color:    "#112233",
		Width:    2,
		Tool:     boardv1.Tool_TOOL_PEN,
	}
}

func userCtx(id uuid.UUID) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs("wb-user-id", id.String()))
}

func opID(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var body struct {
		OpID string `json:"op_id"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	return body.OpID
}

func TestUndoMarksTheSecondStroke(t *testing.T) {
	room := uuid.New()
	user := uuid.New()
	store := &mem{}
	svc := New(store, roles{"admin"}, nopCache{}, nopPub{})
	ctx := context.Background()
	first := testID(t)
	second := testID(t)
	if err := svc.ApplyStroke(ctx, user, strokeOf(room, first)); err != nil {
		t.Fatal(err)
	}
	if err := svc.ApplyStroke(ctx, user, strokeOf(room, second)); err != nil {
		t.Fatal(err)
	}
	if err := svc.ApplyUndo(ctx, user, room, testID(t)); err != nil {
		t.Fatal(err)
	}
	ops := store.copyOps(room)
	if len(ops) != 3 || ops[2].Kind != repo.KindUndo {
		t.Fatalf("ops %+v", ops)
	}
	if got := opID(t, ops[2].Payload); got != second {
		t.Fatalf("undo marked %s, want %s", got, second)
	}
	if ops[2].UserID != user {
		t.Fatalf("principal %s", ops[2].UserID)
	}
	visible := effective(ops)
	if len(visible) != 1 || visible[0].ID != first {
		t.Fatalf("visible %+v", visible)
	}
	if err := svc.ApplyRedo(ctx, user, room, testID(t)); err != nil {
		t.Fatal(err)
	}
	visible = effective(store.copyOps(room))
	if len(visible) != 2 || visible[0].ID != first || visible[1].ID != second {
		t.Fatalf("redo visible %+v", visible)
	}
}

func TestRedoRejectedAfterNewOp(t *testing.T) {
	room := uuid.New()
	user := uuid.New()
	store := &mem{}
	svc := New(store, roles{"admin"}, nopCache{}, nopPub{})
	ctx := context.Background()
	if err := svc.ApplyStroke(ctx, user, strokeOf(room, testID(t))); err != nil {
		t.Fatal(err)
	}
	if err := svc.ApplyUndo(ctx, user, room, testID(t)); err != nil {
		t.Fatal(err)
	}
	if err := svc.ApplyStroke(ctx, user, strokeOf(room, testID(t))); err != nil {
		t.Fatal(err)
	}
	err := svc.ApplyRedo(ctx, user, room, testID(t))
	if !errors.Is(err, ErrRedoRejected) {
		t.Fatalf("redo err %v", err)
	}
	for _, op := range store.copyOps(room) {
		if op.Kind == repo.KindRedo {
			t.Fatal("redo row inserted")
		}
	}
	member := New(store, roles{"member"}, nopCache{}, nopPub{})
	if err := member.ApplyClear(ctx, user, room, testID(t)); !errors.Is(err, ErrNotAdmin) {
		t.Fatalf("member clear %v", err)
	}
	if err := member.ApplyUndo(ctx, user, room, testID(t)); !errors.Is(err, ErrNotAdmin) {
		t.Fatalf("member undo %v", err)
	}
}

func TestReplayStrokeClearUndoClear(t *testing.T) {
	room := uuid.New()
	user := uuid.New()
	store := &mem{}
	svc := New(store, roles{"admin"}, nopCache{}, nopPub{})
	ctx := context.Background()
	strokeID := testID(t)
	if err := svc.ApplyStroke(ctx, user, strokeOf(room, strokeID)); err != nil {
		t.Fatal(err)
	}
	if err := svc.ApplyClear(ctx, user, room, testID(t)); err != nil {
		t.Fatal(err)
	}
	if visible := effective(store.copyOps(room)); len(visible) != 0 {
		t.Fatalf("after clear %+v", visible)
	}
	if err := svc.ApplyUndo(ctx, user, room, testID(t)); err != nil {
		t.Fatal(err)
	}
	ops := store.copyOps(room)
	if got := opID(t, ops[len(ops)-1].Payload); got != ops[1].ID || ops[1].Kind != repo.KindClear {
		t.Fatalf("undo target %s ops %+v", got, ops)
	}
	visible := effective(ops)
	if len(visible) != 1 || visible[0].ID != strokeID {
		t.Fatalf("replay visible %+v", visible)
	}
	snap, _, _, err := svc.GetSnapshot(userCtx(user), room)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap) != 1 || snap[0].ID != strokeID {
		t.Fatalf("snapshot %+v", snap)
	}
}

func TestCommitBeforePublish(t *testing.T) {
	var order []string
	store := orderStore{mem: &mem{}, order: &order}
	svc := New(store, roles{"admin"}, orderCache{order: &order}, orderPub{order: &order})
	if err := svc.ApplyStroke(context.Background(), uuid.New(), strokeOf(uuid.New(), testID(t))); err != nil {
		t.Fatal(err)
	}
	want := []string{"commit", "redis", "publish"}
	if len(order) != len(want) {
		t.Fatalf("order %v", order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order %v", order)
		}
	}
}

func TestDuplicateStrokeID(t *testing.T) {
	room := uuid.New()
	user := uuid.New()
	store := &mem{}
	svc := New(store, roles{"admin"}, nopCache{}, nopPub{})
	id := testID(t)
	ctx := context.Background()
	if err := svc.ApplyStroke(ctx, user, strokeOf(room, id)); err != nil {
		t.Fatal(err)
	}
	if err := svc.ApplyStroke(ctx, user, strokeOf(room, id)); err != nil {
		t.Fatal(err)
	}
	if got := store.copyOps(room); len(got) != 1 || got[0].Seq != 1 {
		t.Fatalf("ops %+v", got)
	}
}
