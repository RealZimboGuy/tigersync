package apply

import (
	"testing"

	tb "github.com/tigerbeetle/tigerbeetle-go"
	"tigersync/internal/record"
)

func a(ts uint64) record.Record { return record.FromAccount(tb.Account{ID: tb.ToUint128(ts), Ledger: 1, Code: 1, Timestamp: ts}) }
func x(ts uint64) record.Record { return record.FromTransfer(tb.Transfer{ID: tb.ToUint128(ts), Ledger: 1, Code: 1, Timestamp: ts}) }
func xl(ts uint64, linked bool) record.Record {
	return record.FromTransfer(tb.Transfer{ID: tb.ToUint128(ts), Ledger: 1, Code: 1, Timestamp: ts, Flags: tb.TransferFlags{Linked: linked}.ToUint16()})
}

// A linked chain longer than the batch limit must NOT be split — it stays in
// one group (the engine guards against chains genuinely exceeding the limit).
func TestGroupKeepsLinkedChainTogetherPastLimit(t *testing.T) {
	groups := group([]record.Record{xl(1, true), xl(2, true), xl(3, false)}, 2)
	if len(groups) != 1 || len(groups[0].recs) != 3 {
		t.Fatalf("chain was split: %d groups, group0 size %d", len(groups), len(groups[0].recs))
	}
}

// A chain that would overflow the current batch starts a fresh batch rather
// than being cut.
func TestGroupChainStartsNewBatchOnOverflow(t *testing.T) {
	groups := group([]record.Record{xl(1, false), xl(2, true), xl(3, false)}, 2)
	if len(groups) != 2 || len(groups[0].recs) != 1 || len(groups[1].recs) != 2 {
		t.Fatalf("unexpected grouping: %d groups sizes %v", len(groups), groups)
	}
}

func TestGroupRunsByKindPreservingOrder(t *testing.T) {
	groups := group([]record.Record{a(1), a(2), x(3), a(4), x(5), x(6)}, 8189)
	if len(groups) != 4 {
		t.Fatalf("want 4 groups, got %d", len(groups))
	}
	if groups[0].kind != record.KindAccount || len(groups[0].recs) != 2 {
		t.Fatalf("group0: %+v", groups[0])
	}
	if groups[1].kind != record.KindTransfer || len(groups[1].recs) != 1 {
		t.Fatalf("group1: %+v", groups[1])
	}
	if groups[3].kind != record.KindTransfer || len(groups[3].recs) != 2 {
		t.Fatalf("group3: %+v", groups[3])
	}
}

func TestGroupSplitsAtBatchLimit(t *testing.T) {
	var recs []record.Record
	for ts := uint64(1); ts <= 5; ts++ {
		recs = append(recs, a(ts))
	}
	if got := len(group(recs, 2)); got != 3 {
		t.Fatalf("want 3 groups, got %d", got)
	}
}
