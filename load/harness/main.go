// Command harness prepares local load-test users and drives the board the
// same way services/board/internal/integration publishes strokes: JetStream
// wb.board.stroke.v1 with Nats-Msg-Id and Wb-User-Id, then BoardService.GetHistory.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/oklog/ulid/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	boardv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/board/v1"
	realtimev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/realtime/v1"
)

type config struct {
	secret   string
	issuer   string
	audience string
	database string
	gateway  string
	natsURL  string
	board    string
}

func main() {
	mode := flag.String("mode", "", "prep, publish, or count")
	users := flag.Int("users", 20, "prep: number of distinct users")
	perRoom := flag.Int("per-room", 10, "prep: users in each room")
	out := flag.String("out", "", "prep: plan JSON path")
	strokes := flag.Int("strokes", 40, "publish: stroke count")
	gap := flag.Duration("gap", 150*time.Millisecond, "publish: delay between strokes")
	status := flag.String("status", "", "publish: status JSON path")
	roomID := flag.String("room", "", "count: room id")
	userID := flag.String("user", "", "count: user id")
	expect := flag.Int("expect", -1, "count: required GetHistory op count, or -1 to print once")
	wait := flag.Duration("wait", 60*time.Second, "count: how long to wait for -expect")
	flag.Parse()

	secret := os.Getenv("HARNESS_JWT_SECRET")
	if secret == "" {
		if path := strings.TrimSpace(os.Getenv("HARNESS_JWT_SECRET_FILE")); path != "" {
			raw, err := os.ReadFile(path)
			if err != nil {
				fail("read HARNESS_JWT_SECRET_FILE: %s", err)
			}
			secret = strings.TrimSpace(string(raw))
		}
	}
	cfg := config{
		secret:   secret,
		issuer:   envOr("HARNESS_JWT_ISSUER", "http://127.0.0.1:54321/auth/v1"),
		audience: envOr("HARNESS_JWT_AUDIENCE", "authenticated"),
		database: envOr("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/whiteboard?sslmode=disable"),
		gateway:  strings.TrimRight(envOr("GATEWAY_URL", "http://localhost:8080"), "/"),
		natsURL:  envOr("NATS_URL", "nats://localhost:4222"),
		board:    envOr("BOARD_GRPC", "localhost:7002"),
	}
	if cfg.secret == "" {
		fail("HARNESS_JWT_SECRET is unset. Set it to the compose gateway SUPABASE_JWT_SECRET. Refusing to mint tokens.")
	}

	ctx := context.Background()
	switch *mode {
	case "prep":
		if *out == "" || *users < 1 || *perRoom < 1 || *users%*perRoom != 0 {
			fail("prep needs -out, -users, and -per-room that divides users")
		}
		if err := prep(ctx, cfg, *users, *perRoom, *out); err != nil {
			fail("%s", err)
		}
	case "publish":
		if *status == "" || *strokes < 1 {
			fail("publish needs -status and -strokes")
		}
		if err := publish(ctx, cfg, *strokes, *gap, *status); err != nil {
			fail("%s", err)
		}
	case "count":
		if *roomID == "" || *userID == "" {
			fail("count needs -room and -user")
		}
		n, err := waitCount(ctx, cfg, *roomID, *userID, *expect, *wait)
		if err != nil {
			fail("%s", err)
		}
		fmt.Println(n)
		if *expect >= 0 && n != *expect {
			os.Exit(1)
		}
	default:
		fail("mode must be prep, publish, or count")
	}
}

func prep(ctx context.Context, cfg config, users, perRoom int, outPath string) error {
	conn, err := pgx.Connect(ctx, cfg.database)
	if err != nil {
		return fmt.Errorf("postgres connect: %w", err)
	}
	defer conn.Close(ctx)

	type slot struct {
		Token  string `json:"token"`
		RoomID string `json:"roomId"`
		UserID string `json:"userId"`
	}
	slots := make([]slot, 0, users)
	rooms := users / perRoom
	for i := 0; i < users; i++ {
		id := uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012x", i+1))
		email := fmt.Sprintf("load%04d@example.test", i+1)
		if err := ensureUser(ctx, conn, id, email); err != nil {
			return err
		}
		token, err := mint(cfg, id.String(), email)
		if err != nil {
			return err
		}
		slots = append(slots, slot{Token: token, UserID: id.String()})
	}
	for r := 0; r < rooms; r++ {
		admin := slots[r*perRoom]
		room, err := createRoom(cfg.gateway, admin.Token)
		if err != nil {
			return fmt.Errorf("create room %d: %w", r, err)
		}
		slots[r*perRoom].RoomID = room
		for m := 1; m < perRoom; m++ {
			idx := r*perRoom + m
			if err := joinRoom(cfg.gateway, slots[idx].Token, room); err != nil {
				return fmt.Errorf("join room %d user %d: %w", r, m, err)
			}
			slots[idx].RoomID = room
		}
	}
	body, err := json.Marshal(map[string]any{
		"ws":  strings.Replace(cfg.gateway, "http", "ws", 1) + "/ws",
		"vus": slots,
	})
	if err != nil {
		return err
	}
	if err := os.WriteFile(outPath, body, 0o600); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "prepared %d users across %d rooms\n", users, rooms)
	return nil
}

func publish(ctx context.Context, cfg config, n int, gap time.Duration, statusPath string) error {
	conn, err := pgx.Connect(ctx, cfg.database)
	if err != nil {
		return fmt.Errorf("postgres connect: %w", err)
	}
	defer conn.Close(ctx)

	id := uuid.New()
	email := "chaos-" + id.String() + "@example.test"
	if err := ensureUser(ctx, conn, id, email); err != nil {
		return err
	}
	token, err := mint(cfg, id.String(), email)
	if err != nil {
		return err
	}
	room, err := createRoom(cfg.gateway, token)
	if err != nil {
		return err
	}
	if err := writeStatus(statusPath, map[string]any{
		"phase": "publishing", "roomId": room, "userId": id.String(), "strokes": n,
	}); err != nil {
		return err
	}

	nc, err := nats.Connect(cfg.natsURL)
	if err != nil {
		return fmt.Errorf("nats connect: %w", err)
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		return err
	}
	for i := 0; i < n; i++ {
		if err := publishStroke(ctx, js, room, id); err != nil {
			return fmt.Errorf("stroke %d: %w", i, err)
		}
		if i+1 < n {
			time.Sleep(gap)
		}
	}
	return writeStatus(statusPath, map[string]any{
		"phase": "done", "roomId": room, "userId": id.String(), "strokes": n,
	})
}

func publishStroke(ctx context.Context, js jetstream.JetStream, room string, user uuid.UUID) error {
	strokeID, err := ulid.New(ulid.Timestamp(time.Now()), rand.Reader)
	if err != nil {
		return err
	}
	body, err := proto.Marshal(&realtimev1.StrokeInput{
		StrokeId: strokeID.String(),
		RoomId:   room,
		Points:   []*boardv1.Point{{X: 10, Y: 12}},
		Color:    "#112233",
		Width:    2,
		Tool:     boardv1.Tool_TOOL_PEN,
	})
	if err != nil {
		return err
	}
	hdr := nats.Header{}
	hdr.Set("Nats-Msg-Id", strokeID.String())
	hdr.Set("Wb-User-Id", user.String())
	hdr.Set("Wb-Username", "chaos")
	_, err = js.PublishMsg(ctx, &nats.Msg{Subject: "wb.board.stroke.v1", Header: hdr, Data: body})
	return err
}

func waitCount(ctx context.Context, cfg config, room, user string, expect int, wait time.Duration) (int, error) {
	conn, err := grpc.NewClient(cfg.board, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	client := boardv1.NewBoardServiceClient(conn)
	deadline := time.Now().Add(wait)
	var last error
	for {
		n, err := historyCount(ctx, client, room, user)
		if err == nil && (expect < 0 || n == expect || time.Now().After(deadline)) {
			return n, nil
		}
		last = err
		if time.Now().After(deadline) {
			if err != nil {
				return 0, fmt.Errorf("get history: %w", err)
			}
			return n, nil
		}
		if last != nil {
			fmt.Fprintf(os.Stderr, "get history retry: %v\n", last)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func historyCount(ctx context.Context, client boardv1.BoardServiceClient, room, user string) (int, error) {
	total := 0
	cursor := ""
	for {
		call, cancel := context.WithTimeout(ctx, 3*time.Second)
		call = metadata.AppendToOutgoingContext(call, "wb-user-id", user)
		resp, err := client.GetHistory(call, &boardv1.GetHistoryRequest{RoomId: room, Cursor: cursor, Limit: 500})
		cancel()
		if err != nil {
			return 0, err
		}
		total += len(resp.GetOps())
		next := resp.GetNextCursor()
		if next == "" || next == cursor {
			return total, nil
		}
		cursor = next
	}
}

func ensureUser(ctx context.Context, conn *pgx.Conn, id uuid.UUID, email string) error {
	if _, err := conn.Exec(ctx, `INSERT INTO auth.users (id, email) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`, id, email); err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	var name string
	if err := conn.QueryRow(ctx, `SELECT username FROM profiles WHERE id = $1`, id).Scan(&name); err != nil {
		return fmt.Errorf("profile missing after insert: %w", err)
	}
	return nil
}

func createRoom(gateway, token string) (string, error) {
	req, err := http.NewRequest(http.MethodPost, gateway+"/api/v1/rooms", strings.NewReader(`{"title":"load"}`))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("create room status %d: %s", res.StatusCode, trim(raw))
	}
	var body struct {
		Room struct {
			ID string `json:"id"`
		} `json:"room"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return "", err
	}
	if _, err := uuid.Parse(body.Room.ID); err != nil {
		return "", fmt.Errorf("create room response missing id")
	}
	return body.Room.ID, nil
}

func joinRoom(gateway, token, room string) error {
	req, err := http.NewRequest(http.MethodPost, gateway+"/api/v1/rooms/"+room+"/join", strings.NewReader(`{}`))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		return fmt.Errorf("join status %d: %s", res.StatusCode, trim(raw))
	}
	return nil
}

func mint(cfg config, sub, email string) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, err := json.Marshal(map[string]any{
		"iss":   cfg.issuer,
		"aud":   cfg.audience,
		"sub":   sub,
		"email": email,
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(2 * time.Hour).Unix(),
	})
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(cfg.secret))
	_, _ = mac.Write([]byte(header + "." + body))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return header + "." + body + "." + sig, nil
}

func writeStatus(path string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func trim(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		return s[:300]
	}
	return s
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
