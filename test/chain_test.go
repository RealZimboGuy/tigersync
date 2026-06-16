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

// A linked chain must sync byte-identically even when a page/batch boundary
// falls in the middle of it. We seed 2 standalone transfers followed by a
// 3-long linked chain (5 total) and run with batch-limit 4, so the first page
// cuts the chain — the engine must hold the partial chain back and apply it
// whole on the next page.
func TestLinkedChainSyncsAcrossPageBoundary(t *testing.T) {
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

	if err := testcluster.MustCreateAccounts(sconn.Raw(), []tb.Account{
		{ID: tb.ToUint128(1), Ledger: 1, Code: 1},
		{ID: tb.ToUint128(2), Ledger: 1, Code: 1},
	}); err != nil {
		t.Fatal(err)
	}

	// transfers: two standalone, then a 3-long linked chain (members 2,3 linked; 4 closes it).
	var xfers []tb.Transfer
	for i := uint64(0); i < 5; i++ {
		linked := i == 2 || i == 3
		xfers = append(xfers, tb.Transfer{
			ID: tb.ToUint128(100 + i), DebitAccountID: tb.ToUint128(1), CreditAccountID: tb.ToUint128(2),
			Amount: tb.ToUint128(1), Ledger: 1, Code: 1,
			Flags: tb.TransferFlags{Linked: linked}.ToUint16(),
		})
	}
	if err := testcluster.MustCreateTransfers(sconn.Raw(), xfers); err != nil {
		t.Fatal(err)
	}

	// batch-limit 4 forces a page boundary through the chain.
	eng := syncengine.New(reader.New(sconn), apply.New(dconn, 4),
		observ.NewLogger("error"), observ.NewCounters(), 4, 0, 50*time.Millisecond)
	runCtx, runCancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- eng.Run(runCtx, 0) }()

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if rep, err := verify.Verify(sconn, dconn, 8189); err == nil && rep.Equal && rep.TransferDest == 5 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	runCancel()
	if err := <-done; err != nil && err != context.Canceled {
		t.Fatalf("engine halted: %v", err)
	}

	rep, err := verify.Verify(sconn, dconn, 8189)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Equal || rep.TransferDest != 5 {
		t.Fatalf("linked chain not byte-identical: equal=%v transfers=%d mismatches=%+v",
			rep.Equal, rep.TransferDest, rep.Mismatches)
	}
}
