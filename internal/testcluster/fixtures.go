package testcluster

import (
	"fmt"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

// MustCreateAccounts creates accounts and returns an error unless every result
// status is AccountCreated. (Results are returned one-per-event positionally.)
func MustCreateAccounts(c tb.Client, accts []tb.Account) error {
	res, err := c.CreateAccounts(accts)
	if err != nil {
		return err
	}
	for i := range res {
		if res[i].Status != tb.AccountCreated {
			return fmt.Errorf("account[%d] id=%v: %s", i, accts[i].ID, res[i].Status)
		}
	}
	return nil
}

// MustCreateTransfers mirrors MustCreateAccounts for transfers.
func MustCreateTransfers(c tb.Client, xfers []tb.Transfer) error {
	res, err := c.CreateTransfers(xfers)
	if err != nil {
		return err
	}
	for i := range res {
		if res[i].Status != tb.TransferCreated {
			return fmt.Errorf("transfer[%d] id=%v: %s", i, xfers[i].ID, res[i].Status)
		}
	}
	return nil
}

// SeedAllScenarios writes one of every supported, replicable scenario from the
// spec's transaction matrix to the source client. IDs are deterministic. On any
// unexpected create status it returns an error. Amounts are chosen so no
// transfer is rejected.
func SeedAllScenarios(c tb.Client) error {
	id := func(n uint64) tb.Uint128 { return tb.ToUint128(n) }

	if err := MustCreateAccounts(c, []tb.Account{
		{ID: id(1), Ledger: 1, Code: 1},
		{ID: id(2), Ledger: 1, Code: 1, Flags: tb.AccountFlags{DebitsMustNotExceedCredits: true}.ToUint16()},
		{ID: id(3), Ledger: 1, Code: 1, Flags: tb.AccountFlags{CreditsMustNotExceedDebits: true}.ToUint16()},
		{ID: id(4), Ledger: 1, Code: 1, Flags: tb.AccountFlags{History: true}.ToUint16()},
		{ID: id(5), Ledger: 2, Code: 9, UserData128: id(42), UserData64: 7, UserData32: 3},
		{ID: id(6), Ledger: 1, Code: 1},
		{ID: id(8), Ledger: 1, Code: 1, Flags: tb.AccountFlags{Linked: true}.ToUint16()},
		{ID: id(9), Ledger: 1, Code: 1},
	}); err != nil {
		return err
	}

	if err := MustCreateTransfers(c, []tb.Transfer{
		{ID: id(100), DebitAccountID: id(1), CreditAccountID: id(9), Amount: id(50), Ledger: 1, Code: 1},
	}); err != nil {
		return err
	}
	if err := MustCreateTransfers(c, []tb.Transfer{
		{ID: id(101), DebitAccountID: id(1), CreditAccountID: id(9), Amount: id(30), Ledger: 1, Code: 1, Flags: tb.TransferFlags{Pending: true}.ToUint16()},
		{ID: id(102), Amount: id(30), PendingID: id(101), Ledger: 1, Code: 1, Flags: tb.TransferFlags{PostPendingTransfer: true}.ToUint16()},
		{ID: id(103), DebitAccountID: id(1), CreditAccountID: id(9), Amount: id(40), Ledger: 1, Code: 1, Flags: tb.TransferFlags{Pending: true}.ToUint16()},
		{ID: id(104), Amount: id(10), PendingID: id(103), Ledger: 1, Code: 1, Flags: tb.TransferFlags{PostPendingTransfer: true}.ToUint16()},
	}); err != nil {
		return err
	}
	if err := MustCreateTransfers(c, []tb.Transfer{
		{ID: id(105), DebitAccountID: id(1), CreditAccountID: id(9), Amount: id(20), Ledger: 1, Code: 1, Flags: tb.TransferFlags{Pending: true}.ToUint16()},
		{ID: id(106), PendingID: id(105), Ledger: 1, Code: 1, Flags: tb.TransferFlags{VoidPendingTransfer: true}.ToUint16()},
	}); err != nil {
		return err
	}
	if err := MustCreateTransfers(c, []tb.Transfer{
		{ID: id(107), DebitAccountID: id(9), CreditAccountID: id(1), Amount: id(1_000_000), Ledger: 1, Code: 1, Flags: tb.TransferFlags{BalancingDebit: true}.ToUint16()},
	}); err != nil {
		return err
	}
	if err := MustCreateTransfers(c, []tb.Transfer{
		{ID: id(108), DebitAccountID: id(1), CreditAccountID: id(9), Amount: id(1), Ledger: 1, Code: 1, Flags: tb.TransferFlags{Linked: true}.ToUint16()},
		{ID: id(109), DebitAccountID: id(1), CreditAccountID: id(9), Amount: id(1), Ledger: 1, Code: 1},
	}); err != nil {
		return err
	}
	if err := MustCreateTransfers(c, []tb.Transfer{
		{ID: id(110), DebitAccountID: id(1), CreditAccountID: id(9), Amount: id(0), Ledger: 1, Code: 1},
	}); err != nil {
		return err
	}
	if err := MustCreateTransfers(c, []tb.Transfer{
		{ID: id(111), DebitAccountID: id(6), CreditAccountID: id(9), Amount: id(0), Ledger: 1, Code: 1, Flags: tb.TransferFlags{Pending: true, ClosingDebit: true}.ToUint16()},
	}); err != nil {
		return err
	}
	// transfers touching the history account (id 4) so it accrues a real balance
	// history (get_account_balances) for the verifier to check.
	if err := MustCreateTransfers(c, []tb.Transfer{
		{ID: id(120), DebitAccountID: id(1), CreditAccountID: id(4), Amount: id(15), Ledger: 1, Code: 1},
		{ID: id(121), DebitAccountID: id(4), CreditAccountID: id(9), Amount: id(5), Ledger: 1, Code: 1},
	}); err != nil {
		return err
	}
	return nil
}
