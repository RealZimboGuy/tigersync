package apply

import (
	"fmt"

	tb "github.com/tigerbeetle/tigerbeetle-go"
	"tigersync/internal/record"
	"tigersync/internal/tbclient"
)

// Stats summarises one Apply call.
type Stats struct {
	Accounts  int // accounts newly created
	Transfers int // transfers newly created
	Exists    int // records already present (idempotent)
}

type Applier struct {
	conn  *tbclient.Conn
	limit int
}

func New(conn *tbclient.Conn, limit int) *Applier { return &Applier{conn: conn, limit: limit} }

// Apply imports records (already in ascending global-timestamp order) to the
// destination. Any non-created/non-exists result, or a transfer with a non-zero
// timeout, is a fatal error that stops the sync.
func (a *Applier) Apply(recs []record.Record) (Stats, error) {
	var s Stats
	for _, g := range group(recs, a.limit) {
		switch g.kind {
		case record.KindAccount:
			batch := make([]tb.Account, len(g.recs))
			for i, r := range g.recs {
				batch[i] = r.ImportedAccount()
			}
			res, err := a.conn.Raw().CreateAccounts(batch)
			if err != nil {
				return s, fmt.Errorf("create accounts: %w", err)
			}
			created, exists, fatal := classifyAccounts(res)
			if fatal != -1 {
				return s, fmt.Errorf("fatal account result %s at ts %d (id %v)",
					res[fatal].Status, batch[fatal].Timestamp, batch[fatal].ID)
			}
			s.Accounts += created
			s.Exists += exists
		case record.KindTransfer:
			batch := make([]tb.Transfer, len(g.recs))
			for i, r := range g.recs {
				t, err := r.ImportedTransfer()
				if err != nil {
					return s, err // fatal: timeout transfer
				}
				batch[i] = t
			}
			res, err := a.conn.Raw().CreateTransfers(batch)
			if err != nil {
				return s, fmt.Errorf("create transfers: %w", err)
			}
			created, exists, fatal := classifyTransfers(res)
			if fatal != -1 {
				return s, fmt.Errorf("fatal transfer result %s at ts %d (id %v)",
					res[fatal].Status, batch[fatal].Timestamp, batch[fatal].ID)
			}
			s.Transfers += created
			s.Exists += exists
		}
	}
	return s, nil
}
