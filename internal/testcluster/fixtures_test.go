package testcluster

import (
	"context"
	"testing"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

func TestMustCreateHelpers(t *testing.T) {
	if testing.Short() {
		t.Skip("requires docker")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c, err := Start(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	client, _ := tb.NewClient(tb.ToUint128(0), []string{c.Address})
	defer client.Close()

	if err := MustCreateAccounts(client, []tb.Account{
		{ID: tb.ToUint128(1), Ledger: 1, Code: 1},
		{ID: tb.ToUint128(2), Ledger: 1, Code: 1},
	}); err != nil {
		t.Fatalf("create accounts: %v", err)
	}
	if err := MustCreateTransfers(client, []tb.Transfer{
		{ID: tb.ToUint128(10), DebitAccountID: tb.ToUint128(1), CreditAccountID: tb.ToUint128(2), Amount: tb.ToUint128(5), Ledger: 1, Code: 1},
	}); err != nil {
		t.Fatalf("create transfer: %v", err)
	}
	if err := MustCreateTransfers(client, []tb.Transfer{
		{ID: tb.ToUint128(11), DebitAccountID: tb.ToUint128(1), CreditAccountID: tb.ToUint128(1), Amount: tb.ToUint128(1), Ledger: 1, Code: 1},
	}); err == nil {
		t.Fatalf("expected error for self-transfer")
	}
}
