package syncengine

import (
	"testing"

	tb "github.com/tigerbeetle/tigerbeetle-go"
	"tigersync/internal/record"
)

func linkedX(ts uint64, linked bool) tb.Transfer {
	return tb.Transfer{ID: tb.ToUint128(ts), Timestamp: ts, Flags: tb.TransferFlags{Linked: linked}.ToUint16()}
}

// A linked chain straddling safeMax must be held back as a whole: the head is
// not applied until its tail is safely below the margin.
func TestStepHoldsBackChainStraddlingSafeMax(t *testing.T) {
	fr := &fakeReader{xfers: []tb.Transfer{
		{ID: tb.ToUint128(1), Timestamp: 1}, // standalone, safely below margin
		linkedX(4, true),                    // chain head (<= safeMax)
		linkedX(7, false),                   // chain tail (> safeMax this round)
	}}
	ap := &recordingApplier{}
	e := &Engine{reader: fr, applier: ap, batchLimit: 10}

	cur, applied, err := e.step(0, 5)
	if err != nil {
		t.Fatal(err)
	}
	if applied != 1 || cur != 1 {
		t.Fatalf("chain head leaked: applied=%d cursor=%d (want 1 and 1)", applied, cur)
	}

	cur, applied, err = e.step(cur, 100)
	if err != nil {
		t.Fatal(err)
	}
	if applied != 2 || cur != 7 {
		t.Fatalf("chain not applied whole: applied=%d cursor=%d (want 2 and 7)", applied, cur)
	}
}

// A linked chain larger than batch-limit can never be applied atomically and
// must be a fatal error, not an infinite loop.
func TestStepFatalOnChainExceedingBatchLimit(t *testing.T) {
	fr := &fakeReader{xfers: []tb.Transfer{linkedX(1, true), linkedX(2, true), linkedX(3, true)}}
	e := &Engine{reader: fr, applier: &recordingApplier{}, batchLimit: 2}
	if _, _, err := e.step(0, 1_000_000); err == nil {
		t.Fatalf("expected fatal error for chain exceeding batch-limit")
	}
}

type fakeReader struct {
	accts []tb.Account
	xfers []tb.Transfer
}

func (f *fakeReader) PageAccounts(after uint64, limit uint32) ([]tb.Account, bool, error) {
	var out []tb.Account
	for _, a := range f.accts {
		if a.Timestamp > after && uint32(len(out)) < limit {
			out = append(out, a)
		}
	}
	return out, uint32(len(out)) == limit, nil
}
func (f *fakeReader) PageTransfers(after uint64, limit uint32) ([]tb.Transfer, bool, error) {
	var out []tb.Transfer
	for _, x := range f.xfers {
		if x.Timestamp > after && uint32(len(out)) < limit {
			out = append(out, x)
		}
	}
	return out, uint32(len(out)) == limit, nil
}

type recordingApplier struct{ applied []record.Record }

func (r *recordingApplier) Apply(recs []record.Record) (int, error) {
	r.applied = append(r.applied, recs...)
	return len(recs), nil
}

func TestStepDrainsAndAdvances(t *testing.T) {
	fr := &fakeReader{
		accts: []tb.Account{{ID: tb.ToUint128(1), Timestamp: 1}, {ID: tb.ToUint128(4), Timestamp: 4}},
		xfers: []tb.Transfer{{ID: tb.ToUint128(2), Timestamp: 2}},
	}
	ap := &recordingApplier{}
	e := &Engine{reader: fr, applier: ap, batchLimit: 10}
	newCursor, applied, err := e.step(0, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if applied != 3 || newCursor != 4 {
		t.Fatalf("applied=%d cursor=%d, want 3 and 4", applied, newCursor)
	}
	if len(ap.applied) != 3 || ap.applied[0].Timestamp != 1 || ap.applied[2].Timestamp != 4 {
		t.Fatalf("order wrong: %+v", ap.applied)
	}
}

func TestStepRespectsSafeMax(t *testing.T) {
	fr := &fakeReader{accts: []tb.Account{{ID: tb.ToUint128(1), Timestamp: 1}, {ID: tb.ToUint128(9), Timestamp: 9}}}
	ap := &recordingApplier{}
	e := &Engine{reader: fr, applier: ap, batchLimit: 10}
	newCursor, applied, err := e.step(0, 5)
	if err != nil {
		t.Fatal(err)
	}
	if applied != 1 || newCursor != 5 {
		t.Fatalf("applied=%d cursor=%d, want 1 and 5", applied, newCursor)
	}
}
