package reader

import (
	"context"
	"testing"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"
	"tigersync/internal/tbclient"
	"tigersync/internal/testcluster"
)

func TestPageAscending(t *testing.T) {
	if testing.Short() {
		t.Skip("requires docker")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cl, err := testcluster.Start(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Stop()
	conn, _ := tbclient.Connect(0, []string{cl.Address})
	defer conn.Close()

	if err := testcluster.MustCreateAccounts(conn.Raw(), []tb.Account{
		{ID: tb.ToUint128(1), Ledger: 1, Code: 1},
		{ID: tb.ToUint128(2), Ledger: 1, Code: 1},
		{ID: tb.ToUint128(3), Ledger: 1, Code: 1},
	}); err != nil {
		t.Fatal(err)
	}
	if err := testcluster.MustCreateTransfers(conn.Raw(), []tb.Transfer{
		{ID: tb.ToUint128(10), DebitAccountID: tb.ToUint128(1), CreditAccountID: tb.ToUint128(2), Amount: tb.ToUint128(5), Ledger: 1, Code: 1},
		{ID: tb.ToUint128(11), DebitAccountID: tb.ToUint128(2), CreditAccountID: tb.ToUint128(3), Amount: tb.ToUint128(5), Ledger: 1, Code: 1},
	}); err != nil {
		t.Fatal(err)
	}

	r := New(conn)
	accts, full, err := r.PageAccounts(0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(accts) != 2 || !full {
		t.Fatalf("accounts: len=%d full=%v", len(accts), full)
	}
	if accts[0].Timestamp >= accts[1].Timestamp {
		t.Fatalf("not ascending: %v", accts)
	}
	xfers, full, err := r.PageTransfers(0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(xfers) != 2 || full {
		t.Fatalf("transfers: len=%d full=%v", len(xfers), full)
	}
}
