// Package observ provides structured logging and concurrency-safe running
// counters for tigersync.
package observ

import (
	"log/slog"
	"os"
	"sync/atomic"
)

// NewLogger returns a JSON slog logger at the given level writing to stdout.
func NewLogger(level string) *slog.Logger {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l}))
}

type Counters struct {
	accounts  atomic.Int64
	transfers atomic.Int64
	exists    atomic.Int64
	batches   atomic.Int64
	errors    atomic.Int64
}

func NewCounters() *Counters { return &Counters{} }

func (c *Counters) AddAccounts(n int64)  { c.accounts.Add(n) }
func (c *Counters) AddTransfers(n int64) { c.transfers.Add(n) }
func (c *Counters) AddExists(n int64)    { c.exists.Add(n) }
func (c *Counters) AddBatches(n int64)   { c.batches.Add(n) }
func (c *Counters) AddErrors(n int64)    { c.errors.Add(n) }

type Snapshot struct {
	Accounts, Transfers, Exists, Batches, Errors int64
}

func (c *Counters) Snapshot() Snapshot {
	return Snapshot{
		Accounts:  c.accounts.Load(),
		Transfers: c.transfers.Load(),
		Exists:    c.exists.Load(),
		Batches:   c.batches.Load(),
		Errors:    c.errors.Load(),
	}
}
