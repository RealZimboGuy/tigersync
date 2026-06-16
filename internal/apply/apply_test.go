package apply

import (
	"context"
	"testing"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"
	"tigersync/internal/record"
	"tigersync/internal/tbclient"
	"tigersync/internal/testcluster"
)

func TestApplyImportsByteIdentical(t *testing.T) {
	if testing.Short() {
		t.Skip("requires docker")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	src, _ := testcluster.Start(ctx, 0)
	defer src.Stop()
	dst, _ := testcluster.Start(ctx, 0)
	defer dst.Stop()
	sconn, _ := tbclient.Connect(0, []string{src.Address})
	defer sconn.Close()
	dconn, _ := tbclient.Connect(0, []string{dst.Address})
	defer dconn.Close()

	testcluster.MustCreateAccounts(sconn.Raw(), []tb.Account{
		{ID: tb.ToUint128(1), Ledger: 1, Code: 1},
		{ID: tb.ToUint128(2), Ledger: 1, Code: 1},
	})
	testcluster.MustCreateTransfers(sconn.Raw(), []tb.Transfer{
		{ID: tb.ToUint128(10), DebitAccountID: tb.ToUint128(1), CreditAccountID: tb.ToUint128(2), Amount: tb.ToUint128(7), Ledger: 1, Code: 1},
	})
	sa, _ := sconn.Raw().LookupAccounts([]tb.Uint128{tb.ToUint128(1), tb.ToUint128(2)})
	sx, _ := sconn.Raw().LookupTransfers([]tb.Uint128{tb.ToUint128(10)})

	recs := []record.Record{record.FromAccount(sa[0]), record.FromAccount(sa[1]), record.FromTransfer(sx[0])}
	stats, err := New(dconn, 8189).Apply(recs)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if stats.Accounts != 2 || stats.Transfers != 1 {
		t.Fatalf("stats: %+v", stats)
	}

	da, _ := dconn.Raw().LookupAccounts([]tb.Uint128{tb.ToUint128(1), tb.ToUint128(2)})
	for i := range sa {
		if record.ExpectedAccount(sa[i]) != da[i] { // dst carries the persisted imported bit
			t.Fatalf("account %d differs:\n expected=%+v\n dst=%+v", i, record.ExpectedAccount(sa[i]), da[i])
		}
	}
	dx, _ := dconn.Raw().LookupTransfers([]tb.Uint128{tb.ToUint128(10)})
	if record.ExpectedTransfer(sx[0]) != dx[0] {
		t.Fatalf("transfer differs:\n expected=%+v\n dst=%+v", record.ExpectedTransfer(sx[0]), dx[0])
	}
}

func TestApplyHaltsOnTimeoutTransfer(t *testing.T) {
	recs := []record.Record{
		record.FromTransfer(tb.Transfer{ID: tb.ToUint128(1), Timeout: 60, Ledger: 1, Code: 1, Timestamp: 5}),
	}
	if _, err := New(nil, 8189).Apply(recs); err == nil { // halts before any RPC
		t.Fatalf("expected fatal error for timeout transfer")
	}
}
