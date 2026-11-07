package redisx_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	presencev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/presence/v1"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/redisx"
	"google.golang.org/protobuf/proto"
)

func TestRateLimitAndCache(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb, err := redisx.New(mr.Addr())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := rdb.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}

	lim := redisx.NewRateLimiter(rdb)
	for i := 1; i <= 5; i++ {
		ok, err := lim.Allow(ctx, "stroke:user", 5, time.Second)
		if err != nil || !ok {
			t.Fatalf("call %d allowed=%v err=%v", i, ok, err)
		}
	}
	ok, err := lim.Allow(ctx, "stroke:user", 5, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("6th call was allowed")
	}
	mr.FastForward(time.Second)
	ok, err = lim.Allow(ctx, "stroke:user", 5, time.Second)
	if err != nil || !ok {
		t.Fatalf("after ttl allowed=%v err=%v", ok, err)
	}
	if _, err := lim.Allow(ctx, "stroke:user", 0, time.Second); err == nil {
		t.Fatal("expected limit error")
	}

	cache := redisx.NewCache(rdb, func() *presencev1.CursorMoved { return &presencev1.CursorMoved{} })
	if _, err := cache.Get(ctx, "missing"); !errors.Is(err, redisx.ErrMiss) {
		t.Fatalf("miss: %v", err)
	}
	orig := &presencev1.CursorMoved{
		RoomId:   "room-1",
		UserId:   "user-1",
		X:        1.5,
		Y:        2.5,
		Color:    "#112233",
		Username: "ada",
	}
	want, err := proto.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Set(ctx, "cursor", orig, time.Second); err != nil {
		t.Fatal(err)
	}
	raw, err := rdb.Get(ctx, "cursor").Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, want) {
		t.Fatalf("stored bytes differ\n got %x\nwant %x", raw, want)
	}
	got, err := cache.Get(ctx, "cursor")
	if err != nil {
		t.Fatal(err)
	}
	gotBytes, err := proto.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotBytes, want) {
		t.Fatalf("roundtrip bytes differ\n got %x\nwant %x", gotBytes, want)
	}
	mr.FastForward(time.Second)
	if _, err := cache.Get(ctx, "cursor"); !errors.Is(err, redisx.ErrMiss) {
		t.Fatalf("after ttl: %v", err)
	}
}

func TestNewAddressForms(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		wantAddr string
		wantPass string
		wantDB   int
		wantTLS  bool
	}{
		{name: "host port", addr: "redis:6379", wantAddr: "redis:6379"},
		{name: "redis url", addr: "redis://:pw@cache.local:6380/2", wantAddr: "cache.local:6380", wantPass: "pw", wantDB: 2},
		{name: "rediss url", addr: "rediss://default:secret@up.example.com:6379", wantAddr: "up.example.com:6379", wantPass: "secret", wantTLS: true}, //nolint:gosec // G101: fake test credentials
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rdb, err := redisx.New(tt.addr)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = rdb.Close() })
			o := rdb.Options()
			if o.Addr != tt.wantAddr || o.Password != tt.wantPass || o.DB != tt.wantDB {
				t.Fatalf("got addr=%q pass=%q db=%d", o.Addr, o.Password, o.DB)
			}
			if (o.TLSConfig != nil) != tt.wantTLS {
				t.Fatalf("tls config set = %v, want %v", o.TLSConfig != nil, tt.wantTLS)
			}
		})
	}
}

func TestNewURLConnects(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb, err := redisx.New("redis://" + mr.Addr())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rdb.Close() })
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
}

func TestNewInvalidURL(t *testing.T) {
	if _, err := redisx.New("redis://host:6379/not-a-db"); err == nil {
		t.Fatal("want error for invalid db")
	}
}
