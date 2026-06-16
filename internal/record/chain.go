package record

// A linked chain is a run of records where every member except the last has
// Linked()==true; the run terminates at the first member with Linked()==false.
// TigerBeetle commits a chain atomically and assigns its members consecutive
// timestamps, so by global-timestamp order a chain is always a contiguous,
// same-kind run. The whole chain must be submitted in one create request, or
// TigerBeetle rejects it with a LinkedEventChainOpen error.

// TrimTrailingOpenChain returns the longest prefix of recs that ends at a chain
// boundary, dropping any trailing records that form an incomplete chain (one
// whose terminating, non-linked member is not present in recs). Returns nil if
// every record is linked (the whole slice is a single open chain).
func TrimTrailingOpenChain(recs []Record) []Record {
	last := -1
	for i := range recs {
		if !recs[i].Linked() {
			last = i
		}
	}
	if last < 0 {
		return nil
	}
	return recs[:last+1]
}

// ChainUnits splits recs into atomic units, each either a single non-linked
// record or a complete linked chain. recs must not end mid-chain — call
// TrimTrailingOpenChain first. Each returned unit is a contiguous slice of recs.
func ChainUnits(recs []Record) [][]Record {
	var units [][]Record
	i := 0
	for i < len(recs) {
		j := i
		for j < len(recs) && recs[j].Linked() {
			j++
		}
		if j < len(recs) {
			j++ // include the chain terminator (or the standalone non-linked record)
		}
		units = append(units, recs[i:j])
		i = j
	}
	return units
}
