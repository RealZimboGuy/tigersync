// Command loadgen writes a high volume of accounts and mixed transfers to a
// TigerBeetle cluster to exercise tigersync under load.
package main

import (
	"flag"
	"log"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

func main() {
	addr := flag.String("addresses", "127.0.0.1:3000", "cluster addresses")
	cluster := flag.Uint64("cluster", 0, "cluster id")
	numAccounts := flag.Int("accounts", 10_000, "accounts to create")
	numTransfers := flag.Int("transfers", 1_000_000, "transfers to create")
	batch := flag.Int("batch", 8189, "events per request")
	flag.Parse()

	c, err := tb.NewClient(tb.ToUint128(*cluster), []string{*addr})
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	start := time.Now()
	for i := 0; i < *numAccounts; i += *batch {
		var b []tb.Account
		for j := i; j < i+*batch && j < *numAccounts; j++ {
			b = append(b, tb.Account{ID: tb.ToUint128(uint64(j + 1)), Ledger: 1, Code: 1})
		}
		res, err := c.CreateAccounts(b)
		if err != nil {
			log.Fatalf("accounts: %v", err)
		}
		for k := range res {
			if res[k].Status != tb.AccountCreated {
				log.Fatalf("account create status: %s", res[k].Status)
			}
		}
	}
	log.Printf("created %d accounts in %s", *numAccounts, time.Since(start))

	start = time.Now()
	id := uint64(1)
	for i := 0; i < *numTransfers; i += *batch {
		var b []tb.Transfer
		for j := i; j < i+*batch && j < *numTransfers; j++ {
			d := uint64(j%*numAccounts) + 1
			cr := uint64((j+1)%*numAccounts) + 1
			if d == cr {
				cr = (cr % uint64(*numAccounts)) + 1
			}
			b = append(b, tb.Transfer{ID: tb.ToUint128(1_000_000_000 + id),
				DebitAccountID: tb.ToUint128(d), CreditAccountID: tb.ToUint128(cr),
				Amount: tb.ToUint128(1), Ledger: 1, Code: 1})
			id++
		}
		res, err := c.CreateTransfers(b)
		if err != nil {
			log.Fatalf("transfers: %v", err)
		}
		for k := range res {
			if res[k].Status != tb.TransferCreated {
				log.Fatalf("transfer create status: %s", res[k].Status)
			}
		}
	}
	log.Printf("created %d transfers in %s", *numTransfers, time.Since(start))
}
