// Package config parses tigersync's CLI/env configuration.
package config

import (
	"flag"
	"fmt"
	"strings"
	"time"
)

const MaxBatch = 8189 // TigerBeetle per-request event limit.

type Config struct {
	SourceAddresses []string
	SourceCluster   uint64
	DestAddresses   []string
	DestCluster     uint64
	PollInterval    time.Duration
	BatchLimit      int
	SafetyMargin    time.Duration
	LogLevel        string
}

func Parse(args []string) (Config, error) {
	fs := flag.NewFlagSet("tigersync", flag.ContinueOnError)
	var (
		srcAddr  = fs.String("source-addresses", "", "comma-separated source replica addresses")
		srcClust = fs.Uint64("source-cluster", 0, "source cluster id")
		dstAddr  = fs.String("dest-addresses", "", "comma-separated destination replica addresses")
		dstClust = fs.Uint64("dest-cluster", 0, "destination cluster id")
		poll     = fs.Duration("poll-interval", 250*time.Millisecond, "idle poll cadence")
		batch    = fs.Int("batch-limit", MaxBatch, "page/apply batch size (<= 8189)")
		margin   = fs.Duration("safety-margin", 2*time.Second, "LagGuard hold-back window")
		level    = fs.String("log-level", "info", "log level: debug|info|warn|error")
	)
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if *srcAddr == "" || *dstAddr == "" {
		return Config{}, fmt.Errorf("both --source-addresses and --dest-addresses are required")
	}
	if *batch < 1 || *batch > MaxBatch {
		return Config{}, fmt.Errorf("--batch-limit must be in [1, %d]", MaxBatch)
	}
	return Config{
		SourceAddresses: strings.Split(*srcAddr, ","),
		SourceCluster:   *srcClust,
		DestAddresses:   strings.Split(*dstAddr, ","),
		DestCluster:     *dstClust,
		PollInterval:    *poll,
		BatchLimit:      *batch,
		SafetyMargin:    *margin,
		LogLevel:        *level,
	}, nil
}
