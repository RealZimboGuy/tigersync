package apply

import "tigersync/internal/record"

type batchGroup struct {
	kind record.Kind
	recs []record.Record
}

// group packs records into same-kind batches of at most limit, preserving
// global order and NEVER splitting a linked chain across a batch boundary
// (a chain must be submitted in one create request). It works in atomic units
// — a unit is a single non-linked record or a complete linked chain — and
// starts a new batch when the next unit would change kind or overflow limit.
// recs must not end mid-chain (the caller trims open chains first); a single
// chain longer than limit is placed in its own batch and will be rejected by
// the cluster, which the sync engine guards against upstream.
func group(recs []record.Record, limit int) []batchGroup {
	var groups []batchGroup
	for _, unit := range record.ChainUnits(recs) {
		kind := unit[0].Kind
		n := len(groups)
		if n == 0 || groups[n-1].kind != kind || len(groups[n-1].recs)+len(unit) > limit {
			groups = append(groups, batchGroup{kind: kind, recs: append([]record.Record{}, unit...)})
			continue
		}
		groups[n-1].recs = append(groups[n-1].recs, unit...)
	}
	return groups
}
