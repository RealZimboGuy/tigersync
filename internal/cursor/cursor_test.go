package cursor

import (
	"context"
	"testing"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"
	"tigersync/internal/tbclient"
	"tigersync/internal/testcluster"
)

func TestMaxTimestampEmptyAndPopulated(t *testing.T) {
	if testing.Short() {
		t.Skip("requires docker")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cl, _ := testcluster.Start(ctx, 0)
	defer cl.Stop()
	conn, _ := tbclient.Connect(0, []string{cl.Address})
	defer conn.Close()

	got, err := MaxTimestamp(conn)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Fatalf("empty cursor = %d, want 0", got)
	}

	testcluster.MustCreateAccounts(conn.Raw(), []tb.Account{
		{ID: tb.ToUint128(1), Ledger: 1, Code: 1},
		{ID: tb.ToUint128(2), Ledger: 1, Code: 1},
	})
	testcluster.MustCreateTransfers(conn.Raw(), []tb.Transfer{
		{ID: tb.ToUint128(10), DebitAccountID: tb.ToUint128(1), CreditAccountID: tb.ToUint128(2), Amount: tb.ToUint128(1), Ledger: 1, Code: 1},
	})
	accts, _ := conn.Raw().LookupAccounts([]tb.Uint128{tb.ToUint128(1), tb.ToUint128(2)})
	xfers, _ := conn.Raw().LookupTransfers([]tb.Uint128{tb.ToUint128(10)})
	want := accts[1].Timestamp
	if xfers[0].Timestamp > want {
		want = xfers[0].Timestamp
	}

	got, err = MaxTimestamp(conn)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("cursor = %d, want %d", got, want)
	}
}
