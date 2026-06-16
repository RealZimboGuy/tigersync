// Package merge interleaves the source's account and transfer pages into one
// global-timestamp-ordered stream, emitting only records below a safe
// watermark so a later-fetched, earlier-timestamped record can never have been
// skipped.
package merge

import (
	tb "github.com/tigerbeetle/tigerbeetle-go"
	"tigersync/internal/record"
)

// Merge takes ascending-by-timestamp account and transfer pages, whether each
// page hit the page limit (full), and safeMax (= destination now - margin).
// The watermark is min(accountFrontier, transferFrontier, safeMax); a stream's
// frontier is its last timestamp if its page was full (complete only through
// there) or safeMax if short (drained). Returns records with timestamp <=
// watermark, merged ascending, plus the watermark.
func Merge(accounts []tb.Account, accountsFull bool,
	transfers []tb.Transfer, transfersFull bool,
	safeMax uint64) ([]record.Record, uint64) {

	aFront := safeMax
	if accountsFull && len(accounts) > 0 {
		aFront = accounts[len(accounts)-1].Timestamp
	}
	tFront := safeMax
	if transfersFull && len(transfers) > 0 {
		tFront = transfers[len(transfers)-1].Timestamp
	}
	wm := safeMax
	if aFront < wm {
		wm = aFront
	}
	if tFront < wm {
		wm = tFront
	}

	var out []record.Record
	i, j := 0, 0
	for i < len(accounts) || j < len(transfers) {
		aOK := i < len(accounts) && accounts[i].Timestamp <= wm
		tOK := j < len(transfers) && transfers[j].Timestamp <= wm
		switch {
		case aOK && (!tOK || accounts[i].Timestamp < transfers[j].Timestamp):
			out = append(out, record.FromAccount(accounts[i]))
			i++
		case tOK:
			out = append(out, record.FromTransfer(transfers[j]))
			j++
		default:
			return out, wm
		}
	}
	return out, wm
}
