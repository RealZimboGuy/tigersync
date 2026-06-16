// Package testcluster spins up disposable single-replica TigerBeetle clusters
// in Docker for tests.
package testcluster

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"
)

const image = "ghcr.io/tigerbeetle/tigerbeetle:0.17.6"

// Cluster is a running single-replica TigerBeetle in a Docker container.
type Cluster struct {
	Address     string
	containerID string
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// Start formats a data file and starts TigerBeetle with the given clusterID,
// returning once the port accepts TCP connections.
func Start(ctx context.Context, clusterID uint64) (*Cluster, error) {
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	name := fmt.Sprintf("tigersync-test-%d", port)
	vol := name + "-data"

	format := exec.CommandContext(ctx, "docker", "run", "--rm",
		"-v", vol+":/data", image,
		"format",
		fmt.Sprintf("--cluster=%d", clusterID),
		"--replica=0", "--replica-count=1", "/data/0.tigerbeetle")
	if out, err := format.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("format: %v: %s", err, out)
	}

	run := exec.CommandContext(ctx, "docker", "run", "-d", "--name", name,
		"-v", vol+":/data",
		"-p", fmt.Sprintf("127.0.0.1:%d:3000", port), image,
		"start", "--addresses=0.0.0.0:3000", "/data/0.tigerbeetle")
	out, err := run.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("start: %v: %s", err, out)
	}
	c := &Cluster{
		Address:     fmt.Sprintf("127.0.0.1:%d", port),
		containerID: strings.TrimSpace(string(out)),
	}
	if err := waitReady(ctx, c.Address); err != nil {
		c.Stop()
		return nil, err
	}
	return c, nil
}

func waitReady(ctx context.Context, addr string) error {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if conn, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
			conn.Close()
			time.Sleep(500 * time.Millisecond)
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	return fmt.Errorf("cluster at %s not ready", addr)
}

// Stop removes the container and prunes its data volume.
func (c *Cluster) Stop() {
	if c.containerID != "" {
		exec.Command("docker", "rm", "-f", c.containerID).Run()
	}
	exec.Command("docker", "volume", "prune", "-f").Run()
}
