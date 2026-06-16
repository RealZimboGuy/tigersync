package tbclient

import (
	"context"
	"testing"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"
	"tigersync/internal/testcluster"
)

func TestConnectAndClose(t *testing.T) {
	if testing.Short() {
		t.Skip("requires docker")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c, err := testcluster.Start(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Stop()

	conn, err := Connect(0, []string{c.Address})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Raw().LookupAccounts([]tb.Uint128{tb.ToUint128(1)}); err != nil {
		t.Fatalf("lookup: %v", err)
	}
}
