// Package syncengine runs the poll -> merge -> apply loop.
package syncengine

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"
	"tigersync/internal/apply"
	"tigersync/internal/merge"
	"tigersync/internal/observ"
	"tigersync/internal/record"
)

// Reader pages source records ascending by timestamp.
type Reader interface {
	PageAccounts(afterTS uint64, limit uint32) ([]tb.Account, bool, error)
	PageTransfers(afterTS uint64, limit uint32) ([]tb.Transfer, bool, error)
}

// Applier imports merged records and returns how many were applied.
type Applier interface {
	Apply(recs []record.Record) (int, error)
}

type Engine struct {
	reader     Reader
	applier    Applier
	batchLimit int
	margin     time.Duration
	poll       time.Duration
	log        *slog.Logger
}

// step fetches one page of each stream, merges below safeMax, applies, and
// returns the new cursor and number of records applied.
//
// Cursor advancement:
//   - If merge emits fewer records than total fetched (safeMax excluded some),
//     advance to wm so the next step resumes from that safe boundary.
//   - Otherwise all fetched records passed merge; advance to the last record's
//     timestamp so we don't overshoot into unexplored territory.
//   - If no records, advance to wm (idle catch-up).
func (e *Engine) step(cursor, safeMax uint64) (uint64, int, error) {
	accts, aFull, err := e.reader.PageAccounts(cursor, uint32(e.batchLimit))
	if err != nil {
		return cursor, 0, err
	}
	xfers, tFull, err := e.reader.PageTransfers(cursor, uint32(e.batchLimit))
	if err != nil {
		return cursor, 0, err
	}
	totalFetched := len(accts) + len(xfers)
	recs, wm := merge.Merge(accts, aFull, xfers, tFull, safeMax)

	// A linked chain must be applied in one create request. Never emit an
	// incomplete trailing chain: trim it and resume before it next round.
	emit := record.TrimTrailingOpenChain(recs)

	if len(emit) == 0 {
		if len(recs) > 0 {
			// Records fetched but no complete chain among them. If the stream
			// holding the open chain returned a full page, the chain is larger
			// than batch-limit and can never be applied atomically — fatal.
			// Otherwise its tail is not available yet (within the safety
			// margin); wait for the next poll.
			full := tFull
			if recs[0].Kind == record.KindAccount {
				full = aFull
			}
			if full {
				return cursor, 0, fmt.Errorf("linked chain at ts %d exceeds batch-limit %d and cannot be applied atomically",
					recs[0].Timestamp, e.batchLimit)
			}
			return cursor, 0, nil
		}
		// Nothing fetched: advance to the safe boundary when it moved (idle catch-up).
		if wm > cursor {
			return wm, 0, nil
		}
		return cursor, 0, nil
	}

	applied, err := e.applier.Apply(emit)
	if err != nil {
		return cursor, 0, err
	}
	// Cursor advancement:
	//   - If we trimmed an open chain (emit < recs), advance only to the last
	//     applied record — never past the trimmed chain's start.
	//   - Else if safeMax held records back, advance to wm (skip the safe gap).
	//   - Else all fetched records were applied; advance to the last one.
	if len(emit) < len(recs) {
		return emit[len(emit)-1].Timestamp, applied, nil
	}
	if len(recs) < totalFetched {
		return wm, applied, nil
	}
	return recs[len(recs)-1].Timestamp, applied, nil
}

func (e *Engine) safeMax(now time.Time) uint64 {
	return uint64(now.Add(-e.margin).UnixNano())
}

// Run loops until ctx is cancelled, sleeping poll between idle iterations.
func (e *Engine) Run(ctx context.Context, startCursor uint64) error {
	cursor := startCursor
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		next, applied, err := e.step(cursor, e.safeMax(time.Now()))
		if err != nil {
			return err
		}
		cursor = next
		if e.log != nil && applied > 0 {
			e.log.Info("applied", "count", applied, "cursor", cursor)
		}
		if applied == 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(e.poll):
			}
		}
	}
}

// applierAdapter lets *apply.Applier satisfy the Applier interface and feeds
// counters.
type applierAdapter struct {
	inner    *apply.Applier
	counters *observ.Counters
}

func (a applierAdapter) Apply(recs []record.Record) (int, error) {
	s, err := a.inner.Apply(recs)
	if err != nil {
		return 0, err
	}
	a.counters.AddAccounts(int64(s.Accounts))
	a.counters.AddTransfers(int64(s.Transfers))
	a.counters.AddExists(int64(s.Exists))
	a.counters.AddBatches(1)
	return s.Accounts + s.Transfers + s.Exists, nil
}

// New builds an Engine from the real reader and applier.
func New(r Reader, ap *apply.Applier, log *slog.Logger, counters *observ.Counters,
	batchLimit int, margin, poll time.Duration) *Engine {
	return &Engine{
		reader:     r,
		applier:    applierAdapter{inner: ap, counters: counters},
		batchLimit: batchLimit,
		margin:     margin,
		poll:       poll,
		log:        log,
	}
}
