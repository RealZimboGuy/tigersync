package record

import (
	"testing"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

func TestFromAccountZeroesBalancesAndSetsImported(t *testing.T) {
	a := tb.Account{
		ID: tb.ToUint128(1), Ledger: 1, Code: 7,
		DebitsPosted: tb.ToUint128(500), CreditsPosted: tb.ToUint128(500),
		Timestamp: 42,
	}
	r := FromAccount(a)
	if r.Kind != KindAccount || r.Timestamp != 42 {
		t.Fatalf("kind/timestamp wrong: %+v", r)
	}
	imp := r.ImportedAccount()
	if imp.DebitsPosted != tb.ToUint128(0) || imp.CreditsPosted != tb.ToUint128(0) {
		t.Fatalf("balances not zeroed: %+v", imp)
	}
	f := imp.AccountFlags()
	if !f.Imported || f.Closed {
		t.Fatalf("imported not set / closed not cleared: %+v", f)
	}
	if imp.Timestamp != 42 {
		t.Fatalf("timestamp not preserved")
	}
}

func TestFromTransferSetsImported(t *testing.T) {
	tr := tb.Transfer{ID: tb.ToUint128(9), Amount: tb.ToUint128(10), Ledger: 1, Code: 1, Timestamp: 99}
	r := FromTransfer(tr)
	if r.Kind != KindTransfer || r.Timestamp != 99 {
		t.Fatalf("kind/timestamp wrong: %+v", r)
	}
	imp, err := r.ImportedTransfer()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !imp.TransferFlags().Imported {
		t.Fatalf("imported not set")
	}
}

func TestImportedTransferRejectsTimeout(t *testing.T) {
	tr := tb.Transfer{ID: tb.ToUint128(9), Timeout: 60, Ledger: 1, Code: 1, Timestamp: 99}
	if _, err := FromTransfer(tr).ImportedTransfer(); err == nil {
		t.Fatalf("expected error for non-zero timeout")
	}
}

func TestExpectedFormSetsImportedKeepsBalances(t *testing.T) {
	a := tb.Account{ID: tb.ToUint128(1), Ledger: 1, Code: 1, DebitsPosted: tb.ToUint128(5), Timestamp: 7}
	e := ExpectedAccount(a)
	if !e.AccountFlags().Imported {
		t.Fatalf("imported not set")
	}
	if e.DebitsPosted != tb.ToUint128(5) || e.Timestamp != 7 {
		t.Fatalf("balances/timestamp must be preserved: %+v", e)
	}
	tr := tb.Transfer{ID: tb.ToUint128(2), Amount: tb.ToUint128(3), Ledger: 1, Code: 1, Timestamp: 8}
	et := ExpectedTransfer(tr)
	if !et.TransferFlags().Imported || et.Amount != tb.ToUint128(3) {
		t.Fatalf("expected transfer wrong: %+v", et)
	}
}
