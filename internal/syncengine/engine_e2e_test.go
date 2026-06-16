package syncengine

import (
	"context"
	"testing"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"
	"tigersync/internal/apply"
	"tigersync/internal/cursor"
	"tigersync/internal/observ"
	"tigersync/internal/reader"
	"tigersync/internal/record"
	"tigersync/internal/tbclient"
	"tigersync/internal/testcluster"
)

func TestEngineMirrorsLiveWrites(t *testing.T) {
	if testing.Short() {
		t.Skip("requires docker")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
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
		{ID: tb.ToUint128(10), DebitAccountID: tb.ToUint128(1), CreditAccountID: tb.ToUint128(2), Amount: tb.ToUint128(3), Ledger: 1, Code: 1},
	})

	start, _ := cursor.MaxTimestamp(dconn)
	eng := New(reader.New(sconn), apply.New(dconn, 8189),
		observ.NewLogger("error"), observ.NewCounters(),
		8189, 0, 50*time.Millisecond) // margin 0 so historical data syncs immediately

	runCtx, runCancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- eng.Run(runCtx, start) }()

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if got, _ := dconn.Raw().LookupTransfers([]tb.Uint128{tb.ToUint128(10)}); len(got) == 1 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	runCancel()
	<-done

	sa, _ := sconn.Raw().LookupAccounts([]tb.Uint128{tb.ToUint128(1), tb.ToUint128(2)})
	da, _ := dconn.Raw().LookupAccounts([]tb.Uint128{tb.ToUint128(1), tb.ToUint128(2)})
	for i := range sa {
		if record.ExpectedAccount(sa[i]) != da[i] {
			t.Fatalf("account %d differs:\n expected=%+v\n dst=%+v", i, record.ExpectedAccount(sa[i]), da[i])
		}
	}
}
