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

// An account with flags.history retains a balance snapshot per transfer,
// queryable via get_account_balances. Replaying the same transfers in the same
// order against a history-flagged account on the destination must reproduce the
// identical balance history, including the per-snapshot timestamps. This proves
// history replicates as a side effect of replaying transfers (we never copy the
// snapshots directly — they are not separate objects).
func TestHistoryBalancesReplicate(t *testing.T) {
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

	// account 1 has history; account 2 is a plain counterparty.
	if err := testcluster.MustCreateAccounts(sconn.Raw(), []tb.Account{
		{ID: tb.ToUint128(1), Ledger: 1, Code: 1, Flags: tb.AccountFlags{History: true}.ToUint16()},
		{ID: tb.ToUint128(2), Ledger: 1, Code: 1},
	}); err != nil {
		t.Fatal(err)
	}
	// several transfers touching account 1 to build a multi-entry history.
	var xfers []tb.Transfer
	for i := uint64(0); i < 4; i++ {
		xfers = append(xfers, tb.Transfer{
			ID: tb.ToUint128(100 + i), DebitAccountID: tb.ToUint128(1), CreditAccountID: tb.ToUint128(2),
			Amount: tb.ToUint128(10 + i), Ledger: 1, Code: 1,
		})
	}
	if err := testcluster.MustCreateTransfers(sconn.Raw(), xfers); err != nil {
		t.Fatal(err)
	}

	eng := syncengine.New(reader.New(sconn), apply.New(dconn, 8189),
		observ.NewLogger("error"), observ.NewCounters(), 8189, 0, 50*time.Millisecond)
	runCtx, runCancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- eng.Run(runCtx, 0) }()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if rep, err := verify.Verify(sconn, dconn, 8189); err == nil && rep.Equal && rep.TransferDest == 4 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	runCancel()
	<-done

	if rep, err := verify.Verify(sconn, dconn, 8189); err != nil {
		t.Fatal(err)
	} else if !rep.Equal {
		t.Fatalf("records not byte-identical: %+v", rep.Mismatches)
	}

	filter := tb.AccountFilter{AccountID: tb.ToUint128(1), Limit: 8189,
		Flags: tb.AccountFilterFlags{Debits: true, Credits: true}.ToUint32()}
	sh, err := sconn.Raw().GetAccountBalances(filter)
	if err != nil {
		t.Fatal(err)
	}
	dh, err := dconn.Raw().GetAccountBalances(filter)
	if err != nil {
		t.Fatal(err)
	}
	if len(sh) == 0 {
		t.Fatalf("expected a non-empty balance history on the source")
	}
	if len(sh) != len(dh) {
		t.Fatalf("history length differs: src=%d dst=%d", len(sh), len(dh))
	}
	for i := range sh {
		if sh[i] != dh[i] {
			t.Fatalf("history snapshot %d differs:\n src=%+v\n dst=%+v", i, sh[i], dh[i])
		}
	}
	t.Logf("balance history replicated byte-identically: %d snapshots", len(sh))
}
