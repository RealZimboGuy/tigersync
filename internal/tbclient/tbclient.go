// Package tbclient wraps the TigerBeetle Go client for tigersync.
package tbclient

import (
	tb "github.com/tigerbeetle/tigerbeetle-go"
)

type Conn struct {
	client tb.Client
}

func Connect(clusterID uint64, addresses []string) (*Conn, error) {
	c, err := tb.NewClient(tb.ToUint128(clusterID), addresses)
	if err != nil {
		return nil, err
	}
	return &Conn{client: c}, nil
}

func (c *Conn) Raw() tb.Client { return c.client }
func (c *Conn) Close()         { c.client.Close() }
