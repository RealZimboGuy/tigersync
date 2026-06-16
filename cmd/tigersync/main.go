// Command tigersync continuously mirrors a source TigerBeetle cluster to a
// fresh destination cluster, byte-identical including timestamps, via queries.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tigersync/internal/apply"
	"tigersync/internal/config"
	"tigersync/internal/cursor"
	"tigersync/internal/observ"
	"tigersync/internal/reader"
	"tigersync/internal/syncengine"
	"tigersync/internal/tbclient"
)

func main() {
	cfg, err := config.Parse(os.Args[1:])
	log := observ.NewLogger(envOr("LOG_LEVEL", cfg.LogLevel))
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(2)
	}

	sconn, err := tbclient.Connect(cfg.SourceCluster, cfg.SourceAddresses)
	if err != nil {
		log.Error("connect source", "err", err)
		os.Exit(1)
	}
	defer sconn.Close()
	dconn, err := tbclient.Connect(cfg.DestCluster, cfg.DestAddresses)
	if err != nil {
		log.Error("connect dest", "err", err)
		os.Exit(1)
	}
	defer dconn.Close()

	start, err := cursor.MaxTimestamp(dconn)
	if err != nil {
		log.Error("derive cursor", "err", err)
		os.Exit(1)
	}
	counters := observ.NewCounters()
	log.Info("starting", "cursor", start, "source", cfg.SourceAddresses, "dest", cfg.DestAddresses,
		"poll", cfg.PollInterval.String(), "margin", cfg.SafetyMargin.String(), "batch", cfg.BatchLimit)

	eng := syncengine.New(reader.New(sconn), apply.New(dconn, cfg.BatchLimit),
		log, counters, cfg.BatchLimit, cfg.SafetyMargin, cfg.PollInterval)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Force-exit watchdog: the TigerBeetle client blocks and retries internally
	// when a cluster is unreachable, so a graceful shutdown can stall inside a
	// client call that never returns. After the first signal, give shutdown a
	// short grace window, then exit hard so Ctrl-C always stops the process.
	go func() {
		<-ctx.Done()
		time.Sleep(2 * time.Second)
		log.Warn("forced exit: graceful shutdown did not complete (cluster unreachable?)")
		os.Exit(130)
	}()

	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s := counters.Snapshot()
				log.Info("stats", "accounts", s.Accounts, "transfers", s.Transfers,
					"exists", s.Exists, "batches", s.Batches, "errors", s.Errors)
			}
		}
	}()

	err = eng.Run(ctx, start)
	f := counters.Snapshot()
	log.Info("stopping", "accounts", f.Accounts, "transfers", f.Transfers, "exists", f.Exists,
		"batches", f.Batches, "errors", f.Errors, "reason", err)
	if err != nil && err != context.Canceled {
		os.Exit(1)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
