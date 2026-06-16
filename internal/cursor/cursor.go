// Package cursor derives tigersync's resume point from the destination cluster:
// the highest timestamp present across accounts and transfers (0 if empty).
package cursor

import (
	tb "github.com/tigerbeetle/tigerbeetle-go"
	"tigersync/internal/tbclient"
)

func MaxTimestamp(conn *tbclient.Conn) (uint64, error) {
	filter := tb.QueryFilter{
		Limit: 1,
		Flags: tb.QueryFilterFlags{Reversed: true}.ToUint32(),
	}
	var max uint64
	accts, err := conn.Raw().QueryAccounts(filter)
	if err != nil {
		return 0, err
	}
	if len(accts) == 1 {
		max = accts[0].Timestamp
	}
	xfers, err := conn.Raw().QueryTransfers(filter)
	if err != nil {
		return 0, err
	}
	if len(xfers) == 1 && xfers[0].Timestamp > max {
		max = xfers[0].Timestamp
	}
	return max, nil
}
