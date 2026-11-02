//go:build integration

package natsx

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/nats"
)

func TestNATSContainer(t *testing.T) {
	runSuite(t, startContainer(t))
}

func startContainer(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	ctr, err := nats.Run(ctx, "nats:2.10")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		termCtx, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		if err := ctr.Terminate(termCtx); err != nil {
			t.Errorf("terminate nats: %v", err)
		}
	})
	url, err := ctr.ConnectionString(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return url
}
