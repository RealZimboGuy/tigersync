// Package reader paginates the source's accounts and transfers ascending by
// global timestamp using a match-all query filter.
package reader

import (
	tb "github.com/tigerbeetle/tigerbeetle-go"
	"tigersync/internal/tbclient"
)

type Reader struct{ conn *tbclient.Conn }

func New(conn *tbclient.Conn) *Reader { return &Reader{conn: conn} }

// matchAll matches every record with timestamp > afterTS, ascending, up to
// limit. All selective fields are zero (ignored).
func matchAll(afterTS uint64, limit uint32) tb.QueryFilter {
	return tb.QueryFilter{
		TimestampMin: afterTS + 1,
		TimestampMax: 0, // 0 = no upper bound
		Limit:        limit,
		Flags:        tb.QueryFilterFlags{Reversed: false}.ToUint32(),
	}
}

func (r *Reader) PageAccounts(afterTS uint64, limit uint32) ([]tb.Account, bool, error) {
	a, err := r.conn.Raw().QueryAccounts(matchAll(afterTS, limit))
	if err != nil {
		return nil, false, err
	}
	return a, uint32(len(a)) == limit, nil
}

func (r *Reader) PageTransfers(afterTS uint64, limit uint32) ([]tb.Transfer, bool, error) {
	x, err := r.conn.Raw().QueryTransfers(matchAll(afterTS, limit))
	if err != nil {
		return nil, false, err
	}
	return x, uint32(len(x)) == limit, nil
}
