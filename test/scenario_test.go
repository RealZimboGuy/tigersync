package test

import (
	"context"
	"testing"
	"time"

	"tigersync/internal/apply"
	"tigersync/internal/observ"
	"tigersync/internal/reader"
	"tigersync/internal/syncengine"
	"tigersync/internal/tbclient"
	"tigersync/internal/testcluster"
	"tigersync/internal/verify"
)

func TestScenarioMatrixByteIdentical(t *testing.T) {
	if testing.Short() {
		t.Skip("requires docker")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()
	src, _ := testcluster.Start(ctx, 0)
	defer src.Stop()
	dst, _ := testcluster.Start(ctx, 0)
	defer dst.Stop()
	sconn, _ := tbclient.Connect(0, []string{src.Address})
	defer sconn.Close()
	dconn, _ := tbclient.Connect(0, []string{dst.Address})
	defer dconn.Close()

	if err := testcluster.SeedAllScenarios(sconn.Raw()); err != nil {
		t.Fatalf("seed: %v", err)
	}

	eng := syncengine.New(reader.New(sconn), apply.New(dconn, 8189),
		observ.NewLogger("error"), observ.NewCounters(), 8189, 0, 50*time.Millisecond)
	runCtx, runCancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- eng.Run(runCtx, 0) }()

	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		if rep, err := verify.Verify(sconn, dconn, 8189); err == nil && rep.Equal && rep.AccountsSource > 0 {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	runCancel()
	<-done

	rep, err := verify.Verify(sconn, dconn, 8189)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !rep.Equal {
		t.Fatalf("not byte-identical: %+v", rep.Mismatches)
	}
	t.Logf("verified: %d accounts, %d transfers", rep.AccountsDest, rep.TransferDest)
}
