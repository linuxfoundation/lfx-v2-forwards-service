// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

// TestClient_Flush_BrokerDown verifies that when the broker is unreachable the
// client (configured like New: RetryOnFailedConnect, unlimited reconnects) comes
// up in a reconnecting state and Flush returns an error within its timeout
// instead of hanging. Startup relies on this to retry the flush while /readyz
// stays 503.
func TestClient_Flush_BrokerDown(t *testing.T) {
	// Port 1 is reserved and nothing listens on it, so the connect fails.
	conn, err := nats.Connect("nats://127.0.0.1:1",
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(50*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("Connect with RetryOnFailedConnect returned %v, want a reconnecting conn", err)
	}
	defer conn.Close()

	c := &Client{conn: conn}

	start := time.Now()
	if err := c.Flush(200 * time.Millisecond); err == nil {
		t.Fatal("Flush with the broker down = nil, want an error")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Flush took %v, want it bounded by its 200ms timeout", elapsed)
	}
}

func TestClient_Flush_NilConn(t *testing.T) {
	if err := (&Client{}).Flush(time.Millisecond); err == nil {
		t.Error("Flush with no connection = nil, want an error")
	}
}
