//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
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
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	chatv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/chat/v1"
	realtimev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/realtime/v1"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/grpcx"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/natsx"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/redisx"
	chatgrpc "github.com/drakeRAGE/realtime-whiteboard/services/chat/internal/grpc"
	"github.com/drakeRAGE/realtime-whiteboard/services/chat/internal/ingest"
	"github.com/drakeRAGE/realtime-whiteboard/services/chat/internal/publish"
	"github.com/drakeRAGE/realtime-whiteboard/services/chat/internal/ratelimit"
	"github.com/drakeRAGE/realtime-whiteboard/services/chat/internal/repo"
	"github.com/drakeRAGE/realtime-whiteboard/services/chat/internal/service"
)

func TestChatService(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
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

	nc, err := nats.Connect(natsURL, nats.Name("chat-integration"))
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

	rdb := redisx.New(redisAddr)
	pub := publish.New(jsx, nc)
	svc := service.New(repo.New(pool), pub, ratelimit.New(rdb))
	go func() {
		_ = ingest.Run(ctx, nc, svc)
	}()

	rejectSub, err := nc.SubscribeSync("wb.chat.rejected.v1")
	if err != nil {
		t.Fatal(err)
	}
	sentSub, err := nc.SubscribeSync("wb.chat.message_sent.v1")
	if err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	gs := grpcx.NewServer(zerolog.Nop())
	chatv1.RegisterChatServiceServer(gs, chatgrpc.New(svc, zerolog.Nop()))
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := grpcx.Dial(lis.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client := chatv1.NewChatServiceClient(conn)

	user := insertUser(t, ctx, pool, "chat-user")
	roomID := insertRoom(t, ctx, pool, user)
	addMember(t, ctx, pool, roomID, user)

	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		msgID := ulid.Make().String()
		send := &realtimev1.ChatSend{MessageId: msgID, RoomId: roomID.String(), Body: fmt.Sprintf("msg-%d", i)}
		data, err := proto.Marshal(send)
		if err != nil {
			t.Fatal(err)
		}
		hdr := nats.Header{}
		hdr.Set("Nats-Msg-Id", msgID)
		hdr.Set("Wb-User-Id", user.String())
		hdr.Set("Wb-Username", "chat-user")
		if _, err := js.PublishMsg(ctx, &nats.Msg{Subject: "wb.chat.message.v1", Header: hdr, Data: data}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 5; i++ {
		waitSubject(t, sentSub, "wb.chat.message_sent.v1")
	}
	rejected := waitSubject(t, rejectSub, "wb.chat.rejected.v1")
	if rejected.Header.Get("Wb-Reason") != "rate_limited" {
		t.Fatalf("reason %q", rejected.Header.Get("Wb-Reason"))
	}

	if _, err := pool.Exec(ctx, `DELETE FROM chat_messages WHERE room_id = $1`, roomID); err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC()
	for i, body := range []string{"alpha", "beta", "gamma"} {
		id := ulid.Make().String()
		if _, err := pool.Exec(ctx, `INSERT INTO chat_messages (id, room_id, user_id, body, created_at) VALUES ($1,$2,$3,$4,$5)`,
			id, roomID, user, body, base.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	page, err := client.GetHistory(userCtx(user), &chatv1.GetHistoryRequest{RoomId: roomID.String(), Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.GetMessages()) != 2 || page.GetMessages()[0].GetBody() != "beta" || page.GetMessages()[1].GetBody() != "gamma" {
		t.Fatalf("first page %+v", page.GetMessages())
	}
	page2, err := client.GetHistory(userCtx(user), &chatv1.GetHistoryRequest{
		RoomId: roomID.String(), Limit: 2, Cursor: page.GetNextCursor(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.GetMessages()) != 1 || page2.GetMessages()[0].GetBody() != "alpha" {
		t.Fatalf("second page %+v", page2.GetMessages())
	}
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
	ctr, err := redisc.Run(ctx, "redis:7-alpine")
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
		"0001_auth_stub.up.sql", "0002_profiles.up.sql", "0003_rooms.up.sql",
		"0004_board.up.sql", "0005_chat_calls_audit.up.sql", "0006_rls.up.sql",
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
	if err := pool.QueryRow(ctx, `INSERT INTO rooms (admin_id, title) VALUES ($1, 'chat') RETURNING id`, admin).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func addMember(t *testing.T, ctx context.Context, pool *pgxpool.Pool, roomID, userID uuid.UUID) {
	t.Helper()
	if _, err := pool.Exec(ctx, `INSERT INTO room_members (room_id, user_id, role) VALUES ($1,$2,'member')`, roomID, userID); err != nil {
		t.Fatal(err)
	}
}

func userCtx(id uuid.UUID) context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), "wb-user-id", id.String(), "wb-username", "chat-user")
}

func waitSubject(t *testing.T, sub *nats.Subscription, subject string) *nats.Msg {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		msg, err := sub.NextMsg(time.Until(deadline))
		if err != nil {
			t.Fatalf("wait %s: %v", subject, err)
		}
		if msg.Subject == subject {
			return msg
		}
	}
	t.Fatalf("missing %s", subject)
	return nil
}
