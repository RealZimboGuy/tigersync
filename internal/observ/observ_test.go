package observ

import (
	"sync"
	"testing"
)

func TestCountersAccumulate(t *testing.T) {
	c := NewCounters()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.AddAccounts(1)
			c.AddTransfers(2)
			c.AddExists(1)
			c.AddBatches(1)
		}()
	}
	wg.Wait()
	s := c.Snapshot()
	if s.Accounts != 100 || s.Transfers != 200 || s.Exists != 100 || s.Batches != 100 {
		t.Fatalf("snapshot wrong: %+v", s)
	}
}
