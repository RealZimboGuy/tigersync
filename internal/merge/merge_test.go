package merge

import (
	"testing"

	tb "github.com/tigerbeetle/tigerbeetle-go"
	"tigersync/internal/record"
)

func acct(ts uint64) tb.Account  { return tb.Account{ID: tb.ToUint128(ts), Timestamp: ts} }
func xfer(ts uint64) tb.Transfer { return tb.Transfer{ID: tb.ToUint128(ts), Timestamp: ts} }

func tsOf(recs []record.Record) []uint64 {
	out := make([]uint64, len(recs))
	for i, r := range recs {
		out[i] = r.Timestamp
	}
	return out
}

func equal(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestMergeBothFull(t *testing.T) {
	accts := []tb.Account{acct(1), acct(4), acct(7)}
	xfers := []tb.Transfer{xfer(2), xfer(3), xfer(5)}
	recs, wm := Merge(accts, true, xfers, true, 1_000_000)
	if wm != 5 {
		t.Fatalf("watermark = %d, want 5", wm)
	}
	if !equal(tsOf(recs), []uint64{1, 2, 3, 4, 5}) {
		t.Fatalf("got %v", tsOf(recs))
	}
}

func TestMergeShortPagesCappedBySafeMax(t *testing.T) {
	accts := []tb.Account{acct(1), acct(4)}
	xfers := []tb.Transfer{xfer(2)}
	recs, wm := Merge(accts, false, xfers, false, 3)
	if wm != 3 {
		t.Fatalf("watermark = %d, want 3", wm)
	}
	if !equal(tsOf(recs), []uint64{1, 2}) {
		t.Fatalf("got %v want [1 2]", tsOf(recs))
	}
}

func TestMergeOneEmpty(t *testing.T) {
	xfers := []tb.Transfer{xfer(2), xfer(8)}
	recs, wm := Merge(nil, false, xfers, false, 5)
	if wm != 5 {
		t.Fatalf("watermark = %d, want 5", wm)
	}
	if !equal(tsOf(recs), []uint64{2}) {
		t.Fatalf("got %v want [2]", tsOf(recs))
	}
}
