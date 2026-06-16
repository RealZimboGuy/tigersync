package test

import (
	"context"
	"testing"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"
	"tigersync/internal/apply"
	"tigersync/internal/record"
	"tigersync/internal/tbclient"
	"tigersync/internal/testcluster"
	"tigersync/internal/verify"
)

func TestTimeoutTransferIsFatal(t *testing.T) {
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
	sconn.Raw().CreateTransfers([]tb.Transfer{
		{ID: tb.ToUint128(10), DebitAccountID: tb.ToUint128(1), CreditAccountID: tb.ToUint128(2),
			Amount: tb.ToUint128(5), Ledger: 1, Code: 1, Timeout: 3600,
			Flags: tb.TransferFlags{Pending: true}.ToUint16()},
	})
	sa, _ := sconn.Raw().LookupAccounts([]tb.Uint128{tb.ToUint128(1), tb.ToUint128(2)})
	sx, _ := sconn.Raw().LookupTransfers([]tb.Uint128{tb.ToUint128(10)})
	recs := []record.Record{record.FromAccount(sa[0]), record.FromAccount(sa[1]), record.FromTransfer(sx[0])}

	if _, err := apply.New(dconn, 8189).Apply(recs); err == nil {
		t.Fatalf("expected fatal error on timeout transfer")
	}
}

func TestReapplyIsIdempotent(t *testing.T) {
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
		{ID: tb.ToUint128(10), DebitAccountID: tb.ToUint128(1), CreditAccountID: tb.ToUint128(2), Amount: tb.ToUint128(5), Ledger: 1, Code: 1},
	})
	sa, _ := sconn.Raw().LookupAccounts([]tb.Uint128{tb.ToUint128(1), tb.ToUint128(2)})
	sx, _ := sconn.Raw().LookupTransfers([]tb.Uint128{tb.ToUint128(10)})
	recs := []record.Record{record.FromAccount(sa[0]), record.FromAccount(sa[1]), record.FromTransfer(sx[0])}

	ap := apply.New(dconn, 8189)
	if _, err := ap.Apply(recs); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	s, err := ap.Apply(recs)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if s.Exists != 3 {
		t.Fatalf("expected 3 exists on reapply, got %+v", s)
	}
	if rep, _ := verify.Verify(sconn, dconn, 8189); !rep.Equal {
		t.Fatalf("not equal after reapply: %+v", rep.Mismatches)
	}
}
