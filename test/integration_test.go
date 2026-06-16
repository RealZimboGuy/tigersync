package test

import (
	"context"
	"testing"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"
	"tigersync/internal/apply"
	"tigersync/internal/observ"
	"tigersync/internal/reader"
	"tigersync/internal/syncengine"
	"tigersync/internal/tbclient"
	"tigersync/internal/testcluster"
	"tigersync/internal/verify"
)

func TestLoadCatchUpByteIdentical(t *testing.T) {
	if testing.Short() {
		t.Skip("requires docker")
	}
	const accounts, transfers = 1_000, 50_000

	ctx, cancel := context.WithTimeout(context.Background(), 360*time.Second)
	defer cancel()
	src, _ := testcluster.Start(ctx, 0)
	defer src.Stop()
	dst, _ := testcluster.Start(ctx, 0)
	defer dst.Stop()
	sconn, _ := tbclient.Connect(0, []string{src.Address})
	defer sconn.Close()
	dconn, _ := tbclient.Connect(0, []string{dst.Address})
	defer dconn.Close()

	seedLoad(t, sconn.Raw(), accounts, transfers)

	eng := syncengine.New(reader.New(sconn), apply.New(dconn, 8189),
		observ.NewLogger("error"), observ.NewCounters(), 8189, 0, 25*time.Millisecond)
	runCtx, runCancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- eng.Run(runCtx, 0) }()

	deadline := time.Now().Add(300 * time.Second)
	var rep verify.Report
	for time.Now().Before(deadline) {
		rep, _ = verify.Verify(sconn, dconn, 8189)
		if rep.Equal && rep.TransferDest == transfers {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	runCancel()
	<-done

	if !rep.Equal || rep.TransferDest != transfers {
		t.Fatalf("did not converge: equal=%v transfers=%d/%d mismatches=%+v",
			rep.Equal, rep.TransferDest, transfers, rep.Mismatches)
	}
	t.Logf("caught up %d transfers in %s", transfers, time.Since(start))
}

func seedLoad(t *testing.T, c tb.Client, accounts, transfers int) {
	t.Helper()
	const batch = 8189
	for i := 0; i < accounts; i += batch {
		var b []tb.Account
		for j := i; j < i+batch && j < accounts; j++ {
			b = append(b, tb.Account{ID: tb.ToUint128(uint64(j + 1)), Ledger: 1, Code: 1})
		}
		if err := testcluster.MustCreateAccounts(c, b); err != nil {
			t.Fatalf("seed accounts: %v", err)
		}
	}
	id := uint64(1)
	for i := 0; i < transfers; i += batch {
		var b []tb.Transfer
		for j := i; j < i+batch && j < transfers; j++ {
			d := uint64(j%accounts) + 1
			cr := uint64((j+1)%accounts) + 1
			if d == cr {
				cr = (cr % uint64(accounts)) + 1
			}
			b = append(b, tb.Transfer{ID: tb.ToUint128(1_000_000_000 + id),
				DebitAccountID: tb.ToUint128(d), CreditAccountID: tb.ToUint128(cr),
				Amount: tb.ToUint128(1), Ledger: 1, Code: 1})
			id++
		}
		if err := testcluster.MustCreateTransfers(c, b); err != nil {
			t.Fatalf("seed transfers: %v", err)
		}
	}
}
