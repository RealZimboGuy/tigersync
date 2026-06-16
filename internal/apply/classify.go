// Package apply writes records to the destination as imported events, in
// strictly-increasing timestamp order, and classifies the results.
//
// Create operations return one result per input event (positionally). Success
// is Created or Exists; anything else is a fatal divergence.
package apply

import (
	tb "github.com/tigerbeetle/tigerbeetle-go"
)

// classifyAccounts returns counts of created and idempotent-exists results and
// the index of the first fatal result (-1 if none).
func classifyAccounts(results []tb.CreateAccountResult) (created, exists, fatalIndex int) {
	fatalIndex = -1
	for i := range results {
		switch results[i].Status {
		case tb.AccountCreated:
			created++
		case tb.AccountExists:
			exists++
		default:
			if fatalIndex == -1 {
				fatalIndex = i
			}
		}
	}
	return
}

func classifyTransfers(results []tb.CreateTransferResult) (created, exists, fatalIndex int) {
	fatalIndex = -1
	for i := range results {
		switch results[i].Status {
		case tb.TransferCreated:
			created++
		case tb.TransferExists:
			exists++
		default:
			if fatalIndex == -1 {
				fatalIndex = i
			}
		}
	}
	return
}
