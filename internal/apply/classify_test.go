package apply

import (
	"testing"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

func TestClassifyTransfers(t *testing.T) {
	created, exists, fatal := classifyTransfers([]tb.CreateTransferResult{
		{Status: tb.TransferCreated},
		{Status: tb.TransferExists},
		{Status: tb.TransferExistsWithDifferentAmount},
	})
	if created != 1 || exists != 1 || fatal != 2 {
		t.Fatalf("created=%d exists=%d fatal=%d", created, exists, fatal)
	}
}

func TestClassifyTransfersAllOK(t *testing.T) {
	created, exists, fatal := classifyTransfers([]tb.CreateTransferResult{
		{Status: tb.TransferCreated}, {Status: tb.TransferCreated},
	})
	if created != 2 || exists != 0 || fatal != -1 {
		t.Fatalf("created=%d exists=%d fatal=%d", created, exists, fatal)
	}
}

func TestClassifyAccounts(t *testing.T) {
	created, exists, fatal := classifyAccounts([]tb.CreateAccountResult{
		{Status: tb.AccountCreated},
		{Status: tb.AccountExistsWithDifferentLedger},
	})
	if created != 1 || exists != 0 || fatal != 1 {
		t.Fatalf("created=%d exists=%d fatal=%d", created, exists, fatal)
	}
}
