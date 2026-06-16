package testcluster

import (
	"context"
	"testing"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

func TestStartAndConnect(t *testing.T) {
	if testing.Short() {
		t.Skip("requires docker")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	c, err := Start(ctx, 0)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer c.Stop()

	client, err := tb.NewClient(tb.ToUint128(0), []string{c.Address})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer client.Close()

	got, err := client.LookupAccounts([]tb.Uint128{tb.ToUint128(999)})
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 accounts, got %d", len(got))
	}
}
