package record

import (
	"testing"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

func linkedXfer(ts uint64, linked bool) Record {
	f := tb.TransferFlags{Linked: linked}
	return FromTransfer(tb.Transfer{ID: tb.ToUint128(ts), Ledger: 1, Code: 1, Timestamp: ts, Flags: f.ToUint16()})
}

func tss(recs []Record) []uint64 {
	out := make([]uint64, len(recs))
	for i, r := range recs {
		out[i] = r.Timestamp
	}
	return out
}

func TestTrimTrailingOpenChain(t *testing.T) {
	// [single(1), chainHead(2,linked), chainTail(3)] -> all complete
	full := []Record{linkedXfer(1, false), linkedXfer(2, true), linkedXfer(3, false)}
	if got := TrimTrailingOpenChain(full); len(got) != 3 {
		t.Fatalf("complete set trimmed: %v", tss(got))
	}
	// trailing open chain: [single(1), head(2,linked)] -> drop the open head
	open := []Record{linkedXfer(1, false), linkedXfer(2, true)}
	got := TrimTrailingOpenChain(open)
	if len(got) != 1 || got[0].Timestamp != 1 {
		t.Fatalf("open trailing chain not trimmed: %v", tss(got))
	}
	// entirely one open chain -> nil
	if got := TrimTrailingOpenChain([]Record{linkedXfer(1, true), linkedXfer(2, true)}); got != nil {
		t.Fatalf("whole-open-chain should trim to nil: %v", tss(got))
	}
}

func TestChainUnits(t *testing.T) {
	// single(1), chain(2->3->4), single(5)
	recs := []Record{
		linkedXfer(1, false),
		linkedXfer(2, true), linkedXfer(3, true), linkedXfer(4, false),
		linkedXfer(5, false),
	}
	units := ChainUnits(recs)
	if len(units) != 3 {
		t.Fatalf("want 3 units, got %d: %v", len(units), units)
	}
	if len(units[0]) != 1 || len(units[1]) != 3 || len(units[2]) != 1 {
		t.Fatalf("unit sizes wrong: %d %d %d", len(units[0]), len(units[1]), len(units[2]))
	}
}
