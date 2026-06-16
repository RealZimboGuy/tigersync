package config

import (
	"testing"
	"time"
)

func TestParseDefaultsAndOverrides(t *testing.T) {
	c, err := Parse([]string{
		"--source-addresses", "127.0.0.1:3000", "--source-cluster", "0",
		"--dest-addresses", "127.0.0.1:3001", "--dest-cluster", "0",
		"--poll-interval", "250ms", "--batch-limit", "8189", "--safety-margin", "2s",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.SourceAddresses[0] != "127.0.0.1:3000" || c.DestAddresses[0] != "127.0.0.1:3001" {
		t.Fatalf("addresses wrong: %+v", c)
	}
	if c.PollInterval != 250*time.Millisecond || c.SafetyMargin != 2*time.Second || c.BatchLimit != 8189 {
		t.Fatalf("values wrong: %+v", c)
	}
}

func TestBatchLimitCappedAt8189(t *testing.T) {
	if _, err := Parse([]string{"--source-addresses", "a", "--dest-addresses", "b", "--batch-limit", "9000"}); err == nil {
		t.Fatalf("expected error for batch-limit > 8189")
	}
}

func TestMissingAddressesIsError(t *testing.T) {
	if _, err := Parse([]string{"--source-addresses", "a"}); err == nil {
		t.Fatalf("expected error for missing dest addresses")
	}
}
