// Package record provides a unified view over TigerBeetle accounts and
// transfers ordered by their global timestamp, plus the transforms to imported.
package record

import (
	"fmt"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

type Kind int

const (
	KindAccount Kind = iota
	KindTransfer
)

// Record is one source object tagged by kind and keyed by global timestamp.
type Record struct {
	Kind      Kind
	Timestamp uint64
	account   tb.Account
	transfer  tb.Transfer
}

func FromAccount(a tb.Account) Record {
	return Record{Kind: KindAccount, Timestamp: a.Timestamp, account: a}
}

func FromTransfer(t tb.Transfer) Record {
	return Record{Kind: KindTransfer, Timestamp: t.Timestamp, transfer: t}
}

// Linked reports whether this record's `linked` flag is set, i.e. it is chained
// to the next event in its source request and must be applied in the same
// create call as the rest of its chain (the chain ends at the first member
// whose Linked is false).
func (r Record) Linked() bool {
	if r.Kind == KindAccount {
		return r.account.AccountFlags().Linked
	}
	return r.transfer.TransferFlags().Linked
}

// ImportedAccount rewrites the account for the create call: balances zeroed
// (TigerBeetle recomputes them from transfers), Imported set, Closed cleared
// (closure is reproduced by replaying closing transfers), timestamp preserved.
func (r Record) ImportedAccount() tb.Account {
	a := r.account
	a.DebitsPending = tb.ToUint128(0)
	a.DebitsPosted = tb.ToUint128(0)
	a.CreditsPending = tb.ToUint128(0)
	a.CreditsPosted = tb.ToUint128(0)
	f := a.AccountFlags()
	f.Imported = true
	f.Closed = false
	a.Flags = f.ToUint16()
	return a
}

// ImportedTransfer rewrites the transfer for the create call: Imported set,
// timestamp preserved. Errors if the source transfer has a non-zero timeout,
// which cannot be imported (spec: pending-timeout limitation -> fatal halt).
func (r Record) ImportedTransfer() (tb.Transfer, error) {
	t := r.transfer
	if t.Timeout != 0 {
		return tb.Transfer{}, fmt.Errorf("transfer %v has non-zero timeout %d: not importable", t.ID, t.Timeout)
	}
	f := t.TransferFlags()
	f.Imported = true
	t.Flags = f.ToUint16()
	return t, nil
}

// ExpectedAccount returns the source account as it must appear on the
// destination after import: identical in every field, with the imported bit set
// (TigerBeetle persists that bit; balances/closed are kept because the
// destination reproduces them via replayed transfers). Used by the verifier and
// integration tests — distinct from ImportedAccount, the create-time transform.
func ExpectedAccount(src tb.Account) tb.Account {
	f := src.AccountFlags()
	f.Imported = true
	src.Flags = f.ToUint16()
	return src
}

// ExpectedTransfer mirrors ExpectedAccount for transfers.
func ExpectedTransfer(src tb.Transfer) tb.Transfer {
	f := src.TransferFlags()
	f.Imported = true
	src.Flags = f.ToUint16()
	return src
}
