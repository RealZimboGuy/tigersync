package verify

import (
	"context"
	"testing"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"
	"tigersync/internal/apply"
	"tigersync/internal/record"
	"tigersync/internal/tbclient"
	"tigersync/internal/testcluster"
)

func TestVerifyEqualAndUnequal(t *testing.T) {
	if testing.Short() {
		t.Skip("requires docker")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
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
		{ID: tb.ToUint128(10), DebitAccountID: tb.ToUint128(1), CreditAccountID: tb.ToUint128(2), Amount: tb.ToUint128(4), Ledger: 1, Code: 1},
	})

	rep, err := Verify(sconn, dconn, 8189)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Equal {
		t.Fatalf("expected unequal before sync")
	}

	sa, _ := sconn.Raw().LookupAccounts([]tb.Uint128{tb.ToUint128(1), tb.ToUint128(2)})
	sx, _ := sconn.Raw().LookupTransfers([]tb.Uint128{tb.ToUint128(10)})
	if _, err := apply.New(dconn, 8189).Apply([]record.Record{
		record.FromAccount(sa[0]), record.FromAccount(sa[1]), record.FromTransfer(sx[0]),
	}); err != nil {
		t.Fatal(err)
	}

	rep, err = Verify(sconn, dconn, 8189)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Equal {
		t.Fatalf("expected equal after sync; mismatches: %+v", rep.Mismatches)
	}
}
