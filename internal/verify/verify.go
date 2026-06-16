// Package verify proves byte-identity between a source and destination cluster
// by scanning all accounts and transfers in timestamp order and comparing each
// destination record with the EXPECTED IMPORTED FORM of the source record
// (source record with its imported bit set) using == (all other fields,
// including timestamp, must match exactly). The imported bit is necessarily set
// on the destination because it is the only timestamp-preserving mechanism.
//
// For accounts with flags.history it additionally compares the full balance
// history (get_account_balances), which the destination reproduces by replaying
// the transfers.
package verify

import (
	"fmt"

	tb "github.com/tigerbeetle/tigerbeetle-go"
	"tigersync/internal/reader"
	"tigersync/internal/record"
	"tigersync/internal/tbclient"
)

type Report struct {
	Equal           bool
	AccountsSource  int
	AccountsDest    int
	TransferSource  int
	TransferDest    int
	HistoryAccounts int // accounts with flags.history whose balance history was checked
	Mismatches      []string
}

const maxMismatches = 20

func Verify(src, dst *tbclient.Conn, pageLimit int) (Report, error) {
	rep := Report{Equal: true}

	srcA, err := allAccounts(src, pageLimit)
	if err != nil {
		return rep, err
	}
	dstA, err := allAccounts(dst, pageLimit)
	if err != nil {
		return rep, err
	}
	rep.AccountsSource, rep.AccountsDest = len(srcA), len(dstA)
	if len(srcA) != len(dstA) {
		rep.Equal = false
		rep.Mismatches = append(rep.Mismatches, fmt.Sprintf("account count: src=%d dst=%d", len(srcA), len(dstA)))
	}
	for i := 0; i < min(len(srcA), len(dstA)); i++ {
		if record.ExpectedAccount(srcA[i]) != dstA[i] {
			rep.Equal = false
			if len(rep.Mismatches) < maxMismatches {
				rep.Mismatches = append(rep.Mismatches, fmt.Sprintf("account #%d: expected=%+v dst=%+v", i, record.ExpectedAccount(srcA[i]), dstA[i]))
			}
		}
	}

	srcT, err := allTransfers(src, pageLimit)
	if err != nil {
		return rep, err
	}
	dstT, err := allTransfers(dst, pageLimit)
	if err != nil {
		return rep, err
	}
	rep.TransferSource, rep.TransferDest = len(srcT), len(dstT)
	if len(srcT) != len(dstT) {
		rep.Equal = false
		rep.Mismatches = append(rep.Mismatches, fmt.Sprintf("transfer count: src=%d dst=%d", len(srcT), len(dstT)))
	}
	for i := 0; i < min(len(srcT), len(dstT)); i++ {
		if record.ExpectedTransfer(srcT[i]) != dstT[i] {
			rep.Equal = false
			if len(rep.Mismatches) < maxMismatches {
				rep.Mismatches = append(rep.Mismatches, fmt.Sprintf("transfer #%d: expected=%+v dst=%+v", i, record.ExpectedTransfer(srcT[i]), dstT[i]))
			}
		}
	}

	// For accounts with flags.history, the per-transfer balance snapshots
	// (get_account_balances) are part of the replicated state. They are not
	// copied directly — they are reproduced by replaying the transfers against a
	// history-flagged account — so we verify them explicitly.
	for i := range srcA {
		if !srcA[i].AccountFlags().History {
			continue
		}
		rep.HistoryAccounts++
		sb, err := allBalances(src, srcA[i].ID, pageLimit)
		if err != nil {
			return rep, err
		}
		db, err := allBalances(dst, srcA[i].ID, pageLimit)
		if err != nil {
			return rep, err
		}
		if len(sb) != len(db) {
			rep.Equal = false
			rep.Mismatches = append(rep.Mismatches, fmt.Sprintf("history count for account %v: src=%d dst=%d", srcA[i].ID, len(sb), len(db)))
			continue
		}
		for j := range sb {
			if sb[j] != db[j] {
				rep.Equal = false
				if len(rep.Mismatches) < maxMismatches {
					rep.Mismatches = append(rep.Mismatches, fmt.Sprintf("history snapshot %d for account %v: src=%+v dst=%+v", j, srcA[i].ID, sb[j], db[j]))
				}
			}
		}
	}
	return rep, nil
}

// allBalances pages the full balance history of one history-flagged account,
// ascending by timestamp.
func allBalances(c *tbclient.Conn, id tb.Uint128, limit int) ([]tb.AccountBalance, error) {
	var out []tb.AccountBalance
	var after uint64
	for {
		page, err := c.Raw().GetAccountBalances(tb.AccountFilter{
			AccountID:    id,
			TimestampMin: after + 1,
			Limit:        uint32(limit),
			Flags:        tb.AccountFilterFlags{Debits: true, Credits: true}.ToUint32(),
		})
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if uint32(len(page)) < uint32(limit) {
			return out, nil
		}
		after = page[len(page)-1].Timestamp
	}
}

func allAccounts(c *tbclient.Conn, limit int) ([]tb.Account, error) {
	r := reader.New(c)
	var out []tb.Account
	var after uint64
	for {
		page, full, err := r.PageAccounts(after, uint32(limit))
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if !full {
			return out, nil
		}
		after = page[len(page)-1].Timestamp
	}
}

func allTransfers(c *tbclient.Conn, limit int) ([]tb.Transfer, error) {
	r := reader.New(c)
	var out []tb.Transfer
	var after uint64
	for {
		page, full, err := r.PageTransfers(after, uint32(limit))
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if !full {
			return out, nil
		}
		after = page[len(page)-1].Timestamp
	}
}
