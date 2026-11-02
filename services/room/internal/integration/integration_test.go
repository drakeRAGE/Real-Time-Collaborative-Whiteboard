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
	"github.com/rs/zerolog"
	natsc "github.com/testcontainers/testcontainers-go/modules/nats"
	postgrec "github.com/testcontainers/testcontainers-go/modules/postgres"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	roomv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/room/v1"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/grpcx"
	roomgrpc "github.com/drakeRAGE/realtime-whiteboard/services/room/internal/grpc"
	"github.com/drakeRAGE/realtime-whiteboard/services/room/internal/publish"
	"github.com/drakeRAGE/realtime-whiteboard/services/room/internal/repo"
	"github.com/drakeRAGE/realtime-whiteboard/services/room/internal/service"
)

func TestRoomService(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	dsn := startPostgres(t, ctx)
	natsURL := startNATS(t, ctx)
	if err := prepareDB(ctx, dsn); err != nil {
		t.Fatal(err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	nc, err := nats.Connect(natsURL, nats.Name("room-integration"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)

	sub, err := nc.SubscribeSync("wb.room.>")
	if err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	svc := service.New(repo.New(pool), publish.New(nc))
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	gs := grpcx.NewServer(zerolog.Nop())
	roomv1.RegisterRoomServiceServer(gs, roomgrpc.New(svc, zerolog.Nop()))
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := grpcx.Dial(lis.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client := roomv1.NewRoomServiceClient(conn)

	admin := insertUser(t, ctx, pool, "ada")
	guest := insertUser(t, ctx, pool, "bea")
	third := insertUser(t, ctx, pool, "cy")

	created, err := client.CreateRoom(userCtx(admin), &roomv1.CreateRoomRequest{Title: "Studio"})
	if err != nil {
		t.Fatal(err)
	}
	roomID := created.GetRoom().GetId()
	if created.GetRoom().GetAdminId() != admin.String() {
		t.Fatalf("admin %s", created.GetRoom().GetAdminId())
	}

	joined, err := client.JoinRoom(userCtx(guest), &roomv1.JoinRoomRequest{RoomId: roomID})
	if err != nil {
		t.Fatal(err)
	}
	if joined.GetRole() != roomv1.Role_ROLE_MEMBER {
		t.Fatalf("role %s", joined.GetRole())
	}
	joinedMsg := waitMsg(t, sub, "wb.room.member_joined.v1")
	var member roomv1.Member
	if err := proto.Unmarshal(joinedMsg.Data, &member); err != nil {
		t.Fatal(err)
	}
	if member.GetUserId() != guest.String() || joinedMsg.Header.Get("Wb-Room-Id") != roomID {
		t.Fatalf("joined member %s header %s", member.GetUserId(), joinedMsg.Header.Get("Wb-Room-Id"))
	}

	membership, err := client.CheckMembership(userCtx(admin), &roomv1.CheckMembershipRequest{RoomId: roomID, UserId: guest.String()})
	if err != nil || membership.GetRole() != roomv1.Role_ROLE_MEMBER {
		t.Fatalf("membership %s %v", membership.GetRole(), err)
	}
	adminRole, err := client.CheckMembership(userCtx(guest), &roomv1.CheckMembershipRequest{RoomId: roomID, UserId: admin.String()})
	if err != nil || adminRole.GetRole() != roomv1.Role_ROLE_ADMIN {
		t.Fatalf("admin role %s %v", adminRole.GetRole(), err)
	}

	_, err = client.DeleteRoom(userCtx(guest), &roomv1.DeleteRoomRequest{RoomId: roomID})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("non-admin delete: %v", err)
	}
	if _, err := client.GetRoom(userCtx(admin), &roomv1.GetRoomRequest{RoomId: roomID}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DeleteRoom(userCtx(admin), &roomv1.DeleteRoomRequest{RoomId: roomID}); err != nil {
		t.Fatal(err)
	}
	deleted := waitMsg(t, sub, "wb.room.deleted.v1")
	if string(deleted.Data) != roomID || deleted.Header.Get("Wb-Room-Id") != roomID {
		t.Fatalf("deleted payload %q header %s", deleted.Data, deleted.Header.Get("Wb-Room-Id"))
	}
	_, err = client.GetRoom(userCtx(admin), &roomv1.GetRoomRequest{RoomId: roomID})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("get deleted: %v", err)
	}

	created, err = client.CreateRoom(userCtx(admin), &roomv1.CreateRoomRequest{Title: "Invites"})
	if err != nil {
		t.Fatal(err)
	}
	inviteRoom := created.GetRoom().GetId()
	invite, err := client.CreateInvite(userCtx(admin), &roomv1.CreateInviteRequest{
		RoomId: inviteRoom, ExpiresInSeconds: 3600, MaxUses: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	valid, err := client.ValidateInvite(userCtx(guest), &roomv1.ValidateInviteRequest{Token: invite.GetToken()})
	if err != nil || !valid.GetValid() || valid.GetRoomId() != inviteRoom {
		t.Fatalf("validate %+v %v", valid, err)
	}
	if _, err := client.JoinRoom(userCtx(third), &roomv1.JoinRoomRequest{RoomId: inviteRoom, InviteToken: invite.GetToken()}); err != nil {
		t.Fatal(err)
	}
	_ = waitMsg(t, sub, "wb.room.member_joined.v1")
	var uses int32
	if err := pool.QueryRow(ctx, `SELECT uses FROM room_invites WHERE id = $1`, invite.GetInviteId()).Scan(&uses); err != nil {
		t.Fatal(err)
	}
	if uses != 1 {
		t.Fatalf("uses %d", uses)
	}

	expired, err := client.CreateInvite(userCtx(admin), &roomv1.CreateInviteRequest{
		RoomId: inviteRoom, ExpiresInSeconds: 60, MaxUses: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE room_invites SET expires_at = now() - interval '1 minute' WHERE id = $1`, expired.GetInviteId()); err != nil {
		t.Fatal(err)
	}
	expiredView, err := client.ValidateInvite(userCtx(guest), &roomv1.ValidateInviteRequest{Token: expired.GetToken()})
	if err != nil || expiredView.GetValid() {
		t.Fatalf("expired validate %+v %v", expiredView, err)
	}
	late := insertUser(t, ctx, pool, "dee")
	_, err = client.JoinRoom(userCtx(late), &roomv1.JoinRoomRequest{RoomId: inviteRoom, InviteToken: expired.GetToken()})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expired join: %v", err)
	}

	left, err := client.LeaveRoom(userCtx(admin), &roomv1.LeaveRoomRequest{RoomId: inviteRoom})
	if err != nil {
		t.Fatal(err)
	}
	if left.GetNewAdminId() != third.String() || left.GetRoomDeleted() {
		t.Fatalf("leave %+v", left)
	}
	_ = waitMsg(t, sub, "wb.room.member_left.v1")
	after, err := client.CheckMembership(userCtx(third), &roomv1.CheckMembershipRequest{RoomId: inviteRoom, UserId: third.String()})
	if err != nil || after.GetRole() != roomv1.Role_ROLE_ADMIN {
		t.Fatalf("transferred %s %v", after.GetRole(), err)
	}

	solo, err := client.CreateRoom(userCtx(admin), &roomv1.CreateRoomRequest{Title: "Solo"})
	if err != nil {
		t.Fatal(err)
	}
	gone, err := client.LeaveRoom(userCtx(admin), &roomv1.LeaveRoomRequest{RoomId: solo.GetRoom().GetId()})
	if err != nil {
		t.Fatal(err)
	}
	if !gone.GetRoomDeleted() {
		t.Fatal("expected soft-delete")
	}
	_ = waitMsg(t, sub, "wb.room.member_left.v1")
	deletedSolo := waitMsg(t, sub, "wb.room.deleted.v1")
	if string(deletedSolo.Data) != solo.GetRoom().GetId() {
		t.Fatalf("solo deleted %q", deletedSolo.Data)
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
	ups := []string{
		"0001_auth_stub.up.sql",
		"0002_profiles.up.sql",
		"0003_rooms.up.sql",
		"0004_board.up.sql",
		"0005_chat_calls_audit.up.sql",
		"0006_rls.up.sql",
	}
	for _, name := range ups {
		body, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // name is one of the fixed migration filenames above
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
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
	var username string
	if err := pool.QueryRow(ctx, `SELECT username FROM profiles WHERE id = $1`, id).Scan(&username); err != nil {
		t.Fatal(err)
	}
	if username != name {
		t.Fatalf("username %s", username)
	}
	return id
}

func userCtx(id uuid.UUID) context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), "wb-user-id", id.String(), "wb-username", "user")
}

func waitMsg(t *testing.T, sub *nats.Subscription, subject string) *nats.Msg {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
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
