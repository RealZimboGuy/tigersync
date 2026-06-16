package testcluster

import (
	"context"
	"testing"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

// TestImportedRoundTrip writes accounts + a transfer to a source, reads them
// back, re-creates them on a destination with the imported flag, and asserts
// byte-identity including timestamp.
func TestImportedRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("requires docker")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	src, err := Start(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Stop()
	dst, err := Start(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Stop()

	sc, _ := tb.NewClient(tb.ToUint128(0), []string{src.Address})
	defer sc.Close()
	dc, _ := tb.NewClient(tb.ToUint128(0), []string{dst.Address})
	defer dc.Close()

	a1, a2 := tb.ToUint128(1), tb.ToUint128(2)
	if err := MustCreateAccounts(sc, []tb.Account{
		{ID: a1, Ledger: 1, Code: 1},
		{ID: a2, Ledger: 1, Code: 1},
	}); err != nil {
		t.Fatal(err)
	}
	if err := MustCreateTransfers(sc, []tb.Transfer{
		{ID: tb.ToUint128(10), DebitAccountID: a1, CreditAccountID: a2, Amount: tb.ToUint128(100), Ledger: 1, Code: 1},
	}); err != nil {
		t.Fatal(err)
	}

	srcAccts, _ := sc.LookupAccounts([]tb.Uint128{a1, a2})
	srcXfers, _ := sc.LookupTransfers([]tb.Uint128{tb.ToUint128(10)})
	if len(srcAccts) != 2 || len(srcXfers) != 1 {
		t.Fatalf("readback wrong: %d accts %d xfers", len(srcAccts), len(srcXfers))
	}

	imp := make([]tb.Account, len(srcAccts))
	for i, a := range srcAccts {
		a.DebitsPending, a.DebitsPosted = tb.ToUint128(0), tb.ToUint128(0)
		a.CreditsPending, a.CreditsPosted = tb.ToUint128(0), tb.ToUint128(0)
		f := a.AccountFlags()
		f.Imported, f.Closed = true, false
		a.Flags = f.ToUint16()
		imp[i] = a
	}
	if err := MustCreateAccounts(dc, imp); err != nil {
		t.Fatalf("import accounts: %v", err)
	}
	it := srcXfers[0]
	if it.Timeout != 0 {
		t.Fatal("fixture transfer unexpectedly has a timeout")
	}
	tf := it.TransferFlags()
	tf.Imported = true
	it.Flags = tf.ToUint16()
	if err := MustCreateTransfers(dc, []tb.Transfer{it}); err != nil {
		t.Fatalf("import transfer: %v", err)
	}

	// The destination must equal the source in every field EXCEPT that the
	// imported bit is necessarily set on the destination (TigerBeetle persists
	// it). Compare against the source with its imported bit set.
	dstAccts, _ := dc.LookupAccounts([]tb.Uint128{a1, a2})
	for i := range srcAccts {
		want := srcAccts[i]
		wf := want.AccountFlags()
		wf.Imported = true
		want.Flags = wf.ToUint16()
		if want != dstAccts[i] {
			t.Fatalf("account %d differs:\n want=%+v\n dst=%+v", i, want, dstAccts[i])
		}
	}
	dstXfers, _ := dc.LookupTransfers([]tb.Uint128{tb.ToUint128(10)})
	wantX := srcXfers[0]
	wxf := wantX.TransferFlags()
	wxf.Imported = true
	wantX.Flags = wxf.ToUint16()
	if wantX != dstXfers[0] {
		t.Fatalf("transfer differs:\n want=%+v\n dst=%+v", wantX, dstXfers[0])
	}
}
