//go:build integration

package integration

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/oklog/ulid/v2"
	"github.com/rs/zerolog"
	natsc "github.com/testcontainers/testcontainers-go/modules/nats"
	postgrec "github.com/testcontainers/testcontainers-go/modules/postgres"
	redisc "github.com/testcontainers/testcontainers-go/modules/redis"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	boardv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/board/v1"
	realtimev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/realtime/v1"
	roomv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/room/v1"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/grpcx"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/natsx"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/redisx"
	boardcache "github.com/drakeRAGE/realtime-whiteboard/services/board/internal/cache"
	boardgrpc "github.com/drakeRAGE/realtime-whiteboard/services/board/internal/grpc"
	"github.com/drakeRAGE/realtime-whiteboard/services/board/internal/health"
	"github.com/drakeRAGE/realtime-whiteboard/services/board/internal/ingest"
	"github.com/drakeRAGE/realtime-whiteboard/services/board/internal/members"
	"github.com/drakeRAGE/realtime-whiteboard/services/board/internal/publish"
	"github.com/drakeRAGE/realtime-whiteboard/services/board/internal/repo"
	"github.com/drakeRAGE/realtime-whiteboard/services/board/internal/service"
)

func TestBoardService(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	dsn := startPostgres(t, ctx)
	natsURL := startNATS(t, ctx)
	redisAddr := startRedis(t, ctx)
	if err := prepareDB(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	nc, err := nats.Connect(natsURL, nats.Name("board-integration"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)
	jsx, err := natsx.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	if err := jsx.EnsureStreams(ctx); err != nil {
		t.Fatal(err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}

	roomLis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	roomSrv := grpcx.NewServer(zerolog.Nop())
	roomv1.RegisterRoomServiceServer(roomSrv, &roomStub{})
	go func() { _ = roomSrv.Serve(roomLis) }()
	t.Cleanup(roomSrv.Stop)
	roomConn, err := grpcx.Dial(roomLis.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = roomConn.Close() })

	rdb, err := redisx.New(redisAddr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rdb.Close() })
	svc := service.New(
		repo.New(pool),
		members.New(roomv1.NewRoomServiceClient(roomConn)),
		boardcache.New(rdb),
		publish.New(jsx),
	)

	boardLis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	gs := grpcx.NewServer(zerolog.Nop())
	boardv1.RegisterBoardServiceServer(gs, boardgrpc.New(svc, zerolog.Nop()))
	go func() { _ = gs.Serve(boardLis) }()
	t.Cleanup(gs.Stop)
	boardConn, err := grpcx.Dial(boardLis.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = boardConn.Close() })
	client := boardv1.NewBoardServiceClient(boardConn)

	admin := insertUser(t, ctx, pool, "ada")
	roomID := insertRoom(t, ctx, pool, admin)
	msgs := make([]*nats.Msg, 1000)
	for i := range msgs {
		msgs[i] = strokeMsg(t, roomID, admin)
	}

	runCtx, stopRun := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		err := ingest.Run(runCtx, nc, svc, zerolog.Nop())
		if err != nil && !errorsIsCanceled(err) {
			t.Errorf("ingest: %v", err)
		}
	}()
	waitConsumer(t, ctx, js)

	info, err := consumerInfo(ctx, js)
	if err != nil {
		t.Fatal(err)
	}
	gotFilters := map[string]bool{}
	for _, subject := range info.Config.FilterSubjects {
		gotFilters[subject] = true
	}
	for _, subject := range []string{
		"wb.board.stroke.v1", "wb.board.shape.v1", "wb.board.clear.v1", "wb.board.undo.v1", "wb.board.redo.v1",
	} {
		if !gotFilters[subject] {
			t.Fatalf("missing filter %s in %v", subject, info.Config.FilterSubjects)
		}
	}
	for _, subject := range info.Config.FilterSubjects {
		if strings.Contains(subject, "_applied") || strings.HasSuffix(subject, "cleared.v1") {
			t.Fatalf("consumer filter includes %s", subject)
		}
	}
	fmt.Printf("board-ingest filters=%v\n", info.Config.FilterSubjects)

	publishAll(t, ctx, js, msgs[:400])
	waitCount(t, ctx, pool, roomID, 400)
	stopRun()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	publishAll(t, ctx, js, msgs[400:])
	runCtx, stopRun = context.WithCancel(ctx)
	defer stopRun()
	go func() {
		err := ingest.Run(runCtx, nc, svc, zerolog.Nop())
		if err != nil && !errorsIsCanceled(err) {
			t.Errorf("ingest restart: %v", err)
		}
	}()
	waitCount(t, ctx, pool, roomID, 1000)
	seqs := querySeqs(t, ctx, pool, roomID)
	if len(seqs) != 1000 {
		t.Fatalf("len %d", len(seqs))
	}
	for i, seq := range seqs {
		if seq != int64(i+1) {
			t.Fatalf("seq gap at %d got %d", i, seq)
		}
	}
	fmt.Printf("restart count==%d seq=%d..%d\n", len(seqs), seqs[0], seqs[len(seqs)-1])

	var storedUser uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT user_id FROM strokes WHERE room_id=$1 AND seq=1`, roomID).Scan(&storedUser); err != nil {
		t.Fatal(err)
	}
	if storedUser != admin {
		t.Fatalf("principal %s", storedUser)
	}

	dupAck, err := js.PublishMsg(ctx, msgs[0])
	if err != nil {
		t.Fatal(err)
	}
	if dupAck == nil || !dupAck.Duplicate {
		t.Fatalf("republish ack %+v", dupAck)
	}
	time.Sleep(200 * time.Millisecond)
	if got := countStrokes(t, ctx, pool, roomID); got != 1000 {
		t.Fatalf("after repeated msg-id count %d", got)
	}
	fmt.Printf("repeated msg-id duplicate=%v count==%d\n", dupAck.Duplicate, 1000)

	var upto int64
	if err := pool.QueryRow(ctx, `SELECT upto_seq FROM board_snapshots WHERE room_id=$1`, roomID).Scan(&upto); err != nil {
		t.Fatal(err)
	}
	if upto != 1000 {
		t.Fatalf("snapshot upto %d", upto)
	}

	first := pageHistory(t, ctx, client, admin, roomID)
	second := pageHistory(t, ctx, client, admin, roomID)
	if len(first) != 1000 || len(second) != 1000 {
		t.Fatalf("pages %d %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] || first[i].seq != int64(i+1) {
			t.Fatalf("unstable page at %d %+v %+v", i, first[i], second[i])
		}
	}
	fmt.Printf("history pages stable ops=%d\n", len(first))

	snap, err := client.GetSnapshot(userCtx(admin), &boardv1.GetSnapshotRequest{RoomId: roomID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if snap.GetUptoSeq() != 1000 || len(snap.GetActiveOps()) != 1000 {
		t.Fatalf("snapshot upto %d ops %d", snap.GetUptoSeq(), len(snap.GetActiveOps()))
	}

	if err := rdb.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	afterFlush := pageHistory(t, ctx, client, admin, roomID)
	if len(afterFlush) != len(first) {
		t.Fatalf("flush len %d", len(afterFlush))
	}
	for i := range first {
		if afterFlush[i] != first[i] {
			t.Fatalf("flush mismatch at %d", i)
		}
	}
	n, err := rdb.LLen(ctx, boardcache.Key(roomID.String())).Result()
	if err != nil {
		t.Fatal(err)
	}
	if n != 500 {
		t.Fatalf("rebuilt list %d", n)
	}
	fmt.Printf("flush redis history match ops=%d list=%d\n", len(afterFlush), n)

	rec := httptest.NewRecorder()
	health.Handler(pool, pingFunc(func(c context.Context) error { return rdb.Ping(c).Err() })).ServeHTTP(
		rec, httptest.NewRequest(http.MethodGet, "/healthz", nil),
	)
	fmt.Printf("healthz %d %s\n", rec.Code, rec.Body.String())
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("health %d %s", rec.Code, rec.Body.String())
	}

	raceRoom := insertRoom(t, ctx, pool, admin)
	raceID := newID(t)
	var wg sync.WaitGroup
	wg.Add(8)
	for i := 0; i < 8; i++ {
		go func() {
			defer wg.Done()
			if err := svc.ApplyStroke(ctx, admin, &realtimev1.StrokeInput{
				StrokeId: raceID,
				RoomId:   raceRoom.String(),
				Points:   []*boardv1.Point{{X: 4, Y: 5}},
				Color:    "#445566",
				Width:    2,
				Tool:     boardv1.Tool_TOOL_PEN,
			}); err != nil {
				t.Errorf("race apply: %v", err)
			}
		}()
	}
	wg.Wait()
	var raced int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM strokes WHERE id=$1`, raceID).Scan(&raced); err != nil {
		t.Fatal(err)
	}
	if raced != 1 {
		t.Fatalf("duplicate id rows %d", raced)
	}
}

type histOp struct {
	id  string
	seq int64
}

func pageHistory(t *testing.T, ctx context.Context, client boardv1.BoardServiceClient, user, roomID uuid.UUID) []histOp {
	t.Helper()
	var out []histOp
	cursor := ""
	for {
		resp, err := client.GetHistory(userCtx(user), &boardv1.GetHistoryRequest{
			RoomId: roomID.String(), Cursor: cursor, Limit: 100,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(resp.GetOps()) == 0 && resp.GetNextCursor() != "" {
			t.Fatal("empty page with cursor")
		}
		for _, op := range resp.GetOps() {
			added := op.GetStrokeAdded()
			if added == nil || added.GetStroke() == nil {
				t.Fatalf("op %+v", op.GetOp())
			}
			out = append(out, histOp{id: added.GetStroke().GetId(), seq: added.GetSeq()})
		}
		if resp.GetNextCursor() == "" {
			break
		}
		if resp.GetNextCursor() == cursor {
			t.Fatal("cursor stuck")
		}
		cursor = resp.GetNextCursor()
	}
	return out
}

func publishAll(t *testing.T, ctx context.Context, js jetstream.JetStream, msgs []*nats.Msg) {
	t.Helper()
	var wg sync.WaitGroup
	wg.Add(4)
	chunk := (len(msgs) + 3) / 4
	for g := 0; g < 4; g++ {
		start := g * chunk
		end := start + chunk
		if start > len(msgs) {
			start = len(msgs)
		}
		if end > len(msgs) {
			end = len(msgs)
		}
		part := msgs[start:end]
		go func() {
			defer wg.Done()
			for _, msg := range part {
				if _, err := js.PublishMsg(ctx, msg); err != nil {
					t.Errorf("publish: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func strokeMsg(t *testing.T, roomID, user uuid.UUID) *nats.Msg {
	t.Helper()
	id := newID(t)
	body, err := proto.Marshal(&realtimev1.StrokeInput{
		StrokeId: id,
		RoomId:   roomID.String(),
		Points:   []*boardv1.Point{{X: 10, Y: 12}},
		Color:    "#112233",
		Width:    2,
		Tool:     boardv1.Tool_TOOL_PEN,
	})
	if err != nil {
		t.Fatal(err)
	}
	hdr := nats.Header{}
	hdr.Set("Nats-Msg-Id", id)
	hdr.Set("Wb-User-Id", user.String())
	hdr.Set("Wb-Username", "ada")
	return &nats.Msg{Subject: "wb.board.stroke.v1", Header: hdr, Data: body}
}

func newID(t *testing.T) string {
	t.Helper()
	id, err := ulid.New(ulid.Timestamp(time.Now()), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func waitConsumer(t *testing.T, ctx context.Context, js jetstream.JetStream) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := js.Consumer(ctx, "BOARD", "board-ingest"); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("board-ingest not visible")
}

func consumerInfo(ctx context.Context, js jetstream.JetStream) (*jetstream.ConsumerInfo, error) {
	cons, err := js.Consumer(ctx, "BOARD", "board-ingest")
	if err != nil {
		return nil, err
	}
	return cons.Info(ctx)
}

func waitCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, roomID uuid.UUID, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Minute)
	var n int
	for time.Now().Before(deadline) {
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM strokes WHERE room_id=$1`, roomID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == want {
			return
		}
		if n > want {
			t.Fatalf("count %d > %d", n, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("count %d want %d", n, want)
}

func countStrokes(t *testing.T, ctx context.Context, pool *pgxpool.Pool, roomID uuid.UUID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM strokes WHERE room_id=$1`, roomID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func querySeqs(t *testing.T, ctx context.Context, pool *pgxpool.Pool, roomID uuid.UUID) []int64 {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT seq FROM strokes WHERE room_id=$1 ORDER BY seq`, roomID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var seq int64
		if err := rows.Scan(&seq); err != nil {
			t.Fatal(err)
		}
		out = append(out, seq)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

type roomStub struct {
	roomv1.UnimplementedRoomServiceServer
}

func (s *roomStub) CheckMembership(ctx context.Context, req *roomv1.CheckMembershipRequest) (*roomv1.CheckMembershipResponse, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok || len(md.Get("wb-user-id")) == 0 {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}
	if md.Get("wb-user-id")[0] != req.GetUserId() {
		return nil, status.Error(codes.PermissionDenied, "principal mismatch")
	}
	return &roomv1.CheckMembershipResponse{Role: roomv1.Role_ROLE_ADMIN}, nil
}

type pingFunc func(context.Context) error

func (f pingFunc) Ping(ctx context.Context) error { return f(ctx) }

func errorsIsCanceled(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}

func startPostgres(t *testing.T, ctx context.Context) string {
	t.Helper()
	ctr, err := postgrec.Run(ctx, "postgres:16",
		postgrec.WithDatabase("whiteboard"),
		postgrec.WithUsername("postgres"),
		postgrec.WithPassword("postgres"),
		postgrec.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := ctr.Terminate(cctx); err != nil {
			t.Errorf("terminate postgres: %v", err)
		}
	})
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	return dsn
}

func startNATS(t *testing.T, ctx context.Context) string {
	t.Helper()
	ctr, err := natsc.Run(ctx, "nats:2.10")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := ctr.Terminate(cctx); err != nil {
			t.Errorf("terminate nats: %v", err)
		}
	})
	url, err := ctr.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return url
}

func startRedis(t *testing.T, ctx context.Context) string {
	t.Helper()
	ctr, err := redisc.Run(ctx, "redis:7")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := ctr.Terminate(cctx); err != nil {
			t.Errorf("terminate redis: %v", err)
		}
	})
	endpoint, err := ctr.Endpoint(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	return endpoint
}

func prepareDB(ctx context.Context, dsn string) error {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return err
	}
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	for _, stmt := range []string{
		`CREATE ROLE anon NOLOGIN NOINHERIT`,
		`CREATE ROLE authenticated NOLOGIN NOINHERIT`,
		`CREATE ROLE service_role NOLOGIN NOINHERIT BYPASSRLS`,
	} {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", stmt, err)
		}
	}
	dir := migrationsDir()
	for _, name := range []string{
		"0001_auth_stub.up.sql",
		"0002_profiles.up.sql",
		"0003_rooms.up.sql",
		"0004_board.up.sql",
		"0005_chat_calls_audit.up.sql",
		"0006_rls.up.sql",
	} {
		body, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // fixed migration filenames
		if err != nil {
			return err
		}
		if _, err := conn.Exec(ctx, string(body)); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func migrationsDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "migrations"))
}

func insertUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	meta, err := json.Marshal(map[string]string{"username": name})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO auth.users (id, email, raw_user_meta_data) VALUES ($1, $2, $3::jsonb)`, id, name+"@example.com", meta); err != nil {
		t.Fatal(err)
	}
	return id
}

func insertRoom(t *testing.T, ctx context.Context, pool *pgxpool.Pool, admin uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO rooms (admin_id, title) VALUES ($1, 'board') RETURNING id`, admin).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func userCtx(id uuid.UUID) context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), "wb-user-id", id.String(), "wb-username", "ada")
}
